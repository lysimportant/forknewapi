/** 统一视频协议可供多个渠道复用；各渠道独立配置地址、密钥和模型映射，售价由管理员设置。 */
export const meta = {
  apiVersion: 1,
  key: "seedance-video",
  name: "Seedance Unified Video",
  description: {
    en: "Seedance video generation through the unified video API",
    zh: "通过统一视频 API 对接 Seedance 视频生成",
  },
  version: "1.0.0",
  author: { name: "lysimportant/forknewapi" },
  baseUrl: "https://cnd-coo-new.pages.dev",
  models: [],
  modelRouting: "channel",
  modelDiscovery: { protocol: "openai", path: "/v1/models" },
  fetchMode: "per_task",
  requiredCapabilities: ["task-submit-no-retry@1"],
  protocols: ["openai_video"],
  usageSchema: {
    count: {
      type: "number",
      unit: "count",
      unitLabel: { en: "video", zh: "个" },
      description: { en: "Video generation unit price", zh: "视频生成单价" },
    },
  },
  usageExamples: [{ label: "1 video", facts: { count: 1 } }],
};

/** 与宿主 MaxTaskDurationSeconds 保持一致的安全上限，不代表模型支持的时长。 */
const MAX_SECONDS = 3600;

/** 仅接收已确认的统一参数及 OpenAI 视频入口别名，禁止原生厂商参数绕过校验。 */
const REQUEST_FIELDS = new Set([
  "model",
  "prompt",
  "duration",
  "seconds",
  "ratio",
  "aspect_ratio",
  "resolution",
  "size",
  "quality",
  "negative_prompt",
  "images",
  "input_reference",
  "start_frame",
  "end_frame",
  "reference_videos",
  "reference_audios",
  "generate_audio",
]);

