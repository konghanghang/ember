# ==================== 第一阶段：构建 ====================
FROM python:3.11-slim AS builder

WORKDIR /app

# 安装依赖
COPY requirements.txt .
RUN pip install --no-cache-dir --user -r requirements.txt

# ==================== 第二阶段：运行 ====================
FROM python:3.11-slim

WORKDIR /app

# 安装运行时依赖
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates curl && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# 创建非 root 用户
RUN groupadd -r ember && useradd -r -g ember ember

# 从构建阶段复制依赖
COPY --from=builder /root/.local /home/ember/.local

# 复制应用代码
COPY . .

# 设置 Python PATH
ENV PATH=/home/ember/.local/bin:$PATH \
    TZ=Asia/Shanghai \
    PYTHONUNBUFFERED=1

# 创建日志目录并修改所有权
RUN mkdir -p /app/logs && chown -R ember:ember /app

# 切换到非 root 用户
USER ember

# 暴露端口
EXPOSE 8000

# 健康检查
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8000/health || exit 1

# 启动应用
CMD ["python", "main.py"]
