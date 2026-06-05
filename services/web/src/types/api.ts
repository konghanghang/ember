export type UserRole = 'admin' | 'user'
export type PlanGroup = string
export type MediaType = 'MOVIE' | 'TV'
export type SubscriptionStatus = 'PENDING' | 'APPROVED' | 'REJECTED' | 'INGESTED' | 'EXPIRED'

export interface ManagedPlanGroup {
  key: string
  name: string
  description?: string
  isDefault: boolean
  sortOrder: number
  planCount?: number
  userCount?: number
  followingUserCount?: number
  mediaLibraryCount?: number
  embyPolicyTemplateConfigured?: boolean
  policySyncStatus?: EmbyPolicySyncStatus
}

export interface MediaLibraryOption {
  id: string
  name: string
  type: string
  itemCount?: number
}

export type EmbyPolicySyncStatus = 'pending' | 'processing' | 'synced' | 'partial_failed' | 'failed'

export interface EmbyPolicySyncBatchCreated {
  batchId: string
  affectedUserCount: number
  status: EmbyPolicySyncStatus
}

export interface EmbyPolicySyncFailedUser {
  userId: string
  username?: string
  embyId?: string
  error: string
}

export interface EmbyPolicySyncBatchDetail {
  id: string
  planGroupKey?: string
  reason: string
  status: EmbyPolicySyncStatus
  totalCount: number
  pendingCount: number
  processingCount: number
  syncedCount: number
  failedCount: number
  failedUsers?: EmbyPolicySyncFailedUser[]
  createdAt: string
  updatedAt: string
  finishedAt?: string | null
}

export interface PlanGroupMediaLibrarySettings {
  planGroupKey: PlanGroup
  planGroupName: string
  libraries: MediaLibraryOption[]
  libraryCount: number
  affectedUserCount: number
}

export interface PlanGroupEmbyPolicyTemplate {
  planGroupKey: PlanGroup
  planGroupName: string
  simultaneousStreamLimit: number
  enableContentDownloading: boolean
  enableLiveTvAccess: boolean
  enableSyncTranscoding: boolean
  enableAudioPlaybackTranscoding: boolean
  enableVideoPlaybackTranscoding: boolean
  enablePlaybackRemuxing: boolean
  enableRemoteAccess: boolean
  affectedUserCount: number
}

export interface PlanGroupEmbyPolicyTemplateUpdateRequest {
  simultaneousStreamLimit: number
  enableContentDownloading: boolean
  enableLiveTvAccess: boolean
  enableSyncTranscoding: boolean
  enableAudioPlaybackTranscoding: boolean
  enableVideoPlaybackTranscoding: boolean
  enablePlaybackRemuxing: boolean
  enableRemoteAccess: boolean
}

export interface UserMediaLibraryItem {
  id: string
  name: string
  type: string
  itemCount?: number
  inGroupTemplate: boolean
  enabled: boolean
}

export interface UserMediaLibrarySettings {
  userId: string
  embyId?: string
  planGroup: PlanGroup
  planGroupName: string
  customized: boolean
  libraries: UserMediaLibraryItem[]
  templateCount: number
  enabledCount: number
  policySyncStatus: EmbyPolicySyncStatus
  pendingSyncTaskId?: string
}

export interface MediaLibrarySyncPreviewResult {
  planGroupKey: PlanGroup
  totalUsers: number
  scannedUsers: number
  consistent: boolean
  candidates: MediaLibrarySyncCandidate[]
  differenceUsers: MediaLibrarySyncDifferenceUser[]
  failedItems: MediaLibrarySyncFailedItem[]
}

export interface MediaLibrarySyncCandidate {
  libraryIds: string[]
  libraries: MediaLibraryOption[]
  userCount: number
  sourceUserIds: string[]
}

export interface MediaLibrarySyncDifferenceUser {
  userId: string
  username: string
  embyId: string
  libraryIds: string[]
  libraries: MediaLibraryOption[]
}

