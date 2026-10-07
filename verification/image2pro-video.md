# Image2Pro 视频插件检查点

## 目标与基线

P1：按供应商公开视频 API 文档接入三个精确模型，支持文生视频、最多 9 张参考图、JSON URL/Data URL 和本地素材上传，按请求视频秒数计费。用户追加每次最多 3 个音频和 3 个视频引用：按 `audios` / `videos` 数组提供兼容扩展，其上游合同待验证。

- 基线：`main@a76c610e1`，工作区干净，上游 `fork/main`，远端 `https://github.com/lysimportant/forknewapi.git`。
- 环境：Windows，Go `1.26.0`、Node `24.12.0`，`web/node_modules` 已存在；Bun 不在默认 PATH，优先复用仓库本地工具。
- Moon 不支持这三个模型的精确 ID，且拒绝 multipart，新增独立 `image2pro`，保留 Moon 原合同。
- 文档：<https://image2pro.top/api-docs>；已只读验证 `GET https://api.image2pro.top/v1/models` 返回三个目标模型。
- 模型：`无限制-Flash-中配-Video`、`无限制-Flash-MAX-Video`、`Seedance2.0 0.9r`；目录中的其他模型不自动开放生成。
- 文档只明确 `input_image` / `images` 图片 URL/Data URL，最多 9 张。用户要求追加 3 音频、3 视频后，使用 `audios` / `videos` 的 URL/Data URL 字符串数组做本地兼容适配并验证上传转换；这两个上游字段尚未公开，已向用户说明待验证，不宣称供应商确认支持。首尾帧语义仍未公开，不静默降级成普通图片。
- 不提交真实付费生成，不修改管理员价格、供应商积分换算、数据库、依赖或生产配置。

## 分工与恢复

- 主代理：供应商合同核对、集中回归、模拟 HTTP 验收、检查点和 Git 交付。
- `moon_compatibility`：仅 `plugins/tasks/image2pro/plugin.js`。
- `upload_discovery_audit`：宿主重复上传引用审计；确认方案后单独修复同名多图读成首图的边界。
- 上次成功：最终源码构建的隔离网关完成全部 HTTP 验收；44 个 Go 测试包均有通过结果，全仓 vet、构建、插件 lint/format、完整 diff 和敏感信息扫描通过。
- Git 交付目标：`fork/main`，中文 annotated Tag `v1.0.0-rc.37.custom.39`。实际提交与推送状态以 Git HEAD、Tag 指向和远端核验为准，部署需更新并重启后端。

## 兼容与回滚

插件复用现有任务轮询、计费与制品代理，不更改公开视频接口。请求必须显式提供合法时长，避免按秒计费猜测供应商默认值。提交结果未知时禁止自动重复生成。

回滚前先等待在途任务完成，禁用 `image2pro` 渠道或回退本次应用提交；保留价格、任务、日志和用户数据。上传修复需保持旧单文件引用格式可用。

## 验收状态

- [x] 读取项目规则、插件 API 和现有 Moon 实现，记录干净基线。
- [x] 读取供应商公开视频文档及真实模型目录，不记录密钥。
- [x] 插件及上传边界实现，集中协议回归。
- [x] 模拟 HTTP 提交、轮询、上传和下载验收。
- [x] Go 检查、插件 lint/format、最终 diff 和敏感信息扫描。

模拟验收只能证明本地合同和转发行为，不能证明供应商账号会员权限、真实生成质量或未公开的音频、视频引用能力。

## 使用配置

更新并重启后端后，启用内置 `Image2Pro` 任务插件，新增类型 61 渠道并绑定 `task_plugin_key=image2pro`，Base URL 使用 `https://api.image2pro.top/v1`。密钥只填渠道密钥栏；「获取模型」读取实时目录，仅允许导入上述三个已声明模型。

管理员按各模型配置视频生成单价。任务表达式示例为 `u("seconds") * P`，`P` 替换为每秒美元单价；不使用供应商积分或按次报价。每次请求必须显式传 `seconds` 或 `duration`，取值大于 0、不超过宿主 3600 秒安全上限；供应商实际可用时长仍由协议决定。

