# 钱包充值返佣数据库验收

本轮只使用隔离测试数据库，不读写站点业务库。源码基线为 `main@e47a383a884f849381ce99effac317ae04788b01`，Tag 为 `v1.0.0-rc.37.custom.1`；运行时为 Go `1.26.0 windows/amd64`，使用 `GOWORK=off`，未安装或升级依赖。

## 环境与基线

- SQLite `3.50.4`：仓库现有驱动直接访问临时磁盘数据库。
- MySQL `8.0.46`：既有专用容器 `rc35-verification-mysql`，监听 `127.0.0.1:33358`。
- PostgreSQL `16.15`：既有专用容器 `rc35-verification-postgres`，监听 `127.0.0.1:33356`。
- 每轮创建全新 `wallet_referral_<运行标识>_*` 数据库；名称碰撞立即失败，不使用 `IF NOT EXISTS`，不清理既有测试库。控制器夹具再创建独立的主库与日志库。
- 认证配置只在脚本进程内读取，命令输出脱敏后保存，不把 DSN 或口令写入源码及报告。

2026-09-18 修改前执行：

```text
GOWORK=off go test ./model/ -run 'TestInviteReward|TestInitializeInviteRewardLedger' -count=1 -v
```

8 个顶层测试、29 条 PASS、0 FAIL、0 SKIP。三库逐一覆盖来源去重、48 小时边界、逐笔提现、幂等与并发提现、历史账本初始化、重复迁移。证据：`.local-tests/wallet-referral-acceptance/20260918095533552-baseline-model.log`，执行时源码哈希另存同前缀 `baseline-source-hashes.json`。本次旧行为基线不代表新返佣需求已经通过。

## 最终验收

2026-09-18 最终运行标识为 `20260918101852085`。本地独立执行器 `.local-tests/wallet-referral-acceptance/database-verify.ps1` 的目标固定当前 `D:\newapi`。旧版验收脚本固定另一工作区，未直接复用其运行入口。

较早一轮 `20260918100807104` 也完成同规模模型、提现与启动检查。后续只读审查发现注册收尾先锁邀请人、充值先锁受邀人的顺序相反；本次修复为两者均先锁受邀人再锁邀请人，避免首次充值与注册收尾相互等待。新增四并发回归后重跑全部矩阵，启动与完整专项以 `20260918101852085` 为准；随后非零赠送夹具补充专项见下文，较早证据保留供复核。

最终执行入口：

```powershell
& D:\newapi\.local-tests\wallet-referral-acceptance\database-verify.ps1 -Mode all
```

命令退出 `0`，模型专项 26 个顶层测试、121 条 PASS、0 FAIL、0 SKIP。以下邀请业务用例分别在三库执行：

- 普通注册与 OAuth 注册均持久化直接邀请人；注册只计邀请人数，受邀人赠送保留，不再发放固定邀请人奖励。
- 易支付、Stripe、Creem、Waffo、Waffo Pancake、人工补单、兑换码共七条实际充值路径按到账额度返还 10%，最小整数额度向下取整；同一来源和重复回调不重复入账，充值不增加邀请人数。
- 新返佣固定冻结 172800 秒，即使旧冻结配置改为 `0` 仍不可提前提取；到期前一秒失败，恰好到期成功。
- 未绑定、已删除、禁用、自邀请、未确认支付条款、返佣不足一个最小额度等条件不产生返佣。
- 邀请人剩余或累计返佣达到 `MaxWalletQuota` 时，订单状态、兑换码状态、用户充值与返佣同事务回滚；真实 MySQL/PostgreSQL 的 `quota`、`aff_quota`、`aff_history` 列均实测为 `bigint`。
- 同一受邀人多笔充值分别返佣；每笔同时运行两个充值回调、两个注册收尾，四个请求均成功，赠送、邀请人数及返佣不重复、不丢失。51 笔奖励的冻结汇总覆盖明细页以外记录，重复初始化保持冻结及历史账本。
- 原有来源去重、逐笔分配、并发提现、48 小时边界、历史结转与重复迁移保持。

现有易支付余额/缓存、兑换码重放/并发/空白匹配/上限测试作为 SQLite 补充执行；其通过不计作额外三库证明。

三库分别以共享日志库、独立日志库运行 `TestTransferAffQuotaEnforcesFreezeAndIdempotency`，共六组全部通过，验证缺少幂等标识、冻结期拒绝、正常入账、重放只记一次及日志落库位置。

## 启动与历史数据保持

`-Mode startup` 对空库和当前已发布版本建立的代表性数据库各运行两次真实 `InitDB()` / `InitLogDB()`。发布基线探针从干净的 `D:\newapi\.local-tests\worktrees\upgrade-rc37`（`e47a383a8`）编译，源程序沿用 `.local-tests/rc37-acceptance/database-probe.go`。

