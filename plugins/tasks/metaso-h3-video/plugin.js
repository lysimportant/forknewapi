/** 对外提供 OpenAI 视频入口，上游使用秘塔 MiniMax H3 原生 v2 协议。 */
export const meta = {
  apiVersion: 1,
  key: "metaso-h3-video",
  name: "Metaso MiniMax H3 Video",
  icon: "MiniMax.Color",
  description: {
    en: "Metaso native MiniMax H3 video generation with multimodal references",
    zh: "秘塔 MiniMax H3 原生视频生成，支持多媒体参考输入",
  },
  version: "1.1.0",
  author: { name: "lysimportant/forknewapi" },
  website: "https://b.metaso.cn/minimax-h3",
  baseUrl: "https://b.metaso.cn/api/minimax",
  allowedHosts: ["b.metaso.cn:443"],
  models: ["Minimax-H3"],
  fetchMode: "per_task",
  requiredCapabilities: ["task-submit-no-retry@1"],
  protocols: ["openai_video"],
  usageSchema: {
    seconds: {
      type: "number",
      unit: "second",
      description: { en: "Video generation unit price", zh: "视频生成单价" },
    },
    size: {
      enum: ["480p", "768p", "1080p", "2k"],
      description: { en: "Output video resolution", zh: "输出视频清晰度" },
    },
    input_images: {
      type: "number",
      unit: "count",
      description: { en: "Input image unit price", zh: "输入图片单价" },
    },
    input_videos: {
      type: "number",
      unit: "count",
      description: { en: "Input video unit price", zh: "输入视频单价" },
    },
    input_audios: {
      type: "number",
      unit: "count",
      description: { en: "Input audio unit price", zh: "输入音频单价" },
    },
  },
  usageExamples: [
    { label: "text · 480p · 5s", facts: { seconds: 5, size: "480p", input_images: 0, input_videos: 0, input_audios: 0 } },
    { label: "multimodal · 768p · 5s", facts: { seconds: 5, size: "768p", input_images: 1, input_videos: 1, input_audios: 1 } },
  ],
};

const MODEL = "Minimax-H3";
const UPSTREAM_MODEL = "MiniMax-H3";
const DEFAULT_SIZE = "480p";
const MIN_SECONDS = 4;
const MAX_SECONDS = 30;
const MAX_INPUT_IMAGES = 9;
const MAX_INPUT_VIDEOS = 3;
const MAX_INPUT_AUDIOS = 3;
const MAX_INPUT_TOTAL = MAX_INPUT_IMAGES + MAX_INPUT_VIDEOS + MAX_INPUT_AUDIOS;
const SIZES = new Set(meta.usageSchema.size.enum);
const SIZE_TO_UPSTREAM = {
  "480p": "480p",
  "768p": "768P",
  "1080p": "1080p",
  "2k": "2K",
};
const LEGACY_SIZE_ALIASES = {
  "854x480": "480p",
  "480x854": "480p",
  "1280x720": "768p",
  "720x1280": "768p",
  "1920x1080": "1080p",
  "1080x1920": "1080p",
  "1792x1024": "2k",
  "1024x1792": "2k",
};
const RATIOS = ["adaptive", "21:9", "16:9", "4:3", "1:1", "3:4", "9:16"];

/** 去除字符串两端空白；空值统一返回空字符串。 */
function trimmed(value) {
  return typeof value === "string" ? value.trim() : String(value || "").trim();
}

