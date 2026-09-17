# rc.37 定制兼容升级

本次从 fork `6e44f3d990aa700958e8f8cf88049639293ba455` / `v1.0.0-rc.35.custom.12` 合入官方 `v1.0.0-rc.37`（`385d2dfd10d821b25c8a6766bd16eea248cb1652`），交付版本为 `v1.0.0-rc.37.custom.1`。只合入该发布 Tag，不追随之后的官方 main。验收使用隔离工作区、临时数据库和模拟供应商，不操作生产。

## 功能与兼容决策

| 范围 | 合入的官方能力 | 保留的 fork 行为 |
| --- | --- | --- |
| 公告与请求错误 | 统一 Query/Mutation 错误处理、后台版本提醒 | 即时保存、失败保留输入、取消旧查询、置顶时间轴、当天关闭、旧 Notice 单独展示 |
| 登录 | Passkey 主域名和兼容 RP ID、域名迁移与错误分类 | 登录前主动同意并在服务端绑定版本，协议 Dialog 阅读，注册无需同意且不创建会话，QQ 邮箱提示 |
| 定价 | fixed()、图片数量和缓存细分、币种编辑、供应商独立价格、可视编辑与请求模拟 | effort 推理档位、管理员价格优先和显式零值、视频清晰度 tabs 与额外规格折叠 |
| 任务插件与渠道 | usageProfiles、插件详情及市场信息、渠道选择配置改进 | 现行视频插件、Sora 六种配置尺寸、Kling credit 单位；不恢复已淘汰的视频实现 |
| 钱包与兑换码 | 官方兑换码批量接口和导出 | 48 小时冻结、手动转入余额、幂等重试、单码复制、首行提取和历史空白匹配 |
| 公共页面 | 适配官方导航和状态查询 | 关于页内容及原始归属、可配置文档入口、API 地址展示复制、Sub2 耗时颜色 |
| 转发 | rc.37 用量 DTO 与流读取改进 | /responses 和 /v1/responses、DeepSeek URL 去重、工具返回关联、流式唯一终态 |

保持原 Dockerfile、docker-compose、端口、挂载卷和部署配置；Go 模块依赖不变。前端依照官方锁文件增加 `yaml@2.9.0`，使用冻结安装。七种语言各 6571 个键，键集合一致。

真实验收额外修复三类缺陷：推荐奖励在重启时被重复结转或把已入账的受邀赠送计入提现；配置主键迁移失败仍继续启动；单行输入框先吞掉多行粘贴中的换行，导致兑换码首行提取失效。兑换码现在仅对含换行/制表符的粘贴复用原提取函数，普通单行光标粘贴保持原行为。

最后核对 Responses 转换时，补齐官方新缓存模态字段在双向转换及客户端输出中的传递。沿用现有 Clone，保留缺失与显式零值、原用量和计费快照隔离，客户端继续过滤内部快照；10 个新增回归先复现失败再通过。

## 主 agent 与 sub-agent 分工

| Agent | 实际完成的工作 | 复核与证据 |
| --- | --- | --- |
| 主 agent | 认证冲突、协议与多 RP ID 联合流程、语言文件结构化合并、依赖、集成构建、全量检查、HTTP 验收、最终差异和 Git 交付 | 协议过期/缺失及 RP ID 重试回归，统一汇总以下检查 |
| announcement_review | Header、公告及其他设置、钱包 hooks、兑换码、公共错误处理；随后用真实浏览器发现并修复结构化粘贴 | 原专项 22 文件/175 测试；钱包 8 文件/23 测试，修复前失败证据及最终浏览器报告 |
| option_persistence | model 合并、迁移失败处理、奖励账本缺陷修复、三库迁移和接口验证；只读复核 Responses 定制 | [数据库记录](database.md)：18 个迁移组合、36 次启动、六组提现接口 |
| release_baseline | 计费表达式、管理员价格覆盖、任务插件、前端新表达式引擎与旧 effort 兼容、定价界面 | [计费记录](billing.md)：36 文件/603 测试、五个 Go 包；另检查 Responses 缓存细分传递 |

主 agent 负责最终整合，子任务专项通过不单独作为整仓完成证据。所有手工冲突已解决，现有定制没有被选择性丢弃。

## 验证环境与记录

运行时为 Windows Go 1.26.0、Node 24.12.0、Bun 1.4.1、Docker 29.7.2；Dockerfile 固定 Bun 1.4.0、Go 1.26.1。前端使用 `bun install --frozen-lockfile`。原始证据位于本地忽略目录 `D:/newapi/.local-tests/rc37-acceptance` 与 `D:/newapi/.local-tests/rc37-*.log`。

