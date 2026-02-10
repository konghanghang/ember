package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/common"
	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/handlers"
	"github.com/konghang/ember/backend/internal/middleware"
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
	inviteHandler := handlers.NewInviteHandler()
	subscriptionHandler := handlers.NewSubscriptionHandler()

	// API 路由组
	api := r.Group("/api/v1")
	{
		// ==================== 公开路由（无需认证） ====================

		// 管理员登录
		api.POST("/admin/login", authHandler.AdminLogin)

		// 用户认证
		api.POST("/user/login", authHandler.UserLogin)
		api.POST("/user/register", authHandler.RegisterUser)

		// 邀请码验证（公开）
		api.GET("/invites/:code/validate", inviteHandler.ValidateInvite)

		// ==================== 管理员路由（需要认证） ====================
		admin := api.Group("/admin")
		admin.Use(middleware.JWTAuth(), middleware.AdminOnly())
		{
			// 认证相关
			admin.GET("/current", authHandler.GetCurrentUser)
			admin.POST("/logout", authHandler.AdminLogout)

			// 用户管理
			admin.GET("/users", userHandler.GetUsers)
			admin.GET("/users/:id", userHandler.GetUserByID)
			admin.PUT("/users/:id/extend", userHandler.ExtendExpiry)
			admin.PUT("/users/:id/toggle", userHandler.ToggleUserStatus)
			admin.PUT("/users/:id/reset-password", userHandler.ResetPassword)
			admin.DELETE("/users/:id", userHandler.DeleteUser)

			// 邀请码管理
			admin.GET("/invites", inviteHandler.GetInvites)
			admin.POST("/invites", inviteHandler.CreateInvite)
			admin.DELETE("/invites/:id", inviteHandler.DeleteInvite)

			// 订阅管理
			admin.GET("/subscriptions", subscriptionHandler.GetAllSubscriptions)
			admin.PUT("/subscriptions/:id/approve", subscriptionHandler.ApproveSubscription)
			admin.PUT("/subscriptions/:id/reject", subscriptionHandler.RejectSubscription)
		}

		// ==================== 用户路由（需要认证） ====================
		user := api.Group("/user")
		user.Use(middleware.JWTAuth(), middleware.UserOnly())
		{
			// 认证相关
			user.POST("/logout", authHandler.UserLogout)

			// 个人信息
			user.GET("/profile", userHandler.GetProfile)
			user.PUT("/profile", userHandler.UpdateProfile)
			user.PUT("/password", userHandler.UpdatePassword)
			user.PUT("/email", userHandler.UpdateEmail)

			// 订阅管理
			user.GET("/subscriptions", subscriptionHandler.GetMySubscriptions)
			user.POST("/subscriptions", subscriptionHandler.CreateSubscription)
			user.DELETE("/subscriptions/:id", subscriptionHandler.DeleteSubscription)
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
