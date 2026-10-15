# 秘塔 MiniMax H3 视频插件

`metaso-h3-video` 1.1.0 对外继续提供 OpenAI 视频入口 `/v1/videos`，上游改用秘塔的 MiniMax 原生 v2 协议。

- 渠道类型：Task Plugin，插件选择 `metaso-h3-video`。
- Base URL：`https://b.metaso.cn/api/minimax`。
- 对外模型：`Minimax-H3`，无需模型映射。插件向秘塔发送 `MiniMax-H3`，不会抢占 `hailuo` 的官方模型路由。
- 认证：渠道密钥通过 `Authorization: Bearer …` 发送。
- 提交：`POST /v2/video_generation`。
- 查询：`GET /v2/query/video_generation/{task_id}`。
- 下载：宿主代理查询响应中的 `task.content.url`，访问 CDN 时不携带渠道密钥。

## 请求

下面的请求发送到自己的 New API `/v1/videos`。将 `YOUR_NEW_API_TOKEN` 替换为 New API 令牌；秘塔密钥保存在渠道中。

```sh
curl --request POST 'https://YOUR_NEW_API/v1/videos' \
  --header 'Authorization: Bearer YOUR_NEW_API_TOKEN' \
  --header 'Content-Type: application/json' \
  --data '{
    "model": "Minimax-H3",
    "content": [
      {"type": "text", "text": "Animate the supplied image."},
      {"type": "image_url", "image_url": {"url": "https://YOUR_CDN/first.png"}, "role": "first_frame"}
    ],
    "resolution": "2K",
    "duration": 5,
    "ratio": "adaptive",
    "fast_mode": false
  }'
```

上游请求使用 `content / resolution / duration / ratio`，支持 `fast_mode`、`aigc_watermark`、`context_ir_enabled` 和 `callback_url`。显式 `false` 会保留。

兼容原有 `prompt / seconds / size / input_reference` 和 Canvas 的 `metadata.content`、`aspect_ratio`；`ratio`、`resolution` 及可选原生字段也可放在 `metadata`。同名顶层字段与 metadata 冲突时拒绝请求。媒体需使用公开 HTTP(S) URL，不支持直接文件上传。

原生 content 保留 `first_frame`、`last_frame`、`reference_image`、`reference_video` 和 `reference_audio` 角色，支持首尾帧与参考素材混用。旧 `images` 单图/双图默认转换为首帧/首尾帧；`reference_images`、全能参考模式、超过两张图片或同时提供视频/音频时转换为参考图。图片最多 9 张，视频和音频各 3 段，首尾帧各 1 张。

清晰度继续支持 `480p / 768P / 1080p / 2K`；旧像素尺寸仍可输入，转换为清晰度及横竖比例。按秘塔当前公开说明，480p/768P 支持 4–30 秒，1080p/2K 支持 4–15 秒。

## 更新已有渠道

加载 1.1.0 后，旧工厂地址 `https://metaso.cn/api/openai`（含 `/v1`、`/v1/videos` 后缀）会在请求构造时转换为新原生地址，无需修改数据库。自定义代理保留自己的主机与路径前缀，只切换至 v2 路径。后台建议将 Base URL 明确改为上面的新地址。

源码修改需重新构建并运行应用，或通过任务插件管理加载此 `plugin.js`。如果存在上传覆盖版本，确认实际生效版本为 1.1.0。

计费字段仍为 `seconds / size / input_images / input_videos / input_audios`。完成后读取 `task.usage.output_seconds`、`task.usage.input_image_count` 和 `task.resolution`；视频、音频输入秒数不会被误当成素材数量。此修改不自动更新价格表达式；秘塔的长视频、优先通道及输入素材价格需在渠道定价中另行匹配。

## 验证与依据

依据用户提供的 cURL，以及 2026-10-16 读取的[秘塔页面 API 文档](https://b.metaso.cn/minimax-h3)中的创建、查询和结果示例。该页面明确给出 `/api/minimax/v2/video_generation`、`/api/minimax/v2/query/video_generation/{task_id}` 和 `task.content.url`。

本地回归使用虚拟凭据，不提交真实生成任务：

```sh
go test ./plugins ./controller -run 'TestMetaso|TestCanvasMetaso|TestBuiltInTaskPluginResponsesAndUsageContracts' -count=1
```
