# Canvas 本地与服务器双实例检查点

日期：2026-10-05。P0 认证边界变更，代码交付与线上部署分别验收。

## 基线与目标

- New API：`D:/newapi`，`main`，上游 `fork/main`，基线 `267b473d9b3e4bf98a656c9933f8fb2b0c3b86ed`；开始时工作区干净。交付远端为 `https://github.com/lysimportant/forknewapi.git`，不推上游 `origin`。
- Go `1.26.0 windows/amd64`；根模块声明 Go `1.25.1`，依赖已存在，未修改依赖或锁文件。已有 `web/dist` 可供根模块嵌入；本任务不改前端。
- Canvas：`G:/multimodal-canvas`，`main @ de523462e4009fee39baa235570ad1a5512e576f`，Node `24.12.0`、pnpm `11.19.0`。现有 UI、品牌和节点改动属于其他任务，未纳入本任务提交。
- 用户确认 New API 目录为 `/opt/forknewapi`，画布目录为 `/opt/multimodal-canvas`，均使用 Docker Compose；服务名与生产原有参数仍需在部署时核对。
- 目标：保留服务器授权，允许本地独立登录；同账号的 grant、分组 Token、重新登录和撤销互相隔离。回调仍逐字匹配，保留 S256 PKCE、会话绑定、一次性与过期校验。
- 不涉及数据库模型、迁移、计费、Provider、依赖升级或画布业务源码；不部署生产、不清理用户数据、不调用付费生成接口。

## 两组固定配置

| 配置 | 服务器 | 本地 Docker |
| --- | --- | --- |
| issuer | `https://api.lolicon.beer` | `https://api.lolicon.beer` |
| client_id | `canvas` | `canvas` |
| instance_id | `main` | `canvas-local` |
| redirect_uri | `https://love.lolicon.beer/v1/auth/newapi/callback` | `http://localhost:8080/v1/auth/newapi/callback` |

主实例 secret 沿用现有部署，不清空或替换；本地当前未配置 secret，追加项采用公开 PKCE。`localhost` 与 `127.0.0.1` 不可互换。

New API 使用 [Compose 覆盖文件](canvas-multi-client.compose.yml)。将其作为原 Compose 文件之后的最后一个 `-f` 参数；它只设置这两组账号配置，不覆盖原 secret、镜像、数据库连接、卷或网络。若原来已有其他附加实例，应合并 JSON 数组，不能直接覆盖。若实际服务名不是 `new-api`，先按原服务名调整此文件。

在 `/opt/forknewapi` 执行，保留生产原有 `--env-file`、`-p` 和其他覆盖文件参数；先确认当前分支为 `main`、上游指向用户的 `lysimportant/forknewapi` 仓库：

```sh
cd /opt/forknewapi
git pull --ff-only
docker compose -f docker-compose.yml -f verification/canvas-multi-client.compose.yml config --quiet
docker compose -f docker-compose.yml -f verification/canvas-multi-client.compose.yml build new-api
docker compose -f docker-compose.yml -f verification/canvas-multi-client.compose.yml up -d --no-deps --wait new-api
```

文件名和 `new-api` 服务名来自当前仓库，执行前必须和服务器配置核对。不能在 `/opt/multimodal-canvas` 中直接执行这组网关命令；只更新画布镜像无法启用 New API 新增配置。生产部署需单独确认并先保存原镜像 ID、私有配置和数据库备份。只做 `restart` 不会加载修改后的容器环境。后续每次启动或重建都保留这份覆盖文件，避免退回单实例配置。

本地 Canvas 的非敏感持久选项保存在被 Git 忽略的 `.env.compose`，沿用现有运行选项，仅将 `MC_NEW_API_INSTANCE_ID` 设为 `canvas-local`。New API 部署成功后，在 `G:/multimodal-canvas` 应用：

```powershell
docker compose --env-file .env.compose -f compose.yaml -p multimodal-canvas-app config --quiet
docker compose --env-file .env.compose -f compose.yaml -p multimodal-canvas-app up -d --no-deps --no-build --pull never --wait api worker
docker compose --env-file .env.compose -f compose.yaml -p multimodal-canvas-app ps -a
```

先核对 API/Worker 镜像与现有运行容器一致，确认无在途任务再切换；只重新创建 API/Worker，不运行迁移或初始化、不改卷。已有本地 `main` 授权不会自动转换为 `canvas-local`，需重新登录；已有项目和身份不可擅自迁移。此配置文件必须显式通过 `--env-file` 选择，通用 `Docker-Start.cmd` 不会自动加载它。

