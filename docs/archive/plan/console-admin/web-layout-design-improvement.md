# 前端页面布局与设计收口方案

> 状态：已归档
> 负责人：Ember
> 更新时间：2026-08-30

## 背景

2026-07-26 对 `services/web` 全部页面做了一轮布局与设计评审（4 路并行，覆盖 26 个页面/组件，逐行对照 [docs/reference/web-design-guide.md](../../../reference/web-design-guide.md)，下称"规范"），结论：

- 基础组件体系在标准列表页落地良好：`UsersView`、`PlaybackCenterView`、`LoginView`、`Sidebar` 是四个贴规范的标杆。
- 拉低一致性的主要是三类问题：**中心页骨架分裂**（支付中心筛选消失/双标题、兑换中心标题双渲染）、**文案克制规则（§2.2.1）被系统性突破**（TopBar 骨架层 18 句 + 各页约 20 处）、**手写组件未回收**（分段控件/统计卡各三套写法）。
- 用户体感"布局太烂"的页面集中在：支付中心、兑换中心、续期中心、媒体缺口、TopBar 骨架。

本方案与 [前端工程质量收口方案](../../../plan/architecture/web-frontend-quality-improvement.md) 互补：那份管工程质量（类型检查、契约、工具函数、死资源），这份管布局与设计一致性。两份的"文案/组件回收"改动不重叠，可独立排期。

如果不收口：规范第 12 条检查清单在多个页面形同虚设；中心页两种骨架并存会让后续每个新中心页都要重新发明一次结构；支付记录筛选在生产环境不可达是真实功能缺失。

## 目标

1. 修复 5 个 P1 布局缺陷（TopBar 描述、支付中心骨架、兑换中心双标题、MediaGaps 视图切换、Navbar 无障碍）。
2. 按 §2.2.1 完成全站文案大扫除。
3. 回收三套并存的手写组件（分段控件、统计卡、日期范围框）到 Ember 基础组件。
4. 收敛一致性长尾（cursor-pointer、弹窗宽度、徽章色板、骨架缝隙）。

## 非目标

- 不重新设计任何页面的信息架构与视觉风格；只做"向现行规范收敛"，不发明新规范。
- 不处理工程质量项（类型检查、API 契约、死资源等），归 `architecture/web-frontend-quality-improvement.md`。
- TVCalendarView、首页重视觉模块属规范 3.4 明确允许的特例页，只修违规点（kicker 文案、空状态组件），不强行套列表骨架。
- MediaGapsView 聚合视图约 400 行自绘 CSS 本方案只做"收敛到基线"（tone、圆角、组件替换），不做整体重写；重写与否在组件回收完成后另行评估。

## 当前事实

- 设计基线：`docs/reference/web-design-guide.md`（2026 年现行版本，含 §2.2.1 文案克制、§3.4 基础组件使用规则、§4 列表页骨架、§12 检查清单）。
- 基础组件：`services/web/src/components/ember/`（EmberPageHeaderCard/EmberFilterPanel/EmberTableCard/EmberSearchInput/EmberSelectField/EmberDateField/EmberDateRangeField/EmberFormDialog/EmberSegmentTabs/EmberMetricCard/EmberEmptyStateCard + tokens.ts）。
- 全局兜底已存在：`base.css:362` 有 `prefers-reduced-motion` 全局降级；Tailwind v3 preflight 兜住原生 button 指针（但规范要求显式 `cursor-pointer`）。
- 中心页现状：播放中心、兑换中心把 tabs 注入子页 Header Card actions（规范模式）；支付中心自造头卡（违规模式）。
- 路由事实：支付中心、播放中心子页均无独立路由（`router/index.ts` 实证），子页的"非 embedded 分支"是死代码。

## 问题清单（评审实证）

### P1（5+1 个）

