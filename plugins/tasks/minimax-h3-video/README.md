# MiniMax H3：第三方 OpenAI 视频兼容插件

插件key为`minimax-h3-video`，版本1.0.0，随应用内置；它使用第三方OpenAI视频协议，不使用MiniMax原生`/v2/video_generation`。原`hailuo`插件和`MiniMax-H3`模型不变。

## 模型与价格

| 第三方真实模型名 | 计费清晰度 |
| --- | --- |
| `minimax_h3-1080p` | `1080P` |
| `minimax_h3-2K` | `2K` |
| `minimax_h3-768p` | `768P` |

模型名按用户提供的大小写原样转发，清晰度从实际转发的模型后缀确定。有768P，不补720P。清晰度配置不会使MiniMax原生H3支持1080P，也不代表对第三方生成质量的验证。

按用户最终要求，本插件**按秒和清晰度计费**，用量为`seconds`及`resolution`，没有按次`count`计费字段。管理员在“模型定价 → 表达式 → 配置任务定价”按档填写每秒美元单价；各模型独立定价，不自动复制原海螺/Grok的价格。金额为秒数乘对应档位单价，再按宿主分组倍率和显式请求规则处理。避免选“按请求”固定价，否则宿主会按管理员选择的固定价格结算。

请求必须提供`seconds`或`duration`，大于0且不超过宿主安全上限3600；同时提供时必须相等。此上限不表示上游支持3600秒。不猜测默认时长；缺失、非法值、多个视频或与模型后缀冲突的清晰度都会拒绝。计费参数只接受顶层字段，metadata/parameters不能另带时长、清晰度或批量参数绕过校验。

成功响应明确返回有效`seconds`或`duration`时按实际时长结算；没有返回时保留提交秒数。完成响应的非法、超限或互相冲突的时长显式报错，交宿主处理，不默认为0。清晰度保留请求模型档位；失败退款沿用宿主生命周期。

## 安装和请求

1. `git pull`并重新构建/替换容器后，在“任务插件”找到 **MiniMax H3 (OpenAI compatible)**；不重建时可单独上传本目录的`plugin.js`。
2. 新建“Task Plugin / 任务插件”渠道，选择`minimax-h3-video`，配置第三方地址、密钥和上述模型名，再配置价格。不要选择原生海螺渠道。
3. 地址填写供应商根地址或`/v1`地址，支持保留自定义路径前缀，不填完整`/videos`路径。

```json
{
  "model": "minimax_h3-1080p",
  "prompt": "海浪拍打礁石，镜头缓慢推进",
  "seconds": 6
}
```

通过`POST /v1/videos`提交JSON或multipart；单个图片文件使用`input_reference`。不填写resolution也能从模型名正确计费，不额外向上游添加resolution。若显式填写resolution或size，应匹配所选模型；支持768P/1080P/2K标签及短边768/1080/1440的像素尺寸，字段值原样转发。

创建接口为`POST /v1/videos`（顶层id/task_id），查询为`GET /v1/videos/{id}`；标准queued/processing/in_progress/completed/failed等状态按宿主处理。视频下载沿用鉴权content接口，或上游明确返回的HTTP(S)视频URL采用无凭据代理。

支持纯字符串input的`/v1/responses`，包含同步、流式、background；视频参数使用同样的seconds/duration、resolution/size规则。不静默丢弃工具、历史输入、会话恢复或图片输入：这些Responses能力显式拒绝，图生视频通过`/v1/videos`。

## 兼容与验证

插件运行时禁止import，且上传插件须为独立单文件，因此沿用现有Grok视频插件已经验证的OpenAI协议实现并保留为自包含文件，不扩展宿主模块系统。不修改其他插件、数据库结构或依赖。回退应用可使用custom.4；回退前停用新插件渠道，保留模型价格备份。

验证使用真实插件宿主引擎、模拟响应与前端测试，不调用付费供应商。供应商特殊响应包装或接口差异需要其文档，不能由模型名推断。

2026-09-09 / custom.5 验证（Go1.26.0、Node24.12.0、Bun1.4.2，无新增依赖）：

- `go test -mod=readonly ./plugins ./pkg/billingexpr ./pkg/jsplugin ./relay/channel/task/jsplugin -count=1 -timeout=180s`：四包通过。覆盖三个模型的公开视频/Responses路由、渠道别名与实际模型透传、6/10秒三档不同单价金额及额度换算、完成时长覆盖、缺失/非法/溢出时长、清晰度冲突、metadata绕过、查询状态和原生H3归属不变。
- `go vet ./plugins`；`go run -mod=readonly . plugin lint plugins/tasks/minimax-h3-video/plugin.js`：通过。
- 前端video-resolution工具与组件测试23项、`bun run typecheck`、`bun run build`：通过。
- 真实Chromium模型定价页读取新插件schema，逐一点击768P/1080P/2K并确认原单价保留：通过。只有既有Combobox nativeButton警告，无新增页面错误。
- 新插件JS及两处TS文件定向lint/format、Go格式和diff检查：通过。