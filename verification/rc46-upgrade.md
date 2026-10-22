# RC.46 定制升级与 Image2Pro 移除

日期：2026-10-22。工作区：`D:\newapi`。

## 版本与范围

- 原 fork 提交：`68a35871f`，版本 `v1.0.0-rc.37.custom.20`。
- 合并官方 `v1.0.0-rc.46`：`7d8b91268dab69d7150f345c448b5e250163146a`。
- 新版本：`v1.0.0-rc.46.custom.1`。
- 交付分支：`codex/remove-image2pro-upgrade-rc46`，授权远端为 `fork`（`lysimportant/forknewapi`），不推送 `origin`。
- 保留 Canvas 桥接、邀请奖励账本、额度及定价/页面等现有定制；保留接手时的暂存和未暂存改动，没有 reset、clean 或中止合并。

删除内置 Image2Pro 插件、专属测试和文档，以及它引入的 `openai_image_tasks`、`/v1/images/tasks`、专属转换接口和路由例外。图片处理采用官方 `openai_image` 与 `/v1/images/generations`、`/v1/images/edits`；上游异步图片任务由官方桥接在请求内轮询，客户端使用标准图片响应。视频和通用任务接口继续保留。旧 Image2Pro 专属协议的客户端不再兼容，需要改用正式图片接口。

官方 RC.46 对缩窄后的插件用量定义投影旧价格表达式；同步更新与该官方行为冲突的旧测试，未另改定价产品语义。投影依赖插件对退役用量报告约定值，不能泛化为任意未知收费项都可安全省略。

## 整合修复与分工

| 任务 | 负责人 | 结果 |
| --- | --- | --- |
| R1 Responses SSE 终态与日志脱敏 | catalog_impl | done；incomplete 保留独立终态，日志不存上游错误原文，计量分支保留 |
| R2 旧异步图片协议清理 | canvas_binding_audit | done；插件、Controller、Router、Relay、中间件定向回归通过 |
| R3 邀请奖励日志断言 | image2pro_dispatch_audit | done；适配结构化日志，移除检查前清空主库日志表的错误操作 |
| F1 Vitest 依赖加载 | 协调者 | done；CodeMirror/i18next 使用一致实例，图表 mock 兼容模块互操作，全量前端测试通过 |
| F2 定价测试兼容 | catalog_impl | done；7 个套件 210 项通过 |
| F3 设置请求测试兼容 | image2pro_dispatch_audit | done；4 文件 34 项通过，精确验证参数与失败重试 |
| F4 移动日志复制 | canvas_binding_audit | done；复用 CopyButton，键盘复制与模型差异详情回归通过 |
| L1/L2 涉及文件 lint | image2pro_dispatch_audit / canvas_binding_audit | done；11 项错误消除，不改变定价或附件提交契约 |
| G1 后端完整验收 | catalog_impl | done；三个模块 test/build/vet 全通过 |

## 验证

工具链为 Go 1.26.10。根模块、relaykit、tokenkit 的声明分别为 1.26.10、1.25.1、1.26。前端使用现有 Node/npm 和本地依赖运行项目脚本；本次续作的 PATH 中 Bun 不可用，没有重新安装依赖。

| 命令/检查 | 结果与证据 |
| --- | --- |
| 根 `go test -json ./...` | 退出 0；50 个包通过，7,156 条测试及子测试通过、103 条跳过；34 个包没有测试 |
| 根 `go vet ./...`、`go build ./...` | 全部退出 0 |
| relaykit：`GOWORK=off go test ./...` / `build` / `vet` | 全部退出 0；617 条测试及子测试通过 |
| tokenkit：`GOWORK=off go test ./...` / `build` / `vet` | 全部退出 0；7 条测试通过 |
| `go test ./controller -count=1` | 单独全量通过，176.131 秒 |
| 邀请提现共用/独立 SQLite 日志库 | SQLite 3.50.4，两种配置均通过冻结、幂等、余额、日志数量/内容/落库位置验证 |
| `npm run typecheck` | 退出 0 |
| `npm run build` | 退出 0，生产资源构建成功 |
| `node node_modules/vitest/vitest.mjs run` | 退出 0；242 个测试文件、2,838 个用例全部通过，零失败、零跳过；证据 `web-final.json` |
| 449 个变更前端源文件 oxlint | 退出 0，保留既有 warning |
| `git diff --check` | 通过；清除合并产生的一处 EOF 空行 |
| 已变更文本的凭据模式扫描 | 1,098 个文本文件；仅命中 PEM 解析器中的两处固定标记，未发现实际凭据。此扫描不等于完整安全认证 |
| 废弃协议/运行实现扫描 | Image2Pro 插件、旧异步图片协议和路径均无运行代码残留；历史验收记录仍有旧 Agent 名称 |

原始证据位于忽略目录 `.local-tests/rc46-acceptance/g1-*`、`web/.local-tests/rc46-acceptance/`。`g1-summary.json` 包含准确退出码及跳过用例。测试数据使用独立临时 fixture，没有连接站点数据库。

## 仍未完成的环境验收

- Docker Linux 引擎无法连接：`npipe:////./pipe/dockerDesktopLinuxEngine` 不存在。MySQL/PostgreSQL 真实实例、新建/升级/重复迁移完整矩阵未运行；外部数据库及真实 vLLM 用例按环境条件跳过。不能据此声明三数据库兼容性验收完成。
- 全仓 lint 初始有 151 个 error；本次触及四文件的 11 个已消除，其余 140 个位于未改文件，未扩展为全仓重构。变更文件 lint 已通过。
- 变更文件格式检查还包含接手时已有的格式问题；只格式化本轮实际修改的相关文件，没有批量格式化所有用户/上游内容。
- 未做线上图片生成调用、生产数据库迁移、Docker 镜像构建或真实浏览器全站验收。本次仅交付 Git 代码，不代表生产部署已通过全部验收。
- 没有操作线上已上传插件、渠道、价格、令牌、用户数据或画布项目，也没有部署。数据库中的上传覆盖版本不会因删除内置源码而自动删除。

所有本轮子 Agent 产物保留。最终推送结果以 Git 远端提交核对为准。