- **P1-1 TopBar 每页功能介绍式描述**：`components/console/TopBar.vue:29-98,157`。`routeMeta` 为约 18 个控制台路由各配一句描述（"维护求片请求与订阅记录""购买方案或兑换续期码"等）并渲染在页面标题下方，§2.2.1 在骨架层被系统性突破。
- **P1-2 支付中心骨架分裂**：
  - `PaymentsView.vue:181` `v-if="!props.embedded"` 把整页 Header Card（3 个筛选字段 + 查询/重置 + "当前仅查看用户 X"提示）全部隐藏；子页无独立路由，**筛选功能在生产环境永远不可达**，用户只剩一张裸表格。
  - `PlansView.vue:263-338` 嵌入时完整保留自己的 Header Card，与 `PaymentCenterView.vue:63-77` 自造头卡形成双标题双卡片；同一中心两个 tab 两种相反骨架。
  - `PaymentCenterView.vue:14-18,33-35` "套餐分组"作为 EmberSegmentTabs 分段项，点击却 `router.push` 跳离当前页——单选分段被当导航链接用。
  - `PaymentCenterView.vue:63-77` 头卡手写 `rounded-3xl border-slate-200`，未用 EmberPageHeaderCard，描述句"把方案配置和订单审计放在一起，支付工作流更完整。"是典型设计者视角文案。
- **P1-3 兑换中心标题描述双渲染**：`RedemptionCodesView.vue:329-330,364-373`。嵌入态下 EmberPageHeaderCard 的 title/description 已是"兑换码池"，默认插槽里又逐字渲染一遍；非嵌入态还有两个计数徽章并存（`:332-334` vs `:366`）。
- **P1-4 MediaGaps 视图切换手写分段**：`MediaGapsView.vue:948-965,1531-1554`。手写 `view-toggle-btn` 未用 EmberSegmentTabs，丢失 roving tabindex/方向键/Home/End 键盘交互，是全站第三种分段样式（另两种：EmberSegmentTabs 标准、UserPlaybackProfiles 红底预设按钮）。
- **P1-5 Navbar 移动菜单按钮无 aria-label**：`components/home/Navbar.vue:50`。纯 SVG 图标按钮无 `aria-label`/`sr-only`，违反 §10/§11 硬性条款；同按钮还缺 `cursor-pointer` 与 `aria-expanded`。
- **P1-6 续期中心兑换码标题整组重复**：`RenewalCenterView.vue:300-311`。外层卡"输入兑换码"+说明与内层灰底块"输入兑换码"+近义说明同屏出现两次。

### 系统性通病（跨页，按收益排序）

- **S1 文案克制全线失守（§2.2.1）**：除 P1-1 的 18 句外还有约 20 处：
  - 复述标题型 description：`SubscriptionsView.vue:665`、`NewSubscriptionView.vue:269`、`RenewalCenterView.vue:221,342`、`RankingsView.vue:338`、`MediaQualityView.vue:271,414`、`PlaybackHistoryView.vue:128`、`RedemptionHistoryView.vue:87`、`ProfileAnalyticsView.vue:188`
  - 设计者视角/功能介绍：`AccountCenterView.vue:778`（"查询账号、续期和常用指令"）、`SettingsView.vue:476-478,469,551-553`（英文眉题+"优先突出当前状态…"+"这里只保留…"）、`HelpWidget.vue:117,159`、`PlanGroupsView.vue:565,573`、`MediaGapsView.vue:944`、`UserPlaybackProfilesView.vue:277`、`DevicesView.vue:272-274`
  - 对数字/入口二次翻译：`PlaybackProfileContent.vue:171,178,187,263,299,333,368`（"有播放的天数越多，节奏越稳定""看看通常什么时候看片""避免信息噪音"等 7 句）
  - Dashboard 过期提示同屏 3 处重复（`DashboardView.vue:172-178,242-248,281-298`），"实时摘要"装饰徽章（`:304-306`）
