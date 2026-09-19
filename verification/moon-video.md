# Moon 视频插件与 Wan3 测试检查点

初版 P1 基线为干净的 `main@9e9065727`，上游为 `fork/main`（`https://github.com/lysimportant/forknewapi.git`）。下方保留初版接入及验证记录；当前参考模式补齐的范围、检查点和验收见末节。验收以模拟供应商为主；不使用对话中的供应商密钥创建任务。

## 范围与兼容

- 新增 `moon` 任务插件，保留百炼、豆包与其他插件的原有协议和管理员价格。
- 复用现有 `/v1/videos` 及任务轮询、计费、制品机制。Wan 按秒计量；新三模型读取实际视频 tokens；不猜测 credits 兑换价格。
- 支持供应商 `202 Accepted` 任务受理。以显式 `task-submit-no-retry@1` 能力和 `noRetry` 提交选项，阻止结果未知时自动换渠道重复创建；旧插件默认重试行为不变。
- 无依赖、数据库 schema 或生产数据变更。旧宿主不支持新能力时拒绝插件加载，不能在旧版本静默忽略保护。
- 回滚可禁用 Moon 插件并还原本次应用版本；先等待在途 Moon 任务完成并保留既有价格、任务与日志，不删除用户配置。

## 初版基线与分工

Windows，Go `1.26.0`、Node `24.12.0`、仓库本地 Bun `1.4.2`；已安装前端依赖，未安装新包。

- 主代理：宿主提交边界、集中回归、HTTP 集成、检查点与最终 Git 交付。
- `moon_finish`：Moon 插件实现与格式检查。
- `moon_host_review`：只读复核重试保护、受理和退款链。
- `wan_test_script`：`verification/video-smoke.ps1` 与本地 HTTP 夹具验收。
- 初版阶段曾使用子代理继承主代理的配置；当前按全局 AGENTS 使用 `gpt-5.6-sol`、`max`。

## 初版进展

- [x] 当前插件基线：`go test -mod=readonly ./plugins ./pkg/jsplugin ./relay/channel/task/jsplugin -count=1 -timeout=180s` 通过。
- [x] 宿主定向回归：`go test -mod=readonly ./relay -run '^TestTaskSubmissionAcceptanceAndSingleAttempt$' -count=1 -timeout=90s` 通过，覆盖 202、原 200、服务端错误、缺少任务编号、损坏 JSON 和断开连接。
- [x] Moon 插件实现和协议边界复核；五个模型声明、计费字段中英文文案、大小写、模型别名、自动时长、参考素材限制、零用量和未知状态均已检查。
- [x] 模拟供应商提交、轮询、计费、错误与制品 HTTP 验收，25/25 通过。
- [x] 百炼 Wan3 测试脚本，本地 26 个 HTTP 场景通过，包括结果未知、超时恢复、并发提交、重定向及凭据检查。
- [x] Go 相关包测试、构建、vet、插件 lint/format、最终 diff 和密钥扫描通过。

交付目标为 `fork/main` 与中文 annotated Tag `v1.0.0-rc.37.custom.4`；提交和推送结果以最终 Git 远端核验为准。没有执行生产部署或真实付费生成。

协议依据：<https://moon.sixai.cc/video-api-docs.html>。未公开或未实际验证的供应商行为须在最终报告单独列出；模拟测试不证明账号权限、真实生成质量或供应商最终收费。

## 页面配置

先更新并重启本次后端版本；新宿主能力不能仅靠向旧版本上传 `plugin.js` 获得。进入「任务插件」，开启任务插件功能，确认内置 `Moon` 可用。新增渠道时选择任务插件 `Moon`（类型 61，`task_plugin_key=moon`），Base URL 填 `https://moon.sixai.cc`，已有 `https://moon.sixai.cc/v1` 也兼容。密钥只填渠道的密钥栏。

「配置模型」可从 Moon 插件声明中勾选下列模型，不代表账号实时模型发现或调用权限验证：

- `wan3.0-video`
- `wan3.0-video-prime`
- `seedance-2-0-mini-official`
- `seedance-2-0-fast-official`
- `seedance-2-0-official`
- `minimax-h3`

旧 `doubao-seedance-2-0-mini-260615`、`doubao-seedance-2-0-fast-260128`、`artsdance-2-0-pro-260801` ID 保留本地兼容；Moon 当前公开目录未承诺旧 ID 的调用权限。Moon H3 的精确 ID 是小写 `minimax-h3`，与官方渠道的 `MiniMax-H3` 分开。

