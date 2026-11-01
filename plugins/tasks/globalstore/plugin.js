/**
 * GlobalStore（GlobalAIOPC）v2 统一模型中心适配。
 *
 * 上游只有一个异步提交/查询接口：
 *   POST {base}/v2/model-center/tasks        {model, ...生成参数}
 *   GET  {base}/v2/model-center/tasks/{id}   裸顶层对象，status = queued|processing|completed|failed
 * 因此视频模型走标准 openai_video 入口，图片模型走仅支持 background 的 openai_responses 入口。
 * 同一份模型参数表同时驱动请求校验、上游请求体和计费事实，避免提交与计费口径不一致。
 */

/** 图片比例集合（按文档分组，逐模型引用）。 */
const RATIOS_I1 = ["21:9", "16:9", "3:2", "4:3", "5:4", "1:1", "4:5", "3:4", "2:3", "9:16", "auto", "1:4", "4:1", "1:8", "8:1"];
const RATIOS_I2 = ["21:9", "16:9", "3:2", "4:3", "1:1", "3:4", "2:3", "9:16", "4:1", "1:4"];
const RATIOS_I3 = ["1:1", "16:9", "9:16", "3:4", "4:3", "2:3", "3:2"];
const RATIOS_I4 = ["1:1", "3:4", "4:3", "16:9", "9:16", "3:2", "2:3", "21:9"];

/** 图片像素尺寸集合；逐模型原样录入文档枚举，不做保守裁剪。 */
const SIZES_B30 = [
  "1584x672", "1376x768", "1264x848", "1200x896", "1152x928", "1024x1024", "928x1152", "896x1200", "848x1264", "768x1376",
  "3168x1344", "2752x1536", "2528x1696", "2400x1792", "2304x1856", "2048x2048", "1856x2304", "1792x2400", "1696x2528", "1536x2752",
  "6336x2688", "5504x3072", "5056x3392", "4800x3584", "4608x3712", "4096x4096", "3712x4608", "3584x4800", "3392x5056", "3072x5504",
];
const SIZES_C14 = [
  "1584x672", "1376x768", "1264x848", "1200x896", "1152x928", "1024x1024", "928x1152", "896x1200", "848x1264", "768x1376",
  "2880x384", "2048x512", "512x2048", "384x2880",
];
const SIZES_G14 = [
  "1024x1024", "1280x720", "720x1280", "2048x2048", "2816x1584", "1584x2816", "1776x2368", "2368x1776", "1664x2496", "2496x1664",
  "1152x864", "864x1152", "1248x832", "832x1248",
];

/** 视频比例集合。 */
const RATIOS_V1 = ["16:9", "9:16", "1:1", "4:3", "3:4", "21:9", "adaptive"];
const RATIOS_V2 = ["16:9", "9:16", "4:3", "3:4", "1:1"];
const RATIOS_V3 = ["1:1", "16:9", "9:16", "4:3", "3:4", "3:2", "2:3"];
const RATIOS_V4 = ["16:9", "9:16"];

/** 常见像素尺寸到清晰度与比例的映射，超出该表的一律拒绝而不是猜测。 */
const VIDEO_PIXEL_SIZES = {
  "1280x720": { resolution: "720p", ratio: "16:9" },
  "720x1280": { resolution: "720p", ratio: "9:16" },
  "1920x1080": { resolution: "1080p", ratio: "16:9" },
  "1080x1920": { resolution: "1080p", ratio: "9:16" },
};

/**
 * 图片模型表。resolutions 的第一项是文档默认值，ratio 默认 1:1（banana_c1 为 21:9），
 * qualities 的第一项是默认质量；maxReferences 是参考图上限。
 */
const IMAGE_MODELS = {
  banana2: { ratios: RATIOS_I1, resolutions: ["1k", "2k", "4k"], sizes: SIZES_B30, maxReferences: 14 },
  bananapro: { ratios: RATIOS_I1, resolutions: ["1k", "2k", "4K"], sizes: SIZES_B30, maxReferences: 14 },
  banana_c1: { ratios: RATIOS_I2, resolutions: ["1K"], sizes: SIZES_C14, maxReferences: 14, qualities: ["standard"], defaultRatio: "21:9" },
  banana_c2: { ratios: RATIOS_I1, resolutions: ["1K", "2K", "4K"], maxReferences: 6 },
  "globalstore/gpt-image-2": { ratios: RATIOS_I3, resolutions: ["1K", "2K"], sizes: SIZES_G14, maxReferences: 14, qualities: ["medium", "high"] },
  "gpt-image-2.5": { ratios: RATIOS_I3, resolutions: ["1K"], sizes: SIZES_G14, maxReferences: 14, qualities: ["low", "medium", "high"] },
  "seedream-5.0": { ratios: RATIOS_I4, resolutions: ["2K", "3K", "4K"], maxReferences: 10 },
  "seedream_5.0Pro": { ratios: RATIOS_I4, resolutions: ["1K", "2K"], maxReferences: 10 },
};

/**
 * 视频模型表。duration 为 {min,max} 或 {only:n}；refs 是图片/视频/音频参考上限；
 * first/last 表示支持首帧/尾帧；seed 表示接受随机种子；audio 表示接受 generate_audio。
 */
