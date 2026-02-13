import request from './request'
import type {
  CreateSubscriptionRequest,
  EmbyConfigResponse,
  MediaStatsResponse,
  Subscription,
  SubscriptionListQuery,
  UserInfo
} from '@/types/api'

// ==================== 订阅管理 ====================
export function getSubscriptions(params: SubscriptionListQuery): Promise<{ data: Subscription[]; total: number }> {
  return request({
    url: '/subscriptions',
    method: 'get',
    params
  })
}

export function createSubscription(data: CreateSubscriptionRequest): Promise<{ success: boolean }> {
  return request({
    url: '/subscriptions',
    method: 'post',
    data
  })
}

export function deleteSubscription(id: string): Promise<{ success: boolean }> {
  return request({
    url: `/subscriptions/${id}`,
    method: 'delete'
  })
}

// ==================== 个人信息 ====================
export function getProfile(): Promise<UserInfo> {
  return request({
    url: '/profile',
    method: 'get'
  })
}

export function updateProfile(data: { email?: string }) {
  return request({
    url: '/profile',
    method: 'put',
    data
  })
}

export function updatePassword(data: { oldPassword: string; newPassword: string }) {
  return request({
    url: '/password',
    method: 'put',
    data
  })
}

export function updateEmail(newEmail: string) {
  return request({
    url: '/email',
    method: 'put',
    data: { newEmail }
  })
}

// ==================== 媒体信息 ====================
export function getEmbyConfig(): Promise<EmbyConfigResponse> {
  return request({
    url: '/emby/config',
    method: 'get'
  })
}

export function getMediaStats(): Promise<MediaStatsResponse> {
  return request({
    url: '/media/stats',
    method: 'get'
  })
}