| 来源 | SQLite 3.50.4 | MySQL 8.0.46 | PostgreSQL 16.15 |
| --- | --- | --- | --- |
| 全新空库 | 共享及独立日志库通过 | 共享及独立日志库通过 | 共享及独立日志库通过 |
| 已发布 `v1.0.0-rc.37.custom.1` 数据库 | 共享及独立日志库通过 | 共享及独立日志库通过 | 共享及独立日志库通过 |

共 12 个场景、24 次目标启动通过。每个场景第二次启动 DDL 为 `0`，表、列、默认值、主键和索引摘要与第一次一致。用户余额、历史邀请余额、冻结奖励、已部分提现的奖励与幂等提现记录、配置、Token、渠道、插件、旧 Passkey、撤销会话和日志保持；实际重复插入验证奖励来源及提现幂等键等唯一约束仍生效。

本次不新增表、列或迁移，不对历史充值补发，不重写旧奖励。新返佣的到期时间存入既有 `available_at`，与原账本和提现读取兼容。

## 命令与证据

```text
GOWORK=off go test ./model/ -run 'TestInviteReward|TestInitializeInviteRewardLedger|TestRechargeEpay|TestRedemptionQuota|TestRedeem|TestRechargeWaffoPancake' -count=1 -v
GOWORK=off go test ./controller/ -run '^TestTransferAffQuotaEnforcesFreezeAndIdempotency$' -count=1 -v
```

脚本只在测试进程中注入 `TEST_MYSQL_DSN`、`TEST_POSTGRES_DSN`；提现专项分别设置 `TEST_MANAGE_USER_DIALECT=sqlite/mysql/postgres` 与 `TEST_MANAGE_USER_SEPARATE_LOG_DB=0/1`。启动探针通过 `SQL_DSN`、`LOG_SQL_DSN` 和独立 SQLite 路径连接本轮新库。

证据目录为 `D:\newapi\.local-tests\wallet-referral-acceptance`：

- `20260918101852085-summary.log`：完整场景结论，末尾 `FINISHED`。
- `20260918101852085-model-regression.log`：模型专项 121 条 PASS。
- `20260918101852085-withdraw-{sqlite,mysql,postgres}-{0,1}.log`：六组接口结果与精确数据库版本。
- `20260918101852085-{fresh,released}-{sqlite,mysql,postgres}-{shared,separate}-start-{1,2}.log`：24 次实际启动与数据保持证据。
- `20260918101852085-released-*-seed.log`：已发布版本创建代表性数据的六组证据。
- `20260918101852085-source-hashes.json`：`model/user.go`、`model/invite_reward.go`、`model/topup.go`、`model/redemption.go` 和 `model/invite_reward_test.go` 的 SHA-256；执行前后完全一致。

本轮 40 份日志/哈希证据的连接凭据、常见令牌及私钥模式检查命中为 `0`；测试夹具只含不可用于真实认证的固定示例。执行器未修改生产代码，未提交临时探针、数据库文件和日志。

## 非零注册赠送并发补充

完整矩阵后，仅修改 `TestInviteRewardMultipleTopUpsAndConcurrentReplay` 的夹具，显式设置 `QuotaForInvitee=100`，并断言最终余额精确为 `15000100`，确认非零赠送更新与首次充值并发时仍不重复或丢失。

2026-09-18 运行标识 `20260918102617671`，在新建隔离库上执行：

```text
GOWORK=off go test ./model/ -run '^TestInviteRewardMultipleTopUpsAndConcurrentReplay$' -count=1 -v
```

SQLite、MySQL、PostgreSQL 全部通过：1 个顶层测试、4 条 PASS、0 FAIL、0 SKIP，退出 `0`。两个充值回调与两个注册收尾同时运行，赠送仅入账一次，邀请人数为 `1`，两单返佣合计 `1500000`。此补充不重跑启动矩阵。

证据为 `20260918102617671-concurrency-model.log` 与 `20260918102617671-concurrency-source-hashes.json`。运行前后哈希一致；与完整矩阵相比，四个生产文件哈希全部不变，只有测试夹具文件改变，其最终 SHA-256 为 `01EF5852CE1DC5B31C96E1508DD11CB8F429D5F28DE0C6B9DB47278859F3CFF9`。两份新增证据的凭据模式检查命中为 `0`。

## 限制与回退

本报告验证所列真实数据库版本，未运行 MySQL 5.7.8、PostgreSQL 9.6 或 ClickHouse。本次未采用依赖最低版本特性的 SQL；三库通过不代表所有部署规模和数据库代理组合均已测试。报告不替代前端、完整 Go 检查或生产支付回调验收。

生产部署前保留程序与配置并备份主库和独立日志库。代码回退不会撤销已完成交易；新返佣仍属于现有邀请账本，旧版本会按显式到期时间处理。恢复旧注册固定奖励前应核对旧配置；任何历史资金纠正需要独立对账，不通过删除账本实现。
