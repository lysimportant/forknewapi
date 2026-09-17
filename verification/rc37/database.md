# rc.37 数据库兼容验收

本次验收在 2026-09-17 执行，源码工作区为 `D:\newapi\.local-tests\worktrees\upgrade-rc37`。本记录只说明数据库迁移与相关业务回归，不替代主任务的全量测试、认证审查或生产发布验收。

## 源码与隔离边界

- fork 基线：`6e44f3d990aa700958e8f8cf88049639293ba455`。
- 官方基线：`v1.0.0-rc.37`，提交 `385d2dfd10d821b25c8a6766bd16eea248cb1652`。
- 升级目标：fork 基线合入官方 rc.37 后的工作区，保留公告配置事务、邀请奖励冻结及提现账本。
- Go：`go1.26.0 windows/amd64`，构建设置 `GOWORK=off`；未改变 GORM 或数据库驱动依赖。
- SQLite：`3.50.4`，每个场景使用全新文件。
- MySQL：`8.0.46`，复用 `rc35-verification-mysql` 的 loopback 服务；每个场景创建唯一新库。
- PostgreSQL：`16.15`，复用 `rc35-verification-postgres` 的 loopback 服务；每个场景创建唯一新库。

没有读写站点数据库或已有测试库。两个容器的认证信息在脚本进程内读取，未写入源码、报告或命令参数。最终 60 份日志（含源码哈希记录）对实际容器口令的扫描命中为 0。所有测试库和日志保留用于复核，不做自动删除。

## 合并与修复

`model/user.go` 保留 fork 的 `grantInviteRewards`、注册奖励按来源去重和 48 小时冻结流程；同时保留上游其余修改。`model/option.go` 自动合并后仍在同一事务内检查查找、插入与保存错误，成功后才更新内存，并保留上游 Passkey 配置分支。

新增的 options 主键迁移已验证：健康表不重建；无唯一约束的旧表修复后拒绝重复键；原始重复行及空键行完整留在 `options_legacy_*` 备份表；再次迁移不新增备份或改变结构。`model/main.go` 将该迁移错误向上传递并中止启动，避免仅记日志后继续使用修复失败的配置表。三库均验证失败保留原始配置值。

检查 fork 升级路径时发现既有结转缺陷：有冻结奖励但没有 `history:<用户ID>` 记录的用户，在重启时会被再次结转为可立即提现的历史余额。使用 fork 基线和新增测试的只读 Go overlay 复现失败，日志为 `database-20260917174607173-baseline-ledger.log`。

修复后的 `InitializeInviteRewardLedger` 只为没有邀请人账本的历史余额结转；已有记录按剩余额度合计对账，不再用单条历史记录与汇总比较。直接记入钱包的受邀人赠送来源不算邀请人账本。三库回归确认重复初始化不新增奖励，尚在冻结期的提现仍被拒绝。

同一账本还有一个既有分类缺陷：受邀人赠送已直接计入钱包，但其去重记录仍被奖励汇总与提现分配读取，导致可提金额显示为负值，且旧赠送记录到期后可被用于绕过新邀请奖励的冻结期。基线只读 overlay 在 `database-20260917180735830-baseline-ledger.log` 同时复现这两个问题。提现和汇总查询现已排除 `invitee` 来源；去重记录完整保留，重复注册不重复赠送，已入账余额不再被当成另一笔可提现奖励。

## 启动矩阵

最终运行标识为 `20260917180815254`。每个“通过”包含共享日志库、独立日志库两个场景；每个场景均运行两次升级目标的真实 `InitDB()` / `InitLogDB()`，由独立进程执行。

| 数据库来源 | SQLite 3.50.4 | MySQL 8.0.46 | PostgreSQL 16.15 |
| --- | --- | --- | --- |
| 全新空库 | 通过 | 通过 | 通过 |
| 当前 fork `6e44f3d99` 建库并写入代表性数据 | 通过 | 通过 | 通过 |
| 官方 rc.37 建库并写入代表性数据 | 通过 | 通过 | 通过 |