配置分组、模型价格和令牌余额后再进行生成。Wan 用量字段为 `seconds`（输出秒数加参考视频秒数）、`resolution`（大写 `480P`/`720P`/`1080P`）；Seedance 为 `tokens`、小写 `resolution`、`video_input`；H3 为 `seconds`、小写 `resolution`（`480p`/`768p`/`1080p`/`2k`/`4k`）。计费表达式示意：每秒美元单价为 P 时使用 `u("seconds") * P`；每百万视频 tokens 美元单价为 P 时使用 `u("tokens") / 1000000 * P`，实际填写时将 P 替换为管理员确定的价格。任务表达式返回整次美元费用，没有聊天 token 表达式的隐式百万折算；不能直接填人民币报价或 Moon credits。已配置的同名模型价格保持不变。

官方阿里云 Key 继续使用「阿里云百炼」渠道（类型 17，对应已有 `alibaba` 插件）。Workspace Base URL 填根地址 `https://llm-i33ji9l2iva3glia.cn-beijing.maas.aliyuncs.com`，删除末尾 `/compatible-mode/v1`。视频上游提交为根地址加 `/api/v1/services/aigc/video-generation/video-synthesis`，查询为 `/api/v1/tasks/{id}`。模型仍填写精确的 `wan3.0-video`；Moon 插件用于 Moon 渠道。

如果通过 Responses 获取视频展示链接，系统中的 `TaskPublicAddress`（若设置）或 `ServerAddress` 必须指向实际可访问的网关地址，不能保留默认的 `http://localhost:3000`；宿主按照该配置生成制品 URL。

## 验证一次 Wan3 生成

渠道「测试」当前是同步接口测试，没有异步视频提交、轮询和下载闭环。插件调试页只运行钩子，不会调用上游；不能将它的成功视作生成成功。

在 PowerShell 7 中先预览，将网关地址和渠道编号替换为实际值。这里的地址是自己的 new-api，不能填写 Moon 或阿里云供应商地址：

```powershell
pwsh -File D:\newapi\verification\video-smoke.ps1 -BaseUrl "https://你的-new-api" -ChannelId 12
```

确认后加 `-Submit`，脚本安全提示输入管理员账号创建的 API 令牌，也支持既有 `NEW_API_TEST_TOKEN` 环境变量。它以令牌渠道后缀固定到所选渠道，默认提交英文海浪提示词、`wan3.0-video`、2 秒、480p，这一步会产生真实生成费用：

```powershell
pwsh -File D:\newapi\verification\video-smoke.ps1 -BaseUrl "https://你的-new-api" -ChannelId 12 -Submit
```

实际请求为 `POST /v1/videos`，JSON 含 `model`、`prompt`、`seconds`、`resolution`；收到网关任务 ID 仅说明受理。随后 `GET /v1/videos/{task_id}` 到 `completed`，再 `GET /v1/videos/{task_id}/content` 下载并检查 MP4 头。脚本成功后应打开视频确认能播放，并在任务/使用日志确认最终状态、用量和结算。

检查点默认在 `.local-tests/moon-video/checkpoint.json`，视频在同目录 `video.mp4`。网络断开或轮询超时，重新运行相同命令即可继续查询；已有检查点时即使带 `-Submit` 也不会再 POST。若提交结果未知且检查点没有编号，先根据提交时间从网关日志核对，拿到原任务编号后加 `-TaskId task_...` 恢复。不要删除未知结果的检查点后直接重新生成。独立的新测试使用新的 `-CheckpointPath` 和输出位置。

脚本只保证客户端单次 POST，百炼仍沿用宿主既有重试策略；Moon 则另外声明了服务端禁止提交重试能力。下载不跟随重定向，不把网关令牌带到 CDN；MP4 头校验不是可播放性或画质验收。

## 初版验证记录与限制

