'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import { createSubscription } from '@/app/actions/subscriptions'
import { MediaType } from '@prisma/client'
import TmdbSearchInput from '@/app/components/TmdbSearchInput'

export default function NewSubscriptionPage() {
  const router = useRouter()
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [mediaType, setMediaType] = useState<MediaType>('MOVIE')
  const [selectedMedia, setSelectedMedia] = useState<{
    tmdbId: string
    name: string
  } | null>(null)

  async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setError('')

    // 验证是否已选择影视作品
    if (!selectedMedia) {
      setError('请先搜索并选择影视作品')
      return
    }

    setLoading(true)

    const formData = new FormData(e.currentTarget)
    const data = {
      type: mediaType,
      name: selectedMedia.name,
      tmdbId: selectedMedia.tmdbId,
      note: formData.get('note') as string,
    }

    const result = await createSubscription(data)

    if (result.success) {
      router.push('/user/subscriptions')
    } else {
      setError(result.error || '提交订阅失败')
      setLoading(false)
    }
  }

  const handleMediaTypeChange = (newType: MediaType) => {
    setMediaType(newType)
    setSelectedMedia(null) // 切换类型时清空选择
  }

  const handleMediaSelect = (result: { tmdbId: string; name: string }) => {
    setSelectedMedia(result)
    setError('') // 清除错误提示
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
            搜索并选择影视作品，管理员审核通过后将自动添加到订阅列表
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
              value={mediaType}
              onChange={(e) =>
                handleMediaTypeChange(e.target.value as MediaType)
              }
              disabled={loading}
              className="w-full px-4 py-3 border border-gray-300 dark:border-gray-600 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent dark:bg-gray-700 dark:text-white transition-colors"
            >
              <option value="MOVIE">电影</option>
              <option value="TV">电视剧</option>
            </select>
          </div>

          {/* TMDB 搜索 */}
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
              搜索影视作品 <span className="text-red-500">*</span>
            </label>
            <TmdbSearchInput
              mediaType={mediaType}
              onSelect={handleMediaSelect}
              placeholder={`搜索${mediaType === 'MOVIE' ? '电影' : '电视剧'}...`}
            />
            {selectedMedia && (
              <div className="mt-3 p-3 bg-indigo-50 dark:bg-indigo-900/20 border border-indigo-200 dark:border-indigo-800 rounded-lg">
                <p className="text-sm text-indigo-900 dark:text-indigo-100">
                  ✓ 已选择：<span className="font-medium">{selectedMedia.name}</span>
                </p>
                <p className="text-xs text-indigo-600 dark:text-indigo-400 mt-1">
                  TMDB ID: {selectedMedia.tmdbId}
                </p>
              </div>
            )}
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
              className="w-full px-4 py-3 border border-gray-300 dark:border-gray-600 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent dark:bg-gray-700 dark:text-white transition-colors resize-none"
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
              disabled={loading || !selectedMedia}
              className="flex-1 px-4 py-3 bg-indigo-600 hover:bg-indigo-700 disabled:bg-gray-400 text-white font-medium rounded-lg transition-colors focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2"
            >
              {loading ? '提交中...' : '提交订阅'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
