/** Canvas 和既有渠道使用展示名；供应商创建接口必须发送目录中的真实模型 ID。 */
const SEEDANCE_MODEL = "Seedance2.0 0.9r";
const FLASH_MODEL = "无限制-Flash-MAX-Video";
/** 保留既有展示名路由，并允许模型目录导入真实 ID；两组名称最终发送同一真实 ID。 */
const MODELS = [SEEDANCE_MODEL, FLASH_MODEL, "Seedance2.0", "Flash视频-MAX"];
const MODEL_SPECS = {
  [SEEDANCE_MODEL]: { upstream: "Seedance2.0", durationMax: 15, imageLimit: 9 },
  "Seedance2.0": { upstream: "Seedance2.0", durationMax: 15, imageLimit: 9 },
  [FLASH_MODEL]: { upstream: "Flash视频-MAX", durationMax: 12, imageLimit: 5 },
  "Flash视频-MAX": { upstream: "Flash视频-MAX", durationMax: 12, imageLimit: 5 },
};
/** 兼容的参考图入口；内部先归一为 content，供应商出站再转换为公开视频字段。 */
const IMAGE_FIELDS = ["input_image", "images", "input_reference", "image", "reference_images", "image_urls"];
/** 兼容识别历史媒体入口；公开创建接口仅允许图片，音视频会在归一阶段拒绝。 */
const MEDIA_RULES = {
  image: {
    field: "images",
    aliases: IMAGE_FIELDS,
    arrays: ["images", "reference_images", "image_urls"],
    limit: 9,
    mimes: ["image/png", "image/jpeg", "image/webp", "image/gif", "image/bmp", "image/tiff", "image/heic", "image/heif"],
  },
  audio: {
    field: "audios",
    aliases: ["audios", "reference_audios", "audio_urls", "input_audio", "reference_audio", "audio"],
    arrays: ["audios", "reference_audios", "audio_urls"],
    limit: 3,
    mimes: ["audio/mpeg", "audio/mp3", "audio/wav", "audio/x-wav"],
  },
  video: {
    field: "videos",
    aliases: ["videos", "reference_videos", "video_urls", "input_video", "reference_video", "video"],
    arrays: ["videos", "reference_videos", "video_urls"],
    limit: 3,
    mimes: ["video/mp4", "video/quicktime"],
  },
};
/** 无 MIME 的上传由扩展名补齐；显式错误的 Content-Type 不凭扩展名覆盖。 */
const FILE_MIMES = {
  png: "image/png",
  jpg: "image/jpeg",
  jpeg: "image/jpeg",
  webp: "image/webp",
  gif: "image/gif",
  bmp: "image/bmp",
  tif: "image/tiff",
  tiff: "image/tiff",
  heic: "image/heic",
  heif: "image/heif",
  mp3: "audio/mpeg",
  wav: "audio/wav",
  mp4: "video/mp4",
};
/** 兼容识别旧请求字段；Image2Pro 公开视频接口未声明这些参数，提交前明确拒绝。 */
const BOOLEAN_FIELDS = ["generate_audio", "watermark", "return_last_frame"];
/** 官方参数与旧入口共用白名单，未适配高级任务不得透传。 */
const REQUEST_FIELDS = [
  "model",
  "content",
  "prompt",
  "seconds",
  "duration",
  "resolution",
  "ratio",
  "aspect_ratio",
  "client_request_id",
  "clientRequestId",
].concat(
  BOOLEAN_FIELDS,
  Object.values(MEDIA_RULES).flatMap((rule) => rule.aliases)
);

