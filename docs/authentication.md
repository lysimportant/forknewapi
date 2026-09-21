# 用户鉴权与登录会话

面板鉴权采用短期 Access Token、HttpOnly Refresh Cookie 与服务端登录会话控制面的组合。面板请求不再依赖 Gin session，也不再要求 `New-Api-User` 请求头。

## 鉴权模型

- Access Token 是有效期 15 分钟的 JWT，只保存在浏览器内存中，通过 `Authorization: Bearer <token>` 发送。
- Refresh Token 是随机不透明值，有效期最长 30 天。浏览器只通过 `HttpOnly`、`SameSite=Strict` Cookie 持有它；服务端仅保存 HMAC 摘要，并在每次刷新时轮换。
- `new_api_has_session` 是 Refresh Cookie 的会话提示，值恒为 `1`，`Path=/`、非 `HttpOnly`，与 Refresh Cookie 同时写入、同时清除、同一过期时间。它只声明"曾签发过 Refresh Cookie"，不含任何凭据，也不参与任何鉴权判定；伪造它唯一的效果是自费一次注定失败的 refresh。它存在的原因是 Refresh Cookie 被 `HttpOnly` 和 `Path=/api/user/auth` 双重限制，`/` 上的页面无法判断自己是否匿名，否则每次冷启动都要发一次注定 401 的 refresh，而该请求还会占用按 IP 计数的 `CriticalRateLimit` 配额。
- `user_sessions` 是登录会话控制面，记录设备、IP、登录方式、最后活跃时间、到期时间和撤销状态。数据库中的 Session 状态是最终权威；撤销传播速度取决于下文所述的 Redis 拓扑。
- 用户的密码、状态、角色或安全因子发生安全相关变化时，`auth_version` 会递增并使旧登录会话失效。订阅带来的分组升降级只刷新授权缓存，不会退出任何登录设备。
- Redis 缓存保存用户鉴权快照和登录会话快照。版本栅栏和撤销 tombstone 防止旧缓存重新授权；Session 快照使用跟随 `SYNC_FREQUENCY` 的短 TTL，缓存未命中或未启用 Redis 时回退到数据库校验。

`SESSION_SECRET` 用于派生 Access Token、Security Proof、Refresh Token 摘要和 AuthFlow 摘要的不同用途密钥。生产环境及多节点部署必须在所有节点配置相同的高强度随机值；更换该值会使现有登录、临时鉴权流程和 Security Proof 全部失效。

## 多节点 Redis 拓扑

多节点部署必须共用同一主数据库。登录 Session、账户级活跃 Session 上限和签发窗口计数都以数据库为权威，因此这些限制在应用节点间全局生效。Redis 中的 Session Hash（包含 `revoking`/`revoked` tombstone）只是缓存，其 TTL 为 Session 剩余寿命与有效 `SYNC_FREQUENCY` 中的较小值；`SYNC_FREQUENCY` 默认及非法值回退均为 `60` 秒。读取缓存不会续期，过期后会按 SID 回源数据库。延迟完成的 active 缓存回写只能使用其数据库观察窗口尚未消耗的 TTL，不能在撤销 tombstone 到期后重新启动一个完整缓存周期。

| Redis 部署方式 | Session 状态传播 | 限流语义 |
| --- | --- | --- |
| 所有节点共享 Redis | 正常撤销和版本发布通过同一缓存即时传播 | Redis 限流额度在所有节点间共享 |
| 每个节点使用独立 Redis | 最迟在该节点 Session 缓存 TTL 到期后回源收敛，即不超过有效 `SYNC_FREQUENCY`；版本轮换期间，新 Token 在持有旧缓存的节点上可能短暂返回 401 | 每个节点独立计数，集群总额度最坏约为单节点阈值乘以节点数 |
| 不使用 Redis | 每次 Session 校验直接读取数据库 | 使用各节点的内存限流器，额度同样按节点独立 |

`SYNC_FREQUENCY` 越大，独立 Redis 部署的陈旧窗口越长；值越小，每个活跃 SID 在每个节点上回源数据库的频率越高。默认配置下，持续活跃的 Session 每个节点最多约每 60 秒增加一次数据库主键点查。共享 Redis 时，撤销 tombstone 和版本发布仍保持即时传播。

