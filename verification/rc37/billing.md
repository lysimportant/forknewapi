# rc.37 计费、任务插件与定价编辑器兼容验证

## 范围与状态

- 子 agent：`release_baseline`。工作区 `D:/newapi/.local-tests/worktrees/upgrade-rc37`，分支 `codex/upgrade-rc37`。
- 基线 `6e44f3d990aa700958e8f8cf88049639293ba455`，整合官方 `v1.0.0-rc.37`。本报告不代表整仓升级或生产发布已完成。
- 已解决并暂存本分工的 17 个冲突文件；新增兼容修复、测试及本报告由主 agent 最终统一暂存。未创建提交、Tag 或推送。
- 生产代码已完成本分工验证，可供主 agent 执行整仓构建、浏览器和集成验收。

## 合并决策与保留行为

1. `pkg/billingexpr/compile.go`、`run.go`、`types.go`、`billingexpr_test.go`：保留请求变量 `effort` 与请求倍率追踪，同时合入上游 `fixed()`、`image_count`、`img_cr` 和共享模拟器样本。
2. `setting/ratio_setting/model_ratio.go`：采用官方新增的 `ResolveCompletionRatio` 快照/草稿解析入口，同时保持管理员覆盖优先，包括显式零值；内置倍率只作缺省，不重新锁定价格。`pkg/billingexpr/expr.md` 同步说明。
3. `plugins/tasks/sora/plugin.js`：保留六种配置尺寸，包括 `1920x1080`、`1080x1920`；合入官方 1.0.3、枚举翻译和响应保留行为。配置尺寸仍不等于所有上游渠道都支持。
4. `plugins/tasks/kling/plugin.js`：保留请求模式对应的 720P/1080P；计费单位仍为 credit，完成后实际 Units 与预扣清晰度按原契约合并，不将 Units 当作秒数。元数据采用官方要求的计费对象和单价表述。
5. `web/src/features/pricing/lib/task-expr.ts`、`__tests__/task-matrix.test.ts`：采用官方首条匹配和最终兜底解析，同时保护旧 schema 扩展尺寸后原价格不变；允许官方新增的部分条件与重复组合优先级，不保留与新能力冲突的旧拒绝断言。
6. `web/src/features/system-settings/models/`：保留清晰度 tabs、默认/额外规格折叠、原始矩阵行索引、推理档位规则；合入官方币种输入、供应商独立定价和切回可视模式确认。`model-pricing-sheet.tsx`、`task-plugin-pricing-editor.tsx`、`task-usage-pricing-editor.tsx` 向所有相关入口传递型号、视频状态和币种。
7. `web/src/features/task-plugins/components/plugin-detail-sheet.tsx`、`usage-schema-table.tsx`：采用官方详情页结构、状态与 usageProfiles，同时在默认/分组 schema 中保留视频清晰度展示；支持官方单位和枚举本地化标签。
8. 新浏览器表达式解析器原先不能识别定制 `effort`。在 `billing-expression/{types,parser,runtime,visual,display}.ts` 增补字符串类型、求值、请求倍率追踪及 token 项排除；`request-simulation.tsx` 复用原有 `REASONING_EFFORT_VALUES` 和 Select，让实际档位可以独立模拟。复用现有翻译键，未编辑语言文件。

## 验证

运行时与依赖沿用主检查点。Bun 由现有本地缓存运行：`C:/Users/Sui/AppData/Local/npm-cache/_npx/5c4f1b4a21be27f7/node_modules/bun/bin/bun.exe`，没有安装新依赖。

- `go test ./plugins ./pkg/billingexpr ./pkg/jsplugin ./relay/channel/task/jsplugin ./setting/ratio_setting -count=1 -timeout=240s`：5 个包全部通过。
- `bun run test src/features/pricing src/features/task-plugins src/features/system-settings/models --maxWorkers=1`：36 个文件、603 个测试全部通过，267.38 秒。
- `bun run typecheck`：通过。
- 修改的前端文件通过 oxlint；插件 oxlint 无 error，两个上游原有 `preserve-caught-error` warning 位于 Kling/Sora 的 JSON 解析错误处理。
- 插件 oxfmt 检查通过；前端使用项目 oxfmt 配置检查并保留原版权头。
- 本分工 `git diff --check` 通过；17 个冲突文件均已解决。

新增 Go/TypeScript 共用样本覆盖 `fixed()` 图片价格乘数量和 max 档位、图片缓存加未命中档位、未指定档位；修复测试读取器把 `img_cr`/`img_o` 按表达式变量名映射到 Go 字段。新增模拟器交互覆盖默认 $0.01、max $0.02、切回默认 $0.01。

