package app

import (
	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/apiroutes"
	"github.com/konghang/ember/backend/internal/middleware"
)

func registerRoutes(r *gin.Engine, h *appHandlers) {
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"message": "Ember Go API is running",
		})
	})

	api := r.Group("/api/v1")
	registerPublicRoutes(api, h)
	registerAdminRoutes(api, h)
	registerInternalRoutes(api, h)
	registerAuthenticatedRoutes(api, h)
	registerUserRoutes(api, h)
}

func registerPublicRoutes(api *gin.RouterGroup, h *appHandlers) {
	api.POST("/login", h.auth.Login)
	api.GET("/login/protection-config", h.setting.GetLoginProtectionConfig)
	api.POST("/logout", middleware.JWTAuth(), h.auth.Logout)
	api.POST("/user/register", h.auth.RegisterUser)
	api.POST("/register/send-code", h.auth.SendEmailCode)
	api.POST("/forgot-password/send-code", h.auth.SendResetCode)
	api.POST("/forgot-password/reset", h.auth.ResetPasswordByCode)
	api.GET("/register/mode", h.setting.GetRegistrationMode)
	api.GET("/register/code/:code/validate", h.redemptionCode.ValidateRegistrationCode)
	api.POST("/webhooks/stripe", h.payment.HandleStripeWebhook)
	api.POST("/webhooks/emby", h.tvCalendar.HandleEmbyWebhook)
}

