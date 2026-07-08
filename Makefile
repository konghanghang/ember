# Ember 项目 Makefile

COMPOSE_FILE := infrastructure/docker/docker-compose.yml
API_DIR := services/api
WEB_DIR := services/web
BOT_DIR := services/bot
PYTHON ?= python3.11

.PHONY: help init setup dev-api dev-web dev-bot build-api build-web build \
	docker-build docker-up docker-down docker-logs docker-logs-api \
	test-api test-web test-bot test test-api-report test-web-report test-bot-report test-report \
	db-backup db-restore clean clean-deps clean-test-artifacts \
	clean-docker fmt-api info

help: ## 显示帮助信息
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

init: ## 初始化必要目录和本地环境文件
	@echo "🚀 初始化 Ember 项目..."
	@mkdir -p $(API_DIR)/logs $(API_DIR)/bin
	@mkdir -p $(BOT_DIR)/logs
	@mkdir -p infrastructure/nginx/logs infrastructure/nginx/ssl
	@mkdir -p infrastructure/database/backups
	@if [ ! -f infrastructure/docker/.env ]; then cp infrastructure/docker/.env.example infrastructure/docker/.env && echo "📝 已创建 infrastructure/docker/.env"; fi
	@if [ ! -f $(API_DIR)/.env ]; then cp $(API_DIR)/.env.example $(API_DIR)/.env && echo "📝 已创建 $(API_DIR)/.env"; fi
	@if [ -f $(BOT_DIR)/.env.example ] && [ ! -f $(BOT_DIR)/.env ]; then cp $(BOT_DIR)/.env.example $(BOT_DIR)/.env && echo "📝 已创建 $(BOT_DIR)/.env"; fi
	@echo "✅ 初始化完成"

setup: ## 安装 API / Web / Bot 依赖
	@echo "📦 安装 Go 依赖..."
	@cd $(API_DIR) && go mod download
	@echo "📦 安装 Web 依赖..."
	@cd $(WEB_DIR) && npm ci
	@echo "📦 安装 Bot 依赖..."
	@if [ ! -x $(BOT_DIR)/.venv/bin/python ]; then \
		echo "⚠️  Bot 虚拟环境不存在，请先执行: cd $(BOT_DIR) && $(PYTHON) -m venv .venv"; \
		exit 1; \
	fi
	@cd $(BOT_DIR) && .venv/bin/python -m pip install -r requirements-dev.txt
	@echo "✅ 依赖安装完成"

# ==================== 开发模式 ====================

dev-api: ## 启动 Go API 服务（手动开发用）
	@echo "🚀 启动 API 服务..."
	@cd $(API_DIR) && go run cmd/server/main.go

dev-web: ## 启动 Vue 前端（手动开发用）
	@echo "🚀 启动 Web 前端..."
	@cd $(WEB_DIR) && npm run dev

dev-bot: ## 启动 Python Bot（手动开发用）
	@echo "🚀 启动 Telegram Bot..."
	@cd $(BOT_DIR) && $(PYTHON) main.py

# ==================== 构建 ====================

build-api: ## 构建 Go API
	@echo "🔨 构建 API 服务..."
	@mkdir -p $(API_DIR)/bin
	@cd $(API_DIR) && go build -o bin/ember cmd/server/main.go
	@echo "✅ API 构建完成: $(API_DIR)/bin/ember"

build-web: ## 构建 Vue 前端
	@echo "🔨 构建 Web 前端..."
	@cd $(WEB_DIR) && npm run build
	@echo "✅ Web 构建完成"

build: build-api build-web ## 构建当前有产物的服务
	@echo "✅ 所有可构建服务已完成"

# ==================== Docker ====================

docker-build: ## 构建 Docker 镜像
	@echo "🐳 构建 Docker 镜像..."
	@docker compose -f $(COMPOSE_FILE) build

docker-up: ## 启动 Docker 服务
	@echo "🚀 启动 Docker 服务..."
	@docker compose -f $(COMPOSE_FILE) up -d

docker-down: ## 停止 Docker 服务
	@echo "🛑 停止 Docker 服务..."
	@docker compose -f $(COMPOSE_FILE) down

docker-logs: ## 查看 Docker 日志
	@docker compose -f $(COMPOSE_FILE) logs -f

docker-logs-api: ## 查看 API 日志
	@docker compose -f $(COMPOSE_FILE) logs -f ember-api

