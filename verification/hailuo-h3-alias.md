# Hailuo 的 h3 映射与画布可用性

## 范围与基线

- P1：修复已确认采用 H3 官方协议、仅模型名改为 `h3` 的渠道映射。验收目标是 `MiniMax-H3 → h3` 同时通过画布目录、费用预估和视频请求校验，并保持对外名称、现有定价及账务规则。
- 2026-09-20 基线：`main @ 6f25a7b84`，跟踪 `fork/main`；工作区干净。交付远端为 `https://github.com/lysimportant/forknewapi.git`，不向上游 `origin` 推送。
- Go `1.26.0 windows/amd64`、Node `24.12.0`；已有 Go 模块缓存与 `web/node_modules`，未安装或修改依赖。
- 用户确认上游要求精确模型名 `h3`，除此以外与现有 H3 协议一致。本次不推断其他别名，不重写计费或渠道配置，不做付费生成。

## 原因与修复

只读检查线上管理页确认：渠道绑定 `hailuo`，映射为 `MiniMax-H3 → h3`，当前生效的工厂插件是 `1.1.3`。桥接开关已启用，但原凭据查询目录仍得到 `MiniMax-H3: missing_profile`；同连接的两个 Wan 模型可用。

旧插件未在 `meta.models` 声明 `h3`，桥接无法匹配调用合同；插件的 H3 分支也只识别 `MiniMax-H3`，映射后会落到旧 `/v1/video_generation` 和 6 秒限制。因此仅放开目录校验不能解决生成。

Hailuo `1.1.4` 增加精确 `h3` 声明，并在现有 `isH3` 判断中将它识别为 H3。提交继续使用 `/v2/video_generation`，查询继续使用 `/v2/query/video_generation/:task_id`；上游请求保留 `model: "h3"`，对外目录、任务和计价身份保留 `MiniMax-H3`。复用 H3 的 4–15 秒、768P/2K、参考输入和用量校验。

已复核插件元数据的中英文计费字段，保持“视频生成单价”“输入图片单价”“输入视频单价”等既有定义。本次没有确认新的上游模型目录接口，继续沿用明确标注来源的“填入插件模型”，不将静态声明冒充实时上游目录。

## 验证

| 检查              | 命令或证据                                                                                                 | 结果                                                                                                       |
| ----------------- | ---------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| 桥接回归          | `go test -mod=readonly ./controller -run 'TestCanvas' -count=1`                                            | 通过；默认 SQLite，未启用的 MySQL/PostgreSQL 案例不算验收                                                  |
| 映射目录与预算    | `TestCanvasBridgeHailuoMappedH3`                                                                           | type 35 与 type 61 均通过；原模型价格计算 5 秒为 500000 quota，未知别名仍返回 `missing_profile`            |
| Hailuo 与内置插件 | `go test -mod=readonly ./plugins -run 'TestHailuo\|TestBuiltIn' -count=1`，`go test ./plugins -count=1`    | 通过                                                                                                       |
| 最终参数边界      | `go test -mod=readonly ./plugins -run '^TestHailuo' -count=1`                                              | 提交/查询 `/v2`、模型名、任务 ID、参考图和用量通过；接受 4/15 秒，拒绝 3/16 秒、1080P 和超过两张首尾帧图片 |
| 全量后端          | `go test -mod=readonly ./... -count=1`                                                                     | 通过；日志 `.local-tests/hailuo-h3-all-tests.log`。随后仅扩展测试边界与注释，最终 Hailuo 定向测试再次通过  |
| 静态检查与构建    | `go vet -mod=readonly ./...`；`go build -mod=readonly -o .local-tests/hailuo-h3-new-api.exe .`             | 通过                                                                                                       |
| 插件验证          | `go run -mod=readonly . plugin lint plugins/tasks/hailuo/plugin.js`                                        | `plugin hailuo@1.1.4 is valid`                                                                             |
| 格式与 lint       | `oxfmt -c plugins/.oxfmtrc.json --check plugins/tasks/hailuo/plugin.js`；插件 `oxlint`；`git diff --check` | 通过；oxlint 有一处未修改代码既有的 `preserve-caught-error` 警告                                           |
| 画布结算与广场    | Worker `billing-execution.test.ts`、Web `newapi-square.test.tsx`                                           | 分别 28/28 与 4/4 通过                                                                                     |