所有节点必须使用相同的 `SESSION_SECRET`。当多个节点连接同一个 Redis 时，还必须使用相同的 `CRYPTO_SECRET`，否则节点生成的缓存键摘要不一致，无法正确共享缓存。上述保证只覆盖登录 Session 鉴权的有界陈旧语义；限流额度及其他 Redis 缓存仍会受到 Redis 拓扑影响，不能据此认为整个控制面与拓扑无关。

## 浏览器接口

登录成功后，密码登录、2FA、Passkey、OAuth、WeChat 和 Telegram 登录均返回统一数据：

```json
{
  "success": true,
  "data": {
    "access_token": "...",
    "token_type": "Bearer",
    "access_expires_at": 1730000000,
    "user": {},
    "session": {
      "sid": "...",
      "current": true,
      "login_method": "password",
      "ip": "...",
      "user_agent": "...",
      "created_at": 1730000000,
      "last_active_at": 1730000000,
      "expires_at": 1732592000
    }
  }
}
```

会话相关接口：

| 接口 | 鉴权 | 用途 |
| --- | --- | --- |
| `POST /api/user/auth/refresh` | Refresh Cookie；Secure 模式附加 Origin 校验 | 轮换 Refresh Token 并签发新的 Access Token |
| `POST /api/user/auth/logout` | Refresh Cookie；Secure 模式附加 Origin 校验，可同时携带 Bearer | 撤销当前登录会话并清除 Cookie |
| `GET /api/user/sessions` | Bearer | 查看当前鉴权版本的有效登录会话，当前会话优先，最多 100 条 |
| `DELETE /api/user/sessions/:sid` | Bearer | 撤销指定登录会话，包括当前会话 |
| `POST /api/user/sessions/revoke-others` | Bearer | 保留当前会话并撤销其他会话 |

客户端内存中已有会话时，应在 refresh/logout 请求中发送 `X-Auth-Session: <sid>`。Refresh Cookie 与该 SID 不一致时，两个端点都返回 `409 AUTH_SESSION_MISMATCH`，且不会轮换、撤销或清除任何会话；客户端先通过 refresh 清除本标签页的旧 SID、恢复 Cookie 当前对应的会话，再重试 logout。冷启动尚无内存会话时可以省略该请求头。

并发使用同一个 Refresh Token 时，服务端通过确定性轮换恢复同一个后继 Token，多个浏览器标签页不会因丢失“胜者”响应而被迫退出。最近一代 Refresh Token 在短暂容错窗口结束后再次出现会撤销对应会话；无法识别的更早代或随机 Token 只会被拒绝，不会允许攻击者凭猜测踢掉会话。

前端使用 Web Locks 串行化同一浏览器配置文件中的刷新，并通过 BroadcastChannel（不支持时回退到 `storage` 事件）仅同步会话标识和登录/退出事件；Access Token 与 Refresh Token 都不会通过跨标签页消息传递或持久化到 Web Storage。

前端将冷启动状态与登录状态分开管理。网络或服务端临时故障允许后续导航重试 refresh；服务端确认 Refresh Cookie 无效时才进入已完成的匿名状态。内存 SID 与 Cookie SID 不一致时，客户端清除旧内存身份并在不携带旧 SID 的情况下重试一次。

公开页面的冷启动会先读 `new_api_has_session`：提示不存在且内存中没有任何身份时跳过 refresh，直接按匿名渲染，且**不**把这次跳过记为已完成的匿名判定——跳过只是延后，不是服务端结论。会依据鉴权结果做跳转的位置（受保护路由与登录页）不看提示，内存为空时一律回源。因此提示缺失但 Refresh Cookie 有效的用户（该 Cookie 上线前建立的会话，或只清理了 `/` 站点数据的浏览器）会在公开页显示为匿名，并在进入上述任一位置时自动恢复登录态，不需要重新输入密码。提示因服务端撤销而过期时，那次 refresh 返回 401 并在同一响应里清除提示，浪费的请求只发生一次。

## Session 签发限额与保留策略

服务端在所有登录方式的统一 Session 签发出口执行两级账户限制：

- `USER_SESSION_ACTIVE_LIMIT`（默认 `50`）：单用户未过期且状态为 active 的 Session 上限。达到上限时新登录返回 `409 AUTH_SESSION_LIMIT`。
- `USER_SESSION_ISSUANCE_LIMIT`（默认 `100`）和 `USER_SESSION_ISSUANCE_WINDOW_SECONDS`（默认 `86400`）：统计窗口内该用户创建的所有 Session，包含已撤销和旧鉴权版本的记录。达到上限时返回 `429 AUTH_SESSION_ISSUANCE_LIMIT`。
- 这两次计数与插入不加跨数据库锁；极端并发登录可能出现少量超额，但计数失败会拒绝签发，不会降级放行。

