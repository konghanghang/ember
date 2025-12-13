'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { getUserSubscriptions, deleteSubscription } from '@/app/actions/subscriptions'
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
}

export default function UserSubscriptionsPage() {
  const [loading, setLoading] = useState(true)
  const [subscriptions, setSubscriptions] = useState<Subscription[]>([])
  const [error, setError] = useState('')
  const [deleteLoading, setDeleteLoading] = useState<string | null>(null)

  useEffect(() => {
    loadSubscriptions()
  }, [])

  async function loadSubscriptions() {
    try {
      const data = await getUserSubscriptions()
      if (data) {
        setSubscriptions(data as Subscription[])
      } else {
        setError('无法加载订阅列表')
      }
    } catch (err) {
      console.error('加载订阅失败：', err)
      setError('加载订阅列表失败')
    } finally {
      setLoading(false)
    }
  }

  async function handleDelete(id: string) {
    if (!confirm('确定要删除这个订阅吗？')) {
      return
    }

    setDeleteLoading(id)
    const result = await deleteSubscription(id)

    if (result.success) {
      // 从列表中移除
      setSubscriptions((prev) => prev.filter((sub) => sub.id !== id))
    } else {
      alert(result.error || '删除失败')
    }

    setDeleteLoading(null)
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

  if (loading) {
    return (
      <div className="flex justify-center items-center py-12">
        <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-emerald-600"></div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* 页面标题 */}
      <div className="flex justify-between items-center">
        <div>
          <h1 className="text-3xl font-bold text-gray-900 dark:text-white">
            我的订阅
          </h1>
          <p className="mt-2 text-sm text-gray-600 dark:text-gray-400">
            管理您的影视订阅请求
          </p>
        </div>
        <Link
          href="/user/subscriptions/new"
          className="px-4 py-2 bg-emerald-600 hover:bg-emerald-700 text-white font-medium rounded-lg transition-colors"
        >
          提交新订阅
        </Link>
      </div>

      {/* 错误提示 */}
      {error && (
        <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4">
          <p className="text-sm text-red-600 dark:text-red-400">{error}</p>
        </div>
      )}

      {/* 订阅列表 */}
      {subscriptions.length === 0 ? (
        <div className="bg-white dark:bg-gray-800 shadow rounded-lg p-8 text-center">
          <p className="text-gray-500 dark:text-gray-400">
            暂无订阅记录
          </p>
          <Link
            href="/user/subscriptions/new"
            className="mt-4 inline-block px-4 py-2 bg-emerald-600 hover:bg-emerald-700 text-white font-medium rounded-lg transition-colors"
          >
            提交第一个订阅
          </Link>
        </div>
      ) : (
        <div className="bg-white dark:bg-gray-800 shadow rounded-lg overflow-hidden">
          <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
            <thead className="bg-gray-50 dark:bg-gray-900">
              <tr>
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
              {subscriptions.map((sub) => (
                <tr key={sub.id} className="hover:bg-gray-50 dark:hover:bg-gray-700/50 transition-colors">
                  <td className="px-6 py-4 whitespace-nowrap">
                    <div className="text-sm font-medium text-gray-900 dark:text-white">
                      {sub.name}
                    </div>
                    {sub.note && (
                      <div className="text-sm text-gray-500 dark:text-gray-400">
                        {sub.note}
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
                      <button
                        onClick={() => handleDelete(sub.id)}
                        disabled={deleteLoading === sub.id}
                        className="text-red-600 hover:text-red-900 dark:text-red-400 dark:hover:text-red-300 font-medium disabled:opacity-50"
                      >
                        {deleteLoading === sub.id ? '删除中...' : '删除'}
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
