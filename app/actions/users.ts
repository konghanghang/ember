'use server'

import { prisma } from '@/lib/db'
import { embyClient } from '@/lib/emby'
import { validateInvite } from './invites'
import { add } from 'date-fns'

/**
 * 用户注册
 * @param data 注册信息
 * @returns { success: boolean, user?: User, error?: string }
 */
export async function registerUser(data: {
  username: string
  password: string
  email: string
  inviteCode: string
}) {
  let embyUserId: string | null = null

  try {
    // 1. 验证邀请码
    const inviteValidation = await validateInvite(data.inviteCode)
    if (!inviteValidation.success || !inviteValidation.invite) {
      return {
        success: false,
        error: inviteValidation.error || '邀请码无效',
      }
    }

    const invite = inviteValidation.invite

    // 2. 检查用户名是否已存在（数据库）
    const existingUser = await prisma.user.findUnique({
      where: { username: data.username },
    })

    if (existingUser) {
      return {
        success: false,
        error: '用户名已存在',
      }
    }

    // 3. 先创建 Emby 用户（事务外，可补偿）
    const embyUser = await embyClient.createUser({
      Name: data.username,
      Password: data.password,
    })
    embyUserId = embyUser.Id

    // 4. 设置用户密码（Emby API 创建用户时不会设置密码）
    await embyClient.setUserPassword(embyUserId, data.password)

    // 5. 应用默认用户策略
    const { EmbyClient } = await import('@/lib/emby')
    await embyClient.setUserPolicy(embyUserId, EmbyClient.getDefaultPolicy())

    // 6. 数据库事务：保存用户 + 更新邀请码（带并发保护）
    const user = await prisma.$transaction(async (tx) => {
      // 6.1 原子操作：更新邀请码使用次数（防止并发超额使用）
      const updateResult = await tx.invite.updateMany({
        where: {
          code: data.inviteCode,
          usedCount: {
            lt: invite.maxUses, // 确保还有剩余次数
          },
          // 如果有过期时间，确保未过期
          ...(invite.expiresAt && {
            expiresAt: {
              gte: new Date(),
            },
          }),
        },
        data: {
          usedCount: {
            increment: 1,
          },
        },
      })

      // 6.2 检查是否更新成功（并发情况下可能失败）
      if (updateResult.count === 0) {
        throw new Error('邀请码已用尽或已过期（并发冲突）')
      }

      // 6.3 保存到数据库
      const newUser = await tx.user.create({
        data: {
          username: data.username,
          email: data.email,
          embyId: embyUserId!,
          inviteCode: data.inviteCode,
          expiresAt: add(new Date(), { days: invite.defaultDays }),
        },
      })

      // 6.4 记录日志
      await tx.log.create({
        data: {
          action: 'create_user',
          targetId: newUser.id,
          details: {
            username: newUser.username,
            email: newUser.email,
            embyId: newUser.embyId,
            inviteCode: data.inviteCode,
            expiresAt: newUser.expiresAt?.toISOString(),
            embyPolicy: 'default',
          },
        },
      })

      return newUser
    })

    return {
      success: true,
      user,
    }
  } catch (error) {
    console.error('用户注册失败：', error)

    // 补偿逻辑：如果 Emby 用户已创建，但数据库失败，则删除 Emby 用户
    if (embyUserId) {
      try {
        console.log(`[Cleanup] 删除 Emby 孤儿用户: ${embyUserId}`)
        await embyClient.deleteUser(embyUserId)
        console.log(`[Cleanup] Emby 用户清理成功: ${embyUserId}`)
      } catch (cleanupError) {
        // 清理失败，记录错误日志（需要人工介入）
        console.error(`[Cleanup] 清理 Emby 孤儿用户失败: ${embyUserId}`, cleanupError)

        // 尝试记录到数据库（如果数据库可用）
        try {
          await prisma.log.create({
            data: {
              action: 'orphan_emby_user',
              targetId: embyUserId,
              details: {
                username: data.username,
                embyId: embyUserId,
                error: '注册失败后无法清理 Emby 用户，需要人工删除',
                originalError: error instanceof Error ? error.message : String(error),
              },
            },
          })
        } catch {
          // 数据库也不可用，只能打日志
          console.error(`[Cleanup] 无法记录孤儿用户日志，请手动检查 Emby 用户: ${embyUserId}`)
        }
      }
    }

    // 区分 Emby API 错误和其他错误
    const errorMessage =
      error instanceof Error && error.message.includes('Emby')
        ? 'Emby 用户创建失败，请检查 Emby 服务器状态'
        : '用户注册失败，请稍后重试'

    return {
      success: false,
      error: errorMessage,
    }
  }
}

