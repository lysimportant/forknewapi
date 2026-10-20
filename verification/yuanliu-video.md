# Yuanliu 视频插件与模型别名

## 2026-10-19：补齐目录模型并隐藏不可用供给

- 本轮基线工作区干净。依据北京时间 2026-10-19 15:40:40 的脱敏目录快照：当前 Key 可见 12 个视频模型，其中 8 个尚未适配、4 个历史 HD 型号名称包含“资源告罄”。该结果仅代表此 Key 的目录，不证明旧模型全局永久下架；历史 14 个型号继续兼容。
- 插件升级为 `1.2.0`，共登记 22 个精确模型和固定别名；同步补齐 New API 的 Canvas bridge 媒体能力及模型广场识别。所有新型号沿用现有创建、查询、下载、稳定任务 ID 和禁止未知结果自动重发的协议。

| 新增固定别名 | 精确上游 ID | 时长（秒） | 分辨率 | 计费 |
| --- | --- | --- | --- | --- |
| `Yuan-Seedance-2.5-QN-02` | `yl_pp_md_sd25_900` | 仅 5/10/15/30 | 720p | 按次 |
| `Yuan-Seedance-2.5-QN-03` | `yl_pp_vg_sd25_wd` | 4–30 | 720p | 按秒 |
| `Yuan-Seedance-2.5-QN-10` | `yl_pp_md_sd25_30010` | 4–30，默认 30 | 480p | 按次 |
| `Yuan-Seedance-2.5-VX-01` | `y_mjna_md_sd25_30010` | 4–30 | 720p | 按秒 |
| `Yuan-Seedance-2.5-VX-02` | `yl_mjna_cd_seedance_25_720p` | 仅 15/30 | 720p | 按次 |
| `Yuan-Seedance-2.0-XK` | `yl_xk_seedance2_0_903` | 4–15 | 720p | 按次 |
| `Yuan-Wan-3.0` | `yl_fuyao_wan3_0_video` | 2–30 | 480p/720p/1080p | 按秒 |
| `Yuan-Wan-3.0-Prime` | `yl_fuyao_wan3_0_video_prime` | 2–30 | 480p/720p/1080p | 按秒 |

- QN-10 必须有 1–30 张 HTTPS 图片，可选最多 10 个音频，总数最多 40。Wan 两型号接受仅素材生成，提示词和素材至少一项；各型提示词字符、比例和参考数量按目录合同校验。Canvas 保留既有 40 项总上限及可执行渠道能力交集。
- 广场只隐藏 Yuan 新鲜状态中的 `sold_out` 和 `disabled`；卡片、表格、页面计数、侧栏筛选及已打开详情使用同一可见模型集合。状态失败、缺失、未知或过期保留展示；状态仍从原始模型集合刷新，可用后恢复展示。其他供应商行为不变。
- 状态沿用既有上游目录判断：名称含“售罄/告罄”为 `sold_out`，成功目录中缺少精确 ID 为 `disabled`，失败或多 Key 抽样不确定为 `unknown`。不通过真实生成探测库存。成功目录缓存 90 秒，前端每 60 秒刷新，超过 3 分钟或未来偏差超过 60 秒视为未知。
- 不改生产数据、渠道映射、管理员售价、依赖或全局配置。更新应用后，新型号仍需由管理员在渠道获取/启用模型并配置本站售价；上游人民币成本不会自动写入价格。Wan 的上游参考视频额外成本不新增为本站计费字段，仍按输出秒数和分辨率计量。
- 素材仍只传 URL，不下载探测文件；素材大小、视频单段和总时长等由上游校验。无数据库迁移；回滚时先停新请求、等待在途任务，再回退应用代码并保留渠道、价格及历史任务。

### 本轮验收