- 完整 HTTP 验收于北京时间 2026-09-19 00:04 完成，`.local-tests/moon-video/report.json` 记录 `success=true`、25 个通过步骤、0 个失败步骤。真实网关二进制在禁网容器、只读根目录及全新 tmpfs SQLite 中运行，上游为本地模拟服务；没有连接生产或真实供应商。
- HTTP 覆盖五个 Moon 模型、自动时长、待结算/缺少用量/零用量、生成失败退款、百炼 Wan3 兼容、Responses 后台/同步/流式、GET/HEAD 制品及 CDN 无凭据。配置 3 次重试时 Moon 503 仍只提交一次，各任务幂等键不同；逐笔用量、退款、日志和最终余额一致，总计扣除 700000 quota。
- `go test -mod=readonly ./plugins ./pkg/jsplugin ./relay ./relay/channel ./relay/channel/task/jsplugin -count=1 -timeout=180s` 通过。
- `go test -mod=readonly ./controller ./service ./router -count=1 -timeout=240s` 通过。
- `go vet -mod=readonly ./...`、`go build -mod=readonly ./...` 通过；最终源码另构建 Linux 无 CGO 二进制用于完整 HTTP 验收。
- `node web/node_modules/oxlint/bin/oxlint -c plugins/.oxlintrc.json plugins/tasks/moon/plugin.js` 和 `node web/node_modules/oxfmt/bin/oxfmt --config plugins/.oxfmtrc.json --check plugins/tasks/moon/plugin.js` 通过。
- PowerShell `7.6.5` 语法、帮助及 26 个本地场景通过，证据 `.local-tests/moon-video/script-run-1789746486452/result.json`；脚本测试使用模拟 MP4 头，未调用真实供应商。
- 隔离 HTTP 首轮揭示 Wan 分辨率枚举大小写错误，已在两个协议的解码阶段修复；自动时长使用内部标记避开宿主非负时长限制，上游仍收到 `-1`。失败提交退款为异步，夹具等待最终余额后断言。
- `noRetry` 拒绝宿主可直接重放的明确空正文；非空 JSON 保留幂等头，但移除 Go 的正文重放函数。未知提交结果不自动换渠道或重发；客户端另发新 HTTP 请求仍可能创建新任务。
- 初版因缺少参考输入文档仅开放 Moon Wan 文生视频；该限制由末节的参考模式实现替代。仍不套用阿里云完成用量。Seedance 等待有效 `usage.total_tokens`，缺失、异常或待结算状态不提前发布制品。
- Moon 完成回包含直接视频 URL 时下载不附带渠道凭据；回退 `/content` 若带凭据并跨域重定向，会被宿主既有安全限制拒绝。没有放宽重定向或 SSRF 规则。
- 无前端文件、依赖、数据库结构或数据库访问实现变更，无生产部署。真实账号权限、余额、供应商实际生成和播放质量需通过用户执行上述付费测试确认。

完整 HTTP 夹具为 `.local-tests/moon-video/runner.mjs`，使用以下隔离命令执行；本地证据和二进制不纳入 Git：

```powershell
docker run --rm --pull never --name moon-video-http-20260919 --network none --read-only --tmpfs /data:rw,nosuid,size=256m --tmpfs /tmp:rw,nosuid,size=128m --mount type=bind,source=D:/newapi/.local-tests/moon-video,target=/out --entrypoint node node:24.12.0-bookworm-slim /out/runner.mjs
```

验收产物 SHA256：

- `plugins/tasks/moon/plugin.js`：`7F7CDD4EDE0CF528B035925ED122A93F0C983132F7E908EC10626BAC6AA5EA46`
- `.local-tests/moon-video/new-api`：`6A4404C1F911AC4A8B204E1BD356F2CBD9A6DEB7D1B50DAB673CC9FE4DC77E83`

模拟视频仅含 24 字节 MP4 文件头，用于校验下载链路，不是可播放的供应商成片。

## 2026-09-19 参考模式补齐

本轮 P1，基线为干净的 `main@38dbb951d`，跟踪 `fork/main`。Go `1.26.0`、Node `24.12.0`，沿用已安装依赖；`go test -mod=readonly ./plugins ./pkg/jsplugin ./relay/channel/task/jsplugin -count=1 -timeout=180s` 基线通过。

目标为 Moon Wan3、Seedance、H3 的已公开参考模式映射、输入边界和计费用量回归，替代上文旧版仅开放 Wan 文生视频的限制。验收采用真实 JS 运行时及禁网网关加模拟供应商；没有执行真实付费生成或生产部署。

影响包括插件请求合同及画布素材传输。保留既有模型 ID 和管理员价格；新 ID 的公开声明不代表账号权限。无数据库迁移，不修改用户画布或凭据。回滚需先等待在途任务结束，再还原插件和相关画布版本；保留任务、价格与日志。