/**
 * 获取用户列表
 * @param params 查询参数 { search?: string }
 * @returns 用户列表
 */
export async function getUsers(params?: { search?: string }) {
  try {
    const users = await prisma.user.findMany({
      where: params?.search
        ? {
            username: {
              contains: params.search,
              mode: 'insensitive',
            },
          }
        : undefined,
      include: {
        invite: true,
      },
      orderBy: {
        createdAt: 'desc',
      },
    })

    return {
      success: true,
      users,
    }
  } catch (error) {
    console.error('获取用户列表失败：', error)
    return {
      success: false,
      error: '获取用户列表失败',
      users: [],
    }
  }
}

/**
 * 延长到期时间
 * @param userId 用户 ID
 * @param days 延长天数
 * @returns { success: boolean, error?: string }
 */
export async function extendExpiry(userId: string, days: number) {
  try {
    const user = await prisma.user.findUnique({
      where: { id: userId },
    })

    if (!user) {
      return {
        success: false,
        error: '用户不存在',
      }
    }

    // 计算新的到期时间
    const newExpiresAt = add(user.expiresAt || new Date(), { days })

    // 判断是否需要重新启用账号
    const shouldReactivate = !user.isActive && newExpiresAt > new Date()

    // 1. 如果需要重新启用，先同步到 Emby（失败则直接返回）
    if (shouldReactivate) {
      try {
        await embyClient.setUserPolicy(user.embyId, {
          IsDisabled: false,
        })
      } catch (embyError) {
        console.error('Emby 重新启用失败：', embyError)
        return {
          success: false,
          error: 'Emby 服务器状态更新失败，请稍后重试',
        }
      }
    }

    // 2. Emby 更新成功（或无需更新）后，再更新数据库
    try {
      await prisma.$transaction(async (tx) => {
        // 更新到期时间（如果需要则同时更新状态）
        await tx.user.update({
          where: { id: userId },
          data: {
            expiresAt: newExpiresAt,
            ...(shouldReactivate && { isActive: true }),
          },
        })

        // 记录日志
        await tx.log.create({
          data: {
            action: 'extend_expiry',
            targetId: userId,
            details: {
              username: user.username,
              days,
              oldExpiresAt: user.expiresAt?.toISOString(),
              newExpiresAt: newExpiresAt.toISOString(),
              reactivated: shouldReactivate,
            },
          },
        })
      })

      return {
        success: true,
      }
    } catch (dbError) {
      // 数据库更新失败，如果已重新启用 Emby，尝试回滚
      console.error('数据库更新失败（尝试回滚 Emby）：', dbError)

      if (shouldReactivate) {
        try {
          // 回滚 Emby 状态到禁用
          await embyClient.setUserPolicy(user.embyId, {
            IsDisabled: true,
          })
          console.log(`[Rollback] Emby 状态已回滚（重新禁用）: ${user.embyId}`)
        } catch (rollbackError) {
          // 回滚失败，记录日志
          console.error(
            `[Rollback] Emby 状态回滚失败: ${user.embyId}`,
            rollbackError
          )

          try {
            await prisma.log.create({
              data: {
                action: 'sync_conflict',
                targetId: userId,
                details: {
                  username: user.username,
                  embyId: user.embyId,
                  error: 'Emby 已重新启用但数据库更新失败，且 Emby 回滚失败',
                  embyStatus: 'enabled',
                  dbStatus: 'disabled',
                },
              },
            })
          } catch {
            console.error(
              `[Rollback] 无法记录同步冲突日志，请手动检查用户: ${userId}`
            )
          }
        }
      }

      return {
        success: false,
        error: '数据库更新失败，请稍后重试',
      }
    }
  } catch (error) {
    console.error('延长到期时间失败：', error)
    return {
      success: false,
      error: '延长到期时间失败，请稍后重试',
    }
  }
}

/**
 * 禁用/启用用户
 * @param userId 用户 ID
 * @returns { success: boolean, error?: string }
 */
