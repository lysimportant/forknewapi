/** 秘塔 OpenAI 视频兼容插件；保留 H3 多媒体参考字段并交给上游处理。 */
export const meta = {
  apiVersion: 1,
  key: "metaso-h3-video",
  name: "Metaso MiniMax H3 Video",
  icon: "MiniMax.Color",
  description: {
    en: "Metaso OpenAI-compatible MiniMax H3 video generation with multimodal references",
    zh: "秘塔 OpenAI 兼容 MiniMax H3 视频生成，支持多媒体参考输入",
  },
  version: "1.0.0",
  author: { name: "lysimportant/forknewapi" },
  website: "https://b.metaso.cn/minimax-h3/new-api-guide",
  baseUrl: "https://metaso.cn/api/openai",
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
      enum: ["1280x720", "720x1280", "1792x1024", "1024x1792"],
      description: { en: "Output video size", zh: "输出视频尺寸" },
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
    { label: "text · 1280x720 · 5s", facts: { seconds: 5, size: "1280x720", input_images: 0, input_videos: 0, input_audios: 0 } },
    { label: "multimodal · 1280x720 · 5s", facts: { seconds: 5, size: "1280x720", input_images: 1, input_videos: 1, input_audios: 1 } },
  ],
};

const MODEL = "Minimax-H3";
const DEFAULT_SIZE = "1280x720";
const MAX_SECONDS = 3600;
const MAX_INPUT_IMAGES = 9;
const MAX_INPUT_VIDEOS = 3;
const MAX_INPUT_AUDIOS = 3;
const MAX_INPUT_TOTAL = MAX_INPUT_IMAGES + MAX_INPUT_VIDEOS + MAX_INPUT_AUDIOS;
const SIZES = new Set(meta.usageSchema.size.enum);

/** 去除字符串两端空白；空值统一返回空字符串。 */
function trimmed(value) {
  return typeof value === "string" ? value.trim() : String(value || "").trim();
}

