# Responses 公共协议与渠道兼容性

## 接入行为

客户端可向 `/responses` 或 `/v1/responses` 提交 Responses 请求。网关先按配置选择渠道，再选择原生 Responses、现有专用转换或通用 Chat 桥接。协议选择发生在请求上游之前，不在失败后探测另一协议并重复生成。

通用桥接流程为 `Responses → 现有 Chat 请求 DTO → 渠道原有上游协议 → 标准 Chat 响应 → Responses`。原有鉴权、模型转换、请求头、渠道参数覆盖和用量结算仍由对应模块负责。桥接返回原始渠道用量，包括 Anthropic 缓存计费元数据；适配器不自行预扣、结算或退款，外层统一处理一次。

OpenAI、DeepSeek 的标准 Responses 上游地址支持根域名或带 `/v1` 的地址。Ali、Cloudflare、Gemini、Azure、Vertex 等供应商专用路径沿用各自规则；“客户端无需手写 `/v1`”不表示所有供应商上游路径都改为 `/v1/responses`。

## 完整 APIType 矩阵

以下为本仓库 `main` 的代码支持矩阵。原生表示有该协议的转发实现，通用桥接表示复用该渠道现有文本能力，均不等同于每个模型已完成供应商真实验收。

| APIType | 渠道类型 | Responses 处理方式 | 边界 |
| --- | --- | --- | --- |
| 0 OpenAI | 1；兼容类型另见下文 | 原生 | 取决于实际上游模型的 Responses 能力 |
| 1 Anthropic | 14 | 通用 Chat 桥接 | 复用 Anthropic Messages 转换 |
| 2 PaLM | 11 | 通用 Chat 桥接 | 使用 `prompt.messages` 专用请求；不支持 `max_output_tokens`；未确认现役服务可用性 |
| 3 Baidu | 15 | 通用 Chat 桥接 | 复用旧文心协议 |
| 4 Zhipu | 16 | 通用 Chat 桥接 | 复用旧智谱协议 |
| 5 Ali | 17 | 原生 | 使用 Ali 专用 Responses 路径 |
| 6 Xunfei | 18 | 通用 Chat 桥接 | 实际 WebSocket 调用在渠道响应阶段执行 |
| 7 AIProxyLibrary | 21 | 明确 400 | 当前没有可分发的适配器 |
| 8 Tencent | 23 | 通用 Chat 桥接 | 保留腾讯签名和凭据格式 |
| 9 Gemini | 24 | 已有 Responses/Gemini 专用转换 | 保留现有转换规则 |
| 10 ZhipuV4 | 26 | 通用 Chat 桥接 | 复用 Chat 兼容接口 |
| 11 Ollama | 4 | 通用 Chat 桥接 | 支持原生逐行 JSON 响应解码 |
| 12 Perplexity | 27 | 原生 | 保留现有 `/v1/responses` 路径 |
| 13 Aws | 33 | 通用 Chat 桥接 | 包括既有 API Key、SDK Claude、SDK Nova 模式；未做 AWS 真实验收 |
| 14 Cohere | 34 | 通用 Chat 桥接 | 复用文本及逐行 JSON 流 |
| 15 Dify | 37 | 通用 Chat 桥接 | 复用应用对话接口与上下文规则 |
| 16 Jina | 38 | 明确 400 | 当前适配器仅 embedding/rerank |
| 17 Cloudflare | 39 | 原生 | 保留账户专用路径 |
| 18 SiliconFlow | 40 | 通用 Chat 桥接 | 复用 Chat 兼容接口 |
| 19 VertexAi | 41 | 通用 Chat 桥接 | 依模型选择 Claude、Gemini 或 OpenSource 协议 |
| 20 Mistral | 42 | 通用 Chat 桥接 | 保留工具标识规范化 |
| 21 DeepSeek | 43 | 原生 | 保留 DeepSeek V4 推理参数 |
| 22 MokaAI | 44 | 明确 400 | 当前适配器仅 embedding |
| 23 VolcEngine | 45 | 原生 | 保留火山供应商路径 |
| 24 BaiduV2 | 46 | 通用 Chat 桥接 | 复用 Chat 兼容接口 |
| 25 OpenRouter | 20 | 复用 OpenAI 原生 | 取决于实际上游能力 |
| 26 Xinference | 47 | 复用 OpenAI 原生 | 取决于部署服务能力 |
| 27 Xai | 48 | 原生 | 保留原生 Responses 处理 |
| 28 Coze | 49 | 通用 Chat 桥接 | 非流模式原有创建、轮询、取详情流程继续有效 |
| 29 Jimeng | 51 | 明确 400 | 实际接口为图像 `CVProcess` |
| 30 Moonshot | 25 | 通用 Chat 桥接 | 复用 Chat 兼容接口 |
| 31 Submodel | 53 | 通用 Chat 桥接 | 复用 Chat 兼容接口 |
| 32 MiniMax | 35 | 通用 Chat 桥接 | 仅本渠道现有文本模型能力 |
| 33 Replicate | 56 | 明确 400 | 当前适配器仅图像预测 |
| 34 Codex | 57 | 原生 | 原渠道不支持 Chat，不能改走 Chat 桥接 |
| 35 AdvancedCustom | 58 | 显式配置 | 保留原生、Responses→Chat 或 Responses→Gemini 路由 |

