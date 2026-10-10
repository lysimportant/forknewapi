# Image2Pro 视频插件检查点

## 2026-10-10：按 ImagePro 公开视频合同修正创建请求

- P1：修复 `Seedance2.0 0.9r`、`无限制-Flash-MAX-Video` 创建请求被供应商以“请提供 prompt 或 input_image”拒绝的问题。恢复后的实际基线为 `main @ f64836751`、上游 `fork/main`；源流、Canvas bridge、定价页及 `AGENTS.md` 改动已存在于该基线提交，不纳入本任务差异。
- 供应商当前公开 `GET https://api.image2pro.top/models` 显示真实 ID 分别为 `Seedance2.0`、`Flash视频-MAX`，展示名分别为 `Seedance2.0 0.9r`、`无限制-Flash-MAX-Video`；后者协议为 `agnes`，不是此前假定的 MiniMax H3。`GET https://api.image2pro.top/v1/models` 无凭据返回 401，未读取或记录生产密钥。
- `POST /v1/videos` 公开文档要求顶层 `prompt`，单图使用 `input_image`，多图使用 `images`。插件 2.2.0 保留 Canvas 及既有渠道的展示名和内部 `content` 输入兼容，最终外发改为真实 ID 与公开字段，不再发送 `content`、`resolution` 或 Seedance 扩展开关。Canvas 默认的 `resolution: "720p"` 仅作为兼容占位接受；其它值明确拒绝，避免静默丢失用户选择。
- 公开目录同时声明 Seedance 为 4–15 整秒、Flash-MAX 为 4–12 整秒，Flash-MAX 最多 5 张参考图。公开创建文档未声明音频、视频或首尾帧角色字段；插件在供应商 POST 前明确拒绝这些输入，避免静默丢素材或再次生成无效请求。旧任务查询、冻结秒数结算、任务 ID、幂等头和 `noRetry` 保持兼容。
- 为兼容既有渠道，插件同时声明两个展示名和两个真实 ID，最终分别映射到 `Seedance2.0`、`Flash视频-MAX`。四个精确名称在 New API 中仍是独立模型及计费键；上线时需复核渠道选择、`model_mapping` 和插件计费表达式，不能假定展示名价格自动继承给真实 ID。
- 本轮不修改数据库、价格、渠道、凭据、Canvas 或生产部署，也不发起真实生成 POST。上线前应确认数据库中的插件 override 已更新为 2.2.0，并复核渠道模型选择；回滚时先停止新提交、等待在途任务完成，再回退本次插件提交，保留任务、账单和日志数据。
- 最终 `go test -p=1 -mod=readonly ./... -count=1 -timeout=600s` 全部通过，其中 controller 用时 184.963 秒；并行复跑曾在 `pkg/jsplugin/TestEngineInterruptsLongRunningHook` 的 5ms 初始化阈值出现一次负载波动，失败用例和整个包独立复跑均通过。`go vet -mod=readonly ./...`、`go build -mod=readonly ./...`、插件宿主 lint、目标 oxlint/oxfmt 和插件专项测试均通过。Canvas `@multimodal-canvas/providers` 的 Image2Pro 43 项序列化回归通过，证明 Canvas 仍发送内部 `content`，转换发生在 New API 插件边界。
- 所有供应商检查均为公开 GET，未读取或记录生产密钥，未发送真实生成 POST。源码与本地模拟不能替代供应商真实受理和成片验收；如获授权进行真实验收，只允许带稳定幂等键提交一次，未知结果不得重发 POST。

## 2026-10-08：Flash-MAX H3 别名恢复（本地验收完成）

