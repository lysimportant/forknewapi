# Responses 渠道兼容修复检查点

## 目标与基线

- P0：解决 Responses 渠道转换缺口，保留 DeepSeek V4 原生请求语义，正确处理流式终态与异常。
- 用户已授权扩展渠道支持；原生能力优先，已确认 Chat 契约的渠道集中转换。不以错误字符串触发第二次上游请求。
- 验收：出站请求、工具回传、终态与断流回归，HTTP 冒烟，Go test/vet/build；线上部署验收独立记录。
- 分支 `codex/video-usage-pricing`，起点 `25a60e31e9c5274ce1c28398086c37b69e705c28`，开始时工作区干净。
- 上游 `fork/codex/video-usage-pricing`，远程 `https://github.com/lysimportant/forknewapi.git`。
- Go `go1.26.0 windows/amd64`；根模块与 relaykit 声明 Go 1.25.1，根模块通过本地 replace 使用 relaykit。
- 无数据库、依赖、前端变更；无需迁移。回滚使用本次提交的 revert，不覆盖工作区或强推。

## 已验证证据（2026-09-07）

- 同一模型 `deepseek-v4-flash-vision-exp`、同一 `/v1/responses` 请求：线上 new-api HTTP 500，`convert_request_failed: not implemented`；直连 Sub2 HTTP 200，收到 `response.completed`。
- 线上命中渠道类型与运行版本仍未知；当前仓库 DeepSeek/OpenAI 原生路径已有实现，不能据此反推线上失败渠道。
- OpenAI 原生转换曾把 DeepSeek V4 `max` 改为 `xhigh`；修复前回归失败，修复后通过。这是另一个已证实的兼容缺陷，不作为线上 500 的唯一根因。
- 本地修改版 OpenAI 适配器经真实 Sub2 完成 `read_file` 调用与工具结果回传，两轮均 HTTP 200 / `response.completed`，最终文本 `OK`。用量分别 input 388 / output 70、input 480 / output 2，出站 effort 保持 `max`。
- 真实冒烟覆盖适配器与上游，不覆盖生产部署、完整 Controller 或钱包扣款。测试凭据仅通过内存/stdin 使用，不进入源码或报告。

## 实施状态

- [x] DeepSeek V4 原生参数保真；OpenAI、Sub2、高级自定义回归。
- [x] 原生 SSE 扩展事件保真，终态即时结束，失败/截断/写失败记录，部分用量与终态零值处理。
- [x] 五种原生渠道本地 HTTP 两轮工具调用冒烟。
- [x] 六种 Chat 渠道集中转换及流/非流 HTTP 冒烟初验；保留原渠道地址、鉴权，拒绝与请求体透传组合。
- [x] Chat 转换流生命周期与工具输入复核：截断、超时、读写错误不伪造完成；保留尾包/失败用量；不可保真的历史及工具返回 400。
- [x] 根模块全量 test/vet/build、diff 与敏感信息检查通过。
- [x] 独立 review 发现的 Mistral 二次转换参数丢失已修复：前后字段检查，错误不含值；16 项回归红→绿，relay test/vet 通过。
- Git 交付定位：中文提交 `fix(responses): 完善原生转发与多渠道协议兼容`，annotated tag `v1.0.0-rc.33.responses.1`；推送目标为上述 fork 分支，最终状态以 Git 提交与远端 refs 为准。
- [ ] 生产部署与 Codex 经线上 new-api 验收：缺少部署目录/连接方式及实际渠道配置，未操作生产环境。

## 验证记录

- 修改前定向基线通过：`go test ./relay/channel/openai ./relay/helper ./controller -run 'Test(OaiResponses|StreamScannerHandler|ShouldRetry)' -count=1`。
- 第一阶段根模块 `GOWORK=off go test ./...`、`go vet ./...`、`go build ./...` 通过。
- 第一阶段 relaykit 独立 `GOWORK=off go test ./...`、`go vet ./...`、`go build ./...` 通过；本次未改 relaykit 源码或接口。
- 原生 usage 二次边界修正后，六相关包完整 test、openai/helper vet 通过；独立只读 review 未发现阻断问题。
- 六渠道转换初验：`go test ./relay -run 'TestResponses(ChatFallback|NativeChannels)' -count=1` 通过。
- 桥接生命周期与入口集成完成后，`GOWORK=off go test ./...`、`go vet ./...`、`go build ./...` 全部通过；`TestResponsesHelperChatRouting` 验证实际入口的模型映射、Chat 转换、参数覆盖和上游错误保留。
- Mistral 最终补丁落盘后，再次运行根模块上述三项全量检查，全部通过；所有任务 Go 文件通过 gofmt 检查，diff 和凭据模式检查通过。

兼容配置与限制见 [responses-compatibility.md](responses-compatibility.md)。
