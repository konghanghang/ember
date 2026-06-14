package subscription

import (
	moviepilotint "github.com/konghang/ember/backend/internal/integrations/moviepilot"
	"github.com/konghang/ember/backend/internal/models"
)

// CreateSubscriptionRequest 创建订阅请求
type CreateSubscriptionRequest struct {
	Type            models.MediaType `json:"type" binding:"required,oneof=MOVIE TV"`
	Name            string           `json:"name" binding:"required"`
	TmdbID          string           `json:"tmdbId" binding:"required"`
	Season          int              `json:"season"`
	PosterPath      *string          `json:"posterPath"`
	Note            *string          `json:"note"`
	ConfirmExisting bool             `json:"confirmExisting"`
}

// CheckExistingRequest 提交前库内检测请求
type CheckExistingRequest struct {
	Type   models.MediaType `json:"type" binding:"required,oneof=MOVIE TV"`
	TmdbID string           `json:"tmdbId" binding:"required"`
	Season int              `json:"season"`
}

type ExistingMatchType string

const (
	ExistingMatchMovie   ExistingMatchType = "movie"
	ExistingMatchSeries  ExistingMatchType = "series"
	ExistingMatchSeason  ExistingMatchType = "season"
	ExistingMatchUnknown ExistingMatchType = "unknown"
)

// ExistingSummary 库内已存在摘要
type ExistingSummary struct {
	MatchType        ExistingMatchType `json:"matchType"`
	EmbyItemID       string            `json:"embyItemId,omitempty"`
	Message          string            `json:"message"`
	AvailableSeasons []int             `json:"availableSeasons,omitempty"`
	EpisodeCount     int               `json:"episodeCount,omitempty"`
	DetectionFailed  bool              `json:"detectionFailed,omitempty"`
}

// CheckExistingResponse 提交前库内检测结果
type CheckExistingResponse struct {
	ExistsInLibrary bool             `json:"existsInLibrary"`
	DetectionFailed bool             `json:"detectionFailed,omitempty"`
	ExistingSummary *ExistingSummary `json:"existingSummary,omitempty"`
}

// CreateSubscriptionResult 创建订阅结果
type CreateSubscriptionResult struct {
	Success              bool             `json:"success"`
	SubscriptionID       string           `json:"subscriptionId,omitempty"`
	AlreadyExists        bool             `json:"alreadyExists,omitempty"`
	ConfirmationRequired bool             `json:"confirmationRequired,omitempty"`
	DetectionFailed      bool             `json:"detectionFailed,omitempty"`
	ExistingSummary      *ExistingSummary `json:"existingSummary,omitempty"`
}

// ResubmitSubscriptionRequest 拒绝后重新提交请求
type ResubmitSubscriptionRequest struct {
	Note            string `json:"note" binding:"required"`
	ConfirmExisting bool   `json:"confirmExisting"`
}

// RejectSubscriptionRequest 审核拒绝请求
type RejectSubscriptionRequest struct {
	Reason string `json:"reason" binding:"required"`
}

// SubscriptionWithUser 订阅记录（包含用户信息）
type SubscriptionWithUser struct {
	models.Subscription
	User struct {
		Username string `json:"username"`
		Email    string `json:"email"`
	} `json:"user"`
}

// GetAllSubscriptionsResponse 管理员查询订阅列表响应
type GetAllSubscriptionsResponse struct {
	Data  []SubscriptionWithUser `json:"data"`
	Total int64                  `json:"total"`
}

// ManualSearchRequest 管理员手动补偿搜索请求。
type ManualSearchRequest struct {
	Season *int `json:"season,omitempty"`
}

// ManualSearchResult 管理员手动补偿搜索结果。
type ManualSearchResult struct {
	Subscription models.Subscription             `json:"subscription"`
	Source       string                          `json:"source"`
	Query        string                          `json:"query"`
	MatchMode    string                          `json:"matchMode"`
	Candidates   []moviepilotint.SearchCandidate `json:"candidates"`
}

// ManualDispatchRequest 管理员手动补偿下发请求。
//
// service 优先使用 CandidatePayload，也兼容读取 Candidate.Payload，
// 整剧订阅还必须携带搜索时使用的 Season。
// 不保留 candidateId：MoviePilot 候选 ID 是本地派生的 SHA1 摘要，
// 无法回查上游，service 层从不读取它。
type ManualDispatchRequest struct {
	Candidate        *moviepilotint.SearchCandidate `json:"candidate,omitempty"`
	CandidatePayload map[string]interface{}         `json:"candidatePayload,omitempty"`
	Season           *int                           `json:"season,omitempty"`
}

// ManualDispatchResult 管理员手动补偿下发结果。
type ManualDispatchResult struct {
	Subscription models.Subscription `json:"subscription"`
	Accepted     bool                `json:"accepted"`
	Message      string              `json:"message,omitempty"`
}

// SubscriptionIngestWebhookPayload webhook 入库事件
type SubscriptionIngestWebhookPayload struct {
	ItemType   string
	TmdbID     string
	SeriesID   string
	Season     int
	Episode    int
	EmbyItemID string
	ItemName   string
}