/** Image2Pro 插件声明；秒数是用量，价格由管理员配置，不换算上游积分。 */
export const meta = {
  apiVersion: 1,
  key: "image2pro",
  name: "Image2Pro",
  icon: "text:I2P",
  version: "2.2.0",
  author: { name: "lysimportant/forknewapi" },
  description: {
    en: "Image2Pro Seedance 2.0 and Flash-MAX text and reference-image video generation",
    zh: "Image2Pro Seedance 2.0 与 Flash-MAX 文本及参考图视频生成",
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

/** 读取有界秒数并兼容旧任务冻结的小数时长；新建请求另按精确模型范围校验。 */
function requestedSeconds(request) {
  if (request.seconds === undefined && request.duration === undefined) throw new Error("seconds or duration is required for Image2Pro per-second billing");
  for (const field of ["seconds", "duration"]) {
    const value = request[field];
    if (value === undefined) continue;
    if (value === -1 || value === "-1")
      throw new Error("automatic duration is unavailable because Image2Pro has no verified output duration for per-second billing");
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

/** 返回已适配模型的公开目录合同，同时兼容渠道已映射的真实模型 ID。 */
function modelSpec(model) {
  const spec = MODEL_SPECS[model];
  if (!spec) throw new Error("unsupported Image2Pro model");
  return spec;
}

/** 校验媒体与上传占位符；公开视频接口只会外发图片，音视频在归一阶段明确拒绝。 */
function mediaReference(value, type, files, model) {
  modelSpec(model);
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
    throw new Error("Image2Pro " + type + " reference URL is invalid");
  }
  if (isHTTPURL(value)) return value;
  const data = typeof value === "string" ? /^data:([^;,]+);base64,((?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?)$/.exec(value) : null;
  if (data && data[2] && rule.mimes.includes(data[1])) return value;
  throw new Error("Image2Pro " + type + " references must be HTTP(S) URLs or supported " + type + " data URLs");
}

/** 校验兼容 content，保留文字与图片顺序、重复引用和角色。 */
function normalizeContent(content, files, model) {
  const spec = modelSpec(model);
  const name = model === FLASH_MODEL || model === "Flash视频-MAX" ? "Image2Pro Flash-MAX" : "Image2Pro Seedance";
  if (!Array.isArray(content) || content.length === 0) throw new Error(name + " content must be a non-empty array");
  const counts = { image: 0, video: 0, audio: 0 };
  let textLength = 0;
  let hasText = false;
  const normalized = content.map((item) => {
    if (!item || typeof item !== "object" || Array.isArray(item)) throw new Error(name + " content items must be objects");
    if (item.type === "text") {
      if (Object.keys(item).some((field) => !["type", "text"].includes(field)) || typeof item.text !== "string")
        throw new Error(name + " text content requires a text string and no extra fields");
      if (item.text.trim()) hasText = true;
      textLength += item.text.length;
      if (textLength > 30000) throw new Error("Image2Pro prompt must not exceed 30000 characters");
      if (
        /^(?:Seedance2\.0|Seedance2\.0 0\.9r)$/.test(model) &&
        /(?:^|\s)--(?:duration|dur|frames|resolution|rs|ratio|rt|seed|camera_fixed|cf|watermark|wm)\b/i.test(item.text)
      )
        throw new Error("inline " + name + " parameter overrides are unsupported; use explicit request fields");
      return { type: "text", text: item.text };
    }
    if (!["image_url", "video_url", "audio_url"].includes(item.type)) throw new Error("unsupported " + name + " content type");
    if (Object.keys(item).some((field) => !["type", item.type, "role"].includes(field))) throw new Error("unsupported " + name + " media content field");
    const reference = item[item.type];
    if (!reference || typeof reference !== "object" || Array.isArray(reference) || Object.keys(reference).some((field) => field !== "url"))
      throw new Error(name + " media content requires a URL object without extra fields");
    const type = item.type.slice(0, -4);
    const role = item.role === undefined ? "reference_" + type : item.role;
    if (role !== "reference_" + type) throw new Error("unsupported " + name + " " + type + " role");
    counts[type]++;
    if (type === "image" && counts.image > spec.imageLimit) throw new Error(name + " supports at most " + spec.imageLimit + " reference images");
    if (type !== "image") throw new Error("Image2Pro public video API supports image references only");
    return { type: item.type, [item.type]: { url: mediaReference(reference.url, type, files, model) }, role };
  });
  if (!hasText && !counts.image) throw new Error("Image2Pro requires prompt or at least one reference image");
  return normalized;
}

/** 兼容入口归一为内部 content；两个模型按公开目录校验时长、比例和图片数量。 */
function normalizeRequest(source, model, files = []) {
  if (!source || typeof source !== "object" || Array.isArray(source)) throw new Error("Image2Pro request body must be an object");
  const spec = modelSpec(model);
  for (const key of Object.keys(source)) if (!REQUEST_FIELDS.includes(key)) throw new Error("unsupported Image2Pro field: " + key);
  if (BOOLEAN_FIELDS.some((field) => source[field] !== undefined)) throw new Error("Image2Pro public video API does not support Seedance generation flags");
  const request = { model: source.model, duration: requestedSeconds(source) };
  if (!Number.isInteger(request.duration) || request.duration < 4 || request.duration > spec.durationMax)
    throw new Error("Image2Pro duration must be an integer from 4 to " + spec.durationMax);
  let content = [];
  if (source.content !== undefined) {
    if (source.prompt !== undefined || Object.values(MEDIA_RULES).some((rule) => rule.aliases.some((field) => source[field] !== undefined)))
      throw new Error("Image2Pro content cannot be combined with prompt or reference aliases");
    content = source.content;
  } else if (source.prompt !== undefined) {
    if (typeof source.prompt !== "string") throw new Error("Image2Pro prompt must be a string");
    if (source.prompt.trim()) content.push({ type: "text", text: source.prompt });
  }
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
    for (const reference of references) {
      let url = reference;
      let role = "reference_" + type;
      if (reference && typeof reference === "object" && !Array.isArray(reference) && !Object.prototype.hasOwnProperty.call(reference, "__fileRef")) {
        if (Object.keys(reference).some((key) => !["url", "role"].includes(key))) throw new Error("unsupported Image2Pro reference field");
        url = reference.url;
        if (reference.role !== undefined) role = reference.role;
      }
      content.push({ type: type + "_url", [type + "_url"]: { url }, role });
    }
  }
  request.content = normalizeContent(content, files, model);
  if (source.ratio !== undefined && source.aspect_ratio !== undefined && source.ratio !== source.aspect_ratio)
    throw new Error("ratio and aspect_ratio conflict");
  request.ratio = source.ratio === undefined ? (source.aspect_ratio === undefined ? "16:9" : source.aspect_ratio) : source.ratio;
  if (!["16:9", "4:3", "1:1", "3:4", "9:16", "21:9", "adaptive"].includes(request.ratio)) throw new Error("unsupported Image2Pro ratio");
  if ((model === FLASH_MODEL || model === "Flash视频-MAX") && request.ratio === "adaptive") throw new Error("Image2Pro Flash-MAX requires a fixed ratio");
  request.resolution = source.resolution === undefined ? "720p" : source.resolution;
  if (request.resolution !== "720p")
    throw new Error("Image2Pro public video API does not expose resolution; only the Canvas compatibility value 720p is accepted");
  const key = clientRequestId(source);
  if (key !== undefined) request.client_request_id = key;
  return request;
}

/** 根据已校验的内部 content 分类宿主动作。 */
function videoAction(content) {
  return content.some((item) => item.type === "image_url") ? "image_to_video" : "text_to_video";
}

/** 解码 multipart 并以内联占位符保留参考图；公开视频接口不接受音视频上传。 */
function multipartRequest(body, model) {
  modelSpec(model);
  const request = {};
  for (const field of Object.keys(body.fields || {})) {
    if (!REQUEST_FIELDS.includes(field)) throw new Error("unsupported Image2Pro field: " + field);
    const values = body.fields[field];
    if (!Array.isArray(values) || !values.length) throw new Error("invalid multipart field: " + field);
    if (field === "content" || BOOLEAN_FIELDS.includes(field)) {
      if (values.length !== 1) throw new Error(field + " must be provided once");
      try {
        request[field] = JSON.parse(values[0]);
      } catch {
        throw new Error(field + " must contain valid JSON");
      }
    } else if (Object.values(MEDIA_RULES).some((rule) => rule.arrays.includes(field))) {
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
    if (type !== "image") throw new Error("Image2Pro public video API supports image references only");
    const field = aliases[0];
    if (type === "image" && !rule.arrays.includes(field) && (mediaFiles.length !== 1 || request[field] !== undefined))
      throw new Error(field + " must contain one image");
    const references = request[field] === undefined ? [] : Array.isArray(request[field]) ? request[field] : [request[field]];
    for (const file of mediaFiles) {
      if (!Number.isInteger(file.size) || file.size <= 0) throw new Error("Image2Pro uploaded reference files must not be empty");
      let mimeType = typeof file.mimeType === "string" ? file.mimeType.toLowerCase().split(";")[0].trim() : "";
      if (!mimeType || mimeType === "application/octet-stream") {
        const extension = /\.([a-z0-9]+)$/i.exec(file.filename || "");
        const suffix = extension ? extension[1].toLowerCase() : "";
        mimeType = FILE_MIMES[suffix] || "";
      }
      if (!rule.mimes.includes(mimeType)) throw new Error("Image2Pro upload MIME type must match a supported " + type + " format");
      references.push({ __fileRef: file.ref, encoding: "dataUrl", mimeType });
    }
    delete request[field];
    request[rule.field] = references;
  }
  return request;
}

/**
 * 将内部 content 转换为 Image2Pro 公开视频接口字段，保持文本片段和图片各自的顺序。
 * @param {object} request 已校验且包含内部 content 的请求。
 * @returns {object} 使用顶层 prompt、input_image 或 images 的供应商请求体。
 * @throws {Error} 内部请求包含公开接口无法表达的素材类型时拒绝发送。
 */
function providerVideoBody(request) {
  const body = { model: modelSpec(request.model).upstream, duration: request.duration, ratio: request.ratio };
  let prompt = "";
  const images = [];
  for (const item of request.content) {
    if (item.type === "text") {
      prompt += item.text;
      continue;
    }
    if (item.type !== "image_url") throw new Error("Image2Pro public video API supports image references only");
    images.push(item.image_url.url);
  }
  if (prompt.trim()) body.prompt = prompt;
  if (images.length === 1) body.input_image = images[0];
  else if (images.length > 1) body.images = images;
  return body;
}

/** 构造单次 JSON 提交，上传占位符由宿主替换；幂等键与禁止重试避免未知结果重复扣费。 */
export function buildSubmitRequest(ctx) {
  const model = ctx.upstreamModel || ctx.model;
  const request = normalizeRequest(ctx.requestBody, model, ctx.files || []);
  request.model = model;
  const key = request.client_request_id === undefined ? ctx.publicTaskId : request.client_request_id;
  if (typeof key !== "string" || !/^[A-Za-z0-9_.-]{6,120}$/.test(key))
    throw new Error("a valid public task ID or client idempotency key is required for Image2Pro");
  delete request.client_request_id;
  const action = videoAction(request.content);
  const body = providerVideoBody(request);
  return {
    url: apiBase(ctx) + "/videos",
    method: "POST",
    headers: { Authorization: "Bearer " + ctx.apiKey, "Content-Type": "application/json", "Idempotency-Key": key },
    body,
    noRetry: true,
    action,
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

/** 解码单轮 Responses 文本和图片；工具、历史消息及音视频输入拒绝。 */
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
          images.push(item.image_url);
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
      else if (body && body.kind === "multipart") source = multipartRequest(body, ctx.upstreamModel || ctx.model);
      else throw new Error("Image2Pro requires a JSON or multipart body");
      const model = ctx.model || (source && source.model);
      const request = normalizeRequest(source, ctx.upstreamModel || model, body.kind === "multipart" ? body.files || [] : []);
      request.model = model;
      return {
        kind: "submit",
        model,
        action: videoAction(request.content),
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
    /** 单轮 input 接受文本和普通参考图，所需秒数必须显式提供。 */
    decodeRequest: function (ctx) {
      const request = responsesRequest(ctx);
      return {
        kind: "submit",
        model: request.model,
        action: videoAction(request.content),
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
