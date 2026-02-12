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

	// API 路由组
	api := r.Group("/api/v1")
	{
		// ==================== 公开路由（无需认证） ====================

		// 统一登录（admin/user）
		api.POST("/login", authHandler.Login)
		// 统一登出（JWT 无状态，仅需认证，不区分角色）
		api.POST("/logout", middleware.JWTAuth(), authHandler.Logout)
		api.POST("/user/register", authHandler.RegisterUser)
		api.GET("/register/mode", settingHandler.GetRegistrationMode)
		api.GET("/register/code/:code/validate", redemptionCodeHandler.ValidateCode)

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
			admin.PUT("/users/:id/extend", userHandler.ExtendExpiry)
			admin.PUT("/users/:id/toggle", userHandler.ToggleUserStatus)
			admin.PUT("/users/:id/reset-password", userHandler.ResetPassword)
			admin.DELETE("/users/:id", userHandler.DeleteUser)

			admin.GET("/redemption-codes", redemptionCodeHandler.GetRedemptionCodes)
			admin.POST("/redemption-codes", redemptionCodeHandler.CreateRedemptionCode)
			admin.DELETE("/redemption-codes/:id", redemptionCodeHandler.DeleteRedemptionCode)
			admin.GET("/settings", settingHandler.GetSettings)
			admin.PUT("/settings/:key", settingHandler.UpdateSetting)
			admin.GET("/redemptions", userHandler.GetAllRedemptions)

			// 订阅管理
			admin.GET("/subscriptions", subscriptionHandler.GetAllSubscriptions)
			admin.PUT("/subscriptions/:id/approve", subscriptionHandler.ApproveSubscription)
			admin.PUT("/subscriptions/:id/reject", subscriptionHandler.RejectSubscription)

			// 系统管理
			admin.GET("/system/info", systemHandler.GetSystemInfo)
			admin.POST("/system/test-emby", systemHandler.TestEmbyConnection)

			// 定时任务
			admin.POST("/cron/check-expired", systemHandler.CheckExpiredUsers)
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
		schedule := os.Getenv("CRON_SCHEDULE")
		if schedule == "" {
			schedule = "0 2 * * *"
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

		systemService := services.NewSystemService()
		c := cron.New(cron.WithLocation(tz))
		if _, err := c.AddFunc(schedule, func() {
			log.Println("[Cron] 开始检查过期用户...")
			result, err := systemService.CheckExpiredUsers()
			if err != nil {
				log.Printf("[Cron] 检查失败：%v", err)
				return
			}
			log.Printf("[Cron] 完成，封禁 %d/%d 个用户", result.DisabledCount, result.TotalExpired)
		}); err != nil {
			log.Printf("定时任务注册失败：%v", err)
		} else {
			c.Start()
			defer c.Stop()
			log.Printf("定时任务已启用：%s (%s)", schedule, tzName)
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
