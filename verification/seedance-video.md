# Seedance 统一视频插件

## 目标与基线

- P1：实现 `seedance-video` 插件，同协议渠道复用源码，仅替换渠道地址、密钥、模型选择和映射。模型名称由管理员在渠道维护，不使用插件源码白名单，也不按名称猜测模型能力。
- 基线：2026-10-11，`main@8602933e6`，跟踪 `fork/main`，工作区干净。Go 1.26.0、Node 24.12.0；模块要求 Go 1.25.1。已有前端本地格式化和 lint 工具，Bun 不在 PATH，不安装依赖。
- 基线测试：`go test -mod=readonly ./plugins ./pkg/jsplugin -count=1 -timeout=180s` 通过。
- 合同来源：用户提供的“纵横科技多能力 API”文本；此前只读模型目录返回 19 项，不从模型名称猜测归属或自动开放生成。
- 验收：渠道显式授权任意精确模型名、同插件多渠道独立映射、真实模型目录、地址切换、创建幂等、状态解析、公开素材字段、显式 false、按次计量、无凭据内容描述和既有插件兼容。新增可选插件合同字段，扩展现有渠道只读查询；无数据库结构、依赖或 UI 修改，不部署生产，不调用收费生成。

## 配置与边界

- 新建“任务插件”渠道（类型 61），绑定 `seedance-video`，地址默认 `https://cnd-coo-new.pages.dev`，Key 仅保存在管理员渠道配置。`GET /v1/models` 复用宿主 Bearer 模型发现，返回真实目录供管理员选择。目录同时包含其他能力时，只选择已确认使用该视频协议的模型。
- 获取或手填模型并保存即可使用，无需修改或重载插件源码。可以直接保存 `Xseadanco2.0`、`kksedanco2.5`，也可以保存本站模型名再映射到这些上游名。同一个本站名称在不同渠道可以有不同映射。在管理端配置每视频的本站售价；平台积分不是本站价格，插件不内置供应商价格。
- `meta.modelRouting: "channel"` 不注册全局模型，仅对启用且显式绑定插件的渠道已保存精确模型名授权。未保存大小写不自动纠正；跨插件绑定歧义拒绝固定插件。映射在最终渠道选中后执行，解码器不得切换已授权的客户端模型。
- 已有插件的全局模型声明优先。若本站名称与全局声明冲突（含唯一大小写折叠匹配），应使用另一个本站别名映射到实际上游 ID；不同插件的渠道名称冲突同样须消除。相同插件下的多个渠道可共享本站名称。
- 旧插件省略该字段，保留 `meta.models` 声明、目录交集和别名行为。现有 API v1 文档、schema、TypeScript 类型同步。旧版宿主拒绝新字段，必须先更新应用再安装新插件。
- 客户端创建 `POST /v1/videos`；插件出站创建 `/v1/videos`，查询 `/v1/tasks/{task_id}`。渠道地址支持根地址、`/v1` 和自定义路径前缀。
- 统一字段：`model`、`prompt`、`duration`、`ratio`、`resolution`、`quality`、`negative_prompt`、`images`、`start_frame`、`end_frame`、`reference_videos`、`reference_audios`、`generate_audio`。入口兼容 `seconds`、`aspect_ratio`、`size`、URL 字符串 `input_reference`；冲突字段报错，不透传 `extra` 或厂商原生字段。
- 时长只应用宿主安全上限 3600 秒，不代表模型能力；不补默认时长、分辨率或素材数量限制。具体模型能力以渠道后台为准。音频开关的 `false` 保留，首尾帧中的尾帧要求同时提供首帧。
- 支持 JSON 和纯文本 multipart，素材须先取得公网 HTTPS 直链；本地文件先通过上游独立 `/v1/media` 上传，本轮不扩展宿主多步上传流程。
- 稳定网关任务 ID 用作 `Idempotency-Key`，未知提交结果禁止宿主自动重发；此机制不提供跨客户端独立请求去重。
- 状态仅映射 `processing`、`succeeded`、`failed`。成功必须带有效 `video_url` / `url` / `result_url` / `download_url`；由宿主无凭据代理，避免把渠道 Key 发往 CDN。失败原因保留错误码与详情并脱敏；客户端查询不回显原始供应商正文。
- `next_poll_seconds` 未被宿主调度契约支持，继续采用宿主轮询间隔；本轮不声称已按供应商建议动态调度。
- 凭据处理参考 OWASP Authentication Cheat Sheet 和 Session Management Cheat Sheet：Bearer 仅用于同渠道 API，内容代理无凭据，错误不回显密钥。本轮不修改登录或会话认证。
- 影响与回滚：渠道别名索引增加读取已有 `setting` 列，使用只读 JSON 解析，不触发配置修复写回；数据库结构与既有记录无需迁移。回滚前停用该插件渠道的新提交，待在途任务结束后回退应用提交；保留已有渠道和任务记录，新插件须与支持该字段的宿主配套使用。