- A1（插件/合同测试）、A2（Canvas 能力/隔离测试）、A3（广场过滤/前端回归）均已完成并由协调者验收；协调者完成最终差异审阅、跨模块验证、构建及 A4 测试配置收尾。前端沿用既有卡片、表格、筛选、详情及状态徽标；只增加共享可见性逻辑，没有替代通用 UI 封装。
- 修改范围：`plugins/tasks/yuanliu/plugin.js`、`plugins/yuanliu_video_test.go`、`controller/canvas_bridge.go`、`controller/canvas_bridge_test.go`、`web/src/features/pricing/` 下 5 个实现及 3 个新增测试文件、`web/vitest.config.ts` 和本记录。无公共 API、数据库或配置数据格式变化。
- `go test -mod=readonly -p=2 -json -count=1 ./...`：45 个测试包通过，5452 条测试通过事件、83 条条件跳过、0 失败；日志 `.local-tests/yuanliu/oct19-go-all.jsonl`。运行时在子进程清除外部数据库及 Redis 连接配置，使用独立临时 SQLite；未运行的外部 MySQL/PostgreSQL 矩阵不计为通过。
- `go test -mod=readonly ./plugins ./pkg/jsplugin -count=1`、`go test ./controller -run TestCanvas -count=1` 及三个受影响包的 `go vet` 通过；22 个模型的插件、Canvas 与广场识别集合一致，新增参数、素材、计量和跨路由能力交集测试通过。
- 在 `web` 运行 `npm run test -- src/features/pricing --maxWorkers=1 --no-isolate --pool=threads --reporter=default --reporter=json --outputFile.json=../.local-tests/yuanliu/pricing-standard-final-20261019.json`：18 文件、319 用例通过，0 失败/跳过，22.77 秒；新增 25 项覆盖模型识别、时效边界、降级恢复和真实页面交互。日志 `.local-tests/yuanliu/pricing-standard-final-20261019.log`，JSON 报告同名。`npm run typecheck`、改动文件 oxlint/oxfmt 全部通过。
- 为让新增真实页面测试通过常规命令复跑，测试配置补齐 VisActor ESM 解析，预打包现有图标/图表依赖，并为其中真实 CJS 模块保留 Node `require`；无依赖变更、模块替身或产品构建配置变化。此前临时测试参数的通过结果不代替上述最终常规命令；JSDOM canvas/reduced-motion 提示为非阻塞环境提示，真实浏览器无控制台错误。
- 测试配置持久化后，额外运行 `npm run test -- src/features/channels/components/__tests__/channel-configuration.test.tsx src/features/channels/components/__tests__/upstream-model-selection.test.tsx --maxWorkers=1 --no-isolate --pool=threads --testTimeout=60000`：2 文件、83 用例通过，退出码 0；日志 `.local-tests/yuanliu/channels-standard-final-20261019.log`，现有渠道获取和模型选择回归正常。
- `npm run build` 通过，日志 `.local-tests/yuanliu/oct19-web-build.log`。随后 `go build -mod=readonly -o .local-tests/yuanliu/new-api-oct19.exe .`、最终二进制插件 lint（`yuanliu@1.2.0`）及独立临时 SQLite 启动通过；首页、状态接口和 5 个内嵌入口脚本均 HTTP 200，专用进程已退出。启动报告 `.local-tests/yuanliu/startup-1792398221584/startup-report.json`，二进制 SHA256 为 `e39343ceefa0ab0ce87882c28ac388d6ea4f4adb1ada4d2557de52bc3d4d66e1`。
- 构建产物在 1440×1000 桌面和 390×844 手机浏览器通过 7 个场景：卡片/表格隐藏与计数筛选一致、恢复可用、详情自动关闭、全部隐藏后仍能刷新、失败/过期降级，以及手机筛选、Escape 和无水平溢出。合成数据、22 次本地 API 请求全部为 GET，无页面或控制台错误；截图已实际查看，服务已退出。报告 `.local-tests/yuanliu/catalog-ui-20261019-s4aCBD/report.json`。
- 改动文件格式、lint、`git diff --check` 和秘密模式扫描通过；没有覆盖用户既有改动，未提交、推送或部署。未调用真实付费生成，目录状态不构成真实成片验收；图片目录未返回有效模型 JSON，本轮仅适配已核对的视频合同。

## 2026-10-10：LW 适配

