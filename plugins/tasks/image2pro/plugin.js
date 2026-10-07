/** Image2Pro 已确认的视频模型；目录中的其他模型不自动开放生成。 */
const MODELS = ["无限制-Flash-中配-Video", "无限制-Flash-MAX-Video", "Seedance2.0 0.9r"];
/** 文档参考图字段及宿主兼容别名，单次请求只选择一种入口。 */
const IMAGE_FIELDS = ["input_image", "images", "input_reference", "image", "reference_images", "image_urls"];
/** 引用素材的统一入口、数量和格式；音视频字段为用户要求的兼容扩展，仍待上游验证。 */
const MEDIA_RULES = {
  image: {
    field: "images",
    aliases: IMAGE_FIELDS,
    arrays: ["images", "reference_images", "image_urls"],
    limit: 9,
    mimes: ["image/png", "image/jpeg", "image/webp", "image/gif"],
  },
  audio: {
    field: "audios",
    aliases: ["audios", "reference_audios", "audio_urls", "input_audio", "reference_audio", "audio"],
    arrays: ["audios", "reference_audios", "audio_urls"],
    limit: 3,
    mimes: ["audio/mpeg", "audio/mp3", "audio/wav", "audio/x-wav", "audio/ogg", "audio/flac", "audio/aac", "audio/mp4", "audio/webm"],
  },
  video: {
    field: "videos",
    aliases: ["videos", "reference_videos", "video_urls", "input_video", "reference_video", "video"],
    arrays: ["videos", "reference_videos", "video_urls"],
    limit: 3,
    mimes: ["video/mp4", "video/webm", "video/quicktime", "video/x-msvideo"],
  },
};
/** 无 MIME 的上传由扩展名补齐；显式错误的 Content-Type 不凭扩展名覆盖。 */
const FILE_MIMES = {
  png: "image/png",
  jpg: "image/jpeg",
  jpeg: "image/jpeg",
  webp: "image/webp",
  gif: "image/gif",
  mp3: "audio/mpeg",
  wav: "audio/wav",
  ogg: "audio/ogg",
  flac: "audio/flac",
  aac: "audio/aac",
  m4a: "audio/mp4",
  mp4: "video/mp4",
  mov: "video/quicktime",
  webm: "video/webm",
  avi: "video/x-msvideo",
};
/** 严格限定公开参数和音视频扩展，防止隐藏计费数量或其他未适配参数被透传。 */
const REQUEST_FIELDS = ["model", "prompt", "seconds", "duration", "ratio", "aspect_ratio", "client_request_id", "clientRequestId"].concat(
  Object.values(MEDIA_RULES).flatMap((rule) => rule.aliases)
);

/** Image2Pro 插件声明；秒数是用量，价格由管理员配置，不换算上游积分。 */
export const meta = {
  apiVersion: 1,
  key: "image2pro",
  name: "Image2Pro",
  icon: "text:I2P",
  version: "1.0.1",
  author: { name: "lysimportant/forknewapi" },
  description: {
    en: "Image2Pro text and reference-image video generation; optional audio/video references require upstream support",
    zh: "Image2Pro 文本与参考图视频生成；可选音视频参考需上游支持",
  },
  baseUrl: "https://api.image2pro.top/v1",
  website: "https://image2pro.top/api-docs",
  models: MODELS,
  modelDiscovery: { protocol: "openai", path: "/v1/models" },
  fetchMode: "per_task",
  requiredCapabilities: ["task-submit-no-retry@1"],
  protocols: ["openai_video", { name: "openai_responses", supports: ["stream", "sync", "background"] }],
  usageSchema: {
    seconds: { type: "number", unit: "second", description: { en: "Video generation unit price", zh: "视频生成单价" } },
  },
};

