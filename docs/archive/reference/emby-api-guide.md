# Emby API 开发指南（历史归档）

> 本文档是早期接入 Emby 时保留下来的开发笔记，包含手工验证顺序、探索性代码样例和当时的排查思路。
> 它不再代表 Ember 当前的稳定实现规范，仅保留追溯价值。

> **创建日期**: 2025-12-06
> **用途**: Day 3 开发 Emby API 客户端时参考

---

## 📚 Emby API 文档

### 官方 Swagger 文档

每个 Emby 服务器都有内置的 API 文档：

```
https://your-emby-server/swagger/index.html
```

访问你的 Emby 服务器的 `/swagger` 路径查看完整 API。

---

## 🔑 核心 API 端点

### 我们需要使用的 API

```
POST   /Users/New                    # 创建用户
DELETE /Users/{userId}               # 删除用户
POST   /Users/{userId}/Policy        # 更新用户权限
GET    /Users                        # 获取用户列表
GET    /Users/{userId}               # 获取用户详情
```

### 认证方式

所有请求需要在 Header 中包含：

```
X-Emby-Token: your-api-key
```

---

## 📖 GitHub 最佳实践参考

### 推荐参考项目

1. **embyboss** - 你要替代的项目
   - 学习如何调用 Emby API
   - 参考错误处理方式

2. **Wizarr** - UI 参考
   - 可能包含 Emby 集成代码

3. **jfa-go** - Jellyfin 管理工具
   - Jellyfin API 和 Emby API 类似
   - 参考认证和错误处理

### GitHub 搜索策略

**搜索关键词**：
```
"emby api" "typescript"
"emby server" "user management"
"emby api client" "nodejs"
"X-Emby-Token"
```

**使用 mcp__grep__searchGitHub 工具**：
```typescript
// 搜索 Emby API 使用示例
{
  query: "X-Emby-Token",
  language: ["TypeScript", "JavaScript"]
}

// 搜索创建用户的实现
{
  query: "/Users/New",
  language: ["TypeScript", "JavaScript"]
}
```

---

## ✅ Day 3 开发顺序（重要）

### 正确的开发流程

**1. 先手动测试 Emby API**

使用 curl 或 Postman 验证 API 可用：

```bash
# 测试创建用户
curl -X POST "https://your-emby.com/Users/New" \
  -H "X-Emby-Token: your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "Name": "testuser",
    "Password": "test123"
  }'

# 测试获取用户列表
curl -X GET "https://your-emby.com/Users" \
  -H "X-Emby-Token: your-api-key"

# 测试删除用户
curl -X DELETE "https://your-emby.com/Users/{userId}" \
  -H "X-Emby-Token: your-api-key"
```

**2. 写最简单的代码**

```typescript
// lib/emby.ts - 第一版（最简单）
export class EmbyClient {
  private baseUrl: string
  private apiKey: string

  constructor() {
    this.baseUrl = process.env.EMBY_URL!
    this.apiKey = process.env.EMBY_API_KEY!
  }

  async createUser(username: string, password: string) {
    const res = await fetch(`${this.baseUrl}/Users/New`, {
      method: 'POST',
      headers: {
        'X-Emby-Token': this.apiKey,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        Name: username,
        Password: password
      })
    })

    if (!res.ok) {
      throw new Error(`Emby API error: ${res.statusText}`)
    }

    return await res.json()
  }
}
```

**3. 验证成功**

- 运行代码
- 在 Emby 后台查看是否创建了新用户
- 确认 API 行为符合预期

**4. 再加错误处理**

```typescript
// lib/emby.ts - 第二版（加重试机制）
async createUser(username: string, password: string) {
  // 重试 3 次
  for (let i = 0; i < 3; i++) {
    try {
      const res = await fetch(...)
      if (!res.ok) throw new Error(...)
      return await res.json()
    } catch (error) {
      if (i === 2) throw error
      await new Promise(r => setTimeout(r, 1000 * (i + 1)))
    }
  }
}
```

---

## ⚠️ 常见陷阱

### 1. 用户权限配置（Policy）很复杂

**错误做法**：
```typescript
// ❌ 只设置部分字段，可能导致权限丢失
await fetch(`/Users/${userId}/Policy`, {
  body: JSON.stringify({ IsDisabled: true })
})
```

**正确做法**：
```typescript
// ✅ 先获取完整 Policy，再修改
const policy = await this.getUserPolicy(userId)
policy.IsDisabled = true
await fetch(`/Users/${userId}/Policy`, {
  body: JSON.stringify(policy)
})
```

### 2. 不要假设 API 会按文档工作

- 先手动测试，验证实际行为
- 有些字段可能是必填的（文档没说）
- 有些操作需要管理员权限

### 3. 错误处理要充分

- Emby 服务器可能暂时不可用
- 网络可能超时
- API Key 可能失效

---

## 📋 Day 3 任务 3.1 检查清单

开发 Emby API 客户端时，按这个顺序：

- [ ] 1. 访问 Emby Swagger 文档，确认 API 端点
- [ ] 2. 用 curl 手动测试 `/Users/New`（创建用户）
- [ ] 3. 用 curl 手动测试 `/Users/{userId}`（删除用户）
- [ ] 4. 用 curl 手动测试 `/Users/{userId}/Policy`（禁用用户）
- [ ] 5. 在 GitHub 搜索 "X-Emby-Token" 的使用示例
- [ ] 6. 写最简单的 `createUser()` 方法
- [ ] 7. 测试创建用户成功
- [ ] 8. 加入重试机制
- [ ] 9. 实现其他方法（deleteUser、setUserPolicy）
- [ ] 10. 写测试脚本 `scripts/test-emby.ts`

---

## 🎯 核心原则

> **Linus 的建议**：先让最简单的情况工作，再处理复杂的。

**不要**：
- ❌ 一上来就写完整的 EmbyClient 类
- ❌ 没测试就开始写事务逻辑
- ❌ 假设 API 会按文档工作

**要**：
- ✅ 先用 curl 手动测试 Emby API
- ✅ 写最小的代码验证可行性
- ✅ 确认 API 行为后再封装

---

## 📝 常用 Emby API 示例

### 创建用户

```bash
curl -X POST "https://your-emby.com/Users/New" \
  -H "X-Emby-Token: your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "Name": "newuser",
    "Password": "password123"
  }'
```
