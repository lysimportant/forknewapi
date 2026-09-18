/** Moon 保留的 Wan 按秒计费模型；参考输入的旧协议尚未核实。 */
const WAN_MODELS = ["wan3.0-video", "wan3.0-video-prime"];
/** Moon 公开文档按 usage.total_tokens 结算的模型。 */
const TOKEN_MODELS = ["doubao-seedance-2-0-mini-260615", "doubao-seedance-2-0-fast-260128", "artsdance-2-0-pro-260801"];
/** 当前接入的完整模型 ID，不使用推测的供应商别名。 */
const MODELS = WAN_MODELS.concat(TOKEN_MODELS);

/** 与百炼同名模型共用价格表达式时，Wan 分辨率事实保持大写。 */
const WAN_RESOLUTIONS = ["480P", "720P", "1080P"];
/** 与豆包同名模型共用的 token 分辨率事实。 */
const TOKEN_RESOLUTIONS = ["480p", "720p", "1080p", "4k"];
/** 新三模型允许的输出比例；Wan 不支持 21:9。 */
const RATIOS = ["21:9", "16:9", "4:3", "1:1", "3:4", "9:16", "adaptive"];
/** 布尔参数只能传 JSON 布尔值，显式 false 必须保留。 */
const TOKEN_BOOLEAN_FIELDS = ["generate_audio", "watermark", "return_last_frame", "web_search", "camera_fixed"];
/** Wan 旧文档不可访问，仅开放已确认的文生视频字段。 */
const WAN_FIELDS = ["model", "prompt", "resolution", "ratio", "aspect_ratio", "duration", "seconds"];
/** 新三模型公开字段；其他参数必须拒绝，不能静默丢弃。 */
const TOKEN_FIELDS = [
  "model",
  "prompt",
  "content",
  "images",
  "videos",
  "audios",
  "first_frame",
  "last_frame",
  "omni_reference_task_type",
  "resolution",
  "ratio",
  "aspect_ratio",
  "duration",
  "seconds",
  "output_format",
  "seed",
].concat(TOKEN_BOOLEAN_FIELDS);

/** Moon 新三模型的 token 计费事实；tokens 是用量数量，不是价格。 */
const TOKEN_USAGE_SCHEMA = {
  tokens: {
    type: "number",
    unit: "token",
    description: { en: "Video token unit price", zh: "视频 Token 单价" },
  },
  resolution: {
    enum: TOKEN_RESOLUTIONS,
    enumLabels: {
      "480p": { en: "480p", zh: "480p" },
      "720p": { en: "720p", zh: "720p" },
      "1080p": { en: "1080p", zh: "1080p" },
      "4k": { en: "4k", zh: "4k" },
    },
    description: { en: "Output video resolution", zh: "输出视频分辨率" },
  },
  video_input: {
    enum: ["none", "video"],
    enumLabels: {
      none: { en: "No reference video", zh: "无参考视频" },
      video: { en: "With reference video", zh: "有参考视频" },
    },
    description: { en: "Reference video input", zh: "参考视频输入" },
  },
};

/** Wan 文生视频按请求输出秒数计费；不套用未经确认的阿里云完成用量字段。 */
const WAN_USAGE_SCHEMA = {
  seconds: {
    type: "number",
    unit: "second",
    description: { en: "Video generation unit price", zh: "视频生成单价" },
  },
  resolution: {
    enum: WAN_RESOLUTIONS,
    enumLabels: {
      "480P": { en: "480P", zh: "480P" },
      "720P": { en: "720P", zh: "720P" },
      "1080P": { en: "1080P", zh: "1080P" },
    },
    description: { en: "Output video resolution", zh: "输出视频分辨率" },
  },
};

/** Moon 插件元信息；价格沿用管理员配置，不内置人民币到美元的换算。 */
export const meta = {
  apiVersion: 1,
  key: "moon",
  name: "Moon",
  icon: "text:Moon",
  baseUrl: "https://moon.sixai.cc",
  description: {
    en: "Moon video generation for Wan, Seedance, and ArtsDance models",
    zh: "Moon Wan、Seedance 与 ArtsDance 视频生成",
  },
  version: "1.0.0",
  author: { name: "QuantumNous" },
  models: MODELS,
  fetchMode: "per_task",
  requiredCapabilities: ["task-submit-no-retry@1"],
  usageSchema: TOKEN_USAGE_SCHEMA,
  usageProfiles: [{ models: WAN_MODELS, schema: WAN_USAGE_SCHEMA }],
  usageExamples: [
    { label: "5s · 720p", facts: { tokens: 108000, resolution: "720p", video_input: "none" } },
    { label: "5s · 1080p", facts: { tokens: 243000, resolution: "1080p", video_input: "none" } },
    { label: "5s · 4k", facts: { tokens: 972000, resolution: "4k", video_input: "none" } },
  ],
  protocols: ["openai_video", { name: "openai_responses", supports: ["stream", "sync", "background"] }],
};

