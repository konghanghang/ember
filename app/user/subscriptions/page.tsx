'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import Image from 'next/image'
import { getUserSubscriptions, deleteSubscription } from '@/app/actions/subscriptions'
import { format } from 'date-fns'
import { zhCN } from 'date-fns/locale'
import type { MediaType, SubscriptionStatus } from '@prisma/client'

interface Subscription {
  id: string
  type: MediaType
  name: string
  tmdbId: string
  posterPath: string | null
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

  // 获取 TMDB URL
  function getTmdbUrl(type: MediaType, tmdbId: string) {
    const baseUrl = 'https://www.themoviedb.org'
    const mediaType = type === 'MOVIE' ? 'movie' : 'tv'
    return `${baseUrl}/${mediaType}/${tmdbId}`
  }

  if (loading) {
    return (
      <div className="flex justify-center items-center py-12">
        <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600"></div>
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
          className="px-4 py-2 bg-indigo-600 hover:bg-indigo-700 text-white font-medium rounded-lg transition-colors"
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
            className="mt-4 inline-block px-4 py-2 bg-indigo-600 hover:bg-indigo-700 text-white font-medium rounded-lg transition-colors"
          >
            提交第一个订阅
          </Link>
        </div>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {subscriptions.map((sub) => (
            <div
              key={sub.id}
              className="bg-white dark:bg-gray-800 rounded-lg shadow hover:shadow-lg transition-shadow overflow-hidden"
            >
              <div className="flex gap-4 p-4">
                {/* 封面图片 */}
                <div className="flex-shrink-0 w-24 h-36 bg-gray-200 dark:bg-gray-700 rounded overflow-hidden relative">
                  {sub.posterPath ? (
                    <Image
                      src={`https://image.tmdb.org/t/p/w185${sub.posterPath}`}
                      alt={sub.name}
                      fill
                      className="object-cover"
                      sizes="96px"
                    />
                  ) : (
                    <div className="w-full h-full flex items-center justify-center text-gray-400 text-xs">
                      无封面
                    </div>
                  )}
                </div>

                {/* 信息区域 */}
                <div className="flex-1 min-w-0 flex flex-col">
                  {/* 标题 */}
                  <h3 className="font-semibold text-gray-900 dark:text-white truncate">
                    {sub.name}
                  </h3>

                  {/* 类型和状态 */}
                  <div className="flex gap-2 mt-2">
                    {getTypeBadge(sub.type)}
                    {getStatusBadge(sub.status, sub.mpError)}
                  </div>

                  {/* TMDB ID 链接 */}
                  <div className="mt-2">
                    <a
                      href={getTmdbUrl(sub.type, sub.tmdbId)}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="text-sm text-indigo-600 hover:text-indigo-700 dark:text-indigo-400 dark:hover:text-indigo-300 flex items-center gap-1"
                    >
                      <span>TMDB: {sub.tmdbId}</span>
                      <svg
                        className="w-3 h-3"
                        fill="none"
                        stroke="currentColor"
                        viewBox="0 0 24 24"
                      >
                        <path
                          strokeLinecap="round"
                          strokeLinejoin="round"
                          strokeWidth={2}
                          d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"
                        />
                      </svg>
                    </a>
                  </div>

                  {/* 备注 */}
                  {sub.note && (
                    <p className="mt-2 text-xs text-gray-500 dark:text-gray-400 line-clamp-2">
                      {sub.note}
                    </p>
                  )}

                  {/* 错误信息 */}
                  {sub.mpError && (
                    <p className="mt-2 text-xs text-red-600 dark:text-red-400">
                      同步错误：{sub.mpError}
                    </p>
                  )}

                  {/* 底部：时间和操作 */}
                  <div className="mt-auto pt-3 flex items-center justify-between">
                    <span className="text-xs text-gray-500 dark:text-gray-400">
                      {format(new Date(sub.createdAt), 'MM-dd HH:mm', {
                        locale: zhCN,
                      })}
                    </span>
                    {sub.status === 'PENDING' && (
                      <button
                        onClick={() => handleDelete(sub.id)}
                        disabled={deleteLoading === sub.id}
                        className="text-sm text-red-600 hover:text-red-900 dark:text-red-400 dark:hover:text-red-300 font-medium disabled:opacity-50"
                      >
                        {deleteLoading === sub.id ? '删除中...' : '删除'}
                      </button>
                    )}
                  </div>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