- **S2 分段控件三套写法**：标准 EmberSegmentTabs（多数页）vs 手写红底按钮组（`PlaybackProfileContent.vue:118-128`、`UserPlaybackProfilesView.vue:301-313`）vs 手写白底红字（MediaGaps P1-4）；MediaGaps 排序 chips（`:1062-1070`）是第四处手写单选组。
- **S3 统计卡三套写法**：EmberMetricCard（Settings/Devices/ProfileAnalytics 已用）vs 手写（`DashboardView.vue:311-345`、`TVCalendarView.vue:328-346`、`PlanGroupsView.vue:547-575` 后两张）。
- **S4 `cursor-pointer` 缺失约 20 处**：`SubscriptionsView.vue:682,692,934`、`NewSubscriptionView.vue:296-299,419`、`RenewalCenterView.vue:284,385`、`RedemptionCodesView.vue:346,420-425,674,763,770`、`PlansView.vue:483-485,561-567`、`MediaGapsView.vue` 六类自定义按钮（view-toggle/sort-chip/episode-chip/season-action/series-expand/series-action）、`ForgotPasswordView.vue:148`、`HeroSection.vue:54`、`PlanGroupsView.vue` 11 处、`DevicesView.vue:395,452`、`MediaQualityView.vue:303-334` 三按钮。
- **S5 EmberTableCard 无 `#header`**：11 个使用页中 8 个缺标题（Users:855、Devices:421、Payments、Plans、PlaybackHistory、RedemptionCodes、RedemptionHistory、MediaGaps），与 §3.4"不接受无标题主表"整体冲突——要么统一补，要么回写规范放宽。
- **S6 弹窗宽度漂移**：560（`UsersView.vue:1176`、`SettingsView.vue:877`）、640（`PlanGroupsView.vue:938`）、720（`PlanGroupsView.vue:874`）、760（`MediaGapsView.vue:1380-1386`）、920（Users 同步预览，高度定制需说明原因），均不在 400/520/680 基线。
- **S7 状态徽章/tone 偏离基线**：Subscriptions 海报高饱和纯色块（`:459-468,747-817`，§8.2 唯一明显违规）；MediaGaps 自造 `missing/requested/settled/muted` tone 命名（`:163-199,391-404`，§3.5 点名反模式）；MediaGaps 纯黑 season-pill（`:1647-1656`）；"已过期"映射灰色 info（`RedemptionCodesView.vue:317`、`PaymentsView.vue:60`）；Sessions 播放方式 blue/orange（`:57-62`）；Plans 分组 tag 用颜色承担无文字语义（`:375`）；剧集 chip 仅靠底色表达 6 种状态（`MediaGapsView.vue:1134-1141`，颜色成为唯一状态表达）。
- **S8 骨架两条缝**：桌面侧边栏双右边框贴成 2px 双色缝（`Layout.vue:44` + `Sidebar.vue:147`）；品牌区 64px 与顶栏 72px 分界线错位 8px（`Sidebar.vue:149` vs `TopBar.vue:143`）。
- **S9 fadeIn 动画无全局定义**：LoginView(0.6s)/ForgotPassword(0.6s)/Dashboard(0.4s)/Settings(0.35s,`917-931`) 各抄一份；`SessionsView.vue:154` 引用的 `animate-fade-in` 是死类（全仓库无定义）。
- **S10 手写日期范围框复刻**：`PlaybackProfileContent.vue:130-149,410-468` 手写 el-date-picker + 约 60 行 scoped CSS 复刻 EmberDateRangeField 外观，且 `display:none` 隐藏清空图标但保留 `clearable`（无法图标清空）；`PlaybackProfileContent.vue:106-153` 手写头卡与 EmberPageHeaderCard 同构。
- **S11 时间格式绕过 utils/date**：`SubscriptionsView.vue:507-519`（formatCardDate/formatCompactDateTime）、`MediaGapsView.vue:270-292`（regex 手写）、`SessionsView.vue:91-97`（手拼 HH:mm:ss）、`TVCalendarView.vue:99-126`（周界函数）——与工程质量方案的 P2-8 日期项同源，修复时一并收口。
- **S12 徽章中英文混用**：`Total:`（Users:762、Devices:245、PlaybackHistory 非嵌入分支）vs "N 个分组/会话"（PlanGroups/Sessions）。

