# 注册邮箱提示调整

## 范围与基线

- 目标：注册入口明确提示使用 QQ 邮箱，邮箱格式、邮箱限制和网络失败提示使用当前界面语言，消除重复弹窗。
- 优先级：P2，局部前端文案与提示修复；保留服务端邮箱规则、验证码及会话行为，无数据库、API 契约或依赖变更。
- 起点：`main` / `3ceddd45f`，工作区干净，上游 `fork/main`，远程 `https://github.com/lysimportant/forknewapi.git`。
- 环境：Node v24.12.0、仓库本地 Bun 1.4.2、Go 1.26.0；复用已安装前端依赖。Bun 位于 `.local-tests/responses-docker/tools/node_modules/bun/bin/bun.exe`。
- 基线：原有登录验证 8 项测试通过。新增邮箱回归测试在修复前复现两条英文重复提示及英文网络错误（3 失败 / 1 通过）。

## 实现与验证

- 复用现有 `Input`、`FormMessage`、`toast` 和 `AuthOperationError`。仅邮箱验证码接口关闭全局错误提示，由调用 hook 翻译后显示一次。
- 输入提示为“请使用 QQ 邮箱（例如：123456@qq.com）”；邮箱不受支持时提示“当前邮箱不支持，请使用 QQ 邮箱（例如：123456@qq.com）。”。
- 两个新增翻译键通过技能要求的临时脚本写入七种语言，再执行 `bun run i18n:sync`；全部语言缺失键为 0，临时脚本已移除。
- `bun run test src/features/auth/hooks/__tests__/email-verification.test.tsx src/features/auth/otp/__tests__/login-verification.test.tsx --maxWorkers=2`：13 项通过，包含实际注册表单输入与点击、两种服务端拒绝、网络失败以及成功倒计时。
- `bun run typecheck`、`bun run build`：通过；生产构建耗时 46.6 秒。修改的四个 TS/TSX 文件定向 oxlint 通过，使用 oxfmt 格式化并原样保留现有版权头；`git diff --check` 通过。
- 最终 `bun run test src/features/auth --maxWorkers=2`：10 文件 / 76 项通过，无控制台错误。`bun run preview --port 4173 --host 127.0.0.1` 启动通过，根页面返回 HTTP 200；预览服务器直接访问 `/sign-up` 返回 404（未提供 SPA 回退），注册交互由上述真实组件测试验证。
- OWASP 参考：Authentication Cheat Sheet、Session Management Cheat Sheet（https://cheatsheetseries.owasp.org/）。仅调整展示，不放宽服务端控制，不回显输入邮箱、验证码或会话令牌；不声明对整个认证系统完成合规审计。

## 交付边界

- 本地验证使用模拟网络响应，不发送真实邮件，不操作生产环境；上线需使用此提交重新构建前端。
- 本次为小改动，提交并推送 `main` 到 `fork/main`，无需 Tag。提交前复查全部差异及敏感信息，推送后核对远端提交。
