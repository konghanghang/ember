export type UserRole = 'admin' | 'user'
export type MediaType = 'MOVIE' | 'TV'
export type SubscriptionStatus = 'PENDING' | 'APPROVED' | 'REJECTED' | 'EXPIRED'

export interface LoginCredentials {
  username: string
  password: string
}

export interface RegisterRequest {
  username: string
  password: string
  email: string
  code?: string
}

export interface UserInfo {
  id: string
  username: string
  role: UserRole
  email?: string
  embyId?: string
  embyDisabled?: boolean
  expiresAt?: string
  isActive: boolean
  createdAt: string
}

export interface LoginResponse {
  token: string
  user: UserInfo
  isExpired?: boolean
}

export interface RegisterResponse {
  token: string
  user: UserInfo
}

export interface PaginationQuery {
  page?: number
  pageSize?: number
}

export interface UserListQuery extends PaginationQuery {
  search?: string
}

export interface UserListResponse {
  data: UserInfo[]
  total: number
  page: number
  pageSize: number
}

export interface UpdateAdminUserRequest {
  email?: string
  isActive?: boolean
  expiresAt?: string
  clearExpiresAt?: boolean
}

export interface RedemptionCode {
  id: string
  code: string
  maxUses: number
  usedCount: number
  defaultDays: number
  expiresAt?: string | null
  createdAt: string
}

export interface RedemptionCodeListResponse {
  data: RedemptionCode[]
  total: number
  page: number
  pageSize: number
  totalPages: number
}

export interface CreateRedemptionCodeRequest {
  maxUses: number
  defaultDays: number
  expiresAt?: string | null
}

export interface Redemption {
  id: string
  userId: string
  code: string
  days: number
  createdAt: string
  username?: string
}

export interface RedemptionListResponse {
  data: Redemption[]
  total: number
  page: number
  pageSize: number
  totalPages: number
}

export interface RedeemCodeRequest {
  code: string
}

export interface RedeemCodeResponse {
  message: string
  days: number
  expiresAt: string
}

export interface RegistrationModeResponse {
  mode: 'open' | 'invite'
  defaultTrialDays?: number
}

export interface Setting {
  key: string
  value: string
  updatedAt: string
}

export interface UpdateSettingRequest {
  value: string
}

export interface Subscription {
  id: string
  userId?: string
  type: MediaType
  name: string
  tmdbId: string
  posterPath?: string
  status: SubscriptionStatus
  note?: string
  mpError?: string | null
  createdAt: string
  user?: {
    username: string
    email: string
  }
}

export interface SubscriptionListQuery extends PaginationQuery {
  status?: SubscriptionStatus
}

export interface CreateSubscriptionRequest {
  type: MediaType
  name: string
  tmdbId: string
  posterPath?: string
  note?: string
}

export interface SystemInfoResponse {
  success: boolean
  info: {
    userCount: number
    activeUserCount: number
    redemptionCodeCount: number
  }
}

export interface EmbyConfigResponse {
  success: boolean
  url: string
}

export interface MediaStats {
  MovieCount: number
  SeriesCount: number
  EpisodeCount: number
}

export interface MediaStatsResponse {
  success: boolean
  data: MediaStats
}

export interface CronCheckResponse {
  success: boolean
  disabledCount: number
  totalExpired: number
  errors: string[]
}

export interface TmdbSearchItem {
  id: number
  title: string
  originalTitle?: string
  overview?: string
  posterPath?: string
  releaseDate?: string
  mediaType?: string
}

export interface TmdbSearchResponse {
  results: TmdbSearchItem[]
  total: number
}

export interface TmdbSelection {
  tmdbId: string
  name: string
  posterPath?: string
}

// AdminInfo 是 UserInfo 的类型别名
// 后端使用统一的 User 模型，通过 role 字段区分 admin 和 user
export type AdminInfo = UserInfo
