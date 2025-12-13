'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import { createSubscription } from '@/app/actions/subscriptions'
import { MediaType } from '@prisma/client'

export default function NewSubscriptionPage() {
  const router = useRouter()
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setError('')
    setLoading(true)

    const formData = new FormData(e.currentTarget)
    const data = {
      type: formData.get('type') as MediaType,
      name: formData.get('name') as string,
      tmdbId: formData.get('tmdbId') as string,
      note: formData.get('note') as string,
    }

    const result = await createSubscription(data)

    if (result.success) {
      // 成功后跳转到订阅列表
      router.push('/user/subscriptions')
    } else {
      setError(result.error || '提交订阅失败')
      setLoading(false)
    }
  }

  return (
    <div className="max-w-2xl mx-auto">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-6">
        {/* 页面标题 */}
        <div className="mb-6">
          <h1 className="text-2xl font-bold text-gray-900 dark:text-white">
            提交新订阅
          </h1>
          <p className="mt-2 text-sm text-gray-600 dark:text-gray-400">
            填写影视信息，管理员审核通过后将自动添加到订阅列表
          </p>
        </div>

        {/* 错误提示 */}
        {error && (
          <div className="mb-6 p-4 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg">
            <p className="text-sm text-red-600 dark:text-red-400">{error}</p>
          </div>
        )}

        {/* 订阅表单 */}
        <form onSubmit={handleSubmit} className="space-y-5">
          {/* 媒体类型 */}
          <div>
            <label
              htmlFor="type"
              className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2"
            >
              媒体类型 <span className="text-red-500">*</span>
            </label>
            <select
              id="type"
              name="type"
              required
              disabled={loading}
              className="w-full px-4 py-3 border border-gray-300 dark:border-gray-600 rounded-lg focus:ring-2 focus:ring-emerald-500 focus:border-transparent dark:bg-gray-700 dark:text-white transition-colors"
            >
              <option value="MOVIE">电影</option>
              <option value="TV">电视剧</option>
            </select>
          </div>

          {/* 影视名称 */}
          <div>
            <label
              htmlFor="name"
              className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2"
            >
              影视名称 <span className="text-red-500">*</span>
            </label>
            <input
              id="name"
              name="name"
              type="text"
              required
              disabled={loading}
              className="w-full px-4 py-3 border border-gray-300 dark:border-gray-600 rounded-lg focus:ring-2 focus:ring-emerald-500 focus:border-transparent dark:bg-gray-700 dark:text-white transition-colors"
              placeholder="例如：肖申克的救赎"
            />
          </div>

          {/* TMDB ID */}
          <div>
            <label
              htmlFor="tmdbId"
              className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2"
            >
              TMDB ID <span className="text-red-500">*</span>
            </label>
            <input
              id="tmdbId"
              name="tmdbId"
              type="text"
              required
              disabled={loading}
              className="w-full px-4 py-3 border border-gray-300 dark:border-gray-600 rounded-lg focus:ring-2 focus:ring-emerald-500 focus:border-transparent dark:bg-gray-700 dark:text-white transition-colors"
              placeholder="例如：278"
            />
            <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
              在{' '}
              <a
                href="https://www.themoviedb.org/"
                target="_blank"
                rel="noopener noreferrer"
                className="text-emerald-600 hover:text-emerald-700 dark:text-emerald-400 dark:hover:text-emerald-300"
              >
                TheMovieDB
              </a>{' '}
              搜索影视作品，从 URL 中获取 ID
            </p>
          </div>

          {/* 备注 */}
          <div>
            <label
              htmlFor="note"
              className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2"
            >
              备注（可选）
            </label>
            <textarea
              id="note"
              name="note"
              rows={3}
              disabled={loading}
              className="w-full px-4 py-3 border border-gray-300 dark:border-gray-600 rounded-lg focus:ring-2 focus:ring-emerald-500 focus:border-transparent dark:bg-gray-700 dark:text-white transition-colors resize-none"
              placeholder="补充说明，例如：希望尽快添加"
            />
          </div>

          {/* 按钮组 */}
          <div className="flex gap-3 pt-2">
            <button
              type="button"
              onClick={() => router.push('/user/subscriptions')}
              disabled={loading}
              className="flex-1 px-4 py-3 bg-gray-200 hover:bg-gray-300 dark:bg-gray-700 dark:hover:bg-gray-600 text-gray-900 dark:text-white font-medium rounded-lg transition-colors disabled:opacity-50"
            >
              取消
            </button>
            <button
              type="submit"
              disabled={loading}
              className="flex-1 px-4 py-3 bg-emerald-600 hover:bg-emerald-700 disabled:bg-emerald-400 text-white font-medium rounded-lg transition-colors focus:outline-none focus:ring-2 focus:ring-emerald-500 focus:ring-offset-2"
            >
              {loading ? '提交中...' : '提交订阅'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