- P1：用户确认 `无限制-Flash-MAX-Video` 为 `MiniMax-H3` 别名，名称中的 MAX 不表示官方 `MiniMax-H3-Max`。恢复新建和模型导入，输出仅 `720p`、4–12 整秒。Seedance 官方参数保持独立，中配 Flash 继续停用。
- 基线为 `main @ deeba7e94`、上游 `fork/main`；Canvas 为 `main @ beb3bfa93`、上游 `origin/main`。两仓工作区干净，Go 1.26.0、Node 24.12.0、pnpm 11.19.0，依赖不变。
- 实施前双边核对官方 H3 v2、插件 API v1/schema/类型及 Canvas 模型目录、模式、冻结参数和恢复路径。请求正文采用 H3 `content`，URL 沿用 Image2Pro `/v1/videos`；不能将官方其它输出档位或 Seedance 参数移植到别名。
- 不修改价格、渠道、数据库或凭据，不调用真实付费接口或部署。回滚本轮两仓提交并重建可撤回 MAX 新建能力；在途任务、公共 ID 和原冻结秒数结算保留。
- 官方正文已只读核对，插件最终源码与运行二进制已冻结；完整 Go 回归及隔离 HTTP 验收结果见下文，Canvas 最终完整逐包回归及无缓存构建、类型、格式检查均通过。日志位于 `.local-tests/flash-h3-20261008/`。最终交付目标仍为 `fork/main`，不推 QuantumNous `origin`。
- H3 非空 `text` 必需、每项最多 7000 字符；帧与参考互斥，参考图/视频/音频上限分别 9/3/3。文生使用六个固定比例，首尾帧只用 `adaptive`，参考含纯音频加文字可用 `adaptive`；尾帧仍须伴随首帧。不开放 Seedance 三个布尔值、`aigc_watermark`、Seed、回调和供应商文件 ID。
- H3 图/音/视频支持 Data URL；内联视频只用 MP4，multipart 在本地转换为 JSON。单文件上限图 30 MB、视频 50 MB、音频 15 MB，最终 JSON 正文上限 64 MB；不能用原始文件大小代替 Base64 膨胀后的正文大小。没有可信远程媒体元数据时不猜测大小、尺寸、编码或时长。
- 插件 2.1.0 源码已冻结，Image2Pro/Moon/Hailuo H3/第三方 H3 专项通过，包含有效 Full-HD 图片在两模型/两协议的默认 Sobek 期限、UTF-8/宿主 HTML 和行分隔符转义、multipart Base64 膨胀。H3 行内样式文字原样保留，本地不把它解释成参数；Seedance 原覆盖标记拒绝保留。`go test ./... -count=1` 完整 44 包通过，controller 用时 220.028 秒；宿主 build、vet、插件 lint、目标 oxlint/oxfmt 与差异检查通过。
- 新版二进制 SHA256：`2604934e3d5d5d67ebd1856026b4d06102242a59ce3c5245bf8f9e8355b81430`。隔离 HTTP 与运行内嵌源码逐字核对，31 个非法请求零 POST/零扣费，11 个合法任务冻结 76 秒并结算 380000 quota（合成每秒 0.01 USD），未知 503 在 `RetryTimes=3` 时仅一次 POST；GET/HEAD 成片读取无供应商凭据，两回环监听已停止。
- HTTP 报告位于 `.local-tests/flash-h3-20261008/run-1791435764284-6550eb/http-report.json`。首次夹具因并行构建负载触发默认 CPU 阈值 503，零 POST；新 fresh SQLite 测试实例仅关闭 CPU 阈值后完整复跑，生产配置未改。真实供应商受理、成片、媒体编码和生产部署仍未验收。
- 目标文件格式通过；全插件格式检查仍报告三份未改文件 `alibaba/plugin.js`、`grok-video/README.md`、`minimax-h3-video/README.md`，已核对与 HEAD 无差异，不扩大本轮格式范围。全插件 oxlint 退出码 0，八条 warning 均在未改文件。
- Canvas 最终 Web 137 个文件、2618 项通过；PC 12 场景通过，包括原始无 `videoMode` 的文字 `content` 连线、固定 v3 txt 预览和刷新后单次 Mock 提交，控制台及未知业务网络错误为 0。API 108、Worker 28 个设施/真实请求等用例跳过，不计外部或生产验收。
- 本轮双仓使用中文附注 Tag `v2026.10.08-image2pro-flashmax-h3`，配套交付为 Canvas `origin/main`、New API `fork/main`。没有更新本机容器或目标生产实例；上线需确认运行插件为 2.1.0，以及数据库 override 和渠道中的模型选择。

## 上一阶段 Seedance 收敛目标与基线

上一阶段 P1：仅适配 `Seedance2.0 0.9r`，移除两个含 Flash 的模型；URL 依据 Image2Pro 文档，参数按官方 Seedance 2.0 传送。最新 MAX 恢复范围见本文件开头。

