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