服务器画布保持 `MC_NEW_API_CLIENT_ID=canvas`、`MC_NEW_API_INSTANCE_ID=main` 与服务器 HTTPS 回调，无需为本功能重建画布源码。

## 实现与安全复核

- `CANVAS_ACCOUNT_ADDITIONAL_CLIENTS` 是严格 JSON 数组；允许最多 32 个附加项、64 KiB 总长度。拒绝未知/重复字段、缺少字段、重复身份组合、不安全 URL；配置错误整体拒绝服务，不跳过坏项。
- 授权批准从数据库保存的 flow 身份选配置；请求只携带原有 opaque request token，不能换绑客户端、实例或回调。code 兑换继续绑定完整身份与 PKCE。
- grant 读取、同步、轮换及创建类执行均复核当前允许列表；移除实例立即封锁这些入口。撤销保持幂等，移除后仍可用原 Bearer 撤销；重新加入配置不会复活已撤销 grant。仅移除配置不等于永久撤销。
- 保留旧单客户端配置及其规范化行为，不调整主实例 secret；附加 secret 按原始值比较，错误不回显内容。普通非 Canvas API Token 合同保持。
- 参考 OWASP ASVS 稳定版 5.0.0 的认证、会话与 OAuth 相关要求，阅读 [Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)、[Session Management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html) 和 [OAuth 2.0](https://cheatsheetseries.owasp.org/cheatsheets/OAuth2_Cheat_Sheet.html)；本次验证上述受影响控制，不声明整站 ASVS 认证。

## 本轮验证

- 修改前：`go test -mod=readonly ./common ./controller ./middleware -run 'TestCanvas' -count=1 -timeout=180s` 通过。
- 修改后专项：`go test -mod=readonly ./common ./controller ./middleware ./router -run 'TestCanvas' -count=1 -timeout=180s` 通过。覆盖双实例登录、重登与撤销隔离、secret/回调/pair 混用、保存流程绑定、PKCE、重放、过期、配置删除和执行阻断。
- 根模块：`go test -mod=readonly ./... -count=1 -timeout=300s`、`go vet -mod=readonly ./...`、`go build -mod=readonly ./...` 均通过，退出 0；44 个测试包通过，38 个包无测试。
- `relaykit/` 独立模块设置 `GOWORK=off` 后同样执行上述 test、vet、build，均通过；9 个测试包通过，8 个包无测试。未修改前端，不重新安装前端依赖；Go build 使用既有嵌入资源，未执行完整生产 Docker 镜像构建。
- SQLite `3.50.4`；MySQL `8.0.46`；PostgreSQL `16.15`。后两者在无卷、仅回环可达的临时容器上执行 `./controller -run '^TestCanvasAccount' -count=1 -timeout=180s`，均通过；测试创建隔离主库和审计库，结束已删除临时容器。
- MySQL 旧回归中裸 `key` 保留字查询触发 1064，已改为 GORM 字段 map，由方言引用列名；修复后完整 Canvas 8 个顶层用例通过。不涉及业务数据库结构调整。
- 多实例 controller 用例合并到现有测试文件后，重新执行 `./controller -run '^TestCanvasAccount' -count=1 -timeout=180s` 通过；仅新增一份 common 配置测试文件，避免分散同一功能用例。
- New API Compose 解析通过，两组身份与回调精确匹配，使用合成占位值验证主实例 secret 继承；本地 `.env.compose` 解析通过，API/Worker 均得到 `canvas-local`，与现有 API 配置相比仅 `NEW_API_INSTANCE_ID` 改变。未输出真实 secret 或管理员身份。
- 终审未发现新问题，`gofmt` 和 `git diff --check` 通过。测试日志保存在被 Git 忽略的 `.local-tests/canvas-multi-client-20261005/`。

## 恢复与验收边界

代码及隔离验证完成；当前线上 New API 尚未部署。本地 `.env.compose` 已写入并验证，现有容器仍使用 `main`；待网关更新后按上文顺序应用，不能据单测声称真实登录已经恢复。

待线上部署后分别从两个入口完成浏览器登录和分组同步，确认本地退出不影响服务器会话；不以生成内容作为登录冒烟，避免额外费用。生产 TLS、现网登录和服务器配置尚未实测。

回滚时先停止本地新提交并处理在途任务，恢复部署前镜像和主实例配置、移除本次附加项；保留数据库、卷、授权和管理 Token 记录，不恢复旧 Key 或清空用户数据。没有新增 schema，功能回滚不要求反向迁移；如需永久使本地授权失效，先显式撤销再撤下配置。