func registerAdminRoutes(api *gin.RouterGroup, h *appHandlers) {
	admin := api.Group("/admin")
	admin.Use(middleware.AdminCredentialAuth(), middleware.AdminOnly())

	admin.GET(apiroutes.CurrentPath, h.auth.GetCurrentUser)
	admin.GET("/emby-users", h.auth.ListAdminEmbyUsers)
	admin.PUT("/current/emby-binding", h.auth.BindEmbyAccount)
	admin.DELETE("/current/emby-binding", h.auth.UnbindEmbyAccount)

	admin.GET("/users", h.user.GetUsers)
	admin.POST("/users", h.user.CreateUserByAdmin)
	admin.GET("/users/:id", h.user.GetUserByID)
	admin.GET("/users/:id/profile", h.playbackProfile.GetAdminUserProfile)
	admin.PUT("/users/:id", h.user.UpdateUserByAdmin)
	admin.PUT("/users/:id/extend", h.user.ExtendExpiry)
	admin.PUT("/users/:id/toggle", h.user.ToggleUserStatus)
	admin.PUT("/users/:id/reset-password", h.user.ResetPassword)
	admin.DELETE("/users/:id", h.user.DeleteUser)
	admin.DELETE("/users/:id/media-libraries/preferences", h.user.ClearAdminUserMediaLibraryPreferences)
	admin.POST("/users/:id/media-libraries/sync", h.user.SyncAdminUserMediaLibraryPreferences)
	admin.POST("/users/:id/emby-policy-sync/retry", h.user.RetryAdminUserPolicySync)
	admin.PUT("/users/:id/emby-access", h.user.UpdateAdminUserEmbyAccess)

	admin.GET("/redemption-codes", h.redemptionCode.GetRedemptionCodes)
	admin.POST("/redemption-codes", h.redemptionCode.CreateRedemptionCode)
	admin.POST("/redemption-codes/batch", h.redemptionCode.CreateRedemptionCodesBatch)
	admin.PUT("/redemption-codes/:id", h.redemptionCode.UpdateRedemptionCode)
	admin.DELETE("/redemption-codes/:id", h.redemptionCode.DeleteRedemptionCode)
	admin.GET("/configs", h.config.GetConfigs)
	admin.PATCH("/configs/:key", h.config.UpdateConfig)
	admin.POST("/configs/:group/test", h.config.TestConfigGroup)
	admin.GET("/external-api-key", h.adminAPIKey.GetStatus)
	admin.POST("/external-api-key", h.adminAPIKey.Generate)
	admin.DELETE("/external-api-key", h.adminAPIKey.Disable)
	admin.GET("/redemptions", h.user.GetAllRedemptions)

	admin.GET("/subscriptions", h.subscription.GetAllSubscriptions)
	admin.PUT("/subscriptions/:id/approve", h.subscription.ApproveSubscription)
	admin.PUT("/subscriptions/:id/reject", h.subscription.RejectSubscription)
	admin.PUT("/subscriptions/:id/ingest", h.subscription.MarkSubscriptionIngested)
	admin.PUT("/subscriptions/:id/redispatch", h.subscription.RedispatchSubscription)
	admin.POST("/subscriptions/:id/manual-search", h.subscription.ManualSearchSubscription)
	admin.POST("/subscriptions/:id/manual-dispatch", h.subscription.ManualDispatchSubscription)
	admin.DELETE("/subscriptions/:id", h.subscription.AdminDeleteSubscription)

	admin.GET("/system/info", h.system.GetSystemInfo)
	admin.POST("/system/test-emby", h.system.TestEmbyConnection)

	admin.GET("/sessions", h.session.GetActiveSessions)
	admin.GET("/playback-history", h.playbackHistory.GetPlaybackHistory)
	admin.GET("/playback-profiles", h.playbackProfile.GetAdminUserProfiles)
	admin.GET("/media-quality/libraries", h.mediaQuality.GetLibraries)
	admin.GET("/media-quality/libraries/:libraryId", h.mediaQuality.GetLibraryQuality)
	admin.GET("/media-quality/libraries/:libraryId/groups/:groupId/details", h.mediaQuality.GetLibraryQualityGroupDetails)
	admin.POST("/media-quality/libraries/:libraryId/scan", h.mediaQuality.ScanLibraryQuality)
	admin.GET("/media-quality/posters/:itemId", h.mediaQuality.GetPoster)
	admin.GET("/media-gaps", h.mediaGap.ListMediaGaps)
	admin.GET("/media-gaps/grouped", h.mediaGap.ListGroupedMediaGaps)
	admin.GET("/media-gaps/scan-status", h.mediaGap.GetMediaGapScanStatus)
	admin.POST("/media-gaps/scan", h.mediaGap.ScanMediaGaps)
	admin.POST("/media-gaps/:id/search", h.mediaGap.SearchMediaGap)
	admin.POST("/media-gaps/:id/dispatch", h.mediaGap.DispatchMediaGap)
	admin.POST("/media-gaps/:id/ignore", h.mediaGap.IgnoreMediaGap)

	admin.GET("/devices", h.device.GetDevices)
	admin.GET("/devices/stats", h.device.GetStats)
	admin.GET("/devices/actions", h.device.GetActions)
	admin.GET("/devices/blacklist", h.device.GetBlacklist)
	admin.POST("/devices/blacklist", h.device.AddToBlacklist)
	admin.DELETE("/devices/blacklist/:clientName", h.device.RemoveFromBlacklist)
	admin.POST("/devices/blacklist/logout-all", h.device.LogoutBlacklistedDevices)
	admin.POST("/devices/logout/:deviceId", h.device.LogoutDevice)

	admin.POST("/tv-calendar/sync", h.tvCalendar.Sync)
	admin.POST("/tv-calendar/refresh", h.tvCalendar.Refresh)

	admin.GET("/media-libraries", h.payment.GetAdminMediaLibraries)
	admin.GET("/plan-groups", h.payment.GetPlanGroups)
	admin.POST("/plan-groups", h.payment.CreatePlanGroup)
	admin.PUT("/plan-groups/:key", h.payment.UpdatePlanGroup)
	admin.DELETE("/plan-groups/:key", h.payment.DeletePlanGroup)
	admin.GET("/plan-groups/:key/media-libraries", h.payment.GetPlanGroupMediaLibraries)
	admin.PUT("/plan-groups/:key/media-libraries", h.payment.UpdatePlanGroupMediaLibraries)
	admin.POST("/plan-groups/:key/media-libraries/sync-preview", h.payment.PreviewPlanGroupMediaLibrarySync)
	admin.POST("/plan-groups/:key/media-libraries/sync-apply", h.payment.ApplyPlanGroupMediaLibrarySync)
	admin.GET("/plan-groups/:key/emby-policy-template", h.payment.GetPlanGroupEmbyPolicyTemplate)
	admin.PUT("/plan-groups/:key/emby-policy-template", h.payment.UpdatePlanGroupEmbyPolicyTemplate)
	admin.GET("/emby-policy-sync-batches/:id", h.payment.GetEmbyPolicySyncBatch)
	admin.POST("/emby-policy-sync-batches/:id/retry-failed", h.payment.RetryFailedEmbyPolicySyncBatch)

	admin.GET("/plans", h.payment.GetPlans)
	admin.POST("/plans", h.payment.CreatePlan)
	admin.PUT("/plans/:id", h.payment.UpdatePlan)
	admin.DELETE("/plans/:id", h.payment.DeletePlan)

	admin.GET("/payments", h.payment.GetAllPayments)

	admin.POST("/cron/check-expired", h.system.CheckExpiredUsers)
	admin.POST("/cron/generate-ranking", h.ranking.GenerateRanking)
	admin.POST("/rankings/preview", h.ranking.PreviewRanking)
	admin.GET("/rankings/library-allowlist", h.ranking.GetRankingLibraryAllowlist)
	admin.PUT("/rankings/library-allowlist", h.ranking.UpdateRankingLibraryAllowlist)
}

