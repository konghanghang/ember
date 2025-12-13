/**
 * 订阅管理 Server Actions 单元测试
 *
 * 注意：Server Actions 涉及数据库操作，这里使用 mock 进行单元测试
 * 集成测试应在实际数据库环境中进行
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { MediaType, SubscriptionStatus } from '@prisma/client'

// Mock 依赖
vi.mock('@/lib/db', () => ({
  prisma: {
    subscription: {
      create: vi.fn(),
      findMany: vi.fn(),
      findUnique: vi.fn(),
      update: vi.fn(),
      delete: vi.fn(),
    },
    log: {
      create: vi.fn(),
    },
  },
}))

vi.mock('@/lib/auth', () => ({
  getServerAuth: vi.fn(),
}))

vi.mock('@/lib/moviepilot', () => ({
  moviepilotClient: {
    isConfigured: vi.fn(),
    createSubscription: vi.fn(),
  },
}))

import { prisma } from '@/lib/db'
import { getServerAuth } from '@/lib/auth'
import { moviepilotClient } from '@/lib/moviepilot'
import {
  createSubscription,
  getUserSubscriptions,
  deleteSubscription,
  getAllSubscriptions,
  approveSubscription,
  rejectSubscription,
} from '../subscriptions'

describe('Subscription Server Actions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('createSubscription', () => {
    it('应该成功创建订阅（用户已登录）', async () => {
      // Mock 用户认证
      vi.mocked(getServerAuth).mockResolvedValue({
        id: 'user-123',
        username: 'testuser',
        role: 'user',
      })

      // Mock 数据库操作
      vi.mocked(prisma.subscription.create).mockResolvedValue({
        id: 'sub-123',
        userId: 'user-123',
        type: MediaType.MOVIE,
        name: '测试电影',
        tmdbId: '278',
        status: SubscriptionStatus.PENDING,
        note: null,
        mpError: null,
        createdAt: new Date(),
        updatedAt: new Date(),
      })

      const result = await createSubscription({
        type: MediaType.MOVIE,
        name: '测试电影',
        tmdbId: '278',
      })

      expect(result.success).toBe(true)
      expect(prisma.subscription.create).toHaveBeenCalledWith({
        data: {
          userId: 'user-123',
          type: MediaType.MOVIE,
          name: '测试电影',
          tmdbId: '278',
          note: undefined,
          status: 'PENDING',
        },
      })
      expect(prisma.log.create).toHaveBeenCalled()
    })

    it('应该拒绝未登录用户', async () => {
      vi.mocked(getServerAuth).mockResolvedValue(null)

      const result = await createSubscription({
        type: MediaType.MOVIE,
        name: '测试',
        tmdbId: '123',
      })

      expect(result.success).toBe(false)
      expect(result.error).toBe('未登录或权限不足')
      expect(prisma.subscription.create).not.toHaveBeenCalled()
    })

    it('应该拒绝管理员用户', async () => {
      vi.mocked(getServerAuth).mockResolvedValue({
        id: 'admin-123',
        username: 'admin',
        role: 'admin',
      })

      const result = await createSubscription({
        type: MediaType.MOVIE,
        name: '测试',
        tmdbId: '123',
      })

      expect(result.success).toBe(false)
      expect(result.error).toBe('未登录或权限不足')
    })

    it('应该验证必填字段', async () => {
      vi.mocked(getServerAuth).mockResolvedValue({
        id: 'user-123',
        username: 'testuser',
        role: 'user',
      })

      const result = await createSubscription({
        type: MediaType.MOVIE,
        name: '',
        tmdbId: '',
      })

      expect(result.success).toBe(false)
      expect(result.error).toBe('影视名称和 TMDB ID 为必填项')
    })
  })

  describe('getUserSubscriptions', () => {
    it('应该返回用户的订阅列表', async () => {
      vi.mocked(getServerAuth).mockResolvedValue({
        id: 'user-123',
        username: 'testuser',
        role: 'user',
      })

      const mockSubscriptions = [
        {
          id: 'sub-1',
          userId: 'user-123',
          type: MediaType.MOVIE,
          name: '电影1',
          tmdbId: '123',
          status: SubscriptionStatus.PENDING,
          note: null,
          mpError: null,
          createdAt: new Date(),
          updatedAt: new Date(),
        },
      ]

      vi.mocked(prisma.subscription.findMany).mockResolvedValue(
        mockSubscriptions
      )

      const result = await getUserSubscriptions()

      expect(result).toEqual(mockSubscriptions)
      expect(prisma.subscription.findMany).toHaveBeenCalledWith({
        where: { userId: 'user-123' },
        orderBy: { createdAt: 'desc' },
      })
    })

    it('应该返回 null 当用户未登录', async () => {
      vi.mocked(getServerAuth).mockResolvedValue(null)

      const result = await getUserSubscriptions()

      expect(result).toBeNull()
    })
  })

  describe('deleteSubscription', () => {
    it('应该成功删除 PENDING 状态的订阅', async () => {
      vi.mocked(getServerAuth).mockResolvedValue({
        id: 'user-123',
        username: 'testuser',
        role: 'user',
      })

      vi.mocked(prisma.subscription.findUnique).mockResolvedValue({
        id: 'sub-123',
        userId: 'user-123',
        type: MediaType.MOVIE,
        name: '测试',
        tmdbId: '123',
        status: SubscriptionStatus.PENDING,
        note: null,
        mpError: null,
        createdAt: new Date(),
        updatedAt: new Date(),
      })

      const result = await deleteSubscription('sub-123')

      expect(result.success).toBe(true)
      expect(prisma.subscription.delete).toHaveBeenCalledWith({
        where: { id: 'sub-123' },
      })
    })

    it('应该拒绝删除 APPROVED 状态的订阅', async () => {
      vi.mocked(getServerAuth).mockResolvedValue({
        id: 'user-123',
        username: 'testuser',
        role: 'user',
      })

      vi.mocked(prisma.subscription.findUnique).mockResolvedValue({
        id: 'sub-123',
        userId: 'user-123',
        type: MediaType.MOVIE,
        name: '测试',
        tmdbId: '123',
        status: SubscriptionStatus.APPROVED,
        note: null,
        mpError: null,
        createdAt: new Date(),
        updatedAt: new Date(),
      })

      const result = await deleteSubscription('sub-123')

      expect(result.success).toBe(false)
      expect(result.error).toBe('只能删除待审核的订阅')
      expect(prisma.subscription.delete).not.toHaveBeenCalled()
    })

    it('应该拒绝删除他人的订阅', async () => {
      vi.mocked(getServerAuth).mockResolvedValue({
        id: 'user-123',
        username: 'testuser',
        role: 'user',
      })

      vi.mocked(prisma.subscription.findUnique).mockResolvedValue({
        id: 'sub-123',
        userId: 'other-user',
        type: MediaType.MOVIE,
        name: '测试',
        tmdbId: '123',
        status: SubscriptionStatus.PENDING,
        note: null,
        mpError: null,
        createdAt: new Date(),
        updatedAt: new Date(),
      })

      const result = await deleteSubscription('sub-123')

      expect(result.success).toBe(false)
      expect(result.error).toBe('无权删除此订阅')
    })
  })

  describe('approveSubscription', () => {
    it('应该成功审核通过订阅（MoviePilot API 成功）', async () => {
      vi.mocked(getServerAuth).mockResolvedValue({
        id: 'admin-123',
        username: 'admin',
        role: 'admin',
      })

      vi.mocked(prisma.subscription.findUnique).mockResolvedValue({
        id: 'sub-123',
        userId: 'user-123',
        type: MediaType.MOVIE,
        name: '测试电影',
        tmdbId: '278',
        status: SubscriptionStatus.PENDING,
        note: null,
        mpError: null,
        createdAt: new Date(),
        updatedAt: new Date(),
      })

      vi.mocked(moviepilotClient.isConfigured).mockReturnValue(true)
      vi.mocked(moviepilotClient.createSubscription).mockResolvedValue({
        success: true,
      })

      const result = await approveSubscription('sub-123')

      expect(result.success).toBe(true)
      expect(moviepilotClient.createSubscription).toHaveBeenCalledWith({
        type: 'movie',
        name: '测试电影',
        tmdbid: '278',
      })
      expect(prisma.subscription.update).toHaveBeenCalledWith({
        where: { id: 'sub-123' },
        data: {
          status: 'APPROVED',
          mpError: null,
        },
      })
    })

    it('应该处理 MoviePilot API 失败（保存错误信息）', async () => {
      vi.mocked(getServerAuth).mockResolvedValue({
        id: 'admin-123',
        username: 'admin',
        role: 'admin',
      })

      vi.mocked(prisma.subscription.findUnique).mockResolvedValue({
        id: 'sub-123',
        userId: 'user-123',
        type: MediaType.MOVIE,
        name: '测试',
        tmdbId: '123',
        status: SubscriptionStatus.PENDING,
        note: null,
        mpError: null,
        createdAt: new Date(),
        updatedAt: new Date(),
      })

      vi.mocked(moviepilotClient.isConfigured).mockReturnValue(true)
      vi.mocked(moviepilotClient.createSubscription).mockRejectedValue(
        new Error('MoviePilot API 错误: 500')
      )

      const result = await approveSubscription('sub-123')

      expect(result.success).toBe(true) // 仍然返回成功
      expect(prisma.subscription.update).toHaveBeenCalledWith({
        where: { id: 'sub-123' },
        data: {
          status: 'APPROVED',
          mpError: 'MoviePilot API 错误: 500',
        },
      })
    })

    it('应该处理 MoviePilot 未配置的情况', async () => {
      vi.mocked(getServerAuth).mockResolvedValue({
        id: 'admin-123',
        username: 'admin',
        role: 'admin',
      })

      vi.mocked(prisma.subscription.findUnique).mockResolvedValue({
        id: 'sub-123',
        userId: 'user-123',
        type: MediaType.TV,
        name: '测试剧集',
        tmdbId: '456',
        status: SubscriptionStatus.PENDING,
        note: null,
        mpError: null,
        createdAt: new Date(),
        updatedAt: new Date(),
      })

      vi.mocked(moviepilotClient.isConfigured).mockReturnValue(false)

      const result = await approveSubscription('sub-123')

      expect(result.success).toBe(true)
      expect(moviepilotClient.createSubscription).not.toHaveBeenCalled()
      expect(prisma.subscription.update).toHaveBeenCalledWith({
        where: { id: 'sub-123' },
        data: {
          status: 'APPROVED',
          mpError: 'MoviePilot 未配置',
        },
      })
    })

    it('应该拒绝非管理员用户', async () => {
      vi.mocked(getServerAuth).mockResolvedValue({
        id: 'user-123',
        username: 'testuser',
        role: 'user',
      })

      const result = await approveSubscription('sub-123')

      expect(result.success).toBe(false)
      expect(result.error).toBe('未登录或权限不足')
    })

    it('应该拒绝审核非 PENDING 状态的订阅', async () => {
      vi.mocked(getServerAuth).mockResolvedValue({
        id: 'admin-123',
        username: 'admin',
        role: 'admin',
      })

      vi.mocked(prisma.subscription.findUnique).mockResolvedValue({
        id: 'sub-123',
        userId: 'user-123',
        type: MediaType.MOVIE,
        name: '测试',
        tmdbId: '123',
        status: SubscriptionStatus.APPROVED,
        note: null,
        mpError: null,
        createdAt: new Date(),
        updatedAt: new Date(),
      })

      const result = await approveSubscription('sub-123')

      expect(result.success).toBe(false)
      expect(result.error).toBe('订阅已被处理')
    })
  })

  describe('rejectSubscription', () => {
    it('应该成功拒绝订阅', async () => {
      vi.mocked(getServerAuth).mockResolvedValue({
        id: 'admin-123',
        username: 'admin',
        role: 'admin',
      })

      vi.mocked(prisma.subscription.findUnique).mockResolvedValue({
        id: 'sub-123',
        userId: 'user-123',
        type: MediaType.MOVIE,
        name: '测试',
        tmdbId: '123',
        status: SubscriptionStatus.PENDING,
        note: null,
        mpError: null,
        createdAt: new Date(),
        updatedAt: new Date(),
      })

      const result = await rejectSubscription('sub-123')

      expect(result.success).toBe(true)
      expect(prisma.subscription.update).toHaveBeenCalledWith({
        where: { id: 'sub-123' },
        data: { status: 'REJECTED' },
      })
      expect(prisma.log.create).toHaveBeenCalled()
    })
  })
})
