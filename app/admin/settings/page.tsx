'use client'

import { useEffect, useState } from 'react'
import { getSystemInfo, testEmbyConnection } from '@/app/actions/settings'
import { checkExpiredUsers } from '@/app/actions/cron'
import { updateAdminPassword } from '@/app/actions/auth'

interface SystemInfo {
  userCount: number
  activeUserCount: number
  inviteCount: number
}

export default function SettingsPage() {
  const [systemInfo, setSystemInfo] = useState<SystemInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [testingEmby, setTestingEmby] = useState(false)
  const [embyStatus, setEmbyStatus] = useState<{ success: boolean; message: string } | null>(null)
  const [runningCron, setRunningCron] = useState(false)
  const [cronResult, setCronResult] = useState<{ success: boolean; message: string } | null>(null)

  // 修改密码相关状态
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [updatingPassword, setUpdatingPassword] = useState(false)
  const [passwordResult, setPasswordResult] = useState<{ success: boolean; message: string } | null>(null)

  useEffect(() => {
    loadSystemInfo()
  }, [])

  async function loadSystemInfo() {
    setLoading(true)
    const result = await getSystemInfo()
    if (result.success && result.info) {
      setSystemInfo(result.info)
    }
    setLoading(false)
  }

  async function handleTestEmby() {
    setTestingEmby(true)
    setEmbyStatus(null)

    const result = await testEmbyConnection()

    setEmbyStatus({
      success: result.success,
      message: result.success ? result.message! : result.error!,
    })

    setTestingEmby(false)
  }

  async function handleRunCron() {
    setRunningCron(true)
    setCronResult(null)

    const result = await checkExpiredUsers()

    setCronResult({
      success: result.success,
      message: `已处理 ${result.totalExpired} 个过期用户，成功禁用 ${result.disabledCount} 个`,
    })

    setRunningCron(false)

    // 刷新系统信息
    await loadSystemInfo()
  }

  async function handleUpdatePassword(e: React.FormEvent) {
    e.preventDefault()
    setPasswordResult(null)

    // 验证新密码和确认密码是否一致
    if (newPassword !== confirmPassword) {
      setPasswordResult({
        success: false,
        message: '新密码和确认密码不一致',
      })
      return
    }

    // 验证新密码长度
    if (newPassword.length < 6) {
      setPasswordResult({
        success: false,
        message: '新密码长度至少 6 个字符',
      })
      return
    }

    setUpdatingPassword(true)

    const result = await updateAdminPassword({
      currentPassword,
      newPassword,
    })

    setPasswordResult({
      success: result.success,
      message: result.success ? '密码修改成功' : result.error!,
    })

    setUpdatingPassword(false)

    // 成功后清空表单
    if (result.success) {
      setCurrentPassword('')
      setNewPassword('')
      setConfirmPassword('')
    }
  }

  if (loading) {
    return <div className="text-center py-12">加载中...</div>
  }

  return (
    <div className="space-y-6">
      {/* 页面标题 */}
      <h1 className="text-3xl font-bold text-gray-900 dark:text-white">
        系统设置
      </h1>

      {/* 系统信息 */}
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-md p-6">
        <h2 className="text-xl font-semibold text-gray-900 dark:text-white mb-4">
          系统信息
        </h2>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div className="bg-gray-50 dark:bg-gray-700 rounded-lg p-4">
            <div className="text-sm text-gray-600 dark:text-gray-400 mb-1">
              总用户数
            </div>
            <div className="text-3xl font-bold text-gray-900 dark:text-white">
              {systemInfo?.userCount || 0}
            </div>
          </div>

          <div className="bg-gray-50 dark:bg-gray-700 rounded-lg p-4">
            <div className="text-sm text-gray-600 dark:text-gray-400 mb-1">
              活跃用户数
            </div>
            <div className="text-3xl font-bold text-green-600 dark:text-green-400">
              {systemInfo?.activeUserCount || 0}
            </div>
          </div>

          <div className="bg-gray-50 dark:bg-gray-700 rounded-lg p-4">
            <div className="text-sm text-gray-600 dark:text-gray-400 mb-1">
              邀请码数量
            </div>
            <div className="text-3xl font-bold text-indigo-600 dark:text-indigo-400">
              {systemInfo?.inviteCount || 0}
            </div>
          </div>
        </div>
      </div>

      {/* Emby 连接测试 */}
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-md p-6">
        <h2 className="text-xl font-semibold text-gray-900 dark:text-white mb-4">
          Emby 服务器
        </h2>

        <div className="flex items-center gap-4">
          <button
            onClick={handleTestEmby}
            disabled={testingEmby}
            className="px-4 py-2 bg-indigo-600 hover:bg-indigo-700 disabled:bg-indigo-400 text-white font-medium rounded-lg transition-colors"
          >
            {testingEmby ? '测试中...' : '测试连接'}
          </button>

          {embyStatus && (
            <div
              className={`text-sm ${
                embyStatus.success
                  ? 'text-green-600 dark:text-green-400'
                  : 'text-red-600 dark:text-red-400'
              }`}
            >
              {embyStatus.message}
            </div>
          )}
        </div>
      </div>

      {/* 定时任务 */}
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-md p-6">
        <h2 className="text-xl font-semibold text-gray-900 dark:text-white mb-4">
          定时任务
        </h2>

        <div className="mb-4">
          <p className="text-sm text-gray-600 dark:text-gray-400 mb-2">
            定时任务每天凌晨 2:00 自动执行，检查并禁用过期用户。
          </p>
          <p className="text-sm text-gray-600 dark:text-gray-400">
            你也可以手动触发定时任务：
          </p>
        </div>

        <div className="flex items-center gap-4">
          <button
            onClick={handleRunCron}
            disabled={runningCron}
            className="px-4 py-2 bg-yellow-600 hover:bg-yellow-700 disabled:bg-yellow-400 text-white font-medium rounded-lg transition-colors"
          >
            {runningCron ? '执行中...' : '手动触发'}
          </button>

          {cronResult && (
            <div
              className={`text-sm ${
                cronResult.success
                  ? 'text-green-600 dark:text-green-400'
                  : 'text-red-600 dark:text-red-400'
              }`}
            >
              {cronResult.message}
            </div>
          )}
        </div>
      </div>

      {/* 修改密码 */}
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-md p-6">
        <h2 className="text-xl font-semibold text-gray-900 dark:text-white mb-4">
          修改密码
        </h2>

        <form onSubmit={handleUpdatePassword} className="space-y-4 max-w-md">
          <div>
            <label
              htmlFor="currentPassword"
              className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1"
            >
              当前密码
            </label>
            <input
              type="password"
              id="currentPassword"
              value={currentPassword}
              onChange={(e) => setCurrentPassword(e.target.value)}
              required
              className="w-full px-4 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
              placeholder="请输入当前密码"
            />
          </div>

          <div>
            <label
              htmlFor="newPassword"
              className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1"
            >
              新密码
            </label>
            <input
              type="password"
              id="newPassword"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              required
              minLength={6}
              className="w-full px-4 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
              placeholder="至少 6 个字符"
            />
          </div>

          <div>
            <label
              htmlFor="confirmPassword"
              className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1"
            >
              确认新密码
            </label>
            <input
              type="password"
              id="confirmPassword"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              required
              minLength={6}
              className="w-full px-4 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
              placeholder="再次输入新密码"
            />
          </div>

          <div className="flex items-center gap-4">
            <button
              type="submit"
              disabled={updatingPassword}
              className="px-4 py-2 bg-indigo-600 hover:bg-indigo-700 disabled:bg-indigo-400 text-white font-medium rounded-lg transition-colors"
            >
              {updatingPassword ? '修改中...' : '修改密码'}
            </button>

            {passwordResult && (
              <div
                className={`text-sm ${
                  passwordResult.success
                    ? 'text-green-600 dark:text-green-400'
                    : 'text-red-600 dark:text-red-400'
                }`}
              >
                {passwordResult.message}
              </div>
            )}
          </div>

          <p className="text-xs text-gray-500 dark:text-gray-400">
            提示：修改密码后不会退出登录，当前 Token 仍然有效。
          </p>
        </form>
      </div>

      {/* 环境信息 */}
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-md p-6">
        <h2 className="text-xl font-semibold text-gray-900 dark:text-white mb-4">
          环境信息
        </h2>

        <div className="space-y-2 text-sm text-gray-600 dark:text-gray-400">
          <div className="flex">
            <span className="w-32 font-medium">Node 版本:</span>
            <span>{process.env.NODE_VERSION || 'N/A'}</span>
          </div>
          <div className="flex">
            <span className="w-32 font-medium">环境:</span>
            <span>{process.env.NODE_ENV || 'development'}</span>
          </div>
        </div>
      </div>
    </div>
  )
}
