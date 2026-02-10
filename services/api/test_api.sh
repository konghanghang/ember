#!/bin/bash

# API 测试脚本

API_BASE="http://localhost:8080/api/v1"

echo "🧪 Ember Go API 测试"
echo "================================"

# 1. 健康检查
echo -e "\n1️⃣ 健康检查"
curl -s http://localhost:8080/health | jq .

# 2. 管理员登录（需要数据库中有管理员）
echo -e "\n2️⃣ 管理员登录"
TOKEN=$(curl -s -X POST "$API_BASE/admin/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}' | jq -r '.token')

if [ "$TOKEN" != "null" ] && [ -n "$TOKEN" ]; then
  echo "✅ 登录成功，Token: ${TOKEN:0:20}..."

  # 3. 获取当前用户
  echo -e "\n3️⃣ 获取当前用户信息"
  curl -s "$API_BASE/admin/current" \
    -H "Authorization: Bearer $TOKEN" | jq .

  # 4. 获取用户列表
  echo -e "\n4️⃣ 获取用户列表"
  curl -s "$API_BASE/admin/users?page=1&pageSize=10" \
    -H "Authorization: Bearer $TOKEN" | jq .

  # 5. 获取邀请码列表
  echo -e "\n5️⃣ 获取邀请码列表"
  curl -s "$API_BASE/admin/invites?page=1&pageSize=10" \
    -H "Authorization: Bearer $TOKEN" | jq .

  # 6. 创建邀请码
  echo -e "\n6️⃣ 创建邀请码"
  curl -s -X POST "$API_BASE/admin/invites" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"maxUses":5,"defaultDays":30}' | jq .

else
  echo "❌ 登录失败，请检查数据库中是否有管理员账号"
  echo "提示：可以运行 Next.js 项目创建管理员账号"
fi

echo -e "\n================================"
echo "✅ 测试完成"
