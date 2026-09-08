# 视频清晰度配置补齐（custom.4）

版本：v1.0.0-rc.35.custom.4；基线：75615a080。日期：2026-09-09。

按用户最新要求，视频型号主入口补齐1080p；没有720p且没有768p时补720p。有768p的型号保留768p，不额外加入720p。仍保留品牌低档、高档、原始大小写和所有旧价格；最多五个主入口，超出部分继续可编辑。补充的是定价配置，不承诺型号或动作支持该生成规格，实际接受范围由上游决定。已有协议校验保持有效：H3的1080P请求仍被拒绝（该型号原生仅768P/2K），Vidu Q1的请求仍归一到1080p；新增价格入口不伪造能力或绕过校验。

- 通义万相、豆包、Google、Vertex AI、Grok、海螺、Vidu：沿用插件已有resolution价格行，在型号主入口补齐。H3主列768P/1080P/2K；Q1主列720p/1080p。未核实型号使用已有插件配置值，不再隐藏全部主档。
- Sora：插件1.0.2在size枚举末尾增加1920x1080、1080x1920；提交及完成用量均保留真实尺寸，既有720与1024短边尺寸不删除。主入口保留两个1080尺寸；1024x1792在其他规格继续编辑。
- 可灵：插件1.0.2新增720P/1080P价格分档，依据实际请求std/pro模式提取resolution，继续按Units计价；原单价表达式在两档使用相同单价。v2-master仍拒绝std。即梦：现有720/1080的product分别定价，保留两种1080产品的独立价格。
- Suno是音乐插件，不添加视频规格。

旧Sora分档表达式可继续在矩阵打开：新增组合继承原表达式的fallback常数与单价，不自动设免费，不保存时不改配置。原条件及原大小写保持不变。自定义复杂表达式仍按原限制保留原始编辑方式。内置插件随应用升级；手动安装的Sora/可灵插件需自行更新至1.0.2。

无数据库、依赖或数据迁移；Responses与Sub2耗时颜色代码不变。升级前保留现有价格表达式备份；可回退custom.3应用版本，新增尺寸表达式在旧编辑器可能显示原始模式，勿重置已有价格。

验证（Windows，Go1.26.0、Node24.12.0、Bun1.4.2；无新增依赖）：

- `web: bun run test src/features/pricing src/features/system-settings/models src/features/task-plugins --maxWorkers=2`：21文件251项通过；最终可灵界面变更后定向video-resolution组件13项再通过。
- `web: bun run typecheck`、`bun run build`：通过。5个TS/TSX文件定向oxlint无错误，task-expr有既有import顺序warning；保护版权头format通过。
- `go test -mod=readonly ./plugins ./pkg/jsplugin ./relay/channel/task/jsplugin -count=1 -timeout=180s`：三包最终通过。Sora六尺寸本地模拟HTTP转发、预扣及完成用量；可灵std/pro/metadata/默认模式、Units费率、master拒std及完成仅覆盖Units均通过。
- `go test -mod=readonly ./service -run '^TestSettle_TieredUsageFactsMergeCompletionOverSubmission$' -count=1 -timeout=180s`：通过。`go test -mod=readonly . -run '^$' -timeout=180s`：主程序编译通过。
- 两插件JS lint无错误，各有一处既有preserve-caught-error warning；oxfmt、Go格式及diff检查通过。
- Chromium运行真实模型定价页，读取全部11个内置插件真实schema，以模拟API逐一点击全部主档并确认原单价保留：11/11通过。即梦保持product价格，Suno保持音乐价格。无新增页面错误，既有Combobox nativeButton警告仍存在。

本轮不调用付费供应商接口。以下custom.3记录保留作官方能力依据；其中旧版主入口限制已由上述用户配置规则覆盖。

---
# 按品牌与官方文档展示视频规格

版本：v1.0.0-rc.35.custom.3；基线：c069d5aa2。核对日期：2026-09-08。

## custom.3行为记录

主界面读取插件自己的resolution/size，按清晰度从低到高展示，最多五档，不凑数。已获得明确官方依据的型号进一步按型号筛选；供应商默认及超出主列的原配置在独立区域保留，保存仍使用原始枚举、大小写和价格索引。插件详情展示品牌并集，模型定价按具体型号展示。未验证的旧型号只保留原配置，不推断能力。

