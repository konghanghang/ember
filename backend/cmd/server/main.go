package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/db"
)

func main() {
	// 初始化数据库
	db.InitDB()
	defer db.Close()

	// 创建 Gin 路由
	r := gin.Default()

	// 健康检查端点
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"message": "Ember Go Backend is running",
		})
	})

	// API 路由组
	api := r.Group("/api/v1")
	{
		// 管理员路由
		admin := api.Group("/admin")
		{
			admin.POST("/login", func(c *gin.Context) {
				c.JSON(200, gin.H{"message": "登录端点（待实现）"})
			})
		}

		// 用户路由
		user := api.Group("/user")
		{
			user.GET("/profile", func(c *gin.Context) {
				c.JSON(200, gin.H{"message": "用户资料端点（待实现）"})
			})
		}
	}

	// 启动服务器
	port := os.Getenv("PORT")
	if port == "" {
		port = "3001"
	}

	log.Printf("🚀 服务器启动在端口 %s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("❌ 服务器启动失败：%v", err)
	}
}