升级时已经超过活跃上限的账户不会被自动下线或挤掉旧会话；限制只作用于后续的新 Session 签发。

`USER_SESSION_REVOKED_RETENTION_DAYS`（默认 `7`）控制 revoked 行的审计保留期。签发计数依赖窗口内的行仍存在，因此签发窗口不得超过 revoked 保留期。如果配置超出，启动时会记录告警并将实际窗口钳制到保留期，避免提前删除 revoked 行导致限流计数被低估。

定时清理即使发现 `expires_at` 已过期，也不会删除 `created_at` 仍落在实际签发窗口内的行；尚未达到 revoked 保留期的撤销记录同样会继续保留。这样在扩大配置窗口时，过期清理不会静默削弱签发计数或审计保留。

活跃数量会计入状态仍为 active 但 `user_auth_version` 已过期的异常残留行，而设备列表只展示当前鉴权版本。因此遇到 `AUTH_SESSION_LIMIT` 时，应优先在仍已登录的设备上执行“撤销其他会话”，该操作会同时清理不可见的旧版本 active 行；没有可用设备时可使用密码重置撤销所有会话。密码重置不会清空签发窗口计数。

仅 master 节点每小时分批删除过期 Session 和超过保留期的 revoked Session。`USER_SESSION_HOURLY_ALERT_THRESHOLD`（默认 `5000`）只在最近一小时全局签发量异常时记录告警，不会形成可被滥用的全站登录拒绝开关。

## Refresh/Logout 的 Origin 校验

refresh/logout 的 Origin 防护与 Refresh Cookie 的 Secure 模式绑定：

- 未配置 `SESSION_COOKIE_SECURE` 或显式设为 `false` 时，Refresh Cookie 可用于本地 HTTP，refresh/logout 的 OriginGuard 关闭，并且不得配置 `SESSION_COOKIE_TRUSTED_URL`。这使 `http://localhost` 上不同端口的 Rsbuild/Vite 开发代理可以正常转发请求。该模式仅用于可信的本地开发环境，不应暴露到公网。
- `SESSION_COOKIE_SECURE=true` 时，Refresh Cookie 仅通过 HTTPS 发送，同时启用严格 OriginGuard。`POST /api/user/auth/refresh` 和 `POST /api/user/auth/logout` 会校验浏览器的 `Origin`；缺少 `Origin` 时只接受合法的单一 `Referer` 作为回退。允许来源包括请求自身的精确 Origin，以及 `SESSION_COOKIE_TRUSTED_URL` 中配置的精确 Origin。

Secure 模式的 Origin 校验不信任客户端直接发送的 `X-Forwarded-Proto`。TLS 在反向代理终止时，应将面板的公开 HTTPS Origin 明确写入 `SESSION_COOKIE_TRUSTED_URL`。

`SESSION_COOKIE_TRUSTED_URL` 现在具有明确的新语义：它是 refresh/logout Cookie 端点的可信 Origin 列表，不是 CORS 白名单。配置规则如下：

- 仅在 `SESSION_COOKIE_SECURE=true` 时配置；多个值用英文逗号分隔。
- 每项必须是精确的 HTTPS Origin，例如 `https://panel.example.com` 或 `https://panel.example.com:8443`。
- 不接受通配符、路径、查询参数、用户信息或域名后缀匹配。
- 不会修改 relay、旧 billing dashboard、`/api/usage/token` 或 `/api/log/token` 的 CORS 行为。浏览器使用 `sk-` key 直连 relay 的场景保持不变。

本地 HTTP 开发示例（OriginGuard 关闭）：

```env
SESSION_SECRET=<local-random-value>
SESSION_COOKIE_SECURE=false
# SESSION_COOKIE_TRUSTED_URL 不得设置
```

生产 HTTPS 示例（OriginGuard 开启）：

```env
SESSION_SECRET=<high-entropy-random-value>
SESSION_COOKIE_SECURE=true
SESSION_COOKIE_TRUSTED_URL=https://panel.example.com,https://admin.example.com
```

该开关只控制面板 Refresh Cookie 和 refresh/logout 的 OriginGuard，不会修改 relay、旧 billing dashboard、`/api/usage/token` 或 `/api/log/token` 的 CORS 行为。