Grok 1.5主列480p、720p、1080p。未新增无文档依据的2K。4k是旧版本已有的第三方兼容参数，仅在“供应商其他规格”保留，不表示官方支持；接口和旧收费行为不变。unspecified显示“供应商默认（未指定）”，不冒充480p。插件版本1.1.0说明同步更新，原生schema未删除旧值。

可灵界面显示官方std/pro与720P/1080P的对应，同时保持Units定价；v2-master仅pro/1080P。即梦显示各product的720P/1080P对应，1080P普通产品与Pro仍分别定价。只展示说明，不新增虚构usage.resolution，也不从消耗Units反推模式。

复用现有Tabs、Collapsible、Combobox、矩阵和价格输入。没有新增依赖、数据库迁移或价格自动迁移；Responses与Sub2耗时颜色代码未改。回退可以使用custom.2应用版本，保留现有管理员价格配置。手动上传过Grok 1.0.0的实例需上传1.1.0以更新插件说明；使用内置版本的实例随镜像更新。

## 官方资料与实际采用范围

以下是本轮实际读取的官方文档，未调用付费生成API。API规格不等于所有动作、时长与清晰度的笛卡尔积都有效，也不等于已验证供应商实际视频像素。第三方OpenAI协议兼容不用于推导清晰度。

| 品牌 | 官方文档 | 本版处理 |
| --- | --- | --- |
| Grok | https://docs.x.ai/developers/model-capabilities/video/generation | 1.5文生/图生三档480p/720p/1080p；参考生最高720p，编辑不能自定义resolution。未声明2K/4K。 |
| Kling | https://kling.ai/document-api/api/video/1-0/text-to-video 、https://kling.ai/document-api/guides/capability-map/video 、https://kling.ai/document-api/pricing/base/video | v1/v1-6：std720P、pro1080P；v2-master仅1080P。仍按上游final_unit_deduction结算，资源单位不是货币或分辨率。 |
| Jimeng | https://docs.volcengine.com/docs/85621/1538636?lang=zh 、https://www.volcengine.com/docs/85621/1792704 、https://www.volcengine.com/docs/85621/1792702 、https://docs.volcengine.com/docs/85621/1777001?lang=zh | s2_pro/v30_720p为720P产品；v30_1080p/v30_pro为1080P产品，保留独立价格与req_key。 |
| MiniMax/Hailuo | https://platform.minimax.io/docs/api-reference/video-generation-v2-create.md 、https://platform.minimax.io/docs/api-reference/video-generation-t2v.md 、https://platform.minimax.io/docs/api-reference/video-generation-i2v.md | H3仅768P/2K；2.3及Fast为768P/1080P；02为512P/768P/1080P（512P仅图生）；五个明确01型号为720P。S2V-01输出规格未验证，放入其他规格。 |
| Vidu | https://platform.vidu.com/docs/text-to-video 、https://platform.vidu.com/docs/image-to-video 、https://platform.vidu.com/docs/reference-to-video | Q2为540p/720p/1080p，Q1仅1080p；2.0按动作和时长可能支持360p/720p/1080p，主入口列这个并集，原适配器仍处理组合约束。1.5当前资料未验证，不展示已确认档位。 |
| Alibaba/Wan | https://help.aliyun.com/zh/model-studio/wan3-video-generation-api-reference 、https://help.aliyun.com/zh/model-studio/text-to-video-api-reference 、https://help.aliyun.com/zh/model-studio/legacy-image-to-video-api-reference | 3.0和2.5可三档；2.7只720P/1080P；2.2-i2v-plus只480P/1080P；2.1-i2v-turbo只480P/720P，plus只720P。旧价不删除。 |
| Doubao/Seedance | https://docs.volcengine.com/docs/82379/1520757?lang=zh | 2.0标准可480/720/1080/4k；fast/mini仅480/720；2.5、1.5pro、1.0pro三档到1080。按系列文档筛既有型号，部分日期ID可调用性未独立验证；lite输出未验证，放入其他规格。 |
| Google/Veo | https://ai.google.dev/gemini-api/docs/veo | 3.1文档列720/1080/4k，高档仅8秒；3.0参数表和型号能力表冲突，主列只确认的720/1080。 |
| Vertex AI/Veo | https://docs.cloud.google.com/gemini-enterprise-agent-platform/reference/rest/Shared.Types/VideoGenerationModelParams | 本次官方通用参考只列720/1080，4k缺型号级证据；与Gemini同名模型共用定价入口时只主列双方确认的720/1080，旧4k价格保留在其他规格。插件详情仍显示原声明，不能当作型号能力证明。 |
| Sora | https://developers.openai.com/api/docs/guides/video-generation 、https://developers.openai.com/api/reference/resources/videos | 官方公共size类型有四个既有尺寸；指南另列Pro 1920x1080/1080x1920，资料不同步。本次保留原尺寸、不推断对应1080p或2K，不扩展适配器。 |
| SunoAPI | https://github.com/Suno-API/Suno-API | 音乐/歌词插件，不启用视频分辨率入口。 |

