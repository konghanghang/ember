# Release Checklist

发布新版本前请确认：

## ✅ 发布前检查

- [ ] 所有功能已在 `master` 分支合并
- [ ] 本地代码已更新 `git pull origin master`
- [ ] CI 检查全部通过（查看 GitHub Actions）
- [ ] 本地测试通过 `npm run build && npm run lint`
- [ ] 更新了相关文档
- [ ] 已提交 `docs/releases/<version>.md`，文件名与即将发布的 tag 完全一致

## 📦 版本号选择

根据 [语义化版本](https://semver.org/lang/zh-CN/) 规范：

- **主版本（v2.0.0）** - 破坏性更新，不兼容旧版本
- **次版本（v1.1.0）** - 新增功能，向后兼容
- **修订版本（v1.0.1）** - Bug 修复，向后兼容

## 🚀 发布命令

```bash
# 创建并推送 tag
git tag v1.0.0
git push origin v1.0.0

# GitHub Actions 会自动：
# 1. 构建 Docker 镜像
# 2. 推送到 ghcr.io
# 3. 读取 docs/releases/v1.0.0.md
# 4. 创建 GitHub Draft Release
```

## 📝 发布后验证

- [ ] GitHub Release 已创建
- [ ] Docker 镜像已推送到 ghcr.io
- [ ] Release Notes 内容与 `docs/releases/<version>.md` 一致
- [ ] 可以拉取新镜像 `docker pull ghcr.io/konghanghang/ember:v1.0.0`

## 🐛 如果发布失败

1. 查看 GitHub Actions 日志
2. 修复问题后删除 tag 重新发布：
   ```bash
   git tag -d v1.0.0
   git push origin :refs/tags/v1.0.0
   git tag v1.0.0
   git push origin v1.0.0
   ```
