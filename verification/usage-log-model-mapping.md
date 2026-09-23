# 使用日志模型差异展示

## 范围与基线

- 任务：P1，前后端日志功能。用户确认同时覆盖请求与调用模型差异、上游响应与调用模型不一致；需要直接展示差异并支持查看和复制完整名称。
- 验收：映射模型直接可见；上游响应不一致醒目标识；同名不重复展示；未知响应不作判定；旧日志和无效字段正常降级；列表、卡片和详情采用一致规则。覆盖 OpenAI Chat/Responses 与 Claude 的流式、非流式解析路径，具体验证见后续记录。
- 不在本次范围：底层模型身份验证、计费规则、数据库结构、权限和生产部署。
- 初始分支：`main`，跟踪 `fork/main`，工作区干净；提交 `870b16c63`。
- 交付远端：`fork` → `https://github.com/lysimportant/forknewapi.git`。
- 运行时：Go `go1.26.0 windows/amd64`、Node `v24.12.0`、Bun `1.4.2`。
- Bun 使用仓库已有的 `.local-tests/responses-docker/tools/node_modules/bun/bin/bun.exe`；`web/node_modules` 与 `web/bun.lock` 已存在，无需安装依赖。
- 复用：既有 `ModelBadge`、`StatusBadge`、`Popover`、`Button`、`CopyButton` 和详情布局，不增加公共组件或依赖；补充七种语言的响应模型文案。

## 实现与验证

原界面仅通过 `is_model_mapped` 显示映射图标，上游名称藏在弹层中。现在直接比较请求与调用名称；确认响应不一致时增加警示行，三个完整名称均可复制。请求名称优先采用 `requested_model_name`，避免把计费别名显示成请求原名。

发送模型在最终 JSON 参数覆写之后采集，透传请求采用原始模型；若目标协议删除了 `model`，明确保持未知。每次渠道重试清空上次观测。Claude 原有 `UpstreamModelName` 回写继续保留，独立快照用于日志，避免改变现有转发和计费。流式响应以结束时有效模型声明为准，Responses 终态声明优先；空声明不覆盖已知值。比较去除首尾空白后按精确字符串进行，不擅自合并大小写、版本、日期或 `latest`/`build` 别名。

消费和错误日志共用模型字段写入方法。没有响应模型仍可展示已确认的调用模型差异；缺少发送或响应模型时不写不一致判定。响应名不截断后参与比较，避免长名误报。

| 检查 | 命令或证据 | 结果 |
| --- | --- | --- |
| 前端基线 | `bun run test src/features/usage-logs` | 17 文件、200 测试通过 |
| 新增行为回归 | `bun run test src/features/usage-logs/components/__tests__/model-mapping.test.tsx --maxWorkers=1` | 先复现失败，再通过全部 25 项 |
| 日志回归 | `bun run test src/features/usage-logs --maxWorkers=2` | 18 文件、225 测试通过 |
| 类型与 lint | `bun run typecheck`；对本次 7 个 TS/TSX 文件运行 `oxlint -c .oxlintrc.json` | 通过，无新增错误 |
| 格式与翻译 | 修改文件使用保留版权头的 oxfmt；通过 `add-missing-keys.mjs` 写入并执行 `bun run i18n:sync` | 通过；七语言各新增 4 项文案，临时脚本已移除 |
| 前端构建 | `bun run build` | 通过 |
| Go 构建 | `go build -o .local-tests/model-mapping-new-api.exe .` | 通过，包含最新 nil ChannelMeta 防护 |
| Go 静态检查 | `go vet ./relay/... ./service ./controller` | 通过 |
| Go 回归 | `go test ./relay/common ./service ./relay/channel/openai ./relay/channel/claude -count=1`；`go test ./relay`；`go test ./controller` | 全部通过；controller 146.641s，终态修复后四包再次通过 |
| 最终差异 | `git diff --check`、新增行凭据模式扫描 | 通过；未发现密钥模式 |

全仓 `bun run lint` 基线已有 192 个 error，未在本轮扩展修复；记录在 `.local-tests/model-mapping-baseline-lint.log`。未改动任何 ORM、数据库驱动、schema、迁移、SQL、Scanner/Valuer 或存储序列化实现，因此没有新增三库迁移验收要求。

## 浏览器验收

通过 Playwright 在本次生产构建上验证 1440 浅色普通用户、1280 深色管理员、420 窄卡片兼容、1440 中文四个场景。覆盖双差异、仅响应差异、请求与计费别名不同、匹配响应、未知/空白/非字符串响应、缺失或 false 标记、三名称完整复制、键盘打开与 Escape 回焦、详情三字段。所有场景均没有 console/page/request 错误、未识别 API 或页面横向溢出。

命令：`$env:BASE_URL='http://127.0.0.1:43918'; node .local-tests/model-mapping-browser/fixture.cjs`。API 与外部请求均由 Playwright fixture 拦截，没有连接真实后端、数据库、账号或付费模型；此结果证明界面行为，不是外部供应商验收。

证据位于 `.local-tests/model-mapping-browser/evidence/acceptance-report.json`。中文列表与三模型弹层截图为同目录 `user-zh-light-1440.png`、`user-zh-light-1440-model-details-popover.png`，均已查看。临时 dev/preview 进程已关闭，43917/43918 端口已释放。

## 数据语义与回退

调用模型来自 `other.upstream_model_name`；响应声明来自新增 `other.upstream_response_model`，不一致标记为 `other.upstream_model_mismatch`。`requested_model_name` 仅在请求名与日志主字段不同时补充。字段缺失不推测；响应声明也不能证明第三方服务内部真正运行的模型。历史日志不会回填响应名，模型筛选继续沿用既有查询语义。