/** 兼容渠道根地址和 /v1 地址，保留路径前缀并拒绝地址内的凭据、查询或片段。 */
function apiBase(ctx) {
  const base = typeof ctx.baseUrl === "string" ? ctx.baseUrl.trim().replace(/\/+$/, "") : "";
  if (!/^https?:\/\/[^/?#@\s\\]+(?:\/[^?#\s\\]*)?$/.test(base))
    throw new Error("channel base URL must be an HTTP(S) API base without credentials, query or fragment");
  return base.endsWith("/v1") ? base : base + "/v1";
}

/** 校验绝对媒体地址；素材必须为 HTTPS，内容下载的 SSRF 检查由宿主执行。 */
function isMediaURL(value, httpsOnly) {
  return typeof value === "string" && (httpsOnly ? /^https:\/\// : /^https?:\/\//).test(value) && /^https?:\/\/[^/?#@\s\\]+(?:[/?#][^\s\\]*)?$/.test(value);
}

/** 统一秒数字段及入口别名；缺省时交给上游，不推定某个模型的默认值。 */
function durationValue(value) {
  if (typeof value !== "number" && !(typeof value === "string" && value.trim())) throw new Error("duration must be a number between 0 and 3600, excluding 0");
  const duration = Number(value);
  if (!Number.isFinite(duration) || duration <= 0 || duration > MAX_SECONDS) throw new Error("duration must be a number between 0 and 3600, excluding 0");
  return duration;
}

/** 模型由宿主渠道绑定授权并映射；只校验非空名称和统一字段，保留显式 false。 */
function upstreamBody(request, model) {
  if (typeof model !== "string" || !model.trim()) throw new Error("model is required");
  if (!request || typeof request !== "object" || Array.isArray(request)) throw new Error("video request must be a JSON object");
  for (const field of Object.keys(request)) {
    if (!REQUEST_FIELDS.has(field)) throw new Error("unsupported video request field");
  }
  if (typeof request.prompt !== "string" || !request.prompt.trim()) throw new Error("prompt is required");
  const body = { model, prompt: request.prompt };
  for (const [field, alias] of [
    ["duration", "seconds"],
    ["ratio", "aspect_ratio"],
    ["resolution", "size"],
  ]) {
    let value = request[field] === undefined ? request[alias] : request[field];
    if (value === undefined) continue;
    if (field === "duration") value = durationValue(value);
    else if (typeof value !== "string" || !value.trim()) throw new Error(field + " must be a non-empty string");
    if (request[field] !== undefined && request[alias] !== undefined) {
      const alternate = field === "duration" ? durationValue(request[alias]) : request[alias];
      if (value !== alternate) throw new Error(field + " conflicts with " + alias);
    }
    body[field] = value;
  }
  for (const field of ["quality", "negative_prompt"]) {
    if (request[field] === undefined) continue;
    if (typeof request[field] !== "string") throw new Error(field + " must be a string");
    body[field] = request[field];
  }
  for (const field of ["images", "reference_videos", "reference_audios"]) {
    if (request[field] === undefined) continue;
    if (!Array.isArray(request[field]) || !request[field].every((url) => isMediaURL(url, true)))
      throw new Error(field + " must be an array of public HTTPS file URLs");
    body[field] = request[field].slice();
  }
  if (request.input_reference !== undefined) {
    if (!isMediaURL(request.input_reference, true))
      throw new Error("input_reference must be a public HTTPS image URL; upload local files with /v1/media first");
    body.images = (body.images || []).concat(request.input_reference);
  }
  for (const field of ["start_frame", "end_frame"]) {
    if (request[field] === undefined) continue;
    if (!isMediaURL(request[field], true)) throw new Error(field + " must be a public HTTPS image URL");
    body[field] = request[field];
  }
  if (body.end_frame !== undefined && body.start_frame === undefined) throw new Error("end_frame requires start_frame");
  if (request.generate_audio !== undefined) {
    if (typeof request.generate_audio !== "boolean") throw new Error("generate_audio must be a boolean");
    body.generate_audio = request.generate_audio;
  }
  return body;
}

/** 读取 JSON 或纯文本 multipart；文件必须先通过上游独立 /v1/media 接口取得 URL。 */
function decodedBody(body) {
  if (!body) throw new Error("video request body is required");
  if (body.kind === "json") return body.value;
  if (body.kind !== "multipart") throw new Error("unsupported video request body");
  if ((body.files || []).length) throw new Error("upload local files with /v1/media first and submit the returned HTTPS URLs");
  const request = {};
  for (const field of Object.keys(body.fields || {})) {
    const values = body.fields[field];
    if (!Array.isArray(values) || !values.length) throw new Error("invalid multipart video field");
    if (["images", "reference_videos", "reference_audios"].includes(field)) {
      request[field] = values.length === 1 && values[0].trim().startsWith("[") ? JSON.parse(values[0]) : values;
    } else {
      if (values.length !== 1) throw new Error("duplicate multipart video field");
      request[field] = field === "generate_audio" ? JSON.parse(values[0]) : values[0];
    }
  }
  return request;
}

/** 创建请求使用稳定公共任务 ID 作为幂等键；结果未知时禁止自动重发。 */
export function buildSubmitRequest(ctx) {
  const body = upstreamBody(ctx.requestBody, ctx.upstreamModel || ctx.model);
  if (typeof ctx.publicTaskId !== "string" || !ctx.publicTaskId) throw new Error("public task id is required for idempotency");
  return {
    url: apiBase(ctx) + "/videos",
    method: "POST",
    headers: { Authorization: "Bearer " + ctx.apiKey, "Content-Type": "application/json", Accept: "application/json", "Idempotency-Key": ctx.publicTaskId },
    body,
    noRetry: true,
  };
}

/** 解析创建 envelope；异常响应不回显上游正文，防止密钥进入错误信息。 */
export function parseSubmitResponse(ctx, response) {
  const body = response && response.body;
  if (!response || response.statusCode < 200 || response.statusCode >= 300 || !body || typeof body !== "object" || Array.isArray(body))
    throw new Error("invalid upstream video creation response");
  if (body.code !== 0 || body.success !== true || body.error) throw new Error("upstream rejected video creation");
  if (typeof body.task_id !== "string" || !body.task_id.trim()) throw new Error("upstream task_id is missing");
  const result = parseTaskResult(ctx, body);
  if (result.status === "UNKNOWN") throw new Error("invalid upstream video creation status or result URL");
  const parsed = { taskId: body.task_id, taskData: body };
  if (result.status === "SUCCESS" || result.status === "FAILURE") parsed.immediate = result;
  return parsed;
}

/** 按单个视频冻结预扣用量；管理员设置单价，倍率阶段不重复计费。 */
export function extractUsage(ctx) {
  upstreamBody(ctx.requestBody, ctx.upstreamModel || ctx.model);
  return ctx.usagePurpose === "billing_ratios" ? {} : { count: 1 };
}

/** 成功生成一个视频结算一次；失败退款由宿主负责，不采用上游积分或金额。 */
export function extractUsageOnComplete(_task, result) {
  return result && result.status === "SUCCESS" ? { count: 1 } : null;
}

/** 查询原渠道任务并编码任务 ID，避免将 ID 解释为地址路径。 */
export function buildQueryRequest(ctx) {
  if (typeof ctx.taskId !== "string" || !ctx.taskId) throw new Error("upstream task id is required");
  return {
    url: apiBase(ctx) + "/tasks/" + encodeURIComponent(ctx.taskId),
    method: "GET",
    headers: { Authorization: "Bearer " + ctx.apiKey, Accept: "application/json" },
  };
}

/** 仅使用上游返回的四种内容地址，不推测 /content 或替换域名。 */
function videoURL(body) {
  if (!body || body.status !== "succeeded") return "";
  for (const field of ["video_url", "url", "result_url", "download_url"]) {
    if (isMediaURL(body[field], false)) return body[field];
  }
  return "";
}

/** 严格映射文档状态；成功缺地址属于异常响应，失败原因脱敏后保留。 */
export function parseTaskResult(ctx, body) {
  if (!body || typeof body !== "object" || Array.isArray(body)) return { status: "UNKNOWN", reason: "invalid upstream video task response" };
  if ((body.code !== undefined && body.code !== 0) || body.success === false || body.error)
    return { status: "UNKNOWN", reason: "upstream rejected video task query" };
  if (body.status === "processing") return { status: "IN_PROGRESS" };
  if (body.status === "succeeded") {
    const url = videoURL(body);
    return url ? { status: "SUCCESS", progress: "100%", url } : { status: "UNKNOWN", reason: "upstream video URL is missing or invalid" };
  }
  if (body.status === "failed") {
    let reason =
      [body.error_code, body.error_detail].filter((value) => typeof value === "string" || typeof value === "number").join(": ") ||
      "upstream video generation failed";
    if (ctx && ctx.apiKey) reason = reason.split(ctx.apiKey).join("[redacted]");
    reason = reason.replace(/Bearer\s+\S+|\bsk-[A-Za-z0-9_-]+/gi, "[redacted]");
    return { status: "FAILURE", reason };
  }
  return { status: "UNKNOWN", reason: "unrecognized upstream video task status" };
}

/** 完成且有地址时声明单个视频制品，通过宿主授权内容入口访问。 */
export function listArtifacts(task) {
  return task.status === "SUCCESS" && videoURL(task.data) ? [{ key: "video", type: "video", mimeType: "video/mp4" }] : [];
}

/** 无凭据代理上游内容地址，Range、HEAD、重定向和 SSRF 校验交给宿主。 */
export function buildContentRequest(ctx) {
  if (ctx.artifactKey !== "video") throw new Error("artifact_not_found");
  const url = videoURL(ctx.data);
  if (!url) throw new Error("upstream video URL is unavailable");
  return { url, method: ctx.clientRequest.method, credentialless: true };
}

/** OpenAI 视频入口只呈现公共任务信息，避免向客户端泄露供应商地址和错误正文。 */
export const protocols = {
  openai_video: {
    decodeRequest: function (ctx) {
      const body = upstreamBody(decodedBody(ctx.body), ctx.upstreamModel || ctx.model);
      const action =
        (body.reference_videos || []).length || (body.reference_audios || []).length || (body.images || []).length > 1
          ? "reference_to_video"
          : body.start_frame || (body.images || []).length
            ? "image_to_video"
            : "text_to_video";
      // 宿主用客户端别名固定路由；buildSubmitRequest 再使用 upstreamModel 发送精确 ID。
      return { kind: "submit", model: ctx.model, action, requestBody: Object.assign({}, body, { model: ctx.model }) };
    },
    render: function (_ctx, task) {
      const statuses = { NOT_START: "queued", SUBMITTED: "queued", QUEUED: "queued", IN_PROGRESS: "in_progress", SUCCESS: "completed", FAILURE: "failed" };
      return { id: task.task_id, object: "video", model: (task.properties || {}).origin_model_name, status: statuses[task.status] || "unknown" };
    },
  },
};
