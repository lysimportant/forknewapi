# rc35 真实数据库迁移验收

## 结论与范围

验收日期：2026-09-08（Asia/Shanghai）。**本次模型层真实数据库迁移矩阵通过**：SQLite、MySQL、PostgreSQL 均覆盖 fresh、原始 main 升级和官方 rc34 升级；每条路径均使用独立日志数据库，目标版本至少执行两次独立进程的启动迁移。

这不是仅运行 AutoMigrate 单测的结论。隔离 Go runner 导入实际版本的 `common` / `model`，依次执行生产入口 `model.InitDB()`、`model.InitLogDB()`，设置 `common.IsMasterNode=true`，执行完整主库和日志库迁移。没有启动 HTTP 服务、任务调度器或任何真实上游调用；因此不代表完整应用启动、HTTP/E2E、支付结算或生产部署验收。

工作区：`D:\newapi\.local-tests\worktrees\responses-main`。验收时正在合并官方 rc35，Responses 等文件由其他 Agent 处理。本子任务不修改任何已有生产文件，不提交、不推送。唯一可提交产物是本文；全部 runner、源码快照、可执行文件、SQLite 文件和原始日志位于已忽略的 `.local-tests/rc35-db/`。

## 源码与依赖证据

- 原始 main：`fe0cd4aa2`，使用 `git archive` 提取独立源码目录，在该目录编译，未用目标版本的模型伪造旧库。
- 官方前版 rc34：`0c76e4dae77a279e015329b7478e6f02d6b62edd`，通过 `git ls-remote origin refs/tags/v1.0.0-rc.34` 和 `git fetch origin refs/tags/v1.0.0-rc.34` 获取，然后 `git archive FETCH_HEAD` 提取独立源码并编译。
- 目标：合并工作区；官方 `v1.0.0-rc.35` 的本地 tag 对象为 `bee45b58a3c0b77e8dc81e6b5aeb4474aa9058d1`。验收后 `git diff --stat v1.0.0-rc.35 -- model go.mod go.sum` 无输出。
- 目标 `model/main.go` SHA256：`BE6390BCD499FF0EE1DB442726A11BA24E73D684F776A083B1ADC9B76CAF0716`。
- 目标 `go.mod` SHA256：`6015AEB99B863F2EC2285954DC89B92140077017DFA33563CDE2E027A9AEE051`。
- 目标 `go.sum` SHA256：`2420A4A300026A1DDE1D962F23ED34FA7C22CF1C34A4A596106A37C8AC439055`。

| 依赖 | 原始 main 二进制 | 目标二进制 |
| --- | --- | --- |
| GORM | 1.25.2 | 1.25.12 |
| MySQL driver | 1.4.3 | 1.5.7 |
| PostgreSQL driver | 1.5.2 | 1.5.9 |
| glebarez/sqlite | 1.9.0 | 1.11.0 |
| modernc.org/sqlite | 1.40.1 | 1.40.1 |

使用 `go version -m` 核验实际二进制依赖。读取下载到 Go module cache 的官方驱动 `go.mod`：MySQL 1.5.7 要求 GORM 1.25.7；PostgreSQL 1.5.9 要求 GORM 1.25.10；glebarez/sqlite 1.11.0 要求 GORM 1.25.7。目标采用官方一致的版本组合，且以下矩阵已实际执行；并非仅凭版本要求推断兼容。

## 环境与隔离

- Go：`go1.26.0 windows/amd64`，构建设置 `GOWORK=off`，不依赖工作区 go.work；源码声明 Go 1.25.1。
- Docker Server：`29.7.2`。
- SQLite：运行实际 SQL `SELECT sqlite_version()` 返回 `3.50.4`，使用磁盘文件而不是 mock / 内存数据库。
- MySQL：`8.0.46`；镜像 `mysql:8.0`，image ID `sha256:7dcddc01f13bab2f15cde676d44d01f61fc9f99fe7785e86196dfc07d358ae2b`。
- PostgreSQL：`16.15`，x86_64 Linux musl / Alpine；镜像 `postgres:16-alpine`，image ID `sha256:cf78e76683b9ca8c5733cbbdce6c9262b45b6767934dd0a95e671f9a0fc20685`。
- 专用容器：`rc35-verification-mysql`、`rc35-verification-postgres`，仅绑定本机回环端口 `33358`、`33356`。
- 每个服务内分配 `rc35_fresh`、`rc35_upgrade`、`rc35_rc34` 及各自 `_log` 独立数据库。SQLite 使用同名 `.sqlite` 和 `_log.sqlite` 文件。
- MySQL 使用 utf8mb4；容器只用于临时本地验收，采用空密码 / PostgreSQL trust 初始化，无实际密码或令牌，不应照搬至生产。
- 原有 `multimodal-canvas-*` 等其他项目容器未执行停止、删除或修改。

