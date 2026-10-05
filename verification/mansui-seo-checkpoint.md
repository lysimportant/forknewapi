# ManSuiAI 站点品牌与 SEO 检查点

日期：2026-10-05。P1 站点改版，按大变更交付。

- 基线：main，67c803073，跟踪 fork/main；工作区开始时干净。
- 正式域名：https://api.lolicon.beer；画布：https://love.lolicon.beer。
- 运行环境：Go 1.26.0（模块 1.25.1），Node 24.12.0；复用项目本地 Bun 1.4.2：.local-tests/responses-docker/tools/node_modules/bun/bin/bun.exe；web/node_modules 已存在。
- 目标：ManSuiAI 部署品牌、鲸鱼娘图标与分享图、公开页 SEO、真实 robots/sitemap、默认主页改版、关于页说明、悬停停止模型滚动、画布入口。
- 保留：源码/许可证/开源归属、后台功能、管理员自定义内容；不改数据库结构、认证、计费或模型调用契约。
- 边界：不部署生产、不调用付费模型，不将静态模型示例冒充当前渠道可用清单。
- 影响与回滚：公开页面 HTML/404/索引规则变化；合法 SPA 深路径保持兼容。无数据迁移；回滚可恢复上一提交/镜像，后台配置保持原值。
- 基线测试：about 与 status-query 共 2 文件 24 测试通过（.local-tests/mansui-seo/frontend-baseline.log）。
- 已实现：主页、后端 SEO、前端品牌与关于页、鲸鱼娘图标/分享图、7 语言文案。模型滚动支持悬停、键盘焦点和手动暂停，复制循环支持悬停说明。
- 兼容复查：保留管理员自定义名称/图标/首页/关于页；旧 New API、NewAPI、ManSui 缓存品牌归一化；公开价格页按现有导航配置决定收录。
- 最终验证：`GOWORK=off go test ./... -p 2 -count=1 -timeout=300s`、`go vet ./...` 通过；最后后端修复后 `go test ./router -count=1` 通过；最终前端 `bun run build:check` 和重新嵌入资源的 `go build` 通过。
- 前端回归：主页、关于页、SEO、status-query、公开导航公告共 5 文件 46 测试通过；改动 TS/TSX 文件 lint 无 error，仅保留 Footer 自定义 HTML 的既有 no-danger warning；格式和 git diff --check 通过。全仓 lint 另有历史问题（如 clerk 图标类型导入、rankings 嵌套条件），不纳入本次品牌改版修复范围。
- 最终浏览器验收：本地隔离 SQLite 服务 http://127.0.0.1:3045；18 项检查通过，包含初始 HTML 唯一 title/canonical、无脚本正文、robots/sitemap、私有 noindex、真实 404、1440/1024/390px 深浅色、tooltip/焦点暂停/减少动效，以及自定义 Home/About 跨 SPA 导航的中性 SEO。无页面异常，截图已人工检查。
- Lighthouse（本地桌面）：首轮性能 67、可访问性 96、最佳实践 100、SEO 100；修复对比度和模型可访问名称后复测为性能 75、可访问性 100、最佳实践 100、SEO 100。性能分数会受本机和加载时序影响，不宣称线上性能或排名提升。
- 产物记录：`.local-tests/mansui-seo/` 保存构建/测试日志、Lighthouse JSON/HTML、browser-result.json 与 final-screens 截图；不提交临时工具、测试数据库和预览二进制。
- 后续独立优化：首屏资源实测约 3.1 MiB，Lighthouse 仍提示未使用 JavaScript 和 LCP 加载时序；需另行测量及拆分主包，避免在品牌改版中大范围重构后台路由。
- 交付：中文提交并创建 `v1.0.0-rc.37.custom.29` annotated Tag，目标为 `fork/main`（https://github.com/lysimportant/forknewapi.git）；不推送官方 origin。本轮不部署生产、不修改线上配置。线上更新需重新构建含 `web/dist` 的镜像/二进制。
- UI 复用：复用 Button/buttonVariants、Tooltip、Accordion、CopyButton、PublicLayout 与 Footer；新增 CSS 球体/波纹与画布流程展示属于品牌视觉，现有通用组件不提供这些视觉布局，未安装额外应用依赖。
- 验收：公开 HTML/title/canonical/robots/sitemap；私有 noindex、未知 404；深浅色桌面/窄屏截图；动画/悬停/键盘/跳转；相关测试、typecheck/lint/build、Go test/vet/build。