const VIDEO_MODELS = {
  "sd_2.0_discount": { ratios: RATIOS_V1, resolutions: ["480p", "720p", "1080p", "4k"], duration: { min: 5, max: 15 }, defaultDuration: 5, refs: { images: 9, videos: 3, audios: 3 }, seed: true, audio: true },
  "sd_2.0_fast_discount": { ratios: RATIOS_V1, resolutions: ["480p", "720p"], duration: { min: 5, max: 15 }, defaultDuration: 5, refs: { images: 9, videos: 3, audios: 3 }, seed: true, audio: true },
  "sd_2.0_mini_discount": { ratios: RATIOS_V1, resolutions: ["480p", "720p"], duration: { min: 5, max: 15 }, defaultDuration: 5, refs: { images: 9, videos: 3, audios: 3 }, seed: true, audio: true },
  "sd_2.5_discount": { ratios: RATIOS_V1, resolutions: ["480p", "720p", "1080p"], duration: { min: 5, max: 30 }, defaultDuration: 5, refs: { images: 30, videos: 10, audios: 10 }, seed: true, audio: true },
  "sd_2.0_special": { ratios: RATIOS_V1, resolutions: ["720p", "1080p", "2k", "4k"], duration: { min: 5, max: 15 }, defaultDuration: 5, refs: { images: 9, videos: 3, audios: 3 }, first: true, last: true, seed: true, audio: true },
  "sd_2.0_fast_special": { ratios: RATIOS_V1, resolutions: ["720p"], duration: { min: 4, max: 15 }, defaultDuration: 5, refs: { images: 9, videos: 3, audios: 3 }, first: true, last: true, seed: true, audio: true },
  "sd_2.0_mini_special": { ratios: RATIOS_V1, resolutions: ["720p"], duration: { min: 4, max: 15 }, defaultDuration: 5, refs: { images: 9, videos: 3, audios: 3 }, first: true, last: true, seed: true, audio: true },
  "sd_2.5_special": { ratios: RATIOS_V1, resolutions: ["720p", "1080p", "2k", "4k"], duration: { min: 5, max: 30 }, defaultDuration: 5, refs: { images: 30, videos: 10, audios: 10 }, first: true, last: true, seed: true, audio: true },
  "seedance_2.0_nsfw_c1": { ratios: RATIOS_V2, resolutions: ["480p", "720p", "1080p", "4k"], duration: { min: 5, max: 15 }, defaultDuration: 5, refs: { images: 9, videos: 3, audios: 3 }, first: true, last: true, seed: true, audio: true, audioDefault: false },
  grok_video: { ratios: RATIOS_V3, resolutions: ["480p", "720p"], duration: { min: 1, max: 15 }, defaultDuration: 8, refs: { images: 7, videos: 0, audios: 0 }, omitDefaultRatio: true },
  "grok_1.5": { ratios: RATIOS_V3, resolutions: ["480p", "720p"], duration: { min: 1, max: 15 }, defaultDuration: 8, refs: { images: 7, videos: 0, audios: 0 }, first: true, omitDefaultRatio: true },
  omni_720P: { ratios: RATIOS_V4, resolutions: ["720p"], duration: { only: 10 }, defaultDuration: 10, refs: { images: 5, videos: 1, audios: 0 }, audio: true },
  "veo3.1-fast": { ratios: RATIOS_V4, resolutions: ["720p"], duration: { only: 8 }, defaultDuration: 8, refs: { images: 0, videos: 0, audios: 0 }, first: true, last: true, audio: true },
  "wan3.0": { ratios: RATIOS_V2, resolutions: ["480p", "720p", "1080p"], duration: { min: 2, max: 30 }, defaultDuration: 5, refs: { images: 10, videos: 0, audios: 5 } },
  "wan3.0-Prime": { ratios: RATIOS_V2, resolutions: ["480p", "720p", "1080p"], duration: { min: 2, max: 30 }, defaultDuration: 5, refs: { images: 10, videos: 0, audios: 5 } },
};

const IMAGE_MODEL_IDS = Object.keys(IMAGE_MODELS);
const VIDEO_MODEL_IDS = Object.keys(VIDEO_MODELS);

/** 只声明视频入口；图片模型改由 fork 的原生 GlobalStore 图片渠道（ChannelTypeGlobalStore）服务。 */
export const meta = {
  apiVersion: 1,
  key: "globalstore",
  name: "GlobalStore Model Center",
  icon: "GS",
  description: {
    en: "GlobalStore v2 model-center video tasks through the OpenAI video API",
    zh: "通过 OpenAI 视频接口接入 GlobalStore v2 模型中心视频任务",
  },
  version: "1.1.0",
  author: { name: "lysimportant/forknewapi" },
  website: "https://docs.globalstore.ai/zh",
  baseUrl: "https://saas.globalstore.ai/kyyReactApiServer",
  allowedHosts: ["saas.globalstore.ai:443"],
  models: VIDEO_MODEL_IDS,
  fetchMode: "per_task",
  requiredCapabilities: ["task-submit-no-retry@1"],
  protocols: [{ name: "openai_video", models: VIDEO_MODEL_IDS }],
  // 默认用量 = 视频：管理员可选「按次」(count) 或「清晰度 + 秒数」计费。
  usageSchema: {
    count: {
      type: "number",
      unit: "count",
      unitLabel: { en: "video", zh: "个" },
      description: { en: "Per-call price of one video", zh: "视频按次单价" },
    },
    seconds: {
      type: "number",
      unit: "second",
      description: { en: "Requested video duration", zh: "请求视频时长" },
    },
    input_images: {
      type: "number",
      unit: "count",
      unitLabel: { en: "image", zh: "张" },
      description: { en: "Input image unit price", zh: "输入图片单价" },
    },
    input_videos: {
      type: "number",
      unit: "count",
      unitLabel: { en: "video", zh: "段" },
      description: { en: "Input video unit price", zh: "输入视频单价" },
    },
    input_audios: {
      type: "number",
      unit: "count",
      unitLabel: { en: "audio", zh: "段" },
      description: { en: "Input audio unit price", zh: "输入音频单价" },
    },
    video_resolution: {
      enum: ["480p", "720p", "1080p", "2k", "4k"],
      enumLabels: { "2k": { en: "2K", zh: "2K" }, "4k": { en: "4K", zh: "4K" } },
      description: { en: "Requested video resolution", zh: "请求视频清晰度" },
    },
  },
  usageExamples: [
    { label: "480p · 5s", facts: { count: 1, seconds: 5, input_images: 0, input_videos: 0, input_audios: 0, video_resolution: "480p" } },
    { label: "720p · 5s", facts: { count: 1, seconds: 5, input_images: 0, input_videos: 0, input_audios: 0, video_resolution: "720p" } },
    { label: "1080p · 5s", facts: { count: 1, seconds: 5, input_images: 0, input_videos: 0, input_audios: 0, video_resolution: "1080p" } },
    { label: "2K · 5s", facts: { count: 1, seconds: 5, input_images: 0, input_videos: 0, input_audios: 0, video_resolution: "2k" } },
    { label: "4K · 5s", facts: { count: 1, seconds: 5, input_images: 0, input_videos: 0, input_audios: 0, video_resolution: "4k" } },
    { label: "720p · 5s · 参考素材", facts: { count: 1, seconds: 5, input_images: 1, input_videos: 1, input_audios: 1, video_resolution: "720p" } },
  ],
};