## 可信代理与 IP 限流

Gin 默认会信任所有代理提供的客户端 IP 请求头。本项目改为兼顾常见反代拓扑和公网直连安全的三态配置：

- 未配置、空字符串或纯空白的 `TRUSTED_PROXIES` 默认信任 `127.0.0.0/8`、`::1`、`10.0.0.0/8`、`172.16.0.0/12`、`192.168.0.0/16` 和 `fc00::/7`，并输出启动告警。该默认值覆盖同机 Nginx、Docker Compose 和常见内网反代；公网直连地址不在列表中，其伪造的 `X-Forwarded-For` 会被忽略。
- `TRUSTED_PROXIES=none`（大小写不敏感且必须单独使用）启用严格直连模式，不信任任何代理，`ClientIP()` 只使用 TCP 直连地址。
- 其他非空值按英文逗号解析为代理 IP/CIDR，并完全替代默认列表。应填写反向代理自身的地址而不是客户端网段；非法 CIDR、空列表或将 `none` 与其他值混用都会阻止服务启动。

Gin 只在请求的直连来源属于可信代理时解析客户端 IP 请求头，并从转发链右侧向左寻找首个非可信地址。因此常见 Nginx `$proxy_add_x_forwarded_for` 链中的公网客户端地址会阻止更左侧的伪造前缀生效。默认信任私网的残余风险是：能够从同一私网直接访问应用的其他机器或容器仍可伪造这些请求头；需要消除此风险时应使用 `none` 或配置精确代理地址。

Redis 限流使用原子 Lua 固定窗口，替代旧的近似滑动窗口 List 实现。这是有意的语义变化：窗口边界两侧可分别打满一次，极短时间内通过量最高约为配置值的两倍。例如 `20 次/20 分钟` 在边界可通过约 40 次。帐户级 Session 上限和签发窗口继续控制数据库增长；如未来需要严格抑制边界突发，需单独迁移为 ZSET 滑动窗口。

用户级模型成功请求限流仍使用原有 Redis List 近似滑动窗口，但列表时间戳统一写为 UTC。滚动升级期间，旧节点写入的本地时间字符串和新节点写入的 UTC 字符串无法从格式上区分，可能在一个模型限流窗口内临时误放行或误拒绝。所有节点升级完成并经过一个完整窗口后会自然收敛；本次升级不会切换 Key 或主动删除现有列表。

开放注册仍会受 Critical IP 限流保护，但分布式 IP 多账号攻击不能仅靠 IP 限流阻止。公网开放注册的部署应同时启用 Turnstile 和邮箱验证；更强的设备或多维风控需作为独立安全项目设计。

## PAT 调用契约

`User.AccessToken`（面板 PAT）继续支持 `Authorization: Bearer <pat>`，也兼容原有的单值 `Authorization: <pat>`。`New-Api-User` 不再参与鉴权，外部脚本不需要再发送 Bearer 与用户 ID 双请求头。这是有意的调用契约简化；旧 PAT 本身无需重新生成。

PAT 不是浏览器登录会话，不能调用登录会话管理接口，也不能签发绑定具体登录会话的 Security Proof。

## 临时鉴权流程与二次验证

OAuth state、2FA pending、Passkey ceremony、Telegram bind 等临时状态存放在 `auth_flows`。客户端只持有随机 `flow_token`，数据库仅保存 HMAC 摘要；流程具有用途、provider、intent、用户和登录会话绑定，并且只能原子消费一次。OAuth 注册的 affiliate code 也随登录 AuthFlow 保存。

标准 OAuth 绑定回调由 popup 通过同源 `postMessage` 交给 opener；只有 opener 使用自身内存中的 Bearer 调用后端绑定接口。Telegram 绑定先由已登录前端创建绑定 AuthFlow，再让 widget 回调携带路径中的 `flow_token`，回调时会重新确认原登录会话仍有效。Telegram 的已签名 widget assertion 也会登记为一次性凭据，重复回放会被拒绝。

敏感操作使用有效期 5 分钟的 `X-Security-Proof`：

- `channel.key.read`：查看渠道密钥；
- `passkey.register`：注册 Passkey；
- `passkey.delete`：删除 Passkey。

Proof 同时绑定用户、登录会话、用户鉴权版本、会话版本和 scope，不能跨用户、跨会话或跨用途复用。