共 18 个场景、36 次升级目标启动，全部通过。第二次启动的表/列/默认值/主键/索引摘要与首次一致，第二次启动 DDL 为 0。代表性数据包含用户余额、历史邀请余额、冻结奖励、部分已提现账本与幂等提现记录、公告及文档配置、Token、渠道 JSON、禁用插件、旧 Passkey、已撤销会话、普通日志和审计日志。旧 Passkey 新增 `rp_id` 后继续保持空值，原凭据字段不变；插件新列验证可完整保存超过 64 KiB 的图标字符串。

除查询索引元数据外，还实际尝试重复插入用户、Token、配置键、奖励来源、提现请求和审计事件，确认数据库拒绝重复写入；验证事务始终回滚。

## 业务回归

同一运行标识下的 `model-regression.log` 有 20 个顶层测试、76 条 PASS，0 FAIL、0 SKIP，涵盖：

- 奖励来源去重、受邀赠送与邀请奖励隔离、48 小时边界、逐笔分配、重复提现、并发提现和重启结转。
- 公告写入成功、失败回滚、不发布内存值和真实连接关闭错误。
- 配置主键修复、原表备份、重复运行和修复失败中止启动。
- GORM 跨库结构稳定、Token/预填分组唯一约束迁移、旧会话刷新摘要字段迁移。

`TestTransferAffQuotaEnforcesFreezeAndIdempotency` 在 SQLite、MySQL、PostgreSQL 的共享及独立日志库下分别运行，六组均通过，无 SKIP。`GOWORK=off go vet ./model/` 与触及文件的 `git diff --check` 通过。

## Controller 三库补充验收

2026-09-17 另以运行标识 `20260917183610830` 补齐新增定价与 Passkey 控制器的真实三库验证。本轮只扩展本地 `database-verify.ps1` 的 `controller` 模式，没有修改生产代码或测试源码，也没有重跑上述 18 个启动组合。脚本新建专用连接库，定价夹具再创建各自的测试库；Passkey 夹具分别新建主库和审计库，不访问已有站点数据库。

| 检查 | SQLite 3.50.4 | MySQL 8.0.46 | PostgreSQL 16.15 |
| --- | --- | --- | --- |
| `TestModelPricingConversionDatabaseMatrix` | 通过 | 通过 | 通过 |
| `^TestPasskey(Domain\|RPID)` | 14 个顶层测试、46 条 PASS | 14 个顶层测试、46 条 PASS | 14 个顶层测试、46 条 PASS |
| 旧凭据 RP ID 迁移 | 全新与 rc.36 旧结构各通过 | 全新与 rc.36 旧结构各通过 | 全新与 rc.36 旧结构各通过 |

定价矩阵共 154 条 PASS（包括父测试和三库分组），没有 FAIL 或 SKIP。`gpt-4o-2024-05-13` 的管理员 `CompletionRatio=99` 转换为 `tier("base", p * 4 + c * 396)`，显式 `0` 转换为 `tier("base", p * 4 + c * 0)`；两个用例均在三库验证预览不写库、保存和回读一致、旧 `expected_version` 被拒绝，以及回切旧倍率模式保留配置。其余用例覆盖缓存、图片/音频价格、别名及前端转换契约。

Passkey 三组共执行 42 个顶层测试、138 条 PASS，没有 FAIL 或 SKIP。`TestPasskeyRPIDMigrationPreservesExistingCredentials` 在每库分别运行 `upgrade=false/true`，两次迁移后旧凭据字段、唯一约束和索引保持，随后实际完成旧 RP ID 的签名登录。域名专项覆盖直接登录、登录二次验证、敏感操作验证、重复断言拒绝、错误 RP ID/Origin/签名/用户、缺少用户验证、过期挑战、登录协议版本、移除域名的确认与审计、RP ID 轮换失败回滚及进行中的注册。失败日志不含凭据的既有回归也在每库执行。

这四次命令全部退出 0；所记录的 10 个控制器/模型源文件哈希在运行前后相同。六份新增证据文件对实际容器口令扫描命中为 0。此结果补足真实数据库执行证据，不代表真实浏览器硬件 Passkey 或全站认证合规认证。

## 命令与证据