## Canvas 粒子首屏修订（2026-10-05）

- 用户要求：取消首屏人物大图，改为科技感 Canvas 粒子特效，文字置于特效中央。
- P1 视觉修订，按大变更交付；基线 `main` / `1a9c177d7`，开始时工作区干净，运行环境与依赖沿用上方记录。
- 实现：三维粒子环、轨道脉冲、透视波面和外围星尘；鼠标轻微视差；中央品牌与 API/画布入口；默认首页导航配色调整。鲸鱼娘继续用作站点图标。
- 复用与范围：复用现有 Button、Tooltip、Accordion 和 PublicLayout；组件库没有装饰性粒子场，使用原生 Canvas 2D，不新增应用依赖。保留 SEO、模型悬停说明、管理员自定义内容；不修改认证、数据库、计费或模型契约，不部署生产。
- 降级：手动暂停、减少动态偏好、离屏/隐藏暂停、有限 DPR 与窄屏降密；Canvas 不可用时文字与链接仍可用。无数据迁移，回滚本轮代码后重新构建即可。
- 验证通过：`bun run build:check`（含 typecheck）、`go build`；主页/关于页/SEO/status-query 共 4 文件 46 测试，最后删除无效粒子排序后主页 9/9 再次通过；改动 TS/TSX 的 oxlint 与 Oxfmt、`git diff --check` 通过。
- 浏览器验收：18 项既有 SEO/模型/自定义页面检查和 10 项粒子首屏检查通过；实看 1920、1440、1024、390px 深浅色截图，中央文字与入口可读，无横向溢出和页面异常。验证实际图像暂停/恢复、动态切换 reduced-motion、离屏停绘/返回恢复、SPA 卸载及无 Canvas 降级。
- 性能测量：桌面 Chrome headless 1440×900、DPR 1，在截图/读回前采样 110 帧；移除 `lighter` 加色混合中无效的逐帧深度排序后，回调中位数 18.3ms、P95 22.9ms，约 30fps（修改前同方式 20.1ms / 24.7ms）。这是本机采样，不作为其他设备或生产性能保证。
- Lighthouse 最终桌面：性能 71、可访问性 100、最佳实践 100、SEO 100；LCP 3.3s、TBT 210ms。首次性能复测因密集测试触发本地 Web 访问限流，确认 HTTP 429 后重启本任务隔离预览并串行复测，未修改任何限流配置。首屏包体及加载时序仍有既有优化空间，另行处理。
- 证据：`.local-tests/mansui-particles/` 保存构建/测试日志、28 项浏览器检查结果、帧采样、最终截图与 Lighthouse 报告；预览为 http://127.0.0.1:3045/，使用隔离 SQLite。
- 交付：任务中文提交与 annotated Tag `v1.0.0-rc.37.custom.30`，目标 `fork/main`；不部署生产。运行时/媒体偏好/生命周期已由代码审查及上述验收确认，没有未完成的本轮实现事项。

## 科技风统一、当年模型与游动鲸鱼（2026-10-05）

