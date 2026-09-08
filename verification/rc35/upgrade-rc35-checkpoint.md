# rc.35 兼容升级检查点

## 范围与基线

- 日期：2026-09-08；目标：合并官方 `v1.0.0-rc.35`（`bee45b58a3c0b77e8dc81e6b5aeb4474aa9058d1`）到 fork/main。
- 原始 main：`fe0cd4aa2a47519e74963672b41722ece8438cac`，工作区干净；保留分支 `codex/backup-main-before-rc35`。
- Node v24.12.0、Go go1.26.0 windows/amd64、本地 Bun 1.4.2、Docker 29.7.2；Dockerfile 按上游固定 Bun 1.4.0 / Go 1.26.1。
- 保留 Responses 路径与工具/流式/计费兼容、Sub2 耗时颜色、地址展示复制、管理员倍率覆盖、部署域名/端口/数据卷等定制。
- 用户明确授权：视频采用官方 rc.35；不合并旧功能分支的视频定制。
- 不修改生产服务器、不调用付费上游、不访问用户真实数据库。不升级到 rc.35 之外的上游版本。

## 风险与回滚边界

- 上游包含 GORM/驱动、schema/迁移及任务插件变更，必须在真实 SQLite/MySQL/PostgreSQL 上验证 fresh/upgrade 与重复启动。
- 官方提示旧版升级需重新配置所有视频模型价格；旧视频价格不视为已迁移或正确计费。
- 部署前停止生产写入并备份数据库（包括独立日志库）、配置及原镜像；迁移后回滚不能只切旧二进制，应恢复相配套数据库备份及原镜像。禁止删除数据卷。
- 官方 rc.35 插件系统仍为实验性，不以本地测试替代生产验收。

## 当前阶段

- [x] 拉取并锁定官方 tag，保留 Git 检查点。
- [x] 开始 `git merge --no-commit --no-ff v1.0.0-rc.35`。
- [x] 合并冲突全部解决并核对定制清单（其后新增审查项与回归修复仍继续）。
- [x] 前端 frozen install、91 文件/729 测试、typecheck、构建与浅深色 UI/真实复制冒烟通过；lint 与官方一致为313 errors/77 warnings，定制文件检查通过。
- [x] 根 Go 全量测试、默认 go vet、build 通过；独立 relaykit 在 GOWORK=off 下构建与测试通过。
- [x] 三数据库 fresh/upgrade/重复启动/独立日志库验证：SQLite 3.50.4、MySQL 8.0.46、PostgreSQL 16.15，含旧 main 与 rc34 两条升级路径。
- [x] 最终Docker程序39/39 HTTP验收；最终镜像状态接口与首页启动冒烟通过。
- [x] Docker构建通过，最终镜像哈希核对完成；差异及敏感信息复查通过，命中的固定测试令牌与官方源码一致，无真实凭据。
- [x] 文档与发布准备完成；交付目标main及中文annotated Tag v1.0.0-rc.35.custom.1，提交和推送结果以Git引用核对。

## 协作边界

- Responses/relaykit：后端 Agent。
- web：前端 Agent。
- DB 验收：数据库 Agent（仅新测试/报告，独立容器）。
- HTTP 验收：应用 Agent（仅隔离 runner/报告）。
- 主 Agent：其他差异、定制倍率回归、构建上下文安全、整体集成、版本与交付。

## 阶段证据更新

- 前端最终两轮均为91文件/729测试通过；使用 bun run test --maxWorkers=2，避免默认并发的环境超时。类型检查通过，官方与合并后全量 lint 均为313 errors/77 warnings，无新增；定制文件定向 lint 通过。
- 设置保存：success:false 会拒绝 mutation 并保留草稿，真实 Axios 适配器及 FAQ 重试回归通过。某些旧调用方外层 catch 仍可能重复失败提示，不扩展无关页面重构；成功通知去重保留。
- 根模块 go test ./... -count=1 -timeout=300s、go vet ./... 通过；relaykit 独立构建及测试通过。合并后的类型迁移、推理摘要、工具分片兼容均有回归覆盖。
- 模型、go.mod、go.sum 与官方rc35完全一致；三库18对主/日志结构快照一致。MySQL约束更名与PG软删除部分唯一索引为已验证的官方有意变化。
- 完整应用HTTP已有39/39通过，涵盖36次Responses请求和官方Sora预扣/实际时长差额结算/异步失败退款；最终Docker二进制于18:25—18:27复验39/39通过。
- 最终Docker构建已通过，镜像 forknewapi:rc35-custom-verification，ID sha256:53f2207532824270461a011582bd19866c653557996cc820170f5135bbdd27e3。仅使用临时构建代理与apt重试，不跳过签名/TLS校验。
- 断点恢复：最终Go、729前端测试与Docker构建日志仍在 D:/newapi/.local-tests。Docker界面18:19触发恢复出厂设置（非本任务操作），随后引擎因残留socket启动失败；运行时socket目录已保留备份并重启。Git原代理已恢复，远端main仍为fe0cd4aa2。
- 为遵守仓库不新增docs文件的规则，本次新验收文档统一放在verification/rc35；现有Responses与耗时文档仅更新链接及版本记录。

## 发布交接

1. Docker恢复且最终镜像仍在；精确制品复验和实际镜像启动冒烟完成。
2. 校验项已通过，发布使用合并提交、中文Tag和原子推送；可用 git ls-remote fork refs/heads/main 检查远端交付。

- 设置保存测试归入hooks/__tests__，仅调整相对导入；迁移后15/15通过。
