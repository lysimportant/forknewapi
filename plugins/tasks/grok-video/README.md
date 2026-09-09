# Grok 视频：第三方 OpenAI 兼容插件

本插件面向第三方提供的 OpenAI 视频兼容接口，用于 Grok 视频的 `/v1/videos` 兼容接口。插件版本 `1.1.2`。支持 `grok-imagine-video-1.5`、`grok-imagine-video-1.5（按次）`、`grok-imagine-video-1.5.1` 和 `grok-imagine-video`。模型名按供应商要求原样配置；Helunox 的 `（按次）` 是实际请求模型名的一部分，不可删除。

各模型分别匹配渠道和价格配置，共享按次计费用量及480p/720p/1080p配置入口，不自动互相改名或复制商业价格。上游须支持你选择的模型与清晰度。内置插件随镜像更新；手动上传的旧插件需更新到1.1.2，并在渠道模型列表添加`grok-imagine-video`后配置该模型价格。

## 安装与入口

- 本地官方插件位于 `plugins/tasks/*/plugin.js`，由 `plugins/embed.go` 自动嵌入。本 fork 新增 `grok-video`，保留原有 Alibaba、Doubao、Google、Hailuo、Jimeng、Kling、Sora、SunoAPI、Vertex AI、Vidu。
- 拉取本次 main 并重新构建、替换应用容器后，超级管理员在 `/task-plugins` 的已安装列表可看到 Grok。无需另外下载插件。
- 不重建时，也可将本目录 `plugin.js` 通过“任务插件 → 上传插件”安装到 rc.35；不要同时安装不同 key、相同模型的重复插件。
- 在“渠道”新建 **Task Plugin / 任务插件** 类型，选择 `grok-video`，填第三方 API 地址、密钥、分组与模型 `grok-imagine-video-1.5`，启用并保存。插件不抢占已有 OpenAI、Sora 或 XAI 渠道类型。
- 地址可以是 `https://供应商域名` 或 `https://供应商域名/v1`，也支持保留自定义路径前缀。也接受完整 `/v1/videos` 地址，提交和查询前会统一规范化，避免重复拼接。

## 定价与清晰度

进入 `/system-settings/billing/model-pricing`。先配置启用渠道；刷新后选中模型重新编辑，避免 rc.35 新建模型表单未加载插件 schema 的现有问题。

1. **清晰度统一价格：**切换“按请求”，填写每次生成的美元单价。
2. **清晰度分别定价：**切换“表达式 / 配置任务定价”，以 `resolution` 分档，为每档 `count` 填每次单价。

用量始终为 `count = 1`；`seconds` 和 `duration` 不参与乘价。成功不因上游返回更长时长增加次数；失败退款由宿主任务结算处理。插件不自行扣款、退款，也不预设免费价格或商业售价。分组倍率及管理员显式请求规则仍由宿主执行。

主界面按 [Grok 官方视频文档](https://docs.x.ai/developers/model-capabilities/video/generation) 显示 `480p`、`720p`、`1080p`。文档明确 1.5 的文生视频与图生视频支持 1080p，未列出 2K/4K。未填写时使用 `unspecified`，显示为“供应商默认（未指定）”，不伪装成某个清晰度；分档定价时仍应配置该默认项。

旧版本的 `4k` 请求与价格仅为兼容既有第三方配置而保留，界面单独放入“供应商其他规格”，不表示 xAI 官方支持，也未验证第三方实际上采样或生成 4K 的能力。本版不增加没有官方依据的 2K。请求仍使用顶层 `resolution` 并原值透传；兼容供应商使用 `size` 文字标签时提取相同计费档。像素短边 480、720、1080 对应官方三档，2160 仅保留旧 4K 兼容；其他尺寸明确拒绝。`resolution` 与 `size` 必须对应同一档，否则返回错误，防止按低档收费而请求高档。

## 本版接口契约

客户端通过 `POST /v1/videos` 提交 JSON 或 multipart，必填 `model`、`prompt`；支持单个 `input_reference` 文件。JSON 无文件请求发 JSON，有文件发 multipart。可传时长、清晰度等供应商参数，但供应商须支持对应字段。

```json
{
  "model": "grok-imagine-video-1.5",
  "prompt": "海浪拍打礁石，镜头缓慢推进",
  "resolution": "720p"
}
```

- 上游创建：`POST /v1/videos`，Bearer 渠道密钥；优先读取顶层 `id` 或 `task_id`，保留 `request_id` 兼容。不凭未知包装字段猜测任务 ID。
- 上游查询：`GET /v1/videos/{上游ID}`；支持 `queued/pending/processing/in_progress/completed/failed/cancelled/canceled/expired`，未知状态返回 UNKNOWN。
- 视频下载：优先使用顶层 `video_url`、`url` 或 `video.url` 提供的公开 CDN 地址，经宿主无凭据代理；没有 URL 时使用鉴权 `GET/HEAD /v1/videos/{上游ID}/content`。私有 CDN 签名/鉴权差异、特殊 JSON 包装或强制 multipart 创建需要按供应商文档适配。
- 可经 `/v1/responses`（本 fork 也支持 `/responses`）用纯字符串 `input` 发起文生视频，支持宿主的同步、流式和 background。本版不接受 Responses 数组历史、工具、图片或会话恢复，显式报错；图生视频使用 `/v1/videos`。
- 每请求只允许一个视频，拒绝 `n/count/batch_size` 非 1；显式时长必须大于 0 且不超过宿主 3600 秒安全边界。此边界不表示供应商支持 3600 秒；未指定时长则不填默认值。

## 验证和范围

```sh
GOWORK=off go run -mod=readonly . plugin lint plugins/tasks/grok-video/plugin.js
GOWORK=off go test -mod=readonly ./plugins ./pkg/jsplugin ./relay/channel/task/jsplugin -count=1 -timeout=180s
GOWORK=off go vet ./plugins
node web/node_modules/oxlint/bin/oxlint -c plugins/.oxlintrc.json plugins/tasks/grok-video/plugin.js
node web/node_modules/oxfmt/bin/oxfmt -c plugins/.oxfmtrc.json --check plugins/tasks/grok-video/plugin.js
```

使用真实插件运行时验证模型路由、JSON/multipart、四档清晰度、按次用量、防重复倍率、失败状态、无凭据制品下载及参数边界。完整应用的模拟供应商冒烟记录见本次提交说明。未调用真实付费供应商，不代表实际生成质量、4K 能力或供应商特殊协议已通过验收。

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