const MAX_MEDIA_ITEMS = 60;

/** 去除字符串两端空白；空值统一返回空字符串。 */
function trimmed(value) {
  return typeof value === "string" ? value.trim() : String(value === undefined || value === null ? "" : value).trim();
}

/** 只接受公开 HTTP(S) 地址；上游只接受公网可读素材。 */
function isHTTPURL(value) {
  return typeof value === "string" && /^https?:\/\/[^\s]+$/i.test(value.trim());
}

/** 兼容 {url}、{image_url}、{video_url}、{audio_url} 与嵌套 {url:{url}} 的素材写法。 */
function mediaURL(value, field) {
  let url = value;
  if (value && typeof value === "object" && !Array.isArray(value)) {
    const urlField = field === "images" ? "image_url" : field === "videos" ? "video_url" : field === "audios" ? "audio_url" : "image_url";
    url = value[urlField] === undefined ? value.url : value[urlField];
    if (url && typeof url === "object" && !Array.isArray(url)) url = url.url;
  }
  if (!isHTTPURL(url)) throw new Error(field + " items must be public HTTP(S) URLs");
  return url.trim();
}

/** 读取单值、数组或 JSON 数组中的公开素材地址。 */
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
  if (values.length > MAX_MEDIA_ITEMS) throw new Error(field + " has too many items");
  return values.map(function (item) {
    return mediaURL(item, field);
  });
}

/** 解析 JSON、表单或 multipart 请求；上游要求公开 URL，不接受文件上传。 */
function decodedBody(body) {
  if (!body || !["json", "form", "multipart"].includes(body.kind)) throw new Error("JSON, form, or multipart body required");
  if (body.kind === "json") {
    if (!body.value || typeof body.value !== "object" || Array.isArray(body.value)) throw new Error("JSON object required");
    return Object.assign({}, body.value);
  }
  if ((body.files || []).length) throw new Error("GlobalStore requires public media URLs; file uploads are unsupported");
  const request = {};
  for (const key of Object.keys(body.fields || {})) {
    const values = body.fields[key];
    if (!Array.isArray(values) || values.length !== 1) throw new Error(key + " must be provided once");
    request[key] = values[0];
  }
  return request;
}

/** JSON 与表单共用 metadata 解析；拒绝错误类型而不是静默丢弃参数。 */
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

/** 顶层参数与 metadata 参数使用同一套读取规则，冲突时报错而不是任选其一。 */
function parameter(request, metadata, key) {
  if (request[key] !== undefined && metadata[key] !== undefined && JSON.stringify(request[key]) !== JSON.stringify(metadata[key])) {
    throw new Error(key + " conflicts with metadata." + key);
  }
  return request[key] === undefined ? metadata[key] : request[key];
}

/** 同一语义的多个别名归一后必须一致，否则拒绝歧义请求。 */
function aliasValue(candidates) {
  let chosen;
  for (const candidate of candidates) {
    if (candidate.value === undefined) continue;
    if (chosen !== undefined && JSON.stringify(chosen) !== JSON.stringify(candidate.value)) throw new Error(candidate.name + " conflicts with " + candidate.conflict);
    if (chosen === undefined) chosen = candidate.value;
  }
  return chosen;
}

/** 严格整数字符串也接受；拒绝 "--1" 这类占位示例。 */
function asInteger(value, field) {
  if (typeof value === "number") {
    if (!Number.isSafeInteger(value)) throw new Error(field + " must be an integer");
    return value;
  }
  if (typeof value === "string" && /^-?\d+$/.test(value.trim())) return Number(value.trim());
  throw new Error(field + " must be an integer");
}

/** 时长别名来自 OpenAI 兼容层时可能分别是数字与数字字符串，先统一比较口径。 */
function normalizeDurationAlias(value) {
  if (typeof value === "string" && /^-?\d+$/.test(value.trim())) return Number(value.trim());
  return value;
}