启用了 2FA 的用户注册 Passkey 时，register begin 与 finish 都必须携带有效的 `passkey.register` Proof；finish 会在消费一次性 AuthFlow 之前重新验证 Proof。未启用 2FA 的首次 Passkey 注册不要求该请求头。

## 升级注意事项

- 旧 `session` Cookie 不再使用；升级后现有面板登录会失效，用户需要重新登录。
- 数据库迁移会新增 `user_sessions`、`auth_flows`、`external_identity_claims` 和 `users.auth_version`，并为已有用户初始化鉴权版本、回填 Telegram 账号唯一归属；若历史数据中同一 Telegram ID 已绑定多个用户，迁移会拒绝继续启动，需先消除歧义。
- 数据库迁移会为 Session 签发计数和分批清理新增索引；已有 `user_sessions` 很大时应为首次启动预留维护窗口。
- `user_sessions.previous_refresh_hash` 会从定长 `char(64)` 迁移为 `varchar(64)`。应用会兼容读取历史定长字段留下的空格填充；迁移后的目标结构必须保持幂等，连续启动不应反复执行列类型变更。
- 仅 master 节点定时清理过期登录会话、超过配置保留期的 revoked 会话和已过保留期的 AuthFlow。
- 未配置 `TRUSTED_PROXIES` 时会兼容信任回环和常见私网代理；使用公网负载均衡器、`100.64.0.0/10`、链路本地地址或自定义 CNI 网段的部署仍需显式配置。需要严格忽略所有转发头时设置为 `none`。
- Redis 限流从近似滑动窗口改为原子固定窗口，存在明确的边界双倍突发语义。
- 用户级模型成功请求限流的 UTC 时间戳在滚动升级期间存在一个窗口的混合格式过渡，期间可能临时误放行或误拒绝。
- 自建客户端应按新的 AuthBundle、`flow_token` 和 Security Proof 契约升级；PAT 客户端可直接移除 `New-Api-User`。

## Canvas 账号接入合同

Canvas 账号接入让一个固定 Canvas 实例通过 New API 浏览器登录取得受限授权，并为当前用户的可用分组创建或恢复专用 Token。该接口不接收 New API 密码，也不能读取或管理用户的其他 Token。

### 部署配置

账号接入默认关闭。启用时必须同时配置以下变量：

```env
CANVAS_ACCOUNT_ENABLED=true
CANVAS_ACCOUNT_ISSUER=https://api.example.com
CANVAS_ACCOUNT_CLIENT_ID=canvas
CANVAS_ACCOUNT_INSTANCE_ID=main
CANVAS_ACCOUNT_REDIRECT_URI=https://canvas.example.com/v1/auth/newapi/callback
CANVAS_ACCOUNT_CLIENT_SECRET=
CANVAS_BRIDGE_ENABLED=true
```

- `CANVAS_ACCOUNT_ISSUER` 是 New API 的规范化来源，只允许无用户信息、查询和片段的绝对 URL，路径必须为空。issuer 和回调均要求 HTTPS，只有 localhost 或回环 IP 允许 HTTP 本地验收。
- `CANVAS_ACCOUNT_CLIENT_ID` 最长 64 字节，`CANVAS_ACCOUNT_INSTANCE_ID` 最长 128 字节。服务端只接受与部署值完全相同的客户端和实例。开启 `CANVAS_ACCOUNT_ENABLED` 即表示将该实例作为受信的一体化应用，用户登录后自动接入本人分组，不再显示单独授权确认；不能用于未受信的第三方客户端。
- `CANVAS_ACCOUNT_REDIRECT_URI` 是唯一允许的精确回调地址，不支持通配符。
- `CANVAS_ACCOUNT_CLIENT_SECRET` 为空时使用公开 PKCE 客户端；配置后，兑换授权码必须提供精确 secret。该值不能写入日志或前端。
- 生产环境应使用 HTTPS，并按现有会话文档配置 `SESSION_SECRET`、Secure Refresh Cookie 和可信 Origin。

固定 scope 为 `identity:read`、`groups:read` 和 `tokens:manage`。授权码有效五分钟、只能消费一次且只接受 S256 PKCE；grant 有效三十天。重新授权沿用 grant ID、轮换 grant Bearer，并使旧 Bearer 立即失效。

### 浏览器一体化登录

Canvas 生成 PKCE verifier、challenge 和一次性 `state`，然后打开：

