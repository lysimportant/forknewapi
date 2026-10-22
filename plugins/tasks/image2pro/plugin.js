/** Canvas 和既有渠道使用展示名；供应商创建接口必须发送目录中的真实模型 ID。 */
const SEEDANCE_MODEL = "Seedance2.0 0.9r";
const FLASH_MODEL = "无限制-Flash-MAX-Video";
const LEGACY_IMAGE_MODEL_PREFIX = "Image2Pro-Image-";
const LEGACY_VIDEO_MODEL_PREFIX = "Image2Pro-Video-";
/** 保留既有展示名建议，并允许模型目录导入真实 ID；两组名称最终发送同一真实 ID。 */
const VIDEO_MODELS = [SEEDANCE_MODEL, FLASH_MODEL, "Seedance2.0", "Flash视频-MAX"];
/** 公开目录中的图片候选；文档未逐项保证其均已接入异步图片接口。 */
const IMAGE_MODELS = [
  "无限制-Seedream-5-Pro",
  "无限制-Seedream-5-Pro-2K",
  "无限制-Qwen-2.1NEW",
  "完全无限制-Z-Image",
  "无限制-NovelAI-V5",
  "无限制-NovelAI-V4.5",
  "gpt-image-2.5-sunburst",
  "gpt-image-2.5-flare",
  "gpt-image-2.0",
  "gpt-image-2.0-原生4K",
  "gpt-image-2.5-不超分",
  "gpt-image-2.0-不超分",
  "Nano-banana-2.1",
  "Nano-banana-Pro",
  "Nano-banana-2",
  "Grok-image-2.0",
  "Flash会员专属",
  "Kolors-极速生图",
  "无限制-Z-Image",
];
const MODELS = VIDEO_MODELS.concat(IMAGE_MODELS);
const MODEL_SPECS = {
  [SEEDANCE_MODEL]: { upstream: "Seedance2.0", durationMin: 4, durationMax: 15, imageLimit: 9 },
  "Seedance2.0": { upstream: "Seedance2.0", durationMin: 4, durationMax: 15, imageLimit: 9 },
  [FLASH_MODEL]: { upstream: "Flash视频-MAX", durationMin: 4, durationMax: 12, imageLimit: 5 },
  "Flash视频-MAX": { upstream: "Flash视频-MAX", durationMin: 4, durationMax: 12, imageLimit: 5 },
};
/** 未登记的新视频型号使用有限通用边界，具体型号能力仍由上游校验。 */
const GENERIC_SPEC = { durationMin: 1, durationMax: 3600, imageLimit: 9 };
/** 兼容的参考图入口；内部先归一为 content，供应商出站再转换为公开视频字段。 */
const IMAGE_FIELDS = ["input_image", "images", "input_reference", "image", "reference_images", "image_urls"];
/** 兼容历史媒体入口；本接口支持公网参考图片与音频 mediaId。 */
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

/** Image2Pro 插件声明；Seedance 按次、Flash 按秒，价格由管理员配置，不换算上游积分。 */
export const meta = {
  apiVersion: 1,
  key: "image2pro",
  name: "Image2Pro",
  icon: "text:I2P",
  version: "2.4.3",
  author: { name: "lysimportant/forknewapi" },
  description: {
    en: "Image2Pro image and video generation with dynamic model discovery and reference audio",
    zh: "Image2Pro 动态图片及视频模型，支持参考图片与音频",
  },
  baseUrl: "https://api.image2pro.top/v1",
  website: "https://image2pro.top/api-docs",
  models: MODELS,
  modelMediaTypes: Object.fromEntries(VIDEO_MODELS.map((model) => [model, "video"]).concat(IMAGE_MODELS.map((model) => [model, "image"]))),
  modelDiscovery: {
    protocol: "openai",
    path: "/v1/models",
    modelTypes: ["image", "video"],
  },
  modelRouting: "channel",
  fetchMode: "per_task",
  requiredCapabilities: ["task-submit-no-retry@1"],
  protocols: ["openai_video", { name: "openai_responses", supports: ["stream", "sync", "background"] }, "openai_image_tasks"],
  usageSchema: {
    count: { type: "number", unit: "count", unitLabel: { en: "request", zh: "次" }, description: { en: "Generation unit price", zh: "生成单价" } },
    seconds: { type: "number", unit: "second", description: { en: "Video generation unit price", zh: "视频生成单价" } },
  },
  usageProfiles: [
    {
      models: [SEEDANCE_MODEL, "Seedance2.0"],
      schema: {
        count: {
          type: "number",
          unit: "count",
          unitLabel: { en: "request", zh: "次" },
          description: { en: "Video generation unit price", zh: "视频生成单价" },
        },
      },
    },
    {
      models: [FLASH_MODEL, "Flash视频-MAX"],
      schema: {
        seconds: { type: "number", unit: "second", description: { en: "Video generation unit price", zh: "视频生成单价" } },
      },
    },
    {
      models: IMAGE_MODELS,
      modelTypes: ["image"],
      schema: {
        count: {
          type: "number",
          unit: "count",
          unitLabel: { en: "image", zh: "张" },
          description: { en: "Image generation unit price", zh: "图片生成单价" },
        },
      },
    },
  ],
};