/** 接受真实布尔或 "true"/"false" 字符串，保留 false 不当作缺省。 */
function asBoolean(value, field) {
  if (typeof value === "boolean") return value;
  if (value === "true") return true;
  if (value === "false") return false;
  throw new Error(field + " must be a boolean");
}

/** 归一化分辨率；输入大小写宽容，输出保持支持列表中的精确写法。 */
function normalizeResolution(value, supported, field) {
  const raw = trimmed(value).toLowerCase();
  for (const candidate of supported) {
    if (candidate.toLowerCase() === raw) return candidate;
  }
  throw new Error(field + " must be one of " + supported.join(", "));
}

/** 解析可选的 content 数组；文本与多媒体引用都保留角色。 */
function contentInputs(request, metadata) {
  let supplied = parameter(request, metadata, "content");
  if (typeof supplied === "string") {
    try {
      supplied = JSON.parse(supplied);
    } catch {
      throw new Error("content must be a JSON array");
    }
  }
  if (supplied === undefined || supplied === null) return [];
  if (!Array.isArray(supplied)) throw new Error("content must be an array");
  if (supplied.length > MAX_MEDIA_ITEMS) throw new Error("content has too many items");
  return supplied.map(function (item) {
    if (typeof item === "string") return { type: "text", text: item };
    if (!item || typeof item !== "object" || Array.isArray(item)) throw new Error("invalid content item");
    if (["text", "input_text"].includes(item.type) && typeof item.text === "string") return { type: "text", text: item.text };
    const type = { input_image: "image_url", input_video: "video_url", input_audio: "audio_url" }[item.type] || item.type;
    const roles = {
      image_url: ["first_frame", "last_frame", "reference_image"],
      video_url: ["reference_video"],
      audio_url: ["reference_audio"],
    };
    if (!roles[type]) throw new Error("unsupported content type");
    const role = item.role === undefined ? (type === "image_url" ? "reference_image" : roles[type][0]) : item.role;
    if (!roles[type].includes(role)) throw new Error("unsupported " + type + " role");
    const field = type === "image_url" ? "images" : type === "video_url" ? "videos" : "audios";
    return { type: type, url: mediaURL(item[type] || item.url, field), role: role };
  });
}

/** 汇总直接字段、别名与 content 数组，得到未按模型校验的原始素材分组。 */
function collectInputs(request, metadata) {
  const content = contentInputs(request, metadata);
  const texts = [];
  for (const item of content) {
    if (item.type === "text" && trimmed(item.text)) texts.push(trimmed(item.text));
  }
  if (!texts.length) {
    const fallback = trimmed(parameter(request, metadata, "prompt") === undefined ? request.input : parameter(request, metadata, "prompt"));
    if (fallback) texts.push(fallback);
  }

  const firstImages = [];
  const lastImages = [];
  const referenceImages = [];
  for (const item of content) {
    if (item.type !== "image_url") continue;
    if (item.role === "first_frame") firstImages.push(item.url);
    else if (item.role === "last_frame") lastImages.push(item.url);
    else referenceImages.push(item.url);
  }

  const explicitFirst = aliasValue([
    { name: "first_frame_image", value: parameter(request, metadata, "first_frame_image"), conflict: "first_image" },
    { name: "first_frame", value: parameter(request, metadata, "first_frame"), conflict: "first_image" },
    { name: "first_image", value: parameter(request, metadata, "first_image"), conflict: "first_frame" },
  ]);
  for (const url of mediaList(explicitFirst, "images")) firstImages.push(url);

  const explicitLast = aliasValue([
    { name: "last_frame_image", value: parameter(request, metadata, "last_frame_image"), conflict: "last_image" },
    { name: "last_frame", value: parameter(request, metadata, "last_frame"), conflict: "last_image" },
    { name: "last_image", value: parameter(request, metadata, "last_image"), conflict: "last_frame" },
  ]);
  for (const url of mediaList(explicitLast, "images")) lastImages.push(url);

  const explicitReferences = aliasValue([
    { name: "reference_images", value: parameter(request, metadata, "reference_images"), conflict: "images" },
    { name: "images", value: parameter(request, metadata, "images"), conflict: "reference_images" },
  ]);
  for (const url of mediaList(explicitReferences, "images")) referenceImages.push(url);

  const inputReference = mediaList(parameter(request, metadata, "input_reference"), "images");
  const references = referenceImages.concat(inputReference);

  const videos = mediaList(
    aliasValue([
      { name: "reference_videos", value: parameter(request, metadata, "reference_videos"), conflict: "videos" },
      { name: "videos", value: parameter(request, metadata, "videos"), conflict: "reference_videos" },
    ]),
    "videos",
  ).concat(
    mediaList(
      aliasValue([
        { name: "video", value: parameter(request, metadata, "video"), conflict: "videos" },
        { name: "reference_video", value: parameter(request, metadata, "reference_video"), conflict: "video" },
      ]),
      "videos",
    ),
  );

  const audios = mediaList(
    aliasValue([
      { name: "reference_audios", value: parameter(request, metadata, "reference_audios"), conflict: "audios" },
      { name: "audios", value: parameter(request, metadata, "audios"), conflict: "reference_audios" },
    ]),
    "audios",
  ).concat(
    mediaList(
      aliasValue([
        { name: "audio", value: parameter(request, metadata, "audio"), conflict: "audios" },
        { name: "reference_audio", value: parameter(request, metadata, "reference_audio"), conflict: "audio" },
      ]),
      "audios",
    ),
  );

  return { content: content, texts: texts, first: firstImages, last: lastImages, references: references, videos: videos, audios: audios };
}