```http
GET /api/canvas/authorize
  ?client_id=canvas
  &instance_id=main
  &redirect_uri=https%3A%2F%2Fcanvas.example.com%2Fv1%2Fauth%2Fnewapi%2Fcallback
  &state=<opaque-state>
  &code_challenge=<base64url-sha256>
  &code_challenge_method=S256
  &prompt=select_account
```

普通登录省略 `prompt`；主动切换账号时才传唯一的 `select_account`。空值、重复值或其他值都返回 `400 canvas_authorization_invalid`。

New API 返回禁止缓存、禁止嵌入且带 nonce CSP 的登录中间页。页面通过 HttpOnly Refresh Cookie 恢复浏览器会话，未登录时进入 New API 登录页；登录成功后自动执行同源 POST，后台连接固定 Canvas 并返回工作区，不需要点击“授权”。Access Token 只在页面内存中使用，长期 Key 不进入浏览器。

只有 `prompt=select_account` 显示“继续登录”和“换一个账号”。打开选择页不会注销会话；点击“换一个账号”才携带当前 Access Token 和 Session ID 调用同源 `POST /api/user/auth/logout`，成功后进入 `/sign-in`。站内 `redirect` 保留原登录事务和 PKCE 参数，并删除 `prompt`，使新账号登录后直接回画布。取消返回固定回调的 `error=access_denied`；Canvas 校验并消费本人 state/浏览器事务后清理临时 Cookie，保留原有作品会话。自动连接请求为：

```http
POST /api/canvas/authorize
Authorization: Bearer <browser-access-token>
Content-Type: application/json

{"request_token":"<authorization-request-token>"}
```

该请求只接受浏览器登录 Session，PAT 不能替代。成功响应中的 `redirect_uri` 带一次性 `code` 和原始 `state`。Canvas 必须先核对 `state`，再从后端兑换：

```http
POST /api/canvas/token
Content-Type: application/json

{
  "code": "<one-time-code>",
  "code_verifier": "<pkce-verifier>",
  "client_id": "canvas",
  "instance_id": "main",
  "redirect_uri": "https://canvas.example.com/v1/auth/newapi/callback",
  "client_secret": ""
}
```

成功响应只在这次兑换中返回 grant Bearer：

```json
{
  "success": true,
  "data": {
    "issuer": "https://api.example.com",
    "user": {
      "id": "123",
      "display_name": "Example",
      "status": "active"
    },
    "grant": {
      "id": "00000000-0000-0000-0000-000000000000",
      "token": "<grant-bearer>",
      "expires_at": "2026-10-21T00:00:00Z",
      "scopes": ["identity:read", "groups:read", "tokens:manage"]
    }
  }
}
```

服务端数据库只保存 grant Bearer 的用途隔离 HMAC。兑换响应未知时必须重新开始浏览器授权，不能重放原授权码。

### 账号与分组 Token

后续接口都使用 `Authorization: Bearer <grant-bearer>`，并返回 `Cache-Control: no-store`。

`GET /api/canvas/account` 返回已验证用户、grant ID 和当前可接入分组：

```json
{
  "success": true,
  "data": {
    "user": {"id": "123", "display_name": "Example", "status": "active"},
    "grant_id": "00000000-0000-0000-0000-000000000000",
    "groups": ["auto", "default", "vip"]
  }
}
```

原始分组标识精确等于 `神秘分组` 时始终排除。它不会创建或检查管理 Token，也不会进入 Auto 范围；相近名称不受影响。

`PUT /api/canvas/groups/:group` 幂等创建、读取或恢复该 grant 的唯一管理 Token：

```json
{"operation_id":"<stable-operation-id>"}
```

`operation_id` 只能包含字母、数字、点、下划线、冒号和连字符，长度为 1 至 64。相同 ID 和相同 grant/分组返回原 Token；相同 ID 改用其他分组返回冲突。创建结果未知时重试原操作，不能更换 ID 后再次创建。

```json
{
  "success": true,
  "data": {
    "token_id": "456",
    "key": "<managed-api-key>",
    "group": "default",
    "status": "active",
    "credential_revision": "1",
    "permission_revision": "3",
    "auto_groups": []
  }
}
```