export interface MediaLibrarySyncFailedItem {
  userId?: string
  username?: string
  embyId?: string
  error: string
}

export interface MediaLibrarySyncApplyRequest {
  libraryIds: string[]
  preferenceUserIds?: string[]
}

export interface MediaLibrarySyncApplyResult {
  batchId: string
  affectedUserCount: number
  status: EmbyPolicySyncStatus
  failedItems?: MediaLibrarySyncFailedItem[]
}

export interface LoginCredentials {
  username: string
  password: string
  turnstileToken?: string
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
  embyAccessDisabled?: boolean
  embyDisabled?: boolean
  telegramId?: number
  planGroup?: PlanGroup | null
  planGroupName?: string | null
  effectivePlanGroup?: PlanGroup
  effectivePlanGroupName?: string
  isPlanGroupMissing?: boolean
  isUsingDefaultPlanGroup?: boolean
  isExpired?: boolean
  mediaLibraryPreferenceCustomized?: boolean
  mediaLibraryTemplateCount?: number
  mediaLibraryEnabledCount?: number
  policySyncStatus?: EmbyPolicySyncStatus
  policySyncBatchStatus?: EmbyPolicySyncStatus
  policySyncBatchId?: string
  expiresAt?: string
  isActive: boolean
  passwordResetRequired?: boolean
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

export interface LoginProtectionConfig {
  turnstileLoginEnabled: boolean
  turnstileSiteKey: string
  turnstileExpectedHostname: string
}

export interface RegisterResponse {
  token: string
  user: UserInfo
  policySyncStatus?: EmbyPolicySyncStatus
}

export interface PaginationQuery {
  page?: number
  pageSize?: number
}

export interface UserListQuery extends PaginationQuery {
  search?: string
  expiresAfter?: string
  embyStatus?: 'available' | 'disabled' | 'unlinked' | ''
  planGroup?: PlanGroup | ''
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
  planGroup?: PlanGroup
  expiresAt?: string
  clearExpiresAt?: boolean
}

export interface CreateAdminUserRequest {
  username: string
  email: string
  password: string
  planGroup: PlanGroup
  neverExpire?: boolean
  expiresAt?: string
}

export interface RedemptionCode {
  id: string
  code: string
  maxUses: number
  usedCount: number
  defaultDays: number
  registrationPlanGroup: PlanGroup
  registrationPlanGroupName?: string | null
  expiresAt?: string | null
  createdAt: string
  notes?: string
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
  registrationPlanGroup?: PlanGroup | ''
}

export interface CreateRedemptionCodeRequest {
  maxUses: number
  defaultDays: number
  registrationPlanGroup: PlanGroup
  expiresAt?: string | null
  notes?: string
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
  registrationPlanGroup: PlanGroup
  expiresAt?: string | null
  notes?: string
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

export type PlaybackProfileRange = 'today' | '7d' | '30d' | '90d' | 'all' | 'custom'

export interface PlaybackProfileQuery {
  range?: PlaybackProfileRange
  startDate?: string
  endDate?: string
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
  startDate?: string
  endDate?: string
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

export interface MediaQualityFailedLibrary {
  libraryId: string
  libraryName: string
  error: string
}

export interface MediaQualityReport {
  resolutionDistribution: MediaQualityResolutionItem[]
  codecDistribution: MediaQualityCodecItem[]
  hdrDistribution: MediaQualityHDRItem[]
  lowQualityItems: MediaQualityLowItem[]
  failedLibraries?: MediaQualityFailedLibrary[]
  lowQualityTotal: number
  page: number
  pageSize: number
  scanAt: string
}

export type MediaGapStatus = 'MISSING' | 'SEARCHED' | 'REQUESTED' | 'INGESTED' | 'IGNORED' | 'DISPATCH_FAILED'
export type MediaGapIgnoreReasonCode = 'manual' | 'season_not_activated'

export interface MediaGapListQuery extends PaginationQuery {
  keyword?: string
  status?: MediaGapStatus | ''
  airDateFrom?: string
  airDateTo?: string
}

export interface MediaGapSearchCandidate {
  id: string
  title: string
  subtitle?: string
  source?: string
  site?: string
  size?: string
  seeders?: number
  publishDate?: string
  language?: string
  releaseGroup?: string
  episodeRange?: string
  matchReason?: string
  description?: string
  payload?: Record<string, unknown>
}

export interface MediaGapSearchSnapshot {
  candidates: MediaGapSearchCandidate[]
  searchedAt?: string
  source?: string
  query?: string
}

export type MediaGapGroupedSortMode = 'missing' | 'updated' | 'requested' | 'name'

export interface MediaGapGroupedQuery extends PaginationQuery {
  keyword?: string
  status?: MediaGapStatus | ''
  airDateFrom?: string
  airDateTo?: string
  sort?: MediaGapGroupedSortMode
}

export interface MediaGapDispatchSnapshot {
  candidateId?: string
  title?: string
  source?: string
  site?: string
  requestedAt?: string
  status?: string
}

export interface MediaGapItem {
  id: string
  tmdbId?: string
  embySeriesId?: string
  seriesName: string
  season: number
  episode: number
  airDate?: string
  status: MediaGapStatus
  searchSnapshot?: MediaGapSearchSnapshot | string | null
  dispatchSnapshot?: MediaGapDispatchSnapshot | string | null
  lastDispatchError?: string | null
  lastScannedAt?: string
  lastSearchedAt?: string
  requestedAt?: string
  ingestedAt?: string
  ignoredAt?: string
  ignoreReasonCode?: MediaGapIgnoreReasonCode
  ignoreReason?: string
  createdAt: string
  updatedAt: string
}

export interface MediaGapListResponse {
  data: MediaGapItem[]
  total: number
  page: number
  pageSize: number
}

export interface MediaGapGroupedSeason {
  season: number
  gaps: MediaGapItem[]
}

export interface MediaGapGroupedSeries {
  key: string
  seriesName: string
  tmdbId?: string
  embySeriesId?: string
  gaps: MediaGapItem[]
  seasons: MediaGapGroupedSeason[]
  totalGaps: number
  missingCount: number
  searchedCount: number
  requestedCount: number
  dispatchFailedCount?: number
  ingestedCount: number
  ignoredCount: number
  latestUpdatedAt?: string
}

export interface MediaGapGroupedSummary {
  missingCount: number
  searchedCount: number
  requestedCount: number
  dispatchFailedCount?: number
  ingestedCount: number
  ignoredCount: number
}

export interface MediaGapGroupedResponse {
  data: MediaGapGroupedSeries[]
  total: number
  itemTotal: number
  page: number
  pageSize: number
  summary: MediaGapGroupedSummary
}

export interface MediaGapSearchResult {
  mediaGap?: MediaGapItem
  candidates: MediaGapSearchCandidate[]
  searchedAt?: string
  source?: string
}

export interface MediaGapDispatchRequest {
  candidateId: string
  candidate?: MediaGapSearchCandidate
  candidatePayload?: Record<string, unknown>
}

export interface MediaGapIgnoreRequest {
  reason?: string
}

export interface MediaGapActionResponse {
  mediaGap?: MediaGapItem
  message?: string
}

export interface MediaGapScanRequest {
  tmdbId?: string
  force?: boolean
}

export type MediaGapScanState = 'idle' | 'running' | 'succeeded' | 'failed'

export interface MediaGapScanStatus {
  scanId?: string
  scope?: 'all' | 'series'
  status: MediaGapScanState
  running: boolean
  startedAt?: string
  finishedAt?: string
  count?: number
  scannedSeries?: number
  skippedSeries?: number
  examinedEpisodes?: number
  created?: number
  updated?: number
  ingested?: number
  error?: string
  message?: string
}

export interface MediaGapScanResponse {
  started?: boolean
  async?: boolean
  scope?: 'all' | 'series'
  scanId?: string
  status?: MediaGapScanState
  running?: boolean
  count?: number
  message?: string
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
  allowedEmailDomains?: string[]
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
  maskedValue?: string
  value?: string
  error?: string
}

export interface AdminConfigListResponse {
  data: AdminConfigItem[]
}

export interface AdminExternalApiKeyStatus {
  configured: boolean
}

export interface AdminExternalApiKeyCreated extends AdminExternalApiKeyStatus {
  apiKey: string
}

export interface AdminExternalApiKeyStatusResponse {
  data: AdminExternalApiKeyStatus
}

export interface AdminExternalApiKeyCreatedResponse {
  data: AdminExternalApiKeyCreated
}

export interface UpdateAdminConfigRequest {
  value?: string
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

export interface Subscription {
  id: string
  userId?: string
  retryFromId?: string | null
  type: MediaType
  name: string
  tmdbId: string
  season: number
  posterPath?: string
  status: SubscriptionStatus
  note?: string
  mpError?: string | null
  rejectReason?: string | null
  ingestProgress?: string | null
  reviewedAt?: string | null
  ingestedAt?: string | null
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
  season?: number
  posterPath?: string
  note?: string
  confirmExisting?: boolean
}

export type SubscriptionExistingMatchType = 'movie' | 'series' | 'season' | 'unknown'

export interface SubscriptionExistingSummary {
  matchType: SubscriptionExistingMatchType
  embyItemId?: string
  message: string
  availableSeasons?: number[]
  episodeCount?: number
  detectionFailed?: boolean
}

export interface CheckExistingSubscriptionRequest {
  type: MediaType
  tmdbId: string
  season?: number
}

export interface CheckExistingSubscriptionResponse {
  existsInLibrary: boolean
  detectionFailed?: boolean
  existingSummary?: SubscriptionExistingSummary
}

export interface CreateSubscriptionResponse {
  success: boolean
  confirmationRequired?: boolean
  detectionFailed?: boolean
  existingSummary?: SubscriptionExistingSummary
}

export interface ResubmitSubscriptionRequest {
  note: string
  confirmExisting?: boolean
}

export interface ResubmitSubscriptionResponse {
  success: boolean
  confirmationRequired?: boolean
  detectionFailed?: boolean
  existingSummary?: SubscriptionExistingSummary
  subscription?: Subscription
}

export interface TmdbTVSeasonOptions {
  id: number
  name: string
  numberOfSeasons: number
  seasons: number[]
}

export interface TmdbTVSeasonOptionsResponse {
  data: TmdbTVSeasonOptions
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
  planGroup: PlanGroup
  planGroupName?: string
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
  planGroup?: PlanGroup
  sortOrder?: number
}

export interface UpdatePlanRequest {
  name?: string
  description?: string
  days?: number
  price?: number
  currency?: string
  planGroup?: PlanGroup
  isActive?: boolean
  sortOrder?: number
}

export interface CreatePlanGroupRequest {
  key: string
  name: string
  description?: string
  isDefault?: boolean
  sortOrder?: number
}

export interface UpdatePlanGroupRequest {
  name?: string
  description?: string
  isDefault?: boolean
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
  configured?: boolean
}

export interface MediaStats {
  MovieCount: number
  SeriesCount: number
  EpisodeCount: number
}

export interface MediaStatsResponse {
  success: boolean
  data: MediaStats | null
  configured?: boolean
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
  configured?: boolean
  bound?: boolean
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

// ==================== 管理员 Emby 绑定 ====================
export interface AdminEmbyUserOption {
  embyId: string
  name: string
  hasPassword: boolean
  boundUsername?: string
  boundToCurrent: boolean
  available: boolean
}

export interface AdminEmbyUserListResponse {
  data: AdminEmbyUserOption[]
}

export interface AdminEmbyBindingRequest {
  embyId: string
}

export interface AdminEmbyBindingResponse {
  embyId: string
  embyUsername: string
}

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