func registerInternalRoutes(api *gin.RouterGroup, h *appHandlers) {
	internal := api.Group("/internal")
	internal.Use(middleware.InternalAuth())

	internal.PUT("/subscriptions/:id/approve", h.subscription.ApproveSubscription)
	internal.PUT("/subscriptions/:id/reject", h.subscription.RejectSubscription)
	internal.GET("/settings/:key", h.setting.GetSettingByKey)
	internal.GET("/media/stats", h.media.GetMediaStats)
	internal.GET("/tmdb/search", h.tmdb.Search)
	internal.GET("/tmdb/tv/:id/seasons", h.tmdb.GetTVSeasons)
	internal.POST("/telegram/bind", h.telegram.VerifyBind)
	internal.POST("/telegram/info", h.telegram.GetAccountInfo)
	internal.POST("/telegram/redeem", h.telegram.RedeemByTelegram)
	internal.POST("/telegram/reset-password", h.telegram.ResetPassword)
	internal.POST("/telegram/subscribe", h.telegram.SubscribeByTelegram)
	internal.POST("/telegram/media-libraries", h.telegram.GetMediaLibraries)
	internal.PUT("/telegram/media-libraries/:libraryId/toggle", h.telegram.ToggleMediaLibrary)
	internal.DELETE("/telegram/media-libraries/preferences", h.telegram.ResetMediaLibraryPreferences)
	internal.POST("/telegram/reject-request/enqueue", h.telegram.EnqueuePendingReject)
	internal.POST("/telegram/reject-request/pop", h.telegram.PopPendingReject)
	internal.POST("/telegram/polling-lock/acquire", h.telegram.AcquirePollingLock)
	internal.POST("/telegram/polling-lock/renew", h.telegram.RenewPollingLock)
	internal.POST("/telegram/polling-lock/release", h.telegram.ReleasePollingLock)
}