## 已发现但未扩展修改的旧适配问题

以下作为后续TODO记录，不能声称本版已完成供应商全功能兼容：

- P1：Wan2.7文生文档已改resolution+ratio，旧插件仍沿用size；需后续单独调整出站协议并验证旧型号兼容。
- P1：Kling旧型号文档仅5/10秒，但旧校验宽松；Vidu2.0参考生与图生组合限制不同，旧代码部分路径共用；本版没有更改协议或计费单位。
- P1：Sora新尺寸与Veo不同官方页面存在冲突，进一步适配需按型号/渠道确认；未把图片Upscale的2K/4K移植为视频生成能力。
- 未验证：Vidu1.5、裸viduq2图生的型号映射、S2V-01输出规格、豆包lite及部分精确日期ID、即梦所有图生req_key。旧配置保留不等于调用支持背书。
- 时效：Kling官方公告这些旧型号2026-09-15下线；OpenAI指南公告Sora 2/Videos API于2026-09-24关闭。仅记录官方公告，未通过付费API验证实际下线状态。

## 验证记录

环境：本地Go1.26.0、Node24.12.0、Bun1.4.2、Docker29.7.2；Dockerfile固定Bun1.4.0与Go1.26.1。

- Go插件三包回归通过；Grok未声明2K时返回错误，旧4K请求兼容仍通过。
- 最终前端全量 `bun run test --maxWorkers=2`：93文件745用例通过；`bun run typecheck`通过。9个本次TS/TSX文件定向lint零错误/警告，保护版权头的定向格式检查通过；七语言四个新增key完整同步。`git diff --check`通过。
- Chromium逐一使用11个插件的实际完整schema与模拟API验证主入口及原生价格，11/11通过；默认区、旧4K价格保存、Kling模式说明、未知规格与键盘操作另有回归测试。保留既有Combobox nativeButton警告和jsdom scrollTo提示，没有新增页面错误。
- Docker构建通过；最终镜像隔离启动后 `/api/status` 精确返回custom.3，首页正常。镜像sha256:418172e0ee121e8b636b972dc2c798ca4fba575a437fa0bb6846810f6b4285d7；二进制SHA256:A247768C05709880C0F3188811993675FB06FFD151D49789EBCD693B7041F927。
- 2026-09-09最终HTTP主轮14/14、补充轮14/14通过。使用最终镜像提取的同一二进制，覆盖官方三档、默认与旧4K兼容、2K及1440短边拒绝且零出站/零扣费、参数冲突、按次表达式、4/12秒同价、ModelPrice固定单价、失败全额退款和GET/HEAD制品下载。运行命令为本地证据目录的 `pwsh -NoProfile -File run.ps1 -SkipBuild` 与 `supplemental.ps1 -SkipBuild`，按顺序执行。
- 所有HTTP使用隔离容器、模拟上游、新建tmpfs数据，无生产凭据、无真实供应商扣费；验证网关计费契约，不验证生成质量。

原始文档证据在D:/newapi/.local-tests/video-provider-docs/（含两个独立报告）；最终接口证据在D:/newapi/.local-tests/video-docs-http/；浏览器证据在工作树web/.local-tests/。旧video-five-http只对应先前方案，不作为本版最终证据。均为忽略目录，不提交数据库、令牌或二进制。