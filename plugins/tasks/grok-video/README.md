# Grok 视频：第三方 OpenAI 兼容插件

本插件面向第三方提供的 OpenAI 视频兼容接口，用于 Grok 视频的 `/v1/videos` 兼容接口。插件版本 `2.0.0`。支持 `grok-imagine-video-1.5`、`grok-imagine-video-1.5（按次）`、`grok-imagine-video-1.5.1` 和 `grok-imagine-video`。模型名按供应商要求原样配置；Helunox 的 `（按次）` 是实际请求模型名的一部分，不可删除。

各模型分别匹配渠道和价格配置，共享按秒和清晰度计费用量及480p/720p/1080p配置入口，不自动互相改名或复制商业价格。上游须支持你选择的模型与清晰度。内置插件随镜像更新；手动上传的旧插件需更新到2.0.0，并在渠道模型列表添加`grok-imagine-video`后配置该模型价格。

## 安装与入口

- 本地官方插件位于 `plugins/tasks/*/plugin.js`，由 `plugins/embed.go` 自动嵌入。本 fork 新增 `grok-video`，保留原有 Alibaba、Doubao、Google、Hailuo、Jimeng、Kling、Sora、SunoAPI、Vertex AI、Vidu。
- 拉取本次 main 并重新构建、替换应用容器后，超级管理员在 `/task-plugins` 的已安装列表可看到 Grok。无需另外下载插件。
- 不重建时，也可将本目录 `plugin.js` 通过“任务插件 → 上传插件”安装到 rc.35；不要同时安装不同 key、相同模型的重复插件。
- 在“渠道”新建 **Task Plugin / 任务插件** 类型，选择 `grok-video`，填第三方 API 地址、密钥、分组与模型 `grok-imagine-video-1.5`，启用并保存。插件不抢占已有 OpenAI、Sora 或 XAI 渠道类型。
- 地址可以是 `https://供应商域名` 或 `https://供应商域名/v1`，也支持保留自定义路径前缀。也接受完整 `/v1/videos` 地址，提交和查询前会统一规范化，避免重复拼接。

## 定价与清晰度

进入 `/system-settings/billing/model-pricing`。先配置启用渠道；刷新后选中模型重新编辑，避免 rc.35 新建模型表单未加载插件 schema 的现有问题。

1. 切换“表达式 / 配置任务定价”，以 `resolution` 分档，为每档 `seconds` 填写**每秒美元单价**。
2. 收费为 **秒数 × 对应清晰度每秒单价**，之后由宿主应用分组倍率及管理员显式请求规则。不要选“按请求”固定价，否则仍按固定单价结算。

请求用量为 `seconds` 和 `resolution`，不再提供 `count`。成功响应明确返回有效 `seconds` 或 `duration` 时按实际秒数结算；未返回时保留请求秒数，清晰度保持请求档位。非法或冲突的上游时长会使插件完成用量钩子报错；当前宿主忽略该钩子错误并保留预扣用量，不会按非法值或零值结算。失败退款沿用宿主生命周期。插件不自行扣款、退款，不预设商业售价。

**升级前必须重新配置价格：**旧 `count` 表达式和固定按次价格不会自动换算或覆盖。先停用相关渠道并等待在途任务结束，备份插件和各模型价格，再更新插件、将四个模型分别配置为按秒分档表达式，验证后启用。模型名中的 `（按次）` 是供应商标识，仍原样转发，不决定本系统对用户的收费方式。