普通分组固定到该分组，`cross_group_retry` 关闭。`auto` 使用当前用户权限、站点 Auto 顺序及 `MaxTokenAutoGroups` 过滤后的非空显式范围，并再次排除 `神秘分组`；空范围拒绝创建，不能回退继承全局 Auto。管理 Token 随 grant 到期且由 New API 计费。用户禁用、删除、改组、限制模型、修改 Auto 范围或修改其他固定字段后，接口返回冲突，不会静默改回。通过令牌管理接口人工修改期限时，令牌与管理关系在同一事务中更新为不可自动恢复；同一秒内的修改、改为永久或撤销后的改期也适用。普通改名不终止管理关系，Canvas 内部续期不走人工配置入口。

受控轮换使用 `POST /api/canvas/groups/:group/rotate`，请求体为 `{"operation_id":"固定操作编号","rotation":{"token_id":123,"credential_revision":1,"key_fingerprint":"原完整 Key 的 SHA-256 小写十六进制"}}`。调用方必须先持久化意图、停止该组新提交并收尾旧任务及回执；New API 不掌握 Canvas 队列状态，不能代替这个检查。接口要求本人有效 grant、纳入组、原 Token、版本和指纹匹配，保留 Token ID 与账务归属，原子更换 Key 并递增修订；响应格式与 PUT 相同。原操作重试返回同一新 Key，换操作编号重试旧版本返回 409，读取 PUT 不接受 rotation 字段。

轮换失效旧 Key 缓存，缓存失效失败时不写数据库；人工改 Key/期限、禁用、删除或撤销不被轮换覆盖。新增 `key_fingerprint` 仅保存摘要且不对外返回，轮换后再人工改 Key 时连原操作重试也拒绝。用户主动撤销仍立即生效，不能为了任务收尾延迟撤销。旧任务继续持有原版本引用，轮换后不承诺旧 Key 可查询，不得切换到新 Key 重发收费请求。审计只记录 grant、组、Token ID、修订和操作 ID，不记录 Key、指纹、请求体或响应体。

升级先备份数据库，AutoMigrate 增加 `key_fingerprint VARCHAR(64) NOT NULL DEFAULT ''`；已有行不重写 Key。回退前停止轮换与新提交并核实待处理操作，保留新列和操作记录，不恢复旧 Key。三数据库验证包含 SQLite 3.50.4、MySQL 5.7.44、PostgreSQL 9.6.24 的事务回滚、幂等重试、人工修改、迁移及重复迁移；权限选项查询使用已引用的保留字列名。

`POST /api/canvas/revoke` 撤销 grant，并只禁用该 grant 明确归属的管理 Token：

```json
{}
```

已由 Canvas 撤销且未被修改的 Token 可在显式重新授权后恢复同一 Token ID 和 Key。撤销前已被用户修改的 Token 会标记为不可恢复，不会因重新授权被复活。其他用户 Token 不受影响。

### 执行受理头

Canvas 使用管理 Token 发起创建类请求时，必须且只能发送一个 `x-canvas-execution`。其值是以下 JSON 的 UTF-8 字节经过无填充 canonical base64url 编码后的结果：

```json
{
  "version": 1,
  "issuer": "https://api.example.com",
  "user_id": "123",
  "instance_id": "main",
  "grant_id": "00000000-0000-0000-0000-000000000000",
  "token_id": "456",
  "expected_group": "auto",
  "permission_revision": "3",
  "auto_groups": ["default", "vip"]
}
```

对象必须恰好包含这九个字段。`token_id` 和 `permission_revision` 是无前导零的正十进制字符串。普通分组的 `auto_groups` 必须为空；`expected_group` 为 `auto` 时必须提供非空、有序、无重复的显式实际分组范围。

该头只允许用于以下管理 Token POST 创建接口：

- `/v1/chat/completions`
- `/v1/images/generations`
- `/v1/images/edits`
- `/v1/audio/speech`
- `/v1/videos`

`GET /v1/videos/...` 轮询不需要该头。New API 在渠道和实际分组确定后复核 issuer、用户、实例、grant、Token、预期策略分组、Auto 范围、权限修订、模型 Ability 和渠道状态；失败会在预扣和供应商 POST 前终止请求。

权限修订只跟随授权和调用资格变化，例如 grant 续期、用户或分组资格、Token 固定权限、Auto 显式范围、模型 Ability 与渠道状态。余额消耗、渠道余额和单纯价格数值变化不会递增该修订，也不构成锁价。

### 错误响应

账号接口统一返回：

```json
{"success":false,"code":"canvas_authorization_invalid","message":"..."}
```

主要错误码：