JSON 使用 `images`、`audios`、`videos` 的 URL/Data URL 数组；普通 `{url, role}` 引用会转换为字符串，保留顺序、重复和签名参数。multipart 可以在同名 `images` / `audios` / `videos` 字段重复上传文件，宿主按独立引用编码成 Data URL 后发 JSON，不把文件占位对象直接发给供应商。音视频字段是兼容扩展，尚未验证真实供应商受理。

支持 OpenAI Video 的 `/v1/videos` 创建、查询和成片内容代理，以及 Responses 同步、流式、后台模式。上游生成失败沿用宿主退款；未知提交结果不自动重发。首尾帧角色和未知参数明确报错，不静默降级。

## 本地 HTTP 验收证据

最终源码与运行网关的内嵌插件逐字比较一致。Node 夹具使用 fresh SQLite、临时合成凭据，网关监听 `127.0.0.1:18435`，模拟供应商监听 `127.0.0.1:18436`，结束后均停止；没有向真实供应商提交任务。

- 24 个非法输入在上游 POST 和扣费前拒绝，包含时长、素材超量、媒体 MIME、角色与未知参数。
- 5 次真实管理目录读取，覆盖新建预览、编辑预览、已保存渠道、空目录和失败目录；仅导入三个精确模型，失败不回退静态成功。
- 9 图、3 音频、3 视频 JSON 引用及 multipart 同名字段多个不同文件外发正确。URL 顺序、重复、签名和 Data URL 字节均保留。
- 14 个受理任务中 13 个成功、1 个失败退款；即便轮询响应不含时长仍保留原请求秒数。失败 8 秒退回 40000 quota，成功 65 秒共结算 325000 quota，夹具使用合成单价。
- Video GET/HEAD 与 Responses 同步、流式、后台模式均完成；5 次成片 GET/HEAD 请求不携带 Authorization、API Key 或 Cookie。
- `RetryTimes=3` 时，模拟 503 未知提交仅 POST 一次，不能自动重复生成。总上游 POST 为 15 次（14 次受理、1 次 503）。

报告：`.local-tests/image2pro/http-report.json`，日志：`.local-tests/image2pro/http-app.log`，夹具：`.local-tests/image2pro/http-smoke.mjs`。以上均为本地模拟证据，音视频扩展未获得供应商实际受理证明。

## 检查命令与结果

- `go test -mod=readonly ./plugins ./common ./middleware ./relay/channel/task/jsplugin -count=1` 通过，包括现有 Moon 合同与新的集中 Image2Pro 回归。
- `go test -mod=readonly ./... -count=1 -timeout=240s` 首轮 43 个包通过，仅 controller 完整套件超时，无其他失败包；`go test -mod=readonly ./controller -count=1 -timeout=600s` 独立复跑通过，用时 221.226 秒。合计所有 44 个含测试的包通过；首次超时不记为通过。
- `go vet -mod=readonly ./...` 通过。
- `go build -mod=readonly -o .local-tests/image2pro/new-api.exe .` 通过，最终二进制与 HTTP 夹具中的内嵌源码一致。
- `node web/node_modules/oxlint/bin/oxlint -c plugins/.oxlintrc.json plugins/tasks/image2pro/plugin.js` 通过。
- `node web/node_modules/oxfmt/bin/oxfmt --config plugins/.oxfmtrc.json --check plugins/tasks/image2pro/plugin.js` 通过。
- `.local-tests/image2pro/new-api.exe plugin lint plugins/tasks/image2pro/plugin.js` 通过。
- `node .local-tests/image2pro/http-smoke.mjs` 通过，报告 `success=true`。
- 修改的 Go 文件已 gofmt；`git diff --check`、完整差异复核和任务文件敏感信息扫描通过。没有修改依赖、relaykit、数据库结构或前端 UI，因此不涉及三数据库迁移矩阵或独立 relaykit 构建。