- P1 目标：将 `yl_lwaigc_mf_sd2_5_v2` 适配为 `Yuan-Seedance-2.5-LW`。仅增加渠道映射不能绕过插件的精确模型注册与校验。
- 基线：New API `main@f1bef1968` 跟踪 `fork/main`，Canvas `main@d79c3f0b` 跟踪 `origin/main`，两仓工作区干净。Go 1.26.0、Node 24.12.0、pnpm 11.19.0；前端依赖已存在，Bun 不在 PATH，继续用项目本地工具执行等价检查。
- 实时证据：北京时间 2026-10-10 18:33，`GET /openapi/v1/models` HTTP 200，9 项，LW 仍在目录中。合同为整数 4–30 秒、720p、16:9 / 9:16 / 1:1、最多 30 图与 10 音频、禁止视频参考、提示词最多 16000 字符；音频须至少搭配 1 张图片。公开文档确认沿用 `/videos`、URL 数组和 `client_request_id`。
- 最终范围仅 New API：插件声明、别名、按秒用量、Canvas bridge 和模型广场识别。经 Canvas `main@d79c3f0b` 真实 Provider 的合成请求核对，现有通用调用发送 `reference_images: [{url}]`、`metadata.content` 的 `content` / `audioTrack` 普通角色；插件兼容这些字段即可，不需要画布单独登记 LW 或添加能力限制。首尾帧 `image` / `last_frame` 字段仍明确拒绝，不能推断为普通参考。
- 验收：目录自动返回别名映射，画布现有文生/图片/图片加音频请求可编解码并按秒计量，非法音频独用/视频/比例/时长在出站前拒绝，模型广场状态正常识别；New API 相关测试、检查、构建和隔离 HTTP 烟测通过。
- 无数据库、依赖、管理员价格或生产配置变更；不调用真实付费生成。回滚先停用 LW 新请求并等待在途任务结束，再回退 New API 应用提交，保留映射、价格与任务记录。
- 基线插件回归：`go test ./plugins ./pkg/jsplugin -run 'TestYuanliu|TestBuiltIn' -count=1`，插件通过；`pkg/jsplugin` 没有匹配此筛选的测试。LW 注册、目录别名、通用参考字段及模型广场识别均已完成。
- 按用户要求删除根 `AGENTS.md` 中强制与画布仓库联调、核对合同的条款；插件目录发现、精确模型适配及本仓回归要求保持有效。

### 本轮验证与交付

- `go test -mod=readonly ./plugins -run '^TestYuanliu' -count=1` 通过；`go test -mod=readonly ./plugins -skip '^TestImage2Pro' -count=1` 通过。组合测试 `go test -mod=readonly ./plugins ./pkg/jsplugin ./controller -count=1` 中宿主运行时和 controller 全包通过，plugins 仅 Image2Pro 并行改动的 `TestImage2ProH3VideoContracts` 失败，日志 `.local-tests/yuanliu/lw-go-tests.log`；该并行修改不纳入本次提交，不能把组合测试记为全通过。
- `go vet -mod=readonly ./plugins ./pkg/jsplugin ./controller` 的对应分包检查通过。`go build -mod=readonly -o .local-tests/yuanliu/new-api-lw.exe .` 与该二进制 `plugin lint plugins/tasks/yuanliu/plugin.js` 通过，插件版本为 `1.1.0`。计费描述仍使用“视频生成单价”，单位独立放在 `unit`，未新增 UI 文案或价格常量。
- 前端 pricing/model-cards、pricing/pricing-controls、channels/channel-configuration、channels/upstream-model-selection 四个既有套件 121/121 通过（`npm run test -- … --testTimeout=60000 --maxWorkers=1`），日志 `.local-tests/yuanliu/lw-web-tests.log`；存在原有 `TimeoutNaNWarning`，退出码 0。`npm run typecheck`、`npm run build`、改动前端及插件的 oxlint/oxfmt 检查均通过，没有依赖或锁文件变更。
- 最终二进制 SHA256 为 `e7132caf2bae0e8fdd76661fbfddc87fb1812d0331dafa21259033ee98978a82`，嵌入源流插件与工作区源码逐字一致。隔离新 SQLite、回环模拟上游与合成凭据的 HTTP 报告为 `.local-tests/yuanliu/run-1791629694482-c60b4f/http-report.json`：14 个历史已适配别名完整创建和轮询；LW 的两种请求格式各 30 图、10 音频保持顺序；27 个非法请求出站 0 次、扣费 0；目录获取、按秒结算、内容下载和 503 不重发通过。20 次 POST 均为模拟请求，14 项模拟目录不代表当前真实目录仍有 14 项。
- PC 浏览器 1440×1000 检查 LW 搜索、详情、模拟售罄和目录失败显示“未知”，4 个场景通过，控制台错误、警告均为空，截图已检查；报告 `.local-tests/yuanliu/lw-browser-report.json`。测试价格表达式仅用于合成计量，页面按现有“特殊计费表达式”展示。专用网关及模拟上游已退出，62804、62805、62806 均无遗留监听。
- Canvas 未提交任何改动，工作区与索引均干净，恢复后的 `pnpm build` 9/9 通过。本轮仅交付 New API；未部署生产，也未执行真实供应商付费生成。
- 交付目标为 `fork/main` 与附注 Tag `v2026.10.10-yuanliu-lw`，最终提交和远端同步以 Git 引用核验。更新应用后重新获取并保存 `Yuan-Seedance-2.5-LW` 的映射，再配置本站按秒售价。