/** 规范化渠道地址，并保留管理员设置的路径前缀。 */
function apiBase(ctx) {
  let base = trimmed(ctx.baseUrl).replace(/\/+$/, "");
  if (!/^https?:\/\/[^/?#]+(?:\/[^?#]*)?$/.test(base)) throw new Error("channel base URL must be an HTTP(S) API base without query or fragment");
  if (base.endsWith("/v1/videos")) base = base.slice(0, -7);
  return /\/v1$/i.test(base) ? base : base + "/v1";
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
  return values.map(function (item) { return mediaURL(item, field); });
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

/** 从 Canvas 的 metadata.content 或兼容字段提取多媒体参考和提示词。 */
function contentInputs(request) {
  let metadata = request.metadata;
  if (typeof metadata === "string") {
    try {
      metadata = JSON.parse(metadata);
    } catch {
      throw new Error("metadata must be a JSON object string");
    }
  }
  if (!metadata || typeof metadata !== "object" || Array.isArray(metadata)) return { prompt: "", images: [], videos: [], audios: [] };
  const content = metadata.content;
  if (!Array.isArray(content)) return { prompt: "", images: [], videos: [], audios: [] };
  const texts = [];
  const images = [];
  const videos = [];
  const audios = [];
  for (const item of content) {
    if (typeof item === "string") {
      texts.push(item);
      continue;
    }
    if (!item || typeof item !== "object" || Array.isArray(item)) continue;
    if (["text", "input_text"].includes(item.type) && typeof item.text === "string") texts.push(item.text);
    if (["image_url", "input_image"].includes(item.type)) images.push(mediaURL(item.image_url || item.url, "images"));
    if (["video_url", "input_video"].includes(item.type)) videos.push(mediaURL(item.video_url || item.url, "videos"));
    if (["audio_url", "input_audio"].includes(item.type)) audios.push(mediaURL(item.audio_url || item.url, "audios"));
  }
  return { prompt: texts.filter(function (text) { return trimmed(text); }).join("\n"), images, videos, audios };
}

/** 解析提示词、时长、尺寸和多媒体引用，输出计费事实。 */
function normalizeRequest(request, model) {
  if (!request || typeof request !== "object" || Array.isArray(request)) throw new Error("request body must be an object");
  if (model !== MODEL) throw new Error("unsupported Metaso H3 model");
  const metadata = contentInputs(request);
  const prompt = trimmed(request.prompt || request.input || metadata.prompt);
  if (!prompt) throw new Error("prompt is required");
  if (request.seconds === undefined && request.duration === undefined) throw new Error("seconds or duration is required");
  if (request.seconds !== undefined && request.duration !== undefined && Number(request.seconds) !== Number(request.duration)) throw new Error("seconds and duration conflict");
  const rawSeconds = request.seconds === undefined ? request.duration : request.seconds;
  const seconds = Number(rawSeconds);
  if (!Number.isSafeInteger(seconds) || seconds < 1 || seconds > MAX_SECONDS) throw new Error("seconds must be an integer between 1 and 3600");
  const rawSize = request.size === undefined ? request.resolution : request.size;
  if (request.size !== undefined && request.resolution !== undefined && String(request.size) !== String(request.resolution)) throw new Error("size and resolution conflict");
  const size = trimmed(rawSize) || DEFAULT_SIZE;
  if (!SIZES.has(size)) throw new Error("size must be one of the documented Metaso H3 sizes");
  let images = mediaList(request.images || request.reference_images, "images");
  let videos = mediaList(request.videos || request.reference_videos, "videos");
  let audios = mediaList(request.audios || request.reference_audios, "audios");
  if (request.input_reference !== undefined && request.input_reference !== null && request.input_reference !== "") images = images.concat(mediaList(request.input_reference, "images"));
  if (request.image !== undefined && request.image !== null && request.image !== "") {
    images = images.concat(mediaList(request.image, "images"));
  }
  if (request.audio !== undefined) audios = audios.concat(mediaList(request.audio, "audios"));
  if (request.video !== undefined) videos = videos.concat(mediaList(request.video, "videos"));
  images = images.concat(metadata.images);
  videos = videos.concat(metadata.videos);
  audios = audios.concat(metadata.audios);
  const total = images.length + videos.length + audios.length;
  if (images.length > MAX_INPUT_IMAGES) throw new Error("multimodal references cannot exceed 9 images");
  if (videos.length > MAX_INPUT_VIDEOS) throw new Error("multimodal references cannot exceed 3 videos");
  if (audios.length > MAX_INPUT_AUDIOS) throw new Error("multimodal references cannot exceed 3 audios");
  if (total > MAX_INPUT_TOTAL) throw new Error("multimodal references cannot exceed 15 items");
  return { prompt, seconds, size, images, videos, audios, input_images: images.length, input_videos: videos.length, input_audios: audios.length, mode: trimmed(request.mode) };
}

/** 把内部请求转换为秘塔文档中的 OpenAI 视频 JSON 字段白名单。 */
function upstreamBody(request, model) {
  const facts = normalizeRequest(request, model);
  const body = { model: MODEL, prompt: facts.prompt, seconds: facts.seconds, size: facts.size };
  if (facts.images.length === 1 && !facts.videos.length && !facts.audios.length) body.input_reference = { image_url: facts.images[0] };
  if (facts.images.length > 1) body.images = facts.images;
  if (facts.videos.length) body.videos = facts.videos;
  if (facts.audios.length) body.audios = facts.audios;
  if (facts.mode) body.mode = facts.mode;
  return { body, facts };
}

/** 生成秘塔视频创建请求；提交结果未知时禁止宿主自动重试。 */
export function buildSubmitRequest(ctx) {
  const built = upstreamBody(ctx.requestBody, ctx.upstreamModel || ctx.model);
  return {
    url: apiBase(ctx) + "/videos",
    method: "POST",
    headers: { Authorization: "Bearer " + ctx.apiKey, "Content-Type": "application/json", Accept: "application/json" },
    body: built.body,
    noRetry: true,
  };
}

/** 解析秘塔创建响应中的公开任务 ID。 */
export function parseSubmitResponse(_ctx, response) {
  const body = response && response.body;
  if (!body || typeof body !== "object" || Array.isArray(body)) throw new Error("upstream video response must be a JSON object");
  if (response.statusCode !== undefined && (response.statusCode < 200 || response.statusCode >= 300)) throw new Error("upstream video HTTP status " + response.statusCode);
  if (body.error) throw new Error("upstream rejected video creation");
  const taskId = body.id || body.task_id || body.request_id;
  if (typeof taskId !== "string" || !taskId.trim()) throw new Error("upstream video id is missing");
  return { taskId, taskData: body };
}

/** 返回提交阶段的冻结计费事实；倍率估算阶段不重复计费。 */
export function extractUsage(ctx) {
  const built = upstreamBody(ctx.requestBody, ctx.upstreamModel || ctx.model);
  if (ctx.usagePurpose === "billing_ratios") return {};
  return { seconds: built.facts.seconds, size: built.facts.size, input_images: built.facts.input_images, input_videos: built.facts.input_videos, input_audios: built.facts.input_audios };
}

/** 只采纳上游返回的有效时长，缺失时保留预扣事实。 */
export function extractUsageOnComplete(_task, result, body) {
  if (!result || result.status !== "SUCCESS") return null;
  const source = body && typeof body === "object" && !Array.isArray(body) ? body : {};
  if (source.seconds === undefined && source.duration === undefined) return null;
  if (source.seconds !== undefined && source.duration !== undefined && Number(source.seconds) !== Number(source.duration)) throw new Error("upstream seconds and duration conflict");
  const seconds = Number(source.seconds === undefined ? source.duration : source.seconds);
  if (!Number.isSafeInteger(seconds) || seconds < 1 || seconds > MAX_SECONDS) throw new Error("upstream duration must be an integer between 1 and 3600");
  return { seconds };
}

/** 构造秘塔任务查询请求。 */
export function buildQueryRequest(ctx) {
  return { url: apiBase(ctx) + "/videos/" + encodeURIComponent(ctx.taskId), method: "GET", headers: { Authorization: "Bearer " + ctx.apiKey, Accept: "application/json" } };
}

/** 映射秘塔异步任务状态；未知状态保持未知，避免误报成功。 */
export function parseTaskResult(_ctx, body) {
  if (!body || typeof body !== "object" || Array.isArray(body)) return { status: "UNKNOWN", reason: "invalid video response" };
  const raw = trimmed(body.status).toLowerCase();
  const status = body.error ? "FAILURE" : {
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
  if (status === "FAILURE") result.reason = "upstream video generation failed";
  if (status === "SUCCESS") result.progress = "100%";
  else {
    const progress = Number(body.progress);
    if (Number.isFinite(progress) && progress >= 0 && progress < 100) result.progress = progress + "%";
  }
  return result;
}

/** 成功任务声明一个视频制品，由宿主通过 content 接口代理下载。 */
export function listArtifacts(task) {
  return task.status === "SUCCESS" ? [{ key: "video", type: "video", mimeType: "video/mp4" }] : [];
}

/** 构造秘塔视频内容下载请求，不把供应商 URL 或凭据暴露给客户端。 */
export function buildContentRequest(ctx) {
  if (ctx.artifactKey !== "video") throw new Error("artifact_not_found");
  const taskId = ctx.upstreamTaskId || ctx.taskId;
  if (!taskId) throw new Error("upstream video id is missing");
  return {
    url: apiBase(ctx) + "/videos/" + encodeURIComponent(taskId) + "/content",
    method: ctx.clientRequest.method,
    headers: { Authorization: "Bearer " + ctx.apiKey, Accept: "video/*" },
  };
}

/** 注册 OpenAI 视频入口；模型身份始终保持 Minimax-H3，不添加 sora-2 映射。 */
export const protocols = {
  openai_video: {
    decodeRequest: function (ctx) {
      const request = decodedBody(ctx.body);
      const built = upstreamBody(request, ctx.upstreamModel || ctx.model);
      const action = built.facts.videos.length || built.facts.audios.length || built.facts.images.length > 1 ? "reference_to_video" : built.facts.images.length ? "image_to_video" : "text_to_video";
      return { kind: "submit", model: ctx.model, action: action, requestBody: Object.assign({}, request, { model: ctx.model }) };
    },
    render: function (_ctx, task) {
      const statuses = { NOT_START: "queued", SUBMITTED: "queued", QUEUED: "queued", IN_PROGRESS: "in_progress", SUCCESS: "completed", FAILURE: "failed" };
      return { id: task.task_id, object: "video", model: (task.properties || {}).origin_model_name || MODEL, status: statuses[task.status] || "unknown" };
    },
  },
};