| HTTP | code | 含义 |
| --- | --- | --- |
| 400 | `canvas_authorization_invalid` | 授权请求、授权码已失效或被重放 |
| 400 | `canvas_token_request_invalid` | 兑换请求或 PKCE verifier 无效 |
| 400 | `canvas_operation_invalid` | 管理 Token 请求体无效 |
| 400 | `canvas_execution_invalid` | 执行头缺失、重复、非 canonical 或字段不合法 |
| 401 | `canvas_client_invalid` | 固定客户端配置或 secret 不匹配 |
| 401 | `canvas_authorization_invalid` | grant 无效、过期或已撤销 |
| 403 | `canvas_session_required` | 授权批准不是有效浏览器 Session |
| 403 | `canvas_group_excluded` | 命中精确排除分组 |
| 403 | `canvas_group_unavailable` | 用户、模型或实际分组当前不可用 |
| 403 | `canvas_execution_mismatch` | 执行身份、策略分组或 Auto 范围不一致 |
| 409 | `canvas_operation_conflict` | 相同 operation ID 携带不同请求 |
| 409 | `canvas_auto_group_empty` | Auto 过滤后没有可用实际分组 |
| 409 | `canvas_managed_token_changed` | 管理 Token 被修改、删除或不可恢复 |
| 409 | `canvas_permission_changed` | 权限修订已过期，Canvas 必须刷新后重选 |
| 409 | `canvas_token_limit_reached` | 用户 Token 数量达到站点上限 |
| 503 | `canvas_account_misconfigured` | 启用后部署配置不完整或不合法 |
| 503 | `canvas_account_unavailable` | 权威账号或数据库操作暂不可用 |

安全审计只记录用户、grant、实例、分组和 Token ID 等不可直接使用的标识，不记录授权码、grant Bearer、API Key、客户端 secret 或浏览器 Access Token。

### 验证记录（2026-09-21）

认证实现参考 OWASP ASVS 稳定版 5.0.0，以及 [Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)、[Session Management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)、[OAuth 2.0](https://cheatsheetseries.owasp.org/cheatsheets/OAuth2_Cheat_Sheet.html) 和 [CSRF Prevention](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html) 指南。核对范围是本次新增的授权与管理 Token 合同，不代表整站 ASVS 认证。

- 回归覆盖固定回调、S256 PKCE、短期一次性授权码、拒绝重放、仅浏览器会话批准、grant 轮换/撤销、分组精确排除、Key 人工变更、Auto 范围与受理身份不一致时在供应商 POST 前拒绝；Redis 缓存失效失败时管理变更回滚。
- 一体化登录参照 ASVS 5.0.0 的 V7.2/V7.4 会话更换与服务端失效要求，以及 OAuth 2.0 指南中的固定回调、浏览器绑定 state 和 S256 PKCE。普通登录在部署方信任的固定实例内自动连接；换号仍为显式操作。参数回归覆盖非法、空值和重复 prompt 拒绝；SID/Cookie 不匹配和旧会话使用仍由现有会话测试覆盖。局部回归不构成整站 ASVS 认证，PC 实测记录见 Canvas 账号接入检查点。
- 追加人工期限回归：同步与重新授权都不抵消用户改期；事务写入失败时期限和管理状态一起回滚。参照 ASVS 5.0.0 V7.3/V7.4 的到期和终止要求，不把时间戳先后当作修改来源；三库矩阵新增 `token-configuration` 用例。
- 本地真实浏览器完成 Canvas → New API 登录授权 → 全部分组同步 → 文字生成和 Worker 归档 → 退出；双用户项目、凭据与 Run 隔离通过。供应商出口为本机 Mock，不证明真实供应商计费或生成成功。
- `go test ./model -run '^TestCanvasAccountDatabaseMatrix$' -count=1 -v`，显式设置隔离 `TEST_MYSQL_DSN`、`TEST_POSTGRES_DSN` 和 `CANVAS_REQUIRE_DATABASE_MATRIX=true`：SQLite 3.50.4、MySQL 5.7.44、PostgreSQL 9.6.24 的 fresh、upgrade、重复迁移、索引/唯一约束及数据保留均通过。日志数据库结构不在本次变更范围。
- 升级增加 Canvas 专属表，不改现有用户资金或渠道数据；账号端点默认关闭。回退前禁用账号接入并停止 Canvas 新提交，保留表和管理记录以恢复既有授权，不删除仍被任务引用的 Token。
