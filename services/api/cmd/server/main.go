package main

import (
	"log"

	apppkg "github.com/konghang/ember/backend/internal/app"
	"github.com/konghang/ember/backend/internal/common"
	"github.com/konghang/ember/backend/internal/db"
)

func main() {
	// 初始化数据库
	db.InitDB()
	defer db.Close()

	// 初始化 JWT
	if err := common.InitJWT(); err != nil {
		log.Fatalf("❌ JWT 初始化失败：%v", err)
	}

	if err := apppkg.Start(); err != nil {
		log.Fatalf("❌ 服务器启动失败：%v", err)
	}
}