/** 读取字符串标识；非字符串按缺失处理，避免把对象拼接到 URL 或模型 ID。 */
function trimmed(value) {
  return typeof value === "string" ? value.trim() : "";
}

/** 驱动和计费使用映射后的模型；协议解码仍保留客户端模型名。 */
function modelName(ctx, request) {
  return trimmed((ctx && (ctx.upstreamModel || ctx.model)) || (request && request.model));
}

/** 判断是否走 Wan 的已确认文生视频合同。 */
function isWan(model) {
  return WAN_MODELS.includes(model);
}

/** 判断是否必须等待真实 token 用量后才能完成任务。 */
function isTokenModel(model) {
  return TOKEN_MODELS.includes(model);
}

/** 渠道根地址和带 /v1 的地址均只追加一次版本路径。 */
function apiBase(ctx) {
  const base = trimmed(ctx && ctx.baseUrl).replace(/\/+$/, "");
  if (!isHTTPURL(base) || /[?#]/.test(base)) throw new Error("channel base URL must be an HTTP(S) URL without credentials, query, or fragment");
  if (base.endsWith("/v1")) return base;
  return base + "/v1";
}

/** 每个网关任务固定一个可见 ASCII 幂等键；没有公有任务 ID 时拒绝提交。 */
function idempotencyKey(ctx) {
  const key = trimmed(ctx && ctx.publicTaskId);
  if (!key || key.length > 128 || /[^\x21-\x7e]/.test(key)) throw new Error("public task id is required for Moon idempotency");
  return key;
}

/** 仅接受无内嵌凭据的绝对 HTTP(S) URL，不支持上传、base64 或 file_id。 */
function isHTTPURL(value) {
  if (typeof value !== "string" || /[\s\\]/.test(value)) return false;
  for (const character of value) if (character.charCodeAt(0) < 32 || character.charCodeAt(0) === 127) return false;
  return /^https?:\/\/(?:\[[0-9a-f:.]+\]|[a-z0-9.-]+)(?::[0-9]+)?(?:[/?#].*)?$/i.test(value);
}

/** 验证 JSON 布尔参数，拒绝会改变语义的字符串或数值转换。 */
function ensureBoolean(value, field) {
  if (typeof value !== "boolean") throw new Error(field + " must be true or false");
}

/** seconds 兼容十进制整数字符串；拒绝空值、指数或十六进制写法。 */
function numericDuration(value, field) {
  if (typeof value !== "number" && typeof value !== "string") throw new Error(field + " must be a number");
  if (typeof value === "string" && !/^-?\d+$/.test(value.trim())) throw new Error(field + " must be an integer");
  const number = Number(value);
  if (!Number.isFinite(number) || !Number.isInteger(number)) throw new Error(field + " must be an integer");
  return number;
}

/** 统一 duration/seconds，检查各模型上限，默认 5 秒；新三模型 -1 表示自动时长。 */
function normalizeDuration(req, model) {
  const hasDuration = Object.prototype.hasOwnProperty.call(req, "duration");
  const hasSeconds = Object.prototype.hasOwnProperty.call(req, "seconds");
  if (Object.prototype.hasOwnProperty.call(req, "auto_duration")) {
    if (isWan(model) || req.auto_duration !== true || hasDuration || hasSeconds) throw new Error("invalid internal auto duration marker");
    return -1;
  }
  if (hasDuration && hasSeconds && numericDuration(req.duration, "duration") !== numericDuration(req.seconds, "seconds"))
    throw new Error("duration and seconds conflict");
  const raw = hasDuration ? req.duration : hasSeconds ? req.seconds : 5;
  const duration = numericDuration(raw, hasDuration ? "duration" : "seconds");
  if (isWan(model)) {
    if (duration < 2 || duration > 30) throw new Error("duration must be an integer between 2 and 30 for Moon Wan models");
  } else if (duration !== -1 && (duration < 4 || duration > 15)) {
    throw new Error("duration must be -1 or an integer between 4 and 15 for Moon video models");
  }
  return duration;
}

/** 输出计费使用规范分辨率值；Mini/Fast 不允许 Pro 的高分辨率。 */
function normalizeResolution(value, model) {
  const raw = trimmed(value).toLowerCase();
  if (isWan(model)) {
    const normalized = raw.toUpperCase();
    if (!WAN_RESOLUTIONS.includes(normalized)) throw new Error("resolution must be 480p, 720p, or 1080p for Moon Wan models");
    return normalized;
  }
  if (!TOKEN_RESOLUTIONS.includes(raw)) throw new Error("resolution is not supported by the Moon model");
  if ((model === "doubao-seedance-2-0-mini-260615" || model === "doubao-seedance-2-0-fast-260128") && !["480p", "720p"].includes(raw))
    throw new Error("this Moon model supports only 480p or 720p");
  return raw;
}

/** 合并比例兼容别名并拒绝冲突；Wan 只接受其已确认比例。 */
function normalizeRatio(req, model) {
  const hasRatio = Object.prototype.hasOwnProperty.call(req, "ratio");
  const hasAspectRatio = Object.prototype.hasOwnProperty.call(req, "aspect_ratio");
  if (hasRatio && hasAspectRatio && trimmed(req.ratio) !== trimmed(req.aspect_ratio)) throw new Error("ratio and aspect_ratio conflict");
  const value = hasRatio ? req.ratio : hasAspectRatio ? req.aspect_ratio : "16:9";
  const ratio = trimmed(value);
  if (!RATIOS.includes(ratio)) throw new Error("ratio must be one of the documented Moon video ratios");
  if (isWan(model) && ratio === "21:9") throw new Error("21:9 ratio is not supported by Moon Wan models");
  return ratio;
}

/** 校验引用数组的 URL 与固定角色，未知子字段（含客户端自报时长）不得透传。 */
function validateReferenceList(value, field, role, maxCount) {
  if (value === undefined) return 0;
  if (!Array.isArray(value) || value.length > maxCount) throw new Error(field + " must contain at most " + maxCount + " references");
  for (const item of value) {
    if (!item || typeof item !== "object" || Array.isArray(item) || !isHTTPURL(item.url) || item.role !== role)
      throw new Error(field + " items must contain an HTTP(S) url and role " + role);
    for (const key of Object.keys(item)) if (key !== "url" && key !== "role") throw new Error("unsupported " + field + " reference field: " + key);
  }
  return value.length;
}

/** 校验两种素材表达并合并计数；参考视频不下载探测，按每段上限 15 秒预留。 */
function validateTokenReferences(req, duration, ratio) {
  let imageCount = validateReferenceList(req.images, "images", "reference_image", 9);
  let videoCount = validateReferenceList(req.videos, "videos", "reference_video", 3);
  let audioCount = validateReferenceList(req.audios, "audios", "reference_audio", 3);
  let text = req.prompt || "";
  for (const field of ["first_frame", "last_frame"]) {
    if (req[field] === undefined) continue;
    if (!isHTTPURL(req[field])) throw new Error(field + " must be an HTTP(S) URL");
    imageCount++;
  }
  if (req.content !== undefined) {
    if (!Array.isArray(req.content) || req.content.length === 0) throw new Error("content must be a non-empty array");
    for (const item of req.content) {
      if (!item || typeof item !== "object" || Array.isArray(item)) throw new Error("content items must be objects");
      if (item.type === "text") {
        if (typeof item.text !== "string" || !item.text.trim()) throw new Error("content text must be a non-empty string");
        for (const key of Object.keys(item)) if (key !== "type" && key !== "text") throw new Error("unsupported content text field: " + key);
        text += item.text;
        continue;
      }
      if (!["image_url", "video_url", "audio_url"].includes(item.type)) throw new Error("unsupported content item type");
      const reference = item[item.type];
      if (!reference || typeof reference !== "object" || Array.isArray(reference) || !isHTTPURL(reference.url))
        throw new Error("content " + item.type + " must contain an HTTP(S) url");
      for (const key of Object.keys(reference)) if (key !== "url") throw new Error("unsupported content URL field: " + key);
      for (const key of Object.keys(item))
        if (key !== "type" && key !== item.type && key !== "role") throw new Error("unsupported content media field: " + key);
      if (item.type === "image_url") {
        if (item.role !== undefined && !["first_frame", "last_frame", "reference_image"].includes(item.role)) throw new Error("unsupported content image role");
        imageCount++;
      } else if (item.type === "video_url") {
        if (item.role !== "reference_video") throw new Error("content video role must be reference_video");
        videoCount++;
      } else {
        if (item.role !== "reference_audio") throw new Error("content audio role must be reference_audio");
        audioCount++;
      }
    }
  }
  if (imageCount > 9 || videoCount > 3 || audioCount > 3 || imageCount + videoCount + audioCount > 15)
    throw new Error("Moon supports at most 9 images, 3 videos, 3 audios, and 15 total references");
  if (audioCount > 0 && imageCount === 0 && videoCount === 0) throw new Error("audio-only references are not supported");
  if (text.length > 20000) throw new Error("prompt and content text must not exceed 20000 characters");
  if (/--(?:duration|resolution)\b/i.test(text)) throw new Error("inline duration and resolution overrides are not supported");
  if (req.omni_reference_task_type !== undefined && !["auto", "edit", "extend"].includes(req.omni_reference_task_type))
    throw new Error("omni_reference_task_type must be auto, edit, or extend");
  if ((req.omni_reference_task_type === "edit" || req.omni_reference_task_type === "extend") && videoCount === 0)
    throw new Error("edit and extend require a reference video");
  if ((req.omni_reference_task_type === "edit" || req.omni_reference_task_type === "extend") && ratio !== "adaptive")
    throw new Error("edit and extend require adaptive ratio");
  if (req.omni_reference_task_type === "edit" && duration !== -1) throw new Error("edit requires duration -1");
  return { videoCount, hasReference: imageCount + videoCount + audioCount > 0 };
}

/** 校验并返回规范计费事实；allowAutoDuration 仅供驱动读取内部标记，协议入口必须省略。 */
function validateKnownFields(req, model, allowAutoDuration = false) {
  if (!req || typeof req !== "object" || Array.isArray(req)) throw new Error("request body must be an object");
  const allowed = isWan(model) ? WAN_FIELDS : TOKEN_FIELDS;
  for (const key of Object.keys(req)) {
    if (allowAutoDuration && key === "auto_duration" && !isWan(model)) continue;
    if (!allowed.includes(key)) throw new Error("unsupported Moon request field: " + key);
  }
  if (typeof req.prompt !== "undefined" && (typeof req.prompt !== "string" || !req.prompt.trim())) throw new Error("prompt must be a non-empty string");
  const hasContent = req.content !== undefined;
  if (hasContent && (req.prompt !== undefined || req.images !== undefined || req.videos !== undefined || req.audios !== undefined))
    throw new Error("content cannot be combined with prompt or reference arrays");
  const hasPrompt = typeof req.prompt === "string" && req.prompt.trim() !== "";
  if (!hasPrompt && !hasContent) throw new Error("prompt or content is required");
  const duration = normalizeDuration(req, model);
  const resolution = normalizeResolution(req.resolution === undefined ? (isWan(model) ? "720P" : "720p") : req.resolution, model);
  const ratio = normalizeRatio(req, model);
  if (isWan(model)) {
    if (req.prompt.length > 20000) throw new Error("prompt must not exceed 20000 characters");
    if (/--(?:duration|resolution)\b/i.test(req.prompt)) throw new Error("inline duration and resolution overrides are not supported");
    return { duration, resolution, ratio, videoInput: "none", videoCount: 0, hasReference: false };
  }
  const references = validateTokenReferences(req, duration, ratio);
  for (const field of TOKEN_BOOLEAN_FIELDS) if (req[field] !== undefined) ensureBoolean(req[field], field);
  if (req.output_format !== undefined && !["mp4", "mov"].includes(req.output_format)) throw new Error("output_format must be mp4 or mov");
  if (req.seed !== undefined && (!Number.isInteger(req.seed) || req.seed < -1 || req.seed > 2147483647))
    throw new Error("seed must be between -1 and 2147483647");
  return { duration, resolution, ratio, videoInput: references.videoCount > 0 ? "video" : "none", ...references };
}

/** 将解码器新建且已校验的请求副本转换为宿主格式，返回同一副本；不向宿主暴露负数用量。 */
function normalizeDecodedRequest(request, facts) {
  // 宿主在用量钩子前校验同名枚举与非负时长；驱动提交时才恢复供应商的 -1 哨兵值。
  if (request.resolution !== undefined) request.resolution = facts.resolution;
  if (facts.duration === -1) {
    request.auto_duration = true;
    delete request.duration;
    delete request.seconds;
  }
  return request;
}

/** 生成规范上游 JSON；仅规范已验证别名，不修改宿主传入对象。 */
function normalizeRequest(req, model) {
  const values = Object.assign({}, req || {});
  delete values.auto_duration;
  delete values.seconds;
  delete values.aspect_ratio;
  values.duration = normalizeDuration(req, model);
  values.resolution = normalizeResolution(req.resolution === undefined ? "720p" : req.resolution, model).toLowerCase();
  values.ratio = normalizeRatio(req, model);
  values.model = model;
  if (values.content !== undefined) values.content = values.content.map((item) => Object.assign({}, item));
  return values;
}

/** 把 Responses 文本和 URL 图片转成 Moon 输入；工具调用等内容不能静默跳过。 */
function responsesInput(req) {
  const result = { prompt: "", images: [] };
  if (typeof req.input === "string") result.prompt = req.input.trim();
  else if (Array.isArray(req.input)) {
    const texts = [];
    for (const item of req.input) {
      if (typeof item === "string") {
        texts.push(item);
        continue;
      }
      if (!item || typeof item !== "object" || Array.isArray(item)) throw new Error("input items must be strings or objects");
      if (item.content !== undefined) {
        for (const key of Object.keys(item)) if (!["type", "role", "content"].includes(key)) throw new Error("unsupported Responses message field: " + key);
        if (item.type !== undefined && item.type !== "message") throw new Error("unsupported Responses message type");
        if (item.role !== undefined && item.role !== "user") throw new Error("Moon video input messages must use the user role");
      }
      const parts = item.content === undefined ? [item] : Array.isArray(item.content) ? item.content : [item.content];
      for (const part of parts) {
        if (typeof part === "string") texts.push(part);
        else if (part && typeof part === "object" && !Array.isArray(part)) {
          if (["input_text", "text"].includes(part.type) && typeof part.text === "string") {
            for (const key of Object.keys(part)) if (key !== "type" && key !== "text") throw new Error("unsupported Responses text field: " + key);
            texts.push(part.text);
          } else if (["input_image", "image_url"].includes(part.type)) {
            for (const key of Object.keys(part)) if (key !== "type" && key !== "image_url") throw new Error("unsupported Responses image field: " + key);
            if (part.image_url && typeof part.image_url === "object") {
              for (const key of Object.keys(part.image_url)) if (key !== "url") throw new Error("unsupported Responses image URL field: " + key);
            }
            const url = part.image_url && typeof part.image_url === "object" ? part.image_url.url : part.image_url;
            if (!isHTTPURL(url)) throw new Error("Responses image input must be an HTTP(S) URL");
            result.images.push({ url: url, role: "reference_image" });
          } else throw new Error("unsupported Responses input item");
        } else throw new Error("invalid Responses input item");
      }
    }
    result.prompt = texts.join("\n").trim();
  } else if (req.input !== undefined) throw new Error("input must be a string or array");
  return result;
}

/** 解码 Responses；stream/background 交给宿主管理，其余未支持字段必须拒绝。 */
function requestFromResponses(ctx) {
  if (!ctx.body || ctx.body.kind !== "json") throw new Error("JSON body required");
  const source = ctx.body.value;
  if (!source || typeof source !== "object" || Array.isArray(source)) throw new Error("request body must be an object");
  const model = trimmed(ctx.model || source.model);
  if (!MODELS.includes(trimmed(ctx.upstreamModel || model))) throw new Error("unsupported Moon model");
  for (const key of Object.keys(source)) {
    if (!TOKEN_FIELDS.includes(key) && !["input", "stream", "background"].includes(key)) throw new Error("unsupported Moon Responses field: " + key);
  }
  if (source.stream !== undefined) ensureBoolean(source.stream, "stream");
  if (source.background !== undefined) ensureBoolean(source.background, "background");
  if (source.input !== undefined && (source.prompt !== undefined || source.content !== undefined))
    throw new Error("input cannot be combined with prompt or content");
  const input = responsesInput(source);
  const request = { model: model };
  if (input.prompt) request.prompt = input.prompt;
  for (const key of TOKEN_FIELDS) if (key !== "model" && source[key] !== undefined) request[key] = source[key];
  if (input.images.length) {
    if (request.images !== undefined && !Array.isArray(request.images)) throw new Error("images must be an array");
    request.images = input.images.concat(request.images || []);
  }
  const facts = validateKnownFields(request, trimmed(ctx.upstreamModel || model));
  return {
    kind: "submit",
    model: model,
    action: facts.hasReference ? "reference_to_video" : "text_to_video",
    requestBody: normalizeDecodedRequest(request, facts),
  };
}

/** Responses 仅展示宿主制品 URL，避免在输出中泄露供应商地址与凭据。 */
function videoOutput(ctx) {
  const artifact = ctx && ctx.artifacts && ctx.artifacts.video;
  const url = trimmed(artifact && artifact.url);
  if (!url) throw new Error("video artifact is unavailable");
  const escaped = url.replace(/&/g, "&amp;").replace(/"/g, "&quot;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  return '<video controls src="' + escaped + '"></video>';
}

/** 兼容旧任务的 data 对象封装；新三模型的 data 数组保留在根响应中。 */
function taskPayload(body) {
  if (!body || typeof body !== "object" || Array.isArray(body)) return {};
  return body.status === undefined && body.data && typeof body.data === "object" && !Array.isArray(body.data) ? body.data : body;
}

/** 完成用量必须是宿主 token 上限内的非负整数；0 有效，缺失或异常返回 null。 */
function actualTokens(body) {
  const usage = body && body.usage;
  const tokens = usage && typeof usage === "object" && !Array.isArray(usage) ? usage.total_tokens : undefined;
  return typeof tokens === "number" && Number.isInteger(tokens) && tokens >= 0 && tokens <= 2147483647 ? tokens : null;
}

/** 从上游完成响应提取无内嵌凭据的原始成片 URL。 */
function artifactURL(data) {
  const body = taskPayload(data);
  const candidates = [];
  if (Array.isArray(body.data)) for (const item of body.data) if (item && typeof item === "object") candidates.push(item.url);
  candidates.push(body.video_url, body.url, body.output && body.output.video_url);
  for (const candidate of candidates) if (isHTTPURL(candidate)) return candidate;
  return "";
}

/** 构造一次性 JSON 提交描述；字段或 256 KiB 请求上限不合法时在发送前抛错。 */
export function buildSubmitRequest(ctx) {
  const request = ctx.requestBody || {};
  const model = modelName(ctx, request);
  if (!MODELS.includes(model)) throw new Error("unsupported Moon model");
  const facts = validateKnownFields(request, model, true);
  const normalized = normalizeRequest(request, model);
  let bodyBytes = 0;
  for (const character of JSON.stringify(normalized)) {
    const point = character.codePointAt(0);
    bodyBytes += point < 128 ? 1 : point < 2048 ? 2 : point < 65536 ? 3 : 4;
  }
  if (bodyBytes > 262144) throw new Error("Moon JSON request must not exceed 256 KiB");
  const headers = {
    Authorization: "Bearer " + ctx.apiKey,
    "Content-Type": "application/json",
    "Idempotency-Key": idempotencyKey(ctx),
  };
  return {
    url: apiBase(ctx) + "/videos",
    method: "POST",
    headers: headers,
    body: normalized,
    noRetry: true,
    action: ctx.action || (facts.hasReference ? "reference_to_video" : "text_to_video"),
  };
}

/** 解析 202/200 创建响应；缺少任务 ID 或明确错误时拒绝持久化。 */
export function parseSubmitResponse(_ctx, response) {
  const body = response && response.body;
  if (!body || typeof body !== "object" || Array.isArray(body)) throw new Error("Moon video response must be a JSON object");
  if (response.statusCode !== undefined && (response.statusCode < 200 || response.statusCode >= 300)) throw new Error("Moon video creation failed");
  if (body.error) throw new Error(typeof body.error === "string" ? body.error : body.error.message || "Moon video creation failed");
  const nested = body.data && !Array.isArray(body.data) && typeof body.data === "object" ? body.data : {};
  const taskId = trimmed(body.id || body.task_id || nested.id || nested.task_id);
  if (!taskId) throw new Error("Moon video task id is missing");
  return { taskId: taskId, taskData: body };
}

/** 预留使用对应分辨率的 16:9 像素上限，与 Moon 保守估算保持一致。 */
function resolutionPixels(resolution) {
  if (resolution === "480p") return [854, 480];
  if (resolution === "1080p") return [1920, 1080];
  if (resolution === "4k") return [3840, 2160];
  return [1280, 720];
}

/** 按 24 × 宽 × 高 / 1024 估算 tokens；自动输出和每段参考视频均按 15 秒预留。 */
function estimateTokens(duration, resolution, videoInputCount) {
  const seconds = duration === -1 ? 15 : duration;
  const totalSeconds = seconds + videoInputCount * 15;
  const pixels = resolutionPixels(resolution);
  return (totalSeconds * pixels[0] * pixels[1] * 24) / 1024;
}

/** 返回宿主用量事实；billing_ratios 不伪造 Moon 的币值或倍率。 */
export function extractUsage(ctx) {
  const request = ctx.requestBody || {};
  const model = modelName(ctx, request);
  if (!MODELS.includes(model)) throw new Error("unsupported Moon model");
  if (ctx.usagePurpose === "billing_ratios") return null;
  const facts = validateKnownFields(request, model, true);
  if (isWan(model)) return { seconds: facts.duration, resolution: facts.resolution };
  return { tokens: estimateTokens(facts.duration, facts.resolution, facts.videoCount), resolution: facts.resolution, video_input: facts.videoInput };
}

/** 成功任务以真实 tokens 覆盖预留；Wan 完成用量未公开，保留已验证的请求秒数。 */
export function extractUsageOnComplete(task, result, body) {
  if (!result || result.status !== "SUCCESS" || !body || typeof body !== "object") return null;
  const source = taskPayload(body);
  const model = modelName(task, source);
  if (!isTokenModel(model)) return null;
  const tokens = actualTokens(source);
  if (tokens === null) throw new Error("Moon completion requires valid usage.total_tokens");
  return { tokens: tokens };
}

/** 构造轮询请求；任务 ID 只作为路径参数编码，不拼接用户字段。 */
export function buildQueryRequest(ctx) {
  const taskId = trimmed(ctx && ctx.taskId);
  if (!taskId) throw new Error("Moon task id is required");
  return { url: apiBase(ctx) + "/videos/" + encodeURIComponent(taskId), method: "GET", headers: { Authorization: "Bearer " + ctx.apiKey } };
}

/** 映射 Moon 已知状态；未知值严格返回 UNKNOWN。 */
export function parseTaskResult(ctx, body) {
  if (!body || typeof body !== "object" || Array.isArray(body)) return { status: "UNKNOWN", reason: "invalid Moon task response" };
  const source = taskPayload(body);
  const raw = trimmed(source.status).toLowerCase();
  const statusMap = {
    queued: "QUEUED",
    pending: "QUEUED",
    submitting: "IN_PROGRESS",
    submitting_unknown: "IN_PROGRESS",
    processing: "IN_PROGRESS",
    running: "IN_PROGRESS",
    in_progress: "IN_PROGRESS",
    usage_pending: "IN_PROGRESS",
    commit_pending: "IN_PROGRESS",
    completed: "SUCCESS",
    succeeded: "SUCCESS",
    success: "SUCCESS",
    failed: "FAILURE",
    canceled: "FAILURE",
    cancelled: "FAILURE",
    expired: "FAILURE",
  };
  const status = statusMap[raw];
  if (typeof status !== "string") return { status: "UNKNOWN", reason: "unrecognized Moon task status: " + raw };
  const result = { status: status };
  if (status === "SUCCESS") {
    // 宿主会忽略完成用量钩子的错误；必须在状态边界阻止按估算误结算。
    if (isTokenModel(modelName(ctx, source)) && actualTokens(source) === null)
      return { status: "IN_PROGRESS", reason: "waiting for valid Moon usage.total_tokens" };
    result.progress = "100%";
    const url = artifactURL(source);
    if (url) result.url = url;
  } else if (status === "FAILURE") {
    result.reason =
      source.error && typeof source.error === "object"
        ? source.error.message || "Moon video generation failed"
        : source.error || "Moon video generation failed";
  } else if (typeof source.progress === "number" && Number.isFinite(source.progress) && source.progress >= 0 && source.progress <= 100) {
    result.progress = source.progress + "%";
  }
  return result;
}

/** 只有任务成功后才发布视频制品；实际下载地址由宿主代理控制。 */
export function listArtifacts(task) {
  if (!task || task.status !== "SUCCESS") return [];
  const source = taskPayload(task.data);
  const mimeType = source.output_format === "mov" || /\.mov(?:[?#]|$)/i.test(artifactURL(source)) ? "video/quicktime" : "video/mp4";
  return [{ key: "video", type: "video", mimeType: mimeType }];
}

/** 优先使用完成响应中的原始 URL，并以 credentialless 请求避免向第三方泄露 Moon 密钥。 */
export function buildContentRequest(ctx) {
  if (!ctx || ctx.artifactKey !== "video") throw new Error("artifact_not_found");
  const direct = artifactURL(ctx.data);
  if (direct) return { url: direct, method: ctx.clientRequest.method, credentialless: true };
  const taskId = trimmed(ctx.upstreamTaskId);
  if (!taskId) throw new Error("Moon task id is required");
  return {
    url: apiBase(ctx) + "/videos/" + encodeURIComponent(taskId) + "/content",
    method: ctx.clientRequest.method,
    headers: { Authorization: "Bearer " + ctx.apiKey },
  };
}

/** 将宿主持久化状态映射到 OpenAI Video 展示状态，不信任上游提前完成标记。 */
function renderStatus(task) {
  const statuses = { NOT_START: "queued", SUBMITTED: "queued", QUEUED: "queued", IN_PROGRESS: "in_progress", SUCCESS: "completed", FAILURE: "failed" };
  return statuses[task.status] || "unknown";
}

/** 标准视频和 Responses 协议入口；异步、同步与流式展示共用一个任务。 */
export const protocols = {
  openai_video: {
    /** OpenAI Video 入口只接受 JSON；Moon 文件上传合同尚未公开，因此不接收 multipart。 */
    decodeRequest: function (ctx) {
      if (!ctx.body || ctx.body.kind !== "json") throw new Error("JSON body required");
      const source = ctx.body.value;
      if (!source || typeof source !== "object" || Array.isArray(source)) throw new Error("request body must be an object");
      const model = trimmed(ctx.model || source.model);
      if (!MODELS.includes(trimmed(ctx.upstreamModel || model))) throw new Error("unsupported Moon model");
      const request = Object.assign({}, source, { model: model });
      const facts = validateKnownFields(request, trimmed(ctx.upstreamModel || model));
      return {
        kind: "submit",
        model: model,
        action: facts.hasReference ? "reference_to_video" : "text_to_video",
        requestBody: normalizeDecodedRequest(request, facts),
      };
    },
    /** 展示网关任务 ID 和状态，供应商任务 ID 不直接返回客户端。 */
    render: function (_ctx, task) {
      const output = {
        id: task.task_id,
        object: "video",
        model: task.properties ? task.properties.origin_model_name || "" : "",
        status: renderStatus(task),
        progress: Number(String(task.progress || "0").replace("%", "")),
        created_at: task.created_at,
      };
      if ((task.status === "SUCCESS" || task.status === "FAILURE") && task.updated_at) output.completed_at = task.updated_at;
      if (task.status === "FAILURE") output.error = { code: "video_generation_failed", message: task.fail_reason || "Moon video generation failed" };
      return output;
    },
  },
  openai_responses: {
    decodeRequest: requestFromResponses,
    /** 状态变化时输出语义事件；成功只发布一次宿主代理的视频链接。 */
    renderEvents: function (ctx, task, previousState) {
      const status = String(task.status || "UNKNOWN").toUpperCase();
      const progressValue = Number(String(task.progress || "").replace("%", ""));
      const progress = Number.isFinite(progressValue) && progressValue >= 0 && progressValue <= 100 ? progressValue : null;
      const state = { status: status, progress: progress };
      if (status === "SUCCESS")
        return { events: previousState && previousState.status === status ? [] : [{ type: "output", data: videoOutput(ctx) }], state: state, done: true };
      if (status === "FAILURE")
        return { events: [{ type: "error", code: "task_failed", message: task.fail_reason || "Moon video generation failed" }], state: state, done: true };
      if (previousState && previousState.status === status && previousState.progress === progress) return { events: [], state: state, done: false };
      const event = { type: "progress", message: status.toLowerCase() };
      if (progress !== null) event.progress = progress;
      return { events: [event], state: state, done: false };
    },
    /** 同步完成和后台任务查询使用相同的 Responses 消息结构。 */
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
        metadata: { vendor: "moon" },
      };
    },
  },
};