响应采集覆盖 OpenAI Chat、Responses、二者互转和 Claude 的本次修改路径；其他独立协议不声称已完成响应核对。错误日志保存错误发生前已观测到的模型，不从错误响应正文推断模型。没有真实供应商调用，不将本地模拟结果视为外部模型身份验证。

现有日志字段保留，新字段写入已有 Other JSON，不涉及 schema、迁移、查询或历史数据覆写；不需要数据迁移或备份恢复操作。需要回退时撤销本次提交并重建，旧版本忽略新增字段。本次未部署或替换正在运行的服务。

独立复核发现并修复了 Chat 流首帧掩盖终态模型的漏报；新增终止分片、最终 usage 分片、末尾空模型回归。最终 Go 构建和静态检查已基于修复版本重跑通过。

交付目标：`fork/main` 和中文 annotated Tag `v1.0.0-rc.37.custom.3`；最终提交与推送核验完成后由交付回复给出提交 ID。

## 2026-09-23：列表显示已记录的推理强度

本次为 P2、小范围前端展示修复。基线为 `main` / `fork/main`、`2e4f1681513d8103deb3bd525ecabfded73a4c51`，工作区干净；继续使用上述 Go、Node 和仓库本地 Bun 版本，不安装或升级依赖。

服务端原本已将非空推理强度写入公开字段 `other.reasoning_effort`，详情对话框也已有展示；缺口是列表模型列及共享卡片未读取该字段。现在模型名称下方直接显示“推理强度: max”等记录值，管理员和普通用户均可见；复用既有 `ModelBadge`、`StatusBadge`、颜色映射及七语言的 `Reasoning Effort` 翻译，不另建徽标或交互封装。

仅接受非空字符串并去除首尾空白，保留 `none`、未知档位等原始值；缺失、空白或类型错误时不显示，不从模型名称猜测默认值。模型映射、响应不一致警告、完整模型名复制及卡片详情入口保持原有行为。

| 检查 | 命令或证据 | 结果 |
| --- | --- | --- |
| 回归先复现 | `bun run test src/features/usage-logs/components/__tests__/model-mapping.test.tsx --maxWorkers=1 --testTimeout=20000` | 实现前 9 项新增展示用例失败、32 项通过；最终全部 41 项通过 |
| 使用日志回归 | `bun run test src/features/usage-logs --maxWorkers=2 --testTimeout=20000` | 18 文件、241 项通过 |
| 类型与构建 | `bun run build:check`，最终另跑 `bun run typecheck` | 通过 |
| 代码检查 | `bun x --no-install oxlint -c .oxlintrc.json` 指定四个修改的 TS/TSX 文件；对同四文件保留原版权头后执行 `oxfmt --check` | 无 error/warning；格式通过，未保留全仓格式化改动 |
| 国际化 | `bun run i18n:sync` | 七语言已有翻译可直接复用，无 locale 差异 |
| 请求元数据 | `go test ./relay/common -run 'TestGenRelayInfoCapturesRequestReasoningEffort\|TestInitChannelMetaRestoresRequestReasoningEffortForRetry\|TestApplyParamOverrideWithRelayInfoSynchronizesReasoningEffort\|TestReasoningEffortOverrideIsAuditedWithoutDebugMode' -count=1 -v` | 通过，覆盖现有协议采集、重试与参数覆盖 |
| 日志生成 | `go test ./service -run 'TestGenerateTextOtherInfoRecordsModelAuditContract\|TestAppendRelayModelLogInfoHandlesMissingChannelMeta' -count=1 -v` | 通过 |
| 日志可见性 | `go test ./model -run 'TestLogOther\|TestFormatUserLogsStripsQuotaSaturation\|TestTaskPluginLogVisibilityIsRoleSeparated\|TestLegacyLogOtherVisibilityIsRoleSeparated' -count=1 -v` | 通过 |
| 生产构建浏览器验收 | `node .local-tests/reasoning-effort-browser/fixture.cjs`，预览端口 43919 | 1440 个人浅色、1280 管理员深色、1440 中文、420 卡片四场景通过；无控制台/页面/请求错误和页面横向溢出 |

独立复核发现无模型差异时的长名称会越过新增限宽容器；浏览器先复现内容右边界 1434px 超出容器 853px，再复用 `wrapText` 修复。最终四场景补查名称边界，构建、日志回归与浏览器验收均基于修复版本重跑。

浏览器仅拦截模拟日志接口，验证 `max`、显式 `none`、缺失字段以及既有模型详情和复制，不连接真实后端、账号、数据库或付费模型。证据保存在 `.local-tests/reasoning-effort-browser/evidence/acceptance-report.json`；中文、深色和卡片截图已经人工查看。

限制：历史日志不会补造字段；未记录强度的请求仍不展示。后端现有全局/渠道请求体透传路径会清空推理强度，本次不改变该行为。显示的是网关日志记录的请求设置，不是对上游实际执行强度的独立验证。现有 service 日志测试没有直接断言该字段写入，采集与可见性由现有定向测试及代码复核确认；没有把它描述为完整后端集成覆盖。`-race` 因当前环境未启用 CGO 未运行。

未改后端、数据库、依赖或公共契约，不需要迁移；可回退本次提交并重建前端。没有替换正在运行的服务或部署生产。交付为当前 `fork/main` 的任务提交；按低风险展示修复处理，不创建发布 Tag。
