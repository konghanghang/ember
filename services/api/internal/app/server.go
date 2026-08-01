package app

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	logpkg "github.com/konghang/ember/backend/internal/logging"
)

func Start() error {
	r := gin.New()
	r.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		Output: logpkg.Writer(),
		SkipPaths: []string{
			"/",
			"/health",
		},
	}))
	r.Use(gin.Recovery())

	handlers, err := newAppHandlers()
	if err != nil {
		return err
	}
	registerRoutes(r, handlers)
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