## 目标与基线

- P1：新增独立 `yuanliu` 任务插件，适配实时目录中的 13 个精确上游模型；渠道模型获取时提供 `Yuan-` 前缀别名及别名到上游 ID 的建议映射。
- 基线：`main@fac2e49f9cf657f99418196719ca3f9673c56af9`，工作区干净，上游 `fork/main`，远端 `https://github.com/lysimportant/forknewapi.git`；不推 QuantumNous `origin`。
- 环境：Windows，Go 1.26.0、Node 24.12.0，`web/node_modules` 已存在；Bun 不在 PATH，使用已有项目本地工具执行等价命令，不新增依赖。
- 上游：[公开合同](https://test.yuanliuai.tsyzai.com/openapi/docs.md)，v1.7.0，2026-10-08；已真实只读查询 `GET /openapi/v1/models`，HTTP 200，13 项。密钥不写入源码、测试、文档或报告。
- 验收：新建/编辑/独立模型获取合并别名并保留已有映射；陌生模型不扩大生成权限；完整创建、轮询、下载与用量合同覆盖；相关 Go/前端检查、启动冒烟与差异检查通过。

## 范围、兼容与回滚

- 初次插件接入只改 New API；2026-10-08 后续画布调用修复同步调整 Canvas，范围见下方双边核对。不修改现有渠道、管理员价格、数据库、依赖或生产配置，不执行真实付费生成。
- `modelAliases` 为 API v1 可选元信息，映射目标必须精确属于 `meta.models`。旧插件行为保留；旧宿主会拒绝此新增字段，插件需随宿主一起升级。
- 内部目录与自动上游更新仍使用精确上游 ID；管理响应显示建议别名并返回 `model_mapping`，保存使用已有渠道字段，已有管理员映射优先。
- 上游人民币报价不自动写成宿主美元价格。按次模型返回数量、按秒模型返回秒数和分辨率，管理员独立配置价格。
- 回滚先停用 Yuanliu 新建请求并等待在途任务结束，再回退应用提交；保留渠道映射、价格、任务和日志。没有数据库迁移。
- 目录存在冲突：LJ 2.0 标准版音视频上限采用名称/描述与结构化字段交集 0；YS 2.0 图片采用交集 9；HD 2.5 Full 时长采用交集 10–30 秒；LJ 2.5 按 `billing=per_call` 建立按次 schema，不按冲突描述改成按秒。仍需真实供应商确认。

## Canvas 双边核对

- 已读两仓 AGENTS 与对应合同。后续修复以 Canvas `main @ e77928544327d00d5f3ae2fdbfbc7fe89f7beb58`、New API `main @ 06e25caa06a347d85f990a1558e672a66f4ee50b` 为基线，补齐 13 个既有精确 ID 和固定别名的 `newapi-video-v1` 合同；仅开放文生和普通全能参考，不借用首尾帧、编辑或延长语义。
- Canvas API 在 Run 受理前校验参数、模式及引用数量；API/Worker 按冻结版本签发本站 HTTPS 地址，Provider 序列化到 `metadata.content`，保持显式重复引用及公共任务 ID。已有任务仍只查询，不因旧参数或素材失效重发 POST。
- New API Canvas bridge 按已定价可执行渠道的真实映射取媒体能力交集；数量遵守各型号和 Canvas 40 项总上限，非法 Yuan 解码返回 400。估价只有类型及角色占位，不读取素材、不调用供应商。
- `Yuan-Seedance-2.5-Official` 保留历史合同，不代表仍在实时目录上架；LW 于 2026-10-10 单独核对后补入，其它未适配的新名称不自动开放。此前 Canvas 检查点已移入 Git 历史，当前以 Canvas `AGENTS.md` / `README.md` 为准；本地验证不等于实际部署或真实供应商成片。

## 使用方式与模型映射

渠道类型选择“任务插件”，绑定 `Yuanliu Video`（`yuanliu`），使用 Base URL `https://test.yuanliuai.tsyzai.com/openapi/v1`，在渠道密钥字段填写自己的 Key。点击获取模型、勾选模型并保存，会同时写入公开别名和 `model_mapping`。原有精确上游 ID 可转换为建议别名，管理员已有映射始终优先；其他插件和没有建议映射的旧入口保持原行为。

| 客户端别名 | 精确上游 ID |
| --- | --- |
| `Yuan-Seedance-2.5-Official` | `seedance-2.5-guanfang-anmiao` |
| `Yuan-Seedance-2.0-LJ` | `yl_g7zy_seedance_v2_0_std` |
| `Yuan-Seedance-2.0-LJ-Full` | `yl_g7zy_seedance_v2_0_std_full` |
| `Yuan-Seedance-2.5-LJ` | `yl_g7zy_seedance_v2_5` |
| `Yuan-Seedance-2.5-LJ-Full` | `yl_g7zy_seedance_v2_5_full` |
| `Yuan-Seedance-2.0-HD` | `yl_seedance-2-0_ba0687ff09f2` |
| `Yuan-Seedance-2.5-HD` | `yl_seedance-2-5_6caffaca7390` |
| `Yuan-Seedance-2.5-HD-Full` | `yl_seedance-2-5_0fab2f1b1f10` |
| `Yuan-Seedance-2.5-HD-PerSecond` | `yl_seedance-2-5_750271498003` |
| `Yuan-Seedance-2.5-YS-Full` | `yl_api_hmstudio_seedance_v2_5_101010_7d58bbb217e6` |
| `Yuan-Seedance-2.5-YS` | `yl_api_hmstudio_seedance_v2_5_dc729300ff39` |
| `Yuan-Seedance-2.5-YL1` | `yl_video-30_76dbb7993f8e` |
| `Yuan-Seedance-2.0-YS` | `yl_api_hmstudio_seedance_v2_0_514a65db713b` |
| `Yuan-Seedance-2.5-LW` | `yl_lwaigc_mf_sd2_5_v2` |

客户端创建走 `POST /v1/videos`，查询和下载走 `GET /v1/videos/{网关任务ID}`、`GET /v1/videos/{网关任务ID}/content`。按秒模型为 Official、LJ 2.5 Full、HD 2.5 PerSecond、LW，其余 10 个按次；在价格管理页配置对应的宿主价格后再使用。LW 的上游目录报价不自动成为本站售价。

参考素材仅接受 HTTP(S) URL。插件保持每类素材顺序及重复项，不接收上传文件、Base64、首尾帧、编辑或延长模式。创建使用稳定网关任务 ID 作为 `client_request_id`，发送后未知结果禁止宿主自动重发。明确的供应商 `unknown` 保持等待；真正陌生状态仍计为轮询失败。内容固定从同渠道鉴权 `/content` 取回，不将渠道 Key 发送到 CDN。

## 验证结果

- `go test -mod=readonly -json -count=1 ./...`：44 个包通过，无失败，日志 `.local-tests/yuanliu/go-all.jsonl`。此后参考 URL 控制字符校验收紧，再运行 `go test -mod=readonly ./plugins -count=1` 通过。
- 宿主别名与模型发现专项通过，包含新建/已保存渠道目录、映射建议与只读不改写配置；最终合并检查 `go test -mod=readonly ./pkg/jsplugin ./plugins ./controller` 三包通过，controller 耗时 182.542 秒；`go vet -mod=readonly ./pkg/jsplugin ./plugins ./controller` 通过。
- 前端两个现有组件套件 78/78 通过，命令使用 `--testTimeout=60000 --maxWorkers=1 --reporter=verbose`；日志 `C:/Users/Sui/AppData/Local/Temp/yuanliu-frontend-final-vitest.log`。并发负载下曾有旧 operator 用例超时，最终串行全套通过，未改变项目超时配置或该用例。
- 最终补齐本地映射校验的翻译，新增 5 个回归覆盖抽屉获取/选择、弹窗获取/保存及同名供应商错误不翻译。两套最终筛选结果为 18 通过、65 跳过、共 83 项，退出码 0；统计摘录 `C:/Users/Sui/AppData/Local/Temp/yuanliu-frontend-alias-i18n-vitest.log`。筛选命令为 `node node_modules/vitest/vitest.mjs run src/features/channels/components/__tests__/channel-configuration.test.tsx src/features/channels/components/__tests__/upstream-model-selection.test.tsx --testNamePattern='alias|invalid existing mapping|discovery keeps|mapping errors' --testTimeout=60000 --maxWorkers=1 --reporter=verbose`。
- 最终项目本地 `node node_modules/@typescript/native-preview/bin/tsgo -b`、`node node_modules/@rsbuild/core/bin/rsbuild.js build`、涉及文件 oxlint/oxfmt、插件 lint/格式与 `git diff --check` 均通过；构建日志 `.local-tests/yuanliu/web-build-release.log`。没有新增依赖、锁文件、locale 或配置。
- 浏览器已在隔离合成账号中获取 13 个 Yuan 模型并点击“保存模型”，重新打开渠道路由/映射页确认 13 条别名到上游映射持久化；当前站点控制台错误/警告为空。截图 `.local-tests/yuanliu/yuan-models.jpg`。

最终前端错误提示收尾后，重新执行 `go build -mod=readonly -o .local-tests/yuanliu/new-api-release.exe .`，并通过该二进制的插件 lint 与隔离启动检查。最终二进制 SHA256 为 `ac4cd6034dcd4094ea12d13706cac802759ea538a0c74e10a5f6126414bab832`；首页、状态接口及全部 5 个内嵌前端入口脚本 HTTP 200，专用进程已清理。报告 `.local-tests/yuanliu/startup-1791446454084/startup-report.json`。此前完整 HTTP/浏览器验证使用的二进制和报告保留，以各报告记录的 Hash 区分；插件与宿主 API 在该错误提示收尾中没有改变。

### 隔离 HTTP 冒烟

使用新 SQLite、回环模拟上游和合成凭据，不访问用户数据库或真实付费生成。模拟合同二进制 `.local-tests/yuanliu/new-api-final.exe` 的 SHA256 为 `851ae48158ff26f89fff9667b9477a62556091531bf8754bed7b3a99f539f0f5`；内嵌插件与当前源码逐字一致，插件源码 SHA256 为 `672e1263d29970617e42a5e39cfc24917400bf2b9c4e4d433a2063ec51f03cd9`，二进制 `plugin lint plugins/tasks/yuanliu/plugin.js` 通过。

报告 `.local-tests/yuanliu/run-1791442359935-fe4153/http-report.json` 记录：

- 13 个别名完整创建、轮询、完成；按次与按秒分别结算，完成响应缺时长时从提交 state 恢复，不采用上游金额。
- 新建/编辑目录返回 13 别名及完整建议映射；401、空目录保留原配置，未适配模型单列。
- 19 个非法参数请求均返回 400，上游零 POST、零扣费；多模态 URL 顺序及重复保留。
- 供应商 `unknown` 可继续完成；宿主 `RetryTimes=3` 时未知提交 503 仅发出一次 POST。
- 内容 GET/HEAD 合成 MP4 字节正确；带凭据的跨源重定向返回 502，CDN 零请求、零密钥。
- 合计 17 次模拟 POST，合成结算额度 255000；这些数量与单价仅属于测试，不是供应商报价或真实消费。
- 专用网关及模拟上游已于 2026-10-08 15:04:12 清理，进程退出码 0；52106、52107、52108 无遗留监听。

以上初次插件验证不覆盖 Canvas。后续画布多模态本地回归见下节；真实供应商生成权限、素材公网可达性和成片质量仍未验证，不构成生产部署或真实上游验收。

## 2026-10-08：画布视频与全能参考配套修复

- 修改仅涉及 `controller/canvas_bridge.go` 及现有测试，插件请求和计费合同不变。13 个 ID/别名逐一验证声明能力、真实映射、边界数量、混合渠道和隐藏组别名歧义；非法参数和目录/估价全程零出站请求。
- `go test -mod=readonly -json -count=1 ./...` 本轮 44 包通过、81 条件用例跳过，另有 38 个包没有测试；日志 `.local-tests/yuanliu/go-canvas-all.jsonl`。数据库矩阵跳过不计为通过，本轮没有 SQL、ORM、数据库模型或迁移变更；Canvas/Yuan 专项、Go vet/build、插件 lint 与 `git diff --check` 通过。
- 最终隔离 HTTP 使用 `.local-tests/yuanliu/new-api-canvas.exe`，SHA256 为 `2523ba936c1ef44146b9e494d8af9aa420c1cbf6e34b44623e1bb600719134c8`。报告 `.local-tests/yuanliu/run-1791468938728-81d075/http-report.json`：13 别名完整创建/查询，19 非法请求零 POST，未知 503 只提交一次，同渠道内容鉴权和合成结算通过，17 次 POST 均为回环模拟。
- Canvas 完整九包 6489 项及 runtime 8 项通过，其中 Web 2626 项、Domain 534、Provider 999、API 1333、Worker 890；API 108、Worker 28 个设施条件用例跳过，不计为通过。最终全仓 build/typecheck/lint 各 9 包通过、零缓存，API/Worker 运行产物打包成功。1440×900、1366×768 共 10 项 Yuan 浏览器场景通过，实际截图已检查；完整命令与边界见 Canvas 检查点。
- 影响为两仓调用合同和素材水合路径，未改数据库或价格。部署须同时更新两端并核对有效插件、映射和公网 HTTPS 素材来源。回滚本轮应用提交并重建，保留在途公共任务、素材、渠道映射和价格，不重复生成。
- 交付目标为 `fork/main` 与附注 Tag `v2026.10.08-yuanliu-canvas`；Canvas 为 `origin/main` 和同名 Tag。已有 8080 站点及生产服务未更新，真实供应商成片未验收。

## 检查点

- 规划与双边合同核对、插件与别名导入实现、完整 Go/前端回归及 HTTP/浏览器验证完成；主代理和原负责子代理已审查最终差异，没有其他阻断项。
- 本地映射校验提示已复用现有七语言翻译；供应商错误保持原文。最终类型检查、构建及隔离启动通过；真实密钥扫描和差异检查通过，源码无用户提供凭据。
- 交付引用为 `fork/main` 和附注 Tag `v2026.10.08-yuanliu-video`，任务提交主题 `feat(yuanliu): 新增视频插件与模型别名自动映射`。提交与远端同步状态以 Git 引用核验；不部署生产、不请求真实付费生成。
