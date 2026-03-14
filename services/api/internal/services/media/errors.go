package media

import "errors"

var (
	ErrMediaQualityLibraryIDRequired = errors.New("libraryId 不能为空")
	ErrMediaQualityGroupIDRequired   = errors.New("groupId 不能为空")
	ErrMediaQualityScanFailed        = errors.New("媒体库质量扫描失败")
)
