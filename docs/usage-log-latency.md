# 使用日志耗时颜色对齐（2026-09-08）

## 目标与范围

P1：仅对齐普通使用日志的首字、总耗时文字及左侧竖条；同步共享组件的移动端状态点与详情文字。无后端、数据库、计费、API、依赖或文案变更。

对照 Sub2API `Wei-Shaw/sub2api` 提交 `772a0382f079676983c06f24b0d41e09139a8462`：`frontend/src/utils/latencyHealth.ts` 和 `frontend/src/components/admin/usage/UsageTable.vue`。

| 指标 | 绿 | 黄 | 橙 | 红 |
| --- | --- | --- | --- | --- |
| 首字 | <10s | ≥10s 且 <30s | ≥30s 且 <60s | ≥60s |
| 总耗时 | <60s | ≥60s 且 <180s | ≥180s 且 <300s | ≥300s |

文字采用 emerald/amber/orange/red 的 600（深色 400）；竖条采用 emerald-500/amber-400/orange-500/red-500。上端对应首字，下端对应总耗时，40%–60% 渐变；无首字数据时为总耗时纯色。总耗时不再依据输出 token 数或 TPS 改色。0ms 首字有效。保留既有时间格式和非流式布局。

## 基线与检查点

- 起点：`codex/video-usage-pricing`，`5c34df95a`，工作区干净；上游 `fork/codex/video-usage-pricing`。
- Node v24.12.0、Go go1.26.0、项目本地 Bun 1.4.2；依赖已存在，无需安装。
- Bun 路径：`.local-tests/responses-docker/tools/node_modules/bun/bin/bun.exe`，执行前加入 PATH。
- 修改前 usage-logs：8 文件、25 测试通过（从 web 目录运行；首次从根目录执行未加载 web 的测试配置，已纠正）。
- `bun run typecheck`、`bun run build` 通过；修改文件定向 oxlint 通过。
- 全量 `bun run lint` 受未修改文件既有错误阻塞，例如 `cache-stats-dialog.tsx` 的 promise/catch-or-return、curly 与 `scripts/sync-i18n.mjs` 的 curly；本任务不扩展为全仓 lint 清理。
- 全量 `bun run test`：61 文件、446 测试通过，含新增 38 个耗时回归案例。
- 浏览器：`bun run dev --port 5173` 启动成功；Playwright 使用完全拦截的模拟 API 数据，未调用上游或写入数据库。普通日志实际渲染、横向滚动定位耗时列、浅深色截图、实际渐变 CSS 检查通过；模拟响应完善后无页面/控制台错误。截图位于 `.local-tests/latency-light.png` 与 `.local-tests/latency-dark.png`（不提交）。随后尝试额外视图切换时工具超时，已重新读取规则/工作区状态并重启浏览器，确认开发服务可访问；未把该额外交互计为通过。
- 未部署生产；实际部署版本的 Sub2 若与所对照提交不同，需另行提供其版本核对。

