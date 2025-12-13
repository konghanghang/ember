'use client'

import { useEffect, useState } from 'react'
import {
  getUserAuth,
  updateUserPassword,
  updateUserEmail,
} from '@/app/actions/user-auth'
import { format, differenceInDays } from 'date-fns'
import { zhCN } from 'date-fns/locale'

interface UserInfo {
  id: string
  username: string
  email: string | null
  embyId: string
  isActive: boolean
  expiresAt: Date | null
  createdAt: Date
  inviteCode?: string
}

export default function UserDashboardPage() {
  const [loading, setLoading] = useState(true)
  const [user, setUser] = useState<UserInfo | null>(null)
  const [error, setError] = useState('')

  // 修改密码表单状态
  const [passwordForm, setPasswordForm] = useState({
    currentPassword: '',
    newPassword: '',
    confirmPassword: '',
  })
  const [passwordLoading, setPasswordLoading] = useState(false)
  const [passwordError, setPasswordError] = useState('')
  const [passwordSuccess, setPasswordSuccess] = useState(false)

  // 修改邮箱表单状态
  const [newEmail, setNewEmail] = useState('')
  const [emailLoading, setEmailLoading] = useState(false)
  const [emailError, setEmailError] = useState('')
  const [emailSuccess, setEmailSuccess] = useState(false)

  useEffect(() => {
    getUserAuth()
      .then((userData) => {
        if (userData) {
          setUser(userData as UserInfo)
          // 初始化邮箱输入框
          setNewEmail(userData.email || '')
        } else {
          setError('无法获取用户信息')
        }
        setLoading(false)
      })
      .catch(() => {
        setError('加载用户信息失败')
        setLoading(false)
      })
  }, [])

  // 处理修改密码
  async function handlePasswordSubmit(e: React.FormEvent) {
    e.preventDefault()
    setPasswordError('')
    setPasswordSuccess(false)

    // 验证新密码
    if (passwordForm.newPassword.length < 6) {
      setPasswordError('新密码至少需要 6 个字符')
      return
    }

    if (passwordForm.newPassword !== passwordForm.confirmPassword) {
      setPasswordError('两次输入的新密码不一致')
      return
    }

    setPasswordLoading(true)

    const result = await updateUserPassword({
      currentPassword: passwordForm.currentPassword,
      newPassword: passwordForm.newPassword,
    })

    if (result.success) {
      setPasswordSuccess(true)
      setPasswordForm({
        currentPassword: '',
        newPassword: '',
        confirmPassword: '',
      })
    } else {
      setPasswordError(result.error || '修改密码失败')
    }

    setPasswordLoading(false)
  }

  // 处理修改邮箱
  async function handleEmailSubmit(e: React.FormEvent) {
    e.preventDefault()
    setEmailError('')
    setEmailSuccess(false)

    if (!newEmail) {
      setEmailError('请输入邮箱地址')
      return
    }

    setEmailLoading(true)

    const result = await updateUserEmail(newEmail)

    if (result.success) {
      setEmailSuccess(true)
      // 更新本地用户信息
      if (user) {
        setUser({ ...user, email: newEmail })
      }
    } else {
      setEmailError(result.error || '修改邮箱失败')
    }

    setEmailLoading(false)
  }

  if (loading) {
    return (
      <div className="flex justify-center items-center py-12">
        <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-emerald-600"></div>
      </div>
    )
  }

  if (error || !user) {
    return (
      <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4">
        <p className="text-sm text-red-600 dark:text-red-400">
          {error || '无法加载用户信息'}
        </p>
      </div>
    )
  }

  // 计算剩余天数
  const daysRemaining = user.expiresAt
    ? differenceInDays(new Date(user.expiresAt), new Date())
    : null

  // 判断到期状态
  const isExpired = daysRemaining !== null && daysRemaining < 0
  const isExpiringSoon = daysRemaining !== null && daysRemaining <= 7 && daysRemaining >= 0

  return (
    <div className="space-y-6">
      {/* 页面标题 */}
      <div>
        <h1 className="text-3xl font-bold text-gray-900 dark:text-white">
          我的账号
        </h1>
        <p className="mt-2 text-sm text-gray-600 dark:text-gray-400">
          查看和管理您的 Emby 账号信息
        </p>
      </div>

      {/* 账号状态警告 */}
      {!user.isActive && (
        <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4">
          <div className="flex">
            <div className="flex-shrink-0">
              <svg
                className="h-5 w-5 text-red-400"
                viewBox="0 0 20 20"
                fill="currentColor"
              >
                <path
                  fillRule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z"
                  clipRule="evenodd"
                />
              </svg>
            </div>
            <div className="ml-3">
              <h3 className="text-sm font-medium text-red-800 dark:text-red-400">
                账号已被禁用
              </h3>
              <p className="mt-1 text-sm text-red-700 dark:text-red-300">
                您的账号已被管理员禁用，请联系管理员了解详情。
              </p>
            </div>
          </div>
        </div>
      )}

      {isExpired && (
        <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4">
          <div className="flex">
            <div className="flex-shrink-0">
              <svg
                className="h-5 w-5 text-red-400"
                viewBox="0 0 20 20"
                fill="currentColor"
              >
                <path
                  fillRule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z"
                  clipRule="evenodd"
                />
              </svg>
            </div>
            <div className="ml-3">
              <h3 className="text-sm font-medium text-red-800 dark:text-red-400">
                账号已过期
              </h3>
              <p className="mt-1 text-sm text-red-700 dark:text-red-300">
                您的账号已于{' '}
                {user.expiresAt &&
                  format(new Date(user.expiresAt), 'yyyy年MM月dd日', {
                    locale: zhCN,
                  })}{' '}
                过期，请联系管理员续期。
              </p>
            </div>
          </div>
        </div>
      )}

      {isExpiringSoon && (
        <div className="bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-lg p-4">
          <div className="flex">
            <div className="flex-shrink-0">
              <svg
                className="h-5 w-5 text-yellow-400"
                viewBox="0 0 20 20"
                fill="currentColor"
              >
                <path
                  fillRule="evenodd"
                  d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z"
                  clipRule="evenodd"
                />
              </svg>
            </div>
            <div className="ml-3">
              <h3 className="text-sm font-medium text-yellow-800 dark:text-yellow-400">
                账号即将过期
              </h3>
              <p className="mt-1 text-sm text-yellow-700 dark:text-yellow-300">
                您的账号将在 {daysRemaining} 天后过期，请及时联系管理员续期。
              </p>
            </div>
          </div>
        </div>
      )}

      {/* 用户信息卡片 */}
      <div className="bg-white dark:bg-gray-800 shadow rounded-lg">
        <div className="px-6 py-5 border-b border-gray-200 dark:border-gray-700">
          <h2 className="text-lg font-medium text-gray-900 dark:text-white">
            账号信息
          </h2>
        </div>
        <div className="px-6 py-5 space-y-4">
          <div>
            <label className="text-sm font-medium text-gray-500 dark:text-gray-400">
              用户名
            </label>
            <p className="mt-1 text-sm text-gray-900 dark:text-white">
              {user.username}
            </p>
          </div>

          <div>
            <label className="text-sm font-medium text-gray-500 dark:text-gray-400">
              邮箱
            </label>
            <p className="mt-1 text-sm text-gray-900 dark:text-white">
              {user.email || '未设置'}
            </p>
          </div>

          <div>
            <label className="text-sm font-medium text-gray-500 dark:text-gray-400">
              Emby ID
            </label>
            <p className="mt-1 text-sm text-gray-900 dark:text-white font-mono">
              {user.embyId}
            </p>
          </div>

          <div>
            <label className="text-sm font-medium text-gray-500 dark:text-gray-400">
              注册时间
            </label>
            <p className="mt-1 text-sm text-gray-900 dark:text-white">
              {format(new Date(user.createdAt), 'yyyy年MM月dd日 HH:mm', {
                locale: zhCN,
              })}
            </p>
          </div>

          {user.inviteCode && (
            <div>
              <label className="text-sm font-medium text-gray-500 dark:text-gray-400">
                邀请码
              </label>
              <p className="mt-1 text-sm text-gray-900 dark:text-white font-mono">
                {user.inviteCode}
              </p>
            </div>
          )}

          <div>
            <label className="text-sm font-medium text-gray-500 dark:text-gray-400">
              到期时间
            </label>
            <div className="mt-1 flex items-center space-x-2">
              <p className="text-sm text-gray-900 dark:text-white">
                {user.expiresAt
                  ? format(new Date(user.expiresAt), 'yyyy年MM月dd日', {
                      locale: zhCN,
                    })
                  : '永久'}
              </p>
              {daysRemaining !== null && !isExpired && (
                <span
                  className={`px-2 py-1 rounded text-xs font-medium ${
                    isExpiringSoon
                      ? 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-400'
                      : 'bg-green-100 text-green-800 dark:bg-green-900/20 dark:text-green-400'
                  }`}
                >
                  剩余 {daysRemaining} 天
                </span>
              )}
            </div>
          </div>

          <div>
            <label className="text-sm font-medium text-gray-500 dark:text-gray-400">
              账号状态
            </label>
            <div className="mt-1">
              <span
                className={`inline-flex px-2 py-1 rounded text-xs font-medium ${
                  user.isActive
                    ? 'bg-green-100 text-green-800 dark:bg-green-900/20 dark:text-green-400'
                    : 'bg-red-100 text-red-800 dark:bg-red-900/20 dark:text-red-400'
                }`}
              >
                {user.isActive ? '正常' : '已禁用'}
              </span>
            </div>
          </div>
        </div>
      </div>

      {/* Emby 服务器信息 */}
      <div className="bg-white dark:bg-gray-800 shadow rounded-lg">
        <div className="px-6 py-5 border-b border-gray-200 dark:border-gray-700">
          <h2 className="text-lg font-medium text-gray-900 dark:text-white">
            Emby 服务器
          </h2>
        </div>
        <div className="px-6 py-5">
          <div>
            <label className="text-sm font-medium text-gray-500 dark:text-gray-400">
              服务器地址
            </label>
            <p className="mt-1 text-sm text-gray-900 dark:text-white font-mono">
              {process.env.NEXT_PUBLIC_EMBY_URL || '请联系管理员获取'}
            </p>
          </div>
          <div className="mt-4">
            <p className="text-sm text-gray-600 dark:text-gray-400">
              使用您的用户名和密码登录 Emby 客户端即可开始观看。
            </p>
          </div>
        </div>
      </div>

      {/* 修改密码 */}
      <div className="bg-white dark:bg-gray-800 shadow rounded-lg">
        <div className="px-6 py-5 border-b border-gray-200 dark:border-gray-700">
          <h2 className="text-lg font-medium text-gray-900 dark:text-white">
            修改密码
          </h2>
        </div>
        <div className="px-6 py-5">
          {passwordSuccess && (
            <div className="mb-4 p-4 bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 rounded-lg">
              <p className="text-sm text-green-600 dark:text-green-400">
                密码修改成功！下次登录时请使用新密码。
              </p>
            </div>
          )}

          {passwordError && (
            <div className="mb-4 p-4 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg">
              <p className="text-sm text-red-600 dark:text-red-400">
                {passwordError}
              </p>
            </div>
          )}

          <form onSubmit={handlePasswordSubmit} className="space-y-4">
            <div>
              <label
                htmlFor="currentPassword"
                className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2"
              >
                当前密码
              </label>
              <input
                id="currentPassword"
                type="password"
                required
                value={passwordForm.currentPassword}
                onChange={(e) =>
                  setPasswordForm({ ...passwordForm, currentPassword: e.target.value })
                }
                className="w-full px-4 py-2 border border-gray-300 dark:border-gray-600 rounded-lg focus:ring-2 focus:ring-emerald-500 focus:border-transparent dark:bg-gray-700 dark:text-white"
                disabled={passwordLoading}
              />
            </div>

            <div>
              <label
                htmlFor="newPassword"
                className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2"
              >
                新密码
              </label>
              <input
                id="newPassword"
                type="password"
                required
                value={passwordForm.newPassword}
                onChange={(e) =>
                  setPasswordForm({ ...passwordForm, newPassword: e.target.value })
                }
                className="w-full px-4 py-2 border border-gray-300 dark:border-gray-600 rounded-lg focus:ring-2 focus:ring-emerald-500 focus:border-transparent dark:bg-gray-700 dark:text-white"
                disabled={passwordLoading}
                minLength={6}
              />
              <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                密码至少需要 6 个字符
              </p>
            </div>

            <div>
              <label
                htmlFor="confirmPassword"
                className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2"
              >
                确认新密码
              </label>
              <input
                id="confirmPassword"
                type="password"
                required
                value={passwordForm.confirmPassword}
                onChange={(e) =>
                  setPasswordForm({
                    ...passwordForm,
                    confirmPassword: e.target.value,
                  })
                }
                className="w-full px-4 py-2 border border-gray-300 dark:border-gray-600 rounded-lg focus:ring-2 focus:ring-emerald-500 focus:border-transparent dark:bg-gray-700 dark:text-white"
                disabled={passwordLoading}
              />
            </div>

            <button
              type="submit"
              disabled={passwordLoading}
              className="px-4 py-2 bg-emerald-600 hover:bg-emerald-700 disabled:bg-emerald-400 text-white font-medium rounded-lg transition-colors"
            >
              {passwordLoading ? '修改中...' : '修改密码'}
            </button>
          </form>
        </div>
      </div>

      {/* 修改邮箱 */}
      <div className="bg-white dark:bg-gray-800 shadow rounded-lg">
        <div className="px-6 py-5 border-b border-gray-200 dark:border-gray-700">
          <h2 className="text-lg font-medium text-gray-900 dark:text-white">
            修改邮箱
          </h2>
        </div>
        <div className="px-6 py-5">
          {emailSuccess && (
            <div className="mb-4 p-4 bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 rounded-lg">
              <p className="text-sm text-green-600 dark:text-green-400">
                邮箱修改成功！
              </p>
            </div>
          )}

          {emailError && (
            <div className="mb-4 p-4 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg">
              <p className="text-sm text-red-600 dark:text-red-400">
                {emailError}
              </p>
            </div>
          )}

          <form onSubmit={handleEmailSubmit} className="space-y-4">
            <div>
              <label
                htmlFor="email"
                className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2"
              >
                邮箱地址
              </label>
              <input
                id="email"
                type="email"
                required
                value={newEmail}
                onChange={(e) => setNewEmail(e.target.value)}
                className="w-full px-4 py-2 border border-gray-300 dark:border-gray-600 rounded-lg focus:ring-2 focus:ring-emerald-500 focus:border-transparent dark:bg-gray-700 dark:text-white"
                disabled={emailLoading}
                placeholder="your@email.com"
              />
              <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                邮箱仅保存在本地数据库，不会同步到 Emby
              </p>
            </div>

            <button
              type="submit"
              disabled={emailLoading}
              className="px-4 py-2 bg-emerald-600 hover:bg-emerald-700 disabled:bg-emerald-400 text-white font-medium rounded-lg transition-colors"
            >
              {emailLoading ? '修改中...' : '修改邮箱'}
            </button>
          </form>
        </div>
      </div>
    </div>
  )
}