### 分页问题详情（P2/P3，未列入以上通病的）

**公开页与骨架**
- HeroSection `:33` "Ember v1.0 正式上线"硬编码，与当前发布线失实（P2）；`:54` 主 CTA 手写 `bg-ember rounded-lg` 非 `.btn-ember`（P2，Navbar `:44` 同型）
- Footer `:11-12` 隐私/条款 `href="#"` 死链（P2）；`:7` 容器 `max-w-4xl` 与同页 Navbar 1400px/Hero 7xl/Features 1100px 构成 4 套宽度（P2）
- RegisterView `:282` 主按钮未对齐登录页 `!h-12 !rounded-xl`（P2）；`:218,249-256` 次按钮 40px vs 输入框 42px 高差（P2）；`:208` 与登录/找回页双套 label 体系（P2）；`:286-288` 返回链接位置与版权行不一致（P2）
- NotFoundView `:2-11` 空状态裸放页面底色（P2，§8.3）；`:7` 按钮 `rounded-lg`（P3）
- MarqueeSection `:10-14` 循环复制组未 `aria-hidden`，读屏重复（P3）
- TopBar `:170-174` 会员状态点仅靠颜色表达（P3）

**控制台用户页**
- Dashboard `:156-168` 徽章"有效"与主值大字"有效"同义重复；`:192` 内层卡 `rounded-3xl` 偏离基线（P2/P3）
- AccountCenter `:826` Telegram 主按钮 `bg-sky-600` 破坏主色统一（P2）；`:721,729` Emby 摘要双处显示（P2）；`:596-681` 中文标签套 `uppercase tracking-[0.18em]` 纯装饰且不一致（P3）
- Subscriptions `:723-729,748-753` PENDING 角标与底部 chip 同义重复（P2）；`:866` 分页缺 jumper（P3）；弹窗 footer `rounded-lg`（P3）
- Rankings `:684-697` 时长同行显示两次（P2）；`:341-409` 页头一行最多 7 控件移动端占半屏（P2）；`:356` 日期框 class `w-full !w-full` 重复粗糙（P3）；`:647-648` kicker 与标题语义重复（P3）
- TVCalendar `:311` "Ember Weekly Watchboard" 英文装饰 kicker（P2）；`:417-433` 空状态手写未用 EmberEmptyStateCard 且说明对用户不可执行（P2）；`:581-595` CSS 类沿用 `ink/ready/today` 历史 tone 命名（P3）；`:318` 同步按钮 6 个 `!important`（P3）
- ProfileAnalytics/PlaybackProfileContent `:316 vs :350` 设备条 ember vs 客户端条 slate-700 双配色（P2）；`:233` 峰值柱 slate 深色渐变（P2）；`:279-281` 向用户展示内部标签 ID（P2）
- RenewalCenter `:220` 标题 `text-xl` 低于 `text-2xl` 基线（P2）；`:341` "历史记录"标题无字号（P2）；`:240,262` 说明句语义重复（P3）

