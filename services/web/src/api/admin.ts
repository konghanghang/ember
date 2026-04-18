import request from './request'
import type {
  ActiveSession,
  AdminPaymentQuery,
  AdminRedemptionQuery,
  AdminConfigItem,
  AdminConfigListResponse,
  ClientBlacklist,
  ConfigGroupTestResult,
  CreateAdminUserRequest,
  CreatePlanRequest,
  CreatePlanGroupRequest,
  CreateRedemptionCodeRequest,
  CreateRedemptionCodesBatchRequest,
  CreateRedemptionCodesBatchResponse,
  CronCheckResponse,
  DeviceAction,
  DeviceListQuery,
  DeviceListResponse,
  DeviceStats,
  MediaGapActionResponse,
  MediaGapDispatchRequest,
  MediaGapListQuery,
  MediaGapListResponse,
  MediaGapScanRequest,
  MediaGapScanResponse,
  MediaGapSearchResult,
  MediaQualityLowDetailItem,
  MediaQualityLibrary,
  MediaQualityReport,
  PaymentListResponse,
  ManagedPlanGroup,
  PlaybackProfileListQuery,
  PlaybackProfileListResponse,
  PlaybackProfileQuery,
  PlaybackProfileResponse,
  PlaybackHistoryQuery,
  PlaybackHistoryResponse,
  Plan,
  PlanListResponse,
  UpdateAdminUserRequest,
  UpdateRedemptionCodeRequest,
  UpdatePlanGroupRequest,
  UpdatePlanRequest,
  RedemptionCode,
  RedemptionCodeListResponse,
  RedemptionListResponse,
  RankingPreviewResponse,
  RankingPeriod,
  RedemptionCodeListQuery,
  SystemInfoResponse,
  UserTemplate,
  UserInfo,
  UserListQuery,
  UserListResponse,
  UpdateAdminConfigRequest
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

export function createAdminUser(data: CreateAdminUserRequest): Promise<UserInfo> {
  return request({
    url: '/admin/users',
    method: 'post',
    data
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

export function getRedemptionCodes(params?: RedemptionCodeListQuery): Promise<RedemptionCodeListResponse> {
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

export function createRedemptionCodesBatch(data: CreateRedemptionCodesBatchRequest): Promise<CreateRedemptionCodesBatchResponse> {
  return request({
    url: '/admin/redemption-codes/batch',
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

export function getUserTemplates(): Promise<{ data: UserTemplate[] }> {
  return request({
    url: '/admin/user-templates',
    method: 'get'
  })
}

export function getConfigs(): Promise<AdminConfigListResponse> {
  return request({
    url: '/admin/configs',
    method: 'get'
  })
}

export function updateConfig(key: string, data: UpdateAdminConfigRequest): Promise<AdminConfigItem> {
  return request({
    url: `/admin/configs/${key}`,
    method: 'patch',
    data
  })
}

export function testConfigGroup(group: string): Promise<ConfigGroupTestResult> {
  return request({
    url: `/admin/configs/${group}/test`,
    method: 'post'
  })
}

export function getAllRedemptions(params?: AdminRedemptionQuery): Promise<RedemptionListResponse> {
	return request({
		url: '/admin/redemptions',
		method: 'get',
		params
	})
}

export function getPlaybackHistory(params?: PlaybackHistoryQuery): Promise<PlaybackHistoryResponse> {
  return request({
    url: '/admin/playback-history',
    method: 'get',
    params
  })
}

export function getUserPlaybackProfile(userId: string, params?: PlaybackProfileQuery): Promise<PlaybackProfileResponse> {
  return request({
    url: `/admin/users/${encodeURIComponent(userId)}/profile`,
    method: 'get',
    params
  })
}

export function getUserPlaybackProfiles(params?: PlaybackProfileListQuery): Promise<PlaybackProfileListResponse> {
  return request({
    url: '/admin/playback-profiles',
    method: 'get',
    params
  })
}

export function getMediaQualityLibraries(): Promise<{ data: MediaQualityLibrary[] }> {
  return request({
    url: '/admin/media-quality/libraries',
    method: 'get'
  })
}

export function getMediaQualityReport(
  libraryId: string,
  opts?: { force?: boolean; page?: number; pageSize?: number }
): Promise<{ data: MediaQualityReport }> {
  return request({
    url: `/admin/media-quality/libraries/${encodeURIComponent(libraryId)}`,
    method: 'get',
    params: {
      force: opts?.force === true,
      page: opts?.page,
      pageSize: opts?.pageSize
    }
  })
}

export function scanMediaQualityLibrary(libraryId: string): Promise<{ data: MediaQualityReport }> {
  return request({
    url: `/admin/media-quality/libraries/${encodeURIComponent(libraryId)}/scan`,
    method: 'post'
  })
}

export function getMediaQualityPoster(itemId: string): Promise<Blob> {
  return request({
    url: `/admin/media-quality/posters/${encodeURIComponent(itemId)}`,
    method: 'get',
    responseType: 'blob',
    silent: true
  }) as unknown as Promise<Blob>
}

export function getMediaQualityGroupDetails(
  libraryId: string,
  groupId: string,
  params?: { page?: number; pageSize?: number; force?: boolean }
): Promise<{ data: MediaQualityLowDetailItem[]; total: number; page: number; pageSize: number }> {
  return request({
    url: `/admin/media-quality/libraries/${encodeURIComponent(libraryId)}/groups/${encodeURIComponent(groupId)}/details`,
    method: 'get',
    params
  })
}

// ==================== 缺集管理 ====================
export function getMediaGaps(params?: MediaGapListQuery): Promise<MediaGapListResponse> {
  return request({
    url: '/admin/media-gaps',
    method: 'get',
    params
  })
}

export function scanMediaGaps(data?: MediaGapScanRequest): Promise<{ data: MediaGapScanResponse }> {
  return request({
    url: '/admin/media-gaps/scan',
    method: 'post',
    data: data ?? {}
  })
}

export function searchMediaGap(id: string): Promise<{ data: MediaGapSearchResult }> {
  return request({
    url: `/admin/media-gaps/${encodeURIComponent(id)}/search`,
    method: 'post'
  })
}

export function dispatchMediaGap(id: string, data: MediaGapDispatchRequest): Promise<{ data: MediaGapActionResponse }> {
  return request({
    url: `/admin/media-gaps/${encodeURIComponent(id)}/dispatch`,
    method: 'post',
    data
  })
}

export function ignoreMediaGap(id: string, data?: { reason?: string }): Promise<{ data: MediaGapActionResponse }> {
  return request({
    url: `/admin/media-gaps/${encodeURIComponent(id)}/ignore`,
    method: 'post',
    data: data ?? {}
  })
}

// ==================== 付费方案 ====================
export function getPlanGroups(): Promise<{ data: ManagedPlanGroup[] }> {
  return request({
    url: '/admin/plan-groups',
    method: 'get'
  })
}

export function createPlanGroup(data: CreatePlanGroupRequest): Promise<ManagedPlanGroup> {
  return request({
    url: '/admin/plan-groups',
    method: 'post',
    data
  })
}

export function updatePlanGroup(key: string, data: UpdatePlanGroupRequest): Promise<ManagedPlanGroup> {
  return request({
    url: `/admin/plan-groups/${encodeURIComponent(key)}`,
    method: 'put',
    data
  })
}

export function deletePlanGroup(key: string) {
  return request({
    url: `/admin/plan-groups/${encodeURIComponent(key)}`,
    method: 'delete'
  })
}

export function getPlans(params?: { page?: number; pageSize?: number; showAll?: boolean; planGroup?: string }): Promise<PlanListResponse> {
  return request({
    url: '/admin/plans',
    method: 'get',
    params
  })
}

export function createPlan(data: CreatePlanRequest): Promise<Plan> {
  return request({
    url: '/admin/plans',
    method: 'post',
    data
  })
}

export function updatePlan(id: string, data: UpdatePlanRequest): Promise<Plan> {
  return request({
    url: `/admin/plans/${id}`,
    method: 'put',
    data
  })
}

export function deletePlan(id: string) {
  return request({
    url: `/admin/plans/${id}`,
    method: 'delete'
  })
}

export function getAllPayments(params?: AdminPaymentQuery): Promise<PaymentListResponse> {
  return request({
    url: '/admin/payments',
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

export function rejectSubscription(id: string, reason: string) {
  return request({
    url: `/admin/subscriptions/${id}/reject`,
    method: 'put',
    data: { reason }
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
export function previewRanking(type: RankingPeriod): Promise<RankingPreviewResponse> {
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

// ==================== 设备管理 ====================
export function getDevices(params?: DeviceListQuery): Promise<DeviceListResponse> {
  return request({
    url: '/admin/devices',
    method: 'get',
    params
  })
}

export function getDeviceStats(): Promise<{ data: DeviceStats }> {
  return request({
    url: '/admin/devices/stats',
    method: 'get'
  })
}

export function getDeviceActions(params?: { limit?: number }): Promise<{ data: DeviceAction[] }> {
  return request({
    url: '/admin/devices/actions',
    method: 'get',
    params
  })
}

export function getDeviceBlacklist(): Promise<{ data: ClientBlacklist[] }> {
  return request({
    url: '/admin/devices/blacklist',
    method: 'get'
  })
}

export function addDeviceBlacklist(data: { clientName: string; reason?: string }) {
  return request({
    url: '/admin/devices/blacklist',
    method: 'post',
    data
  })
}

export function removeDeviceBlacklist(clientName: string) {
  return request({
    url: `/admin/devices/blacklist/${encodeURIComponent(clientName)}`,
    method: 'delete'
  })
}

export function logoutDevice(deviceId: string) {
  return request({
    url: `/admin/devices/logout/${encodeURIComponent(deviceId)}`,
    method: 'post'
  })
}

export function logoutBlacklistedDevices() {
  return request({
    url: '/admin/devices/blacklist/logout-all',
    method: 'post'
  })
}

// ==================== 追剧日历 ====================
export function syncTVCalendar(data?: { tmdbId?: string; force?: boolean; weekOffsets?: number[] }): Promise<{ success: boolean; count: number }> {
  return request({
    url: '/admin/tv-calendar/sync',
    method: 'post',
    data: data ?? {}
  })
}

export const refreshTVCalendar = syncTVCalendar
