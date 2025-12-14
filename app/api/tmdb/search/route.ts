import { NextRequest, NextResponse } from 'next/server'

interface TmdbSearchResult {
  id: number
  title?: string
  name?: string
  original_title?: string
  original_name?: string
  overview: string
  poster_path: string | null
  release_date?: string
  first_air_date?: string
}

interface TmdbApiResponse {
  results: TmdbSearchResult[]
  total_results: number
}

/**
 * TMDB 搜索 API 代理
 * 用于在订阅页面搜索电影和电视剧
 */
export async function GET(request: NextRequest) {
  try {
    const searchParams = request.nextUrl.searchParams
    const query = searchParams.get('query')
    const type = searchParams.get('type') || 'movie' // movie 或 tv

    if (!query) {
      return NextResponse.json(
        { error: '缺少搜索关键词' },
        { status: 400 }
      )
    }

    const apiKey = process.env.TMDB_API_KEY
    if (!apiKey) {
      console.error('TMDB_API_KEY 未配置')
      return NextResponse.json(
        { error: 'TMDB API 未配置' },
        { status: 500 }
      )
    }

    // 根据类型选择搜索端点
    const endpoint = type === 'tv' ? 'search/tv' : 'search/movie'
    const tmdbUrl = `https://api.themoviedb.org/3/${endpoint}?api_key=${apiKey}&query=${encodeURIComponent(query)}&language=zh-CN&page=1`

    const response = await fetch(tmdbUrl)

    if (!response.ok) {
      throw new Error(`TMDB API 返回错误: ${response.status}`)
    }

    const data = (await response.json()) as TmdbApiResponse

    // 转换为统一格式
    const results = data.results.map((item) => ({
      id: item.id,
      title: item.title || item.name, // 电影用 title，电视剧用 name
      originalTitle: item.original_title || item.original_name,
      overview: item.overview,
      posterPath: item.poster_path,
      releaseDate: item.release_date || item.first_air_date,
      mediaType: type,
    }))

    return NextResponse.json({
      results,
      total: data.total_results,
    })
  } catch (error) {
    console.error('TMDB 搜索失败：', error)

    return NextResponse.json(
      {
        error: error instanceof Error ? error.message : '搜索失败',
      },
      { status: 500 }
    )
  }
}