/** 图片模型请求归一化。 */
function normalizeImageRequest(request, metadata, modelId) {
  const spec = IMAGE_MODELS[modelId];
  if (!spec) throw new Error("unsupported GlobalStore model");
  const inputs = collectInputs(request, metadata);
  if (!inputs.texts.length) throw new Error("prompt is required");
  if (inputs.first.length || inputs.last.length) throw new Error("GlobalStore image models do not accept first or last frames");
  if (inputs.videos.length || inputs.audios.length) throw new Error("GlobalStore image models accept image references only");
  if (inputs.references.length > spec.maxReferences) throw new Error("reference_images cannot exceed " + spec.maxReferences);

  const rawSize = parameter(request, metadata, "size");
  const rawResolution = parameter(request, metadata, "resolution");
  if (rawSize !== undefined && rawResolution !== undefined) throw new Error("size and resolution are mutually exclusive");
  const ratio = trimmed(aliasValue([
    { name: "aspect_ratio", value: parameter(request, metadata, "aspect_ratio"), conflict: "ratio" },
    { name: "ratio", value: parameter(request, metadata, "ratio"), conflict: "aspect_ratio" },
  ]));
  const n = parameter(request, metadata, "n");
  if (n !== undefined && asInteger(n, "n") !== 1) throw new Error("n greater than 1 is not supported");
  const responseFormat = parameter(request, metadata, "response_format");
  if (responseFormat !== undefined && trimmed(responseFormat) !== "" && trimmed(responseFormat) !== "url") {
    throw new Error("response_format must be url");
  }

  const facts = { kind: "image", model: modelId, prompt: inputs.texts.join("\n"), references: inputs.references, count: 1 };
  if (rawSize !== undefined) {
    const size = trimmed(rawSize).toLowerCase();
    if (!/^\d+x\d+$/.test(size)) throw new Error("size must be WxH");
    if (spec.sizes && !spec.sizes.includes(size)) throw new Error("size is not supported by " + modelId);
    facts.size = size;
    facts.resolution = "unspecified";
  } else {
    facts.ratio = ratio
      ? normalizeAspectRatio(ratio, spec.ratios)
      : spec.defaultRatio || "1:1";
    facts.resolution = normalizeResolution(rawResolution === undefined ? spec.resolutions[0] : rawResolution, spec.resolutions, "resolution");
  }
  if (spec.qualities) {
    const quality = parameter(request, metadata, "quality");
    facts.quality = quality === undefined ? spec.qualities[0] : normalizeResolution(quality, spec.qualities, "quality").toLowerCase();
  }
  return facts;
}

/** 视频模型请求归一化。 */
function normalizeVideoRequest(request, metadata, modelId) {
  const spec = VIDEO_MODELS[modelId];
  if (!spec) throw new Error("unsupported GlobalStore model");
  const inputs = collectInputs(request, metadata);
  if (!inputs.texts.length) throw new Error("prompt is required");
  if (spec.refs.images === 0 && (inputs.references.length || inputs.first.length || inputs.last.length)) {
    throw new Error(modelId + " does not accept image inputs");
  }
  if (inputs.references.length + inputs.first.length + inputs.last.length > spec.refs.images) {
    throw new Error("image inputs cannot exceed " + spec.refs.images);
  }
  if (inputs.videos.length > spec.refs.videos) throw new Error("reference_videos cannot exceed " + spec.refs.videos);
  if (inputs.audios.length > spec.refs.audios) throw new Error("reference_audios cannot exceed " + spec.refs.audios);
  if ((inputs.first.length || inputs.last.length) && (!spec.first || !spec.last)) throw new Error(modelId + " does not accept first or last frames");
  if (inputs.first.length > 1) throw new Error("first_image accepts at most one URL");
  if (inputs.last.length > 1) throw new Error("last_image accepts at most one URL");

  const rawDuration = aliasValue([
    { name: "seconds", value: normalizeDurationAlias(parameter(request, metadata, "seconds")), conflict: "duration" },
    { name: "duration", value: normalizeDurationAlias(parameter(request, metadata, "duration")), conflict: "seconds" },
  ]);
  const duration = rawDuration === undefined ? spec.defaultDuration : asInteger(rawDuration, "duration");
  if (spec.duration.only !== undefined && duration !== spec.duration.only) throw new Error("duration must be " + spec.duration.only + " for " + modelId);
  if (spec.duration.min !== undefined && (duration < spec.duration.min || duration > spec.duration.max)) {
    throw new Error("duration must be an integer between " + spec.duration.min + " and " + spec.duration.max + " for " + modelId);
  }

  const rawSize = trimmed(parameter(request, metadata, "size"));
  const rawResolution = parameter(request, metadata, "resolution");
  const ratio = trimmed(aliasValue([
    { name: "aspect_ratio", value: parameter(request, metadata, "aspect_ratio"), conflict: "ratio" },
    { name: "ratio", value: parameter(request, metadata, "ratio"), conflict: "aspect_ratio" },
  ]));
  const facts = {
    kind: "video",
    model: modelId,
    prompt: inputs.texts.join("\n"),
    duration: duration,
    first: inputs.first,
    last: inputs.last,
    references: inputs.references,
    videos: inputs.videos,
    audios: inputs.audios,
    count: 1,
  };

  if (rawSize) {
    if (rawResolution !== undefined) throw new Error("size and resolution are mutually exclusive");
    const pixel = VIDEO_PIXEL_SIZES[rawSize.toLowerCase()];
    if (!pixel) throw new Error("size must be 1280x720, 720x1280, 1920x1080, or 1080x1920");
    facts.resolution = normalizeResolution(pixel.resolution, spec.resolutions, "size");
    facts.ratio = normalizeAspectRatio(ratio || pixel.ratio, spec.ratios);
  } else {
    facts.resolution = normalizeResolution(rawResolution === undefined ? spec.resolutions[spec.resolutions.length - 1] : rawResolution, spec.resolutions, "resolution");
    const fallbackRatio = spec.omitDefaultRatio ? "" : "16:9";
    const chosenRatio = ratio || fallbackRatio;
    facts.ratio = chosenRatio ? normalizeAspectRatio(chosenRatio, spec.ratios) : "";
  }

  const seed = parameter(request, metadata, "seed");
  if (seed !== undefined) {
    if (!spec.seed) throw new Error(modelId + " does not accept seed");
    facts.seed = asInteger(seed, "seed");
  }
  if (spec.audio) {
    const audio = parameter(request, metadata, "generate_audio");
    facts.generateAudio = audio === undefined ? spec.audioDefault !== false : asBoolean(audio, "generate_audio");
  } else if (parameter(request, metadata, "generate_audio") !== undefined) {
    throw new Error(modelId + " does not accept generate_audio");
  }
  return facts;
}

