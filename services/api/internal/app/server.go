package app

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
)

func Start() error {
	r := gin.Default()

	registerRoutes(r, newAppHandlers())
	defer initCronJobs()()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("🚀 Ember Go API 服务启动")
	log.Printf("📍 端口: %s", port)
	log.Printf("📚 API 文档: http://localhost:%s/health", port)
	log.Printf("✅ JWT 认证已启用")

	return r.Run(":" + port)
}