## 矩阵及实际结果

| 路径 | SQLite 3.50.4 | MySQL 8.0.46 | PostgreSQL 16.15 |
| --- | --- | --- | --- |
| 空库 → 目标迁移、写代表数据、重启 | PASS | PASS | PASS |
| fe0cd4aa2 建库及写数据、旧版本再次启动 → 目标两次启动 | PASS | PASS | PASS |
| 官方 rc34 建库及写数据、旧版本再次启动 → 目标两次启动 | PASS | PASS | PASS |
| 各路径独立 log DB 迁移、数据隔离 | PASS | PASS | PASS |
| 各路径目标两次启动 schema 相同 | PASS | PASS | PASS |
| 额外充值单 / 订阅单 / 2FA / 活跃预填组唯一性 | PASS | PASS | PASS |

目标进程每次输出 `RC35_VERIFY_PASS` 并返回 0；额外唯一性检查输出 `RC35_EXTRA_UNIQUE_PASS`。日志里的 duplicate key / UNIQUE constraint 错误是主动插入重复值并断言失败的预期结果，随后事务回滚，不是启动迁移失败。

### 代表数据检查

旧源码真实创建并写入用户、渠道、token、task、log，然后由新源码读取检查：

- 用户中文显示名、额度 `5000000000`（超过 int32）、已用额度 `12345` 保持不变。
- 渠道名称、虚构本地标记 key、模型名保持不变；无可调用上游凭据。
- token 112 字符 key、600 字符 model_limits、余额 `54321` 保持不变。
- task 的 SUCCESS 状态、额度 `123`、JSON 中文字段和显式数值零保持不变。
- 独立日志库的中文内容、额度和模型名保持不变；主库同标记日志数量必须为零。
- 每次迁移后实际插入重复用户名、重复 token key，数据库必须拒绝；不是只检查 ORM 标签。
- 额外在回滚事务内验证重复充值交易号、订阅交易号、2FA 用户 ID、活跃预填组名称均被拒绝，九条目标路径全部通过。

### Schema、索引、约束与幂等性

每次目标迁移之后，分别读取主库和日志库实际元数据：SQLite sqlite_master 的表/索引 DDL；MySQL information_schema 的列、完整索引明细和约束；PostgreSQL 的列、pg_indexes 和 pg_constraint 定义。排除自增序列当前值，避免主动失败插入所消耗的 ID 影响 schema 对比。

- 三条来源 × 三引擎 × 两个库，共 **18 对 schema JSON 快照逐字节一致**。
- 九份 `*-new2.log` 中实际执行的 `CREATE`、`ALTER`、`DROP` 数量均为 **0**。
- 同时验证用户名、token key、task_id、日志时间/ID 等代表索引存在。
- 对比旧库所有索引/约束元数据，没有未解释的丢失；不是要求所有旧索引名称永远不变。下列差异已经单独核实：
  1. **旧 main → MySQL**：四个唯一约束及其背后的索引由 `username`、`trade_no`、`user_id` 等旧名字改为 `uni_users_username`、`uni_subscription_orders_trade_no`、`uni_top_ups_trade_no`、`uni_two_fas_user_id`。共 8 条元数据差异；目标列、唯一性、索引列顺序未变，且真实重复插入均被拒绝。
  2. **旧 main → PostgreSQL**：旧 `idx_prefill_groups_name` 全局唯一约束/索引被官方 `migratePrefillGroupUniqueness` 替换为 `uk_prefill_name` 部分唯一索引，条件是 `deleted_at IS NULL`。这是明确的软删除行为修正，不能说全局唯一约束原样保留；活跃名称唯一性已检查，官方迁移回归测试也通过。
  3. rc34 → 目标：三个引擎的旧索引/约束明细均原样保留；新增对象不视为旧对象丢失。

完整差异及受限例外在 `schema-comparison.log`，比较程序 `compare.cjs` 返回 0；程序只接受上述确切命名/定义变更，不忽略任意索引丢失。

## 可复现命令与本地证据

以下命令在合并工作区执行；脚本位于忽略目录。初始化只执行一次，已经存在的容器、库或数据不得盲目重建。

