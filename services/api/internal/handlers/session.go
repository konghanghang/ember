package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/common/httpx"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
)

// SessionHandler 活跃会话处理器
type SessionHandler struct {
	embyService *embyint.EmbyService
}

type ActiveNowPlayingItem struct {
	Name              string `json:"name"`
	ID                string `json:"id"`
	Type              string `json:"type"` // "Movie" 或 "Episode"
	MediaType         string `json:"mediaType"`
	RunTimeTicks      int64  `json:"runTimeTicks"`                // ticks（÷10000000=秒）
	SeriesName        string `json:"seriesName,omitempty"`        // 仅 Episode
	IndexNumber       int    `json:"indexNumber,omitempty"`       // 仅 Episode
	ParentIndexNumber int    `json:"parentIndexNumber,omitempty"` // 仅 Episode
	ProductionYear    int    `json:"productionYear,omitempty"`
}

type ActivePlayState struct {
	PositionTicks int64  `json:"positionTicks"`
	IsPaused      bool   `json:"isPaused"`
	IsMuted       bool   `json:"isMuted"`
	PlayMethod    string `json:"playMethod"` // "DirectPlay" / "DirectStream" / "Transcode"
}

type ActiveSession struct {
	ID                 string                `json:"id"`
	UserID             string                `json:"userId"`
	UserName           string                `json:"userName"`
	Client             string                `json:"client"`
	DeviceName         string                `json:"deviceName"`
	DeviceID           string                `json:"deviceId"`
	RemoteEndpoint     string                `json:"remoteEndpoint"`
	ApplicationVersion string                `json:"applicationVersion"`
	LastActivityDate   string                `json:"lastActivityDate"`
	NowPlayingItem     *ActiveNowPlayingItem `json:"nowPlayingItem,omitempty"`
	PlayState          *ActivePlayState      `json:"playState,omitempty"`
}

// NewSessionHandler 创建活跃会话处理器
func NewSessionHandler() *SessionHandler {
	return &SessionHandler{
		embyService: embyint.GetSharedService(),
	}
}

// GetActiveSessions 获取当前活跃播放会话
// GET /api/v1/admin/sessions
func (h *SessionHandler) GetActiveSessions(c *gin.Context) {
	sessions, err := h.embyService.GetSessions()
	if err != nil {
		httpx.InternalError(c, err)
		return
	}

	out := make([]ActiveSession, 0, len(sessions))
	for _, s := range sessions {
		dto := ActiveSession{
			ID:                 s.ID,
			UserID:             s.UserID,
			UserName:           s.UserName,
			Client:             s.Client,
			DeviceName:         s.DeviceName,
			DeviceID:           s.DeviceID,
			RemoteEndpoint:     s.RemoteEndPoint,
			ApplicationVersion: s.ApplicationVersion,
			LastActivityDate:   s.LastActivityDate,
		}

		if s.NowPlayingItem != nil {
			dto.NowPlayingItem = &ActiveNowPlayingItem{
				Name:              s.NowPlayingItem.Name,
				ID:                s.NowPlayingItem.ID,
				Type:              s.NowPlayingItem.Type,
				MediaType:         s.NowPlayingItem.MediaType,
				RunTimeTicks:      s.NowPlayingItem.RunTimeTicks,
				SeriesName:        s.NowPlayingItem.SeriesName,
				IndexNumber:       s.NowPlayingItem.IndexNumber,
				ParentIndexNumber: s.NowPlayingItem.ParentIndexNumber,
				ProductionYear:    s.NowPlayingItem.ProductionYear,
			}
		}

		if s.PlayState != nil {
			dto.PlayState = &ActivePlayState{
				PositionTicks: s.PlayState.PositionTicks,
				IsPaused:      s.PlayState.IsPaused,
				IsMuted:       s.PlayState.IsMuted,
				PlayMethod:    s.PlayState.PlayMethod,
			}
		}

		out = append(out, dto)
	}

	c.JSON(http.StatusOK, gin.H{"data": out})
}
