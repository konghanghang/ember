import { request } from './request'
import type {
  CheckExistingSubscriptionRequest,
  CheckExistingSubscriptionResponse,
  CheckoutResponse,
  ConsoleAccountLink,
  CreateSubscriptionResponse,
  CreateSubscriptionRequest,
  EmbyConfigResponse,
  LatestMediaResponse,
  MediaStatsResponse,
  PaymentListResponse,
  PersonalP115Account,
  PersonalP115ValidationResult,
  PersonalP115Usage,
  PlaybackProfileQuery,
  PlaybackProfileResponse,
  RankingResponse,
  Plan,
  RankingHistoryResponse,
  RankingPeriod,
  ResubmitSubscriptionRequest,
  ResubmitSubscriptionResponse,
  Subscription,
  SubscriptionListQuery,
  TVCalendarItem,
  TVCalendarStatus,
  TVCalendarWeeklyData,
  TVCalendarWeekOffset,
  TelegramBindCodeResponse,
  UserMediaLibrarySettings,
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

export function checkExistingSubscription(data: CheckExistingSubscriptionRequest): Promise<CheckExistingSubscriptionResponse> {
  return request({
    url: '/subscriptions/check-existing',
    method: 'post',
    data
  })
}

export function createSubscription(data: CreateSubscriptionRequest): Promise<CreateSubscriptionResponse> {
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

export function resubmitSubscription(id: string, data: ResubmitSubscriptionRequest): Promise<ResubmitSubscriptionResponse> {
  return request({
    url: `/subscriptions/${id}/resubmit`,
    method: 'post',
    data
  })
}

// ==================== 个人信息 ====================
export function getProfile(): Promise<UserInfo> {
  return request({
    url: '/profile',
    method: 'get'
  })
}

export function updatePassword(data: { oldPassword: string; newPassword: string }) {
  return request({
    url: '/password',
    method: 'put',
    data
  })
}

export function sendEmailChangeCode(newEmail: string): Promise<{ message: string }> {
  return request({
    url: '/email/send-code',
    method: 'post',
    data: { newEmail }
  })
}

export function updateEmail(newEmail: string, code: string) {
  return request({
    url: '/email',
    method: 'put',
    data: { newEmail, code }
  })
}

export function getConsoleAccountLinks(): Promise<{ data: ConsoleAccountLink[] }> {
  return request({
    url: '/account-links',
    method: 'get'
  })
}

export function getProfileAnalytics(params?: PlaybackProfileQuery): Promise<PlaybackProfileResponse> {
  return request({
    url: '/profile/analytics',
    method: 'get',
    params
  })
}

export function generateTelegramBindCode(): Promise<TelegramBindCodeResponse> {
  return request({
    url: '/telegram/bindcode',
    method: 'post'
  })
}

export function unbindTelegram(): Promise<{ message: string }> {
  return request({
    url: '/telegram/unbind',
    method: 'delete'
  })
}

export function getUserMediaLibraries(): Promise<{ data: UserMediaLibrarySettings }> {
  return request({
    url: '/user/media-libraries',
    method: 'get',
    silent: true
  })
}

export function updateUserMediaLibraries(enabledLibraryIds: string[]): Promise<{ data: UserMediaLibrarySettings }> {
  return request({
    url: '/user/media-libraries',
    method: 'put',
    data: { enabledLibraryIds }
  })
}

export function resetUserMediaLibraryPreferences(): Promise<{ data: UserMediaLibrarySettings }> {
  return request({
    url: '/user/media-libraries/preferences',
    method: 'delete'
  })
}

export function applyCurrentUserMediaLibraryPolicySync(): Promise<{ data: UserMediaLibrarySettings }> {
  return request({
    url: '/user/emby-policy-sync/apply-current',
    method: 'post'
  })
}

// ==================== 个人 115 账号 ====================
/** 获取当前用户的个人 115 账号；404 由页面转为空状态。 */
export function getPersonalP115Account(): Promise<PersonalP115Account> {
  return request({
    url: '/user/p115-account',
    method: 'get',
    silent: true
  })
}

/** 获取当前用户跨账号归因的播放与转存配额用量。 */
export function getPersonalP115Usage(): Promise<PersonalP115Usage> {
  return request({
    url: '/user/p115-usage',
    method: 'get',
    silent: true
  })
}

/** 只提交 write-only Cookie 创建当前用户的个人 playback 账号。 */
export function createPersonalP115Account(cookie: string): Promise<PersonalP115Account> {
  return request({
    url: '/user/p115-account',
    method: 'post',
    data: { cookie }
  })
}

/** 替换 write-only Cookie 并重置 Provider 派生状态。 */
export function replacePersonalP115Cookie(cookie: string): Promise<PersonalP115Account> {
  return request({
    url: '/user/p115-account/cookie',
    method: 'put',
    data: { cookie }
  })
}

/** 解析并保存已经存在的个人 playback 目标目录。 */
export function updatePersonalP115Directory(targetParentPath: string): Promise<PersonalP115Account> {
  return request({
    url: '/user/p115-account/directory',
    method: 'put',
    data: { targetParentPath }
  })
}

/** 保存受当前套餐模板约束的个人账号并发上限。 */
export function updatePersonalP115Concurrency(maxConcurrentStreams: number): Promise<PersonalP115Account> {
  return request({
    url: '/user/p115-account/concurrency',
    method: 'put',
    data: { maxConcurrentStreams }
  })
}

/** 显式验证当前用户已经保存的 Cookie。 */
export function validatePersonalP115Account(): Promise<PersonalP115ValidationResult> {
  return request({
    url: '/user/p115-account/validate',
    method: 'post'
  })
}

/** 启用完整账号或停用当前个人账号。 */
export function setPersonalP115Enabled(enabled: boolean): Promise<PersonalP115Account> {
  return request({
    url: '/user/p115-account/enabled',
    method: 'put',
    data: { enabled }
  })
}

/** 不可逆擦除当前个人账号凭证并保留历史 tombstone。 */
export function revokePersonalP115Account(): Promise<{ message: string }> {
  return request({
    url: '/user/p115-account',
    method: 'delete'
  })
}

// ==================== 媒体信息 ====================
// 协议：emby 未配置 / 用户未绑定 Emby 时后端返回 200 + configured/bound 标志位，
// 前端按业务标志渲染空态。这些是 dashboard 首屏初始化探测，统一走 silent，
// 避免初次启动叠加触发 toast 风暴（参见 docs/reference/api-response-standard.md）。
export function getEmbyConfig(): Promise<EmbyConfigResponse> {
  return request({
    url: '/emby/config',
    method: 'get',
    silent: true
  })
}

export function getMediaStats(): Promise<MediaStatsResponse> {
  return request({
    url: '/media/stats',
    method: 'get',
    silent: true
  })
}

// ==================== 最近入库 ====================
export function getLatestMedia(type: 'Movie' | 'Series', limit: number = 20): Promise<LatestMediaResponse> {
  return request({
    url: '/media/latest',
    method: 'get',
    params: { type, limit },
    silent: true
  })
}

export function getMediaPoster(itemId: string, type: 'Movie' | 'Series'): Promise<Blob> {
  return request<Blob>({
    url: `/media/posters/${encodeURIComponent(itemId)}`,
    method: 'get',
    params: {
      type,
      maxHeight: 400,
      quality: 90
    },
    responseType: 'blob',
    silent: true
  })
}

// ==================== 播放排行 ====================
export function getLatestRanking(
  period: RankingPeriod
): Promise<RankingResponse> {
  return request({
    url: '/rankings/latest',
    method: 'get',
    params: { period }
  })
}

export function getRankingHistory(period: RankingPeriod, date: string): Promise<RankingHistoryResponse> {
  return request({
    url: '/rankings/history',
    method: 'get',
    params: { period, date }
  })
}

// ==================== 支付 ====================
export function getActivePlans(): Promise<{ data: Plan[] }> {
  return request({
    url: '/payments/plans',
    method: 'get'
  })
}

export function createCheckout(planId: string): Promise<CheckoutResponse> {
  return request({
    url: '/payments/checkout',
    method: 'post',
    data: { planId }
  })
}

export function getMyPayments(params?: { page?: number; pageSize?: number }): Promise<PaymentListResponse> {
  return request({
    url: '/payments',
    method: 'get',
    params
  })
}

// ==================== 追剧日历 ====================
export function getTVCalendar(params?: {
  startDate?: string
  endDate?: string
  status?: TVCalendarStatus | ''
}): Promise<{ data: TVCalendarItem[] }> {
  return request({
    url: '/tv-calendar',
    method: 'get',
    params
  })
}

export function getGlobalTVCalendar(params?: {
  weekDate?: string
  weekOffset?: TVCalendarWeekOffset
  status?: TVCalendarStatus | ''
}): Promise<{ data: TVCalendarWeeklyData }> {
  return request({
    url: '/tv-calendar/global',
    method: 'get',
    params
  })
}
