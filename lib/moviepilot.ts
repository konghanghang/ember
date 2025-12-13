/**
 * MoviePilot API 客户端
 *
 * 功能：
 * - OAuth2 认证（每次调用重新登录，不缓存 token）
 * - 创建订阅（调用 MoviePilot API）
 *
 * API 文档参考：
 * - 登录：POST /api/v1/login/access-token
 * - 订阅：POST /api/v1/subscribe/
 */

export class MoviePilotClient {
  private baseUrl: string
  private username: string
  private password: string

  constructor() {
    this.baseUrl = (process.env.MOVIEPILOT_URL || '').replace(/\/$/, '')
    this.username = process.env.MOVIEPILOT_USERNAME || ''
    this.password = process.env.MOVIEPILOT_PASSWORD || ''

    // 启动时检查配置
    if (!this.baseUrl || !this.username || !this.password) {
      console.warn('MoviePilot 配置不完整，API 功能将不可用')
    }
  }

  /**
   * 获取 Access Token（每次都重新登录）
   *
   * 不缓存 token 的原因：
   * - 简化实现，无需处理 token 过期、刷新逻辑
   * - 调用频率低（仅在审核通过时），性能影响可忽略
   */
  private async login(): Promise<string> {
    const response = await fetch(`${this.baseUrl}/api/v1/login/access-token`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
      },
      body: new URLSearchParams({
        username: this.username,
        password: this.password,
      }),
    })

    if (!response.ok) {
      throw new Error(`MoviePilot 登录失败: ${response.status} ${response.statusText}`)
    }

    const data = await response.json()
    return data.access_token
  }

  /**
   * 创建订阅
   *
   * @param data.type - 媒体类型（'movie' | 'tv'）
   * @param data.name - 影视名称
   * @param data.tmdbid - TMDB ID（字符串，会转为整数）
   * @returns MoviePilot API 响应
   * @throws 登录失败或 API 调用失败时抛出错误
   */
  async createSubscription(data: {
    type: 'movie' | 'tv'
    name: string
    tmdbid: string
  }) {
    // 1. 登录获取 token
    const token = await this.login()

    // 2. 调用订阅 API
    const response = await fetch(`${this.baseUrl}/api/v1/subscribe/`, {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        type: data.type === 'movie' ? '电影' : '电视剧',
        name: data.name,
        tmdbid: parseInt(data.tmdbid, 10),
      }),
    })

    if (!response.ok) {
      const errorText = await response.text()
      throw new Error(`MoviePilot API 错误: ${response.status} ${errorText}`)
    }

    return await response.json()
  }

  /**
   * 检查配置是否完整
   */
  isConfigured(): boolean {
    return !!(this.baseUrl && this.username && this.password)
  }
}

// 导出单例
export const moviepilotClient = new MoviePilotClient()