export async function toggleUserStatus(userId: string) {
  try {
    const user = await prisma.user.findUnique({
      where: { id: userId },
    })

    if (!user) {
      return {
        success: false,
        error: '用户不存在',
      }
    }

    // 计算目标状态
    const targetStatus = !user.isActive
    const targetEmbyStatus = !targetStatus // IsDisabled 和 isActive 相反

    // 1. 先同步到 Emby（失败则直接返回）
    try {
      await embyClient.setUserPolicy(user.embyId, {
        IsDisabled: targetEmbyStatus,
      })
    } catch (embyError) {
      console.error('Emby 状态更新失败：', embyError)
      return {
        success: false,
        error: 'Emby 服务器状态更新失败，请稍后重试',
      }
    }

    // 2. Emby 更新成功后，再更新数据库
    try {
      await prisma.$transaction(async (tx) => {
        // 更新数据库
        await tx.user.update({
          where: { id: userId },
          data: {
            isActive: targetStatus,
          },
        })

        // 记录日志
        await tx.log.create({
          data: {
            action: user.isActive ? 'disable_user' : 'enable_user',
            targetId: userId,
            details: {
              username: user.username,
              embyId: user.embyId,
            },
          },
        })
      })

      return {
        success: true,
      }
    } catch (dbError) {
      // 数据库更新失败，尝试回滚 Emby 状态
      console.error('数据库状态更新失败（尝试回滚 Emby）：', dbError)

      try {
        // 回滚 Emby 状态到原始值
        await embyClient.setUserPolicy(user.embyId, {
          IsDisabled: !user.isActive, // 恢复原状态
        })
        console.log(`[Rollback] Emby 状态已回滚: ${user.embyId}`)
      } catch (rollbackError) {
        // 回滚失败，记录日志
        console.error(`[Rollback] Emby 状态回滚失败: ${user.embyId}`, rollbackError)

        try {
          await prisma.log.create({
            data: {
              action: 'sync_conflict',
              targetId: userId,
              details: {
                username: user.username,
                embyId: user.embyId,
                error: 'Emby 状态已更新但数据库更新失败，且 Emby 回滚失败',
                embyStatus: targetEmbyStatus ? 'disabled' : 'enabled',
                dbStatus: user.isActive ? 'active' : 'inactive',
              },
            },
          })
        } catch {
          console.error(
            `[Rollback] 无法记录同步冲突日志，请手动检查用户: ${userId}`
          )
        }
      }

      return {
        success: false,
        error: '数据库更新失败，请稍后重试',
      }
    }
  } catch (error) {
    console.error('切换用户状态失败：', error)
    return {
      success: false,
      error: '操作失败，请稍后重试',
    }
  }
}

/**
 * 删除用户
 * @param userId 用户 ID
 * @returns { success: boolean, error?: string }
 */
export async function deleteUser(userId: string) {
  try {
    const user = await prisma.user.findUnique({
      where: { id: userId },
    })

    if (!user) {
      return {
        success: false,
        error: '用户不存在',
      }
    }

    // 1. 先删除 Emby 用户（失败则直接返回，不影响数据库）
    try {
      await embyClient.deleteUser(user.embyId)
    } catch (embyError) {
      console.error('删除 Emby 用户失败：', embyError)
      return {
        success: false,
        error: 'Emby 用户删除失败，请检查 Emby 服务器状态后重试',
      }
    }

    // 2. Emby 删除成功后，再删除数据库记录
    try {
      await prisma.$transaction(async (tx) => {
        // 删除数据库记录
        await tx.user.delete({
          where: { id: userId },
        })

        // 记录日志
        await tx.log.create({
          data: {
            action: 'delete_user',
            targetId: userId,
            details: {
              username: user.username,
              email: user.email,
              embyId: user.embyId,
            },
          },
        })
      })

      return {
        success: true,
      }
    } catch (dbError) {
      // 数据库删除失败，但 Emby 已删除（留下孤儿数据库记录）
      console.error('删除数据库记录失败（Emby 用户已删除）：', dbError)

      // 记录孤儿数据库记录（如果可能）
      try {
        await prisma.log.create({
          data: {
            action: 'orphan_db_record',
            targetId: userId,
            details: {
              username: user.username,
              embyId: user.embyId,
              error: 'Emby 用户已删除，但数据库记录删除失败，需要人工清理',
              originalError:
                dbError instanceof Error ? dbError.message : String(dbError),
            },
          },
        })
      } catch {
        console.error(
          `[Cleanup] 无法记录孤儿数据库记录，请手动检查用户: ${userId}`
        )
      }

      return {
        success: false,
        error: '数据库记录删除失败，但 Emby 用户已删除，请联系管理员处理',
      }
    }
  } catch (error) {
    console.error('删除用户失败：', error)
    return {
      success: false,
      error: '删除用户失败，请稍后重试',
    }
  }
}
