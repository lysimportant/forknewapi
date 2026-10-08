# Yuanliu 视频插件与模型别名

## 目标与基线

- P1：新增独立 `yuanliu` 任务插件，适配实时目录中的 13 个精确上游模型；渠道模型获取时提供 `Yuan-` 前缀别名及别名到上游 ID 的建议映射。
- 基线：`main@fac2e49f9cf657f99418196719ca3f9673c56af9`，工作区干净，上游 `fork/main`，远端 `https://github.com/lysimportant/forknewapi.git`；不推 QuantumNous `origin`。
- 环境：Windows，Go 1.26.0、Node 24.12.0，`web/node_modules` 已存在；Bun 不在 PATH，使用已有项目本地工具执行等价命令，不新增依赖。
- 上游：[公开合同](https://test.yuanliuai.tsyzai.com/openapi/docs.md)，v1.7.0，2026-10-08；已真实只读查询 `GET /openapi/v1/models`，HTTP 200，13 项。密钥不写入源码、测试、文档或报告。
- 验收：新建/编辑/独立模型获取合并别名并保留已有映射；陌生模型不扩大生成权限；完整创建、轮询、下载与用量合同覆盖；相关 Go/前端检查、启动冒烟与差异检查通过。

## 范围、兼容与回滚

- 只改 New API，不修改 Canvas、现有渠道、管理员价格、数据库、依赖或生产配置；不执行真实付费生成。
- `modelAliases` 为 API v1 可选元信息，映射目标必须精确属于 `meta.models`。旧插件行为保留；旧宿主会拒绝此新增字段，插件需随宿主一起升级。
- 内部目录与自动上游更新仍使用精确上游 ID；管理响应显示建议别名并返回 `model_mapping`，保存使用已有渠道字段，已有管理员映射优先。
- 上游人民币报价不自动写成宿主美元价格。按次模型返回数量、按秒模型返回秒数和分辨率，管理员独立配置价格。
- 回滚先停用 Yuanliu 新建请求并等待在途任务结束，再回退应用提交；保留渠道映射、价格、任务和日志。没有数据库迁移。
- 目录存在冲突：LJ 2.0 标准版音视频上限采用名称/描述与结构化字段交集 0；YS 2.0 图片采用交集 9；HD 2.5 Full 时长采用交集 10–30 秒；LJ 2.5 按 `billing=per_call` 建立按次 schema，不按冲突描述改成按秒。仍需真实供应商确认。

## Canvas 双边核对

- 已读两仓 AGENTS 与对应合同。Canvas 当前 New API 账号使用 `newapi-video-v1`（`POST /v1/videos`），可通用文生与单首帧；新 Yuan 别名尚未登记精确生成合同，不声称已完成 Canvas 多模态。
- Canvas 的精确模型合同、报价媒体声明与资源持久化边界需后续适配：New API Canvas bridge 默认仅 text、估算限制 9/3/3 合计 15，Canvas `resourceRefs` 最多 40；不能据此宣称插件的 30/10/10 共 50 参考已在 Canvas 可用。
- 既有 Canvas 只读基线：domain 33/33、providers 160/160 合同专项通过。这是旧合同回归，不是新 Yuan 模型或生产验收。

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

客户端创建走 `POST /v1/videos`，查询和下载走 `GET /v1/videos/{网关任务ID}`、`GET /v1/videos/{网关任务ID}/content`。按秒模型为 Official、LJ 2.5 Full、HD 2.5 PerSecond，其余 10 个按次；在价格管理页配置对应的宿主价格后再使用。

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

真实供应商生成权限、素材公网可达性、成片质量及 Canvas 多模态合同尚未验证。当前验证覆盖本地插件/宿主/前端合同，不构成生产部署或真实上游验收。

## 检查点

- 规划与双边合同核对、插件与别名导入实现、完整 Go/前端回归及 HTTP/浏览器验证完成；主代理和原负责子代理已审查最终差异，没有其他阻断项。
- 本地映射校验提示已复用现有七语言翻译；供应商错误保持原文。最终类型检查、构建及隔离启动通过；真实密钥扫描和差异检查通过，源码无用户提供凭据。
- 交付引用为 `fork/main` 和附注 Tag `v2026.10.08-yuanliu-video`，任务提交主题 `feat(yuanliu): 新增视频插件与模型别名自动映射`。提交与远端同步状态以 Git 引用核验；不部署生产、不请求真实付费生成。