/** 视频清晰度默认取支持列表的最后一项（文档默认 720p）；显式传入时严格校验。 */
function normalizeAspectRatio(value, supported) {
  const raw = trimmed(value);
  for (const candidate of supported) {
    if (candidate === raw) return candidate;
  }
  throw new Error("aspect_ratio must be one of " + supported.join(", "));
}

/** 按模型类型分派；提交、Responses 与计费共用这一条链路。 */
function normalizeRequest(request, modelId) {
  if (!request || typeof request !== "object" || Array.isArray(request)) throw new Error("request body must be an object");
  const metadata = requestMetadata(request);
  if (IMAGE_MODELS[modelId]) return normalizeImageRequest(request, metadata, modelId);
  if (VIDEO_MODELS[modelId]) return normalizeVideoRequest(request, metadata, modelId);
  throw new Error("unsupported GlobalStore model");
}

/**
 * 上游模型名。插件对外的模型 id 不能占用本 fork 的内置模型名，否则会把该模型从
 * 内置计费/路由改判成任务插件模型；因此个别模型对外加 `globalstore/` 前缀，
 * 在这里映射回上游真实模型名。
 */
const UPSTREAM_MODEL_NAMES = {
  "globalstore/gpt-image-2": "gpt-image-2",
};

/** 组装上游 v2 请求体；未声明的字段一律不下发。 */
function upstreamBody(facts) {
  const body = { model: UPSTREAM_MODEL_NAMES[facts.model] || facts.model, prompt: facts.prompt };
  if (facts.kind === "image") {
    if (facts.references.length) body.reference_images = facts.references;
    if (facts.size) body.size = facts.size;
    else {
      body.aspect_ratio = facts.ratio;
      body.resolution = facts.resolution;
    }
    if (facts.quality) body.quality = facts.quality;
    return body;
  }
  body.duration = facts.duration;
  if (facts.resolution) body.resolution = facts.resolution;
  if (facts.ratio) body.aspect_ratio = facts.ratio;
  if (facts.first.length) body.first_image = facts.first;
  if (facts.last.length) body.last_image = facts.last;
  if (facts.references.length) body.reference_images = facts.references;
  if (facts.videos.length) body.reference_videos = facts.videos;
  if (facts.audios.length) body.reference_audios = facts.audios;
  if (facts.seed !== undefined) body.seed = facts.seed;
  if (facts.generateAudio !== undefined) body.generate_audio = facts.generateAudio;
  return body;
}