```powershell
# 专用容器；不引用任何其他项目容器。
docker run -d --name rc35-verification-mysql -e MYSQL_ALLOW_EMPTY_PASSWORD=yes -p 127.0.0.1:33358:3306 mysql:8.0 --character-set-server=utf8mb4 --collation-server=utf8mb4_unicode_ci
docker run -d --name rc35-verification-postgres -e POSTGRES_HOST_AUTH_METHOD=trust -p 127.0.0.1:33356:5432 postgres:16-alpine
# 等待 SELECT VERSION()/SELECT version() 成功，再创建前述六个数据库。

$env:GOWORK='off'
go build -o .local-tests/rc35-db/merged.exe ./.local-tests/rc35-db/main.go
# 旧版必须分别在各自 git archive 源码目录内构建，而不是从当前模块引用旧 runner 文件。
Push-Location .local-tests/rc35-db/old-main
go build -o ../old-main-verified.exe ./rc35_runner.go
Pop-Location
Push-Location .local-tests/rc35-db/rc34
go build -o ../rc34.exe ./rc35_runner.go
Pop-Location

$r='./.local-tests/rc35-db/run.ps1'
# Seed 仅用于空库；已有数据时不可重复 Seed。
& $r -Scenario fresh -Binary merged.exe -Seed 1 -Label seed
& $r -Scenario fresh -Binary merged.exe -Label restart2
& $r -Scenario upgrade -Binary old-main-verified.exe -Seed 1 -Label old-seed
& $r -Scenario upgrade -Binary old-main-verified.exe -Label old-restart2
& $r -Scenario upgrade -Binary old-main-verified.exe -Label old-snapshot
& $r -Scenario rc34 -Binary rc34.exe -Seed 1 -Label old-seed
& $r -Scenario rc34 -Binary rc34.exe -Label old-restart2
foreach($scenario in @('fresh','upgrade','rc34')) {
  & $r -Scenario $scenario -Label new1
  & $r -Scenario $scenario -Label new2
  & $r -Scenario $scenario -Label extra-uniqueness
}
node .local-tests/rc35-db/compare.cjs
```

脚本通过临时子进程环境传入 `SQL_DSN` / `LOG_SQL_DSN`；SQLite 在调用两个入口之间切换 `common.SQLitePath`，确保是两个物理文件。应用当前 SQLite DSN 分支共享该全局路径，因此这是入口级独立日志迁移测试，不是声称仅靠部署环境变量就能配置两个 SQLite 文件。

补充运行仓库已有针对性回归测试，MySQL/PostgreSQL DSN 均指向以上隔离容器；所有引擎执行，没有 SKIP：

```powershell
$env:TEST_MYSQL_DSN='root@tcp(127.0.0.1:33358)/rc35_fresh?charset=utf8mb4&parseTime=true'
$env:TEST_POSTGRES_DSN='postgres://postgres@127.0.0.1:33356/rc35_fresh?sslmode=disable'
go test ./model -run 'TestMigrationSchemaStability$' -count=1 -v
# PASS，包测试 2.454s：身份/索引、唯一约束变化、MySQL decimal/default 迁移。
go test ./model -run 'TestMigrate(TokenKey|PrefillGroup)Uniqueness' -count=1 -v
# PASS，包测试 3.087s：含旧约束迁移、未知约束拒绝、非冲突索引保留等。
```

证据目录 `.local-tests/rc35-db/`：

- `main.go`、`run.ps1`、`compare.cjs`：隔离验收程序、执行器、schema 比较器。
- `old-main/`、`rc34/`：旧源码快照及仅存在于快照中的 runner。
- `*-old-seed.log`、`*-old-restart2.log`、`*-new1.log`、`*-new2.log`、`*-extra-uniqueness.log`：执行日志。
- `*-schema-main.json`、`*-schema-log.json`、`schema-comparison.log`：结构和差异证据。
- `migration-schema-tests.log`、`uniqueness-migration-tests.log`：仓库回归测试结果。

## 限制、风险及交接

1. 未测试 MySQL 5.7.8 / PostgreSQL 9.6 最低版本；本轮覆盖各一种受支持真实版本，不声称所有版本全覆盖。未测试事务池代理、复制拓扑、ClickHouse、跨引擎主库/日志库组合。
2. 使用合成代表数据，不含生产全量历史数据、未知自定义约束或特殊损坏数据。PostgreSQL 预填组唯一性范围发生上述有意改变；真实发布前仍需按部署环境备份主库和独立日志库。
3. 这里只验证生产模型迁移入口和针对性测试；完整后端 build、lint、全量测试、应用/前端冒烟由主任务负责。尚未完成的合并不能凭本文标记为整体验收完成。
4. 未做原地降级验收。回滚生产升级应恢复升级前主库与日志库一致性备份，而不是直接用旧二进制连接已升级数据库。隔离库可由旧源码快照重放，未触及任何生产数据库。
5. 两个 rc35 专用容器与隔离数据保留供主任务复核；如需清理，必须仅针对本文列出的两个名字及对应测试文件，不执行全局 prune，不操作其他项目容器。
6. 本轮无生产文件变更、无提交、无推送；文档不能代替后续源码变动后的重新验收。若主任务继续修改模型、数据库依赖或迁移入口，需要重新构建目标 runner 并复测。
