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
  emailCode?: string
  code?: string
}

export interface UserInfo {
  id: string
  username: string
  role: UserRole
  email?: string
  embyId?: string
  embyDisabled?: boolean
  telegramId?: number
  expiresAt?: string
  isActive: boolean
  createdAt: string
}

export type ConsoleAccountLinkIcon = 'notify' | 'group' | 'wiki'

export interface ConsoleAccountLink {
  key: string
  title: string
  description: string
  url: string
  icon: ConsoleAccountLinkIcon
  sortOrder: number
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
  expiresAfter?: string
  embyStatus?: 'available' | 'disabled' | 'unlinked' | ''
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
  templateUserId?: string | null
  templateUserName?: string | null
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

export type RedemptionCodeStatusFilter = 'active' | 'expired' | 'exhausted'

export interface RedemptionCodeListQuery extends PaginationQuery {
  showAll?: boolean
  code?: string
  status?: RedemptionCodeStatusFilter | ''
  templateUserId?: string
}

export interface CreateRedemptionCodeRequest {
  maxUses: number
  defaultDays: number
  templateUserId?: string | null
  expiresAt?: string | null
}

export interface CreateRedemptionCodesBatchRequest extends CreateRedemptionCodeRequest {
  count: number
}

export interface CreateRedemptionCodesBatchResponse {
  data: RedemptionCode[]
  count: number
}

export interface UpdateRedemptionCodeRequest {
  maxUses: number
  defaultDays: number
  templateUserId?: string | null
  expiresAt?: string | null
}

export interface UserTemplate {
  id: string
  username: string
  email?: string
  expiresAt?: string
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

export interface AdminRedemptionQuery extends PaginationQuery {
  userId?: string
  username?: string
  code?: string
}

export interface PlaybackHistoryQuery extends PaginationQuery {
  userId?: string
  username?: string
  keyword?: string
  startDate?: string
  endDate?: string
}

export interface PlaybackHistoryItem {
  userId: string
  username: string
  itemName: string
  itemType: string
  playedAt: string
  deviceName: string
  clientName: string
  playDuration: number
  playDurationFormatted: string
}

export interface PlaybackHistoryResponse {
  data: PlaybackHistoryItem[]
  total: number
  page: number
  pageSize: number
}

export type PlaybackProfileRange = '7d' | '30d' | '90d' | 'all'

export interface PlaybackProfileQuery {
  range?: PlaybackProfileRange
}

export interface PlaybackProfileHourlyBucket {
  hour: number
  count: number
}

export interface PlaybackProfileDeviceBucket {
  deviceName: string
  count: number
  duration: number
  durationFormatted: string
}

export interface PlaybackProfileClientBucket {
  clientName: string
  count: number
  duration: number
  durationFormatted: string
}

export interface PlaybackProfileBadge {
  id: string
  name: string
  description: string
}

export interface UserPlaybackProfile {
  userId: string
  username: string
  range: PlaybackProfileRange
  totalPlayCount: number
  totalPlayDuration: number
  totalPlayDurationFormatted: string
  activeDays: number
  averagePlayDuration: number
  averagePlayDurationFormatted: string
  lastPlayedAt: string | null
  hourlyDistribution: PlaybackProfileHourlyBucket[]
  deviceDistribution: PlaybackProfileDeviceBucket[]
  clientDistribution: PlaybackProfileClientBucket[]
  badges: PlaybackProfileBadge[]
  recentRecords: PlaybackHistoryItem[]
}

export interface PlaybackProfileResponse {
  data: UserPlaybackProfile
}

export type PlaybackProfileListSortBy = 'totalDuration' | 'playCount' | 'activeDays' | 'lastPlayedAt'
export type PlaybackProfileListSortOrder = 'asc' | 'desc'

export interface PlaybackProfileListQuery extends PaginationQuery {
  range?: PlaybackProfileRange
  keyword?: string
  sortBy?: PlaybackProfileListSortBy
  sortOrder?: PlaybackProfileListSortOrder
}

export interface PlaybackProfileListItem {
  userId: string
  username: string
  range: PlaybackProfileRange
  totalPlayCount: number
  totalPlayDuration: number
  totalPlayDurationFormatted: string
  activeDays: number
  lastPlayedAt: string | null
  peakHour: number | null
  peakHourLabel: string
  badges: PlaybackProfileBadge[]
}

export interface PlaybackProfileListSummary {
  userCount: number
  totalPlayCount: number
  totalPlayDuration: number
  totalPlayDurationFormatted: string
}

export interface PlaybackProfileListResponse {
  data: PlaybackProfileListItem[]
  total: number
  page: number
  pageSize: number
  summary: PlaybackProfileListSummary
}

export interface MediaQualityLibrary {
  id: string
  name: string
  type: string
  itemCount: number
}

export interface MediaQualityResolutionItem {
  resolution: string
  count: number
}

export interface MediaQualityCodecItem {
  codec: string
  count: number
}

export interface MediaQualityHDRItem {
  type: string
  count: number
}

export interface MediaQualityLowItem {
  id: string
  groupId?: string
  name: string
  itemType: 'Movie' | 'Series'
  itemCount: number
  posterItemId: string
  resolution: string
  codec: string
  bitrate: number
}

export interface MediaQualityLowDetailItem {
  id: string
  groupId: string
  groupName: string
  itemType: string
  name: string
  posterItemId: string
  resolution: string
  codec: string
  bitrate: number
  season?: number
  episode?: number
}

export interface MediaQualityReport {
  resolutionDistribution: MediaQualityResolutionItem[]
  codecDistribution: MediaQualityCodecItem[]
  hdrDistribution: MediaQualityHDRItem[]
  lowQualityItems: MediaQualityLowItem[]
  lowQualityTotal: number
  page: number
  pageSize: number
  scanAt: string
}

export interface RedeemCodeRequest {
  code: string
}

export interface RedeemCodeResponse {
  message: string
  days: number
  expiresAt: string
}

export interface TelegramBindCodeResponse {
  code: string
  expiresAt: string
}

export interface RegistrationModeResponse {
  mode: 'open' | 'invite'
  defaultTrialDays?: number
  emailVerification?: boolean
}

export type ConfigValueType = 'string' | 'secret' | 'boolean' | 'integer' | 'url' | 'enum' | 'json_list'
export type ConfigSource = 'database' | 'env' | 'default' | 'unset'
export type ConfigEmptyValueMode = 'not_allowed' | 'disable' | 'fallback' | 'inherit'
export type ConfigRiskLevel = 'none' | 'info' | 'warning' | 'critical'

export interface ConfigOption {
  label: string
  value: string
}

export interface AdminConfigItem {
  key: string
  group: string
  groupLabel: string
  label: string
  description: string
  type: ConfigValueType
  placeholder?: string
  multiline: boolean
  editable: boolean
  sensitive: boolean
  restartRequired: boolean
  allowEmpty: boolean
  emptyValueMode: ConfigEmptyValueMode
  emptyValueHint?: string
  readOnlyHint?: string
  missingValueHint?: string
  missingValueLevel: ConfigRiskLevel
  fallbackHint?: string
  minValue?: number
  maxValue?: number
  options?: ConfigOption[]
  source: ConfigSource
  hasValue: boolean
  value?: string
  error?: string
}

export interface AdminConfigListResponse {
  data: AdminConfigItem[]
}

export interface UpdateAdminConfigRequest {
  value?: string
  clear?: boolean
}

export interface ConfigGroupTestDetail {
  target: string
  success: boolean
  message: string
}

export interface ConfigGroupTestResult {
  success: boolean
  message: string
  details: ConfigGroupTestDetail[]
}

export interface ImportEnvResult {
  imported: string[]
  skipped: Record<string, string>
  failed: Record<string, string>
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

export type TVCalendarStatus = 'ready' | 'missing' | 'upcoming' | 'today'
export type TVCalendarWeekOffset = -1 | 0 | 1

export interface TVCalendarItem {
  id: string
  tmdbId: string
  season: number
  episode: number
  airDate: string
  episodeName: string
  status: TVCalendarStatus
  embyItemId?: string
  showName: string
  posterUrl?: string
}

export interface TVCalendarWeeklyItem {
  tmdbId: string
  seriesId?: string
  showName: string
  posterUrl?: string
  season: number
  episode: string
  airDate: string
  status: TVCalendarStatus
  episodeName?: string
  overview?: string
}

export interface TVCalendarWeeklyDay {
  date: string
  weekdayCn: string
  isToday: boolean
  items: TVCalendarWeeklyItem[]
}

export interface TVCalendarWeeklyData {
  dateRange: string
  days: TVCalendarWeeklyDay[]
}

// ==================== 付费方案 ====================
export interface Plan {
  id: string
  name: string
  description: string
  days: number
  price: number
  currency: string
  isActive: boolean
  sortOrder: number
  createdAt: string
  updatedAt: string
}

export interface CreatePlanRequest {
  name: string
  description?: string
  days: number
  price: number
  currency?: string
  sortOrder?: number
}

export interface UpdatePlanRequest {
  name?: string
  description?: string
  days?: number
  price?: number
  currency?: string
  isActive?: boolean
  sortOrder?: number
}

export type PaymentStatus = 'pending' | 'completed' | 'expired' | 'failed'

export interface Payment {
  id: string
  userId: string
  planId: string
  stripeSessionId: string
  stripePaymentIntentId?: string
  amount: number
  currency: string
  days: number
  status: PaymentStatus
  expiresAt?: string
  createdAt: string
  updatedAt: string
  username?: string
  planName?: string
}

export interface PaymentListResponse {
  data: Payment[]
  total: number
  page: number
  pageSize: number
  totalPages: number
}

export interface AdminPaymentQuery extends PaginationQuery {
  userId?: string
  planId?: string
  status?: PaymentStatus | ''
}

export interface PlanListResponse {
  data: Plan[]
  total: number
  page: number
  pageSize: number
  totalPages: number
}

export interface CheckoutResponse {
  url: string
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

// ==================== 最近入库 ====================
export interface LatestMediaItem {
  id: string
  name: string
  type: 'Movie' | 'Series'
  productionYear: number
  dateCreated: string
  communityRating?: number
  officialRating?: string
  overview?: string
  childCount: number
}

export interface LatestMediaResponse {
  success: boolean
  data: LatestMediaItem[]
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

// ==================== 播放排行 ====================
export type RankingPeriod = 'daily' | 'weekly'
export type RankingCategory = 'media_movie' | 'media_episode'

export interface RankingItem {
  rank: number
  itemKey: string
  itemName: string
  playCount: number
  duration: number // 秒
}

export interface RankingResponse {
  period: RankingPeriod
  batchId?: string
  snapshotAt?: string
  periodStart: string
  periodEnd: string
  cutoffAt: string
  movies: RankingItem[]
  episodes: RankingItem[]
}

export type RankingPreviewResponse = RankingResponse
export type RankingHistoryResponse = RankingResponse

// ==================== 活跃会话 ====================
export interface ActiveNowPlayingItem {
  name: string
  id: string
  type: string // "Movie" | "Episode"
  mediaType: string
  runTimeTicks: number // ticks（÷10000000=秒）
  seriesName?: string // 仅 Episode
  indexNumber?: number // 集号，仅 Episode
  parentIndexNumber?: number // 季号，仅 Episode
  productionYear?: number
}

export interface ActivePlayState {
  positionTicks: number // ticks
  isPaused: boolean
  isMuted: boolean
  playMethod: string // "DirectPlay" | "DirectStream" | "Transcode"
}

export interface ActiveSession {
  id: string
  userId: string
  userName: string
  client: string
  deviceName: string
  deviceId: string
  remoteEndpoint: string
  applicationVersion: string
  lastActivityDate: string
  nowPlayingItem?: ActiveNowPlayingItem
  playState?: ActivePlayState
}

// ==================== 设备管理 ====================
export interface DeviceItem {
  deviceId: string
  deviceName: string
  clientName: string
  userId?: string
  userName?: string
  embyUserId?: string
  isActive: boolean
  isBlacklisted: boolean
  blacklistReason?: string
  lastActivityDate?: string
  applicationVersion?: string
  remoteEndpoint?: string
}

export interface DeviceListQuery extends PaginationQuery {
  userId?: string
  clientName?: string
  isBlacklisted?: boolean
}

export interface DeviceListResponse {
  data: DeviceItem[]
  total: number
  page: number
  pageSize: number
  totalPages: number
}

export interface ClientBlacklist {
  id: string
  clientName: string
  reason?: string
  createdAt: string
}

export interface DeviceAction {
  id: string
  deviceId?: string
  userId?: string
  clientName?: string
  action: string
  note?: string
  createdAt: string
}

export interface DeviceStats {
  clientDistribution: Array<{
    clientName: string
    count: number
  }>
  topDevices: Array<{
    deviceName: string
    count: number
  }>
  blacklistedClientCount: number
  activeSessionCount: number
}