- P1 首页视觉与展示内容修订；基线 `main` / `cd4cf9d6b`，开始时工作区干净，沿用已记录的本地运行环境、依赖及上一轮通过的测试基线。
- 验收目标：首页下方画布/能力/FAQ/CTA/Footer 与首屏保持连续深色科技风；滚动模型按官方最新可确认资料更新；光环内部以 Canvas 绘制可识别、缓慢游动的 DS 鲸鱼，文字和按钮保持可读可操作。
- 范围：默认首页的样式、展示资料、装饰性动画及必要回归。保留站点图标、公开页 SEO、自定义首页/页脚、开源归属和后台行为；不改 Provider、计费、数据库或认证，不调用付费生成，不部署生产。无数据迁移，回滚本轮代码并重建即可恢复。
- 实现：画布、能力介绍、步骤、FAQ、CTA、Footer 与模型提示统一深黑/冰青配色，浅色系统主题下也不再出现白色断层；保留默认首页以外的主题行为。
- 模型：替换旧 DeepSeek Chat/GPT-4.1/Seedream 4.0，展示 12 个当年型号，注明目录核实日期及“本站模型 ID”；悬停、键盘聚焦、手动暂停与重复循环说明保持可用。七语言新增 15 个文案键，通过规定脚本写入并完成 i18n:sync。
- 动效：Canvas 绘制 DS 鲸鱼，20 秒沿环顶→右侧下潜→环心→左侧上浮回游；尾鳍摆动、尾迹、深度缩放及透明度变化与主场景时间同步。转身短暂呈侧影，环内降低亮度以保持正文清晰。继续支持手动暂停、减少动态、离屏/隐藏停绘及 Canvas 缺失降级。
- 复用与许可：继续使用 Button、Tooltip、Accordion、PublicLayout；现有组件不提供鲸鱼视觉，采用 Canvas 2D 与缓存 Path2D，不增依赖。轮廓复用已安装 @lobehub/icons 的 DeepSeek 路径，保留 LobeHub MIT 许可和原仓库 AGPL 头。
- 资料核实：2026-10-05 GET https://api.lolicon.beer/api/pricing 返回 63 个模型，展示的 12 个本站 ID 均存在；快照位于 .local-tests/mansui-whale/public-model-catalog.json。此结果不证明付费调用可用性，也不保证涵盖全球所有最新模型。官方名称与网关别名可能不同，尤其 DeepSeek、Seedream、Seedance 和 MiniMax。
- 官方参考：DeepSeek https://api-docs.deepseek.com/news/news260910；Qwen https://help.aliyun.com/zh/model-studio/getting-started/models；Kimi https://platform.kimi.com/docs/guide/kimi-k3-quickstart；GLM https://docs.bigmodel.cn/cn/guide/models/text/glm-5.3；Seedream https://seed.bytedance.com/en/seedream5_0_pro；Seedance https://seed.bytedance.com/en/seedance2_5；MiniMax https://platform.minimax.io/docs/release-notes/models。
- 官方参考（前期代理核对，收尾直连受限）：GPT-6.1 Sol https://developers.openai.com/api/docs/models/gpt-6.1-sol；GPT-6 Astra https://developers.openai.com/api/docs/models/gpt-6-astra；图像目录 https://developers.openai.com/api/docs/models；Claude https://platform.claude.com/docs/en/models/opus-5-5/overview；Gemini https://ai.google.dev/gemini-api/docs/changelog。收尾直连分别遇到 OpenAI 403、Claude 地区跳转和 Google 连接失败；不添加无法复核的发布日期、排名或知识截止宣称。
- 验证：主页/关于页/SEO/status-query 共 4 文件 46 测试通过，最终文字空白修正后主页 9/9 再次通过；目标 TS/TSX 的 oxlint、限定文件格式检查、build:check（含 typecheck）、Go 嵌入资源构建通过。未改后端源码，不重复上轮全量后端矩阵。
- 浏览器：18 项 SEO/交互/自定义页面检查、10 项动画生命周期检查通过，无页面异常；1440/1024/390px 实测鲸鱼垂直运动范围约 144–495px，确认进入环心后上浮、暂停冻结图像、没有横向溢出。最终预览仍为 http://127.0.0.1:3045/，隔离 SQLite，不接触生产数据。
- 过程恢复：全量 format:check 的目录快照还原曾覆盖并行修改，已补回并逐一核对；后续仅对本轮文件格式化。密集浏览器检查触发本地 429 时仅重启隔离预览，未修改限流配置。Tooltip 检查改用实际 Tab 聚焦和 data-open 状态，避开关闭过渡中的重复浮层。
- 最终性能：本机 Chromium 桌面 1440×900、DPR 1，采样 110 帧；去除鲸鱼眼部 Canvas 阴影后，中位绘制时间由 26.3ms 降至 16.5ms，P95 由 29.5ms 降至 19.1ms，约 30fps。该数据仅代表本机样本；保留整体页面既有拆包优化待办，不扩展本轮范围。
- Lighthouse 最终结果：性能 73、可访问性 100、最佳实践 100、SEO 100；LCP 3.3s、TBT 180ms。修正模型名称与 ID 的实际文本空白后，label-content-name-mismatch 不再报错。首屏约 3 MiB 与未使用 JavaScript 仍属既有性能限制。
- 证据目录：.local-tests/mansui-whale/ 包含最终构建与测试日志、模型目录快照、官方资料复核结果、18 项浏览器结果、10 项动画结果、三个视口下潜/上浮轨迹及截图、帧采样和 Lighthouse；临时工具、数据库和二进制不提交。
- 交付检查点：本轮实现与验收完成，任务提交及 annotated Tag v1.0.0-rc.37.custom.31 交付至 fork/main（https://github.com/lysimportant/forknewapi.git）；不推官方 origin、不部署生产。生产生效仍需重新构建并更新服务。