/** 迁移旧工厂地址；自定义代理保留主机与路径前缀。 */
function apiBase(ctx) {
  let base = trimmed(ctx.baseUrl || meta.baseUrl).replace(/\/+$/, "");
  if (!/^https?:\/\/[^\s/?#@]+(?:\/[^\s?#]*)?$/.test(base))
    throw new Error("channel base URL must be an HTTP(S) API base without credentials, query or fragment");
  if (/^https:\/\/metaso\.cn\/api\/openai(?:\/v1(?:\/videos)?)?$/.test(base)) base = meta.baseUrl;
  return base.replace(/\/(?:v2(?:\/video_generation)?|v1(?:\/videos)?)$/, "") + "/v2";
}

/** 校验公开媒体 URL；媒体类型由字段名保留给上游。 */
function mediaURL(value, field) {
  let url = value;
  if (value && typeof value === "object" && !Array.isArray(value)) {
    const urlField = field === "images" ? "image_url" : field === "videos" ? "video_url" : field === "audios" ? "audio_url" : "image_url";
    url = value[urlField] === undefined ? value.url : value[urlField];
    if (url && typeof url === "object" && !Array.isArray(url)) url = url.url;
  }
  if (typeof url !== "string" || !/^https?:\/\/[^\s]+$/i.test(url.trim())) throw new Error(field + " items must be public HTTP(S) URLs");
  return url.trim();
}

/** 读取单值、数组或 JSON 数组中的公开媒体 URL。 */
function mediaList(value, field) {
  if (value === undefined || value === null || value === "") return [];
  let source = value;
  if (typeof source === "string" && source.trim().startsWith("[")) {
    try {
      source = JSON.parse(source);
    } catch {
      throw new Error(field + " must be a JSON array");
    }
  }
  const values = Array.isArray(source) ? source : [source];
  return values.map(function (item) {
    return mediaURL(item, field);
  });
}

/** 解析 JSON、表单或 multipart 请求；秘塔合同不接受文件上传。 */
function decodedBody(body) {
  if (!body || !["json", "form", "multipart"].includes(body.kind)) throw new Error("JSON, form, or multipart body required");
  if (body.kind === "json") {
    if (!body.value || typeof body.value !== "object" || Array.isArray(body.value)) throw new Error("JSON object required");
    return Object.assign({}, body.value);
  }
  if ((body.files || []).length) throw new Error("Metaso H3 requires a public image URL; file uploads are unsupported");
  const request = {};
  for (const key of Object.keys(body.fields || {})) {
    const values = body.fields[key];
    if (!Array.isArray(values) || values.length !== 1) throw new Error(key + " must be provided once");
    request[key] = values[0];
  }
  return request;
}

/** JSON 和表单共用 metadata 解析，拒绝错误类型以免静默丢弃参数。 */
function requestMetadata(request) {
  let metadata = request.metadata;
  if (metadata === undefined || metadata === null) return {};
  if (typeof metadata === "string") {
    try {
      metadata = JSON.parse(metadata);
    } catch {
      throw new Error("metadata must be a JSON object string");
    }
  }
  if (!metadata || typeof metadata !== "object" || Array.isArray(metadata)) throw new Error("metadata must be a JSON object");
  return metadata;
}

/** 顶层原生参数与 Canvas metadata 参数使用同一套读取规则。 */
function parameter(request, metadata, key) {
  if (request[key] !== undefined && metadata[key] !== undefined && JSON.stringify(request[key]) !== JSON.stringify(metadata[key])) {
    throw new Error(key + " conflicts with metadata." + key);
  }
  return request[key] === undefined ? metadata[key] : request[key];
}

function mediaItem(type, url, role) {
  return { type, [type]: { url: mediaURL(url, type === "video_url" ? "videos" : type === "audio_url" ? "audios" : "images") }, role };
}

/** 保留原生 content 的角色；兼容旧请求和 Canvas 的多媒体字段。 */
function contentInputs(request, metadata) {
  let supplied = parameter(request, metadata, "content");
  if (typeof supplied === "string") {
    try {
      supplied = JSON.parse(supplied);
    } catch {
      throw new Error("content must be a JSON array");
    }
  }
  if (supplied !== undefined && !Array.isArray(supplied)) throw new Error("content must be an array");
  const content = (supplied || []).map(function (item) {
    if (typeof item === "string") return { type: "text", text: item };
    if (!item || typeof item !== "object" || Array.isArray(item)) throw new Error("invalid content item");
    if (["text", "input_text"].includes(item.type) && typeof item.text === "string") return { type: "text", text: item.text };
    const type = { input_image: "image_url", input_video: "video_url", input_audio: "audio_url" }[item.type] || item.type;
    const roles = { image_url: ["first_frame", "last_frame", "reference_image"], video_url: ["reference_video"], audio_url: ["reference_audio"] };
    if (!roles[type]) throw new Error("unsupported content type");
    const role = item.role === undefined ? roles[type][0] : item.role;
    if (!roles[type].includes(role)) throw new Error("unsupported " + type + " role");
    return mediaItem(type, item[type] || item.url, role);
  });
  const prompt = trimmed(request.prompt || request.input);
  if (
    !content.some(function (item) {
      return item.type === "text" && trimmed(item.text);
    }) &&
    prompt
  )
    content.unshift({ type: "text", text: prompt });
  const videos = mediaList(request.videos || request.reference_videos || metadata.reference_video, "videos").concat(mediaList(request.video, "videos"));
  const audios = mediaList(request.audios || request.reference_audios || metadata.reference_audio, "audios").concat(mediaList(request.audio, "audios"));
  const images = mediaList(request.images || request.reference_images, "images");
  const referenceMode =
    trimmed(request.mode) === "omni_reference" ||
    trimmed(request.mode) === "multi_reference" ||
    request.reference_images !== undefined ||
    videos.length ||
    audios.length ||
    images.length > 2;
  images.forEach(function (url, index) {
    content.push(mediaItem("image_url", url, referenceMode ? "reference_image" : index === 0 ? "first_frame" : "last_frame"));
  });
  for (const url of mediaList(request.input_reference, "images").concat(mediaList(request.image, "images")))
    content.push(mediaItem("image_url", url, "first_frame"));
  for (const role of ["first_frame", "last_frame"]) {
    const url = parameter(request, metadata, role + "_image");
    if (url !== undefined) content.push(mediaItem("image_url", url, role));
  }
  for (const url of videos) content.push(mediaItem("video_url", url, "reference_video"));
  for (const url of audios) content.push(mediaItem("audio_url", url, "reference_audio"));
  if (
    !content.some(function (item) {
      return item.type === "text" && trimmed(item.text);
    })
  )
    throw new Error("content requires a non-empty text item or prompt");
  for (const role of ["first_frame", "last_frame"]) {
    if (
      content.filter(function (item) {
        return item.role === role;
      }).length > 1
    )
      throw new Error("content accepts at most one " + role);
  }
  return content;
}

/** 解析提示词、时长、尺寸和多媒体引用，输出计费事实。 */
function normalizeRequest(request, model) {
  if (!request || typeof request !== "object" || Array.isArray(request)) throw new Error("request body must be an object");
  if (model !== MODEL && model !== UPSTREAM_MODEL) throw new Error("unsupported Metaso H3 model");
  const metadata = requestMetadata(request);
  const content = contentInputs(request, metadata);
  if (request.seconds === undefined && request.duration === undefined) throw new Error("seconds or duration is required");
  if (request.seconds !== undefined && request.duration !== undefined && Number(request.seconds) !== Number(request.duration))
    throw new Error("seconds and duration conflict");
  const rawSeconds = request.seconds === undefined ? request.duration : request.seconds;
  const seconds = Number(rawSeconds);
  if ((typeof rawSeconds !== "number" && typeof rawSeconds !== "string") || !Number.isSafeInteger(seconds) || seconds < MIN_SECONDS || seconds > MAX_SECONDS)
    throw new Error("seconds must be an integer between 4 and 30");
  const resolution = parameter(request, metadata, "resolution");
  const rawSize = request.size === undefined ? resolution : request.size;
  const alternateSize = request.size === undefined ? undefined : resolution;
  const normalizedSize = normalizeSize(rawSize || DEFAULT_SIZE);
  if (alternateSize !== undefined && normalizeSize(alternateSize).size !== normalizedSize.size) throw new Error("size and resolution conflict");
  const size = normalizedSize.size;
  if ((size === "1080p" || size === "2k") && seconds > 15) throw new Error("1080p and 2K support at most 15 seconds");
  const images = content.filter(function (item) {
    return item.type === "image_url";
  });
  const videos = content.filter(function (item) {
    return item.type === "video_url";
  });
  const audios = content.filter(function (item) {
    return item.type === "audio_url";
  });
  const total = images.length + videos.length + audios.length;
  if (images.length > MAX_INPUT_IMAGES) throw new Error("multimodal references cannot exceed 9 images");
  if (videos.length > MAX_INPUT_VIDEOS) throw new Error("multimodal references cannot exceed 3 videos");
  if (audios.length > MAX_INPUT_AUDIOS) throw new Error("multimodal references cannot exceed 3 audios");
  if (total > MAX_INPUT_TOTAL) throw new Error("multimodal references cannot exceed 15 items");
  const explicitRatio = parameter(request, metadata, "ratio");
  const aspectRatio = parameter(request, metadata, "aspect_ratio");
  if (explicitRatio !== undefined && aspectRatio !== undefined && explicitRatio !== aspectRatio) throw new Error("ratio and aspect_ratio conflict");
  const pixelRatio = LEGACY_SIZE_ALIASES[trimmed(rawSize)]
    ? Number(trimmed(rawSize).split("x")[0]) > Number(trimmed(rawSize).split("x")[1])
      ? "16:9"
      : "9:16"
    : "";
  const ratio = trimmed(explicitRatio === undefined ? aspectRatio : explicitRatio) || pixelRatio || (images.length || videos.length ? "adaptive" : "16:9");
  if (!RATIOS.includes(ratio)) throw new Error("unsupported ratio");
  if (ratio === "adaptive" && !images.length && !videos.length) throw new Error("ratio adaptive requires an image or video input");
  return {
    content,
    metadata,
    ratio,
    seconds,
    size,
    upstreamSize: normalizedSize.upstreamSize,
    images,
    videos,
    audios,
    input_images: images.length,
    input_videos: videos.length,
    input_audios: audios.length,
  };
}

/** 将清晰度标签或旧版像素尺寸归一化为计费档位和秘塔请求尺寸。 */
function normalizeSize(value) {
  const raw = trimmed(value).toLowerCase();
  const size = SIZES.has(raw) ? raw : LEGACY_SIZE_ALIASES[raw];
  if (!size) throw new Error("size must be one of 480p, 768p, 1080p, or 2k");
  return { size, upstreamSize: SIZE_TO_UPSTREAM[size] };
}

/** 把内部请求转换为秘塔 MiniMax 原生 JSON，保留显式 false。 */
function upstreamBody(request, model) {
  const facts = normalizeRequest(request, model);
  const body = { model: UPSTREAM_MODEL, content: facts.content, resolution: facts.upstreamSize, duration: facts.seconds, ratio: facts.ratio };
  for (const key of ["fast_mode", "aigc_watermark", "context_ir_enabled"]) {
    let value = parameter(request, facts.metadata, key);
    if (value === undefined) continue;
    if (value === "true" || value === "false") value = value === "true";
    if (typeof value !== "boolean") throw new Error(key + " must be a boolean");
    body[key] = value;
  }
  const callback = parameter(request, facts.metadata, "callback_url");
  if (callback !== undefined) body.callback_url = mediaURL(callback, "callback_url");
  return { body, facts };
}

/** 生成秘塔视频创建请求；提交结果未知时禁止宿主自动重试。 */
export function buildSubmitRequest(ctx) {
  const built = upstreamBody(ctx.requestBody, ctx.upstreamModel || ctx.model);
  return {
    url: apiBase(ctx) + "/video_generation",
    method: "POST",
    headers: { Authorization: "Bearer " + ctx.apiKey, "Content-Type": "application/json", Accept: "application/json" },
    body: built.body,
    noRetry: true,
  };
}

/** 解析秘塔创建响应中的上游任务 ID。 */
export function parseSubmitResponse(_ctx, response) {
  const body = response && response.body;
  if (!body || typeof body !== "object" || Array.isArray(body)) throw new Error("upstream video response must be a JSON object");
  if (response.statusCode !== undefined && (response.statusCode < 200 || response.statusCode >= 300))
    throw new Error("upstream video HTTP status " + response.statusCode);
  const failure = apiError(body);
  if (failure) throw new Error(failure.message);
  const taskId = body.task_id;
  if (typeof taskId !== "string" || !taskId.trim()) throw new Error("upstream video id is missing");
  return { taskId, taskData: body };
}

/** 返回提交阶段的冻结计费事实；倍率估算阶段不重复计费。 */
export function extractUsage(ctx) {
  const built = upstreamBody(ctx.requestBody, ctx.upstreamModel || ctx.model);
  if (ctx.usagePurpose === "billing_ratios") return {};
  return {
    seconds: built.facts.seconds,
    size: built.facts.size,
    input_images: built.facts.input_images,
    input_videos: built.facts.input_videos,
    input_audios: built.facts.input_audios,
  };
}

/** 从 v2 task.usage 结算已声明事实；视频/音频秒数不能当作素材数量。 */
export function extractUsageOnComplete(_task, result, body) {
  if (!result || result.status !== "SUCCESS") return null;
  const task = queryTask(body);
  if (!task) return null;
  const usage = task.usage || {};
  const facts = {};
  const size = trimmed(task.resolution).toLowerCase();
  if (SIZES.has(size)) facts.size = size;
  const seconds = usage.output_seconds === undefined ? task.duration : usage.output_seconds;
  if (typeof seconds === "number" && Number.isFinite(seconds) && seconds >= MIN_SECONDS && seconds <= MAX_SECONDS) facts.seconds = seconds;
  const images = usage.input_image_count;
  if (typeof images === "number" && Number.isSafeInteger(images) && images >= 0 && images <= MAX_INPUT_IMAGES) facts.input_images = images;
  return Object.keys(facts).length ? facts : null;
}

/** 构造秘塔任务查询请求。 */
export function buildQueryRequest(ctx) {
  return {
    url: apiBase(ctx) + "/query/video_generation/" + encodeURIComponent(ctx.taskId),
    method: "GET",
    headers: { Authorization: "Bearer " + ctx.apiKey, Accept: "application/json" },
  };
}

function queryTask(body) {
  const task = body && body.task;
  return task && typeof task === "object" && !Array.isArray(task) ? task : null;
}

function apiError(body) {
  if (body.error)
    return { code: Number(body.error.http_code || body.error.code || 0), message: trimmed(body.error.message) || "upstream video request failed" };
  if (body.base_resp && Number(body.base_resp.status_code) !== 0)
    return { code: Number(body.base_resp.status_code), message: trimmed(body.base_resp.status_msg) || "upstream video request failed" };
  return null;
}

/** 映射秘塔异步任务状态；未知状态保持未知，避免误报成功。 */
export function parseTaskResult(_ctx, body) {
  if (!body || typeof body !== "object" || Array.isArray(body)) return { status: "UNKNOWN", reason: "invalid video response" };
  const failure = apiError(body);
  if (failure) {
    if (failure.code === 408 || failure.code === 429 || (failure.code >= 500 && failure.code < 600)) throw new Error(failure.message);
    return { status: "FAILURE", reason: failure.message };
  }
  const task = queryTask(body);
  if (!task) return { status: "UNKNOWN", reason: "missing video task" };
  const raw = trimmed(task.status).toLowerCase();
  const status = {
    queued: "QUEUED",
    pending: "QUEUED",
    processing: "IN_PROGRESS",
    in_progress: "IN_PROGRESS",
    running: "IN_PROGRESS",
    succeeded: "SUCCESS",
    success: "SUCCESS",
    completed: "SUCCESS",
    failed: "FAILURE",
    failure: "FAILURE",
    cancelled: "FAILURE",
    canceled: "FAILURE",
    expired: "FAILURE",
  }[raw];
  if (!status) return { status: "UNKNOWN", reason: "unrecognized video status" };
  const result = { status };
  if (status === "FAILURE") result.reason = trimmed(task.error && task.error.message) || "upstream video generation failed";
  if (status === "SUCCESS") {
    result.url = mediaURL(task.content && task.content.url, "video result");
    result.progress = "100%";
  } else {
    const progress = Number(task.progress);
    if (Number.isFinite(progress) && progress >= 0 && progress < 100) result.progress = progress + "%";
  }
  return result;
}

/** 宿主持久化的查询响应可能含一层旧版任务封装。 */
function artifactURL(ctx) {
  let body = (ctx && ctx.data) || {};
  if (body.data && body.data.task_id && Object.prototype.hasOwnProperty.call(body.data, "data")) body = body.data.data || {};
  const task = queryTask(body);
  return task && task.content ? trimmed(task.content.url) : "";
}

/** 成功任务声明一个视频制品，由宿主代理 task.content.url。 */
export function listArtifacts(task) {
  return task.status === "SUCCESS" && artifactURL(task) ? [{ key: "video", type: "video", mimeType: "video/mp4" }] : [];
}

/** 构造秘塔视频内容下载请求，不把供应商 URL 或凭据暴露给客户端。 */
export function buildContentRequest(ctx) {
  if (ctx.artifactKey !== "video") throw new Error("artifact_not_found");
  const url = artifactURL(ctx);
  if (!url) throw new Error("artifact_not_found");
  return {
    url: mediaURL(url, "video result"),
    method: ctx.clientRequest.method,
    credentialless: true,
  };
}

/** 注册 OpenAI 视频入口；模型身份始终保持 Minimax-H3，不添加 sora-2 映射。 */
export const protocols = {
  openai_video: {
    decodeRequest: function (ctx) {
      const request = decodedBody(ctx.body);
      const built = upstreamBody(request, ctx.upstreamModel || ctx.model);
      const action =
        built.facts.videos.length || built.facts.audios.length || built.facts.images.length > 1
          ? "reference_to_video"
          : built.facts.images.length
            ? "image_to_video"
            : "text_to_video";
      return { kind: "submit", model: ctx.model, action: action, requestBody: Object.assign({}, request, { model: ctx.model }) };
    },
    render: function (_ctx, task) {
      const statuses = { NOT_START: "queued", SUBMITTED: "queued", QUEUED: "queued", IN_PROGRESS: "in_progress", SUCCESS: "completed", FAILURE: "failed" };
      return { id: task.task_id, object: "video", model: (task.properties || {}).origin_model_name || MODEL, status: statuses[task.status] || "unknown" };
    },
  },
};
