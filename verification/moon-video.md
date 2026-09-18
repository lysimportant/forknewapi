# Moon 视频插件与 Wan3 测试检查点

本轮 P1，基线为干净的 `main@9e9065727`，上游为 `fork/main`（`https://github.com/lysimportant/forknewapi.git`）。目标是接入 Moon 的五个视频模型，并给出百炼 Wan3 可复现的提交、查询与视频下载验证方法。验收以模拟供应商为主；未授权真实生成，不使用对话中的供应商密钥创建任务。

## 范围与兼容

- 新增 `moon` 任务插件，保留百炼、豆包与其他插件的原有协议和管理员价格。
- 复用现有 `/v1/videos` 及任务轮询、计费、制品机制。Wan 按秒计量；新三模型读取实际视频 tokens；不猜测 credits 兑换价格。
- 支持供应商 `202 Accepted` 任务受理。以显式 `task-submit-no-retry@1` 能力和 `noRetry` 提交选项，阻止结果未知时自动换渠道重复创建；旧插件默认重试行为不变。
- 无依赖、数据库 schema 或生产数据变更。旧宿主不支持新能力时拒绝插件加载，不能在旧版本静默忽略保护。
- 回滚可禁用 Moon 插件并还原本次应用版本；先等待在途 Moon 任务完成并保留既有价格、任务与日志，不删除用户配置。

## 基线与分工

Windows，Go `1.26.0`、Node `24.12.0`、仓库本地 Bun `1.4.2`；已安装前端依赖，未安装新包。

- 主代理：宿主提交边界、集中回归、HTTP 集成、检查点与最终 Git 交付。
- `moon_finish`：Moon 插件实现与格式检查。
- `moon_host_review`：只读复核重试保护、受理和退款链。
- `wan_test_script`：`verification/video-smoke.ps1` 与本地 HTTP 夹具验收。
- 用户中途调整的全局规则已生效：子代理继承主代理模型和推理强度，后续创建不指定 `model`、`reasoning_effort`。

## 进展

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
- `doubao-seedance-2-0-mini-260615`
- `doubao-seedance-2-0-fast-260128`
- `artsdance-2-0-pro-260801`

配置分组、模型价格和令牌余额后再进行生成。Wan 用量字段为 `seconds`、`resolution`（大写 `480P`/`720P`/`1080P`）；新三模型为 `tokens`、小写 `resolution`、`video_input`。计费表达式示意：每秒美元单价为 P 时使用 `u("seconds") * P`；每百万视频 tokens 美元单价为 P 时使用 `u("tokens") / 1000000 * P`，实际填写时将 P 替换为管理员确定的价格。任务表达式返回整次美元费用，没有聊天 token 表达式的隐式百万折算；不能直接填人民币报价或 Moon credits。已配置的同名模型价格保持不变。

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

## 验证记录与限制

- 完整 HTTP 验收于北京时间 2026-09-19 00:04 完成，`.local-tests/moon-video/report.json` 记录 `success=true`、25 个通过步骤、0 个失败步骤。真实网关二进制在禁网容器、只读根目录及全新 tmpfs SQLite 中运行，上游为本地模拟服务；没有连接生产或真实供应商。
- HTTP 覆盖五个 Moon 模型、自动时长、待结算/缺少用量/零用量、生成失败退款、百炼 Wan3 兼容、Responses 后台/同步/流式、GET/HEAD 制品及 CDN 无凭据。配置 3 次重试时 Moon 503 仍只提交一次，各任务幂等键不同；逐笔用量、退款、日志和最终余额一致，总计扣除 700000 quota。
- `go test -mod=readonly ./plugins ./pkg/jsplugin ./relay ./relay/channel ./relay/channel/task/jsplugin -count=1 -timeout=180s` 通过。
- `go test -mod=readonly ./controller ./service ./router -count=1 -timeout=240s` 通过。
- `go vet -mod=readonly ./...`、`go build -mod=readonly ./...` 通过；最终源码另构建 Linux 无 CGO 二进制用于完整 HTTP 验收。
- `node web/node_modules/oxlint/bin/oxlint -c plugins/.oxlintrc.json plugins/tasks/moon/plugin.js` 和 `node web/node_modules/oxfmt/bin/oxfmt --config plugins/.oxfmtrc.json --check plugins/tasks/moon/plugin.js` 通过。
- PowerShell `7.6.5` 语法、帮助及 26 个本地场景通过，证据 `.local-tests/moon-video/script-run-1789746486452/result.json`；脚本测试使用模拟 MP4 头，未调用真实供应商。
- 隔离 HTTP 首轮揭示 Wan 分辨率枚举大小写错误，已在两个协议的解码阶段修复；自动时长使用内部标记避开宿主非负时长限制，上游仍收到 `-1`。失败提交退款为异步，夹具等待最终余额后断言。
- `noRetry` 拒绝宿主可直接重放的明确空正文；非空 JSON 保留幂等头，但移除 Go 的正文重放函数。未知提交结果不自动换渠道或重发；客户端另发新 HTTP 请求仍可能创建新任务。
- Moon Wan 的旧参考输入和完成用量合同未取得有效文档，本版只开放文生视频，并按请求输出秒数结算；不假装兼容阿里云返回用量。新三模型等待有效 `usage.total_tokens`，缺失、异常或待结算状态不提前发布制品。
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