func registerAuthenticatedRoutes(api *gin.RouterGroup, h *appHandlers) {
	authenticated := api.Group("")
	authenticated.Use(middleware.JWTAuth(), middleware.PasswordResetRequired())

	authenticated.GET("/subscriptions", h.subscription.GetSubscriptions)
	authenticated.POST("/subscriptions/check-existing", h.subscription.CheckExisting)
	authenticated.POST("/subscriptions", h.subscription.CreateSubscription)
	authenticated.POST("/subscriptions/:id/resubmit", h.subscription.ResubmitSubscription)
	authenticated.DELETE("/subscriptions/:id", h.subscription.DeleteSubscription)
	authenticated.GET("/tmdb/search", h.tmdb.Search)
	authenticated.GET("/tmdb/tv/:id/seasons", h.tmdb.GetTVSeasons)

	authenticated.GET(apiroutes.ProfilePath, h.user.GetProfile)
	authenticated.GET("/profile/analytics", h.playbackProfile.GetCurrentUserProfile)
	authenticated.GET(apiroutes.AccountLinksPath, h.setting.GetConsoleAccountLinks)
	authenticated.PUT(apiroutes.PasswordPath, h.user.UpdatePassword)
	authenticated.POST("/email/send-code", h.user.SendEmailChangeCode)
	authenticated.PUT("/email", h.user.UpdateEmail)
	authenticated.POST("/redeem", h.user.RedeemCode)
	authenticated.GET("/redeem/:code/validate", h.user.ValidateRedeemCode)
	authenticated.GET("/redemptions", h.user.GetRedemptions)
	authenticated.POST("/telegram/bindcode", h.telegram.GenerateBindCode)
	authenticated.DELETE("/telegram/unbind", h.telegram.Unbind)

	authenticated.GET("/emby/config", h.media.GetEmbyConfig)
	authenticated.GET("/media/stats", h.media.GetMediaStats)
	authenticated.GET("/media/latest", h.media.GetLatestItems)
	authenticated.GET("/media/posters/:itemId", h.media.GetPoster)
	authenticated.GET("/user/media-libraries", h.user.GetUserMediaLibraries)
	authenticated.PUT("/user/media-libraries", h.user.UpdateUserMediaLibraries)
	authenticated.DELETE("/user/media-libraries/preferences", h.user.ResetUserMediaLibraryPreferences)

	authenticated.GET("/rankings/latest", h.ranking.GetLatestRanking)
	authenticated.GET("/rankings/history", h.ranking.GetHistoryRanking)

	authenticated.GET("/plans", h.payment.GetUserPlans)
	authenticated.GET("/payments/plans", h.payment.GetUserPlans)
	authenticated.POST("/payments/checkout", h.payment.CreateCheckout)
	authenticated.GET("/payments", h.payment.GetMyPayments)

	authenticated.GET("/tv-calendar/global", h.tvCalendar.GetGlobalWeeklyCalendar)
	authenticated.GET("/tv-calendar/following", h.tvCalendar.GetFollowingWeeklyCalendar)
	authenticated.GET("/tv-calendar", h.tvCalendar.GetCalendar)
	authenticated.GET("/tv-calendar/subscriptions", h.tvCalendar.GetSubscriptions)
	authenticated.POST("/tv-calendar/subscriptions", h.tvCalendar.Subscribe)
	authenticated.DELETE("/tv-calendar/subscriptions/:tmdbId", h.tvCalendar.Unsubscribe)
}

func registerUserRoutes(api *gin.RouterGroup, h *appHandlers) {
	user := api.Group("/user")
	user.Use(middleware.JWTAuth(), middleware.PasswordResetRequired(), middleware.UserOnly())

	user.GET(apiroutes.ProfilePath, h.user.GetProfile)
	user.PUT(apiroutes.PasswordPath, h.user.UpdatePassword)
	user.POST("/email/send-code", h.user.SendEmailChangeCode)
	user.PUT("/email", h.user.UpdateEmail)
	user.POST("/redeem", h.user.RedeemCode)
	user.GET("/redeem/:code/validate", h.user.ValidateRedeemCode)
	user.GET("/redemptions", h.user.GetRedemptions)

	user.GET("/subscriptions", h.subscription.GetMySubscriptions)
	user.POST("/subscriptions/check-existing", h.subscription.CheckExisting)
	user.POST("/subscriptions", h.subscription.CreateSubscription)
	user.POST("/subscriptions/:id/resubmit", h.subscription.ResubmitSubscription)
	user.DELETE("/subscriptions/:id", h.subscription.DeleteSubscription)

	user.GET("/emby/config", h.media.GetEmbyConfig)
	user.GET("/media/stats", h.media.GetMediaStats)
}
