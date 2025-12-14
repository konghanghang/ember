'use client'

import { useState, useEffect, useRef } from 'react'
import { MediaType } from '@prisma/client'
import Image from 'next/image'

interface TmdbResult {
  id: number
  title: string
  originalTitle: string
  overview: string
  posterPath: string | null
  releaseDate: string
  mediaType: string
}

interface TmdbSearchInputProps {
  mediaType: MediaType
  onSelect: (result: { tmdbId: string; name: string }) => void
  placeholder?: string
}

export default function TmdbSearchInput({
  mediaType,
  onSelect,
  placeholder = '输入影视名称搜索...',
}: TmdbSearchInputProps) {
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<TmdbResult[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [showResults, setShowResults] = useState(false)
  const wrapperRef = useRef<HTMLDivElement>(null)

  // 点击外部关闭结果列表
  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (
        wrapperRef.current &&
        !wrapperRef.current.contains(event.target as Node)
      ) {
        setShowResults(false)
      }
    }

    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [])

  // 搜索函数
  const handleSearch = async () => {
    const trimmedQuery = query.trim()
    if (!trimmedQuery) return

    setLoading(true)
    setError('')
    setResults([])

    try {
      const type = mediaType === 'MOVIE' ? 'movie' : 'tv'
      const response = await fetch(
        `/api/tmdb/search?query=${encodeURIComponent(trimmedQuery)}&type=${type}`
      )

      if (!response.ok) {
        throw new Error('搜索失败')
      }

      const data = await response.json()
      setResults(data.results || [])
      setShowResults(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : '搜索失败')
      setResults([])
    } finally {
      setLoading(false)
    }
  }

  // 回车键触发搜索
  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault()
      handleSearch()
    }
  }

  const handleSelect = (result: TmdbResult) => {
    onSelect({
      tmdbId: String(result.id),
      name: result.title,
    })
    setQuery(result.title)
    setShowResults(false)
  }

  return (
    <div ref={wrapperRef} className="relative">
      {/* 搜索输入框 */}
      <div className="relative flex gap-2">
        <input
          type="text"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={placeholder}
          disabled={loading}
          className="flex-1 px-4 py-3 border border-gray-300 dark:border-gray-600 rounded-lg focus:ring-2 focus:ring-indigo-500 focus:border-transparent dark:bg-gray-700 dark:text-white transition-colors disabled:opacity-50"
        />
        <button
          type="button"
          onClick={handleSearch}
          disabled={loading || !query.trim()}
          className="px-6 py-3 bg-indigo-600 hover:bg-indigo-700 disabled:bg-gray-400 text-white font-medium rounded-lg transition-colors focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2 flex items-center gap-2"
        >
          {loading ? (
            <>
              <div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin" />
              <span>搜索中</span>
            </>
          ) : (
            <>
              <svg
                className="w-4 h-4"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
                />
              </svg>
              <span>搜索</span>
            </>
          )}
        </button>
      </div>

      {/* 错误提示 */}
      {error && (
        <p className="mt-2 text-sm text-red-600 dark:text-red-400">{error}</p>
      )}

      {/* 搜索结果列表 */}
      {showResults && results.length > 0 && (
        <div className="absolute z-10 w-full mt-2 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg shadow-lg max-h-96 overflow-y-auto">
          {results.map((result) => (
            <button
              key={result.id}
              type="button"
              onClick={() => handleSelect(result)}
              className="w-full p-3 flex gap-3 hover:bg-gray-50 dark:hover:bg-gray-700 transition-colors text-left border-b border-gray-100 dark:border-gray-700 last:border-b-0"
            >
              {/* 海报 */}
              <div className="flex-shrink-0 w-12 h-18 bg-gray-200 dark:bg-gray-700 rounded overflow-hidden relative">
                {result.posterPath ? (
                  <Image
                    src={`https://image.tmdb.org/t/p/w92${result.posterPath}`}
                    alt={result.title}
                    fill
                    className="object-cover"
                    sizes="48px"
                  />
                ) : (
                  <div className="w-full h-full flex items-center justify-center text-gray-400 text-xs">
                    无图
                  </div>
                )}
              </div>

              {/* 信息 */}
              <div className="flex-1 min-w-0">
                <h3 className="font-medium text-gray-900 dark:text-white truncate">
                  {result.title}
                </h3>
                {result.originalTitle !== result.title && (
                  <p className="text-xs text-gray-500 dark:text-gray-400 truncate">
                    {result.originalTitle}
                  </p>
                )}
                <p className="text-sm text-gray-600 dark:text-gray-300 mt-1">
                  {result.releaseDate
                    ? new Date(result.releaseDate).getFullYear()
                    : '未知'}
                </p>
              </div>
            </button>
          ))}
        </div>
      )}

      {/* 无结果提示 */}
      {showResults && results.length === 0 && !loading && (
        <div className="absolute z-10 w-full mt-2 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg shadow-lg p-4 text-center text-gray-500 dark:text-gray-400">
          未找到相关结果
        </div>
      )}
    </div>
  )
}
