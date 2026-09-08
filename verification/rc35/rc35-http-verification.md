# rc35 完整应用隔离 HTTP 验收

## 最终 Docker 制品复验

2026-09-08 18:25—18:27（Asia/Shanghai），直接提取最终镜像 forknewapi:rc35-custom-verification 内的 /new-api，使用 run.ps1 -SkipBuild 在新隔离容器再次完成 **39/39 通过**。镜像本身另在无外网、只读文件系统与全新 tmpfs 数据库下启动，/api/status 版本 v1.0.0-rc.35.custom.1 和首页访问均通过；测试容器已移除。

- 镜像 ID：sha256:53f2207532824270461a011582bd19866c653557996cc820170f5135bbdd27e3。
- 最终二进制 SHA256：05AB1F47B2D48DA83ACD45BE1E2E10D43DC5D9FAC09CB4FB4A4CD04FB4B1AADE。
- 最终证据：report.json、run.log、image-status.json、build-manifest.json；旧主机编译清单保留为 build-manifest-host-build.json。
- 后续改动仅为测试文件位置、中文注释及验收文档；生产行为不变。以下旧时间和主机编译哈希为首轮历史记录，不是最终制品身份。

## 首轮结论

2026-09-08 **16:24:39—16:25:24（Asia/Shanghai）**，从 rc35 合并工作树重新构建的完整应用 `v1.0.0-rc.35.custom.1` 在隔离容器中完成 **39/39 项通过**：36 个 Responses 请求、1 项消费日志总账核对、2 项官方 Sora 视频计费流程。

只证明本地模拟上游下的 HTTP/协议/计费行为，不代表真实供应商、生产部署、全量前端或三数据库验收通过。

## 范围与隔离

- 工作树：`D:\newapi\.local-tests\worktrees\responses-main`；分支 `main`，起点/构建时 HEAD `fe0cd4aa2a47519e74963672b41722ece8438cac`；测试对象包含其他 Agent 正在集成的未提交 rc35 改动，而非该 HEAD 的纯净树。
- 仅新增专属 `D:\newapi\.local-tests\rc35-http\` runner/忽略输出及本报告；未编辑生产代码，未提交、推送或操作 Git 索引。
- 缓存镜像 `forknewapi:responses-main-test` **仅作为 Linux/Node 运行时**；实际运行 `/out/new-api` 是从当前工作树新编译的二进制，没有使用镜像中的旧应用。
- 每轮新建 `rc35-http-<random>` 容器：`--network none --read-only`，`/data`、`/tmp` 使用 tmpfs，结束 `--rm` 自动删除。没有挂载生产数据、访问生产容器或占用生产端口，也没有公网出口。
- 应用端口 18335 仅位于该容器的隔离网络空间，runner 访问 `127.0.0.1:18335`；fixture 显式绑定 `127.0.0.1:18336`。无宿主端口发布。
- 新管理员随机密码、会话密钥、专属 API 令牌、假渠道密钥仅保留内存及该轮 tmpfs 数据库。输出过滤这些凭据；无真实上游、真实密钥或付费请求。
- 只使用新建 SQLite 数据库验证完整应用 HTTP 链路；数据库 Agent 的 fresh/upgrade/多数据库矩阵不由本测试替代。

## 精确测试结果

### Responses

每个渠道覆盖客户端 `/responses` 与 `/v1/responses`，每条路径覆盖 JSON/SSE 两种模式；每种模式分别发起文字请求、工具调用、回传工具输出。

| 渠道 | 类型 | 上游 Base URL 覆盖 | 结果 |
| --- | ---: | --- | --- |
| OpenAI | 1 | `http://127.0.0.1:18336/native`，自动补齐 `/v1` | 12/12 |
| DeepSeek | 43 | `http://127.0.0.1:18336/native/v1`，不重复追加 `/v1` | 12/12 |
| Mistral / Chat 桥接 | 42 | `http://127.0.0.1:18336/bridge` | 12/12 |

各请求断言：

- HTTP 200；文字为 `TEXT_OK`，工具结果返回文字为 `ROUNDTRIP_OK`。
- 工具名 `read_file`，参数 `{"path":"README.md"}`；回传内容 `LOCAL_FILE_OK` 确实到达 fake upstream。
- SSE 事件名与 JSON 类型一致、序号连续、唯一 `response.completed` 终态；JSON 输出状态 `completed`。
- 响应用量均为输入 **11**、输出 **3**、总计 **14** token。
- 显式隔离价格：输入/输出倍率及 default 分组倍率均为 1；每次用户余额、用户累计使用、令牌余额、令牌累计使用四个数值的变化均为 **14 quota**。
- 消费日志共 **36 条**，每条输入/输出 token 与实际响应一致，每条 **14 quota**，文本总收费 **504 quota**。
- 上游只收到 **24 次原生 `/native/v1/responses` POST + 12 次 `/bridge/v1/chat/completions` POST**，每个客户端请求只生成一次，没有自动重试。

### 官方视频插件

通过完整应用 `POST /v1/videos` 创建、`GET /v1/videos/:id` 查询；官方 Sora 渠道类型 55，消费审计确认插件 **`sora` / `1.0.1` / QuantumNous**。价格只在本轮隔离配置中设为 `u("seconds") * 0.1`，并选择 `tiered_expr` 模式。