| 验证 | 命令或入口 | 结果 |
| --- | --- | --- |
| 前端全量 | `bun run test --maxWorkers=2 --testTimeout=20000` | 154 文件、1655 测试通过；首次在并行构建负载下触发的 5 秒超时改用命令行 20 秒上限，仓库默认配置不变 |
| 新增粘贴回归 | `vitest run src/features/wallet` | 8 文件、23 测试通过，包括全量收集后新增的 4 项粘贴用例 |
| 类型与前端构建 | `bun run build:check`，设置版本变量 | 最终前端通过，见 `rc37-build-delivery.log` |
| 后端全量及构建 | `GOWORK=off go test ./... -p 1 -count=1 -timeout=300s`、`go build ./...`、`go vet ./...` | 最终 44 个测试包全部通过，根包及其他无测试包编译成功；build/vet 退出 0，见 `rc37-go-{delivery,build-delivery,vet-delivery}.log` |
| relaykit 独立模块 | `GOWORK=off go build ./...`、`go test ./... -count=1 -timeout=180s` | 最终转换修复后全部通过 |
| 三库 | `database-verify.ps1 -Mode all` / `-Mode controller` | 18 个迁移组合、六组提现、定价及 Passkey 专项全通过，无跳过；详见数据库报告 |
| 桌面浏览器 | `announcement-browser.cjs`、`core-browser.cjs` | 最终同一 Windows 产物分别 10/10、14/14 通过；本地 console/page errors 为空，1440×900、1280×720 截图已复核 |
| 完整 HTTP 链路 | `docker run --network none ... node /out/relay-runner.mjs` | 最终 Linux 产物 39/39 通过：36 项 Responses 路径/渠道/流式/工具往返、消费日志审计、Sora 预扣结算及失败退款 |
| Docker 构建 | 原 Dockerfile，`forknewapi:rc37-custom-verification` | 最终构建通过；仅进程级代理恢复本地 registry 连通性，无全局配置修改 |
| 静态差异 | scoped lint/format、gofmt、`git diff --check`、新增内容凭据特征扫描 | 手工兼容修改检查通过；四处 URL 凭据命中均为官方 `example.com` 拒绝测试夹具，无真实凭据 |

最终 Windows 文件 `new-api-rc37-delivery.exe` SHA-256 为 `6982307aca447d4c3b5da7130482d7364784dfa227a967334efbf927fad1d121`；使用忽略目录中的 Go overlay 仅把监听限制到 loopback，源 `main.go` 不变。预览为 `http://127.0.0.1:3022/about`，数据使用独立 SQLite。浏览器 3023 专项进程已退出，原 3021 预览未切换。

最终 Docker image config SHA-256 为 `a13d16bacf653d5d18f2e581ced4ae43c84ffede5be0755f223dfe926dbc69e5`，提取的 Linux 程序 SHA-256 为 `cd1f1b6e64cbfb7bc19fec5dc96a1370919b521f485b0f2402bde0b444dd065b`。HTTP 验收使用 `--network none`、全新 tmpfs 数据库和同容器 loopback 供应商，实际请求不访问外部付费服务。

早期后端检查与前端重建并发，根包嵌入 `web/dist` 时遇到目录被构建清理；前端结束后根包重试通过。最终检查在产物稳定后顺序执行，不用该次环境失败掩盖代码问题。最初的安全页面、关于页与定价测试适配已修正并全量复验。

认证合并按 [OWASP Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html) 和 [Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html) 核对。ASVS 5.0.0 参考沿用 `next.md` 中的 V6.3.1、V6.3.8、V6.5.1、V7.2.4、V7.4.1 等控制；本轮保护服务端确认门禁、挑战过期/单次消费、RP ID 与凭据匹配及错误不泄露挑战。没有宣称整站 ASVS 合规。

## 限制、迁移与回滚

- 全仓 lint 有既有存量问题：当前 192 个 error，官方 rc.37 基线 194 个，fork 原基线 298 个。逐条按文件/规则/消息比较没有本次新增诊断；手工兼容改动文件单独通过。未用无关重构扩大升级范围。
- 三库使用 SQLite 3.50.4、MySQL 8.0.46、PostgreSQL 16.15，未验证最低版本或 ClickHouse。详见数据库报告，不把单元测试当作所有数据库兼容的证明。
- 未调用付费模型或验证外部 OAuth/微信/Telegram/真实 Passkey 认证器；模拟供应商的 HTTP 通过不代表外部服务可用。
- 钱包嵌入的外部商店在浏览器验收中返回 HTTP 403，单独记入外部资源错误，不声称商店访问正常。
- 未自动修正历史上已生成的异常奖励账本。发现既有重复结转时，应另行结合奖励来源和提现记录对账。
- 生产上线仍需独立授权、维护窗口、停止写入并备份主库/日志库、配置及旧镜像，验证备份可恢复。options 主键修复会保留原表备份；MySQL DDL 不保证事务性回滚。回退应使用旧代码加配套数据库备份，不能让旧程序直接写未经验证的新 schema。
- `next.md` 中外部身份联调、生产 HTTPS/可信代理、保留期限和失败登录审计等原有待验事项继续保留。
