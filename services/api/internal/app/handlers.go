package app

import "github.com/konghang/ember/backend/internal/handlers"

type appHandlers struct {
	auth            *handlers.AuthHandler
	user            *handlers.UserHandler
	redemptionCode  *handlers.RedemptionCodeHandler
	setting         *handlers.SettingHandler
	config          *handlers.ConfigHandler
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
}

func newAppHandlers() *appHandlers {
	return &appHandlers{
		auth:            handlers.NewAuthHandler(),
		user:            handlers.NewUserHandler(),
		redemptionCode:  handlers.NewRedemptionCodeHandler(),
		setting:         handlers.NewSettingHandler(),
		config:          handlers.NewConfigHandler(),
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
	}
}