| 场景 | 预扣快照 | 上游终态 | 结算 / 退款 | 结果 |
| --- | ---: | --- | --- | --- |
| 成功，请求 4 秒 | 用户/令牌均扣 200000 | 完成，实际 6 秒 | 追加扣 100000，最终 300000 | 通过 |
| 失败，请求 4 秒 | 用户/令牌均扣 200000 | `LOCAL_FAKE_FAILURE` | 退回 200000，最终净收费 0 | 通过 |

- fixture 在读取预扣快照之前保持 `queued`，之后才放行终态，避免快速完成掩盖预扣阶段。
- 成功任务日志 ID **37 / 38**：两条消费日志（type=2），分别 200000 / 100000；结算日志 `actual_quota=300000`、`usage_facts.seconds=6`。
- 失败任务日志 ID **39 / 40**：预扣消费日志（type=2）200000、退款日志（type=6）200000。退款后四个用户/令牌余额与累计使用字段均恢复到本任务创建前。
- 两次视频创建 POST，fake upstream 各收到一次 GET 查询；无重复创建。
- 全轮用户额度 `100000000 → 99699496`；令牌额度 `50000000 → 49699496`；两者累计使用均为 **300504 = 504 + 300000 + 0**。

### 应用与 fixture

- `/api/status` 返回 `v1.0.0-rc.35.custom.1`，应用首页 HTTP 200。
- Node fixture 合约自检 **12/12** 通过；仅属测试工具自检，未将其混入 39 项应用结果。
- 完成后核对没有残留 `rc35-http-*` 运行容器，临时数据库随容器删除。

## 构建与复现

检查环境：Go `go1.26.0 windows/amd64`、Node `v24.12.0`、Docker Server `29.7.2`。该 shell 的 Bun 不在 PATH；未安装无关依赖，前端构建由另一 Agent 负责。

```powershell
# 从工作树重建当前完整应用，然后新建隔离容器执行；构建失败不会退回旧应用。
& D:\newapi\.local-tests\rc35-http\run.ps1

# 工具层独立检查。
node --check D:\newapi\.local-tests\rc35-http\runner.mjs
node D:\newapi\.local-tests\rc35-http\fixture-selftest.mjs
```

构建实际使用 `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 GOWORK=off`，命令：

```text
go build -ldflags "-X github.com/QuantumNous/new-api/common.Version=v1.0.0-rc.35.custom.1" -o D:\newapi\.local-tests\rc35-http\new-api .
```

构建退出码 0，最终 SHA256：

```text
93F4F27AA207EEEE8E4099274B88E474F6919C336687F2C20169DFC262F7FB1C
```

`build-manifest.json` 另记录构建时间、HEAD、运行时与当时嵌入的前端入口 SHA256。工作树仍可能继续变化；之后生产代码有变化需重新运行，不把此二进制结果无条件推广到最终提交。

专属输出目录 `D:\newapi\.local-tests\rc35-http\`：

- `run.ps1`、`runner.mjs`、`fixture-selftest.mjs`：可重跑工具。
- `report.json`：最终逐项 HTTP 响应、上游工具历史、余额、用量、视频消费/退款日志。
- `run.log`：`{"success":true,"passed":39,"failed":[],"upstream":40}`。
- `build.log`、`build-manifest.json`：最终构建成功与二进制身份。
- `app.log`：过滤临时凭据后的完整应用日志。
- `report-first.json`、`report-second.json`、`report-third.json`：保留调试轮与第一次全通过结果，不覆盖为最终结果。

## 中间问题与归因

1. 初始完整编译被 `relay/responses_chat_writer.go` 引用已不存在的 `service/relayconvert` 阻断。保留 `build-retry.log`、`build-retry-3.log` 证据；backend Agent 修改为 relaykit 路径后，本 Agent 自动重试并构建成功。没有安装不存在的内部包，也未抢改生产文件。
2. 前两轮 Mistral 工具回传出现 **fixture 返回的 HTTP 500**：原测试错误要求上游 `tool_call_id` 保持 `call_rc35` 且 `content` 是字符串。实际适配器按既有 Mistral 规则把调用 ID 映射为 9 位字母数字，并将 content 编码为文本数组；assistant 工具调用与工具结果引用的 ID 一致，内容未丢失。
3. 修正的只有专属 fixture 断言：验证相同 ID 关联、9 位格式及文本数组的实际内容，而不是强制另一种合法表现形式。随后完整 HTTP 39/39 通过，并在新构建上再次 39/39 通过。未将 harness 错误上报成生产代码缺陷。

## 缺口与交接

- 覆盖代表性的 3 种文本渠道和 1 个官方视频插件，不是全部渠道/插件矩阵。
- 没有真实供应商签名、模型权限、限流、网络故障或真实视频媒体内容/下载/播放验收。
- 此轮未覆盖视频 Responses 同步/流式/background 输出、立即创建失败、进程重启恢复、重复终态/幂等退款、超时退款；视频预扣/成功差额结算/异步失败退款已经覆盖。
- 此轮未复跑旧版 fixture 的截断流、length、缓存 token、compact 和其他供应商专用协议场景。
- 首页 HTTP 200 不等于桌面 UI 交互、控制台错误或最终前端构建验证通过；相关工作仍由前端 Agent 验证。
- 根模块单元测试、lint/vet、独立 relaykit 构建和数据库矩阵由对应 Agent 执行；本报告只认领完整应用编译及上述 HTTP 冒烟。
- 不含生产数据兼容性、生产价格迁移或上线授权结论。最终集成提交前若源码再次改变，应使用同一 runner 重跑。
