import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { MoviePilotClient } from './moviepilot'

// Mock fetch
global.fetch = vi.fn()

describe('MoviePilotClient', () => {
  let client: MoviePilotClient
  const originalEnv = process.env

  beforeEach(() => {
    // 设置测试环境变量
    process.env = {
      ...originalEnv,
      MOVIEPILOT_URL: 'http://test-mp.com',
      MOVIEPILOT_USERNAME: 'testuser',
      MOVIEPILOT_PASSWORD: 'testpass',
    }
    client = new MoviePilotClient()
    vi.clearAllMocks()
  })

  afterEach(() => {
    process.env = originalEnv
  })

  describe('constructor', () => {
    it('应该正确初始化配置', () => {
      expect(client.isConfigured()).toBe(true)
    })

    it('应该移除 URL 末尾的斜杠', () => {
      process.env.MOVIEPILOT_URL = 'http://test-mp.com/'
      const newClient = new MoviePilotClient()
      expect(newClient.isConfigured()).toBe(true)
    })

    it('应该检测缺少配置', () => {
      process.env.MOVIEPILOT_URL = ''
      const newClient = new MoviePilotClient()
      expect(newClient.isConfigured()).toBe(false)
    })
  })

  describe('isConfigured', () => {
    it('应该返回 true 当所有配置存在', () => {
      expect(client.isConfigured()).toBe(true)
    })

    it('应该返回 false 当缺少 URL', () => {
      process.env.MOVIEPILOT_URL = ''
      const newClient = new MoviePilotClient()
      expect(newClient.isConfigured()).toBe(false)
    })

    it('应该返回 false 当缺少用户名', () => {
      process.env.MOVIEPILOT_USERNAME = ''
      const newClient = new MoviePilotClient()
      expect(newClient.isConfigured()).toBe(false)
    })

    it('应该返回 false 当缺少密码', () => {
      process.env.MOVIEPILOT_PASSWORD = ''
      const newClient = new MoviePilotClient()
      expect(newClient.isConfigured()).toBe(false)
    })
  })

  describe('createSubscription', () => {
    it('应该成功创建电影订阅', async () => {
      // Mock 登录响应
      const mockLoginResponse = {
        ok: true,
        json: async () => ({ access_token: 'test-token-123' }),
      }

      // Mock 订阅响应
      const mockSubscribeResponse = {
        ok: true,
        json: async () => ({ success: true, id: 'sub-123' }),
      }

      ;(global.fetch as any)
        .mockResolvedValueOnce(mockLoginResponse) // 登录
        .mockResolvedValueOnce(mockSubscribeResponse) // 创建订阅

      const result = await client.createSubscription({
        type: 'movie',
        name: '肖申克的救赎',
        tmdbid: '278',
      })

      expect(result).toEqual({ success: true, id: 'sub-123' })

      // 验证登录请求
      expect(global.fetch).toHaveBeenNthCalledWith(
        1,
        'http://test-mp.com/api/v1/login/access-token',
        expect.objectContaining({
          method: 'POST',
          headers: {
            'Content-Type': 'application/x-www-form-urlencoded',
          },
        })
      )

      // 验证订阅请求
      expect(global.fetch).toHaveBeenNthCalledWith(
        2,
        'http://test-mp.com/api/v1/subscribe/',
        expect.objectContaining({
          method: 'POST',
          headers: {
            'Authorization': 'Bearer test-token-123',
            'Content-Type': 'application/json',
          },
        })
      )

      // 验证请求体
      const subscribeCall = (global.fetch as any).mock.calls[1]
      const requestBody = JSON.parse(subscribeCall[1].body)
      expect(requestBody).toEqual({
        type: '电影',
        name: '肖申克的救赎',
        tmdbid: 278,
      })
    })

    it('应该成功创建电视剧订阅', async () => {
      const mockLoginResponse = {
        ok: true,
        json: async () => ({ access_token: 'test-token' }),
      }

      const mockSubscribeResponse = {
        ok: true,
        json: async () => ({ success: true }),
      }

      ;(global.fetch as any)
        .mockResolvedValueOnce(mockLoginResponse)
        .mockResolvedValueOnce(mockSubscribeResponse)

      await client.createSubscription({
        type: 'tv',
        name: '权力的游戏',
        tmdbid: '1399',
      })

      // 验证类型映射
      const subscribeCall = (global.fetch as any).mock.calls[1]
      const requestBody = JSON.parse(subscribeCall[1].body)
      expect(requestBody.type).toBe('电视剧')
    })

    it('应该在登录失败时抛出错误', async () => {
      const mockLoginResponse = {
        ok: false,
        status: 401,
        statusText: 'Unauthorized',
      }

      ;(global.fetch as any).mockResolvedValueOnce(mockLoginResponse)

      await expect(
        client.createSubscription({
          type: 'movie',
          name: 'Test Movie',
          tmdbid: '123',
        })
      ).rejects.toThrow('MoviePilot 登录失败: 401 Unauthorized')
    })

    it('应该在订阅 API 失败时抛出错误', async () => {
      const mockLoginResponse = {
        ok: true,
        json: async () => ({ access_token: 'test-token' }),
      }

      const mockSubscribeResponse = {
        ok: false,
        status: 400,
        text: async () => 'Invalid request',
      }

      ;(global.fetch as any)
        .mockResolvedValueOnce(mockLoginResponse)
        .mockResolvedValueOnce(mockSubscribeResponse)

      await expect(
        client.createSubscription({
          type: 'movie',
          name: 'Test Movie',
          tmdbid: '123',
        })
      ).rejects.toThrow('MoviePilot API 错误: 400 Invalid request')
    })

    it('应该正确解析 TMDB ID（字符串转整数）', async () => {
      const mockLoginResponse = {
        ok: true,
        json: async () => ({ access_token: 'test-token' }),
      }

      const mockSubscribeResponse = {
        ok: true,
        json: async () => ({ success: true }),
      }

      ;(global.fetch as any)
        .mockResolvedValueOnce(mockLoginResponse)
        .mockResolvedValueOnce(mockSubscribeResponse)

      await client.createSubscription({
        type: 'movie',
        name: 'Test',
        tmdbid: '12345',
      })

      const subscribeCall = (global.fetch as any).mock.calls[1]
      const requestBody = JSON.parse(subscribeCall[1].body)
      expect(requestBody.tmdbid).toBe(12345)
      expect(typeof requestBody.tmdbid).toBe('number')
    })
  })
})
