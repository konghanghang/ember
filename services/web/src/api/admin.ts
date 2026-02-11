import request from './request'
import type {
  CreateInviteRequest,
  InviteListResponse,
  Subscription,
  SubscriptionListQuery,
  SystemInfoResponse,
  UserInfo,
  UserListQuery,
  UserListResponse,
  CronCheckResponse
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

// Invite Management
export function getInvites(params?: Record<string, never>): Promise<InviteListResponse> {
  return request({
    url: '/admin/invites',
    method: 'get',
    params
  })
}

export function createInvite(data: CreateInviteRequest): Promise<{ invite: InviteListResponse['invites'][number] }> {
  return request({
    url: '/admin/invites',
    method: 'post',
    data
  })
}

export function deleteInvite(id: string) {
  return request({
    url: `/admin/invites/${id}`,
    method: 'delete'
  })
}

// Subscription Management
export function getAllSubscriptions(params: SubscriptionListQuery): Promise<{ data: Subscription[]; total: number }> {
  return request({
    url: '/admin/subscriptions',
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