# ==================== 测试 ====================

test-api: ## 运行 Go API 测试
	@echo "🧪 运行 API 测试..."
	@cd $(API_DIR) && go test -v -race ./...

test-api-report: ## 运行 API 测试并输出结果产物
	@echo "🧪 运行 API 测试并生成报告..."
	@bash scripts/test/api.sh

test-web: ## 运行 Web 测试
	@echo "🧪 运行 Web 测试..."
	@cd $(WEB_DIR) && npm run test

test-web-report: ## 运行 Web 测试并输出结果产物
	@echo "🧪 运行 Web 测试并生成报告..."
	@bash scripts/test/web.sh

test-bot: ## 运行 Bot 测试
	@echo "🧪 运行 Bot 测试..."
	@if [ ! -x $(BOT_DIR)/.venv/bin/python ]; then \
		echo "⚠️  Bot 虚拟环境不存在，请先执行: cd $(BOT_DIR) && $(PYTHON) -m venv .venv"; \
		exit 1; \
	fi
	@cd $(BOT_DIR) && .venv/bin/python -m py_compile main.py && .venv/bin/python -m pytest tests

test-bot-report: ## 运行 Bot 测试并输出结果产物
	@echo "🧪 运行 Bot 测试并生成报告..."
	@bash scripts/test/bot.sh

test: test-api test-web test-bot ## 运行所有测试
	@echo "✅ 所有测试执行完成"

test-report: ## 运行 API / Web / Bot 并统一输出结果产物
	@echo "🧪 运行全量测试并生成报告..."
	@bash scripts/test/all.sh
	@echo "✅ 测试结果已写入 artifacts/test-results"

# ==================== 数据库 ====================

db-backup: ## 备份数据库
	@echo "💾 备份数据库..."
	@docker compose -f $(COMPOSE_FILE) exec postgres \
		pg_dump -U postgres ember > infrastructure/database/backups/backup_$(shell date +%Y%m%d_%H%M%S).sql
	@echo "✅ 备份完成"

db-restore: ## 恢复数据库（需要指定 FILE=xxx.sql）
	@echo "📥 恢复数据库..."
	@docker compose -f $(COMPOSE_FILE) exec -T postgres \
		psql -U postgres ember < $(FILE)
	@echo "✅ 恢复完成"

# ==================== 清理 ====================

clean: ## 清理构建产物和日志
	@echo "🧹 清理构建产物..."
	@rm -rf $(API_DIR)/bin
	@rm -rf $(API_DIR)/logs/*
	@rm -rf $(WEB_DIR)/dist
	@rm -rf $(BOT_DIR)/logs/*
	@find $(BOT_DIR) -type d -name "__pycache__" -prune -exec rm -rf {} +
	@echo "✅ 清理完成"

clean-test-artifacts: ## 清理统一测试结果产物
	@echo "🧹 清理测试结果产物..."
	@find artifacts/test-results -mindepth 1 ! -name '.gitkeep' -exec rm -rf {} +
	@echo "✅ 测试结果产物已清理"

clean-deps: ## 清理依赖目录（node_modules / .venv）
	@echo "🧹 清理依赖目录..."
	@rm -rf $(WEB_DIR)/node_modules
	@rm -rf $(BOT_DIR)/.venv
	@echo "✅ 依赖目录清理完成"

clean-docker: ## 清理 Docker 容器和卷
	@echo "🧹 清理 Docker..."
	@docker compose -f $(COMPOSE_FILE) down -v
	@echo "✅ Docker 清理完成"

# ==================== 格式化 ====================

fmt-api: ## 格式化 Go 代码
	@echo "✨ 格式化 Go 代码..."
	@cd $(API_DIR) && go fmt ./...
	@echo "✅ Go 代码格式化完成"

# ==================== 项目信息 ====================

info: ## 显示项目信息
	@echo "📊 Ember 项目信息"
	@echo "================================"
	@echo "项目名称: Ember"
	@echo "架构: Monorepo（微服务）"
	@echo "服务列表:"
	@echo "  - $(API_DIR)     (Go + Gin + GORM)"
	@echo "  - $(WEB_DIR)     (Vue 3 + Vite)"
	@echo "  - $(BOT_DIR)     (Python + Telegram Bot)"
	@echo "================================"
	@echo "使用 'make help' 查看所有命令"
