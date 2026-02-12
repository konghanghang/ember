import request from './request'
import type {
  CreateSubscriptionRequest,
  EmbyConfigResponse,
  MediaStatsResponse,
  RedeemCodeRequest,
  RedeemCodeResponse,
  RedemptionCode,
  RedemptionListResponse,
  Subscription,
  TmdbSearchResponse,
  UserInfo
} from '@/types/api'

// User Profile
export function getUserProfile(): Promise<UserInfo> {
  return request({
    url: '/user/profile',
    method: 'get'
  })
}

export function updateUserProfile(data: { email: string }): Promise<{ success: boolean }> {
  return request({
    url: '/user/profile',
    method: 'put',
    data
  })
}

export function updateUserEmail(email: string): Promise<{ success: boolean }> {
  return request({
    url: '/user/email',
    method: 'put',
    data: { newEmail: email }
  })
}

export function updateUserPassword(data: { oldPassword: string; newPassword: string }): Promise<{ success: boolean }> {
  return request({
    url: '/user/password',
    method: 'put',
    data
  })
}

export function redeemCode(data: RedeemCodeRequest): Promise<RedeemCodeResponse> {
	return request({
		url: '/user/redeem',
		method: 'post',
		data
	})
}

export function validateRedeemCode(code: string): Promise<RedemptionCode> {
	return request({
		url: `/user/redeem/${code}/validate`,
		method: 'get'
	})
}

export function getRedemptions(params?: { page?: number; pageSize?: number }): Promise<RedemptionListResponse> {
	return request({
		url: '/user/redemptions',
		method: 'get',
		params
	})
}

// Media Info
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

// Subscriptions
export function getUserSubscriptions(): Promise<Subscription[]> {
  return request({
    url: '/user/subscriptions',
    method: 'get'
  })
}

export function createSubscription(data: CreateSubscriptionRequest): Promise<{ success: boolean }> {
  return request({
    url: '/user/subscriptions',
    method: 'post',
    data
  })
}

export function deleteSubscription(id: string): Promise<{ success: boolean }> {
  return request({
    url: `/user/subscriptions/${id}`,
    method: 'delete'
  })
}

// TMDB Search
export function searchTmdb(query: string, type: 'movie' | 'tv'): Promise<TmdbSearchResponse> {
  return request({
    url: '/tmdb/search',
    method: 'get',
    params: { query, type }
  })
}
