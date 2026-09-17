# rc.37 PC Web 独立浏览器验收

最终产物于 2026-09-17 18:43:41–18:44:04（Asia/Shanghai）完成完整复跑，14 项检查通过、0 个待修复发现。真实多行兑换码粘贴缺陷已修复并在交付二进制验证；外部商店的 403 限制单列如下。

## 执行方式与证据

- 执行者：`announcement_review` 子 agent。浏览器为独立 Playwright Chromium，窗口 1440×900，协议另验 1280×720。
- 脚本：`D:/newapi/.local-tests/rc37-acceptance/core-browser.cjs`。运行命令为 `node core-browser.cjs`，默认使用同目录 `new-api-rc37-delivery.exe`，可用 `CORE_BROWSER_BINARY` 选择产物。最终实际命令为 `$env:CORE_BROWSER_BINARY = 'new-api-rc37-delivery.exe'; node core-browser.cjs`，退出码 0。
- 后端固定 loopback 3023，使用新建独立 SQLite；随机账号密码、会话密钥只在进程内生成，不输出凭据。每轮 `finally` 关闭浏览器和该轮后端。已有 3021、3022 服务没有停止或修改。
- 首轮产物 `new-api-rc37.exe` SHA-256：`847477ef142ce1a809734dbbcaf694fb4c7da2ce085b70d3fdf1638175dcee6b`。
- 最终产物 `new-api-rc37-delivery.exe` SHA-256：`6982307aca447d4c3b5da7130482d7364784dfa227a967334efbf927fad1d121`，运行开始与结束哈希相同。
- 结果与截图为同目录 `core-browser.results.json`、`core-browser.*.png`；修复前精确复现保存在 `core-browser.paste-reproduction.json`。

## 最终产物核实的行为

| 流程 | 验证结果 |
| --- | --- |
| 登录同意 | 未勾选点击登录显示提醒且保留登录页；主动勾选后使用真实后端登录成功 |
| 协议阅读 | 长正文在 Dialog 内可滚到第 24 节；1280×720 视口中 Dialog 为 720×576，URL 与同意状态不变 |
| 注册 | 实际创建账号；请求不含 `consent` / `consent_version`，响应没有 token/Set-Cookie；匿名 `self` 仍为 401；返回登录页保持未勾选 |
| QQ 邮箱提示 | 中文界面输入非 QQ 邮箱点击发码，仅出现一条原因提示，没有发出邮件请求 |
| 冻结奖励 | 独立 SQLite 写入一笔明确的 48 小时测试奖励；真实 API 返回冻结额及 0 可提额；钱包禁用提现，详情展示冻结与到期时间 |
| 兑换码首行 | 真实 Ctrl+V 粘贴带标签的 CRLF 多行文本，`/api/user/topup` 只提交首条 32 位码并成功兑换；实际余额增加 $1 |
| 兑换码复制/批删 | 复制两条选中记录得到纯兑换码；Windows 剪贴板的 CRLF 作为换行归一；真实 `/api/redemption/batch` 删除两条，刷新后不再出现 |
| 页面入口 | Channels、Task Plugins、Pricing 正常打开，空库价格页面显示空态 |
| 关于/文档 | 运营联系信息保留；管理员账号、QQ群复制值均正确；文档入口 href 与真实 `/api/status.docs_link` 一致 |
| 关于/页脚协议 | “阅读协议全文”、用户协议、隐私政策均打开对应 Dialog，URL 保持 `/about` |

没有发送真实邮件、操作外部身份认证器或调用付费 Provider。冻结奖励为显式数据库夹具，本报告只验证真实 API 和 UI 展示，不替代奖励产生/提现的三库回归。

## 发现并修复的兑换码粘贴问题

原生 Ctrl+V 粘贴 `Batch label\t<32 位测试兑换码>\r\nsecond-line-must-be-ignored`，单行 Input 在 `onChange` 前移除换行。`extractRedemptionKey` 收到的内容已经被拼接，提交的 key 长度为 59 而非 32，服务端返回失败。直接输入同一 32 位码可以兑换。这是实际浏览器路径中的遗漏，不能以提取函数的单测通过判定首行契约成立。

修复仅在 `recharge-form-card.tsx` 的粘贴事件检查换行或制表符，在默认粘贴前复用 `extractRedemptionKey`。普通单行粘贴继续在光标处插入，不自动触发兑换。现有共享 Input 已支持 `onPaste`，无需引入替代输入组件或新依赖。

唯一新增组件测试为 `components/__tests__/recharge-form-card.test.tsx`，覆盖 LF、带标签的 CRLF、单行标签和普通单行输入。修复前 3 失败/1 通过；修复后钱包全目录 8 文件/23 用例通过。命令和结果：

- `node node_modules/vitest/vitest.mjs run src/features/wallet`：23/23，日志 `core-browser.paste-after.log`。
- `node node_modules/@typescript/native-preview/bin/tsgo -b`：通过。
- 两个修改文件 `oxlint -c .oxlintrc.json`、`oxfmt --check` 与 `git diff --check`：通过。

## 外部限制与收尾

- 最终浏览器本地 `pageerror`、console error 和 warning 均为 0；预期匿名 refresh 的 401 单独计数为 2。
- 钱包嵌入的外部商店 `https://wzyp.cn/shop/RPE3AZIX` 返回 403，已保留响应来源与截图；这不是本地页面异常，也不能声称商店可正常访问。
- [x] 最终二进制固定后重新运行全部核心流程，确认真实 Ctrl+V 只提交首行码并兑换成功。
- [x] 10 张最终截图逐一检查，正文和控件可见，没有本次修复引入的重叠或视口溢出。长协议内部滚动、QQ 原因提示、冻结奖励详情、兑换成功、批量操作及页面入口截图保存在 `core-browser.*.png`。
- [x] 运行结束 `serverStopped: true`，测试后端 PID 41192 与独立 Chromium 已关闭；3023 无监听，既有 3021 和主 agent 管理的 3022 仍在。

本子任务不创建提交；上述生产修复、测试及报告由主 agent 完成最终集成和 Git 交付。
