import { request } from './request'
import type {
  ActiveSession,
  AdminEmbyBindingRequest,
  AdminEmbyBindingResponse,
  AdminExternalApiKeyCreatedResponse,
  AdminExternalApiKeyStatusResponse,
  AdminEmbyUserListResponse,
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
  MediaGapGroupedQuery,
  MediaGapGroupedResponse,
  MediaGapListQuery,
  MediaGapListResponse,
  MediaGapScanRequest,
  MediaGapScanResponse,
  MediaGapScanStatus,
  MediaGapSearchResult,
  MediaQualityLowDetailItem,
  MediaQualityLibrary,
  MediaQualityReport,
  MediaLibraryOption,
  MediaLibrarySyncApplyRequest,
  MediaLibrarySyncApplyResult,
  MediaLibrarySyncPreviewResult,
  PaymentListResponse,
  ManagedPlanGroup,
  EmbyPolicySyncBatchCreated,
  EmbyPolicySyncBatchDetail,
  PlanGroupMediaLibraryUpdateResult,
  PlaybackProfileListQuery,
  PlaybackProfileListResponse,
  PlaybackProfileQuery,
  PlaybackProfileResponse,
  PlaybackHistoryQuery,
  PlaybackHistoryResponse,
  P115Account,
  P115AccountListResponse,
  ReplaceP115AccountCookieRequest,
  P115ValidationResult,
  Plan,
  PlanGroupEmbyPolicyTemplate,
  PlanGroupEmbyPolicyTemplateUpdateRequest,
  PlanGroupMediaLibrarySettings,
  PlanListResponse,
  UpdateAdminUserRequest,
  UpdateRedemptionCodeRequest,
  UpdatePlanGroupRequest,
  UpdatePlanRequest,
  RedemptionCode,
  RedemptionCodeListResponse,
  RedemptionListResponse,
  RankingPreviewResponse,
  RankingLibraryAllowlistSettings,
  RankingPeriod,
  RedemptionCodeListQuery,
  SystemInfoResponse,
  SubscriptionManualDispatchRequest,
  SubscriptionManualDispatchResult,
  SubscriptionManualSearchRequest,
  SubscriptionManualSearchResult,
  UserInfo,
  UserListQuery,
  UserListResponse,
  UpdateAdminConfigRequest,
  UpdateP115SourceLocationRequest,
  UpdateP115PlaybackConfigRequest,
  CreateP115AccountRequest
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

export function getExternalApiKeyStatus(): Promise<AdminExternalApiKeyStatusResponse> {
  return request({
    url: '/admin/external-api-key',
    method: 'get'
  })
}

export function generateExternalApiKey(): Promise<AdminExternalApiKeyCreatedResponse> {
  return request({
    url: '/admin/external-api-key',
    method: 'post'
  })
}

export function deleteExternalApiKey(): Promise<AdminExternalApiKeyStatusResponse> {
  return request({
    url: '/admin/external-api-key',
    method: 'delete'
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
  return request<Blob>({
    url: `/admin/media-quality/posters/${encodeURIComponent(itemId)}`,
    method: 'get',
    responseType: 'blob',
    silent: true
  })
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

export function getGroupedMediaGaps(params?: MediaGapGroupedQuery): Promise<MediaGapGroupedResponse> {
  return request({
    url: '/admin/media-gaps/grouped',
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

export function getMediaGapScanStatus(): Promise<{ data: MediaGapScanStatus }> {
  return request({
    url: '/admin/media-gaps/scan-status',
    method: 'get',
    silent: true
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

export function getAdminMediaLibraries(): Promise<{ data: MediaLibraryOption[] }> {
  return request({
    url: '/admin/media-libraries',
    method: 'get'
  })
}

export function getPlanGroupMediaLibraries(key: string): Promise<{ data: PlanGroupMediaLibrarySettings }> {
  return request({
    url: `/admin/plan-groups/${encodeURIComponent(key)}/media-libraries`,
    method: 'get'
  })
}

export function updatePlanGroupMediaLibraries(
  key: string,
  libraryIds: string[],
  applyToExistingUsers: boolean = true
): Promise<{ data: PlanGroupMediaLibraryUpdateResult }> {
  return request({
    url: `/admin/plan-groups/${encodeURIComponent(key)}/media-libraries`,
    method: 'put',
    data: { libraryIds, applyToExistingUsers }
  })
}

export function getPlanGroupEmbyPolicyTemplate(key: string): Promise<{ data: PlanGroupEmbyPolicyTemplate }> {
  return request({
    url: `/admin/plan-groups/${encodeURIComponent(key)}/emby-policy-template`,
    method: 'get'
  })
}

export function updatePlanGroupEmbyPolicyTemplate(
  key: string,
  data: PlanGroupEmbyPolicyTemplateUpdateRequest
): Promise<{ data: EmbyPolicySyncBatchCreated }> {
  return request({
    url: `/admin/plan-groups/${encodeURIComponent(key)}/emby-policy-template`,
    method: 'put',
    data
  })
}

export function getEmbyPolicySyncBatch(id: string): Promise<{ data: EmbyPolicySyncBatchDetail }> {
  return request({
    url: `/admin/emby-policy-sync-batches/${encodeURIComponent(id)}`,
    method: 'get'
  })
}

export function retryFailedEmbyPolicySyncBatch(id: string): Promise<{ data: EmbyPolicySyncBatchCreated }> {
  return request({
    url: `/admin/emby-policy-sync-batches/${encodeURIComponent(id)}/retry-failed`,
    method: 'post'
  })
}

export function previewPlanGroupMediaLibrarySync(
  key: string,
  data?: { userIds?: string[] }
): Promise<{ data: MediaLibrarySyncPreviewResult }> {
  return request({
    url: `/admin/plan-groups/${encodeURIComponent(key)}/media-libraries/sync-preview`,
    method: 'post',
    data: data ?? {}
  })
}

export function applyPlanGroupMediaLibrarySync(
  key: string,
  data: MediaLibrarySyncApplyRequest
): Promise<{ data: MediaLibrarySyncApplyResult }> {
  return request({
    url: `/admin/plan-groups/${encodeURIComponent(key)}/media-libraries/sync-apply`,
    method: 'post',
    data
  })
}

export function clearAdminUserMediaLibraryPreferences(id: string) {
  return request({
    url: `/admin/users/${encodeURIComponent(id)}/media-libraries/preferences`,
    method: 'delete'
  })
}

export function syncAdminUserMediaLibraryPreferences(id: string) {
  return request({
    url: `/admin/users/${encodeURIComponent(id)}/media-libraries/sync`,
    method: 'post'
  })
}

export function retryAdminUserPolicySync(id: string): Promise<{ data: UserInfo }> {
  return request({
    url: `/admin/users/${encodeURIComponent(id)}/emby-policy-sync/retry`,
    method: 'post'
  })
}

export function applyAdminUserCurrentPolicySync(id: string): Promise<{ data: UserInfo }> {
  return request({
    url: `/admin/users/${encodeURIComponent(id)}/emby-policy-sync/apply-current`,
    method: 'post'
  })
}

export function updateAdminUserEmbyAccess(id: string, disabled: boolean): Promise<{ data: UserInfo }> {
  return request({
    url: `/admin/users/${encodeURIComponent(id)}/emby-access`,
    method: 'put',
    data: { disabled }
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

export function markSubscriptionIngested(id: string) {
  return request({
    url: `/admin/subscriptions/${id}/ingest`,
    method: 'put'
  })
}

export function redispatchSubscription(id: string) {
  return request({
    url: `/admin/subscriptions/${id}/redispatch`,
    method: 'put'
  })
}

export function manualSearchSubscription(id: string, data?: SubscriptionManualSearchRequest): Promise<{ data: SubscriptionManualSearchResult }> {
  return request({
    url: `/admin/subscriptions/${encodeURIComponent(id)}/manual-search`,
    method: 'post',
    data: data ?? {}
  })
}

export function manualDispatchSubscription(id: string, data: SubscriptionManualDispatchRequest): Promise<{ data: SubscriptionManualDispatchResult }> {
  return request({
    url: `/admin/subscriptions/${encodeURIComponent(id)}/manual-dispatch`,
    method: 'post',
    data
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

// ==================== 115 账号控制面 ====================

/** 获取管理员维护的 115 账号安全摘要，响应不包含 Cookie 字段。 */
export function getP115Accounts(): Promise<P115AccountListResponse> {
  return request({
    url: '/admin/p115-accounts',
    method: 'get'
  })
}

/** 获取单个 115 账号安全摘要，响应不包含 Cookie 字段。 */
export function getP115Account(id: string): Promise<P115Account> {
  return request({
    url: `/admin/p115-accounts/${encodeURIComponent(id)}`,
    method: 'get'
  })
}

/** 创建 115 账号；Cookie 仅随本次写请求发送。 */
export function createP115Account(data: CreateP115AccountRequest): Promise<P115Account> {
  return request({
    url: '/admin/p115-accounts',
    method: 'post',
    data
  })
}

/** 覆盖 115 Cookie，并让账号回到待验证和停用状态。 */
export function replaceP115AccountCookie(id: string, data: ReplaceP115AccountCookieRequest): Promise<P115Account> {
  return request({
    url: `/admin/p115-accounts/${encodeURIComponent(id)}/cookie`,
    method: 'put',
    data
  })
}

/** 更新 source 账号的 Emby 挂载前缀和 115 源目录 ID。 */
export function updateP115AccountSourceLocation(id: string, data: UpdateP115SourceLocationRequest): Promise<P115Account> {
  return request({
    url: `/admin/p115-accounts/${encodeURIComponent(id)}/source-location`,
    method: 'put',
    data
  })
}

/** 原子更新共享 playback 账号的目录路径和账号总并发上限。 */
export function updateP115AccountPlaybackConfig(id: string, data: UpdateP115PlaybackConfigRequest): Promise<P115Account> {
  return request({
    url: `/admin/p115-accounts/${encodeURIComponent(id)}/playback-config`,
    method: 'put',
    data
  })
}

/** 显式执行只读 Cookie 有效性验证。 */
export function validateP115Account(id: string): Promise<P115ValidationResult> {
  return request({
    url: `/admin/p115-accounts/${encodeURIComponent(id)}/validate`,
    method: 'post'
  })
}

/** 启用已验证账号或停用现有账号。 */
export function setP115AccountEnabled(id: string, enabled: boolean): Promise<P115Account> {
  return request({
    url: `/admin/p115-accounts/${encodeURIComponent(id)}/enabled`,
    method: 'put',
    data: { enabled }
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

export function getRankingLibraryAllowlist(): Promise<{ data: RankingLibraryAllowlistSettings }> {
  return request({
    url: '/admin/rankings/library-allowlist',
    method: 'get'
  })
}

export function updateRankingLibraryAllowlist(libraryIds: string[]): Promise<{ data: RankingLibraryAllowlistSettings }> {
  return request({
    url: '/admin/rankings/library-allowlist',
    method: 'put',
    data: { libraryIds }
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

export function logoutBlacklistedDevices(): Promise<{ successDeviceIds: string[]; failedDeviceIds: Array<{ deviceId: string; error: string }> }> {
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

// ==================== 管理员 Emby 绑定 ====================
export function getAdminEmbyUsers(params?: { query?: string; limit?: number }): Promise<AdminEmbyUserListResponse> {
  return request({
    url: '/admin/emby-users',
    method: 'get',
    params
  })
}

export function bindAdminEmbyAccount(data: AdminEmbyBindingRequest): Promise<AdminEmbyBindingResponse> {
  return request({
    url: '/admin/current/emby-binding',
    method: 'put',
    data
  })
}

export function unbindAdminEmbyAccount(): Promise<{ message: string }> {
  return request({
    url: '/admin/current/emby-binding',
    method: 'delete'
  })
}
