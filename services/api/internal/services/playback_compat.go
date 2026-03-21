package services

import playbackpkg "github.com/konghang/ember/backend/internal/services/playback"

type PlaybackHistoryService = playbackpkg.PlaybackHistoryService
type PlaybackHistoryRequest = playbackpkg.PlaybackHistoryRequest
type PlaybackHistoryItem = playbackpkg.PlaybackHistoryItem
type PlaybackHistoryResponse = playbackpkg.PlaybackHistoryResponse
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
)

func NewPlaybackHistoryService() *PlaybackHistoryService {
	return playbackpkg.NewPlaybackHistoryService()
}

func NewPlaybackRankingService() *PlaybackRankingService {
	return playbackpkg.NewPlaybackRankingService()
}