测试使用本地插件 hook、临时 SQLite 和合成凭据，不请求真实生成，不改变用户余额。没有数据库、SQL、迁移、依赖或插件宿主合同修改，因此本轮不新增三库迁移验收要求。

## 部署与恢复检查点

- [x] 本地修复、回归、构建及插件校验完成。
- [x] 8080 画布已按用户授权保存 `https://api.lolicon.beer/pricing`，读取 33 个模型原价；浏览页可见，两个 Wan 模型可用，H3 等待线上插件更新。
- [ ] 取得生产更新授权后，将线上 `hailuo` 更新到本次验证的 `1.1.4`。可通过管理页上传并启用该版本，或拉取本次 fork 提交、重建镜像后部署；仅 `git pull` 或重启旧镜像不会替换嵌入的工厂插件。
- [ ] 更新后检查实际生效版本与注册状态，再同步画布连接，确认 `MiniMax-H3` 的目录状态为可用且合同为 `newapi-video-v1`。上传的旧覆盖版本优先于工厂插件，不能只凭镜像版本判断成功。
- [ ] 真实付费生成、最终回执和人民币扣款仍需单独授权验收。不得将本地通过或目录可用视为上游生成成功。

生产操作前记录现有生效版本和镜像；保留旧插件或镜像作为回退点，保持渠道映射、价格、凭据和余额。插件上传回退使用管理页原版本或原工厂版本，镜像部署回退到记录的旧镜像；不恢复或覆盖账务数据库。回退到 `1.1.3` 会恢复原先的 `h3` 不可用状态。新版本对映射后的历史 H3 任务也采用 `/v2` 查询，上线前应查看该渠道是否有待处理任务。

画布继续通过 `/v1/canvas/estimate` 取得上游预算，通过原冻结 Key 的 `/v1/canvas/receipts/:requestId` 核对最终净 quota，以冻结汇率换算 CNY 并按确认预算封顶。缺失或未完成回执保持待核实，不使用 Key 总余额差值结算。账务链路本轮仅核查，未重写。

交付目标：`fork/main` 与中文附注 Tag `v1.0.0-rc.37.custom.12`；最终提交及远端一致性以交付回复的 Git 核验为准。线上插件仍未更新，本记录不表示生产验收完成。

## 2026-09-20 后续：H3 图片提及与媒体预估

最新状态覆盖上节历史部署检查点：8080 的 H3 已发布、`missing_profile` 已消失；本次错误为“模型 MiniMax-H3 不支持 image 类型资源提及”。P1 起点 `main @ 9470bf372`，工作区干净。目录的 `mentionMediaTypes` 固定为 `text`，与 Hailuo 已实现的 H3 多模态协议不符。

修复仅涉及宿主桥接和原有桥接测试。目录仅为画布已适配的对外 `MiniMax-H3`，按实际 `hailuo` 插件、精确上游 `MiniMax-H3`/`h3` 与 `newapi-video-v1` 的可执行且已定价路由交集声明 `text,image,video,audio`；其他对外别名及未知/混合渠道仍保持保守。估算新增可选 `input_media` 数组，每项仅含 `type` 和 `role`，不接受真实 URL、资产身份或其他字段。图片最多 9 张、视频与音频各 3 段，首尾帧各一张且不能与参考混用。

