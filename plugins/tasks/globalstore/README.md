# GlobalStore 模型中心接入

GlobalStore v2 模型中心在本 fork 里由两条通路承载，shared 同一个上游账号、同一个 Base URL 与同一套上游任务接口：

| 媒体 | 通路 | 入口 | 上游 |
| --- | --- | --- | --- |
| 视频（15 个模型） | 任务插件 `globalstore`（渠道类型 Task Plugin） | `/v1/videos` | `POST /v2/model-center/tasks` → `GET /v2/model-center/tasks/{id}` |
| 图片（8 个模型） | 原生渠道类型 GlobalStore（类型 `62`） | `/v1/images/generations`、`/v1/images/edits` | 同上（适配器内部轮询，对外是同步图片响应） |

图片模型之所以不放在任务插件里：任务插件渠道只能声明 `openai_video` / `openai_responses` 两种宿主协议，而画布取模型走 `GET /v1/canvas/catalog`，该接口对「有插件绑定」的渠道只放行视频合同（`controller/canvas_bridge.go` 的 `canvasCandidate.supports`）。图片走原生渠道后，画布能像其它 OpenAI 图片渠道一样直接列出并调用。

## 对外模型

视频（任务插件 `globalstore`，走 `/v1/videos`）：`sd_2.0_discount`、`sd_2.0_fast_discount`、`sd_2.0_mini_discount`、`sd_2.5_discount`、`sd_2.0_special`、`sd_2.0_fast_special`、`sd_2.0_mini_special`、`sd_2.5_special`、`seedance_2.0_nsfw_c1`、`grok_video`、`grok_1.5`、`omni_720P`、`veo3.1-fast`、`wan3.0`、`wan3.0-Prime`。

图片（原生渠道 `GlobalStore`，走 `/v1/images/*`）：`banana2`、`bananapro`、`banana_c1`、`banana_c2`、`globalstore/gpt-image-2`、`gpt-image-2.5`、`seedream-5.0`、`seedream_5.0Pro`。

### 关于 `globalstore/gpt-image-2`

`gpt-image-2` 是 New API 内置的 OpenAI 图片模型名（`setting/billing_setting/builtin_billing.go` 里带固定价格表达式）。直接占用该名字会把内置模型改判成插件模型：内置计费表达式不再适用，`TestModelPricingConversionDatabaseMatrix` 会报 `fixed pricing is not supported for task usage expressions`。

两个通路因此都对内使用 `globalstore/gpt-image-2`，向上游仍发送 `gpt-image-2`（映射见 `plugin.js` 与 `relay/channel/globalstore/constants.go` 的 `upstreamModelNames`）。其余 22 个模型名与本 fork 的内置模型清单不冲突，保持与文档一致的原名。

## 部署

1. 建一个 **Task Plugin** 渠道，插件选 `globalstore`，Base URL `https://saas.globalstore.ai/kyyReactApiServer`，填 GlobalStore 密钥 → 提供 15 个视频模型。
2. 再建一个 **GlobalStore**（类型 `62`）渠道，Base URL 同上（留空也回落到该默认值），填同一个 GlobalStore 密钥 → 提供 8 个图片模型。
3. 两个渠道都要配价格，否则模型不会出现在画布的模型列表里（`controller/model.go` 的 `ListModels` 会跳过没有计费配置的模型）。图片模型配「按次」单价即可。

## 请求

下面的请求发送到自己的 New API。把 `YOUR_NEW_API_TOKEN` 换成 New API 令牌；GlobalStore 密钥保存在渠道里。

图片 · 文生图（`/v1/images/generations`）：

```sh
curl --request POST 'https://YOUR_NEW_API/v1/images/generations' \
  --header 'Authorization: Bearer YOUR_NEW_API_TOKEN' \
  --header 'Content-Type: application/json' \
  --data '{
    "model": "banana2",
    "prompt": "一只在沙滩上散步的猫",
    "aspect_ratio": "21:9",
    "resolution": "1K"
  }'
```

图片 · 参考图（`/v1/images/edits`，JSON）：`image` 传单个公网地址，`images` 传地址数组。GlobalStore 只接受**服务端可访问的公网 HTTP(S) 地址**，base64 与文件字节一律拒绝：

