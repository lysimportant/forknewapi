# Responses Docker 完整性验收

## 目标与基线

- P0：在本地 Docker 中运行 main 的完整服务，将用户授权的上游 Key 加入隔离渠道，验证 Responses 文本、流式工具往返、视觉输入、路径兼容和消费记录。
- 起点：`main=6cc4ff47f`，工作区干净；用户远端 `fork=https://github.com/lysimportant/forknewapi.git`。本轮版本为 `v1.0.0-rc.33.responses.3`。
- 无数据库结构、依赖或前端源码变更；不操作生产服务。新增用量提取继续使用既有字段，推理 token 属于输出总数，不重复叠加。回滚到此前应用版本并重建应用，保留数据库与数据卷。

## 环境与启动

- Docker Engine `29.7.2`、Compose `5.4.0`，Linux 容器。
- 主分支前端使用锁文件安装，Bun `1.4.2`；生产 build 通过。该运行时只安装在忽略的本地测试工具目录，并有独立 manifest/lockfile。
- Go `1.26.0 windows/amd64` 交叉编译 Linux/amd64，`CGO_ENABLED=0`、`GOEXPERIMENT=greenteagc`；前端资源由当前 main 自身构建。
- 原 Dockerfile 的固定基础镜像构建被 `auth.docker.io` 的 DNS/连接超时阻断。未修改用户 Docker 配置；本地验收改用已缓存的 `node:24.12.0-bookworm-slim` 作为 Debian 运行镜像装入完整 Linux 二进制。此结果不代表原 Dockerfile 的在线拉取与构建已通过。
- 独立 Compose 项目 `newapi-responses-local`，入口 `http://127.0.0.1:3300`，PostgreSQL `16.15`、Redis `7.4.11`，数据库端口不发布到主机。数据保存在独立 Docker 卷。
- 临时 Compose、驱动脚本与脱敏结果位于忽略目录 `.local-tests/responses-docker`；真实上游凭据只经不回显的 stdin 传入，再通过正常管理 API 写入用户授权的隔离渠道数据库，不进入仓库、镜像或报告。

## 阶段检查点

- [x] 检查 Docker、现有服务、Git 和运行时，隔离 PostgreSQL/Redis 已启动并健康。
- [x] 主分支前端锁文件安装及 build 通过，Linux 依赖编译通过。
- [x] 本地可控上游复现：失败/未完成终态用量丢失且日志显示成功，缺终态 EOF 不报错，推理 token 明细丢失，未知扩展对象 delta 被丢弃。
- [x] 修复原生流处理缺口并通过确定性回归。
- [x] Linux 容器启动，DeepSeek/OpenAI 各 root 与 /v1 渠道创建，真实 Key 已经管理 API 加入渠道。
- [x] 原生路径真实请求与消费日志核对、六类容器内故障注入验收；真实上游部分 502 如下保留，不标记全通过。
- [x] 全部 36 个 APIType 矩阵、7 个非文本任务渠道拒绝、6 个代表性本地供应商 HTTP 回归通过。
- [x] 初版公共转换容器实测：文本与视觉成功，发现工具终态帧被旧 Chat handler 丢弃、客户端用量暴露内部字段。
- [x] 修复公共转换实测缺口，验证流式工具生成与包含推理摘要的完整历史回传。
- [x] 最终容器公共转换故障注入及消费日志、令牌和钱包余额核对。
- [x] 完整 Go test/build、定向 vet、diff、凭据扫描及独立代码审阅。

## 原生渠道真实验收

模型均为 `deepseek-v4-flash-vision-exp`，实际上游为用户授权的 Sub2 服务；请求经本地完整应用、渠道、鉴权、PostgreSQL 和计费流程。root 与 `/v1` 表示渠道上游 Base URL，两种客户端路径交叉使用。

| 渠道与场景 | 结果 | 输入 / 输出 token |
| --- | --- | --- |
| DeepSeek root 文本 | 上游 HTTP 502 | 无消费日志 |
| DeepSeek root 工具生成 | 上游 HTTP 502；未发送依赖它的工具回传 | 无消费日志 |
| DeepSeek /v1 视觉 | HTTP 200，`LEFT=RED;RIGHT=BLUE` | 251 / 80 |
| OpenAI /v1 文本 | HTTP 200，`TEXT_OK` | 101 / 20 |
| OpenAI /v1 工具生成 | HTTP 200，一次 `read_file(README.md)` | 387 / 77 |
| OpenAI root 工具回传 | HTTP 200，`TOOL_OK` | 482 / 4 |
| OpenAI root 视觉 | HTTP 200，`LEFT=RED;RIGHT=BLUE` | 251 / 80 |

