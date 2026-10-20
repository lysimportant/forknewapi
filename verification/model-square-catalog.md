# 模型广场与目录状态（2026-10-16）

- 卡片与表格每批展示 20 个模型，滚动至底部继续追加；提供键盘可操作的“加载更多模型”按钮。筛选或排序改变结果时重置，价格和目录状态刷新不重置已加载数量。
- Seedance Unified Video 使用当前可见渠道的模型名及映射查询上游目录，前端每 60 秒刷新，后端成功结果缓存 90 秒、失败缓存 15 秒。可手动刷新网关状态；仍遵循缓存期限。
- Seedance Unified Video 仅在目录状态为新鲜的 available 时显示；目录缺失、不可用、未知、请求失败或结果超过三分钟时先从广场隐藏，恢复后自动显示。卡片、表格、模型数量和筛选统一使用可见模型；已打开的详情在模型隐藏时关闭。Yuanliu 保持仅隐藏新鲜 sold_out/disabled 状态、未知时展示的策略。目录列出不保证生成成功。
- 广场卡片及详情不再展示价格示例或示例估价；正常按次、按秒等单价和分组倍率保留。插件示例数据、生成请求时长与实际计费公式不变。
- Image2Pro 2.3.0 的 Seedance2.0 新任务按次计费，Flash-MAX 仍按秒。旧任务继续使用冻结的秒数；已有 Seedance 按秒价格需由管理员重新保存为按次单价，不自动换算售价。

此改动只修改源码。实际服务需更新前后端及对应插件后生效；没有修改运行环境、渠道、密钥或线上售价。

## GlobalStore 渠道默认值与文本接入

| 用途 | 新建时的入口 | 默认地址 | 默认模型 |
| --- | --- | --- | --- |
| 图片 | GlobalStore 图片（原生渠道类型 62） | `https://saas.globalstore.ai/kyyReactApiServer` | 8 个内置图片模型，含 `seedream_5.0Pro` |
| 视频 | GlobalStore 视频（原 GlobalStore Model Center，插件 key `globalstore`） | 同上 | 插件声明的 15 个视频模型 |
| 文本 | OpenAI 兼容渠道 | `http://saas.globalstore.ai/gw_llm_power` | 获取上游模型或填写上游提供的文本模型 ID |

图片清单通过已有 `/api/models` 的类型 62 返回值填入，不发起上游发现请求。接口失败或空清单会显示重试入口。新建时切换媒体类型会替换默认模型；已有渠道和手动填写的地址保留，手动输入模型后不再用晚到的默认列表覆盖。

文本接入步骤：添加 OpenAI 兼容渠道，名称填“GlobalStore 文本”，API 地址填上述文本入口，密钥填该服务的文本 API Key；点击获取模型，只选择文本模型，再保存渠道分组和模型价格。若上游不提供模型发现接口，按其控制台给出的精确模型 ID 手动添加。API 地址不要额外拼接 `/v1/chat/completions`，适配器会添加请求路径。

文本地址来自仓库既有 GlobalStore 接入文档；此次未使用真实密钥验证线上文本入口或获取文本模型清单。

## 验收记录（2026-10-17）

| 检查 | 结果 |
| --- | --- |
| `bun run typecheck` | 通过 |
| `bun run build` | 生产前端构建通过 |
| 修改的 TS/TSX/JSON 和两个插件 JS 的目标 `oxlint`、`oxfmt --check` | 通过 |
| pricing 卡片、工具栏、价格示例 | 42 项通过 |
| pricing 触底加载 | 14 项通过 |
| pricing 目录状态、轮询 | 16 项通过；稳定模型引用先复现 3 个表格刷新失败，修复后通过 |
| pricing 动态价格、任务价格展示 | 50 项通过 |
| channels 默认值、渠道配置、供应商选择、插件地址 | 117 项均有通过证据：单进程批次 112 项通过，另 5 项用命令级 15 秒超时定向通过；原 5 秒超时未修改业务断言或全局配置 |
| `go test -mod=readonly ./controller ./router ./plugins -count=1 -timeout=180s` | router、plugins 通过；controller 首次超时后单独复跑 |
| `go test -mod=readonly ./controller -count=1 -timeout=600s` | 完整包通过，227.708 秒；30 项有条件测试跳过 |
| `git diff --check`、新增行凭据模式扫描 | 通过，扫描无匹配 |

生产构建的 Playwright 验收使用 1440 和 390 像素视口、本地模拟 API：

- 卡片、表格共四组：20→40→55 触底追加、末页停止、筛选重置、状态缺失/恢复/失败、详情显示；无页面横向溢出、页面异常或控制台错误。
- GlobalStore 默认值两组：图片 8 个、视频 15 个；API 地址自动填入、切换不混入另一类模型，未提交渠道。头像和版本检查同样使用本地 fixture。
- 中文状态专项：Seedance 和 Yuanliu 均显示“上游暂不可用”。