```sh
curl --request POST 'https://YOUR_NEW_API/v1/images/edits' \
  --header 'Authorization: Bearer YOUR_NEW_API_TOKEN' \
  --header 'Content-Type: application/json' \
  --data '{
    "model": "banana2",
    "prompt": "把背景换成正午的沙漠",
    "image": "https://cdn.example/reference.png",
    "aspect_ratio": "1:1",
    "resolution": "2K"
  }'
```

图片参数与 `plugin.js` 的模型声明一致：`aspect_ratio`（`ratio` 是别名）、`resolution`、`size`（与 `resolution` 互斥）、`quality`（仅 `banana_c1` / `globalstore/gpt-image-2` / `gpt-image-2.5` 声明）、`n` 只支持 1、`response_format` 只支持 `url`。不符合该模型声明的取值会在本地直接失败，不会先发给上游计费再被拒。

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

`/v1/videos` 接受 `prompt`、`ratio`、`resolution`、`duration`（`seconds` 亦可）、`size`、`seed`、`generate_audio`，以及按角色提供的参考素材：`image_url`（`first_frame` / `last_frame` / `reference_image`）、`video_url`（`reference_video`）、`audio_url`（`reference_audio`）。`omni_720P` 与 `veo3.1-fast` 固定时长（10s / 8s）；`grok_video`、`grok_1.5` 不接受默认宽高比，需要显式 `ratio`。超出该模型允许范围的字段（例如给不支持首帧的模型传 `last_frame`、给不支持的模型传 `seed` 或 `generate_audio`）会被拒绝。

## 计费

视频（任务插件的 `usageSchema`，与 `seedance-video` 通用插件同一种口径）：`count`、`seconds`、`video_resolution`。渠道定价里可以只给 `count` 配「按次调用」单价，也可以按 `video_resolution`（480p / 720p / 1080p / 2k / 4k）加 `seconds` 配「清晰度 + 秒数」，两种口径都由管理员在定价表达式里选择。

`video_resolution` 在提交阶段就已按模型支持列表归一化（未指定时取该模型支持列表的最后一项，与文档默认一致），`seconds` 取最终下发的时长（含 `seconds` / `duration` 别名与模型固定时长）。任务完成时只落 `count: 1`，`seconds` 与 `video_resolution` 沿用提交阶段冻结的值（`extractUsageOnComplete` 返回 `{count: 1}`），不会把参考音频/视频秒数当成素材数量。

图片（原生渠道）：走 New API 的原生图片计费，即「模型固定价 × 张数」，**只按次**，不会出现 enable/disable、音频时长等与该模型无关的字段。适配器只在图片任务真正 `completed` 时才结算用量；上游失败、连续报错或轮询超时都报错并不结算。

两个通路都不自动写入价格表达式；各模型价格需要在渠道定价里按上面的字段自行配置。

## 适配器行为（图片）

`relay/channel/globalstore/` 把上游的异步图片任务包装成同步的 OpenAI 图片响应：

- 提交：`POST {base}/v2/model-center/tasks`，`Authorization: Bearer <渠道密钥>`。
- 轮询：首次等待 5s，之后每 10s 一次，最多 20 次（总预算 210s）；连续 3 次请求出错即放弃；非 2xx 与未知状态都当作错误，不会被误判成任务结果。
- 收口：只有 `status == completed` 才返回图片并把用量记为 1 张；`failed` 直接把上游错误信息透出。
- 参考图：只接受绝对公网 HTTP(S) 地址（`image` / `images`）。multipart 上传的文件字节无法转发给上游，适配器会在请求前明确报错，而不是静默降级成文生图。

## 验证与依据

依据 2026-10-16 读取的 [GlobalStore 中文文档](https://docs.globalstore.ai/zh) 中 v2 模型中心的提交/查询/模型参数说明，以及文档列出的 `http://saas.globalstore.ai/gw_llm_power`（OpenAI 兼容）与 `https://saas.globalstore.ai/kyyReactApiServer`（媒体任务）两个入口。文档中的文本模型（11 个，走 OpenAI 兼容的 `gw_llm_power`）不在这两条通路里，需要按普通 OpenAI 兼容渠道单独接入。

`plugin.js` 与图片适配器都只依赖离线回归，不需要真实密钥或网络：

```sh
go test ./plugins -count=1
go test ./relay/channel/globalstore/ -count=1
go test ./controller -run 'TestCanvas' -count=1
```

实机联调需要一台能直连 `saas.globalstore.ai` 的机器：本机 IP 访问该域名会被 Cloudflare 403 拦截，因此仓库内不包含任何真实上游调用。
