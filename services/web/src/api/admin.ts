import request from './request'
import type {
  CreateRedemptionCodeRequest,
  UpdateRedemptionCodeRequest,
  RedemptionCode,
  RedemptionCodeListResponse,
  RedemptionListResponse,
  RankingPreviewResponse,
  Setting,
  UpdateSettingRequest,
  SystemInfoResponse,
  UserInfo,
  UserListQuery,
  UserListResponse,
  CronCheckResponse,
  UpdateAdminUserRequest,
  ActiveSession
} from '@/types/api'

// User Management
export function getUsers(params: UserListQuery): Promise<UserListResponse> {
  return request({
    url: '/admin/users',
    method: 'get',
    params
  })
}

export function getUserDetail(id: string): Promise<UserInfo> {
  return request({
    url: `/admin/users/${id}`,
    method: 'get'
  })
}

export function updateAdminUser(id: string, data: UpdateAdminUserRequest): Promise<UserInfo> {
  return request({
    url: `/admin/users/${id}`,
    method: 'put',
    data
  })
}

export function extendUserExpiry(id: string, days: number) {
  return request({
    url: `/admin/users/${id}/extend`,
    method: 'put',
    data: { days }
  })
}

export function toggleUserStatus(id: string) {
  return request({
    url: `/admin/users/${id}/toggle`,
    method: 'put'
  })
}

export function deleteUser(id: string) {
  return request({
    url: `/admin/users/${id}`,
    method: 'delete'
  })
}

export function resetUserPassword(id: string, newPassword: string) {
  return request({
    url: `/admin/users/${id}/reset-password`,
    method: 'put',
    data: { newPassword }
  })
}

export function getRedemptionCodes(params?: { page?: number; pageSize?: number; showAll?: boolean }): Promise<RedemptionCodeListResponse> {
  return request({
    url: '/admin/redemption-codes',
    method: 'get',
    params
  })
}

export function createRedemptionCode(data: CreateRedemptionCodeRequest): Promise<RedemptionCode> {
  return request({
    url: '/admin/redemption-codes',
    method: 'post',
    data
  })
}

export function updateRedemptionCode(id: string, data: UpdateRedemptionCodeRequest): Promise<RedemptionCode> {
  return request({
    url: `/admin/redemption-codes/${id}`,
    method: 'put',
    data
  })
}

export function deleteRedemptionCode(id: string) {
  return request({
    url: `/admin/redemption-codes/${id}`,
    method: 'delete'
  })
}

export function getSettings(): Promise<Setting[]> {
	return request({
		url: '/admin/settings',
		method: 'get'
	})
}

export function updateSetting(key: string, data: UpdateSettingRequest): Promise<Setting> {
	return request({
		url: `/admin/settings/${key}`,
		method: 'put',
		data
	})
}

export function getAllRedemptions(params?: { page?: number; pageSize?: number; userId?: string }): Promise<RedemptionListResponse> {
	return request({
		url: '/admin/redemptions',
		method: 'get',
		params
	})
}

export function approveSubscription(id: string) {
  return request({
    url: `/admin/subscriptions/${id}/approve`,
    method: 'put'
  })
}

export function rejectSubscription(id: string) {
  return request({
    url: `/admin/subscriptions/${id}/reject`,
    method: 'put'
  })
}

export function deleteSubscriptionAsAdmin(id: string) {
  return request({
    url: `/admin/subscriptions/${id}`,
    method: 'delete'
  })
}

// System Settings
export function getSystemInfo(): Promise<SystemInfoResponse> {
  return request({
    url: '/admin/system/info',
    method: 'get'
  })
}

export function testEmbyConnection(): Promise<{ success: boolean; message?: string; error?: string }> {
  return request({
    url: '/admin/system/test-emby',
    method: 'post'
  })
}

export function runCronJob(): Promise<CronCheckResponse> {
  return request({
    url: '/admin/cron/check-expired',
    method: 'post'
  })
}

// ==================== 播放排行（管理员预览） ====================
export function previewRanking(type: 'daily' | 'weekly'): Promise<RankingPreviewResponse> {
  return request({
    url: '/admin/rankings/preview',
    method: 'post',
    params: { type }
  })
}

// ==================== 活跃会话 ====================
export function getActiveSessions(opts?: { silent?: boolean }): Promise<{ data: ActiveSession[] }> {
  return request({
    url: '/admin/sessions',
    method: 'get',
    silent: opts?.silent === true
  })
}
