package main

import (
	"log"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/common"
	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/handlers"
	"github.com/konghang/ember/backend/internal/middleware"
	"github.com/konghang/ember/backend/internal/models"
	"github.com/konghang/ember/backend/internal/services"
	"github.com/robfig/cron/v3"
)

func main() {
	// 初始化数据库
	db.InitDB()
	defer db.Close()

	// 初始化 JWT
	if err := common.InitJWT(); err != nil {
		log.Fatalf("❌ JWT 初始化失败：%v", err)
	}

	// 创建 Gin 路由
	r := gin.Default()

	// 健康检查端点
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"message": "Ember Go API is running",
		})
	})

	// 创建 Handlers
	authHandler := handlers.NewAuthHandler()
	userHandler := handlers.NewUserHandler()
	redemptionCodeHandler := handlers.NewRedemptionCodeHandler()
	settingHandler := handlers.NewSettingHandler()
	subscriptionHandler := handlers.NewSubscriptionHandler()
	mediaHandler := handlers.NewMediaHandler()
	systemHandler := handlers.NewSystemHandler()
	tmdbHandler := handlers.NewTMDBHandler()
	rankingHandler := handlers.NewRankingHandler()
	sessionHandler := handlers.NewSessionHandler()
	deviceHandler := handlers.NewDeviceHandler()
	paymentHandler := handlers.NewPaymentHandler()
	telegramHandler := handlers.NewTelegramHandler()

	// API 路由组
	api := r.Group("/api/v1")
	{
		// ==================== 公开路由（无需认证） ====================

		// 统一登录（admin/user）
		api.POST("/login", authHandler.Login)
		// 统一登出（JWT 无状态，仅需认证，不区分角色）
		api.POST("/logout", middleware.JWTAuth(), authHandler.Logout)
		api.POST("/user/register", authHandler.RegisterUser)
		api.POST("/register/send-code", authHandler.SendEmailCode)
		api.POST("/forgot-password/send-code", authHandler.SendResetCode)
		api.POST("/forgot-password/reset", authHandler.ResetPasswordByCode)
		api.GET("/register/mode", settingHandler.GetRegistrationMode)
		api.GET("/register/code/:code/validate", redemptionCodeHandler.ValidateCode)
		api.GET("/plans", paymentHandler.GetActivePlans)
		api.POST("/webhooks/stripe", paymentHandler.HandleStripeWebhook)

		// TMDB 搜索（公开）
		api.GET("/tmdb/search", tmdbHandler.Search)

		// ==================== 管理员路由（需要认证） ====================
		admin := api.Group("/admin")
		admin.Use(middleware.JWTAuth(), middleware.AdminOnly())
		{
			// 认证相关
			admin.GET("/current", authHandler.GetCurrentUser)

			// 用户管理
			admin.GET("/users", userHandler.GetUsers)
			admin.GET("/users/:id", userHandler.GetUserByID)
			admin.PUT("/users/:id", userHandler.UpdateUserByAdmin)
			admin.PUT("/users/:id/extend", userHandler.ExtendExpiry)
			admin.PUT("/users/:id/toggle", userHandler.ToggleUserStatus)
			admin.PUT("/users/:id/reset-password", userHandler.ResetPassword)
			admin.DELETE("/users/:id", userHandler.DeleteUser)

			admin.GET("/redemption-codes", redemptionCodeHandler.GetRedemptionCodes)
			admin.POST("/redemption-codes", redemptionCodeHandler.CreateRedemptionCode)
			admin.PUT("/redemption-codes/:id", redemptionCodeHandler.UpdateRedemptionCode)
			admin.DELETE("/redemption-codes/:id", redemptionCodeHandler.DeleteRedemptionCode)
			admin.GET("/user-templates", redemptionCodeHandler.GetUserTemplates)
			admin.GET("/settings", settingHandler.GetSettings)
			admin.PUT("/settings/:key", settingHandler.UpdateSetting)
			admin.GET("/redemptions", userHandler.GetAllRedemptions)

			// 订阅管理
			admin.GET("/subscriptions", subscriptionHandler.GetAllSubscriptions)
			admin.PUT("/subscriptions/:id/approve", subscriptionHandler.ApproveSubscription)
			admin.PUT("/subscriptions/:id/reject", subscriptionHandler.RejectSubscription)
			admin.DELETE("/subscriptions/:id", subscriptionHandler.AdminDeleteSubscription)

			// 系统管理
			admin.GET("/system/info", systemHandler.GetSystemInfo)
			admin.POST("/system/test-emby", systemHandler.TestEmbyConnection)

			// 活跃会话监控
			admin.GET("/sessions", sessionHandler.GetActiveSessions)

			// 设备管理
			admin.GET("/devices", deviceHandler.GetDevices)
			admin.GET("/devices/stats", deviceHandler.GetStats)
			admin.GET("/devices/actions", deviceHandler.GetActions)
			admin.GET("/devices/blacklist", deviceHandler.GetBlacklist)
			admin.POST("/devices/blacklist", deviceHandler.AddToBlacklist)
			admin.DELETE("/devices/blacklist/:clientName", deviceHandler.RemoveFromBlacklist)
			admin.POST("/devices/blacklist/logout-all", deviceHandler.LogoutBlacklistedDevices)
			admin.POST("/devices/logout/:deviceId", deviceHandler.LogoutDevice)

			// 付费方案
			admin.GET("/plans", paymentHandler.GetPlans)
			admin.POST("/plans", paymentHandler.CreatePlan)
			admin.PUT("/plans/:id", paymentHandler.UpdatePlan)
			admin.DELETE("/plans/:id", paymentHandler.DeletePlan)

			// 支付记录
			admin.GET("/payments", paymentHandler.GetAllPayments)

			// 定时任务
			admin.POST("/cron/check-expired", systemHandler.CheckExpiredUsers)
			admin.POST("/cron/generate-ranking", rankingHandler.GenerateRanking)

			// 排行榜预览（不入库、不推送）
			admin.POST("/rankings/preview", rankingHandler.PreviewRanking)
		}

		// ==================== 内部服务路由（Bot 调用） ====================
		internal := api.Group("/internal")
		internal.Use(middleware.InternalAuth())
		{
			internal.PUT("/subscriptions/:id/approve", subscriptionHandler.ApproveSubscription)
			internal.PUT("/subscriptions/:id/reject", subscriptionHandler.RejectSubscription)
			internal.GET("/settings/:key", settingHandler.GetSettingByKey)
			internal.POST("/telegram/bind", telegramHandler.VerifyBind)
			internal.POST("/telegram/info", telegramHandler.GetAccountInfo)
			internal.POST("/telegram/redeem", telegramHandler.RedeemByTelegram)
			internal.POST("/telegram/reset-password", telegramHandler.ResetPassword)
			internal.POST("/telegram/subscribe", telegramHandler.SubscribeByTelegram)
		}

		// ==================== 统一认证路由（admin + user 共享） ====================
		authenticated := api.Group("")
		authenticated.Use(middleware.JWTAuth())
		{
			// 订阅管理（统一）
			authenticated.GET("/subscriptions", subscriptionHandler.GetSubscriptions)
			authenticated.POST("/subscriptions", subscriptionHandler.CreateSubscription)
			authenticated.DELETE("/subscriptions/:id", subscriptionHandler.DeleteSubscription)

			// 个人信息
			authenticated.GET("/profile", userHandler.GetProfile)
			authenticated.PUT("/profile", userHandler.UpdateProfile)
			authenticated.PUT("/password", userHandler.UpdatePassword)
			authenticated.PUT("/email", userHandler.UpdateEmail)
			authenticated.POST("/telegram/bindcode", telegramHandler.GenerateBindCode)
			authenticated.DELETE("/telegram/unbind", telegramHandler.Unbind)

			// 媒体相关
			authenticated.GET("/emby/config", mediaHandler.GetEmbyConfig)
			authenticated.GET("/media/stats", mediaHandler.GetMediaStats)
			authenticated.GET("/media/latest", mediaHandler.GetLatestItems)

			// 播放排行
			authenticated.GET("/rankings/latest", rankingHandler.GetLatestRanking)
			authenticated.GET("/rankings/history", rankingHandler.GetHistoryRanking)

			// 支付
			authenticated.POST("/payments/checkout", paymentHandler.CreateCheckout)
			authenticated.GET("/payments", paymentHandler.GetMyPayments)
		}

		// ==================== 用户路由（需要认证） ====================
		user := api.Group("/user")
		user.Use(middleware.JWTAuth(), middleware.UserOnly())
		{
			// 个人信息
			user.GET("/profile", userHandler.GetProfile)
			user.PUT("/profile", userHandler.UpdateProfile)
			user.PUT("/password", userHandler.UpdatePassword)
			user.PUT("/email", userHandler.UpdateEmail)
			user.POST("/redeem", userHandler.RedeemCode)
			user.GET("/redeem/:code/validate", userHandler.ValidateRedeemCode)
			user.GET("/redemptions", userHandler.GetRedemptions)

			// 订阅管理
			user.GET("/subscriptions", subscriptionHandler.GetMySubscriptions)
			user.POST("/subscriptions", subscriptionHandler.CreateSubscription)
			user.DELETE("/subscriptions/:id", subscriptionHandler.DeleteSubscription)

			// 媒体相关（需要认证）
			user.GET("/emby/config", mediaHandler.GetEmbyConfig)
			user.GET("/media/stats", mediaHandler.GetMediaStats)
		}
	}

	cronEnabled := os.Getenv("CRON_ENABLED")
	if cronEnabled == "" {
		cronEnabled = "true"
	}

	if cronEnabled == "true" {
		expiredSchedule := os.Getenv("CRON_SCHEDULE")
		if expiredSchedule == "" {
			expiredSchedule = "0 2 * * *"
		}

		rankingCronEnabled := os.Getenv("RANKING_CRON_ENABLED")
		if rankingCronEnabled == "" {
			// 默认关闭，避免升级后在未配置 Playback Reporting 插件/Emby 环境变量时产生隐式行为变化
			rankingCronEnabled = "false"
		}

		rankingDailySchedule := os.Getenv("RANKING_DAILY_SCHEDULE")
		if rankingDailySchedule == "" {
			rankingDailySchedule = "0 20 * * *"
		}
		rankingWeeklySchedule := os.Getenv("RANKING_WEEKLY_SCHEDULE")
		if rankingWeeklySchedule == "" {
			// 默认：周日 20:30
			rankingWeeklySchedule = "30 20 * * 0"
		}

		tzName := os.Getenv("CRON_TIMEZONE")
		if tzName == "" {
			tzName = "Asia/Shanghai"
		}

		tz, err := time.LoadLocation(tzName)
		if err != nil {
			log.Printf("时区解析失败，使用 UTC：%v", err)
			tz = time.UTC
		}

		c := cron.New(cron.WithLocation(tz))

		systemService := services.NewSystemService()
		emailService := services.NewEmailService()
		telegramService := services.NewTelegramService()
		var rankingService *services.PlaybackRankingService
		if rankingCronEnabled == "true" {
			rankingService = services.NewPlaybackRankingService()
		}

		taskRegistered := false

		if _, err := c.AddFunc("0 3 * * *", func() {
			count, err := emailService.CleanupExpired()
			if err != nil {
				log.Printf("[Cron] 清理过期验证码失败：%v", err)
			} else if count > 0 {
				log.Printf("[Cron] 已清理 %d 条过期验证码", count)
			}

			telegramCount, telegramErr := telegramService.CleanupExpiredBindCodes()
			if telegramErr != nil {
				log.Printf("[Cron] 清理过期 Telegram 绑定码失败：%v", telegramErr)
			} else if telegramCount > 0 {
				log.Printf("[Cron] 已清理 %d 条过期 Telegram 绑定码", telegramCount)
			}
		}); err != nil {
			log.Printf("定时任务注册失败（验证码清理）：%v", err)
		} else {
			taskRegistered = true
		}

		if _, err := c.AddFunc(expiredSchedule, func() {
			log.Println("[Cron] 开始检查过期用户...")
			result, err := systemService.CheckExpiredUsers()
			if err != nil {
				log.Printf("[Cron] 检查失败：%v", err)
				return
			}
			log.Printf("[Cron] 完成，封禁 %d/%d 个用户", result.DisabledCount, result.TotalExpired)
		}); err != nil {
			log.Printf("定时任务注册失败（过期检查）：%v", err)
		} else {
			taskRegistered = true
		}

		if rankingCronEnabled == "true" {
			if _, err := c.AddFunc(rankingDailySchedule, func() {
				log.Println("[Cron] 开始生成播放日榜...")
				if err := rankingService.GenerateRanking(models.RankingDaily, nil, nil); err != nil {
					log.Printf("[Cron] 日榜生成失败：%v", err)
					return
				}
				log.Println("[Cron] 日榜生成完成")
			}); err != nil {
				log.Printf("定时任务注册失败（日榜）：%v", err)
			} else {
				taskRegistered = true
			}

			if _, err := c.AddFunc(rankingWeeklySchedule, func() {
				log.Println("[Cron] 开始生成播放周榜...")
				if err := rankingService.GenerateRanking(models.RankingWeekly, nil, nil); err != nil {
					log.Printf("[Cron] 周榜生成失败：%v", err)
					return
				}
				log.Println("[Cron] 周榜生成完成")
			}); err != nil {
				log.Printf("定时任务注册失败（周榜）：%v", err)
			} else {
				taskRegistered = true
			}
		}

		if taskRegistered {
			c.Start()
			defer c.Stop()
			if rankingCronEnabled == "true" {
				log.Printf(
					"定时任务已启用：过期检查(%s), 日榜(%s), 周榜(%s) (%s)",
					expiredSchedule,
					rankingDailySchedule,
					rankingWeeklySchedule,
					tzName,
				)
			} else {
				log.Printf("定时任务已启用：过期检查(%s) (%s)", expiredSchedule, tzName)
			}
		}
	}

	// 启动服务器
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("🚀 Ember Go API 服务启动")
	log.Printf("📍 端口: %s", port)
	log.Printf("📚 API 文档: http://localhost:%s/health", port)
	log.Printf("✅ JWT 认证已启用")

	if err := r.Run(":" + port); err != nil {
		log.Fatalf("❌ 服务器启动失败：%v", err)
	}
}