- 基线：`main@a76c610e1`，工作区干净，上游 `fork/main`，远端 `https://github.com/lysimportant/forknewapi.git`。
- 环境：Windows，Go `1.26.0`、Node `24.12.0`，`web/node_modules` 已存在；Bun 不在默认 PATH，优先复用仓库本地工具。
- Moon 不包含 `Seedance2.0 0.9r` 精确 ID，按 `usage.total_tokens` 结算且拒绝 multipart；Image2Pro 完成文档只承诺 `url`，当前按请求秒数计费并支持图片/音频上传转换，因此保留独立 `image2pro`，不借模型映射改变身份或收费。
- 文档：<https://image2pro.top/api-docs>；已只读验证 `GET https://api.image2pro.top/v1/models` 返回三个目标模型。
- 上一阶段仅声明 `Seedance2.0 0.9r`，两个 Flash 不再适配或导入；撤回当时未交付的 Flash-MAX 4–12 秒特判。旧任务的只读查询和已冻结用量保留，最新范围见开头。
- 用户已确认该接口接入 Seedance，body 按官网：`model/content/duration/resolution/ratio`，内容包含 `text/image_url/video_url/audio_url` 与明确角色；布尔 `generate_audio/watermark/return_last_frame` 保留显式 `false`。
- 官方依据：<https://docs.volcengine.com/docs/82379/1520757?lang=zh>，已保存正文 `.local-tests/video-provider-docs/doubao-official.md`。最多 9 图、3 视频、3 音频、合计 15；首尾帧与全模态参考互斥、尾帧需首帧、不可纯音频。图片/音频可用 Data URL，视频仅 URL；未确认的 `seed/camera_fixed/output_format` 等参数不开放给 2.0。
- 不提交真实付费生成，不修改管理员价格、供应商积分换算、数据库、依赖或生产配置。

## 初轮分工与恢复

- 主代理：供应商合同核对、集中回归、模拟 HTTP 验收、检查点和 Git 交付。
- `moon_compatibility`：仅 `plugins/tasks/image2pro/plugin.js`。
- `upload_discovery_audit`：宿主重复上传引用审计；确认方案后单独修复同名多图读成首图的边界。
- 上次成功：最终源码构建的隔离网关完成全部 HTTP 验收；44 个 Go 测试包均有通过结果，全仓 vet、构建、插件 lint/format、完整 diff 和敏感信息扫描通过。
- 初轮交付为 `fork/main` 与中文 annotated Tag `v1.0.0-rc.37.custom.39`；本轮交付见文末。部署需更新并重启后端，源码验证不代表运行插件已更新。

## 兼容与回滚

插件复用现有任务轮询、计费与制品代理，不更改公开视频接口。请求必须显式提供合法时长，避免按秒计费猜测供应商默认值。提交结果未知时禁止自动重复生成。

回滚前先等待在途任务完成，禁用 `image2pro` 渠道或回退本次应用提交；保留价格、任务、日志和用户数据。上传修复需保持旧单文件引用格式可用。

## 历史通用 body 验收状态（本轮新合同见文末）

- [x] 读取项目规则、插件 API 和现有 Moon 实现，记录干净基线。
- [x] 读取供应商公开视频文档及真实模型目录，不记录密钥。
- [x] 插件及上传边界实现，集中协议回归。
- [x] 模拟 HTTP 提交、轮询、上传和下载验收。
- [x] Go 检查、插件 lint/format、最终 diff 和敏感信息扫描。

模拟验收只能证明本地合同和转发行为，不能证明供应商账号会员权限、真实生成质量或未公开的音频、视频引用能力。

## 使用配置

更新并重启后端后，启用内置 `Image2Pro` 任务插件，类型 61 渠道绑定 `task_plugin_key=image2pro`，Base URL 使用 `https://api.image2pro.top/v1`。密钥只填渠道密钥栏；「获取模型」读取实时目录，仅允许导入 `Seedance2.0 0.9r` 与 `无限制-Flash-MAX-Video`。不改现有渠道、价格或数据库 override；管理员应清理渠道旧中配 Flash 选择并确认有效插件为 2.1.0。

管理员按模型配置视频生成单价。任务表达式示例为 `u("seconds") * P`，`P` 替换为每秒美元单价；不使用供应商积分或按次报价。新建必须显式传 `seconds` 或 `duration`：Seedance 为 4–15 整秒，Flash-MAX 为 4–12 整秒。由于 Image2Pro 查询文档未承诺实际时长，`-1` 自动时长明确拒绝；旧任务仍按原冻结秒数结算。

JSON 首选官方 `content`；旧 `prompt/images/audios/videos` 入口兼容转换为同一结构，不能与显式 `content` 混用而覆盖或重复素材。上传使用独立文件引用转 Data URL，保留角色、顺序、重复及签名参数。具体限制按精确模型区分：

| 参数          | Seedance2.0 0.9r                                       | 无限制-Flash-MAX-Video（H3 别名）   |
| ------------- | ------------------------------------------------------ | ----------------------------------- |
| 秒数          | 4–15 整秒                                              | 4–12 整秒                           |
| 分辨率        | 480p/720p/1080p/4k                                     | 仅 720p                             |
| 提示词        | 最多 30000 字符，视觉参考可无文本                      | 必须非空，每项最多 7000 字符        |
| 布尔字段      | generate_audio/watermark/return_last_frame，保留 false | 不开放这三个字段                    |
| 纯音频参考    | 不允许                                                 | 允许音频加文字                      |
| 视频内联/上传 | 不允许，需可读 URL                                     | MP4 可用 Data URL 或本地上传转 JSON |
| 首尾帧比例    | 原合同不变                                             | 仅 adaptive，固定比例明确报错       |