示例：`{"model":"MiniMax-H3","contract":"newapi-video-v1","parameters":{"seconds":5,"resolution":"768P"},"input_text":"Animate this image","input_media":[{"type":"image","role":"reference_image"}]}`。合法角色为 `first_frame`、`last_frame`、`reference_image`、`reference_video`、`reference_audio`，必须与类型配对。宿主只在估算内存中补安全占位内容，再调用现有插件解码/用量提取及原定价表达式；不提交、下载或查询任务，不改价格。视频用量沿用 H3 原有 15 秒预留规则。

验证均为本地合成请求：

- `go test -mod=readonly ./controller -run '^TestCanvas' -count=1` 通过；`TestCanvasBridgeHailuoMappedH3` 覆盖 type 35/61、原名/映射、未适配对外别名、未知和混合插件、未知字段/URL/数量边界。5 秒合成表达式的纯文本为 500000 quota，1 图 500500、2 图 501000、含视频 515000，证明复用了输入用量。
- `go test -mod=readonly ./... -count=1`、`go vet -mod=readonly ./...`、`go build -mod=readonly -o .local-tests/h3-media-new-api.exe .`、gofmt 和 diff 检查通过。
- Canvas 侧同时实现冻结媒体描述，真实报价入口测试覆盖同步前拒绝、同步后仅报价；能力同步新增绑定而不改历史绑定/价格。全量 lint/typecheck/test/build/runtime 通过，详见 Canvas `docs/newapi-pricing-sync-checkpoint.md`。

没有数据库、依赖、迁移或插件文件变更，无需数据迁移；默认 SQLite 桥接测试不代表未启用的 MySQL/PostgreSQL 验收。生产部署前保留两端旧镜像和生效插件版本；回滚代码后重同步目录，不能恢复旧账务数据库覆盖新流水。旧文本客户端兼容，旧宿主会拒绝新媒体字段，部署须两端配套。

交付目标为 `fork/main` 和中文附注 Tag `v1.0.0-rc.37.custom.13`。生产更新仍待授权：需要重建并更新 Go 宿主，单独上传 Hailuo JS 不能修复目录；更新后再同步原 Canvas 连接。尚未部署、未真实付费生成、未核验线上最终扣款。

## 2026-09-22 后续：H3 提交响应未知的重复保护

本地 Canvas 的一次 H3 创建在 New API `hailuo@1.1.4` 解析提交响应时返回 `missing task_id`。Canvas 将该发送意图记录为 `unknown`，后续 Worker 重试被“原请求可能已经送达，禁止重复创建”拦截；没有第二次 Canvas 创建 POST。该原请求没有平台任务号，必须由渠道或供应商日志核实，不能重新提交。

本次将 Hailuo 升级到 `1.1.5`，声明 `task-submit-no-retry@1`，并仅为 H3 `/v2/video_generation` 提交设置 `noRetry: true`。宿主对已经收到提交响应后的读取、解析或结果校验失败统一设置 `NoRetry`，防止 JSON 插件把未知受理结果当作可安全重试。旧 Hailuo `/v1` 描述不设置该选项，保持原有策略。

上游 `https://shumai.siphot.com` 从本机访问根路径、`/docs`、`/openapi.json` 和 H3 路径均返回区域阻断，New API 日志也未保存原始响应体。因此不能推测 `task_id` 的替代字段；待获得脱敏原始响应或供应商合同后，再为该精确格式补解析测试和适配。

验证使用合成 HTTP 响应和插件 hook，不发送付费创建请求：`TestTaskSubmissionAcceptanceAndSingleAttempt` 覆盖未声明 `noRetry` 的 JSON 解析失败仍不可重试；`TestHailuoH3BuildSubmitRequest` 与 `TestHailuoH3MappedUpstream` 覆盖 H3 描述，`TestHailuoLegacySubmitRequestUnchanged` 确认旧 `/v1` 路径不变。发布需更新整个 New API 宿主及其嵌入的 Hailuo 插件到 `v1.0.0-rc.37.custom.20`，然后核对实际插件版本；本记录不表示真实 H3 生成已验收。
