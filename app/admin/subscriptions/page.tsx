'use client'

import { useEffect, useState } from 'react'
import {
  getAllSubscriptions,
  approveSubscription,
  rejectSubscription,
} from '@/app/actions/subscriptions'
import { format } from 'date-fns'
import { zhCN } from 'date-fns/locale'
import type { MediaType, SubscriptionStatus } from '@prisma/client'

interface Subscription {
  id: string
  type: MediaType
  name: string
  tmdbId: string
  status: SubscriptionStatus
  note: string | null
  mpError: string | null
  createdAt: Date
  user: {
    username: string
    email: string
  }
}

export default function AdminSubscriptionsPage() {
  const [loading, setLoading] = useState(true)
  const [subscriptions, setSubscriptions] = useState<Subscription[]>([])
  const [statusFilter, setStatusFilter] = useState<SubscriptionStatus | 'ALL'>('PENDING')
  const [searchTerm, setSearchTerm] = useState('')
  const [processingId, setProcessingId] = useState<string | null>(null)

  useEffect(() => {
    loadSubscriptions()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [statusFilter])

  async function loadSubscriptions() {
    setLoading(true)
    const filter = statusFilter === 'ALL' ? undefined : statusFilter
    const data = await getAllSubscriptions(filter)
    if (data) {
      setSubscriptions(data as Subscription[])
    }
    setLoading(false)
  }

  // 审核通过
  async function handleApprove(id: string) {
    if (!confirm('确定批准这个订阅请求吗？')) return

    setProcessingId(id)
    const result = await approveSubscription(id)

    if (result.success) {
      await loadSubscriptions()
    } else {
      alert(result.error || '审核失败')
    }

    setProcessingId(null)
  }

  // 拒绝订阅
  async function handleReject(id: string) {
    if (!confirm('确定拒绝这个订阅请求吗？')) return

    setProcessingId(id)
    const result = await rejectSubscription(id)

    if (result.success) {
      await loadSubscriptions()
    } else {
      alert(result.error || '操作失败')
    }

    setProcessingId(null)
  }

  // 状态标签样式
  function getStatusBadge(status: SubscriptionStatus, mpError: string | null) {
    switch (status) {
      case 'PENDING':
        return (
          <span className="px-2 py-1 rounded text-xs font-medium bg-yellow-100 text-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-400">
            待审核
          </span>
        )
      case 'APPROVED':
        if (mpError) {
          return (
            <span className="px-2 py-1 rounded text-xs font-medium bg-red-100 text-red-800 dark:bg-red-900/20 dark:text-red-400">
              同步失败
            </span>
          )
        }
        return (
          <span className="px-2 py-1 rounded text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900/20 dark:text-green-400">
            已批准
          </span>
        )
      case 'REJECTED':
        return (
          <span className="px-2 py-1 rounded text-xs font-medium bg-gray-100 text-gray-800 dark:bg-gray-900/20 dark:text-gray-400">
            已拒绝
          </span>
        )
    }
  }

  // 媒体类型标签
  function getTypeBadge(type: MediaType) {
    return type === 'MOVIE' ? (
      <span className="px-2 py-1 rounded text-xs font-medium bg-blue-100 text-blue-800 dark:bg-blue-900/20 dark:text-blue-400">
        电影
      </span>
    ) : (
      <span className="px-2 py-1 rounded text-xs font-medium bg-purple-100 text-purple-800 dark:bg-purple-900/20 dark:text-purple-400">
        电视剧
      </span>
    )
  }

  // 客户端搜索过滤
  const filteredSubscriptions = subscriptions.filter(
    (sub) =>
      sub.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      sub.user.username.toLowerCase().includes(searchTerm.toLowerCase())
  )

  if (loading) {
    return (
      <div className="flex justify-center items-center py-12">
        <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-emerald-600"></div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* 页面标题和筛选 */}
      <div className="flex flex-col md:flex-row justify-between items-start md:items-center gap-4">
        <div>
          <h1 className="text-3xl font-bold text-gray-900 dark:text-white">
            订阅管理
          </h1>
          <p className="mt-2 text-sm text-gray-600 dark:text-gray-400">
            审核用户提交的影视订阅请求
          </p>
        </div>

        {/* 搜索框 */}
        <div className="w-full md:w-auto">
          <input
            type="text"
            placeholder="搜索影视名称或用户名..."
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            className="w-full md:w-64 px-4 py-2 border border-gray-300 dark:border-gray-600 rounded-lg dark:bg-gray-700 dark:text-white"
          />
        </div>
      </div>

      {/* 状态筛选 */}
      <div className="flex gap-2 flex-wrap">
        <button
          onClick={() => setStatusFilter('ALL')}
          className={`px-4 py-2 rounded-lg font-medium transition-colors ${
            statusFilter === 'ALL'
              ? 'bg-emerald-600 text-white'
              : 'bg-gray-200 dark:bg-gray-700 text-gray-700 dark:text-gray-300 hover:bg-gray-300 dark:hover:bg-gray-600'
          }`}
        >
          全部
        </button>
        <button
          onClick={() => setStatusFilter('PENDING')}
          className={`px-4 py-2 rounded-lg font-medium transition-colors ${
            statusFilter === 'PENDING'
              ? 'bg-emerald-600 text-white'
              : 'bg-gray-200 dark:bg-gray-700 text-gray-700 dark:text-gray-300 hover:bg-gray-300 dark:hover:bg-gray-600'
          }`}
        >
          待审核
        </button>
        <button
          onClick={() => setStatusFilter('APPROVED')}
          className={`px-4 py-2 rounded-lg font-medium transition-colors ${
            statusFilter === 'APPROVED'
              ? 'bg-emerald-600 text-white'
              : 'bg-gray-200 dark:bg-gray-700 text-gray-700 dark:text-gray-300 hover:bg-gray-300 dark:hover:bg-gray-600'
          }`}
        >
          已批准
        </button>
        <button
          onClick={() => setStatusFilter('REJECTED')}
          className={`px-4 py-2 rounded-lg font-medium transition-colors ${
            statusFilter === 'REJECTED'
              ? 'bg-emerald-600 text-white'
              : 'bg-gray-200 dark:bg-gray-700 text-gray-700 dark:text-gray-300 hover:bg-gray-300 dark:hover:bg-gray-600'
          }`}
        >
          已拒绝
        </button>
      </div>

      {/* 订阅列表 */}
      {filteredSubscriptions.length === 0 ? (
        <div className="bg-white dark:bg-gray-800 shadow rounded-lg p-8 text-center">
          <p className="text-gray-500 dark:text-gray-400">
            {searchTerm ? '未找到匹配的订阅' : '暂无订阅记录'}
          </p>
        </div>
      ) : (
        <div className="bg-white dark:bg-gray-800 shadow rounded-lg overflow-hidden">
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
              <thead className="bg-gray-50 dark:bg-gray-900">
                <tr>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    用户
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    影视名称
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    类型
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    TMDB ID
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    状态
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    提交时间
                  </th>
                  <th className="px-6 py-3 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    操作
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                {filteredSubscriptions.map((sub) => (
                  <tr
                    key={sub.id}
                    className="hover:bg-gray-50 dark:hover:bg-gray-700/50 transition-colors"
                  >
                    <td className="px-6 py-4 whitespace-nowrap">
                      <div className="text-sm font-medium text-gray-900 dark:text-white">
                        {sub.user.username}
                      </div>
                      <div className="text-xs text-gray-500 dark:text-gray-400">
                        {sub.user.email}
                      </div>
                    </td>
                    <td className="px-6 py-4">
                      <div className="text-sm font-medium text-gray-900 dark:text-white">
                        {sub.name}
                      </div>
                      {sub.note && (
                        <div className="text-xs text-gray-500 dark:text-gray-400 mt-1">
                          备注：{sub.note}
                        </div>
                      )}
                      {sub.mpError && (
                        <div className="text-xs text-red-600 dark:text-red-400 mt-1">
                          同步错误：{sub.mpError}
                        </div>
                      )}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      {getTypeBadge(sub.type)}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <code className="text-sm text-gray-900 dark:text-white">
                        {sub.tmdbId}
                      </code>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      {getStatusBadge(sub.status, sub.mpError)}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400">
                      {format(new Date(sub.createdAt), 'yyyy-MM-dd HH:mm', {
                        locale: zhCN,
                      })}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-right text-sm">
                      {sub.status === 'PENDING' && (
                        <div className="flex justify-end gap-2">
                          <button
                            onClick={() => handleApprove(sub.id)}
                            disabled={processingId === sub.id}
                            className="text-green-600 hover:text-green-900 dark:text-green-400 dark:hover:text-green-300 font-medium disabled:opacity-50"
                          >
                            {processingId === sub.id ? '处理中...' : '批准'}
                          </button>
                          <button
                            onClick={() => handleReject(sub.id)}
                            disabled={processingId === sub.id}
                            className="text-red-600 hover:text-red-900 dark:text-red-400 dark:hover:text-red-300 font-medium disabled:opacity-50"
                          >
                            拒绝
                          </button>
                        </div>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* 统计信息 */}
      <div className="bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg p-4">
        <p className="text-sm text-blue-800 dark:text-blue-300">
          共 {filteredSubscriptions.length} 条订阅
          {searchTerm && ` (已过滤)`}
        </p>
      </div>
    </div>
  )
}
