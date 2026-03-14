package services

import mediapkg "github.com/konghang/ember/backend/internal/services/media"

type MediaService = mediapkg.MediaService
type MediaQualityService = mediapkg.MediaQualityService
type ResolutionDistributionItem = mediapkg.ResolutionDistributionItem
type CodecDistributionItem = mediapkg.CodecDistributionItem
type HDRDistributionItem = mediapkg.HDRDistributionItem
type LowQualityItem = mediapkg.LowQualityItem
type LowQualityDetailItem = mediapkg.LowQualityDetailItem
type QualityReport = mediapkg.QualityReport

var (
	ErrMediaQualityLibraryIDRequired = mediapkg.ErrMediaQualityLibraryIDRequired
	ErrMediaQualityGroupIDRequired   = mediapkg.ErrMediaQualityGroupIDRequired
	ErrMediaQualityScanFailed        = mediapkg.ErrMediaQualityScanFailed
)

func NewMediaService() *MediaService {
	return mediapkg.NewMediaService()
}

func NewMediaQualityService() *MediaQualityService {
	return mediapkg.NewMediaQualityService()
}
