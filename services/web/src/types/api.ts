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
  inviteCode: string
}

export interface AdminInfo {
  id: string
  username: string
  createdAt: string
}

export interface UserInfo {
  id: string
  username: string
  email: string
  embyId: string
  expiresAt: string
  isActive: boolean
  createdAt: string
}

export interface AdminLoginResponse {
  token: string
  admin: AdminInfo
}

export interface UserLoginResponse {
  token: string
  user: UserInfo
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

export interface Invite {
  id: string
  code: string
  maxUses: number
  usedCount: number
  defaultDays: number
  expiresAt?: string | null
  createdAt: string
}

export interface InviteListResponse {
  invites: Invite[]
  total?: number
}

export interface ValidateInviteResponse {
  valid: boolean
  invite: Invite
}

export interface CreateInviteRequest {
  maxUses?: number
  defaultDays?: number
  expiresAt?: string | null
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
    inviteCount: number
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
