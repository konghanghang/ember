'use client'

import { useEffect, useState } from 'react'
import { createInvite, getInvites, deleteInvite } from '@/app/actions/invites'
import { formatDateTime } from '@/lib/utils'

interface Invite {
  id: string
  code: string
  maxUses: number
  usedCount: number
  expiresAt: Date | null
  defaultDays: number
  createdAt: Date
  users: { id: string; username: string; createdAt: Date }[]
}

export default function InvitesPage() {
  const [invites, setInvites] = useState<Invite[]>([])
  const [loading, setLoading] = useState(true)
  const [creating, setCreating] = useState(false)
  const [showForm, setShowForm] = useState(false)

  // 表单状态
  const [maxUses, setMaxUses] = useState(1)
  const [defaultDays, setDefaultDays] = useState(30)
  const [hasExpiry, setHasExpiry] = useState(false)
  const [expiryDays, setExpiryDays] = useState(30)

  useEffect(() => {
    loadInvites()
  }, [])

  async function loadInvites() {
    setLoading(true)
    const result = await getInvites()
    if (result.success && result.invites) {
      setInvites(result.invites as Invite[])
    }
    setLoading(false)
  }

  async function handleCreate() {
    setCreating(true)

    const data: any = {
      maxUses,
      defaultDays,
    }

    if (hasExpiry) {
      const expiresAt = new Date()
      expiresAt.setDate(expiresAt.getDate() + expiryDays)
      data.expiresAt = expiresAt
    }

    const result = await createInvite(data)

    if (result.success) {
      await loadInvites()
      setShowForm(false)
      // 重置表单
      setMaxUses(1)
      setDefaultDays(30)
      setHasExpiry(false)
      setExpiryDays(30)
    } else {
      alert(result.error || '创建失败')
    }

    setCreating(false)
  }

  async function handleDelete(id: string) {
    if (!confirm('确定删除此邀请码？')) return

    const result = await deleteInvite(id)

    if (result.success) {
      await loadInvites()
    } else {
      alert(result.error || '删除失败')
    }
  }

  function copyToClipboard(code: string) {
    navigator.clipboard.writeText(code)
    alert('邀请码已复制到剪贴板')
  }

  if (loading) {
    return <div className="text-center py-12">加载中...</div>
  }

  return (
    <div>
      {/* 页面标题和操作 */}
      <div className="flex justify-between items-center mb-6">
        <h1 className="text-3xl font-bold text-gray-900 dark:text-white">
          邀请码管理
        </h1>
        <button
          onClick={() => setShowForm(!showForm)}
          className="px-4 py-2 bg-indigo-600 hover:bg-indigo-700 text-white font-medium rounded-lg transition-colors"
        >
          {showForm ? '取消' : '+ 生成邀请码'}
        </button>
      </div>

      {/* 生成邀请码表单 */}
      {showForm && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow-md p-6 mb-6">
          <h2 className="text-xl font-semibold text-gray-900 dark:text-white mb-4">
            生成新邀请码
          </h2>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                最大使用次数
              </label>
              <input
                type="number"
                min="1"
                value={maxUses}
                onChange={(e) => setMaxUses(parseInt(e.target.value))}
                className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg dark:bg-gray-700 dark:text-white"
              />
            </div>

            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                默认账号有效天数
              </label>
              <input
                type="number"
                min="1"
                value={defaultDays}
                onChange={(e) => setDefaultDays(parseInt(e.target.value))}
                className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg dark:bg-gray-700 dark:text-white"
              />
            </div>

            <div className="md:col-span-2">
              <label className="flex items-center space-x-2 text-sm font-medium text-gray-700 dark:text-gray-300">
                <input
                  type="checkbox"
                  checked={hasExpiry}
                  onChange={(e) => setHasExpiry(e.target.checked)}
                  className="rounded border-gray-300 text-indigo-600 focus:ring-indigo-500"
                />
                <span>设置邀请码过期时间</span>
              </label>
            </div>

            {hasExpiry && (
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                  邀请码有效天数
                </label>
                <input
                  type="number"
                  min="1"
                  value={expiryDays}
                  onChange={(e) => setExpiryDays(parseInt(e.target.value))}
                  className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg dark:bg-gray-700 dark:text-white"
                />
              </div>
            )}
          </div>

          <div className="mt-6 flex justify-end">
            <button
              onClick={handleCreate}
              disabled={creating}
              className="px-6 py-2 bg-indigo-600 hover:bg-indigo-700 disabled:bg-indigo-400 text-white font-medium rounded-lg transition-colors"
            >
              {creating ? '生成中...' : '生成邀请码'}
            </button>
          </div>
        </div>
      )}

      {/* 邀请码列表 */}
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-md overflow-hidden">
        {invites.length === 0 ? (
          <div className="text-center py-12 text-gray-500 dark:text-gray-400">
            暂无邀请码，点击上方按钮生成
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-gray-50 dark:bg-gray-700">
                <tr>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                    邀请码
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                    使用情况
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                    默认天数
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                    过期时间
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                    创建时间
                  </th>
                  <th className="px-6 py-3 text-right text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                    操作
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                {invites.map((invite) => (
                  <tr key={invite.id} className="hover:bg-gray-50 dark:hover:bg-gray-700">
                    <td className="px-6 py-4 whitespace-nowrap">
                      <button
                        onClick={() => copyToClipboard(invite.code)}
                        className="font-mono text-sm text-indigo-600 dark:text-indigo-400 hover:text-indigo-800 dark:hover:text-indigo-300"
                        title="点击复制"
                      >
                        {invite.code}
                      </button>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-900 dark:text-gray-100">
                      {invite.usedCount} / {invite.maxUses}
                      {invite.usedCount >= invite.maxUses && (
                        <span className="ml-2 px-2 py-1 text-xs bg-gray-200 dark:bg-gray-600 text-gray-700 dark:text-gray-300 rounded">
                          已用完
                        </span>
                      )}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-900 dark:text-gray-100">
                      {invite.defaultDays} 天
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-900 dark:text-gray-100">
                      {invite.expiresAt
                        ? formatDateTime(invite.expiresAt)
                        : '永久有效'}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400">
                      {formatDateTime(invite.createdAt)}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-right text-sm font-medium">
                      <button
                        onClick={() => handleDelete(invite.id)}
                        disabled={invite.usedCount > 0}
                        className="text-red-600 hover:text-red-900 dark:text-red-400 dark:hover:text-red-300 disabled:text-gray-400 disabled:cursor-not-allowed"
                      >
                        删除
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}
