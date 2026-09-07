# Responses 渠道兼容与验收

客户端可使用 `POST /responses` 或 `POST /v1/responses`；根路径在内部规范化后复用原鉴权、限流、插件与中继链，不重定向 POST。`/responses/compact` 和已有 retrieve 路径也提供相同别名。渠道选择优先保留上游原生 Responses；只有已确认支持 Chat Completions 的渠道才自动进行协议转换，不在上游失败后重发请求探测协议。

## 渠道选择

| 类型 | 行为 |
| --- | --- |
| OpenAI、NewAPI、Sub2API、DeepSeek 等已有原生渠道 | 使用原生 Responses 路径；上游必须实际支持该端点和模型 |
| SiliconFlow、Mistral、Moonshot、MiniMax、Submodel、百度 V2 | 自动将 Responses 转为渠道 Chat 请求，并将响应转回 Responses |
| Claude、Gemini | 保留已有专门转换路径 |
| 高级自定义 | 尊重配置的原生、Responses→Chat 或 Responses→Gemini 转换方式 |
| AWS、Vertex、腾讯 TC3 等专有协议及非聊天渠道 | 本次未新增通用桥接，不能将其裸响应当作 OpenAI Chat 使用 |

这里的兼容表示网关有对应请求/响应路径，不保证每个供应商模型都支持工具、图片或全部 Responses 功能。六种新增转换路径已完成本地 HTTP 契约验证，尚未逐一使用真实供应商账号验收。

## Sub2 与 DeepSeek V4 推荐配置

- 对提供标准 `/v1/responses` 的 Sub2 上游，优先选择 Sub2API 或 OpenAI 渠道，Base URL 填上游根地址，模型使用上游目录中的精确 ID，例如 `deepseek-v4-flash-vision-exp`。
- OpenAI、NewAPI、Sub2API、DeepSeek 的标准 Responses 上游可填写根域名或带 `/v1` 的地址，自动补齐或去重后发送 `/v1/responses`；保留自定义路径前缀。其他协议仍遵循各渠道规则，非标准上游可用高级自定义显式设置路径。
- 原生 DeepSeek V4 保留 `reasoning.effort=max`；GPT 模型和跨协议转换维持原有 OpenAI 推理参数映射。
- Chat 桥接渠道须关闭全局和渠道请求体透传；冲突返回 400，避免把 Responses 请求体发到 Chat 端点。
- Chat 桥接请求的参数覆盖在转换后执行，应使用 Chat 字段，例如 `max_tokens`；原生渠道仍使用 Responses 字段。
- Mistral 的既有渠道转换会重建请求。若因此删除结构化输出、推理、`store:false` 等参数，新桥接会返回 400 并列出字段名；请按实际需求调整参数或使用原生 Responses 渠道。基础文本与 function 工具请求保留既有转换。

## 协议边界

- Chat 桥接不提供 Responses 服务端会话存储。`previous_response_id`、`conversation`、`prompt`、`context_management` 在请求上游前返回不可重试的 400；客户端需要传完整历史及工具结果。
- `/v1/responses/compact` 保留原有能力检查，不自动转为普通 Chat 请求。
- 内置搜索、文件检索、计算机使用等能力受转换器与上游支持限制；不应假设等同于原生 Responses 的全部工具能力。
- 新增 Chat 桥接仅接受可保真的普通消息与 `function_call` / `function_call_output` 历史。`custom_tool_call_output`、`reasoning` 密文/摘要、`item_reference` 等历史及不可转换的工具声明返回 400，提示改用原生渠道，避免变为空消息或被静默删除。Codex 的这类会话应使用原生 Sub2API/OpenAI/DeepSeek 路径。
- 原生 SSE 按原事件转发；completed/done 即结束。失败、未完成、异常 EOF 和写失败记录 `stream_status`，保留已产生用量，防止流输出后重试、退款或追加普通 JSON。

## 实测与部署边界

2026-09-07 对照中，线上 new-api 返回 HTTP 500 / `convert_request_failed: not implemented`，Sub2 相同请求成功。修改版适配器通过真实 Sub2 完成两轮工具往返，最终 `OK`。这证明修改版原生转发可工作，不代表线上 new-api 已部署修复，也不能确定其实际命中的渠道类型。

后续复核已确认失败来自渠道 37 / DeepSeek（类型 43），fork 默认 main 的该转换函数仍是 `not implemented`，修复此前只在功能分支。main 落后功能分支 145 个提交，因此单独回补 DeepSeek 原生支持、根路径与 URL 规范化，不整体引入数据库和依赖变更。本文六渠道桥接等内容描述功能分支；main 的本轮补丁范围以其 `docs/responses-deployment.md` 为准。main 发布版本为 `v1.0.0-rc.33.responses.2`，功能分支同步版本为 `v1.0.0-rc.33.responses.2-preview`。

尚未取得生产部署连接信息；当前未修改线上配置或服务。无数据库迁移，回滚使用此前服务版本及对应提交的 revert。
