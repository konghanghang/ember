# Ember 项目 Makefile
# 参考 nextnewep 架构，统一管理所有服务

.PHONY: help init setup run stop clean test

help: ## 显示帮助信息
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

init: ## 初始化项目
	@echo "🚀 初始化 Ember 项目..."
	@echo "📁 检查目录结构..."
	@mkdir -p services/api/logs services/api/bin
	@mkdir -p services/web/dist
	@mkdir -p services/bot/logs
	@mkdir -p infrastructure/nginx/logs infrastructure/nginx/ssl
	@mkdir -p infrastructure/database/backups
	@echo "✅ 目录结构就绪"
	@if [ ! -f .env ]; then cp .env.example .env && echo "📝 已创建 .env 文件，请编辑配置"; fi
	@if [ ! -f services/api/.env ]; then cp services/api/.env.example services/api/.env && echo "📝 已创建 API .env 文件"; fi

setup: ## 安装所有依赖
	@echo "📦 安装依赖..."
	@echo "🚀 安装 Go 依赖..."
	@cd services/api && go mod download
	@echo "📦 安装 Vue.js 依赖（待实现）..."
	# @cd services/web && npm install
	@echo "🐍 安装 Python 依赖（待实现）..."
	# @cd services/bot && pip install -r requirements.txt
	@echo "✅ 依赖安装完成"

# ==================== 开发模式 ====================

dev-api: ## 启动 Go API 服务（开发模式）
	@echo "🚀 启动 API 服务..."
	@cd services/api && go run cmd/server/main.go

dev-web: ## 启动 Vue 前端（开发模式，待实现）
	@echo "🚀 启动 Web 前端..."
	# @cd services/web && npm run dev

dev-bot: ## 启动 Python Bot（开发模式，待实现）
	@echo "🚀 启动 Telegram Bot..."
	# @cd services/bot && python main.py

# ==================== 构建 ====================

build-api: ## 构建 Go API
	@echo "🔨 构建 API 服务..."
	@cd services/api && go build -o bin/ember cmd/server/main.go
	@echo "✅ API 构建完成: services/api/bin/ember"

build-web: ## 构建 Vue 前端（待实现）
	@echo "🔨 构建 Web 前端..."
	# @cd services/web && npm run build

build: build-api ## 构建所有服务
	@echo "✅ 所有服务构建完成"

# ==================== Docker ====================

docker-build: ## 构建 Docker 镜像
	@echo "🐳 构建 Docker 镜像..."
	@docker-compose -f infrastructure/docker/docker-compose.yml build

docker-up: ## 启动 Docker 服务
	@echo "🚀 启动 Docker 服务..."
	@docker-compose -f infrastructure/docker/docker-compose.yml up -d

docker-down: ## 停止 Docker 服务
	@echo "🛑 停止 Docker 服务..."
	@docker-compose -f infrastructure/docker/docker-compose.yml down

docker-logs: ## 查看 Docker 日志
	@docker-compose -f infrastructure/docker/docker-compose.yml logs -f

docker-logs-api: ## 查看 API 日志
	@docker-compose -f infrastructure/docker/docker-compose.yml logs -f api

# ==================== 测试 ====================

test-api: ## 运行 Go API 测试
	@echo "🧪 运行 API 测试..."
	@cd services/api && go test -v -race ./...

test-health: ## 测试健康检查端点
	@echo "🔍 测试健康检查..."
	@curl -s http://localhost:8080/health | python3 -m json.tool || echo "⚠️ 服务未启动"

# ==================== 数据库 ====================

db-backup: ## 备份数据库
	@echo "💾 备份数据库..."
	@docker-compose -f infrastructure/docker/docker-compose.yml exec postgres \
		pg_dump -U postgres ember > infrastructure/database/backups/backup_$(shell date +%Y%m%d_%H%M%S).sql
	@echo "✅ 备份完成"

db-restore: ## 恢复数据库（需要指定 FILE=xxx.sql）
	@echo "📥 恢复数据库..."
	@docker-compose -f infrastructure/docker/docker-compose.yml exec -T postgres \
		psql -U postgres ember < $(FILE)
	@echo "✅ 恢复完成"

# ==================== 清理 ====================

clean: ## 清理项目
	@echo "🧹 清理项目..."
	@rm -rf services/api/bin
	@rm -rf services/api/logs/*
	@rm -rf services/web/dist
	@rm -rf services/web/node_modules
	@rm -rf services/bot/__pycache__
	@echo "✅ 清理完成"

clean-docker: ## 清理 Docker 容器和卷
	@echo "🧹 清理 Docker..."
	@docker-compose -f infrastructure/docker/docker-compose.yml down -v
	@echo "✅ Docker 清理完成"

# ==================== 格式化 ====================

fmt-api: ## 格式化 Go 代码
	@echo "✨ 格式化 Go 代码..."
	@cd services/api && go fmt ./...
	@echo "✅ Go 代码格式化完成"

# ==================== 项目信息 ====================

info: ## 显示项目信息
	@echo "📊 Ember 项目信息"
	@echo "================================"
	@echo "项目名称: Ember"
	@echo "架构: Monorepo（微服务）"
	@echo "服务列表:"
	@echo "  - services/api     (Go + Gin + GORM)"
	@echo "  - services/web     (Vue 3 + Vite)   [待实现]"
	@echo "  - services/bot     (Python + Telegram Bot) [待实现]"
	@echo "================================"
	@echo "使用 'make help' 查看所有命令"
