/** SUB 统一 H3 视频插件；使用网关的 OpenAI 风格异步视频协议。 */
export const meta = {
  apiVersion: 1,
  key: "shumai-h3-video",
  name: "SUB MiniMax H3 Video",
  icon: "Hailuo.Color",
  description: {
    en: "SUB MiniMax H3 unified video generation with text, frame, reference, audio, and lip-sync modes",
    zh: "SUB MiniMax H3 统一视频生成，支持文生、首尾帧、多图参考、多图音频和口型同步",
  },
  version: "1.0.0",
  author: { name: "QuantumNous" },
  website: "https://shumai.siphot.com",
  baseUrl: "https://shumai.siphot.com",
  models: ["h3"],
  modelDiscovery: { protocol: "openai", path: "/v1/models" },
  fetchMode: "per_task",
  requiredCapabilities: ["task-submit-no-retry@1"],
  protocols: [{ name: "openai_responses", supports: ["stream", "sync", "background"] }, "openai_video"],
  usageSchema: {
    seconds: {
      type: "number",
      unit: "second",
      description: { en: "Video generation unit price", zh: "视频生成单价" },
    },
    resolution: {
      enum: ["480p", "768p", "1080p"],
      description: { en: "Output video resolution", zh: "输出视频分辨率" },
    },
    mode: {
      enum: ["text", "frames", "reference", "reference-15s", "image-audio", "image-audio-15s", "image-audio-quality", "image-audio-fast", "lipsync"],
      description: { en: "Video generation mode", zh: "视频生成模式" },
    },
  },
  usageExamples: [
    { label: "text · 480p · 5s", facts: { seconds: 5, resolution: "480p", mode: "text" } },
    { label: "frames · 768p · 10s", facts: { seconds: 10, resolution: "768p", mode: "frames" } },
    { label: "reference · 1080p · 10s", facts: { seconds: 10, resolution: "1080p", mode: "reference" } },
  ],
};

const MODEL = "h3";
const DEFAULT_DURATION = 5;
const MAX_SEED = 2147483647;
const MODE_ALIASES = {
  text: "text",
  h3: "text",
  "h3-text": "text",
  frames: "frames",
  "h3-frames": "frames",
  "first-last-frame": "frames",
  first_last_frame: "frames",
  reference: "reference",
  "h3-reference": "reference",
  "multi-image": "reference",
  multi_image: "reference",
  "reference-15s": "reference-15s",
  "multi-image-15s": "reference-15s",
  multi_image_15s: "reference-15s",
  "image-audio": "image-audio",
  "multi-image-audio": "image-audio",
  multi_image_audio: "image-audio",
  "image-audio-15s": "image-audio-15s",
  "multi-image-audio-15s": "image-audio-15s",
  multi_image_audio_15s: "image-audio-15s",
  "image-audio-quality": "image-audio-quality",
  "image-audio-hq": "image-audio-quality",
  image_audio_quality: "image-audio-quality",
  "multi-image-audio-quality": "image-audio-quality",
  u24: "image-audio-quality",
  "image-audio-fast": "image-audio-fast",
  image_audio_fast: "image-audio-fast",
  "multi-image-audio-fast": "image-audio-fast",
  u08: "image-audio-fast",
  lipsync: "lipsync",
  "h3-lipsync": "lipsync",
  "lip-sync": "lipsync",
  lip_sync: "lipsync",
};
const MODE_LIMITS = {
  text: { maxDuration: 15 },
  frames: { maxDuration: 15, minImages: 2, maxImages: 2 },
  reference: { maxDuration: 10, minImages: 1, maxImages: 9 },
  "reference-15s": { fixedDuration: 15, minImages: 1, maxImages: 9 },
  "image-audio": { maxDuration: 10, minImages: 1, maxImages: 9, minAudios: 1, maxAudios: 3 },
  "image-audio-15s": { fixedDuration: 15, minImages: 1, maxImages: 9, minAudios: 1, maxAudios: 3 },
  "image-audio-quality": { fixedDuration: 15, minImages: 1, maxImages: 9, minAudios: 1, maxAudios: 3 },
  "image-audio-fast": { fixedDuration: 15, minImages: 1, maxImages: 9, minAudios: 1, maxAudios: 3 },
  lipsync: { fixedDuration: 15, minImages: 1, maxImages: 9, minAudios: 1, maxAudios: 3 },
};