五条成功请求与五条消费日志逐项吻合。两次 502 请求 ID：`202609071642125330498778268d9d64usvMxN1`、`202609071642236961446028268d9d6pBx3YWT3`；保留失败结果，不自动重放付费请求。

本地原生故障注入 `failed/incomplete/truncated/extension/zero/malformed` 六例全部通过，模拟上游恰好六次调用。失败和未完成保留实际用量并记录错误，截断不会伪造完成，未知事件原样保留，明确零用量不替换为估算，畸形响应只发一个错误终态。测试渠道已删除。

## 公共转换验收检查点

新增 SiliconFlow 类型 40 的 root 与 `/v1/` 渠道，使用同一用户授权的上游 Key 验证标准 Chat 桥接；这不等同于测试 SiliconFlow 官方供应商网络。初次真实文本为 `TEXT_OK`（88/31），视觉为 `LEFT=RED;RIGHT=BLUE`（235/183），均 HTTP 200；工具已收到完整参数，但终态被旧 Chat handler 错误过滤，实际 374/78 已结算且日志标记错误。

已修复同时携带 usage 与 finish_reason 的 Chat 帧被丢弃；Responses 用量仅输出公共字段；未加密推理摘要可随完整输出历史回传，密文继续明确拒绝。

修复后只新增两次针对该缺口的真实验收，不重复此前成功的文本与视觉生成。两次均 HTTP 200 且恰好一个 `response.completed`：

| 场景 | 请求 ID | 结果 | 输入 / 输出 token |
| --- | --- | --- | --- |
| root 渠道，`/v1/responses` 工具生成 | `202609071718211906635918268d9d686NSzzAQ` | 一次 `read_file(README.md)` | 374 / 74 |
| /v1/ 渠道，`/responses` 完整历史及工具结果回传 | `202609071718340621316758268d9d6fq2dCLsy` | `TOOL_OK` | 466 / 21 |

专属测试令牌恰好两条消费日志，用量逐项匹配，流状态均为 `ok`。响应只包含公共用量字段，推理 token 分别为 29、17，均已包含于输出总数，未重复计费。工具调用标识与回传结果关联保持不变。两个 compact 路径按能力边界返回 400，非法 token 上限返回 400，无鉴权请求返回 401。

最终容器公共转换故障注入 `bridge-fixtures.mjs` 七例通过：Mistral 文本、工具、length、异常断流，Claude JSON 缓存，Cohere NDJSON，以及 Cohere 工具预检拒绝。六次模拟生成对应六条消费日志；每步令牌和管理员钱包扣减与日志 quota 一致，总计 92 quota（Claude 缓存场景沿用既有缓存倍率，22 quota；其余五次各 14）。拒绝请求 HTTP 400、零上游调用、零扣费，临时七个渠道及专属令牌已清理。

本地保留六个真实上游渠道（DeepSeek/OpenAI/SiliconFlow 各 root、版本地址两种），服务及独立数据卷继续保留，供用户复核。

## 最终验证与发布记录

```sh
GOWORK=off go test ./...
GOWORK=off go build ./...
go vet -unreachable=false ./relay ./controller ./service/relayconvert/... ./dto ./relay/channel/openai ./relay/channel/palm
git diff --cached --check
```

以上检查通过。全部 24 个任务文件的真实凭据扫描零匹配，独立最终审阅无新增 P0/P1。未修改数据库结构或 ORM/驱动，不涉及迁移；没有用单一数据库测试替代跨数据库兼容性声明。Windows 未运行 race detector：当前 CGO 关闭且没有 C 编译器。

最终运行版本为 `v1.0.0-rc.33.responses.3`，健康检查通过。Linux 二进制 SHA256 为 `90F7DAE2537962532305035B65B264A310B708FDE57DCEEC57DA34D869BFAD7B`；容器镜像为 `sha256:9e4e2ef199d7a3b92ffde8ce436f023d763c73c1a5c4eedf9f3209b3617a683c`。发布使用用户 fork 的 `main` 和中文 annotated tag `v1.0.0-rc.33.responses.3`，提交及标签的实际远端状态以 `git ls-remote fork refs/heads/main 'refs/tags/v1.0.0-rc.33.responses.3*'` 核对。

## 已知基线限制

main 的前端 lint 与 typecheck 在未修改的代码上失败，主要包括既有系统设置的 boolean/number 与 string 类型不一致及存量 lint 规则错误；前端生产 build 已通过。后端全量 vet 的 17 条既有告警见 [部署核对](responses-deployment.md)。不能将这些检查标记为通过，也不在 Responses 修复中扩展为无关前端重构。

生产服务的升级与最终外部验收仍需单独确认，本地容器成功不等于线上已更新。