首轮 16 个前端失败中，视频价格测试需要适配官方 PricingAmountInput 的 textbox 和新可访问名称；矩阵旧断言需要匹配官方扩展能力；工作日条件编辑超时在单 worker 专项及最终完整专项均通过，未提高超时或删除该用例。

## 交接与边界

- 主 agent 负责全仓 lint/build/test、真实三库验证、服务启动、浏览器验收和 Git 交付；本子任务没有启动开发服务。
- 模块全量 lint 扫描仍能看到官方原有问题，集中在 `model-details-uptime-sparkline.tsx`、`model-details-api.tsx`、`model-details-apps.tsx`、`mock-stats.ts`、`filters.ts`、`constants.ts`、`upstream-ratio-sync-helpers.ts`、`upstream-price-cells.tsx`、`utils.ts`。未以无关格式或重构扩大此次兼容补丁。
- 建议浏览器验收：视频不同清晰度输入/币种切换和保存、供应商独立价格、插件分组用量详情、推理档位规则与请求模拟器。
- 没有调用付费供应商或生产 API；本地测试不能代替供应商实际能力和生产验收。

最后成功命令：上述 36 文件/603 测试专项、5 个 Go 包专项；下一步由主 agent 汇总整仓验证和 Git 交付。

## Responses 缓存模态明细补充复核

主 agent 的最终复核发现官方 rc.37 新增的缓存模态信息会在现有 Responses 转换边界丢失。本补充仅修改三个生产文件及两份既有测试；不变更数据库、依赖或其他供应商协议。

- `relaykit/relayconvert/internal/oai_responses/to_oai_chat_resp.go`：`usageFromResponsesUsage` 原先逐字段复制遗漏 `CachedTokensDetails`。原生 `NormalizeResponsesUsage` 和协议转换均复用此入口；原生计费无法获得图片缓存拆分。现改用已有 `InputTokenDetails.Clone()`，保留嵌套字段的缺失、显式零值和源对象隔离。
- `relaykit/relayconvert/internal/oai_chat/to_oai_responses_resp.go`：`UsageFromChatUsage` 原先只在标量非零时浅复制。现识别非 nil 的缓存模态结构并调用 `Clone()`，保护仅报告 `image_tokens: 0` 的输入以及双向转换中的独立副本。
- `relay/responses_chat_writer.go`：客户端用量白名单原先仅保留缓存总数/写入和推理 token。现保留 DTO 已支持的输入/输出 text、image、audio 明细及缓存模态子字段，按字段存在性保留零值；仍剔除内部快照、语义和 Chat 别名。
- `relaykit/relayconvert/internal/oai_responses/to_oai_chat_resp_test.go`：集中新增四个转换用例，覆盖非零模态、仅图片缓存零值、原生无新建快照、双向转换深复制和计费快照隔离。全零用量沿用原契约，不创建计费快照。
- `relay/responses_chat_writer_test.go`：集中新增六个用例，覆盖 JSON、SDK JSON 转 SSE、Chat SSE 的模态和零值输出；验证原计费用量不被修改、快照不泄露。原有三个精确 JSON 断言同步接纳标准模态零值。

修复前上述新增用例分别复现四个转换失败和六个 writer 失败。修复后验证：

```powershell
# relaykit/，Go 1.26.0；独立模块不依赖根工作区
$env:GOWORK = 'off'
go build ./...
go test ./... -count=1 -timeout=180s

# 根目录；完整执行这三个包的相关行为与计费回归
go test ./relay ./relay/channel/openai ./service -count=1 -timeout=180s
```

全部通过。根目录三个测试包耗时分别为 0.876、0.637、1.398 秒；五个变更 Go 文件均经 gofmt，`git diff --check` 通过。测试仅使用本地构造的兼容字段，不代表外部供应商已报告这些字段。

生产源码固定时 SHA-256：

- Responses→Chat：`2499C323B413A2389C0177C5ECFEE799336BC46D154493199C5CF8B16C0CECCE`
- Chat→Responses：`70796FFED77949DB3B768A3B26FE21B6F4A74A121D32ADF49F68F0568677CAF2`
- Responses writer：`246B2E8AABDD404DAB589F39001FB42BDEAF9134BDDC49D46BF84D3DCD582586`

补充复核结束，五个变更源码/测试文件及报告交由主 agent 统一暂存；最终构建和浏览器验收由主 agent 继续。