/** 去除首尾空白；协议字段为空时统一按未提供处理。 */
function trimmed(value) {
  return typeof value === "string" ? value.trim() : String(value || "").trim();
}

/** 规范化渠道 Base URL，并保留管理员配置的 `/v1` 前缀。 */
function apiBase(ctx) {
  const base = trimmed(ctx.baseUrl).replace(/\/+$/, "");
  if (!/^https?:\/\/[^/?#]+(?:\/[^?#]*)?$/.test(base)) throw new Error("channel base URL must be an HTTP(S) API base without query or fragment");
  return /\/v1$/i.test(base) ? base : base + "/v1";
}

/** 将公开媒体值转换为 SUB 要求的 URL 字符串。 */
function mediaURL(value, field) {
  let url = value;
  if (value && typeof value === "object" && !Array.isArray(value)) {
    url = value.url || value[field === "images" ? "image_url" : "audio_url"];
    if (url && typeof url === "object" && !Array.isArray(url)) url = url.url;
  }
  if (typeof url !== "string" || !/^https?:\/\/[^\s]+$/i.test(url.trim())) throw new Error(field + " items must be public HTTP(S) URLs");
  return url.trim();
}

/** 读取数组、单值或 multipart 中的 JSON 数组，并限制为 URL 媒体。 */
function mediaList(request, plural, singular) {
  let raw = request[plural];
  if (raw === undefined) raw = request[singular];
  if (raw === undefined || raw === null || raw === "") return [];
  if (typeof raw === "string" && raw.trim().startsWith("[")) {
    try {
      raw = JSON.parse(raw);
    } catch {
      throw new Error(plural + " must be a JSON array");
    }
  }
  const values = Array.isArray(raw) ? raw : [raw];
  return values.map(function (value) {
    return mediaURL(value, plural);
  });
}

/** 规范化 SUB 文档中的模式别名。 */
function normalizeMode(request) {
  const raw = trimmed(request.mode || request.workflow_mode || "text").toLowerCase();
  const mode = MODE_ALIASES[raw];
  if (!mode) throw new Error("unsupported H3 video mode");
  return mode;
}

/** 把标签或像素尺寸转换为 SUB 接受的方向清晰度及计费档位。 */
function normalizeResolutionValue(value) {
  const raw = trimmed(value).toLowerCase();
  if (!raw) return null;
  const aliases = {
    "480p": "480p竖",
    "480p竖": "480p竖",
    "480p纵": "480p竖",
    "480p横": "480p横",
    "480p(1:1)": "480p(1:1)",
    "480p1:1": "480p(1:1)",
    "480p方": "480p(1:1)",
    "768p": "768p竖",
    "768p竖": "768p竖",
    "768p纵": "768p竖",
    "768p横": "768p横",
    "768p(1:1)": "768p(1:1)",
    "768p1:1": "768p(1:1)",
    "768p方": "768p(1:1)",
    "1080p": "1080p竖",
    "1080p竖": "1080p竖",
    "1080p纵": "1080p竖",
    "1080p横": "1080p横",
    "1080p(1:1)": "1080p(1:1)",
    "1080p1:1": "1080p(1:1)",
    "1080p方": "1080p(1:1)",
  };
  if (aliases[raw]) return aliases[raw];
  const dimensions = /^(\d{1,4})x(\d{1,4})$/.exec(raw);
  if (dimensions) {
    const shortSide = Math.min(Number(dimensions[1]), Number(dimensions[2]));
    if (shortSide === 480) return "480p竖";
    if (shortSide === 768) return "768p竖";
    if (shortSide === 1080) return "1080p竖";
  }
  throw new Error("unsupported H3 video resolution");
}

/** 返回清晰度的计费档位，不把方向后缀写入价格事实。 */
function resolutionTier(value) {
  const normalized = normalizeResolutionValue(value);
  return normalized.slice(0, normalized.indexOf("p") + 1);
}

/** 读取秒数，拒绝冲突、浮点、溢出和模式不支持的时长。 */
function durationValue(request, mode) {
  if (request.seconds !== undefined && request.duration !== undefined && Number(request.seconds) !== Number(request.duration)) throw new Error("seconds and duration conflict");
  const raw = request.duration === undefined ? request.seconds : request.duration;
  const duration = raw === undefined || raw === null || raw === "" ? DEFAULT_DURATION : Number(raw);
  const limit = MODE_LIMITS[mode];
  if (!Number.isSafeInteger(duration) || duration < 1 || duration > (limit.fixedDuration || limit.maxDuration)) throw new Error("duration must be an integer within the selected H3 mode limits");
  if (limit.fixedDuration && duration !== limit.fixedDuration) throw new Error(mode + " requires a fixed 15 second duration");
  return duration;
}

/** 校验种子，避免将非整数或超范围值发送到上游。 */
function seedValue(request) {
  if (request.seed === undefined || request.seed === null || request.seed === "") return undefined;
  const seed = Number(request.seed);
  if (!Number.isSafeInteger(seed) || Math.abs(seed) > MAX_SEED) throw new Error("seed must be a 32-bit integer");
  return seed;
}

/** 生成并校验 SUB 统一入口请求及其计费事实。 */
function normalizeRequest(request, model) {
  if (!request || typeof request !== "object" || Array.isArray(request)) throw new Error("request body must be an object");
  if (model !== MODEL) throw new Error("unsupported H3 model");
  const mode = normalizeMode(request);
  const limit = MODE_LIMITS[mode];
  const duration = durationValue(request, mode);
  const rawResolution = request.resolution === undefined ? request.size : request.resolution;
  if (request.resolution !== undefined && request.size !== undefined && resolutionTier(request.resolution) !== resolutionTier(request.size)) throw new Error("resolution and size conflict");
  const resolution = normalizeResolutionValue(rawResolution || "480p");
  const tier = resolutionTier(resolution);
  if (tier === "1080p" && !["reference", "image-audio", "lipsync"].includes(mode)) throw new Error("1080p is only supported by reference, image-audio, and lipsync modes");
  const images = mediaList(request, "images", "image");
  const audios = mediaList(request, "audios", "audio");
  if (images.length < (limit.minImages || 0) || images.length > (limit.maxImages || 0)) throw new Error(mode + " requires between " + limit.minImages + " and " + limit.maxImages + " images");
  if (audios.length < (limit.minAudios || 0) || audios.length > (limit.maxAudios || 0)) throw new Error(mode + " requires between " + limit.minAudios + " and " + limit.maxAudios + " audios");
  if (mode === "text" && (images.length || audios.length)) throw new Error("text mode does not accept image or audio inputs");
  const prompt = trimmed(request.prompt);
  if (mode === "text" && !prompt) throw new Error("text mode requires a prompt");
  const seed = seedValue(request);
  return { mode, duration, resolution, tier, images, audios, prompt, seed };
}

/** 把请求体转换为 SUB 允许的白名单字段，避免 metadata 绕过计费校验。 */
function upstreamBody(request, model) {
  const facts = normalizeRequest(request, model);
  const body = { model: MODEL, mode: facts.mode, duration: facts.duration, resolution: facts.resolution };
  if (facts.prompt && facts.mode !== "lipsync") body.prompt = facts.prompt;
  if (facts.images.length) body.images = facts.images;
  if (facts.audios.length) body.audios = facts.audios;
  if (facts.seed !== undefined) body.seed = facts.seed;
  return { body, facts };
}

/** 解析 JSON、表单或 multipart 视频请求；SUB 不提供文件上传合同。 */
function decodedRequest(body) {
  if (!body || (body.kind !== "json" && body.kind !== "multipart" && body.kind !== "form")) throw new Error("JSON, form, or multipart body required");
  if (body.kind === "json") return body.value;
  if ((body.files || []).length) throw new Error("SUB H3 requires public media URLs; file uploads are unsupported");
  const request = {};
  for (const key of Object.keys(body.fields || {})) {
    const values = body.fields[key];
    if (!Array.isArray(values) || values.length !== 1) throw new Error(key + " must be provided once");
    let value = values[0];
    if (["images", "image", "audios", "audio"].includes(key) && typeof value === "string" && value.trim().startsWith("[")) {
      try {
        value = JSON.parse(value);
      } catch {
        throw new Error(key + " must be a JSON array");
      }
    }
    request[key] = value;
  }
  return request;
}

/** 从 Responses input 提取文本及图片，并保留视频参数。 */
function responsesRequest(request) {
  const input = request.input;
  const texts = [];
  const images = [];
  if (typeof input === "string") texts.push(input);
  else if (Array.isArray(input)) {
    for (const item of input) {
      if (typeof item === "string") {
        texts.push(item);
        continue;
      }
      if (!item || typeof item !== "object" || Array.isArray(item)) continue;
      const parts = item.content === undefined ? [item] : Array.isArray(item.content) ? item.content : [item.content];
      for (const part of parts) {
        if (typeof part === "string") texts.push(part);
        else if (part && typeof part === "object") {
          if (["input_text", "text"].includes(part.type) && typeof part.text === "string") texts.push(part.text);
          if (["input_image", "image_url"].includes(part.type)) images.push(mediaURL(part.image_url || part.url, "images"));
        }
      }
    }
  } else throw new Error("video Responses input must be a string or array");
  const result = { prompt: texts.filter(function (text) { return trimmed(text); }).join("\n") };
  if (images.length) result.images = images;
  for (const key of ["mode", "workflow_mode", "duration", "seconds", "resolution", "size", "audios", "audio", "seed"]) {
    if (request[key] !== undefined) result[key] = request[key];
  }
  return result;
}

/** 返回经过宿主授权的视频制品 HTML，不暴露渠道凭据或临时地址。 */
function videoOutput(ctx) {
  const artifact = ctx && ctx.artifacts && ctx.artifacts.video;
  const url = trimmed(artifact && artifact.url);
  if (!url) throw new Error("video artifact is unavailable");
  const escaped = url.replace(/&/g, "&amp;").replace(/"/g, "&quot;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  return '<video controls src="' + escaped + '"></video>';
}

/** 构造 SUB 统一入口的创建请求；提交响应未知时宿主不得自动重试。 */
export function buildSubmitRequest(ctx) {
  const built = upstreamBody(ctx.requestBody, ctx.upstreamModel || ctx.model);
  return {
    url: apiBase(ctx) + "/videos/generations",
    method: "POST",
    headers: { Authorization: "Bearer " + ctx.apiKey, "Content-Type": "application/json", Accept: "application/json" },
    body: built.body,
    noRetry: true,
  };
}

/** 解析 SUB 返回的公开任务 ID；只接受明确的 JSON 成功对象。 */
export function parseSubmitResponse(_ctx, response) {
  const body = response && response.body;
  if (!body || typeof body !== "object" || Array.isArray(body)) throw new Error("upstream video response must be a JSON object");
  if (response.statusCode !== undefined && (response.statusCode < 200 || response.statusCode >= 300)) throw new Error("upstream video HTTP status " + response.statusCode);
  if (body.error) throw new Error("upstream rejected video creation");
  const taskId = body.id || body.request_id || body.task_id;
  if (typeof taskId !== "string" || !taskId.trim()) throw new Error("upstream video id is missing");
  return { taskId, taskData: body };
}

/** 返回提交阶段确定的计费事实；比例预估阶段不重复增加倍率。 */
export function extractUsage(ctx) {
  const built = upstreamBody(ctx.requestBody, ctx.upstreamModel || ctx.model);
  if (ctx.usagePurpose === "billing_ratios") return {};
  return { seconds: built.facts.duration, resolution: built.facts.tier, mode: built.facts.mode };
}

/** 仅采纳上游明确返回且有界的实际时长，缺失时保留预扣事实。 */
export function extractUsageOnComplete(_task, result, body) {
  if (!result || result.status !== "SUCCESS") return null;
  const source = body && typeof body === "object" && !Array.isArray(body) ? body : {};
  if (source.seconds === undefined && source.duration === undefined) return null;
  if (source.seconds !== undefined && source.duration !== undefined && Number(source.seconds) !== Number(source.duration)) throw new Error("upstream seconds and duration conflict");
  const value = Number(source.seconds === undefined ? source.duration : source.seconds);
  if (!Number.isSafeInteger(value) || value < 1 || value > 15) throw new Error("upstream duration must be an integer between 1 and 15");
  return { seconds: value };
}

/** 构造统一入口轮询地址，任务 ID 只作为路径参数编码。 */
export function buildQueryRequest(ctx) {
  return { url: apiBase(ctx) + "/videos/generations/" + encodeURIComponent(ctx.taskId), method: "GET", headers: { Authorization: "Bearer " + ctx.apiKey, Accept: "application/json" } };
}

/** 映射 SUB 的异步状态；未知状态不能默认为处理中或成功。 */
export function parseTaskResult(_ctx, body) {
  if (!body || typeof body !== "object" || Array.isArray(body)) return { status: "UNKNOWN", reason: "invalid video response" };
  const status = body.error ? "FAILURE" : {
    queued: "QUEUED",
    pending: "QUEUED",
    running: "IN_PROGRESS",
    processing: "IN_PROGRESS",
    in_progress: "IN_PROGRESS",
    succeeded: "SUCCESS",
    completed: "SUCCESS",
    failed: "FAILURE",
    cancelled: "FAILURE",
    canceled: "FAILURE",
    expired: "FAILURE",
  }[trimmed(body.status).toLowerCase()];
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

/** 固定使用 SUB content 接口下载，避免把上游地址或渠道密钥暴露给客户端。 */
export function buildContentRequest(ctx) {
  if (ctx.artifactKey !== "video") throw new Error("artifact_not_found");
  const taskId = ctx.upstreamTaskId || ctx.taskId;
  if (!taskId) throw new Error("upstream video id is missing");
  return {
    url: apiBase(ctx) + "/videos/generations/" + encodeURIComponent(taskId) + "/content",
    method: ctx.clientRequest.method,
    headers: { Authorization: "Bearer " + ctx.apiKey, Accept: "video/*" },
  };
}

/** 注册 OpenAI 视频协议，统一入口的原生模式由请求体选择。 */
export const protocols = {
  openai_video: {
    decodeRequest: function (ctx) {
      const request = decodedRequest(ctx.body);
      const model = ctx.upstreamModel || ctx.model;
      const built = upstreamBody(request, model);
      return { kind: "submit", model: ctx.model, action: built.facts.mode === "text" ? "text_to_video" : "image_to_video", requestBody: Object.assign({}, request, { model: ctx.model }) };
    },
    render: function (_ctx, task) {
      const statuses = { NOT_START: "queued", SUBMITTED: "queued", QUEUED: "queued", IN_PROGRESS: "in_progress", SUCCESS: "completed", FAILURE: "failed" };
      return { id: task.task_id, object: "video", model: (task.properties || {}).origin_model_name || MODEL, status: statuses[task.status] || "unknown", progress: Number(String(task.progress || "0").replace("%", "")) };
    },
  },
  openai_responses: {
    decodeRequest: function (ctx) {
      const request = ctx.body && ctx.body.kind === "json" && ctx.body.value;
      if (!request || typeof request !== "object" || Array.isArray(request)) throw new Error("video Responses body must be an object");
      for (const key of ["tools", "tool_choice", "previous_response_id", "conversation", "instructions"]) {
        if (request[key] !== undefined && request[key] !== null) throw new Error("video Responses does not support " + key);
      }
      const normalized = responsesRequest(request);
      const model = ctx.upstreamModel || ctx.model;
      const built = upstreamBody(normalized, model);
      return { kind: "submit", model: ctx.model, action: built.facts.mode === "text" ? "text_to_video" : "image_to_video", requestBody: Object.assign({}, normalized, { model: ctx.model }) };
    },
    renderEvents: function (ctx, task, previousState) {
      const status = task.status || "UNKNOWN";
      const state = { status: status, progress: task.progress || "" };
      if (status === "SUCCESS") return { events: previousState && previousState.status === status ? [] : [{ type: "output", data: videoOutput(ctx) }], state: state, done: true };
      if (status === "FAILURE") return { events: [{ type: "error", code: "video_generation_failed", message: "Video generation failed" }], state: state, done: true };
      return { events: previousState && previousState.status === status && previousState.progress === state.progress ? [] : [{ type: "progress", message: status.toLowerCase() }], state: state, done: false };
    },
    renderFinal: function (ctx) {
      return { output: [{ type: "message", status: "completed", role: "assistant", content: [{ type: "output_text", text: videoOutput(ctx), annotations: [], logprobs: [] }] }], metadata: { vendor: "shumai-h3-video" } };
    },
  },
};
