# 钱包支付方式与充值返佣

本轮为 P1、跨前后端账务变更。基线 `main@e47a383a8`，工作区干净，跟踪 `fork/main`（`https://github.com/lysimportant/forknewapi.git`）。Go 1.26.0、Node 24.12.0，Bun 使用仓库既有本地入口；前端依赖已存在，不升级依赖。原始测试基线由前端、数据库子 agent 分别记录。

验收：钱包默认微信，可切换支付宝兑换码店铺，嵌入页和新窗口链接同步切换；取消邀请人注册固定奖励，保留受邀人注册赠送；每笔成功在线充值或兑换码充值按到账余额的 10% 返给直接邀请人，固定冻结 48 小时，原有手动提现规则保留。用户已明确确认返佣基数与兑换码参与。额度以最小整数单位向下取整，不跨级返佣，不对历史充值补发。

影响与回退：复用既有奖励账本及唯一来源索引，不更改表结构，不重写历史奖励；订单状态、充值入账与返佣同事务提交，失败全部回滚。仅操作隔离测试数据库。生产上线前备份主库、日志库、配置及程序；回退程序不会删除已产生返佣，旧提现逻辑仍识别账本及显式到期时间，恢复注册固定奖励前需复核旧配置。

范围外：生产部署、店铺实际支付联调、历史奖励清理、移动端重设计。

- [x] 读取规则、README、已有 next.md 与升级验收记录，确认业务口径和分工。
- [x] 前端 Tabs、推荐文案、管理端旧奖励设置及七语言翻译。
- [x] 注册去掉邀请人固定奖励、各充值入口事务返佣、冻结与幂等回归。
- [x] SQLite/MySQL/PostgreSQL 专项、前后端检查、启动和浏览器冒烟；全仓存量问题如下记录。
- [x] 最终 diff、格式、无关改动与新增凭据特征检查。

Git 交付：本检查点随任务中文提交交付，中文 annotated Tag 为 `v1.0.0-rc.37.custom.2`，目标 `fork/main`；提交、Tag 与远端一致性以最后 `git ls-remote` 核验结果为准。

认证边界：只调整注册完成后的奖励副作用，不改变注册校验、身份绑定、会话、口令或认证响应。实施前阅读 OWASP Authentication Cheat Sheet、Session Management Cheat Sheet；现有认证回归随 controller 测试执行，不声称整站 ASVS 合规。

实现及本次功能验收完成。前端专项 9 文件/29 测试、typecheck、修改文件 lint/format、生产 build 与 i18n 同步通过；全仓 lint 对比基线同为 192 errors/67 warnings，新增 0。后端全量 44 个测试包、build/vet 通过；最终锁顺序修复后的 model 全量、相关 vet、三库 121 条 PASS/六组提现/24 次启动及浏览器 13 项再次通过，页面与应用控制台错误均为 0。最终补充非零注册赠送的四并发三库测试通过。集成程序使用只替换监听地址的 Go overlay 限制到 127.0.0.1，测试结束已关闭。

实现同时补齐 OAuth 创建用户时的邀请关系持久化；奖励汇总改为统计完整账本，最近 50 笔限制只用于明细，避免多笔返佣把更早的冻结余额误报为可提。注册零额度幂等记录不显示为奖励，也不阻止旧余额的历史结转。

前端首次全量为 1656/1663，两处未改模块有 7 项失败（6 个超时和 1 个后续输入干扰）；同依赖的原提交副本可复现渠道权限测试超时。当前相同两文件单 worker 复核 62/62 通过，未改变测试或超时配置。最终 `bun run test --maxWorkers=2` 为 154/155 文件、1662/1663 用例通过，仅剩同一个基线渠道权限场景的 5000ms 超时；该范围外不稳定测试留待独立处理，保留所有失败及复核证据，不宣称完整前端测试全绿。

后端首次 `go test ./...` 全通过；最后独立重跑 controller 包出现未改动的 `TestSecurityAccountDeletionConcurrentRequestsHaveOneWinner` SQLite `SQLITE_BUSY`（两并发删除无成功响应）。随后 `go test ./controller -run 'TestSecurityAccountDeletionConcurrentRequestsHaveOneWinner|TestTransferAffQuota|TestRegister|TestOAuth' -count=1 -timeout=120s` 通过。该偶发删除并发问题不在本次邀请奖励路径，记录待后续独立处理，不宣称所有完整检查始终稳定。

验证命令：`bun run test src/features/wallet src/features/system-settings/general/__tests__/quota-settings-section.test.tsx`、`bun run typecheck`、`bun run build`、`bun run i18n:sync`，修改 TS/TSX 使用本地 oxlint 与 oxfmt；后端 `GOWORK=off go test ./... -p 1 -count=1 -timeout=300s`、`go build ./...`、`go vet ./...`。三库及浏览器精确命令、版本见各自报告。

已知边界：微信店铺在 Chromium iframe 返回外部 403，支付宝已正常渲染；已验证两店铺新窗口目标及安全隔离，但未进行真实支付。未测最低数据库版本和 ClickHouse。支付条款未确认、未绑定邀请人、邀请人已删除或禁用、自邀请时沿用不发奖励行为。已有但缺失持久邀请关系的历史 OAuth 账号无法可靠推断邀请人，本轮不自动回填。