两个型号共用六个固定比例及 `adaptive` 枚举，但 H3 文生不能使用 `adaptive`；H3 参考可使用固定比例或 `adaptive`。H3 单文件限制按字节实施为图 31457280、视频 52428800、音频 15728640，最终 UTF-8 JSON 正文最多 67108864 bytes（64 MiB）。

支持 OpenAI Video 的 `/v1/videos` 创建、查询和成片内容代理，以及 Responses 同步、流式、后台模式。上游生成失败沿用宿主退款；未知提交结果不自动重发。未知参数、冲突别名与不合法角色组合明确报错，不静默降级。

## 历史三模型本地 HTTP 验收证据

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

## 2026-10-08：Seedance 单模型收敛

- 恢复基线为 `main @ 5c42d5a27`、上游 `fork/main`，三份旧 Flash-MAX 专项文件有未提交改动；Canvas 为 `main @ ff19f0d`、上游 `origin/main`。环境与依赖沿用现有版本。
- 实施前核对两仓规则、插件 API v1、schema/类型声明、Moon、Image2Pro 与 Canvas 创建/冻结/查询合同。没有扩展宿主 API，schema/类型声明无需改变。
- Image2Pro 升级至 2.0.0：白名单仅 `Seedance2.0 0.9r`，两个 Flash 的新创建和目录导入退出；查询钩子、已冻结秒数、上传与 noRetry 保留。不修改 Moon、现有价格或渠道，不删除在途任务。
- 这是不兼容的模型范围收窄。更新前应停止提交 Flash，保留旧任务查询；回滚本轮宿主提交即可恢复原模型声明。数据库 override 可覆盖内嵌插件，部署时应确认有效版本及渠道旧选择，不能只凭源码证明线上生效。
- 第一阶段仅收窄白名单的 44 包 Go test、vet/build/plugin lint/oxlint/oxfmt 已通过；用户随后明确参数按官网，以上不能代替最终 body 改动的回归。
- 最终插件源码已冻结。`go test -mod=readonly ./... -count=1 -timeout=600s` 完整通过 44 个测试包，controller 用时 182.496 秒；`go vet -mod=readonly ./...`、宿主 build、插件 lint、oxlint/oxfmt 与 `git diff --check` 均通过。日志位于 `.local-tests/seed-official-20261008/`，不能混用旧 body 阶段结果。
- 最新二进制 SHA256 为 `354f338951c0764d1f588845ca735fcf67dd1ae049509d17ee78d69410e68a59`。隔离 HTTP 夹具核对运行插件与当前源码逐字一致，`node .local-tests/seed-official-smoke-20261008/http-smoke.mjs` 退出码 0；报告位于 `.local-tests/seed-official-smoke-20261008/run-1791430801612-bae134/http-report.json`。
- HTTP 覆盖官方首尾帧、三个显式 `false`、9 图/3 视频/3 音频、旧签名重复图、multipart 不同图片及 header-only 幂等键；14 个非法场景零上游 POST、零扣费。四个成功任务冻结 30 秒，按合成单价结算 150000 quota；未知 503 在 `RetryTimes=3` 下仅提交一次并退款。两个回环监听均已停止，无真实收费请求。
- Responses 三种模式的官方 body 转换已有 JS 合同专项，未做本轮 HTTP 端到端验收；真实供应商受理、成片效果、生产部署和额外尾帧结果仍未验收。网关没有可信媒体元数据，远程参考素材的实际时长、大小和格式仍由上游检查；Canvas 对已冻结的参考音视频时长做独立请求前校验。
- Canvas 最终无缓存 lint/typecheck/build、同源 Web 构建、逐包串行测试与 7 项 PC 浏览器冒烟通过；其中 Web 为 137 文件、2591 项。独立复核已确认冻结参考时长及公网 MIME 校验的遗漏解决；设施和真实服务测试跳过不算集成通过。
- 交付引用：只提交本任务文件，中文附注 Tag `v2026.10.08-image2pro-seedance-official`，推 `fork/main` 并核验远端；不推 QuantumNous `origin`。实际提交以 Git 和远端核验为准。本轮不部署，数据库 override 与渠道旧 Flash 选择仍需管理员在更新时核对。
