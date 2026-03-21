import request from './request'
import type {
  CheckoutResponse,
  ConsoleAccountLink,
  CreateSubscriptionRequest,
  EmbyConfigResponse,
  LatestMediaResponse,
  MediaStatsResponse,
  PaymentListResponse,
  RankingResponse,
  Plan,
  RankingHistoryResponse,
  RankingPeriod,
  Subscription,
  SubscriptionListQuery,
  TVCalendarItem,
  TVCalendarStatus,
  TVCalendarWeeklyData,
  TVCalendarWeekOffset,
  TelegramBindCodeResponse,
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

export function getConsoleAccountLinks(): Promise<{ data: ConsoleAccountLink[] }> {
  return request({
    url: '/account-links',
    method: 'get'
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

// ==================== 最近入库 ====================
export function getLatestMedia(type: 'Movie' | 'Series', limit: number = 20): Promise<LatestMediaResponse> {
  return request({
    url: '/media/latest',
    method: 'get',
    params: { type, limit }
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
    url: '/plans',
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