**后台页**
- Users `:1249,1327` 弹窗日期选择器缺 `form-date`（P2）；`:1354-1361` 全仓库唯一表格行 hover 改浅红+删底边框（P2）；`:986` 下拉菜单 `w-40` 装不下长文案且项过多（P2）；`:870-873` Emby ID 空 chip 像脏数据（P3）
- PlanGroups `:771-775,833-837` 默认分组开关裸放（同页权益模板弹窗却用了容器，自相矛盾，P2）；`:724-740` tooltip/title 混用（P3）；`:150` 空状态映射"未知"（P3，后端行为未证实）
- Settings `:877-912` API Key 弹窗裸 el-dialog 未走 EmberFormDialog（P2）；`:510-514` tab 激活态 `bg-gray-900` 黑底（P2，检查清单第 12 条）；`:794-825` 开关两层/checkbox 三层盒中盒且带 ember 底色（P2）；`:563` 面板 `rounded-3xl`（P3）；`:481-484` 四张 EmberMetricCard 统一压 `text-lg` 绕开组件默认（P3）；风险信号四层重复（红点徽章+红横幅+行内红底+行徽标，`:572-729`，P3）
- Sessions `:156-159,184-188` 会话数徽章与 chips 行重复（P2）；`:174` 按钮 `font-bold` vs 基线 semibold（P3）
- Devices `:385,478` 黑名单表/日志表裸 el-table 与主表两种表头语言（P2）；`:346` 一键注销实心 `bg-red-600` 压过主按钮，危险操作全站三种深浅（P2）；`:248-255` 刷新+查询灰色地带（P3）；`:289-305` 下拉无 change 自动查询，与 Users 不一致（P3）；`:393-398` "移除"纯文本按钮点击目标过小（P3）；`:307-315` 筛选下拉无 icon 节奏不齐（P3）
- MediaQuality `:303-334` 筛选区按钮语义混乱："读取报告"冗余（change 已自动加载）、"强制刷新"语义重叠、主按钮给了"手动重扫"而非查询（P2）；`:384-406` 三张分布表手写 section 壳+裸 el-table（P2）；`:475` 抽屉 60% 定死移动端偏窄（P3）
- MediaGaps `:967-973,1035-1041` 刷新+查询并存（P2）；`:1264-1275` 聚合分页独立 `rounded-3xl` 卡（P2）；`:1253-1262,1496-1505` 两处空状态手写 dashed（P2）；`:1380-1386` 搜索弹窗裸 el-dialog 第三套 chrome（P2）
- Redemption `:345-352,426-433` 刷新+查询并存（P2）；`:694,697` 编辑弹窗缺 `form-number`（新建弹窗有，P2）；`:341-344` vs `PlansView.vue:321-327` "包含失效/包含下架"同类开关两种落点（P2）；`:783-786` scoped 表头样式与 EmberTableCard 内建重复（P3）；`:167` 延长天数用 success tag 偏装饰（P3）
- Plans `:321-327` 开关手写白底容器与筛选字段灰底不一致（P3）；`:553-555` 编辑弹窗开关裸露（P3）；`:264` 嵌入态标题"方案池"与 tab 名"付费方案"不一致（P3）
- PlaybackHistory/UserPlaybackProfiles：非 embedded 分支死代码（无独立路由）；徽章中英混杂（P3）
- HelpWidget FAB 无障碍属性齐全（合规标杆）；Sidebar 导航逐条符合 §7（合规标杆）

## 方案设计

### 修复批次

**批次 1：P1 快修（P1-1 ~ P1-6）**
- TopBar：删 `currentMeta.description` 渲染行，`routeMeta` 只留 title。
- 支付中心对齐播放/兑换模式：PaymentCenterView 删自造头卡、改向子页传 `#tabs`；PaymentsView 去掉 `v-if` 恢复 Header Card（筛选对生产用户恢复可见）；PlansView 嵌入态标题与 tab 名统一；"套餐分组"从分段项移出头卡次按钮或导航。
- RedemptionCodesView：删默认插槽重复标题/描述块（`:364-373`），计数徽章留一枚；顺带清理子页非 embedded 死分支。
- MediaGapsView：视图切换换 EmberSegmentTabs（图标现成）。
- Navbar：补 `aria-label="打开导航菜单"`、`:aria-expanded`、`cursor-pointer`。
- RenewalCenter：删内层灰底块标题行，只留图标+输入框+按钮+一句关键约束。