## 检查点

- 已完成插件、宿主渠道模型授权、模型目录及合同三件套实现。插件元数据人工审查：`count` 描述为“Video generation unit price / 视频生成单价”，单位为 `count`，不嵌价格或协议限制。
- 已通过 `go test -mod=readonly ./pkg/jsplugin -count=1 -timeout=180s`；目录新建/编辑/失败兼容回归通过。schema 17 个正反合同用例与独立 `.d.ts` 类型检查通过。
- 插件、middleware、任务适配器、router 和 service 全包回归通过。`TestSeedanceVideoHostFlow` 使用本地 HTTP 上游验证三个不同模型名的创建、查询、完成用量和无凭据内容描述；不等于真实下载、实际退款入账或供应商收费验收。
- 三数据库矩阵及旧别名/定价回归通过，无跳过：SQLite 3.50.4、MySQL 8.0.46、PostgreSQL 16.15。命令：`GOWORK=off go test ./model -run 'TestTaskModelAliasChannelRouting|TestTaskPluginChannelSelection|TestSharedPluginKeys|TestPricingAlias|TestPricingUsageProfiles' -count=1 -timeout=120s -v`，耗时 4.913s。MySQL、PostgreSQL 使用仅本任务创建的本地隔离实例和 `TEST_MYSQL_DSN`、`TEST_POSTGRES_DSN`；测试仅创建并删除独立表前缀，不接触业务数据库。
- Go build 全模块通过：`go build -mod=readonly ./...`。Go vet 通过：`go vet -mod=readonly ./plugins ./pkg/jsplugin ./middleware ./controller ./relay/channel/task/jsplugin ./router ./service` 及 `go vet ./model`。
- 新插件 oxfmt 与 oxlint 通过。首次集中回归的 controller 全包超过 180 秒时限；延长至 600 秒后执行 202.001s，仅 `TestSecurityAccountDeletionConcurrentRequestsHaveOneWinner` 失败，日志出现 SQLite `SQLITE_BUSY`，成功请求数为 0。随后 `go test -mod=readonly ./controller -run '^TestSecurityAccountDeletionConcurrentRequestsHaveOneWinner$' -count=1 -timeout=120s` 通过（3.367s），完整 `go test -mod=readonly ./controller -count=1 -timeout=600s` 复查通过（192.262s）。保留首次失败证据，未修改账户删除实现，未认定为既有问题。
- 最终生产 diff、Go 格式及敏感信息检查通过。无 UI、依赖、登录认证或数据库结构变更；此前只读前端检查中的权限受限操作员用例超时，未在本轮修改其实现。本轮相关 Go 包最终验证通过，供应商收费生成、真实内容下载和实际退款入账仍未实测。
- 交付分支为 `main`，远端为 `fork`（`https://github.com/lysimportant/forknewapi.git`），任务带注释 Tag 为 `v2026.10.11-seedance-channel-routing`；不向官方 `origin` 推送。提交前远端 `main` 为基线 `8602933e6`，Tag 尚未使用。仅清理本任务两个隔离数据库容器，不操作其他容器或业务数据。

## 可选计费方案（2026-10-12）