## 刷新白底修复（2026-10-05）

- P1 首屏衔接修复；基线 `main` / `d99b5755a`，跟踪 `fork/main`，开始时工作区干净。Node 24.12.0、Go 1.26.0（模块 1.25.1）、本地 Bun 1.4.2 与现有 web/node_modules。
- 已复现：线上刷新先显示未排版的白底 SEO 摘要，随后进入深色首页；首页内容接口等待分支也使用普通主题。基线 SEO/首页 2 文件 22 项测试通过。
- 目标：默认首页初始 HTML、接口等待页和正式首页背景一致；主题 cookie 在入口脚本前应用；保留可抓取 SEO 正文、自定义首页及其他页面的主题行为。
- 范围与回滚：仅 Web 首屏背景与 HTML 启动标记，不变更数据库、认证或计费契约，不安装依赖、不部署生产。回滚本轮提交并重建前后端产物即可，无数据迁移。
- 验收：先失败后通过的首屏回归；相关 Go/前端测试、lint/typecheck/build；浏览器普通刷新、延迟资源/接口、深浅色、自定义页面和路由离开检查；最终 diff 与目标远端核验。
- 实现：后端在首页 HTML 中输出默认/自定义内容标记；入口内联脚本提前恢复主题 cookie，内联样式为 SEO 摘要提供对应背景。默认首页等待接口时沿用深色壳，React 提交时清理摘要，内容就绪或离开路由时清理启动标记；兼容 StrictMode effect 重放。复用 PublicLayout、LoadingState 和 mansui-landing，不新增组件或依赖。
- 红绿验证：旧入口脚本保存 dark cookie 后未设置 html.dark，新增服务端模式测试也先失败；补齐实现后通过。StrictMode 下 pending 标记提前清理的回归已修复。
- 最终检查：`bun run test src/features/home/__tests__/bootstrap.test.tsx --reporter=verbose` 13/13；既有首页/关于页/SEO/status-query 共 4 文件 46/46。`go test ./router -count=1`、`go vet ./router`、目标 TS/TSX oxlint、限定 Oxfmt、`bun run build:check`、最终 `bun run typecheck`、重新嵌入 web/dist 的 `GOWORK=off go build`、`git diff --check` 通过。
- 浏览器验收：隔离 SQLite 服务 http://127.0.0.1:3046/，本地代理覆盖外部 JS/CSS 不到达、首页接口挂起/恢复、无脚本和自定义页面。默认首页在初始摘要、等待和正式页面三个阶段的 html/body 背景均为 rgb(3, 7, 11)；自定义内容和离开首页保持普通主题；深色偏好刷新保持，最终产物页面无控制台 error。截图已逐项检查，日志及临时代理保存在 `.local-tests/home-flash/`。
- 过程恢复：测试子代理遇到临时 429 后由主代理接手；使用现有 jsdom Document 和 node:vm 验证真实入口脚本，未安装依赖。远端 Schannel 握手失败后使用单次 `http.sslBackend=openssl` 成功核对远端，不修改全局 Git 配置。
- 交付：任务中文提交及 annotated Tag `v1.0.0-rc.37.custom.34`，目标 `fork/main`（https://github.com/lysimportant/forknewapi.git）。本轮不部署生产；服务器需拉取 main 并重新构建前端及包含 web/dist 的 Go 产物后生效。现有首屏资源体积优化继续保持独立待办。
