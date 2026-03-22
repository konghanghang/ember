package services

import playbackpkg "github.com/konghang/ember/backend/internal/services/playback"

type PlaybackHistoryService = playbackpkg.PlaybackHistoryService
type PlaybackHistoryRequest = playbackpkg.PlaybackHistoryRequest
type PlaybackHistoryItem = playbackpkg.PlaybackHistoryItem
type PlaybackHistoryResponse = playbackpkg.PlaybackHistoryResponse
type PlaybackProfileQuery = playbackpkg.PlaybackProfileQuery
type PlaybackProfileListQuery = playbackpkg.PlaybackProfileListQuery
type PlaybackProfileHourlyBucket = playbackpkg.PlaybackProfileHourlyBucket
type PlaybackProfileDeviceBucket = playbackpkg.PlaybackProfileDeviceBucket
type PlaybackProfileClientBucket = playbackpkg.PlaybackProfileClientBucket
type PlaybackProfileBadge = playbackpkg.PlaybackProfileBadge
type UserPlaybackProfile = playbackpkg.UserPlaybackProfile
type UserPlaybackProfileResponse = playbackpkg.UserPlaybackProfileResponse
type PlaybackProfileListItem = playbackpkg.PlaybackProfileListItem
type PlaybackProfileListSummary = playbackpkg.PlaybackProfileListSummary
type PlaybackProfileListResponse = playbackpkg.PlaybackProfileListResponse
type UserPlaybackProfileService = playbackpkg.UserPlaybackProfileService
type PlaybackRankingService = playbackpkg.PlaybackRankingService
type RankingComputeResult = playbackpkg.RankingComputeResult
type RankingResult = playbackpkg.RankingResult
type RankingResultItem = playbackpkg.RankingResultItem

var (
	ErrPlaybackHistoryInvalidDate    = playbackpkg.ErrInvalidDate
	ErrPlaybackHistoryInvalidKeyword = playbackpkg.ErrInvalidKeyword
	ErrPlaybackHistoryInvalidUserID  = playbackpkg.ErrInvalidUserID
	ErrPlaybackHistoryUserNotFound   = playbackpkg.ErrUserNotFound
	ErrPlaybackHistoryQueryFailed    = playbackpkg.ErrQueryFailed
	ErrPlaybackProfileInvalidDate    = playbackpkg.ErrPlaybackProfileInvalidDate
	ErrPlaybackProfileInvalidRange   = playbackpkg.ErrPlaybackProfileInvalidRange
	ErrPlaybackProfileRangeTooLarge  = playbackpkg.ErrPlaybackProfileRangeTooLarge
)

func NewPlaybackHistoryService() *PlaybackHistoryService {
	return playbackpkg.NewPlaybackHistoryService()
}

func NewUserPlaybackProfileService() *UserPlaybackProfileService {
	return playbackpkg.NewUserPlaybackProfileService()
}

func NewPlaybackRankingService() *PlaybackRankingService {
	return playbackpkg.NewPlaybackRankingService()
}