- P1：为 `seedance-video` 提供互斥的“按次”和“秒数 × 清晰度”售价编辑。按次一个单价，按秒提供 480p、720p、1080p、2K 四档每秒单价；默认模型和插件独立售价使用同一入口。验收包括模式恢复、单价保存、旧价格保持、零价缺计量拒绝、成功冻结结算、失败退款和 PC 界面交互。不扩展其他供应商协议，不调用付费生成。
- 本轮基线：`main@d9f0c91c2`，跟踪 `fork/main`，工作区干净；Go 1.26.0、Node 24.12.0，依赖已存在，Bun 不在 PATH。基线 `go test -mod=readonly ./plugins ./pkg/billingexpr -count=1 -timeout=180s` 通过；前端原 forks 池只输出 RUN，恢复为 threads 后原 `plugin-pricing.test.tsx` 7/7 通过，未修改 Vitest 配置。
- 插件升至 1.1.0，增加 `seconds`（second）和独立计费枚举 `video_resolution`。后者避免宿主递归枚举校验误拒绝旧按次请求的 `resolution`；上游仍收到请求原值。仅将 480/720/1080 与对应 p 后缀、大小写 2K 归一化计费，不推定像素尺寸、模型默认值或上游支持范围。
- `count=1` 始终保留；只有合法时长和四档清晰度同时明确时才提供秒数事实。按秒每个表达式叶子包含 `u("seconds") * price`，包括零价；缺时长、缺清晰度或未知清晰度会在宿主预扣及出站前求值失败，而旧按次表达式继续有效。`billing_ratios` 仍为空，不重复叠加两种用量。
- 按秒使用请求时长，上游响应没有已确认的实际时长合同。成功 hook 仅返回 count，宿主将其合并到被冻结的时长、清晰度与售价表达式；在途任务不因后来改价重计。失败仍由既有宿主退款路径处理。
- 沿用模型表达式与插件售价覆盖存储，不增加数据库字段、迁移、依赖或计费引擎。插件元数据人工复核：两个数值字段均为“视频生成单价 / Video generation unit price”，单位分别为 count/second，枚举为“输出视频分辨率 / Output video resolution”，不嵌数字价格。
- 回滚：暂停该插件新提交，等在途任务结束；先将按秒售价切回按次并保存，再回退本次应用提交。保留渠道和任务记录；旧插件源码不声明秒数/清晰度事实，不能继续使用本轮新增按秒表达式。
- 页面入口：系统管理 → 计费与支付 → 模型定价 → 编辑已绑定 `seedance-video` 的模型。默认页和该插件独立售价页均提供“每次调用 / 按秒 × 清晰度”Tab，另保留表达式编辑器。按秒售价是请求时长乘对应清晰度的每秒单价，仍应用现有组倍率和请求规则；配置四档不表示上游模型支持四档。
- UI 复用：原 `TaskUsagePricingEditor` 允许多个数值用量相加，无法直接表达两种互斥方案，因此新增 Seedance 业务组合；复用 `PricingAmountInput`、Tabs、Field、`ConfirmDialog` 和原编辑器的表达式、预览能力。按精确插件 key 选择入口，不按模型名称猜测。未配置默认按次，旧公式打开不改写，复杂公式保留在高级编辑器，替换前确认；模式草稿各自保留，只有保存才生效。
- Go 验证通过：`go test -mod=readonly ./plugins -run TestSeedance -count=1 -timeout=180s`；`go test -mod=readonly ./plugins ./pkg/billingexpr ./relay/channel/task/jsplugin ./service ./relay -count=1 -timeout=180s`；`go build -mod=readonly ./...`；`go vet -mod=readonly ./plugins ./pkg/billingexpr ./relay/channel/task/jsplugin ./service ./relay`。真实 JS 引擎、宿主适配器和本地 HTTP 上游覆盖四档、入口别名、小数秒、原值透传、未知尺寸按次、零价缺计量拒绝及冻结事实结算；既有宿主退款回归通过，不等于真实供应商失败或业务账户退款入账验收。
- 前端回归：`node node_modules/vitest/vitest.mjs run src/features/system-settings/models/__tests__/plugin-pricing.test.tsx --pool=threads --maxWorkers=1 --testTimeout=10000 --reporter=verbose`，14/14 通过；`node node_modules/@typescript/native-preview/bin/tsgo -b` 全项目类型检查通过；修改的五份 TSX 文件格式与 oxlint 检查通过。七语言各新增六个 key，经技能要求的插入脚本和 `node scripts/sync-i18n.mjs` 同步，临时脚本不交付。插件 oxfmt/oxlint 通过。
- 生产构建：在 `web/` 执行 `node node_modules/@rsbuild/core/bin/rsbuild.js build`，10.1s 成功。Bun 不在 PATH，采用项目已安装工具的 Node CLI，未修改依赖或锁文件。
- PC 浏览器 smoke：`node .local-tests/seedance-pricing/browser-smoke.cjs`，中文浅色 1440×1000、英文深色 1280×1000 均通过。使用真实页面和本地 API fixture，覆盖两套草稿切换、四档输入、默认按秒与插件按次独立保存、PATCH 版本及精确模型名、刷新恢复、复杂公式保留和取消替换；无未知接口或 pageErrors，外网请求被阻断。截图已人工检查，证据留在忽略的 `.local-tests/seedance-pricing/evidence/`，没有真实账号或付费调用。
- 待跟进：开发模式浏览器仍输出同一种 Base UI `nativeButton` 组合告警，定位为共享 `TaskBillingPreview → ComboboxInput → InputGroupButton → Button`；本轮未改共享组件。原七个定价基线用例没有留存同告警的运行证据，因此不认定为已证实的基线问题，也不声称控制台零错误；此次保存、恢复和切换断言均通过。
- 检查点：本轮实现、相关回归、类型检查和构建已完成。无数据库行为、依赖或宿主合同变化；不运行本轮不涉及的三数据库矩阵。收费上游生成、实际下载及供应商实际时长仍未验收。交付 `main` 至 `fork`（`https://github.com/lysimportant/forknewapi.git`），带注释 Tag `v2026.10.12-seedance-billing-modes`。

