# Ember 前端构建优化规范

这份文档只讲 Ember Web 的构建体积、依赖注册和 chunk 切分约束。

它不负责视觉规范，不负责页面交互，也不记录一次性的排障流水账。遇到按钮、表单、筛选区样式问题，去看 [Web 设计规范](./web-design-guide.md)；遇到 chunk 过大、首包过重、依赖注册失控，先看这里。

## 1. 核心原则

- 先修入口依赖边界，再谈分包策略。
- 路由懒加载只能拆页面，不能掩盖 `main.ts` 里错误的全局重依赖注册。
- `manualChunks` 的作用是重建缓存边界，不是凭空减少字节数。
- 所有构建优化都必须用 `npm run build` 前后结果验证，不接受“理论上应该更小”。

## 2. 当前默认做法

### 2.1 Element Plus 注册入口

- `Element Plus` 的统一注册入口是 [services/web/src/plugins/element-plus.ts](../../services/web/src/plugins/element-plus.ts)。
- 应用入口 [services/web/src/main.ts](../../services/web/src/main.ts) 只负责调用 `registerElementPlus(app)`，不再直接 `app.use(ElementPlus)`。
- 新增 `Element Plus` 组件、指令或配套样式时，统一在 `src/plugins/element-plus.ts` 收口，不要把注册逻辑散落到页面或入口文件。

### 2.2 图标策略

- 禁止在入口全局注册 `@element-plus/icons-vue` 全量图标。
- 页面或组件只按需导入自己实际使用的图标。
- 如果一个图标只在单个页面出现，就让它留在页面内部，不要为了省一行 import 把整包拖回首入口。

### 2.3 样式策略

- 禁止恢复 `import 'element-plus/dist/index.css'` 这种整包样式入口。
- `Element Plus` 样式跟随组件按需引入，统一在 `src/plugins/element-plus.ts` 维护。
- 如果新增组件但漏掉样式，补局部样式导入，不要因为省事回退到整包 CSS。

## 3. Vite 分包约束

### 3.1 当前分包边界

- [services/web/vite.config.ts](../../services/web/vite.config.ts) 中的 `manualChunks` 是当前默认策略。
- `vue`、`vue-router`、`pinia` 和 `@vue/*` 归入 `vue-vendor`。
- `element-plus` 及其直接配套依赖归入 `element-plus-vendor`。
- `axios` 独立为 `network-vendor`。

### 3.2 分包规则

- 只有共享度高、更新频率低、缓存收益明确的依赖，才值得单独切成 vendor chunk。
- 不要为了“看起来更专业”硬拆很多小 chunk。过度切分只会增加请求和维护成本。
- 如果某个 chunk 依然过大，先检查是不是入口注册范围太宽，而不是先继续堆更多 `manualChunks` 规则。

## 4. 开发约束

- 不要在 `main.ts` 注入与所有页面无关的重型依赖。
- `public` 页面和 `console` 页面共享依赖时，优先问一句：这个依赖是否真的需要首屏加载。
- 新增第三方库时，先判断它是页面级依赖、共享业务依赖，还是应该进入 vendor chunk。
- 若只是某个后台页面使用的能力，不要把它升级成全局依赖。

## 5. 排查顺序

当前前端构建再次出现大 chunk 或首包异常时，按下面顺序排查：

1. 先跑 `npm run build`，看真实产物体积，不看感觉。
2. 检查 [services/web/src/main.ts](../../services/web/src/main.ts) 是否重新引入了整包插件、整包样式或全局图标注册。
3. 检查 [services/web/src/plugins/element-plus.ts](../../services/web/src/plugins/element-plus.ts) 是否出现了明显超范围注册。
4. 检查路由级懒加载是否被绕过，尤其是公共布局、共享组件和入口初始化逻辑。
5. 最后才调整 [services/web/vite.config.ts](../../services/web/vite.config.ts) 的 `manualChunks`。

## 6. 验证要求

- 每次构建优化后，必须重新执行 `npm run build`。
- 至少记录以下结果：
  - 最大 JS chunk 体积
  - 最大 CSS chunk 体积
  - 是否仍有 Vite 的 chunk size warning
- 如果改动影响了依赖注册方式，必须额外确认页面没有因为漏注册组件、指令或样式而出现运行期缺失。

## 7. 不该做的事

- 不要通过提高 `chunkSizeWarningLimit` 来假装问题消失。
- 不要把 `manualChunks` 当成唯一优化手段。
- 不要把一次性分析过程、临时对比数字、情绪化结论塞进这份文档。
- 不要把构建策略写进 [Web 设计规范](./web-design-guide.md)，那是另一类约束。