Azure 以及旧 OpenAI 兼容渠道继续使用既有 OpenAI 适配器。Midjourney、MidjourneyPlus、SunoAPI、Kling、Vidu、DoubaoVideo、Sora 等非文本任务渠道不能因默认 APIType 回落到 OpenAI 而发送 Responses 请求；这类请求在出站前明确拒绝。

## 功能边界

通用桥接提供无状态文本对话的公共入口，能够转换的工具调用、工具结果和图像内容仍受原有渠道及模型能力约束。不同渠道的文本接入能力不意味着工具、视觉、推理和结构化输出字段全部可用。转换器不能忠实表示的功能应明确报错，不以丢弃输入后返回成功代替兼容。

本次预检会拒绝 PaLM、Baidu、旧 Zhipu、Tencent、Xunfei、Cohere、Coze 以及 AWS Nova 适配器无法保留的工具或非文本内容。Dify 拒绝工具与非用户图像内容，Coze 拒绝非用户历史；Ollama 将工具结果的调用标识解析为该协议所需的工具名称。PaLM 没有输出 token 上限字段，显式请求 `max_output_tokens` 返回 400。上述限制来自当前适配器实现，不能仅靠更换模型名解除。

需要桥接的渠道不能同时启用全局或渠道级“原始请求体透传”；此冲突在出站前返回 400，避免把 Responses JSON 直接发送到 Chat 接口。原生渠道继续遵守原有透传配置。

原生 Responses 的 `previous_response_id`、服务端 conversation、托管 prompt、context management 和加密 reasoning 状态不能凭本地 Chat 桥接恢复；此类状态应使用支持它们的原生上游。客户端在无状态桥接中需要回传完整消息及工具往返历史。自定义工具的格式及语义必须遵守转换器和实际模型支持范围。

`/responses/compact` 与 `/v1/responses/compact` 的既有支持边界不因文本桥接扩大；不能把普通 Chat 摘要冒充服务端 compaction。原生 Gemini 与 AdvancedCustom 保留已有专用转换契约，本次通用桥接不重新定义其全部字段语义。

流式响应由 Responses 编码器输出；内部 Chat 的 JSON、逐行 JSON、`data: [DONE]` 不直接泄露到客户端协议。已开流后的处理失败通过流错误反馈并保留已取得的实际用量，避免重试生成、混入普通 JSON 错误尾巴或因整笔退款丢失已发生消费。

## 验证范围与证据

`relay/responses_all_channels_test.go` 包含全部 36 个 APIType 的真实请求转换用例：31 个原生/专用或桥接分支断言上游协议内实际用户文本，5 个非文本/缺失适配器断言 400；另覆盖 7 个任务渠道默认回落拒绝。转换用例只运行本地适配器，不访问外网。

代表性本地 HTTP 回归覆盖 Claude 文本、工具及缓存计费元数据，Cohere 原生逐行流，Ollama 原生逐行流及工具结果名称，PaLM 专用 JSON，Dify 应用 JSON，Mistral 工具历史标识和 Chat SSE。用例检查最终 Responses 文本、终态、工具和用量，以及原始请求、writer、RelayInfo 恢复；每例仅一次生成请求，包装器不调用计费会话的预扣、结算或退款方法。Claude 返回原始非缓存计费用量，同时客户端输入总数包含缓存读取与创建。该层证据保护不会产生第二次结算，完整账户结算仍由应用验收覆盖。

AWS SDK、Xunfei WebSocket、Nova 非流内部输出等特殊路径的编码器 fixture 只验证本地输出契约，不是供应商网络、签名或 SDK 实际调用验收。供应商已停止服务、账号额度、权限和模型能力不由本地 fixture 证明。

Go `1.26.0 windows/amd64` 下，下列验证通过：

```sh
go test ./relay -count=1 -timeout=180s
go vet -unreachable=false ./relay
git diff --check -- relay/responses_all_channels_test.go docs/responses-protocol-compatibility.md
```

`go test ./relay` 包含以上 36 个 APIType、7 个任务渠道、6 个本地 HTTP 场景及 PaLM 不支持输出上限时的 400 回归。定向 vet 排除了未修改旧渠道的既有不可达代码检查项，不能写成全量默认 vet 已通过。

部署和真实容器请求结果以 [部署核对](responses-deployment.md) 和 [Docker 验收](responses-docker-acceptance.md) 为准；不要将主分支本地成功直接表述为线上已经升级。
