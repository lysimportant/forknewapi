# Responses 路径与部署复核检查点

## 本轮目标与基线

- P0：修复根路径 `/responses` 返回 HTML、DeepSeek/OpenAI 上游 `/v1` 拼接不一致，确认线上 `not implemented` 的实际渠道及部署差异。
- 起点 `618894395`，分支 `codex/video-usage-pricing`，上游 `fork/codex/video-usage-pricing`；工作区初始干净。
- Go `go1.26.0 windows/amd64`；定向基线 `GOWORK=off go test ./router ./relay ./relay/channel/openai ./relay/channel/deepseek ./relay/common -count=1` 通过。
- 用户部署方式：先 git pull，再 `docker compose up -d --build`。本次无数据库、依赖或前端变更，无迁移；回滚到上一代码版本并重建应用即可。

## 已确认事实

- 线上流式与最简非流式 `/v1/responses` 都返回 500 / `convert_request_failed: not implemented`；根路径 `/responses` 返回 HTML 200。
- 使用用户 Key 的只读 `/api/log/token` 查询确认：两次复测命中渠道 ID 37、渠道类型 43（DeepSeek），请求路径 `/v1/responses`。
- 复测请求 ID：`202609071536305166767688268d9d6UyOC8zEU`。本地当前 DeepSeek `ConvertOpenAIResponsesRequest` 已返回有效请求，不含该占位错误；当前线上执行代码与本地实现不一致，需确认拉取分支/运行镜像/代理目标。
- `VERSION` 原文件长度为 0；Dockerfile 从该文件注入 `common.Version`，所以源码构建可能报告空版本。填入本次发布版本，后续以接口版本与固定 tag 验证部署。

## 实施状态

- [x] 功能分支根路径别名复用既有鉴权、限流、插件与 Responses 处理，内部规范化路径，不重定向 POST。
- [x] 功能分支四类原生渠道上游根地址自动补 `/v1`，已有 `/v1` 去重，保留其他渠道及协议特殊路径。
- [x] 功能分支填入版本 `v1.0.0-rc.33.responses.2-preview`；main 回补版为 `v1.0.0-rc.33.responses.2`，避免本地 Docker 构建空版本。
- [x] 功能分支回归、独立 review、全量 test/vet/build 通过。
- [x] main 回补的完整可执行应用 + 隔离 SQLite + DeepSeek 43 渠道 + 真实 Sub2 上游冒烟通过：根路径工具调用与版本路径工具结果回传两轮均 200/completed，最终 OK，用量及两条消费日志一致，实际仅两次上游请求。
- [x] main 完整 test/build、独立 review 通过；全量 vet 被 17 条已核实基线告警阻断（锁值拷贝、IPv6 测试地址、旧渠道不可达代码）。定向 `go vet -unreachable=false ./router ./relay/common ./relay/channel/deepseek` 通过，未扩大本次生产代码范围。
- 交付引用：用户 fork 的 `main` / `v1.0.0-rc.33.responses.2`；功能分支同步 `codex/video-usage-pricing` / `v1.0.0-rc.33.responses.2-preview`。部署以 main 对应版本为准，推送状态由远端引用核对。
- [ ] 线上升级后验收：尚无服务器连接，不能把本地成功标记为线上修复成功。

真实凭据只用于进程内存与 stdin；测试报告不保存请求鉴权头。

## 默认分支差异

- `fork/main=1e88cbaaa`，DeepSeek 转换函数仍返回 `not implemented`；功能分支起点 `618894395` 已实现，main 落后 145 个提交。
- 默认分支 `git pull` 不会自动获得功能分支补丁，这是前次交付未覆盖实际部署分支的问题；不归因于 Docker 缓存。
- 已在独立 worktree 从 main 准备定向回补，避免同时引入功能分支的大量数据库、依赖和协议模块迁移。main 仅回补实际已有的 OpenAI/DeepSeek 标准路径、DeepSeek 原生 Responses、根路径别名及明确版本。
- 功能分支本轮全量 `GOWORK=off go test ./...`、`go vet ./...`、`go build ./...` 已通过，独立路由/URL review 通过。
- main 完整应用实测使用源码编译二进制，公开状态接口返回 `v1.0.0-rc.33.responses.2`。第一轮 input/output 387/77，第二轮 482/2，消费日志一致；报告保存在忽略目录，真实凭据不进入数据库或日志。
- main 原有 service 缓存测试在 Windows 以时间戳生成键时碰撞；还原本轮生产补丁的 overlay 同样复现。精确回补既有 `25a60e31e` 的六行测试名称隔离后，全量 test 通过，未改变缓存生产行为。