公开合同来源：[Moon 视频文档](https://moon.sixai.cc/video-api-docs.html)、[模型与 H3 尺寸目录](https://moon.sixai.cc/h3-catalog.json)。Wan 使用 `reference_images/videos/audios`，视频每段 2–15 秒、合计不超过 15 秒；输出 2–30 秒。Seedance 支持 `content` 或简化素材数组、首尾帧、全能参考及 `edit`/`extend`；编辑需要自动时长和 `adaptive` 比例。H3 使用原生 `workflow_id`、`seconds`、`size`，按工作流限制图片、视频和音频，超分使用 `cf-*`、`2K`/`4K` 和显式比例。

Canvas Wan 的时长以 `metadata.reference_video_durations` 数组附加，顺序与 `metadata.input.media` 中的视频一致；Moon 将其映射为参考视频的 `duration`，百炼继续仅消费原来的 `input`。时长来源为选定素材版本的 `metadata.durationSeconds`，不能用输出时长替代或自行猜测。Moon 参考素材需 HTTP(S) URL，Wan 原生请求也可用 `file_id`；画布托管素材需配置指向同一存储对象、可被上游访问的 `MC_S3_PROVIDER_ENDPOINT`。本地回环地址和 Base64 不能替代 Moon 的公网素材地址。

Moon Wan 公开合同未支持 `negative_prompt` 或 `duration=-1`，插件会明确拒绝，不静默删除；官方百炼继续保留负向提示和智能时长。两者共用 `wan3.0-video` ID，Canvas 无法仅凭模型名区分渠道：Moon 的总引用数 12、参考视频合计 15 秒等额外限制由网关插件在预扣费和供应商创建 POST 前检查。Canvas 到网关的请求可能先被发送并得到明确拒绝，这不代表已经调用或扣费于真实供应商。

- [x] 核实 Moon 当前公开文档：Wan 使用 `reference_images/videos/audios`，视频时长必填；Seedance 支持 `content` 和简化素材数组；H3 使用工作流、`size`、`seconds` 及 URL 素材。
- [x] 复现现有 Canvas 元数据参考输入被拒绝、H3 未注册的问题。
- [x] 插件映射、校验及集中协议测试；Moon 1.1.0 支持 Wan/Seedance/H3 已确认的原生和 Canvas 参考输入，Responses 图片引用按系列转换。
- [x] 画布 URL、冻结素材时长和精确模型 ID 的兼容核对；73 组 Canvas → 实际插件解码 → 上游创建体映射通过，包含官方 Wan 智能时长和比例保留。
- [x] 网关 HTTP 提交、查询、制品、计费与失败退款验收：禁网容器 36 项通过、0 失败，实际网关进程和模拟上游合计结算 1700000 quota，与每笔日志及余额一致。
- [x] 最终差异、检查点和凭据检查；交付使用 `fork/main` 与 annotated Tag `v1.0.0-rc.37.custom.7`，提交和远端一致性在最终交接中核验。

主代理负责集中测试、HTTP 验收与交付；`moon_reference_plugin` 负责插件；`moon_reference_docs` 只读核实供应商合同；`canvas_reference_runtime` 核对画布到网关的参考素材链路。子代理采用 `gpt-5.6-sol`、`max`。

上述为初期分工。最终复核由主代理接管；按用户最新要求，全局子代理默认已改为继承主代理模型和推理强度，原 Sol 子代理已停止。

宿主允许 Moon `minimax-h3` 与官方 `MiniMax-H3` 并存：精确 ID 独立注册、路由及定价，大小写折叠有歧义时不自动选择。插件 API v1 的文字说明已同步，无 schema 或类型字段变更；旧宿主会拒绝该插件与官方 H3 共存，应更新网关宿主和 Moon 插件。

本轮验证：`go test -mod=readonly ./pkg/jsplugin ./middleware -count=1 -timeout=240s`、插件与 relay 相关包、controller/service/router 回归、`go vet -mod=readonly ./...`、`go build -mod=readonly ./...`、Moon oxlint/oxfmt 和插件 CLI lint 通过。首次全插件回归遇到既有 Grok multipart 字段顺序断言偶发失败，复查后再次运行通过；没有修改 Grok 行为或测试。

本轮禁网验收于北京时间 2026-09-19 07:26 完成，证据为 `.local-tests/moon-video/report.json` 与 `http-tests.log`。包括缺少参考时长、超界时长、错误编辑参数和 Base64 参考在预扣前拒绝，Wan 输出及参考秒数计费，Seedance 实际/零 tokens 结算，H3 工作流及 `metadata.url` 制品，原有百炼和 Responses 合同。二进制 SHA256 为 `16C63FF98F83A742D86EDDEFC876761EEACDF10922914CC48319487A8EFC8B53`，插件 SHA256 为 `AA355CD9609B095E0C4A83EA07F194C7BED4D64A6EA22608446ACD587DB58CBB`。未连接供应商，模拟 MP4 只用于下载协议验证。

Canvas 配套在 `G:\multimodal-canvas` 完成：全量 lint/typecheck/test/build 通过（已有设施跳过保留），实际 PostgreSQL 版本时长到本插件的组合验证 4/4、浏览器 Mock 9/9、跨仓库映射 73 组通过。本地 8080 的 API、Worker、Web 已更新并健康，用户资产和画布未改写；线上 New API 仍需更新宿主与 Moon 1.1.0，配置公网素材地址后再独立验收真实生成。
