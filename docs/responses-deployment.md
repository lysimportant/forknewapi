# Responses 修复与部署核对

## 修复范围

- DeepSeek 渠道（类型 43）现在执行原生 Responses 转发，不再在请求转换阶段返回 `not implemented`；保留 DeepSeek V4 的 `reasoning.effort=max` 和现有思考后缀语义。
- 客户端可使用 `POST /responses` 或 `POST /v1/responses`；compact 也提供根路径别名。别名内部规范化后复用既有鉴权、限流与分发，不重定向 POST、不落入前端 HTML。
- OpenAI 与 DeepSeek 的标准 Responses 上游可填根域名或带 `/v1` 的 Base URL，自动补齐或去重 `/v1`，保留子路径前缀。其他供应商专用路径与其他协议沿用原契约。
- `VERSION` 设置为 `v1.0.0-rc.33.responses.2`；本地 Compose 构建通过原 Dockerfile 注入二进制，状态接口和版本响应头不再为空。
- 本次 main 回补不包含功能分支的其他新渠道、数据库、依赖或 relaykit 迁移，无数据库迁移。

## 已确认的线上失败原因

用户重新部署后，`/v1/responses` 的流式和最简非流式请求仍返回 500 / `convert_request_failed: not implemented`。只读令牌日志确认命中渠道 ID 37、类型 43（DeepSeek），请求 ID 为 `202609071536305166767688268d9d6UyOC8zEU`。

此前修复只在 `codex/video-usage-pricing`，fork 默认 `main` 的 DeepSeek 仍是占位实现。在 main 执行 `git pull`、`docker compose up -d --build` 会构建 main 自身，不能获得另一个分支的修复。main 与功能分支相差 145 个提交，因此本次只回补本问题需要的改动。

## 部署

在当前生产仓库目录执行，先确认当前分支是 `main`，远端指向自己的 `forknewapi`，本地源码改动已处理：

```sh
git status --short
git branch --show-current
git remote get-url origin
git pull --ff-only origin main
cat VERSION
```

`VERSION` 必须输出 `v1.0.0-rc.33.responses.2`。确认后重建应用服务：

```sh
docker compose up -d --build --no-deps new-api
docker compose exec -T new-api /new-api --version
curl -sS https://api.lolicon.beer/api/status
```

容器 `--version` 与外部 `/api/status` 的 `data.version` 都应为该版本。如果容器版本正确而外部版本仍旧，应检查反向代理目标或其他实例。无需重建数据库/Redis，也无需删除数据卷。

在 Codex 中可以只填 `https://api.lolicon.beer`，由客户端追加 `/responses`；原先填写 `https://api.lolicon.beer/v1` 的配置也继续有效。上游已支持标准 `/v1/responses` 时，无需更换现有 DeepSeek 渠道。

## 验收边界与回滚

本地已从本次 main 回补源码编译完整 `new-api` 可执行程序，使用隔离 SQLite、正常登录/令牌鉴权、DeepSeek 类型 43 渠道完成实际 HTTP 冒烟。上游使用用户提供的 Sub2 服务，真实 Key 只在回环代理进程内存中使用，未写入测试数据库。模型为 `deepseek-v4-flash-vision-exp`：

| 请求 | 结果 | 输入 / 输出 token |
| --- | --- | --- |
| `POST /responses` | HTTP 200，`response.completed`，一次 `read_file` 调用 | 387 / 77 |
| `POST /v1/responses` 回传工具结果 | HTTP 200，`response.completed`，文本 `OK` | 482 / 2 |

上游实际只收到两次 `/v1/responses`，本地两条消费日志计数与响应一致。客户端根路径、版本路径和 DeepSeek 上游根地址均在完整应用流程中验证通过；带 `/v1` 的上游地址由本地 HTTP 回归覆盖。

Go `1.26.0 windows/amd64` 下，`GOWORK=off go test ./...`、`go build ./...` 与独立 review 通过。排除旧渠道既有的不可达代码分析后，定向 `go vet -unreachable=false ./router ./relay/common ./relay/channel/deepseek` 通过。旧 main 的 service 测试因时间戳缓存键碰撞在 Windows 误报，已仅给测试键加入用例名称隔离，未修改生产缓存行为。

全量 `go vet ./...` 尚有 17 条既有告警，位于本次未修改的 `common/custom-event.go`（4 条锁拷贝）、`common/email_test.go`（1 条 IPv6 地址）及 11 个旧渠道适配器（12 条不可达代码）。已逐个核对相关 13 个文件与 `go.mod/go.sum`，内容均与修复前 `main=1e88cbaaa` 一致。普通定向 vet 也会受旧渠道依赖阻断，不能宣称全量或未排除分析项的定向 vet 通过。

上线后检查文本输出、工具调用及工具结果回传，并核对消费日志。生产服务未由本地代理直接更新；本地测试成功不代替部署后的真实验收。回滚使用此前应用镜像或撤销本次代码提交后重建应用，不删除数据卷。
