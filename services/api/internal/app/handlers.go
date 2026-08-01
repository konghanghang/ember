package app

import (
	"fmt"
	"os"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/handlers"
	p115integration "github.com/konghang/ember/backend/internal/integrations/p115"
	p115accountpkg "github.com/konghang/ember/backend/internal/services/p115account"
)

type appHandlers struct {
	auth            *handlers.AuthHandler
	user            *handlers.UserHandler
	redemptionCode  *handlers.RedemptionCodeHandler
	setting         *handlers.SettingHandler
	config          *handlers.ConfigHandler
	adminAPIKey     *handlers.AdminAPIKeyHandler
	subscription    *handlers.SubscriptionHandler
	media           *handlers.MediaHandler
	system          *handlers.SystemHandler
	tmdb            *handlers.TMDBHandler
	ranking         *handlers.RankingHandler
	session         *handlers.SessionHandler
	device          *handlers.DeviceHandler
	payment         *handlers.PaymentHandler
	telegram        *handlers.TelegramHandler
	tvCalendar      *handlers.TVCalendarHandler
	playbackHistory *handlers.PlaybackHistoryHandler
	playbackProfile *handlers.UserPlaybackProfileHandler
	mediaQuality    *handlers.MediaQualityHandler
	mediaGap        *handlers.MediaGapHandler
	p115Account     *handlers.P115AccountHandler
}

func newAppHandlers() (*appHandlers, error) {
	p115AccountService, err := p115accountpkg.NewService(
		db.DB,
		os.Getenv("CONFIG_ENCRYPTION_KEY"),
		p115integration.NewCookieCredentialValidator(),
	)
	if err != nil {
		return nil, fmt.Errorf("初始化 115 账号服务失败: %w", err)
	}
	return &appHandlers{
		auth:            handlers.NewAuthHandler(),
		user:            handlers.NewUserHandler(),
		redemptionCode:  handlers.NewRedemptionCodeHandler(),
		setting:         handlers.NewSettingHandler(),
		config:          handlers.NewConfigHandler(),
		adminAPIKey:     handlers.NewAdminAPIKeyHandler(),
		subscription:    handlers.NewSubscriptionHandler(),
		media:           handlers.NewMediaHandler(),
		system:          handlers.NewSystemHandler(),
		tmdb:            handlers.NewTMDBHandler(),
		ranking:         handlers.NewRankingHandler(),
		session:         handlers.NewSessionHandler(),
		device:          handlers.NewDeviceHandler(),
		payment:         handlers.NewPaymentHandler(),
		telegram:        handlers.NewTelegramHandler(),
		tvCalendar:      handlers.NewTVCalendarHandler(),
		playbackHistory: handlers.NewPlaybackHistoryHandler(),
		playbackProfile: handlers.NewUserPlaybackProfileHandler(),
		mediaQuality:    handlers.NewMediaQualityHandler(),
		mediaGap:        handlers.NewMediaGapHandler(),
		p115Account:     handlers.NewP115AccountHandler(p115AccountService),
	}, nil
}
