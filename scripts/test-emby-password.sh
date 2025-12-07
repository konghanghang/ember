#!/bin/bash

# Emby 密码设置测试脚本

echo "========================================="
echo "测试 Emby API 创建用户并设置密码"
echo "========================================="

# 从 .env 读取配置
source .env

EMBY_URL="${EMBY_URL}"
EMBY_API_KEY="${EMBY_API_KEY}"

echo ""
echo "1. 测试创建用户（带密码）"
echo "-------------------------------------------"

# 创建测试用户
RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "${EMBY_URL}/Users/New" \
  -H "X-Emby-Token: ${EMBY_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "Name": "test_password_user",
    "Password": "test123456"
  }')

HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
BODY=$(echo "$RESPONSE" | sed '$d')

echo "HTTP 状态码: $HTTP_CODE"
echo "响应内容:"
echo "$BODY" | jq '.'

if [ "$HTTP_CODE" = "200" ]; then
  USER_ID=$(echo "$BODY" | jq -r '.Id')
  echo ""
  echo "✅ 用户创建成功，User ID: $USER_ID"

  echo ""
  echo "2. 查看用户详情（检查 HasPassword 字段）"
  echo "-------------------------------------------"

  USER_DETAIL=$(curl -s "${EMBY_URL}/Users/${USER_ID}" \
    -H "X-Emby-Token: ${EMBY_API_KEY}")

  echo "$USER_DETAIL" | jq '{Id, Name, HasPassword, HasConfiguredPassword}'

  HAS_PASSWORD=$(echo "$USER_DETAIL" | jq -r '.HasPassword')

  if [ "$HAS_PASSWORD" = "true" ]; then
    echo ""
    echo "✅ 密码已设置（HasPassword = true）"
  else
    echo ""
    echo "❌ 密码未设置（HasPassword = false）"
    echo ""
    echo "3. 尝试使用 POST /Users/{userId}/Password 设置密码"
    echo "-------------------------------------------"

    curl -X POST "${EMBY_URL}/Users/${USER_ID}/Password" \
      -H "X-Emby-Token: ${EMBY_API_KEY}" \
      -H "Content-Type: application/json" \
      -d '{
        "Id": "'$USER_ID'",
        "NewPw": "test123456"
      }'

    echo ""
    echo "密码设置完成，请在 Emby 中验证"
  fi

  echo ""
  echo "4. 清理测试用户"
  echo "-------------------------------------------"
  read -p "是否删除测试用户? (y/n): " -n 1 -r
  echo
  if [[ $REPLY =~ ^[Yy]$ ]]; then
    curl -X DELETE "${EMBY_URL}/Users/${USER_ID}" \
      -H "X-Emby-Token: ${EMBY_API_KEY}"
    echo "✅ 测试用户已删除"
  else
    echo "⚠️ 测试用户保留: $USER_ID"
  fi
else
  echo "❌ 创建用户失败"
fi

echo ""
echo "========================================="
echo "测试完成"
echo "========================================="
