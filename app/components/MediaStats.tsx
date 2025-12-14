'use client'

import { useEffect, useState } from 'react'
import { getMediaStats } from '@/app/actions/media'
import type { MediaStats as MediaStatsType } from '@/lib/emby'

export default function MediaStats() {
  const [loading, setLoading] = useState(true)
  const [stats, setStats] = useState<MediaStatsType | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    getMediaStats()
      .then((result) => {
        if (result.success && result.data) {
          setStats(result.data)
        } else {
          setError(result.error || '无法加载媒体库统计')
        }
        setLoading(false)
      })
      .catch(() => {
        setError('加载媒体库统计失败')
        setLoading(false)
      })
  }, [])

  if (loading) {
    return (
      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        {[1, 2, 3].map((i) => (
          <div
            key={i}
            className="bg-gray-200 dark:bg-gray-700 rounded-2xl p-6 animate-pulse"
          >
            <div className="h-20"></div>
          </div>
        ))}
      </div>
    )
  }

  if (error || !stats) {
    return (
      <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4">
        <p className="text-sm text-red-600 dark:text-red-400">
          {error || '无法加载媒体库统计'}
        </p>
      </div>
    )
  }

  // 格式化数字（添加千位分隔符）
  const formatNumber = (num: number) => {
    return num.toLocaleString('zh-CN')
  }

  return (
    <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
      {/* 电影 */}
      <div className="bg-gradient-to-br from-purple-500 to-purple-600 rounded-2xl p-6 text-white shadow-lg hover:shadow-xl transition-shadow">
        <div className="flex items-center gap-4">
          <div className="p-3 bg-white/20 rounded-xl backdrop-blur-sm">
            <svg
              className="w-8 h-8"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M7 4v16M17 4v16M3 8h4m10 0h4M3 12h18M3 16h4m10 0h4M4 20h16a1 1 0 001-1V5a1 1 0 00-1-1H4a1 1 0 00-1 1v14a1 1 0 001 1z"
              />
            </svg>
          </div>
          <div>
            <p className="text-sm opacity-90 font-medium">电影</p>
            <p className="text-3xl font-bold">{formatNumber(stats.MovieCount)}</p>
          </div>
        </div>
      </div>

      {/* 电视剧 */}
      <div className="bg-gradient-to-br from-green-500 to-green-600 rounded-2xl p-6 text-white shadow-lg hover:shadow-xl transition-shadow">
        <div className="flex items-center gap-4">
          <div className="p-3 bg-white/20 rounded-xl backdrop-blur-sm">
            <svg
              className="w-8 h-8"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"
              />
            </svg>
          </div>
          <div>
            <p className="text-sm opacity-90 font-medium">电视剧</p>
            <p className="text-3xl font-bold">{formatNumber(stats.SeriesCount)}</p>
          </div>
        </div>
      </div>

      {/* 剧集 */}
      <div className="bg-gradient-to-br from-orange-500 to-orange-600 rounded-2xl p-6 text-white shadow-lg hover:shadow-xl transition-shadow">
        <div className="flex items-center gap-4">
          <div className="p-3 bg-white/20 rounded-xl backdrop-blur-sm">
            <svg
              className="w-8 h-8"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"
              />
            </svg>
          </div>
          <div>
            <p className="text-sm opacity-90 font-medium">剧集</p>
            <p className="text-3xl font-bold">{formatNumber(stats.EpisodeCount)}</p>
          </div>
        </div>
      </div>
    </div>
  )
}
