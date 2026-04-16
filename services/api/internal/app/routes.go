package app

import (
	"github.com/gin-gonic/gin"
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
	api.GET("/tmdb/search", h.tmdb.Search)
	api.GET("/tmdb/tv/:id/seasons", h.tmdb.GetTVSeasons)
}

func registerAdminRoutes(api *gin.RouterGroup, h *appHandlers) {
	admin := api.Group("/admin")
	admin.Use(middleware.JWTAuth(), middleware.AdminOnly())

	admin.GET("/current", h.auth.GetCurrentUser)

	admin.GET("/users", h.user.GetUsers)
	admin.POST("/users", h.user.CreateUserByAdmin)
	admin.GET("/users/:id", h.user.GetUserByID)
	admin.GET("/users/:id/profile", h.playbackProfile.GetAdminUserProfile)
	admin.PUT("/users/:id", h.user.UpdateUserByAdmin)
	admin.PUT("/users/:id/extend", h.user.ExtendExpiry)
	admin.PUT("/users/:id/toggle", h.user.ToggleUserStatus)
	admin.PUT("/users/:id/reset-password", h.user.ResetPassword)
	admin.DELETE("/users/:id", h.user.DeleteUser)

	admin.GET("/redemption-codes", h.redemptionCode.GetRedemptionCodes)
	admin.POST("/redemption-codes", h.redemptionCode.CreateRedemptionCode)
	admin.POST("/redemption-codes/batch", h.redemptionCode.CreateRedemptionCodesBatch)
	admin.PUT("/redemption-codes/:id", h.redemptionCode.UpdateRedemptionCode)
	admin.DELETE("/redemption-codes/:id", h.redemptionCode.DeleteRedemptionCode)
	admin.GET("/user-templates", h.redemptionCode.GetUserTemplates)
	admin.GET("/configs", h.config.GetConfigs)
	admin.PATCH("/configs/:key", h.config.UpdateConfig)
	admin.POST("/configs/:group/test", h.config.TestConfigGroup)
	admin.GET("/redemptions", h.user.GetAllRedemptions)

	admin.GET("/subscriptions", h.subscription.GetAllSubscriptions)
	admin.PUT("/subscriptions/:id/approve", h.subscription.ApproveSubscription)
	admin.PUT("/subscriptions/:id/reject", h.subscription.RejectSubscription)
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

	admin.GET("/plan-groups", h.payment.GetPlanGroups)
	admin.POST("/plan-groups", h.payment.CreatePlanGroup)
	admin.PUT("/plan-groups/:key", h.payment.UpdatePlanGroup)
	admin.DELETE("/plan-groups/:key", h.payment.DeletePlanGroup)

	admin.GET("/plans", h.payment.GetPlans)
	admin.POST("/plans", h.payment.CreatePlan)
	admin.PUT("/plans/:id", h.payment.UpdatePlan)
	admin.DELETE("/plans/:id", h.payment.DeletePlan)

	admin.GET("/payments", h.payment.GetAllPayments)

	admin.POST("/cron/check-expired", h.system.CheckExpiredUsers)
	admin.POST("/cron/generate-ranking", h.ranking.GenerateRanking)
	admin.POST("/rankings/preview", h.ranking.PreviewRanking)
}

func registerInternalRoutes(api *gin.RouterGroup, h *appHandlers) {
	internal := api.Group("/internal")
	internal.Use(middleware.InternalAuth())

	internal.PUT("/subscriptions/:id/approve", h.subscription.ApproveSubscription)
	internal.PUT("/subscriptions/:id/reject", h.subscription.RejectSubscription)
	internal.GET("/settings/:key", h.setting.GetSettingByKey)
	internal.GET("/media/stats", h.media.GetMediaStats)
	internal.POST("/telegram/bind", h.telegram.VerifyBind)
	internal.POST("/telegram/info", h.telegram.GetAccountInfo)
	internal.POST("/telegram/redeem", h.telegram.RedeemByTelegram)
	internal.POST("/telegram/reset-password", h.telegram.ResetPassword)
	internal.POST("/telegram/subscribe", h.telegram.SubscribeByTelegram)
}

func registerAuthenticatedRoutes(api *gin.RouterGroup, h *appHandlers) {
	authenticated := api.Group("")
	authenticated.Use(middleware.JWTAuth())

	authenticated.GET("/subscriptions", h.subscription.GetSubscriptions)
	authenticated.POST("/subscriptions/check-existing", h.subscription.CheckExisting)
	authenticated.POST("/subscriptions", h.subscription.CreateSubscription)
	authenticated.DELETE("/subscriptions/:id", h.subscription.DeleteSubscription)

	authenticated.GET("/profile", h.user.GetProfile)
	authenticated.GET("/profile/analytics", h.playbackProfile.GetCurrentUserProfile)
	authenticated.GET("/account-links", h.setting.GetConsoleAccountLinks)
	authenticated.PUT("/profile", h.user.UpdateProfile)
	authenticated.PUT("/password", h.user.UpdatePassword)
	authenticated.PUT("/email", h.user.UpdateEmail)
	authenticated.POST("/redeem", h.user.RedeemCode)
	authenticated.GET("/redeem/:code/validate", h.user.ValidateRedeemCode)
	authenticated.GET("/redemptions", h.user.GetRedemptions)
	authenticated.POST("/telegram/bindcode", h.telegram.GenerateBindCode)
	authenticated.DELETE("/telegram/unbind", h.telegram.Unbind)

	authenticated.GET("/emby/config", h.media.GetEmbyConfig)
	authenticated.GET("/media/stats", h.media.GetMediaStats)
	authenticated.GET("/media/latest", h.media.GetLatestItems)

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
	user.Use(middleware.JWTAuth(), middleware.UserOnly())

	user.GET("/profile", h.user.GetProfile)
	user.PUT("/profile", h.user.UpdateProfile)
	user.PUT("/password", h.user.UpdatePassword)
	user.PUT("/email", h.user.UpdateEmail)
	user.POST("/redeem", h.user.RedeemCode)
	user.GET("/redeem/:code/validate", h.user.ValidateRedeemCode)
	user.GET("/redemptions", h.user.GetRedemptions)

	user.GET("/subscriptions", h.subscription.GetMySubscriptions)
	user.POST("/subscriptions/check-existing", h.subscription.CheckExisting)
	user.POST("/subscriptions", h.subscription.CreateSubscription)
	user.DELETE("/subscriptions/:id", h.subscription.DeleteSubscription)

	user.GET("/emby/config", h.media.GetEmbyConfig)
	user.GET("/media/stats", h.media.GetMediaStats)
}