脚本和原始证据位于本地忽略目录 `D:\newapi\.local-tests\rc37-acceptance`。脚本调用仓库实际 Go 模型，不用 mock 替代数据库。可复核入口：

```powershell
& D:\newapi\.local-tests\rc37-acceptance\database-verify.ps1 -Mode all
& D:\newapi\.local-tests\rc37-acceptance\database-verify.ps1 -Mode controller
```

`-Mode matrix` 只运行启动矩阵，`-Mode tests` 只运行原有模型与提现回归，`-Mode controller` 只运行上述定价与 Passkey 三库专项；`all` 包含三类检查。脚本固定独立 fork 基线工作区 `.local-tests\rc37-acceptance\fork-baseline`；官方基线工作区为 `.local-tests\next-acceptance\backend-upstream-rc37`。

核心测试命令：

```text
GOWORK=off go test ./model/ -run 'TestInviteReward|TestInitializeInviteRewardLedger|TestUpdateOptionPersists|TestOptionPrimaryKey|TestMigrationSchemaStability|TestUserSessionPreviousRefreshHashMigration|TestMigrateTokenKeyUniqueness|TestMigratePrefillGroupUniqueness' -count=1 -v
GOWORK=off go test ./controller/ -run '^TestTransferAffQuotaEnforcesFreezeAndIdempotency$' -count=1 -v
GOWORK=off go test ./controller/ -run '^TestModelPricingConversionDatabaseMatrix$' -count=1 -v
GOWORK=off go test ./controller/ -run '^TestPasskey(Domain|RPID)' -count=1 -v
GOWORK=off go vet ./model/
```

脚本为上述测试注入独立 `TEST_MYSQL_DSN`、`TEST_POSTGRES_DSN`；提现回归另设 `TEST_MANAGE_USER_DIALECT` 和 `TEST_MANAGE_USER_SEPARATE_LOG_DB`，Passkey 每库分别设置 `TEST_SECURITY_DIALECT=sqlite/mysql/postgres`。启动矩阵使用 `SQL_DSN`、`LOG_SQL_DSN` 和独立 SQLite 路径。

最终证据文件：

- `database-20260917180815254-summary.log`：18 个场景及回归结论。
- `database-20260917180815254-{fresh,fork,upstream}-{sqlite,mysql,postgres}-{shared,separate}-start-{1,2}.log`：实际迁移与数据验证输出。
- `database-20260917180815254-{fork,upstream}-*-seed.log`：对应基线真实建库及代表性数据写入。
- `database-20260917180815254-model-regression.log`：20 个模型顶层测试。
- `database-20260917180815254-withdraw-{sqlite,mysql,postgres}-{0,1}.log`：六组接口回归。
- `database-20260917180815254-final-source-hashes.log`：最终模型生产文件与回归测试的 SHA-256。
- `database-20260917183610830-summary.log`、`database-20260917183610830-controller-source-hashes.log`：控制器补充验证结果与十个源文件的 SHA-256。
- `database-20260917183610830-controller-pricing.log`：三库定价转换，包含管理员 99/0 覆盖。
- `database-20260917183610830-controller-passkey-{sqlite,mysql,postgres}.log`：三库旧凭据迁移与域名验证专项。

## 部署与回滚边界

本轮不操作生产。正式升级前应停止写入并备份主库及独立日志库，保留升级前代码和配置。上游新增字段不删除旧数据；options 修复会重建配置表并保留原始备份。MySQL DDL 不保证随业务事务整体回滚，启动失败后应保留现场、修复根因或恢复升级前备份，不应反复盲目启动。

代码回退可以恢复旧版本行为，但不能替代数据库恢复，也不能回退期间已发生的钱包交易。本次没有自动删除既有账本记录；若历史实例曾产生重复结转，需依据实际账本和提现记录另行对账，不能把本次防重复修复当作历史数据纠正。

本轮验证了所列真实数据库版本，未测试 MySQL 5.7.8 / PostgreSQL 9.6 最低版本或 ClickHouse；本次修复未引入依赖最低版本特性的 SQL。生产数据规模、外部数据库代理和认证端到端验收仍以主任务及部署检查为准。