**批次 2：文案大扫除（S1）**
- 按问题清单 S1 逐条删除/压缩；规则：能通过标题/标签/按钮表达的全删，每区最多留一条状态确认/关键约束/风险提示。
- Dashboard 过期提示三处合并为一处（保留锁定空态+按钮）。
- 本批次只动文案，不动结构；完成后规范 §2.2.1 无需修改。

**批次 3：组件回收（S2、S3、S10、S11 + MediaGaps 收敛）**
- PlaybackProfileContent/UserPlaybackProfiles 预设按钮组 → EmberSegmentTabs；PlaybackProfileContent 日期范围框 → EmberDateRangeField（删 60 行复刻 CSS）；头卡 → EmberPageHeaderCard。
- Dashboard/TVCalendar/PlanGroups 手写统计卡 → EmberMetricCard（图标走 slot）。
- MediaGaps：compactStats tone 映射回 5 标准 tone、排序 chips 评估并入 SegmentTabs、空状态 → EmberEmptyStateCard、series 卡圆角/分页卡向基线收敛、搜索弹窗宽度 680 或写明特例原因、日期函数换 `@/utils/date`。
- Subscriptions 海报徽章：纯色块改 `bg-white/15 backdrop-blur` 中性底 + 状态点。
- 时间格式统一走 `utils/date.ts`（与工程质量方案批次 3 的日期项合并执行，避免两次触碰同一批文件）。

**批次 4：一致性长尾（S4、S5、S6、S7 余项、S8、S9、S12 + 分页 P2/P3）**
- cursor-pointer 全站补齐（约 20 处）。
- EmberTableCard `#header`：统一补标题；若评审后认为部分页面标题冗余，则回写规范放宽该条，二选一不留中间态。
- 弹窗宽度 560/640/720 → 520/680；920 同步预览弹窗在代码注释或规范写明特例原因。
- 危险按钮统一一族（浅底/描边），实心 `bg-red-600` 只保留给真正不可逆确认。
- 骨架两条缝：删 Layout 容器 `lg:border-r`；品牌区/顶栏高度对齐。
- fadeIn 收到全局 CSS 统一时长，删 SessionsView 死类。
- 徽章统一中文计数；Hero 版本徽章改中性文案或从构建信息注入；Footer 死链删除或接真实路由、容器宽度对齐。
- 注册页向登录页对齐（按钮高度/圆角、label 体系、返回链接位置、版权行）。

### 关键约束

- 所有改动以 `docs/reference/web-design-guide.md` 为唯一设计与交互基线；确需偏离时必须先说明原因、范围和收口条件，并回写规范的特例边界。
- Never break userspace：批次 1 的 PaymentsView 恢复筛选是"恢复本应可见的功能"，其余改动不得改变任何用户可见的交互行为（纯视觉/结构收敛）。
- 规范自身需要修订的项（S5 表头标题是否放宽、批次 4 危险按钮基线）必须先改规范再改代码，保持文档与实现一致。
- 涉及 Ember 基础组件契约改动的，必须同步检查全部调用点并补组件测试。
- 每批次完成后跑 `npm run test` 与 `npm run build`；有组件测试的页面（PlanGroups/Users/Settings 等 spec）必须同步更新。

## 影响范围

- API：无。
- Web：有。全部 `views/`、部分 `components/`、`assets/base.css`（fadeIn 全局化时）。
- Bot：无。
- 配置/部署：无。
- 文档：若 S5/危险按钮基线触发规范修订，同步 `docs/reference/web-design-guide.md`；批次全部完成后本方案归档。

## 验证方式

### 编译/测试

- `cd services/web && npm run build`
- `cd services/web && npm run test`

### 手工验证