/** 迁移到 v2 模型中心路径；自定义代理保留主机与路径前缀。 */
function apiBase(ctx) {
  let base = trimmed(ctx.baseUrl || meta.baseUrl).replace(/\/+$/, "");
  if (!/^https?:\/\/[^\s/?#@]+(?:\/[^\s?#]*)?$/.test(base)) {
    throw new Error("channel base URL must be an HTTP(S) API base without credentials, query or fragment");
  }
  return base.replace(/\/(?:v2\/model-center(?:\/tasks)?|v2|v1)$/, "");
}

/** 上游成功码包含 202；返回非 2xx 直接报错而不是当作任务。 */
function submitStatusOK(statusCode) {
  return statusCode === undefined || (statusCode >= 200 && statusCode < 300);
}

/** 上游 v2 返回裸顶层对象；只接受对象，不凭空包装。 */
function taskObject(body) {
  return body && typeof body === "object" && !Array.isArray(body) ? body : null;
}

/** 上游错误字段既可能是字符串也可能是对象。 */
function errorMessage(value) {
  if (typeof value === "string") return trimmed(value);
  if (value && typeof value === "object") return trimmed(value.message) || trimmed(value.code);
  return "";
}

/** 结果地址优先取与媒体类型匹配的字段，其次 result_url。 */
function resultURL(body, kind) {
  const task = taskObject(body);
  if (!task) return "";
  const specific = kind === "image" ? task.image_url : task.video_url;
  if (isHTTPURL(specific)) return specific.trim();
  if (isHTTPURL(task.result_url)) return task.result_url.trim();
  return "";
}

/** 依据上游 object 字段与结果字段推断媒体类型。 */
function taskKind(body, fallback) {
  const task = taskObject(body);
  if (task) {
    const object = trimmed(task.object).toLowerCase();
    if (object === "image" || object === "video") return object;
    if (isHTTPURL(task.video_url) && !isHTTPURL(task.image_url)) return "video";
    if (isHTTPURL(task.image_url) && !isHTTPURL(task.video_url)) return "image";
  }
  if (IMAGE_MODELS[fallback]) return "image";
  if (VIDEO_MODELS[fallback]) return "video";
  return "video";
}

/** 清晰度事实只上报宿主声明的枚举；像素尺寸与上游大小写差异都归为「未知」。 */
function videoUsageResolution(facts) {
  const raw = trimmed(facts.resolution).toLowerCase();
  return ["480p", "720p", "1080p", "2k", "4k"].includes(raw) ? raw : "";
}

/** 生成 GlobalStore 任务创建请求；提交结果未知时禁止宿主自动重试。 */
export function buildSubmitRequest(ctx) {
  const model = ctx.upstreamModel || ctx.model;
  const facts = normalizeRequest(ctx.requestBody, model);
  return {
    url: apiBase(ctx) + "/v2/model-center/tasks",
    method: "POST",
    headers: { Authorization: "Bearer " + ctx.apiKey, "Content-Type": "application/json", Accept: "application/json" },
    body: upstreamBody(facts),
    noRetry: true,
  };
}

/** 解析 v2 创建响应中的上游任务 ID。 */
export function parseSubmitResponse(_ctx, response) {
  if (!submitStatusOK(response && response.statusCode)) {
    throw new Error("upstream task HTTP status " + response.statusCode + (errorMessage(response && response.body && response.body.error) ? ": " + errorMessage(response.body.error) : ""));
  }
  const body = taskObject(response && response.body);
  if (!body) throw new Error("upstream task response must be a JSON object");
  const failure = errorMessage(body.error);
  if (failure) throw new Error(failure);
  const taskId = trimmed(body.id);
  if (!taskId) throw new Error("upstream task id is missing");
  return { taskId: taskId, taskData: body };
}

/**
 * 返回提交阶段冻结的计费事实；倍率估算阶段不重复计费。
 * 图片模型只上报按次的 count；视频模型始终给 count，并在能确定清晰度时补 seconds +
 * video_resolution，管理员写「按次」或「清晰度 + 秒数」两种表达式都能命中。
 * 参考图片/视频/音频按件计数；首帧、尾帧是生成条件，不计费。
 */
export function extractUsage(ctx) {
  if (ctx.usagePurpose === "billing_ratios") return {};
  const model = ctx.upstreamModel || ctx.model;
  const facts = normalizeRequest(ctx.requestBody, model);
  if (facts.kind === "image") return { count: 1 };
  const usage = {
    count: 1,
    input_images: facts.references.length,
    input_videos: facts.videos.length,
    input_audios: facts.audios.length,
  };
  const resolution = videoUsageResolution(facts);
  if (resolution) {
    usage.seconds = facts.duration;
    usage.video_resolution = resolution;
  }
  return usage;
}

/** 完成响应没有已确认的实际时长，宿主沿用冻结的时长与清晰度，这里只落一个按次计数。 */
export function extractUsageOnComplete(_ctx, result, _body) {
  if (!result || result.status !== "SUCCESS") return null;
  return { count: 1 };
}

/** 构造 v2 任务查询请求。 */
export function buildQueryRequest(ctx) {
  return {
    url: apiBase(ctx) + "/v2/model-center/tasks/" + encodeURIComponent(ctx.taskId),
    method: "GET",
    headers: { Authorization: "Bearer " + ctx.apiKey, Accept: "application/json" },
  };
}

/** 映射上游任务状态；未知状态保持未知，避免误报成功。 */
export function parseTaskResult(ctx, body, response) {
  const statusCode = response && response.statusCode;
  if (statusCode !== undefined && statusCode >= 400) {
    if (statusCode === 404 || statusCode === 410) return { status: "FAILURE", reason: "upstream task was not found" };
    throw new Error("upstream task HTTP status " + statusCode);
  }
  const task = taskObject(body);
  if (!task) return { status: "UNKNOWN", reason: "invalid task response" };
  const kind = taskKind(task, ctx && (ctx.upstreamModel || ctx.model));
  const failure = errorMessage(task.error);
  const raw = trimmed(task.status).toLowerCase();
  const status = { queued: "QUEUED", processing: "IN_PROGRESS", completed: "SUCCESS", failed: "FAILURE" }[raw];
  if (!status) {
    if (failure) return { status: "FAILURE", reason: failure };
    return { status: "UNKNOWN", reason: "unrecognized task status" };
  }
  const result = { status: status, state: { kind: kind } };
  if (status === "FAILURE") {
    result.reason = failure || "upstream task failed";
    return result;
  }
  if (status === "SUCCESS") {
    const url = resultURL(task, kind);
    if (!url) throw new Error("upstream task completed without a result URL");
    result.url = url;
    result.progress = "100%";
    return result;
  }
  const progress = Number(task.progress);
  if (Number.isFinite(progress) && progress >= 0 && progress < 100) result.progress = Math.floor(progress) + "%";
  return result;
}

/** 判定任务媒体类型：优先持久化状态，其次上游 object 字段与任务动作。 */
function artifactKind(ctx) {
  const state = ctx && ctx.state;
  if (state && typeof state === "object" && (state.kind === "image" || state.kind === "video")) return state.kind;
  const data = ctx && ctx.data;
  const object = trimmed((data || {}).object).toLowerCase();
  if (object === "image" || object === "video") return object;
  if (isHTTPURL((data || {}).video_url)) return "video";
  if (isHTTPURL((data || {}).image_url)) return "image";
  return trimmed(ctx && ctx.action).indexOf("image") >= 0 ? "image" : "video";
}

/** 成功任务声明唯一制品；未完成、失败或缺少地址时返回空列表。 */
export function listArtifacts(task) {
  if (!task || task.status !== "SUCCESS") return [];
  const kind = artifactKind(task);
  if (!resultURL(task.data, kind)) return [];
  return [{ key: kind, type: kind }];
}

/** 构造内容下载请求，不把供应商地址或渠道凭据暴露给客户端。 */
export function buildContentRequest(ctx) {
  const kind = artifactKind(ctx);
  if (ctx.artifactKey !== kind) throw new Error("artifact_not_found");
  const url = resultURL(ctx.data, kind);
  if (!url) throw new Error("artifact_not_found");
  return { url: url, method: ctx.clientRequest.method, credentialless: true };
}

/** Responses 只展示宿主授权制品链接，HTML 属性转义避免地址注入。 */
function imageOutput(ctx) {
  const artifact = ctx.artifacts && ctx.artifacts.image;
  if (!artifact || !isHTTPURL(artifact.url)) throw new Error("GlobalStore image artifact is unavailable");
  const url = artifact.url.replace(/&/g, "&amp;").replace(/"/g, "&quot;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  return "![image](" + url + ")";
}

/** 解码单轮 Responses 文本与普通参考图；工具、历史消息、音视频输入一律拒绝。 */
function responsesRequest(ctx) {
  const source = ctx.body && ctx.body.kind === "json" && ctx.body.value;
  if (!source || typeof source !== "object" || Array.isArray(source)) throw new Error("GlobalStore Responses requires a JSON object");
  const aliases = [
    "prompt", "metadata", "content", "reasoning_effort",
    "aspect_ratio", "ratio", "resolution", "size", "quality", "n", "response_format",
    "reference_images", "images", "input_reference",
  ];
  for (const key of Object.keys(source)) {
    if (!["input", "stream", "background", "model"].includes(key) && !aliases.includes(key)) {
      throw new Error("unsupported GlobalStore Responses field: " + key);
    }
  }
  const request = {};
  for (const key of aliases) if (source[key] !== undefined) request[key] = source[key];
  let text = "";
  const images = [];
  if (typeof source.input === "string") text = source.input;
  else if (Array.isArray(source.input) && source.input.length === 1) {
    const message = source.input[0];
    if (!message || typeof message !== "object" || Array.isArray(message) || message.role !== "user" || (message.type !== undefined && message.type !== "message")) {
      throw new Error("GlobalStore Responses supports one user message");
    }
    for (const key of Object.keys(message)) {
      if (!["type", "role", "content"].includes(key)) throw new Error("unsupported GlobalStore Responses message field: " + key);
    }
    if (typeof message.content === "string") text = message.content;
    else if (Array.isArray(message.content)) {
      for (const item of message.content) {
        if (!item || typeof item !== "object" || Array.isArray(item)) throw new Error("invalid GlobalStore Responses content");
        if (["input_text", "text"].includes(item.type)) {
          if (Object.keys(item).some((key) => !["type", "text"].includes(key)) || typeof item.text !== "string") throw new Error("invalid GlobalStore Responses text");
          text += item.text;
        } else if (["input_image", "image_url"].includes(item.type)) {
          if (Object.keys(item).some((key) => !["type", "image_url"].includes(key))) throw new Error("unsupported GlobalStore Responses image field");
          images.push(item.image_url);
        } else throw new Error("GlobalStore Responses supports text and ordinary reference images only");
      }
    } else throw new Error("GlobalStore Responses message content must be text or an array");
  } else throw new Error("GlobalStore Responses requires a text input or one user message");
  if (request.prompt !== undefined && request.prompt !== text) throw new Error("prompt and Responses input conflict");
  request.prompt = text;
  if (images.length) {
    if (request.reference_images !== undefined || request.images !== undefined || request.input_reference !== undefined) {
      throw new Error("Responses image input conflicts with image aliases");
    }
    request.reference_images = images;
  }
  request.model = ctx.upstreamModel || ctx.model || source.model;
  return request;
}

/** 只注册视频入口：图片模型由 fork 的原生 GlobalStore 图片渠道承载。 */
export const protocols = {
  openai_video: {
    decodeRequest: function (ctx) {
      const request = decodedBody(ctx.body);
      const model = ctx.upstreamModel || ctx.model || request.model;
      const facts = normalizeRequest(request, model);
      const action =
        facts.videos.length || facts.audios.length || facts.references.length > 1
          ? "reference_to_video"
          : facts.first.length || facts.last.length || facts.references.length
            ? "image_to_video"
            : "text_to_video";
      return { kind: "submit", model: ctx.model || request.model, action: action, requestBody: Object.assign({}, request, { model: model }) };
    },
    render: function (_ctx, task) {
      const statuses = { NOT_START: "queued", SUBMITTED: "queued", QUEUED: "queued", IN_PROGRESS: "in_progress", SUCCESS: "completed", FAILURE: "failed" };
      const output = { id: task.task_id, object: "video", model: (task.properties || {}).origin_model_name || "", status: statuses[task.status] || "unknown" };
      if (task.status === "SUCCESS" && isHTTPURL((task.data || {}).url)) output.url = task.data.url;
      if (task.status === "FAILURE") output.error = { code: "video_generation_failed", message: "GlobalStore video generation failed" };
      return output;
    },
  },
};
