# Grok 视频：第三方 OpenAI 兼容插件

本插件面向第三方提供的 OpenAI 视频兼容接口，不适用于 xAI 官方 `/v1/videos/generations`。模型 API 名为 `grok-imagine-video-1.5`；“（按次）”是计费说明，不属于模型名。插件版本 `1.0.0`。

## 安装与入口

- 本地官方插件位于 `plugins/tasks/*/plugin.js`，由 `plugins/embed.go` 自动嵌入。本 fork 新增 `grok-video`，保留原有 Alibaba、Doubao、Google、Hailuo、Jimeng、Kling、Sora、SunoAPI、Vertex AI、Vidu。
- 拉取本次 main 并重新构建、替换应用容器后，超级管理员在 `/task-plugins` 的已安装列表可看到 Grok。无需另外下载插件。
- 不重建时，也可将本目录 `plugin.js` 通过“任务插件 → 上传插件”安装到 rc.35；不要同时安装不同 key、相同模型的重复插件。
- 在“渠道”新建 **Task Plugin / 任务插件** 类型，选择 `grok-video`，填第三方 API 地址、密钥、分组与模型 `grok-imagine-video-1.5`，启用并保存。插件不抢占已有 OpenAI、Sora 或 XAI 渠道类型。
- 地址可以是 `https://供应商域名` 或 `https://供应商域名/v1`，也支持保留自定义路径前缀。不要填写完整 `/videos` 路径。

## 定价与清晰度

进入 `/system-settings/billing/model-pricing`。先配置启用渠道；刷新后选中模型重新编辑，避免 rc.35 新建模型表单未加载插件 schema 的现有问题。

1. **清晰度统一价格：**切换“按请求”，填写每次生成的美元单价。
2. **清晰度分别定价：**切换“表达式 / 配置任务定价”，以 `resolution` 分档，为每档 `count` 填每次单价。

用量始终为 `count = 1`；`seconds` 和 `duration` 不参与乘价。成功不因上游返回更长时长增加次数；失败退款由宿主任务结算处理。插件不自行扣款、退款，也不预设免费价格或商业售价。分组倍率及管理员显式请求规则仍由宿主执行。

清晰度选项：`480p`、`720p`、`1080p`、`4k`。未填写时使用 `unspecified`，保留供应商默认行为，不伪装成 480p。若采用分档价格，应配置该默认档，或要求客户端明确传入清晰度。

**这些是用户要求的配置选项，不是供应商能力证明。** 尚未收到第三方文档或真实响应；尤其 `4k` 必须由供应商实际支持。请求使用顶层 `resolution`，原值透传；兼容供应商若用 `size` 传这四种文字标签，也会提取相同计费档。像素尺寸按短边 480、720、1080、2160 分别归入四档；其他尺寸明确拒绝。`resolution` 与 `size` 同时提供时必须对应同一档，否则返回错误，避免请求高档而按低档计费。实际像素尺寸仍原样提交，是否支持取决于供应商。

## 本版接口契约

客户端通过 `POST /v1/videos` 提交 JSON 或 multipart，必填 `model`、`prompt`；支持单个 `input_reference` 文件。JSON 无文件请求发 JSON，有文件发 multipart。可传时长、清晰度等供应商参数，但供应商须支持对应字段。

```json
{
  "model": "grok-imagine-video-1.5",
  "prompt": "海浪拍打礁石，镜头缓慢推进",
  "resolution": "720p"
}
```

- 上游创建：`POST /v1/videos`，Bearer 渠道密钥；响应顶层 `id` 或 `task_id`。
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

2026-09-08 最终验收：上述插件测试、lint/format/vet通过；完整Docker构建成功，镜像实际启动返回精确版本 `v1.0.0-rc.35.custom.2`，首页正常。精确镜像程序两轮隔离HTTP分别12/12通过，包括声明字段、清晰度冲突400且不扣费/不出站、按次表达式及ModelPrice固定单价成功/失败、下载。

最终镜像ID：`sha256:6dd301a7940ca384e266a6b297ae852bb954b985d09fca0984ae2f7f500ad8d5`；二进制SHA256：`51E01F579BB6249CADA96E3EFF25A18F0D221062A599D9930CC4E5B85C4BB099`。本地证据在忽略目录 `D:/newapi/.local-tests/grok-http/` 的 `report.json`、`supplemental-report.json`、`final-image-manifest.json`；未提交测试数据库或凭据。
