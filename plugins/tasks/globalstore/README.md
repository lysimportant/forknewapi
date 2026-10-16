# GlobalStore 模型中心插件

`globalstore` 1.0.0 把 GlobalStore v2 模型中心的图片与视频任务接入 New API：图片走仅后台模式的 Responses 入口 `/v1/responses`，视频走标准视频入口 `/v1/videos`。两个入口共用同一个渠道密钥和同一套上游任务接口。

- 渠道类型：Task Plugin，插件选择 `globalstore`。
- Base URL：`https://saas.globalstore.ai/kyyReactApiServer`。
- 对外模型：8 个图片模型 + 15 个视频模型，见下节。
- 认证：渠道密钥通过 `Authorization: Bearer …` 发送。
- 提交：`POST /v2/model-center/tasks`。
- 查询：`GET /v2/model-center/tasks/{id}`。
- 下载：宿主代理结果里的 `result_url` / `video_url` / `image_url`，取 CDN 时不携带渠道密钥。

## 对外模型

图片（`/v1/responses`，仅后台模式）：`banana2`、`bananapro`、`banana_c1`、`banana_c2`、`globalstore/gpt-image-2`、`gpt-image-2.5`、`seedream-5.0`、`seedream_5.0Pro`。

视频（`/v1/videos`）：`sd_2.0_discount`、`sd_2.0_fast_discount`、`sd_2.0_mini_discount`、`sd_2.5_discount`、`sd_2.0_special`、`sd_2.0_fast_special`、`sd_2.0_mini_special`、`sd_2.5_special`、`seedance_2.0_nsfw_c1`、`grok_video`、`grok_1.5`、`omni_720P`、`veo3.1-fast`、`wan3.0`、`wan3.0-Prime`。

### 关于 `globalstore/gpt-image-2`

`gpt-image-2` 是 New API 内置的 OpenAI 图片模型名（`setting/billing_setting/builtin_billing.go` 里带固定价格表达式）。插件若直接占用该名字，会把内置模型改判成任务插件模型：内置计费表达式不再适用，`TestModelPricingConversionDatabaseMatrix` 会报 `fixed pricing is not supported for task usage expressions`。

插件因此对外使用 `globalstore/gpt-image-2`，向上游仍发送 `gpt-image-2`（映射见 `plugin.js` 的 `UPSTREAM_MODEL_NAMES`）。其余 22 个模型名与本 fork 的内置模型清单不冲突，保持与文档一致的原名。

## 请求

下面的请求发送到自己的 New API。把 `YOUR_NEW_API_TOKEN` 换成 New API 令牌；GlobalStore 密钥保存在渠道里。

图片（`/v1/responses`，必须 `background`）：

```sh
curl --request POST 'https://YOUR_NEW_API/v1/responses' \
  --header 'Authorization: Bearer YOUR_NEW_API_TOKEN' \
  --header 'Content-Type: application/json' \
  --data '{
    "model": "banana2",
    "background": true,
    "input": "一只在沙滩上散步的猫",
    "aspect_ratio": "21:9",
    "resolution": "1K"
  }'
```

视频（`/v1/videos`）：

```sh
curl --request POST 'https://YOUR_NEW_API/v1/videos' \
  --header 'Authorization: Bearer YOUR_NEW_API_TOKEN' \
  --header 'Content-Type: application/json' \
  --data '{
    "model": "sd_2.0_special",
    "prompt": "镜头缓慢推进的海岸线",
    "duration": 5,
    "resolution": "720p",
    "ratio": "16:9",
    "generate_audio": true
  }'
```

`/v1/responses` 只接受单轮用户消息（字符串 `input` 或一条 `role: "user"` 消息），内容只支持文本与普通参考图；工具、历史消息、系统角色、多模态音视频输入一律拒绝。可用的别名参数为 `prompt / metadata / content / reasoning_effort / aspect_ratio / ratio / resolution / size / quality / n / response_format / reference_images / images / input_reference`，顶层出现未列出的字段会直接报错。

`/v1/videos` 接受 `prompt`、`ratio`、`resolution`、`duration`（`seconds` 亦可）、`size`、`seed`、`generate_audio`，以及按角色提供的参考素材：`image_url`（`first_frame` / `last_frame` / `reference_image`）、`video_url`（`reference_video`）、`audio_url`（`reference_audio`）。`omni_720P` 与 `veo3.1-fast` 固定时长（10s / 8s）；`grok_video`、`grok_1.5` 不接受默认宽高比，需要显式 `ratio`。超出该模型允许范围的字段（例如给不支持首帧的模型传 `last_frame`、给不支持的模型传 `seed` 或 `generate_audio`）会被拒绝。

## 计费字段

计费字段为 `seconds / resolution / images / count / quality / input_images / input_videos / input_audios / audio_mode`。任务完成后按上游回执读取，不会把参考音频/视频秒数当成素材数量。此插件不自动写入价格表达式；GlobalStore 各模型的价格需要按上面的字段在渠道定价里自行配置。

## 验证与依据

依据 2026-10-16 读取的 [GlobalStore 中文文档](https://docs.globalstore.ai/zh) 中 v2 模型中心的提交/查询/模型参数说明，以及文档列出的 `http://saas.globalstore.ai/gw_llm_power`（OpenAI 兼容）与 `https://saas.globalstore.ai/kyyReactApiServer`（媒体任务）两个入口。插件只实现后者的图片与视频任务；文档中的文本模型未纳入本插件。

`plugin.js` 只依赖离线回归，不需要真实密钥或网络：

```sh
go test ./plugins -count=1
go test ./plugins -run 'TestGlobalstore|TestBuiltInTaskPluginResponsesAndUsageContracts|TestBuiltInResponsesDecodersEchoChannelMappedAlias' -count=1
```

实机联调需要一台能直连 `saas.globalstore.ai` 的机器：本机 IP 访问该域名会被 Cloudflare 403 拦截，因此仓库内不包含任何真实上游调用。