/** 先排除 data URL，避免扫描整段 Base64；HTTP(S) 地址仍拒绝内嵌凭据、控制字符和反斜杠。 */
function isHTTPURL(value) {
  if (typeof value !== "string" || !/^https?:\/\//i.test(value) || /[\s\\]/.test(value)) return false;
  for (const character of value) if (character.charCodeAt(0) < 32 || character.charCodeAt(0) === 127) return false;
  return /^https?:\/\/(?:\[[0-9a-f:.]+\]|[a-z0-9.-]+)(?::[0-9]+)?(?:[/?#].*)?$/i.test(value);
}

function isHTTPSURL(value) {
  return typeof value === "string" && /^https:\/\//i.test(value) && isHTTPURL(value);
}

function hasControlCharacters(value) {
  if (typeof value !== "string") return false;
  for (const character of value) {
    const code = character.charCodeAt(0);
    if (code < 32 || code === 127) return true;
  }
  return false;
}

/** 兼容渠道根地址及 /v1 地址，保留自定义路径前缀且不重复版本段。 */
function apiBase(ctx) {
  const base = typeof ctx.baseUrl === "string" ? ctx.baseUrl.replace(/\/+$/, "") : "";
  if (!isHTTPURL(base) || /[?#]/.test(base)) throw new Error("Image2Pro channel base URL must be an HTTP(S) URL without credentials, query, or fragment");
  return base.endsWith("/v1") ? base : base + "/v1";
}

function isImageModel(model) {
  return typeof model === "string" && (IMAGE_MODELS.includes(model) || model.startsWith(LEGACY_IMAGE_MODEL_PREFIX));
}

function isVideoModel(model) {
  return typeof model === "string" && (Object.prototype.hasOwnProperty.call(MODEL_SPECS, model) || model.startsWith(LEGACY_VIDEO_MODEL_PREFIX));
}

function isImageOperation(ctx) {
  const request = ctx && ctx.requestBody;
  const state = ctx && ctx.state;
  return Boolean(
    (request && request["__imageTask"] === true) ||
    (state && state.kind === "image") ||
    isImageModel(ctx && ctx.model) ||
    isImageModel(ctx && ctx.upstreamModel) ||
    isImageModel(request && request.model)
  );
}

/** 目录映射保留原始 upstreamModel，图片类型由接口与内部标记传递。 */
function isImageContext(ctx) {
  return Boolean(ctx && (isImageOperation(ctx) || ctx.protocol === "openai_image_tasks" || ctx.operation === "image" || ctx.action === "image_generation"));
}

function upstreamModelName(ctx, fallback) {
  const model = (ctx && (ctx.upstreamModel || ctx.model)) || fallback;
  if (typeof model !== "string" || !model.trim()) throw new Error("Image2Pro model is required");
  const spec = Object.prototype.hasOwnProperty.call(MODEL_SPECS, model) ? MODEL_SPECS[model] : null;
  return spec ? spec.upstream : model;
}

const IMAGE_REQUEST_FIELDS = [
  "model",
  "prompt",
  "n",
  "size",
  "aspect_ratio",
  "ratio",
  "resolution",
  "image_size",
  "input_image",
  "reference_image",
  "image",
  "images",
  "mask",
  "response_format",
];

/** 图片异步接口仅接受公开 JSON 字段；不接收 multipart 或兼容性透传参数。 */
function normalizeImageRequest(source, model, allowInternal = false) {
  if (typeof model !== "string" || !model.trim() || isVideoModel(model)) throw new Error("unsupported Image2Pro image model");
  if (!source || typeof source !== "object" || Array.isArray(source)) throw new Error("Image2Pro image request body must be an object");
  for (const key of Object.keys(source))
    if (!IMAGE_REQUEST_FIELDS.includes(key) && !(allowInternal && key === "__imageTask" && source[key] === true))
      throw new Error("unsupported Image2Pro image field: " + key);
  if (source.model !== model) throw new Error("Image2Pro image model does not match the selected model");
  if (typeof source.prompt !== "string" || !source.prompt.trim()) throw new Error("Image2Pro image prompt is required");
  if (source.n !== undefined && (!Number.isInteger(source.n) || source.n < 1 || source.n > 4))
    throw new Error("Image2Pro image n must be an integer from 1 to 4");
  if (source.response_format !== undefined && source.response_format !== "url") throw new Error("Image2Pro image response_format must be url");
  return { ...source };
}

function requestedImageCount(request) {
  const count = request && request.n !== undefined ? request.n : 1;
  if (!Number.isInteger(count) || count < 1 || count > 4) throw new Error("Image2Pro image n must be an integer from 1 to 4");
  return count;
}

function validTaskId(value) {
  return typeof value === "string" && value !== "." && value !== ".." && /^[A-Za-z0-9_.:-]{1,200}$/.test(value);
}

function imageResultURLs(body) {
  if (!body || !Array.isArray(body.data) || body.data.length === 0) return null;
  const urls = [];
  for (const item of body.data) {
    if (!item || typeof item !== "object" || Array.isArray(item) || !isHTTPURL(item.url)) return null;
    urls.push(item.url);
  }
  return urls;
}

/** 读取有界秒数并兼容旧任务冻结的小数时长；新建请求另按精确模型范围校验。 */
function requestedSeconds(request) {
  if (request.seconds === undefined && request.duration === undefined) throw new Error("seconds or duration is required for Image2Pro video generation");
  for (const field of ["seconds", "duration"]) {
    const value = request[field];
    if (value === undefined) continue;
    if (value === -1 || value === "-1") throw new Error("automatic duration is unavailable for Image2Pro video generation");
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

/** 旧型号保留专用限制，目录新增视频型号采用通用边界。 */
function modelSpec(model) {
  return Object.prototype.hasOwnProperty.call(MODEL_SPECS, model) ? MODEL_SPECS[model] : GENERIC_SPEC;
}

/** 校验媒体与上传占位符；参考音频仅接受 HTTPS URL 或 mediaId。 */
function mediaReference(value, type, files, model) {
  modelSpec(model);
  const rule = MEDIA_RULES[type];
  if (value && typeof value === "object" && !Array.isArray(value)) {
    if (type === "audio" && Object.keys(value).length === 1 && typeof value.mediaId === "string" && /^[A-Za-z0-9][A-Za-z0-9_.:-]{0,199}$/.test(value.mediaId))
      return { mediaId: value.mediaId };
    if (Object.prototype.hasOwnProperty.call(value, "__fileRef")) {
      if (type === "audio") throw new Error("Image2Pro audio references require an HTTPS URL or mediaId");
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
  if (type === "audio" && isHTTPSURL(value)) return value;
  if (type !== "audio" && isHTTPURL(value)) return value;
  if (type === "audio") throw new Error("Image2Pro audio references require an HTTPS URL or mediaId");
  const data = typeof value === "string" ? /^data:([^;,]+);base64,((?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?)$/.exec(value) : null;
  if (data && data[2] && rule.mimes.includes(data[1])) return value;
  throw new Error("Image2Pro " + type + " references must be HTTP(S) URLs or supported " + type + " data URLs");
}

/** 校验兼容 content，保留文字与图片顺序、重复引用和角色。 */
function normalizeContent(content, files, model) {
  const spec = modelSpec(model);
  const name =
    model === FLASH_MODEL || model === "Flash视频-MAX"
      ? "Image2Pro Flash-MAX"
      : Object.prototype.hasOwnProperty.call(MODEL_SPECS, model)
        ? "Image2Pro Seedance"
        : "Image2Pro video";
  if (!Array.isArray(content) || content.length === 0) throw new Error(name + " content must be a non-empty array");
  const counts = { image: 0, video: 0, audio: 0 };
  const seenAudios = new Set();
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
    if (
      !reference ||
      typeof reference !== "object" ||
      Array.isArray(reference) ||
      Object.keys(reference).some((field) => field !== "url" && field !== "mediaId") ||
      (item.type === "audio_url" && Object.keys(reference).length !== 1) ||
      (item.type !== "audio_url" && Object.keys(reference).some((field) => field !== "url"))
    )
      throw new Error(name + " media content requires a URL or mediaId object without extra fields");
    const type = item.type.slice(0, -4);
    const role = item.role === undefined ? "reference_" + type : item.role;
    if (role !== "reference_" + type) throw new Error("unsupported " + name + " " + type + " role");
    counts[type]++;
    if (type === "image" && counts.image > spec.imageLimit) throw new Error(name + " supports at most " + spec.imageLimit + " reference images");
    if (type === "audio" && counts.audio > MEDIA_RULES.audio.limit)
      throw new Error(name + " supports at most " + MEDIA_RULES.audio.limit + " reference audios");
    if (type === "video") throw new Error("Image2Pro public video API does not support video references");
    const rawReference = type === "audio" && Object.prototype.hasOwnProperty.call(reference, "mediaId") ? { mediaId: reference.mediaId } : reference.url;
    const normalizedReference = mediaReference(rawReference, type, files, model);
    if (type === "audio") {
      const audioKey = normalizedReference && typeof normalizedReference === "object" ? "mediaId:" + normalizedReference.mediaId : "url:" + normalizedReference;
      if (seenAudios.has(audioKey)) throw new Error(name + " does not allow duplicate audio references");
      seenAudios.add(audioKey);
    }
    return {
      type: item.type,
      [item.type]: type === "audio" && normalizedReference && typeof normalizedReference === "object" ? normalizedReference : { url: normalizedReference },
      role,
    };
  });
  if (!hasText && !counts.image && !counts.audio) throw new Error("Image2Pro requires prompt or at least one reference image or audio");
  return normalized;
}

/** 兼容入口归一为内部 content；旧型号保留时长、比例和图片数量限制。 */
function normalizeRequest(source, model, files = []) {
  if (typeof model !== "string" || !model.trim() || isImageModel(model)) throw new Error("unsupported Image2Pro video model");
  if (!source || typeof source !== "object" || Array.isArray(source)) throw new Error("Image2Pro request body must be an object");
  const spec = modelSpec(model);
  for (const key of Object.keys(source)) if (!REQUEST_FIELDS.includes(key)) throw new Error("unsupported Image2Pro field: " + key);
  if (BOOLEAN_FIELDS.some((field) => source[field] !== undefined)) throw new Error("Image2Pro public video API does not support Seedance generation flags");
  const request = { model: source.model, duration: requestedSeconds(source) };
  if (!Number.isInteger(request.duration) || request.duration < spec.durationMin || request.duration > spec.durationMax)
    throw new Error("Image2Pro duration must be an integer from " + spec.durationMin + " to " + spec.durationMax);
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
        const audioMediaId = type === "audio" && Object.keys(reference).length === 1 && Object.prototype.hasOwnProperty.call(reference, "mediaId");
        if (
          Object.keys(reference).some((key) => !["url", "role", ...(type === "audio" ? ["mediaId"] : [])].includes(key)) ||
          (type === "audio" && Object.prototype.hasOwnProperty.call(reference, "mediaId") && !audioMediaId) ||
          (type === "audio" && !audioMediaId && !Object.prototype.hasOwnProperty.call(reference, "url"))
        )
          throw new Error("unsupported Image2Pro reference field");
        url = audioMediaId ? { mediaId: reference.mediaId } : reference.url;
        if (reference.role !== undefined) role = reference.role;
      }
      content.push({ type: type + "_url", [type + "_url"]: type === "audio" && url && typeof url === "object" ? url : { url }, role });
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
  if (content.some((item) => item.type === "audio_url" || item.type === "video_url")) return "reference_to_video";
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
function providerVideoBody(request, upstreamModel) {
  const body = { model: upstreamModel, duration: request.duration, ratio: request.ratio };
  let prompt = "";
  const images = [];
  const audios = [];
  for (const item of request.content) {
    if (item.type === "text") {
      prompt += item.text;
      continue;
    }
    if (item.type === "image_url") images.push(item.image_url.url);
    else if (item.type === "audio_url") {
      const audio = item.audio_url;
      audios.push(audio && Object.prototype.hasOwnProperty.call(audio, "mediaId") ? { mediaId: audio.mediaId } : audio.url);
    } else throw new Error("Image2Pro public video API does not support video references");
  }
  if (prompt.trim()) body.prompt = prompt;
  if (images.length === 1) body.input_image = images[0];
  else if (images.length > 1) body.images = images;
  if (audios.length) body.audios = audios;
  return body;
}

/** 构造单次 JSON 提交，上传占位符由宿主替换；幂等键与禁止重试避免未知结果重复扣费。 */
export function buildSubmitRequest(ctx) {
  const model = ctx.upstreamModel || ctx.model;
  if (isImageContext(ctx)) {
    const request = normalizeImageRequest(ctx.requestBody, model, true);
    delete request["__imageTask"];
    const key = ctx.publicTaskId;
    if (typeof key !== "string" || !/^[A-Za-z0-9][A-Za-z0-9_.:-]{0,99}$/.test(key))
      throw new Error("a valid NewAPI public task ID is required for Image2Pro image generation");
    return {
      url: apiBase(ctx) + "/images/tasks",
      method: "POST",
      headers: { Authorization: "Bearer " + ctx.apiKey, "Content-Type": "application/json", "Idempotency-Key": key },
      body: request,
      noRetry: true,
    };
  }
  const request = normalizeRequest(ctx.requestBody, model, ctx.files || []);
  request.model = model;
  const key = request.client_request_id === undefined ? ctx.publicTaskId : request.client_request_id;
  if (typeof key !== "string" || !/^[A-Za-z0-9_.-]{6,120}$/.test(key))
    throw new Error("a valid public task ID or client idempotency key is required for Image2Pro");
  delete request.client_request_id;
  const action = videoAction(request.content);
  const body = providerVideoBody(request, upstreamModelName(ctx, model));
  return {
    url: apiBase(ctx) + "/videos",
    method: "POST",
    headers: { Authorization: "Bearer " + ctx.apiKey, "Content-Type": "application/json", "Idempotency-Key": key },
    body,
    noRetry: true,
    action,
  };
}

/** 保存请求时长和新 Seedance 的按次标记；旧任务无标记时继续按原冻结秒数结算。 */
export function parseSubmitResponse(ctx, response) {
  const body = response && response.body;
  const model = ctx.upstreamModel || ctx.model;
  if (isImageContext(ctx)) {
    if (response && response.statusCode !== undefined && (response.statusCode < 200 || response.statusCode >= 300))
      throw new Error("Image2Pro image task creation failed");
    if (!body || typeof body !== "object" || Array.isArray(body) || !validTaskId(body.id)) throw new Error("Image2Pro image task response is invalid");
    if (!["queued", "processing", "completed", "failed"].includes(body.status)) throw new Error("Image2Pro returned an unrecognized image task status");
    if (body.status === "completed" && !imageResultURLs(body)) throw new Error("Image2Pro completed image task response is invalid");
    const result = { taskId: body.id, taskData: body, state: { kind: "image", n: requestedImageCount(ctx.requestBody || {}) } };
    if (body.status === "completed" || body.status === "failed") result.immediate = imageTaskResult(ctx, body);
    return result;
  }
  if (response && response.statusCode !== undefined && (response.statusCode < 200 || response.statusCode >= 300))
    throw new Error("Image2Pro video creation failed");
  if (!body || typeof body !== "object" || Array.isArray(body) || typeof body.id !== "string" || !/^[a-f0-9]{16,80}$/.test(body.id))
    throw new Error("Image2Pro video response must contain a valid hexadecimal task ID");
  if (!["processing", "completed", "failed"].includes(body.status)) throw new Error("Image2Pro returned an unrecognized video status");
  if (body.status !== "failed" && body.error) throw new Error("Image2Pro video creation failed");
  const state = { seconds: requestedSeconds(ctx.requestBody || {}) };
  if (model && modelSpec(model).upstream === "Seedance2.0") state.billingUnit = "count";
  const result = { taskId: body.id, taskData: body, state };
  if (body.status !== "processing") {
    const immediate = parseTaskResult({ state }, body);
    if (immediate.status === "UNKNOWN") throw new Error("Image2Pro completed video response is invalid");
    result.immediate = immediate;
  }
  return result;
}

/** Seedance 每个任务计一次，Flash 使用请求秒数；旧倍率入口不叠加用量或上游积分。 */
export function extractUsage(ctx) {
  const model = ctx.upstreamModel || ctx.model;
  if (isImageContext(ctx)) {
    if (ctx.usagePurpose === "billing_ratios") return {};
    return { count: requestedImageCount(ctx.requestBody || {}), seconds: 0 };
  }
  const request = normalizeRequest(ctx.requestBody, model, ctx.files || []);
  if (ctx.usagePurpose === "billing_ratios") return {};
  return modelSpec(model).upstream === "Seedance2.0" ? { count: 1 } : { seconds: request.duration };
}

/** 新 Seedance 完成时仍计一次；没有按次标记的旧任务保留原冻结秒数。 */
export function extractUsageOnComplete(task, result) {
  if (!result || result.status !== "SUCCESS") return null;
  const state = task.state || {};
  if (state.kind === "image" || isImageModel(task.upstreamModel || task.model)) return { count: requestedImageCount(state), seconds: 0 };
  const seconds = requestedSeconds(state);
  return state.billingUnit === "count" ? { count: 1 } : { seconds };
}

/** 查询只接受供应商十六进制任务 ID，鉴权固定使用当前渠道。 */
export function buildQueryRequest(ctx) {
  if (isImageContext(ctx)) {
    if (!validTaskId(ctx.taskId)) throw new Error("Image2Pro image task ID is invalid");
    return {
      url: apiBase(ctx) + "/images/tasks/" + encodeURIComponent(ctx.taskId),
      method: "GET",
      headers: { Authorization: "Bearer " + ctx.apiKey },
    };
  }
  if (typeof ctx.taskId !== "string" || !/^[a-f0-9]{16,80}$/.test(ctx.taskId))
    throw new Error("Image2Pro task ID must contain 16 to 80 hexadecimal characters");
  return { url: apiBase(ctx) + "/videos/" + ctx.taskId, method: "GET", headers: { Authorization: "Bearer " + ctx.apiKey } };
}

/** 映射文档明确状态；完成须有安全视频地址，未知状态不伪装为处理中。 */
export function parseTaskResult(ctx, body) {
  if (isImageContext(ctx)) return imageTaskResult(ctx, body);
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

/** 仅把文档状态映射为宿主状态；供应商错误文本和凭据不进入任务记录。 */
function imageTaskResult(ctx, body) {
  if (!body || typeof body !== "object" || Array.isArray(body)) return { status: "UNKNOWN", reason: "invalid Image2Pro image task response" };
  const status = { queued: "QUEUED", processing: "IN_PROGRESS", completed: "SUCCESS", failed: "FAILURE" }[body.status];
  if (!status) return { status: "UNKNOWN", reason: "unrecognized Image2Pro image task status" };
  const result = { status };
  if (status === "FAILURE") result.reason = "Image2Pro image generation failed";
  else if (status === "SUCCESS") {
    const urls = imageResultURLs(body);
    if (!urls) return { status: "UNKNOWN", reason: "Image2Pro completed image task has invalid image URLs" };
    result.url = urls[0];
    result.progress = "100%";
    result.state = { kind: "image", n: requestedImageCount((ctx && (ctx.state || ctx.requestBody)) || {}) };
  } else if (status === "IN_PROGRESS") result.progress = "50%";
  return result;
}

/** 输出 OpenAI image.task 外形；只选取宿主任务字段和已校验的图片 URL。 */
function renderImageTask(_ctx, task) {
  const data = task && task.data && typeof task.data === "object" && !Array.isArray(task.data) ? task.data : {};
  const statuses = {
    NOT_START: "queued",
    SUBMITTED: "queued",
    QUEUED: "queued",
    IN_PROGRESS: "processing",
    SUCCESS: "completed",
    FAILURE: "failed",
  };
  const status = statuses[task && task.status] || "unknown";
  const output = {
    id: (task && task.task_id) || "",
    object: "image.task",
    model: typeof data.model === "string" ? data.model : "",
    status,
    created_at: (task && task.created_at) || 0,
  };
  if (task && Number.isFinite(task.updated_at) && task.updated_at > 0) output.updated_at = task.updated_at;
  if (
    data.expires_at !== undefined &&
    ((typeof data.expires_at === "number" && Number.isFinite(data.expires_at)) ||
      (typeof data.expires_at === "string" && data.expires_at.length <= 64 && !hasControlCharacters(data.expires_at)))
  )
    output.expires_at = data.expires_at;
  if (Number.isInteger(data.n) && data.n >= 1 && data.n <= 4) output.n = data.n;
  if (typeof data.accounting_status === "string" && data.accounting_status.length <= 64 && !hasControlCharacters(data.accounting_status))
    output.accounting_status = data.accounting_status;
  if (status === "completed") {
    const urls = imageResultURLs(data);
    if (!urls) {
      output.status = "failed";
      output.error = { code: "image_generation_failed", message: "Image2Pro image generation failed" };
    } else output.data = urls.map((url) => ({ url }));
  } else if (status === "failed") output.error = { code: "image_generation_failed", message: "Image2Pro image generation failed" };
  return output;
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
  if (isImageModel(ctx.model || source.model)) throw new Error("unsupported Image2Pro video model");
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

/** 标准视频与 Responses 共用生成驱动、逐模型计费和宿主管理的制品权限。 */
export const protocols = {
  openai_image_tasks: {
    /** 图片异步任务只接受文档定义的 JSON 字段，接口路径标识图片操作。 */
    decodeRequest: function (ctx) {
      const body = ctx.body;
      if (!body || body.kind !== "json") throw new Error("Image2Pro image tasks require a JSON body");
      const source = body.value;
      if (!source || typeof source !== "object" || Array.isArray(source)) throw new Error("Image2Pro image request body must be an object");
      const model = ctx.model || source.model;
      const upstreamModel = ctx.upstreamModel || model;
      if (typeof model !== "string" || !model.trim() || isVideoModel(model) || isVideoModel(upstreamModel))
        throw new Error("unsupported Image2Pro image model");
      if (source.model !== model) throw new Error("Image2Pro image model does not match the selected model");
      const request = normalizeImageRequest(source, model);
      request.model = upstreamModel;
      request["__imageTask"] = true;
      return { kind: "submit", model, requestBody: request };
    },
    render: function (ctx, task) {
      return renderImageTask(ctx, task);
    },
  },
  openai_video: {
    /** 接收 JSON 或 multipart 引用素材，统一字段并保留客户端模型别名。 */
    decodeRequest: function (ctx) {
      const body = ctx.body;
      let source;
      if (body && body.kind === "json") source = body.value;
      else if (body && body.kind === "multipart") source = multipartRequest(body, ctx.upstreamModel || ctx.model);
      else throw new Error("Image2Pro requires a JSON or multipart body");
      const model = ctx.model || (source && source.model);
      if (isImageModel(model)) throw new Error("unsupported Image2Pro video model");
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