## 渠道模型计费入口修复（2026-10-12）

- P1；基线 `main@8a7fb6f79`，跟踪 `fork/main`，开始时工作区干净。Go 1.26.0、Node 24.12.0，已有前端依赖可用，Bun 不在 PATH。
- 问题：`/models/metadata` 编辑渠道授权模型 `LLseadanco2.5` 时显示通用 Token 编辑器。渠道已绑定 `seedance-video`，但定价快照只返回 usage schema，旧页面依赖插件全局声明生成的 variants 识别专用编辑器；该插件不声明全局模型，因此无法识别。
- 修复：定价快照增加只读可选 `task_plugin_key`，将唯一绑定的插件身份传入模型元数据和系统定价入口；只按精确插件 key 识别，不猜测模型名称或 schema。共享声明模型仍通过各插件 variants 区分。新建渠道成功后立即刷新渠道缓存，避免等待同步周期。
- 影响与回滚：接口只新增可选返回字段，无数据库结构或定价存储变化，无需迁移。保留已存模型名、映射、价格和任务；回退本次提交即可恢复旧页面行为。
- Go 检查通过：`go build -mod=readonly ./...`；`go vet -mod=readonly ./model ./controller`；`go test -mod=readonly ./model -count=1 -timeout=300s`（43.340s）；`go test -mod=readonly ./controller -run 'Channel|ModelPricingConfig|Pricing' -count=1 -timeout=180s`（7.650s）。新增渠道用例覆盖启用与关闭内存缓存，快照用例覆盖任意渠道模型名、唯一声明模型、共享声明模型和普通模型。
- 真实三数据库矩阵通过：SQLite 3.50.4、MySQL 8.0.46、PostgreSQL 16.15。执行 `pwsh -NoProfile -File .local-tests/seedance-pricing/metadata-matrix.ps1`，内部运行 `go test -mod=readonly ./model -run '^TestTaskModelAliasChannelRouting$' -count=1 -timeout=180s -v`，无跳过，4.566s。仅使用隔离实例和唯一测试表，测试容器及匿名卷已清理。
- 前端检查通过：`node node_modules/vitest/vitest.mjs run src/features/system-settings/models/__tests__/plugin-pricing.test.tsx --pool=threads --maxWorkers=1 --testTimeout=10000 --reporter=verbose`，19/19，38.11s；`node node_modules/@typescript/native-preview/bin/tsgo -b`；涉及的五份前端文件 oxlint/oxfmt；`git diff --check`。覆盖无 variants 的渠道模型、原始公式与版本保存、其他/近似/缺失插件 key，以及原共享模型路径。Vitest 仍显示前文记录的共享 Combobox `nativeButton` 告警。
- 生产镜像构建通过：`docker compose build new-api`，锁定的 Bun 1.4.0、Go 1.26.1-alpine 构建成功，未修改依赖或锁文件。
- 真实前后端 PC smoke 通过：使用同一生产镜像、回环端口 43926、临时 SQLite 数据库，通过正式 setup 与加密登录 UI 获得测试账号会话，上传插件并新建 `LLseadanco2.5` 渠道。立即确认快照返回 `task_plugin_key=seedance-video`、秒数 schema 且无 variants；在真实 `/models/metadata` 切换两种方案，保存四档每秒单价及按次单价，重新进入后恢复，两个 PATCH 均成功且保留精确模型名与版本。截图已人工检查；无 pageErrors。登录前 `/api/user/auth/refresh` 返回预期 401，因此不声称控制台零错误。脚本与脱敏证据留在忽略的 `.local-tests/seedance-pricing/`，未进行上游生成调用。
- 本地更新（2026-10-13）：`docker compose up -d --no-deps new-api`，应用与 PostgreSQL/Redis 均 healthy；应用使用已验证镜像，`/api/status` 成功、`/models/metadata` 返回 200。实际用户已有账号页面未代登录，Tab 的完整操作验证来自上述隔离实例；没有改用户模型价格或数据库结构。旧镜像保留为 `forknewapi:seedance-metadata-before-fix` 便于本地回滚。
- 检查点：实现、回归、三数据库、生产构建与本地更新完成；交付分支为 `main`，远端为 `fork`（`https://github.com/lysimportant/forknewapi.git`），带注释 Tag `v2026.10.13-seedance-pricing-routing`。本轮完成的是计费入口识别，供应商收费生成仍未验收。