- 批次 1：支付中心两个 tab 骨架一致且筛选可用；兑换中心标题只出现一次；移动端菜单按钮有焦点与读屏名称。
- 批次 2：逐页过一遍标题下/卡片内不再有解释性文案。
- 批次 3：分段控件键盘交互（方向键/Home/End）在三处回收点可用；MediaGaps 视图切换视觉与 Subscriptions tabs 一致。
- 每个批次过一遍规范 §12 检查清单中本次触达的条目。

## 落地后文档处理

- 全部批次完成后，本文档移入 `docs/archive/plan/console-admin/`。
- 批次执行中产生的规范修订直接落在 `docs/reference/web-design-guide.md`，规范始终代表现行事实。
- 与 `architecture/web-frontend-quality-improvement.md` 的日期/工具函数交集项（S11）在那份文档批次 3 执行时一并收口，两边进度记录互相标注。

## 进度记录

- 2026-07-26：完成 4 路并行布局评审（公开页+骨架 / 控制台用户页 / 后台组一 / 后台组二），问题清单定稿（P1×6、系统性通病 12 项、分页 P2/P3 约 60 条），等待排期修复。
- 2026-07-26：四批全部实施完成，`vue-tsc --noEmit` 0 错误、`npm run build` 通过、`npm run test` 155 passed / 3 skipped。规范先行修订：§2.2 字体改 system-ui 基线、§3.2 危险按钮补浅底/描边基线、§3.4 EmberTableCard 表头标题放宽（页头已表达同一语义时允许无标题主表）。
  - 批次 1（P1 快修）：完成。P1-1 TopBar 18 句描述删除；P1-2 支付中心骨架对齐播放/兑换模式（PaymentsView 恢复筛选、PaymentCenterView 删自造头卡、套餐分组分段项移出头卡）；P1-3 兑换中心双标题去重；P1-4 MediaGaps 视图切换换 EmberSegmentTabs；P1-5 Navbar 补 aria-label/aria-expanded/cursor-pointer；P1-6 续期中心标题重复去重。
  - 批次 2（S1 文案大扫除）：完成。按 §2.2.1 全站删除/压缩复述标题型、设计者视角、二次翻译文案；Dashboard 过期提示三合一。
  - 批次 3（组件回收 S2/S3/S10/S11）：完成。分段控件三套→EmberSegmentTabs（PlaybackProfileContent/UserPlaybackProfiles/MediaGaps）；统计卡→EmberMetricCard（Dashboard/TVCalendar/PlanGroups）；手写日期范围框→EmberDateRangeField、头卡→EmberPageHeaderCard（PlaybackProfileContent）；MediaGaps tone/圆角/空状态/搜索弹窗收敛；Subscriptions 海报徽章改中性底+状态点；S11 时间格式走 `utils/date`。
  - 批次 4（一致性长尾 S4/S5/S6/S7/S8/S9/S12）：完成。cursor-pointer 全站补齐；EmberTableCard 表头按规范放宽处理；弹窗宽度 560/640/720→520/680；危险按钮一族统一；骨架双缝（Layout border-r + 品牌/顶栏高度）；fadeIn 全局化进 base.css 并被 prefers-reduced-motion 覆盖；徽章统一中文计数；Hero 徽章中性文案、Footer 死链删除+容器宽度对齐；RegisterView 向登录页对齐。
  - 延后项（随迭代做）：EmberMetricCard 图标 slot（需扩组件契约）；媒体库 409 toast 双弹（需 api 层 `silent`）；SubscriptionsView 本地 fadeIn-up；PlaybackProfileContent 的 `description` prop 残留（待清理调用方后删）；MediaGapsView 聚合视图约 400 行自绘 CSS 是否整体重写（组件回收完成后另行评估）。
- 2026-08-30：归档复核确认四个实施批次、规范修订和组件测试均已有代码与 Git 证据；延后项不属于本方案完成条件，后续随对应功能迭代处理。稳定设计规则由现行 Web 设计规范接管，本计划仅保留历史追溯价值。
