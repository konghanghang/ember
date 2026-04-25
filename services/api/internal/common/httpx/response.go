package httpx

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// InternalError 把内部 / 上游错误统一回写为对客户端不暴露细节的 500 响应。
// 完整 err 仅记录在服务端日志中，附带 requestId（若 gin 上下文里设置过）。
//
// 客户端响应固定为 {"error": "上游服务暂不可用"}，避免把 wrap 链中的 URL / api_key / 上游响应体外泄。
func InternalError(c *gin.Context, err error) {
	requestID := ""
	if c != nil {
		if value, ok := c.Get("requestId"); ok {
			if rid, ok := value.(string); ok {
				requestID = strings.TrimSpace(rid)
			}
		}
		if requestID == "" {
			requestID = strings.TrimSpace(c.GetHeader("X-Request-Id"))
		}
	}

	if err != nil {
		if requestID != "" {
			log.Printf("[Internal] requestId=%s err=%v", requestID, err)
		} else {
			log.Printf("[Internal] err=%v", err)
		}
	}

	if c != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "上游服务暂不可用"})
	}
}
