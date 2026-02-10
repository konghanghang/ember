package models

import (
	"crypto/rand"
	"fmt"
	"time"
)

// generateCUID 生成兼容 Prisma 的 cuid
// 这是一个简化实现，生产环境应该使用 github.com/lucsky/cuid
func generateCUID() string {
	// cuid 格式：c + timestamp + counter + fingerprint + random
	// 示例：cl9k3l8w40001qg5k5g5k5g5k

	// 简化版本：使用时间戳 + 随机字符
	timestamp := time.Now().UnixNano()
	randomBytes := make([]byte, 8)
	rand.Read(randomBytes)

	return fmt.Sprintf("cl%x%x", timestamp, randomBytes)[:25] // 限制为25字符
}