脚本、截图和 JSON 结果位于本次创建且被忽略的 `.local-tests/model-square-20261016/`。详情组件在 JSDOM 中受到既有 VChart ESM 加载限制，因此由真实浏览器验收，未用图表 mock 冒充详情通过。未运行整个仓库的前端测试集、外部 MySQL/PostgreSQL 矩阵或真实供应商收费请求。

完整后端测试发现并修复了既有即时完成任务保存错误：在设置终态前保存原状态，数据库条件更新使用原状态。即时成功/失败、重复处理、退款等定向测试通过，随后 controller 整包通过；未改变价格或数据库结构。

分工：`square_scroll` 完成自动加载与状态测试，`seedance_availability` 完成目录后端与 GlobalStore 默认值，`image2pro_billing` 完成计费与即时落库修复；主 Agent 完成前端状态/价格/翻译接入、审阅和整体验收。所有任务已验收。未提交、推送或部署；任务开始时两个仓库均干净，画布仓库本轮没有改动。

## Seedance 隐藏与价格示例移除验收（2026-10-20）

本次只修改前端展示：Seedance 仅在三分钟内的新鲜目录结果为 `available` 时出现在广场；缺失、disabled、sold_out、unknown、请求失败和过期均隐藏，恢复后重新显示。数量、供应商与标签筛选同步更新，已打开的详情在模型隐藏时关闭且不会在恢复后自动重开。Yuanliu 保持工作区已有的隐藏策略。

卡片示例估价及详情“价格示例”表格已移除，基础单价、分组价格、倍率、插件示例数据、API 和实际计费逻辑不变。沿用现有列表与详情组件，没有增加 UI 组件、依赖或语言键。

| 命令 / 检查 | 结果与证据 |
| --- | --- |
| `node node_modules/vitest/vitest.mjs run` + pricing 的 `visible-models`、`seedance-visibility`、`yuanliu-visibility`、`seedance-availability-query`、`catalog-availability`、`price-examples-hidden`、`task-price-display`、`model-cards`，`--maxWorkers=1` | 8 文件、96 例均有通过证据：联合批次 7 文件 64 例通过，`model-cards` 的旧示例展示断言按新需求更新后定向 32 例通过；正式单价与分组倍率断言保留 |
| `node_modules/.bin/tsgo.cmd -b` | 最终类型检查通过 |
| `node node_modules/oxlint/bin/oxlint -c .oxlintrc.json <本次 11 个 TS/TSX 文件>` | 通过，无 lint error |
| `node node_modules/oxfmt/bin/oxfmt --check <同上 11 文件>` | 全部格式检查通过 |
| `node node_modules/@rsbuild/core/bin/rsbuild.js build --dist-path D:/newapi/.local-tests/seedance-square-20261020-dist` | 生产构建通过，16.2 秒；使用本次独立产物目录，没有覆盖既有 dist |
| `node .local-tests/seedance-square-20261020/browser-check.cjs` | 1440×950 与 390×844、卡片与表格共四组通过：disabled/缺失/查询失败隐藏、恢复、数量、无不可用徽标、无价格示例、正式单价、详情、Escape、键盘刷新；无页面横向溢出、页面异常或控制台错误 |
| `git diff --check`、本次文件凭据模式扫描 | 通过，无新增凭据匹配 |

行为实现前的 Seedance 可见性用例曾复现 8 个失败；实现后均通过。真实页面集成测试中的失败刷新断言已改为等待异步界面结果。旧任务价格测试的不完整图表 mock 已移除，采用真实依赖；JSDOM 打印 canvas `getContext` 未实现提示，但不影响测试结果。初次测试曾遇到临时目录 EPERM，后续使用进程独立临时目录及串行 worker 正常通过，没有修改全局环境或测试配置。

浏览器验收在本地临时端口启动独立静态服务器，API 全部使用合成 fixture，外部请求阻止；没有访问真实上游、用户数据或收费接口。脚本、截图和 JSON 结果保留在被忽略的 `.local-tests/seedance-square-20261020/`，服务随脚本结束停止。截图已检查桌面卡片与手机详情布局。

分工与状态：T1 主 Agent 完成 Seedance 可见性及回归测试；T2 `hide_price_examples` 完成卡片、详情示例移除及价格测试；T3 主 Agent 完成集成审阅、验证与记录。三项均已验收。用户开始时的后端、Yuanliu、测试配置及其他已有改动保留。本次未运行整仓前端或 Go 全套测试、真实供应商请求；未修改后端、计费、全局配置，未提交、推送或部署。