/** 先排除 data URL，避免扫描整段 Base64；HTTP(S) 地址仍拒绝内嵌凭据、控制字符和反斜杠。 */
function isHTTPURL(value) {
  if (typeof value !== "string" || !/^https?:\/\//i.test(value) || /[\s\\]/.test(value)) return false;
  for (const character of value) if (character.charCodeAt(0) < 32 || character.charCodeAt(0) === 127) return false;
  return /^https?:\/\/(?:\[[0-9a-f:.]+\]|[a-z0-9.-]+)(?::[0-9]+)?(?:[/?#].*)?$/i.test(value);
}

/** 兼容渠道根地址及 /v1 地址，保留自定义路径前缀且不重复版本段。 */
function apiBase(ctx) {
  const base = typeof ctx.baseUrl === "string" ? ctx.baseUrl.replace(/\/+$/, "") : "";
  if (!isHTTPURL(base) || /[?#]/.test(base)) throw new Error("Image2Pro channel base URL must be an HTTP(S) URL without credentials, query, or fragment");
  return base.endsWith("/v1") ? base : base + "/v1";
}

/** 读取必填秒数；3600 是宿主安全上限，上游协议可能进一步限制合法时长。 */
function requestedSeconds(request) {
  if (request.seconds === undefined && request.duration === undefined) throw new Error("seconds or duration is required for Image2Pro per-second billing");
  for (const field of ["seconds", "duration"]) {
    const value = request[field];
    if (value === undefined) continue;
    if (
      (typeof value !== "number" && typeof value !== "string") ||
      (typeof value === "string" && !/^\d+(?:\.\d+)?$/.test(value.trim())) ||
      !Number.isFinite(Number(value)) ||
      Number(value) <= 0 ||
      Number(value) > 3600
    )
      throw new Error(field + " must be a positive number at most 3600");
  }
  if (request.seconds !== undefined && request.duration !== undefined && Number(request.seconds) !== Number(request.duration))
    throw new Error("seconds and duration conflict");
  return Number(request.seconds === undefined ? request.duration : request.seconds);
}

/** 验证客户端幂等键，两个字段并存时必须一致，不能静默覆盖。 */
function clientRequestId(request) {
  const snake = request.client_request_id;
  const camel = request.clientRequestId;
  if (snake !== undefined && camel !== undefined && snake !== camel) throw new Error("client_request_id and clientRequestId conflict");
  const value = snake === undefined ? camel : snake;
  if (value !== undefined && (typeof value !== "string" || !/^[A-Za-z0-9_.-]{6,120}$/.test(value)))
    throw new Error("Image2Pro idempotency key must contain 6 to 120 letters, digits, underscores, dots, or hyphens");
  return value;
}

/** 将普通引用转换为字符串；占位符须属于本类上传，角色及附加时长不得静默丢弃。 */
function mediaReference(value, type, files) {
  const rule = MEDIA_RULES[type];
  if (value && typeof value === "object" && !Array.isArray(value)) {
    if (Object.prototype.hasOwnProperty.call(value, "__fileRef")) {
      const file = files.find((item) => item.ref === value.__fileRef);
      if (
        !file ||
        !rule.aliases.includes(file.field) ||
        files.filter((item) => item.ref === value.__fileRef).length !== 1 ||
        Object.keys(value).some((key) => !["__fileRef", "encoding", "mimeType"].includes(key)) ||
        value.encoding !== "dataUrl" ||
        !rule.mimes.includes(value.mimeType)
      )
        throw new Error("Image2Pro " + type + " file reference is invalid");
      return { __fileRef: value.__fileRef, encoding: "dataUrl", mimeType: value.mimeType };
    }
    if (Object.keys(value).some((key) => key !== "url" && key !== "role") || (value.role !== undefined && value.role !== "reference_" + type))
      throw new Error("Image2Pro " + type + " references accept url and ordinary reference role only; frame roles and duration metadata are unsupported");
    value = value.url;
  }
  if (isHTTPURL(value)) return value;
  const data = typeof value === "string" ? /^data:([^;,]+);base64,((?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?)$/.exec(value) : null;
  if (data && data[2] && rule.mimes.includes(data[1])) return value;
  throw new Error("Image2Pro " + type + " references must be HTTP(S) URLs or supported " + type + " data URLs");
}

/** 统一参考素材别名并投影上游字段；音视频按兼容扩展转发，图片保留文档合同。 */
function normalizeRequest(source, model, files = []) {
  if (!source || typeof source !== "object" || Array.isArray(source)) throw new Error("Image2Pro request body must be an object");
  if (!MODELS.includes(model)) throw new Error("unsupported Image2Pro model");
  for (const key of Object.keys(source)) if (!REQUEST_FIELDS.includes(key)) throw new Error("unsupported Image2Pro field: " + key);
  const request = { model: source.model, duration: requestedSeconds(source) };
  if (source.prompt !== undefined) {
    if (typeof source.prompt !== "string" || source.prompt.length > 30000) throw new Error("Image2Pro prompt must be a string of at most 30000 characters");
    if (source.prompt.trim()) request.prompt = source.prompt;
  }
  let referenceCount = 0;
  for (const [type, rule] of Object.entries(MEDIA_RULES)) {
    const fields = rule.aliases.filter((field) => source[field] !== undefined);
    if (fields.length > 1) throw new Error("Image2Pro reference " + type + " aliases are mutually exclusive");
    if (!fields.length) continue;
    const field = fields[0];
    const raw = source[field];
    if (rule.arrays.includes(field) && !Array.isArray(raw)) throw new Error(field + " must be an array");
    if (type === "image" && !rule.arrays.includes(field) && Array.isArray(raw)) throw new Error(field + " must contain one image");
    const references = Array.isArray(raw) ? raw : [raw];
    if (references.length > rule.limit) throw new Error("Image2Pro supports at most " + rule.limit + " reference " + type + "s");
    referenceCount += references.length;
    if (references.length) request[rule.field] = references.map((reference) => mediaReference(reference, type, files));
  }
  if (referenceCount > 15) throw new Error("Image2Pro supports at most 15 total reference files");
  if (!request.prompt && !request.images) throw new Error("Image2Pro requires a prompt or at least one reference image");
  if (source.ratio !== undefined && source.aspect_ratio !== undefined && source.ratio !== source.aspect_ratio)
    throw new Error("ratio and aspect_ratio conflict");
  const ratio = source.ratio === undefined ? source.aspect_ratio : source.ratio;
  if (ratio !== undefined) {
    if (typeof ratio !== "string" || !ratio.trim() || ratio.length > 64) throw new Error("Image2Pro ratio must be a non-empty string");
    for (const character of ratio)
      if (character.charCodeAt(0) < 32 || character.charCodeAt(0) === 127) throw new Error("Image2Pro ratio must not contain control characters");
    request.ratio = ratio;
  }
  const key = clientRequestId(source);
  if (key !== undefined) request.client_request_id = key;
  return request;
}

/** 解码 multipart 文本及三类引用素材；重复字段逐一保留不透明引用，由宿主编码。 */
function multipartRequest(body) {
  const request = {};
  for (const field of Object.keys(body.fields || {})) {
    if (!REQUEST_FIELDS.includes(field)) throw new Error("unsupported Image2Pro field: " + field);
    const values = body.fields[field];
    if (!Array.isArray(values) || !values.length) throw new Error("invalid multipart field: " + field);
    if (Object.values(MEDIA_RULES).some((rule) => rule.arrays.includes(field))) {
      if (values.length === 1 && /^\s*\[/.test(values[0])) {
        try {
          request[field] = JSON.parse(values[0]);
        } catch {
          throw new Error(field + " must contain a valid JSON array or reference URLs");
        }
      } else request[field] = values.slice();
    } else {
      if (values.length !== 1) throw new Error(field + " must be provided once");
      request[field] = values[0];
    }
  }
  const files = body.files || [];
  const fileFields = [];
  const seen = [];
  for (const file of files) {
    if (!Object.values(MEDIA_RULES).some((rule) => rule.aliases.includes(file.field))) throw new Error("unsupported Image2Pro upload field: " + file.field);
    if (typeof file.ref !== "string" || !file.ref || seen.includes(file.ref)) throw new Error("Image2Pro uploaded files require distinct file references");
    seen.push(file.ref);
    if (!fileFields.includes(file.field)) fileFields.push(file.field);
  }
  for (const [type, rule] of Object.entries(MEDIA_RULES)) {
    const mediaFiles = files.filter((file) => rule.aliases.includes(file.field));
    const aliases = rule.aliases.filter((field) => request[field] !== undefined || fileFields.includes(field));
    if (aliases.length > 1) throw new Error("Image2Pro reference " + type + " aliases are mutually exclusive");
    if (!mediaFiles.length) continue;
    const field = aliases[0];
    if (type === "image" && !rule.arrays.includes(field) && (mediaFiles.length !== 1 || request[field] !== undefined))
      throw new Error(field + " must contain one image");
    const references = request[field] === undefined ? [] : Array.isArray(request[field]) ? request[field] : [request[field]];
    for (const file of mediaFiles) {
      if (!Number.isInteger(file.size) || file.size <= 0) throw new Error("Image2Pro uploaded reference files must not be empty");
      let mimeType = typeof file.mimeType === "string" ? file.mimeType.toLowerCase().split(";")[0].trim() : "";
      if (!mimeType || mimeType === "application/octet-stream") {
        const extension = /\.([a-z]+)$/i.exec(file.filename || "");
        const suffix = extension ? extension[1].toLowerCase() : "";
        mimeType = type === "audio" && ["mp4", "webm"].includes(suffix) ? "audio/" + suffix : FILE_MIMES[suffix] || "";
      }
      if (!rule.mimes.includes(mimeType)) throw new Error("Image2Pro upload MIME type must match a supported " + type + " format");
      references.push({ __fileRef: file.ref, encoding: "dataUrl", mimeType });
    }
    delete request[field];
    request[rule.field] = references;
  }
  return request;
}

/** 构造单次 JSON 提交，上传占位符由宿主替换；幂等键与禁止重试避免未知结果重复扣费。 */
export function buildSubmitRequest(ctx) {
  const model = ctx.upstreamModel || ctx.model;
  const body = normalizeRequest(ctx.requestBody, model, ctx.files || []);
  body.model = model;
  const key = body.client_request_id === undefined ? ctx.publicTaskId : body.client_request_id;
  if (typeof key !== "string" || !/^[A-Za-z0-9_.-]{6,120}$/.test(key))
    throw new Error("a valid public task ID or client idempotency key is required for Image2Pro");
  return {
    url: apiBase(ctx) + "/videos",
    method: "POST",
    headers: { Authorization: "Bearer " + ctx.apiKey, "Content-Type": "application/json", "Idempotency-Key": key },
    body,
    noRetry: true,
    action: body.audios || body.videos ? "reference_to_video" : body.images ? "image_to_video" : "text_to_video",
  };
}

/** 校验提交任务 ID 并保存请求秒数；完成响应不含时长，不能从积分推测结算量。 */
export function parseSubmitResponse(ctx, response) {
  const body = response && response.body;
  if (response && response.statusCode !== undefined && (response.statusCode < 200 || response.statusCode >= 300))
    throw new Error("Image2Pro video creation failed");
  if (!body || typeof body !== "object" || Array.isArray(body) || typeof body.id !== "string" || !/^[a-f0-9]{16,80}$/.test(body.id))
    throw new Error("Image2Pro video response must contain a valid hexadecimal task ID");
  if (!["processing", "completed", "failed"].includes(body.status)) throw new Error("Image2Pro returned an unrecognized video status");
  if (body.status !== "failed" && body.error) throw new Error("Image2Pro video creation failed");
  const state = { seconds: requestedSeconds(ctx.requestBody || {}) };
  const result = { taskId: body.id, taskData: body, state };
  if (body.status !== "processing") {
    const immediate = parseTaskResult({ state }, body);
    if (immediate.status === "UNKNOWN") throw new Error("Image2Pro completed video response is invalid");
    result.immediate = immediate;
  }
  return result;
}

/** 返回有界的请求秒数；旧倍率入口不叠加用量或上游积分。 */
export function extractUsage(ctx) {
  const request = normalizeRequest(ctx.requestBody, ctx.upstreamModel || ctx.model, ctx.files || []);
  return ctx.usagePurpose === "billing_ratios" ? {} : { seconds: request.duration };
}

/** 完成时保留已冻结请求秒数，不把缺失实际时长解释为默认值或零用量。 */
export function extractUsageOnComplete(task, result) {
  if (!result || result.status !== "SUCCESS") return null;
  return { seconds: requestedSeconds(task.state || {}) };
}

/** 查询只接受供应商十六进制任务 ID，鉴权固定使用当前渠道。 */
export function buildQueryRequest(ctx) {
  if (typeof ctx.taskId !== "string" || !/^[a-f0-9]{16,80}$/.test(ctx.taskId))
    throw new Error("Image2Pro task ID must contain 16 to 80 hexadecimal characters");
  return { url: apiBase(ctx) + "/videos/" + ctx.taskId, method: "GET", headers: { Authorization: "Bearer " + ctx.apiKey } };
}

/** 映射文档明确状态；完成须有安全视频地址，未知状态不伪装为处理中。 */
export function parseTaskResult(ctx, body) {
  if (!body || typeof body !== "object" || Array.isArray(body)) return { status: "UNKNOWN", reason: "invalid Image2Pro task response" };
  const status = { processing: "IN_PROGRESS", completed: "SUCCESS", failed: "FAILURE" }[body.status];
  if (typeof status !== "string") return { status: "UNKNOWN", reason: "unrecognized Image2Pro task status" };
  const result = { status };
  if (status === "FAILURE") result.reason = "Image2Pro video generation failed";
  else if (status === "SUCCESS") {
    if (!isHTTPURL(body.url)) return { status: "UNKNOWN", reason: "Image2Pro completed task requires an HTTP(S) video URL" };
    requestedSeconds(ctx.state || {});
    result.url = body.url;
    result.progress = "100%";
  } else if (typeof body.progress === "number" && Number.isFinite(body.progress) && body.progress >= 0 && body.progress <= 100)
    result.progress = body.progress + "%";
  return result;
}

/** 任务完成且有合法成片地址时声明视频制品，未完成不发布。 */
export function listArtifacts(task) {
  return task && task.status === "SUCCESS" && isHTTPURL((task.data || {}).url) ? [{ key: "video", type: "video" }] : [];
}

/** 文档只提供成片 URL；下载始终不携带渠道凭据，不猜测上游 /content 接口。 */
export function buildContentRequest(ctx) {
  if (ctx.artifactKey !== "video") throw new Error("artifact_not_found");
  const url = (ctx.data || {}).url;
  if (!isHTTPURL(url)) throw new Error("Image2Pro video URL is unavailable");
  return { url, method: ctx.clientRequest.method, credentialless: true };
}

/** Responses 只展示宿主授权制品链接，HTML 属性转义避免地址注入。 */
function videoOutput(ctx) {
  const artifact = ctx.artifacts && ctx.artifacts.video;
  if (!artifact || typeof artifact.url !== "string" || !artifact.url) throw new Error("Image2Pro video artifact is unavailable");
  const url = artifact.url.replace(/&/g, "&amp;").replace(/"/g, "&quot;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  return '<video controls src="' + url + '"></video>';
}

/** 解码单轮 Responses 文本和图片；音视频使用顶层扩展字段，工具及历史消息拒绝。 */
function responsesRequest(ctx) {
  const source = ctx.body && ctx.body.kind === "json" && ctx.body.value;
  if (!source || typeof source !== "object" || Array.isArray(source)) throw new Error("Image2Pro Responses requires a JSON object");
  for (const key of Object.keys(source))
    if (!["input", "stream", "background"].includes(key) && !REQUEST_FIELDS.includes(key)) throw new Error("unsupported Image2Pro Responses field: " + key);
  const request = {};
  for (const key of REQUEST_FIELDS) if (source[key] !== undefined) request[key] = source[key];
  let text = "";
  const images = [];
  if (typeof source.input === "string") text = source.input;
  else if (Array.isArray(source.input) && source.input.length === 1) {
    const message = source.input[0];
    if (
      !message ||
      typeof message !== "object" ||
      Array.isArray(message) ||
      message.role !== "user" ||
      (message.type !== undefined && message.type !== "message")
    )
      throw new Error("Image2Pro Responses supports one user message");
    for (const key of Object.keys(message))
      if (!["type", "role", "content"].includes(key)) throw new Error("unsupported Image2Pro Responses message field: " + key);
    if (typeof message.content === "string") text = message.content;
    else if (Array.isArray(message.content)) {
      for (const item of message.content) {
        if (!item || typeof item !== "object" || Array.isArray(item)) throw new Error("invalid Image2Pro Responses content");
        if (["input_text", "text"].includes(item.type)) {
          if (Object.keys(item).some((key) => !["type", "text"].includes(key)) || typeof item.text !== "string")
            throw new Error("invalid Image2Pro Responses text");
          text += item.text;
        } else if (["input_image", "image_url"].includes(item.type)) {
          if (Object.keys(item).some((key) => !["type", "image_url"].includes(key))) throw new Error("unsupported Image2Pro Responses image field");
          images.push(mediaReference(item.image_url, "image", []));
        } else throw new Error("Image2Pro Responses supports text and ordinary reference images only");
      }
    } else throw new Error("Image2Pro Responses message content must be text or an array");
  } else throw new Error("Image2Pro Responses requires a text input or one user message");
  if (request.prompt !== undefined && request.prompt !== text) throw new Error("Image2Pro prompt and Responses input conflict");
  request.prompt = text;
  if (images.length) {
    if (IMAGE_FIELDS.some((field) => request[field] !== undefined)) throw new Error("Image2Pro Responses image input conflicts with image aliases");
    request.images = images;
  }
  request.model = ctx.model || source.model;
  return normalizeRequest(request, ctx.upstreamModel || request.model);
}

/** 标准视频与 Responses 共用生成驱动、秒数计费和宿主管理的制品权限。 */
export const protocols = {
  openai_video: {
    /** 接收 JSON 或 multipart 引用素材，统一字段并保留客户端模型别名。 */
    decodeRequest: function (ctx) {
      const body = ctx.body;
      let source;
      if (body && body.kind === "json") source = body.value;
      else if (body && body.kind === "multipart") source = multipartRequest(body);
      else throw new Error("Image2Pro requires a JSON or multipart body");
      const model = ctx.model || (source && source.model);
      const request = normalizeRequest(source, ctx.upstreamModel || model, body.kind === "multipart" ? body.files || [] : []);
      request.model = model;
      return {
        kind: "submit",
        model,
        action: request.audios || request.videos ? "reference_to_video" : request.images ? "image_to_video" : "text_to_video",
        requestBody: request,
      };
    },
    /** 只返回公共任务字段与已校验视频地址，不透传供应商原始错误或其他数据。 */
    render: function (_ctx, task) {
      const statuses = { NOT_START: "queued", SUBMITTED: "queued", QUEUED: "queued", IN_PROGRESS: "in_progress", SUCCESS: "completed", FAILURE: "failed" };
      const output = { id: task.task_id, object: "video", model: (task.properties || {}).origin_model_name || "", status: statuses[task.status] || "unknown" };
      if (task.status === "SUCCESS" && isHTTPURL((task.data || {}).url)) output.url = task.data.url;
      if (task.status === "FAILURE") output.error = { code: "video_generation_failed", message: "Image2Pro video generation failed" };
      return output;
    },
  },
  openai_responses: {
    /** 单轮 input 文本和图片可携带顶层音视频扩展，所需秒数必须显式提供。 */
    decodeRequest: function (ctx) {
      const request = responsesRequest(ctx);
      return {
        kind: "submit",
        model: request.model,
        action: request.audios || request.videos ? "reference_to_video" : request.images ? "image_to_video" : "text_to_video",
        requestBody: request,
      };
    },
    /** 输出变化的进度与唯一终态；错误使用固定摘要，避免泄露上游凭据。 */
    renderEvents: function (ctx, task, previousState) {
      const status = task.status || "UNKNOWN";
      const state = { status, progress: task.progress || "" };
      if (status === "SUCCESS")
        return { events: previousState && previousState.status === status ? [] : [{ type: "output", data: videoOutput(ctx) }], state, done: true };
      if (status === "FAILURE")
        return {
          events:
            previousState && previousState.status === status
              ? []
              : [{ type: "error", code: "video_generation_failed", message: "Image2Pro video generation failed" }],
          state,
          done: true,
        };
      return {
        events:
          previousState && previousState.status === status && previousState.progress === state.progress
            ? []
            : [{ type: "progress", message: status.toLowerCase() }],
        state,
        done: false,
      };
    },
    /** 成功结果引用宿主制品地址，兼容同步与后台 Responses 查询。 */
    renderFinal: function (ctx) {
      return {
        output: [
          {
            type: "message",
            status: "completed",
            role: "assistant",
            content: [{ type: "output_text", text: videoOutput(ctx), annotations: [], logprobs: [] }],
          },
        ],
        metadata: { vendor: "image2pro" },
      };
    },
  },
};