主界面按 [Grok 官方视频文档](https://docs.x.ai/developers/model-capabilities/video/generation) 显示 `480p`、`720p`、`1080p`。文档明确 1.5 的文生视频与图生视频支持 1080p，未列出 2K/4K。未填写时使用 `unspecified`，显示为“供应商默认（未指定）”，不伪装成某个清晰度；分档定价时仍应配置该默认项。

旧版本的 `4k` 请求与价格仅为兼容既有第三方配置而保留，界面单独放入“供应商其他规格”，不表示 xAI 官方支持，也未验证第三方实际上采样或生成 4K 的能力。本版不增加没有官方依据的 2K。请求仍使用顶层 `resolution` 并原值透传；兼容供应商使用 `size` 文字标签时提取相同计费档。像素短边 480、720、1080 对应官方三档，2160 仅保留旧 4K 兼容；其他尺寸明确拒绝。`resolution` 与 `size` 必须对应同一档，否则返回错误，防止按低档收费而请求高档。

## 本版接口契约

客户端通过 `POST /v1/videos` 提交 JSON 或 multipart，必填 `model`、`prompt` 和 `seconds` 或 `duration`；支持单个 `input_reference` 文件。JSON 无文件请求发 JSON，有文件发 multipart。可传时长、清晰度等供应商参数，但供应商须支持对应字段。

```json
{
  "model": "grok-imagine-video-1.5",
  "prompt": "海浪拍打礁石，镜头缓慢推进",
  "seconds": 10,
  "resolution": "720p"
}
```

- 上游创建：`POST /v1/videos`，Bearer 渠道密钥；优先读取顶层 `id` 或 `task_id`，保留 `request_id` 兼容。不凭未知包装字段猜测任务 ID。
- 上游查询：`GET /v1/videos/{上游ID}`；支持 `queued/pending/processing/in_progress/completed/failed/cancelled/canceled/expired`，未知状态返回 UNKNOWN。
- 视频下载：优先使用顶层 `video_url`、`url` 或 `video.url` 提供的公开 CDN 地址，经宿主无凭据代理；没有 URL 时使用鉴权 `GET/HEAD /v1/videos/{上游ID}/content`。私有 CDN 签名/鉴权差异、特殊 JSON 包装或强制 multipart 创建需要按供应商文档适配。
- 可经 `/v1/responses`（本 fork 也支持 `/responses`）用纯字符串 `input` 发起文生视频，支持宿主的同步、流式和 background。本版不接受 Responses 数组历史、工具、图片或会话恢复，显式报错；图生视频使用 `/v1/videos`。
- 每请求只允许一个视频，拒绝 `n/count/batch_size` 非 1；时长必填且必须大于 0 且不超过宿主 3600 秒安全边界。此边界不表示供应商支持 3600 秒；未指定时长会拒绝请求，不猜测默认值。metadata/parameters 内禁止携带时长、清晰度或数量参数，计费参数只接受顶层值。

## 验证和范围

```sh
GOWORK=off go run -mod=readonly . plugin lint plugins/tasks/grok-video/plugin.js
GOWORK=off go test -mod=readonly ./plugins ./pkg/jsplugin ./relay/channel/task/jsplugin -count=1 -timeout=180s
GOWORK=off go vet ./plugins
node web/node_modules/oxlint/bin/oxlint -c plugins/.oxlintrc.json plugins/tasks/grok-video/plugin.js
node web/node_modules/oxfmt/bin/oxfmt -c plugins/.oxfmtrc.json --check plugins/tasks/grok-video/plugin.js
```

使用真实插件运行时验证模型路由、JSON/multipart、四档清晰度、按秒用量、防重复倍率、失败状态、无凭据制品下载及参数边界。完整应用的模拟供应商冒烟记录见下方 2.0.0 检查点。未调用真实付费供应商，不代表实际生成质量、4K 能力或供应商特殊协议已通过验收。

本次不改数据库、Go/前端依赖或原有插件。若第三方协议不符，可停用对应渠道；不要通过降低计费校验或猜测响应字段来掩盖不兼容。

2026-09-08 1.0.0 / custom.2 历史验收：上述插件测试、lint/format/vet通过；完整Docker构建成功，镜像实际启动返回精确版本 `v1.0.0-rc.35.custom.2`，首页正常。精确镜像程序两轮隔离HTTP分别12/12通过，包括声明字段、清晰度冲突400且不扣费/不出站、按次表达式及ModelPrice固定单价成功/失败、下载。

最终镜像ID：`sha256:6dd301a7940ca384e266a6b297ae852bb954b985d09fca0984ae2f7f500ad8d5`；二进制SHA256：`51E01F579BB6249CADA96E3EFF25A18F0D221062A599D9930CC4E5B85C4BB099`。本地证据在忽略目录 `D:/newapi/.local-tests/grok-http/` 的 `report.json`、`supplemental-report.json`、`final-image-manifest.json`；未提交测试数据库或凭据。

## 1.1.0 / custom.3

模型任务定价和插件详情改为按品牌原生规格排序，主入口最多五档、不凑数；有官方型号限制时按型号筛选，额外规格与默认价格独立保留。可灵、即梦显示官方模式/产品的清晰度对应，计费仍使用原生 Units / product。

Grok 本版主列官方三档，保留旧4K兼容，不增加2K。已有表达式和金额不自动迁移。手动上传过1.0.0的实例需上传1.1.0更新说明；内置版本随镜像更新。完整的11插件范围、官方来源、未知项和验收见[品牌视频规格记录](../../../verification/rc35/video-brand-pricing.md)。

## 1.1.1

新增grok-imagine-video模型匹配，原1.5模型保留；两个模型的公开视频与Responses路由均经真实插件注册表验证。Go插件三包回归、前端清晰度7项测试、typecheck、生产build、插件宿主lint及定向lint/format通过。未调用付费上游。


## 1.1.2：Helunox 实测与检查点（2026-09-09）

一次 POST `/v1/videos`，模型原样为 `grok-imagine-video-1.5（按次）`，JSON 包含 prompt 和 seconds="10"。实测 HTTP 200、application/json，顶层 id/task_id、status=queued；后续 GET `/v1/videos/{id}` 返回 in_progress。未重复提交。该响应本来就符合旧版 ID 解析，不能据此认定先前错误由缺少 request_id 导致。

本版修复完整地址误拼接为 `/v1/videos/v1/videos` 的确定性路径问题；无法确认线上渠道当时是否填写完整地址。非 JSON 响应明确报错但不回显正文或密钥。计费规则不变，不修改生产渠道或数据库；回滚可重新上传旧插件并将渠道地址改为根地址或 /v1。同一任务后续返回 done、progress=100，video.url 为 /v1/videos/{id}/content。鉴权 GET 内容接口实测 HTTP 200、video/mp4，Content-Length=8133203（只检查响应，不保存视频）。插件仅把当前任务的标准相对内容路径转为渠道鉴权下载；其他相对路径拒绝，外部 CDN 仍无凭据访问。

检查点：Go 1.26.0 windows/amd64；`go test ./plugins ./pkg/jsplugin ./relay/channel/task/jsplugin -count=1 -timeout=180s`、`go vet ./plugins`、`go build -o .local-tests/grok-verify.exe .`、构建程序的 `plugin lint`、定向 oxlint/oxfmt 与 `git diff --check` 均通过。线上插件仍需上传或更新部署，未修改生产渠道。

## 2.0.0：按秒和清晰度计费（2026-09-09）

- 计费契约改为 seconds × resolution 档位单价；保留全部模型名、完整地址规范化、done 状态和内容下载兼容。
- 不修改数据库、依赖或其它插件，不自动迁移已有价格；本次未操作生产或调用付费供应商。
- 回滚：停用渠道并排空在途任务，恢复 1.1.2 插件及其配套按次价格备份，再验证并启用。不能只回滚插件而保留新表达式。
- 检查点：基线 main/1caaf132e，工作区干净；Go 1.26.0、Node 24.12.0；Bun 不在 PATH，使用现有本地 Node 工具进行插件 lint/format，无需安装依赖。基线三包测试通过；变更后七包回归、go vet ./plugins、Windows/Linux 应用 build、plugin lint、oxlint、oxfmt 及 diff 检查通过。

验证命令：`GOWORK=off go test -mod=readonly ./plugins ./pkg/jsplugin ./relay/channel/task/jsplugin ./pkg/billingexpr ./service ./relay/helper ./common -count=1 -timeout=180s`，`go vet ./plugins`，`go build -mod=readonly -o .local-tests/grok-seconds-verify.exe .`；JS 工具命令见上文。

隔离 HTTP 冒烟：无网络、只读容器内启动本次构建的完整应用和 loopback 模拟上游，临时 SQLite 位于 tmpfs，不挂载生产数据。第一轮 15/15 通过，覆盖五档价格、4 秒预扣/37 秒实际结算、用户及令牌账本、失败退款、GET/HEAD 视频下载、非法输入400且不扣费/不出站；最终二进制复验13/13通过，追加缺失时长及 Responses 隐藏计费字段拒绝。只使用测试单价，不代表商业售价。证据分别在忽略目录 `.local-tests/grok-seconds-http/report.json` 和 `.local-tests/grok-seconds-final/report.json`；应用首页200，进程正常退出。未重新执行三库迁移矩阵，因为没有数据库行为变更。

最终根模块全量回归：`GOWORK=off go test -mod=readonly ./... -count=1 -timeout=180s` 通过，日志位于 `.local-tests/grok-seconds-full-test.log`。没有前端代码或 relaykit 变更；本次没有运行前端构建或 relaykit 独立构建。部署剩余步骤：备份并排空在途任务、更新插件2.0.0、重新配置各模型每秒分档价格，再进行受控上线验收。
