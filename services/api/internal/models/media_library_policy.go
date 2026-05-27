package models

import (
	"time"

	"gorm.io/gorm"
)

// PlanGroupMediaLibrary 记录套餐分组允许展示的 Emby 媒体库模板。
type PlanGroupMediaLibrary struct {
	ID           string    `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	PlanGroupKey string    `json:"planGroupKey" gorm:"column:plan_group_key;type:varchar(50);not null;index"`
	LibraryID    string    `json:"libraryId" gorm:"column:library_id;type:varchar(100);not null"`
	LibraryName  string    `json:"libraryName" gorm:"column:library_name;type:varchar(255);not null"`
	LibraryType  string    `json:"libraryType" gorm:"column:library_type;type:varchar(50);not null"`
	SortOrder    int       `json:"sortOrder" gorm:"column:sort_order;not null;default:0"`
	CreatedAt    time.Time `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
}

func (PlanGroupMediaLibrary) TableName() string {
	return "plan_group_media_libraries"
}

func (p *PlanGroupMediaLibrary) BeforeCreate(tx *gorm.DB) error {
	if p.ID == "" {
		p.ID = generateCUID()
	}
	return nil
}

// PlanGroupEmbyPolicyTemplate 记录套餐分组托管的 Emby Policy 字段。
type PlanGroupEmbyPolicyTemplate struct {
	PlanGroupKey                   string    `json:"planGroupKey" gorm:"column:plan_group_key;type:varchar(50);primaryKey"`
	SimultaneousStreamLimit        int       `json:"simultaneousStreamLimit" gorm:"column:simultaneous_stream_limit;not null;default:3"`
	EnableContentDownloading       bool      `json:"enableContentDownloading" gorm:"column:enable_content_downloading;not null;default:false"`
	EnableLiveTvAccess             bool      `json:"enableLiveTvAccess" gorm:"column:enable_live_tv_access;not null;default:false"`
	EnableSyncTranscoding          bool      `json:"enableSyncTranscoding" gorm:"column:enable_sync_transcoding;not null;default:false"`
	EnableAudioPlaybackTranscoding bool      `json:"enableAudioPlaybackTranscoding" gorm:"column:enable_audio_playback_transcoding;not null;default:false"`
	EnableVideoPlaybackTranscoding bool      `json:"enableVideoPlaybackTranscoding" gorm:"column:enable_video_playback_transcoding;not null;default:false"`
	EnablePlaybackRemuxing         bool      `json:"enablePlaybackRemuxing" gorm:"column:enable_playback_remuxing;not null;default:true"`
	EnableRemoteAccess             bool      `json:"enableRemoteAccess" gorm:"column:enable_remote_access;not null;default:true"`
	CreatedAt                      time.Time `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt                      time.Time `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
}

func (PlanGroupEmbyPolicyTemplate) TableName() string {
	return "plan_group_emby_policy_templates"
}

// UserMediaLibraryPreference 记录用户自定义媒体库显示偏好的完整快照。
type UserMediaLibraryPreference struct {
	ID        string    `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	UserID    string    `json:"userId" gorm:"column:user_id;type:varchar(25);not null;index"`
	LibraryID string    `json:"libraryId" gorm:"column:library_id;type:varchar(100);not null"`
	Enabled   bool      `json:"enabled" gorm:"column:enabled;not null;default:true"`
	CreatedAt time.Time `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
}

func (UserMediaLibraryPreference) TableName() string {
	return "user_media_library_preferences"
}

func (p *UserMediaLibraryPreference) BeforeCreate(tx *gorm.DB) error {
	if p.ID == "" {
		p.ID = generateCUID()
	}
	return nil
}

// EmbyPolicySyncBatch 记录一次分组级 Emby Policy 同步批次。
type EmbyPolicySyncBatch struct {
	ID              string     `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	PlanGroupKey    string     `json:"planGroupKey" gorm:"column:plan_group_key;type:varchar(50);not null;index"`
	Reason          string     `json:"reason" gorm:"column:reason;type:varchar(100);not null"`
	Status          string     `json:"status" gorm:"column:status;type:varchar(20);not null;default:pending"`
	TotalCount      int        `json:"totalCount" gorm:"column:total_count;not null;default:0"`
	PendingCount    int        `json:"pendingCount" gorm:"column:pending_count;not null;default:0"`
	ProcessingCount int        `json:"processingCount" gorm:"column:processing_count;not null;default:0"`
	SyncedCount     int        `json:"syncedCount" gorm:"column:synced_count;not null;default:0"`
	FailedCount     int        `json:"failedCount" gorm:"column:failed_count;not null;default:0"`
	CreatedBy       *string    `json:"createdBy,omitempty" gorm:"column:created_by;type:varchar(25)"`
	CreatedAt       time.Time  `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time  `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
	FinishedAt      *time.Time `json:"finishedAt,omitempty" gorm:"column:finished_at"`
}

func (EmbyPolicySyncBatch) TableName() string {
	return "emby_policy_sync_batches"
}

func (b *EmbyPolicySyncBatch) BeforeCreate(tx *gorm.DB) error {
	if b.ID == "" {
		b.ID = generateCUID()
	}
	return nil
}

// EmbyPolicySyncTask 记录单个用户的 Emby Policy 同步任务。
type EmbyPolicySyncTask struct {
	ID           string     `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	BatchID      *string    `json:"batchId,omitempty" gorm:"column:batch_id;type:varchar(25);index"`
	UserID       string     `json:"userId" gorm:"column:user_id;type:varchar(25);not null;index"`
	EmbyID       string     `json:"embyId" gorm:"column:emby_id;type:varchar(50);not null"`
	PlanGroupKey string     `json:"planGroupKey" gorm:"column:plan_group_key;type:varchar(50);not null;index"`
	Reason       string     `json:"reason" gorm:"column:reason;type:varchar(100);not null"`
	Status       string     `json:"status" gorm:"column:status;type:varchar(20);not null;default:pending"`
	Attempts     int        `json:"attempts" gorm:"column:attempts;not null;default:0"`
	LastError    *string    `json:"lastError,omitempty" gorm:"column:last_error;type:varchar(500)"`
	NextRetryAt  *time.Time `json:"nextRetryAt,omitempty" gorm:"column:next_retry_at"`
	CreatedAt    time.Time  `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time  `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
}

func (EmbyPolicySyncTask) TableName() string {
	return "emby_policy_sync_tasks"
}

func (t *EmbyPolicySyncTask) BeforeCreate(tx *gorm.DB) error {
	if t.ID == "" {
		t.ID = generateCUID()
	}
	return nil
}
