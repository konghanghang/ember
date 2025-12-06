/**
 * Emby API 客户端
 *
 * 参考文档：docs/emby-api-guide.md
 */

/**
 * Emby 用户接口
 */
export interface EmbyUser {
  Id: string
  Name: string
  ServerId?: string
  HasPassword?: boolean
  HasConfiguredPassword?: boolean
  HasConfiguredEasyPassword?: boolean
  EnableAutoLogin?: boolean
  LastLoginDate?: string
  LastActivityDate?: string
  Policy?: EmbyUserPolicy
}

/**
 * Emby 用户权限策略
 */
export interface EmbyUserPolicy {
  IsAdministrator: boolean
  IsDisabled: boolean
  EnableAllFolders: boolean
  EnabledFolders: string[]
  EnableRemoteAccess: boolean
  EnableLiveTvAccess: boolean
  EnableContentDeletion: boolean
  EnableContentDownloading: boolean
  EnableSyncTranscoding: boolean
  EnableMediaPlayback: boolean
  EnableAudioPlaybackTranscoding: boolean
  EnableVideoPlaybackTranscoding: boolean
  EnablePlaybackRemuxing: boolean
}

/**
 * 创建用户请求参数
 */
export interface CreateUserRequest {
  Name: string
  Password?: string
}

/**
 * Emby API 错误
 */
export class EmbyAPIError extends Error {
  constructor(
    message: string,
    public statusCode?: number,
    public response?: any
  ) {
    super(message)
    this.name = 'EmbyAPIError'
  }
}

/**
 * Emby API 客户端类
 */
export class EmbyClient {
  private baseUrl: string
  private apiKey: string

  constructor() {
    const embyUrl = process.env.EMBY_URL
    const embyApiKey = process.env.EMBY_API_KEY

    if (!embyUrl || !embyApiKey) {
      throw new Error('缺少 Emby 配置：请在 .env 中设置 EMBY_URL 和 EMBY_API_KEY')
    }

    this.baseUrl = embyUrl.replace(/\/$/, '') // 移除末尾斜杠
    this.apiKey = embyApiKey
  }

  /**
   * 发送 HTTP 请求（带重试机制）
   * @param endpoint API 端点（如 /Users/New）
   * @param options Fetch 选项
   * @param retries 重试次数（默认 3 次）
   */
  private async request<T>(
    endpoint: string,
    options: RequestInit = {},
    retries = 3
  ): Promise<T> {
    const url = `${this.baseUrl}${endpoint}`

    for (let attempt = 0; attempt < retries; attempt++) {
      try {
        const response = await fetch(url, {
          ...options,
          headers: {
            'X-Emby-Token': this.apiKey,
            'Content-Type': 'application/json',
            ...options.headers,
          },
        })

        // 处理非 2xx 响应
        if (!response.ok) {
          const errorText = await response.text()
          throw new EmbyAPIError(
            `Emby API 错误: ${response.statusText}`,
            response.status,
            errorText
          )
        }

        // 解析 JSON（如果响应为空则返回空对象）
        const text = await response.text()
        return text ? JSON.parse(text) : ({} as T)
      } catch (error) {
        // 最后一次重试失败，抛出异常
        if (attempt === retries - 1) {
          if (error instanceof EmbyAPIError) {
            throw error
          }
          throw new EmbyAPIError(
            `Emby API 请求失败: ${error instanceof Error ? error.message : String(error)}`
          )
        }

        // 指数退避：等待 1s, 2s, 4s...
        const delay = Math.pow(2, attempt) * 1000
        await new Promise((resolve) => setTimeout(resolve, delay))
      }
    }

    throw new EmbyAPIError('Emby API 请求失败：超过最大重试次数')
  }

  /**
   * 创建 Emby 用户
   * @param data 用户信息 { Name, Password }
   * @returns 创建的用户对象
   */
  async createUser(data: CreateUserRequest): Promise<EmbyUser> {
    return this.request<EmbyUser>('/Users/New', {
      method: 'POST',
      body: JSON.stringify(data),
    })
  }

  /**
   * 删除 Emby 用户
   * @param userId Emby 用户 ID
   */
  async deleteUser(userId: string): Promise<void> {
    await this.request(`/Users/${userId}`, {
      method: 'DELETE',
    })
  }

  /**
   * 更新用户权限策略
   * @param userId Emby 用户 ID
   * @param policy 权限策略（部分更新）
   */
  async setUserPolicy(
    userId: string,
    policy: Partial<EmbyUserPolicy>
  ): Promise<void> {
    await this.request(`/Users/${userId}/Policy`, {
      method: 'POST',
      body: JSON.stringify(policy),
    })
  }

  /**
   * 获取所有 Emby 用户
   * @returns 用户列表
   */
  async getUsers(): Promise<EmbyUser[]> {
    return this.request<EmbyUser[]>('/Users')
  }

  /**
   * 获取单个 Emby 用户
   * @param userId Emby 用户 ID
   * @returns 用户对象
   */
  async getUser(userId: string): Promise<EmbyUser> {
    return this.request<EmbyUser>(`/Users/${userId}`)
  }

  /**
   * 测试 Emby 连接
   * @returns { success: boolean, message: string, serverName?: string }
   */
  async testConnection(): Promise<{
    success: boolean
    message: string
    serverName?: string
  }> {
    try {
      const info = await this.request<{ ServerName: string }>('/System/Info')
      return {
        success: true,
        message: '连接成功',
        serverName: info.ServerName,
      }
    } catch (error) {
      return {
        success: false,
        message:
          error instanceof EmbyAPIError
            ? error.message
            : 'Emby 连接失败：未知错误',
      }
    }
  }

  /**
   * 获取默认用户权限配置
   *
   * 根据需求文档 (docs/specs/requirements.md) 定义的默认权限
   */
  static getDefaultPolicy(): EmbyUserPolicy {
    return {
      IsAdministrator: false,
      IsDisabled: false,
      EnableAllFolders: true,
      EnabledFolders: [],
      EnableRemoteAccess: true,
      EnableLiveTvAccess: false,
      EnableContentDeletion: false,
      EnableContentDownloading: false,
      EnableSyncTranscoding: false,
      EnableMediaPlayback: true,
      EnableAudioPlaybackTranscoding: true,
      EnableVideoPlaybackTranscoding: true,
      EnablePlaybackRemuxing: true,
    }
  }
}

/**
 * 导出单例实例
 */
export const embyClient = new EmbyClient()
