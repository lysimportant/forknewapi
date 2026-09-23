/** Moon Wan 按输出秒数和参考视频秒数计费的模型。 */
const WAN_MODELS = ["wan3.0-video", "wan3.0-video-prime"];
/** 旧渠道配置继续保留的 Seedance 兼容模型 ID。 */
const LEGACY_TOKEN_MODELS = ["doubao-seedance-2-0-mini-260615", "doubao-seedance-2-0-fast-260128", "artsdance-2-0-pro-260801"];
/** Moon 当前公开的 Seedance 模型 ID。 */
const OFFICIAL_TOKEN_MODELS = ["seedance-2-0-mini-official", "seedance-2-0-fast-official", "seedance-2-0-official", "seedance-2-5-official"];
/** Moon 按 usage.total_tokens 结算的 Seedance 模型。 */
const TOKEN_MODELS = LEGACY_TOKEN_MODELS.concat(OFFICIAL_TOKEN_MODELS);
/** Mini 和 Fast 仅支持 480p、720p。 */
const LIMITED_TOKEN_MODELS = ["doubao-seedance-2-0-mini-260615", "doubao-seedance-2-0-fast-260128", "seedance-2-0-mini-official", "seedance-2-0-fast-official"];
/** 官转按请求输出秒数计费，与官方 Token 系列分开定价。 */
const PT_MODELS = ["seedance2.0-9-3-3-PT", "seedance2.5-30-10-10-PT", "seedance2.0-fast-PT"];
/** 底价渠道的按次与按秒模型；供应商成本不强制用于下游定价。 */
const BUDGET_PER_REQUEST_MODELS = ["sd2mini", "sd2-930-face", "sd2.5-30-10-face", "sd2-930-no-face", "sd2.5-30-10-10-per-request"];
const BUDGET_PER_SECOND_MODELS = ["sd2-930-fast", "sd2.5-30-10-10-480", "sd2.5-30-10-10"];
const BUDGET_MODELS = BUDGET_PER_REQUEST_MODELS.concat(BUDGET_PER_SECOND_MODELS);
/** Moon MiniMax H3 的公开模型 ID。 */
const H3_MODEL = "minimax-h3";
/** Moon 按次计费的 Grok 模型，不能套用其他渠道的模型 ID。 */
const GROK_MODEL = "grok-v1.5-video";
/** 当前接入的完整模型 ID；渠道别名由宿主映射到这些上游 ID。 */
const MODELS = WAN_MODELS.concat(TOKEN_MODELS, PT_MODELS, BUDGET_MODELS, [H3_MODEL, GROK_MODEL]);

/** Grok 精确尺寸同时约束分辨率和比例，不能仅按短边推断。 */
const GROK_SIZES = {
  "1280x720": { resolution: "720p", ratio: "16:9" },
  "720x1280": { resolution: "720p", ratio: "9:16" },
  "1920x1080": { resolution: "1080p", ratio: "16:9" },
  "1080x1920": { resolution: "1080p", ratio: "9:16" },
};

/** 与百炼同名模型共用价格表达式时，Wan 分辨率事实保持大写。 */
const WAN_RESOLUTIONS = ["480P", "720P", "1080P"];
/** 与豆包同名模型共用的 token 分辨率事实。 */
const TOKEN_RESOLUTIONS = ["480p", "720p", "1080p", "4k"];
/** H3 计费事实使用目录中的分辨率档位，不使用供应商积分。 */
const H3_RESOLUTIONS = ["480p", "768p", "1080p", "2k", "4k"];
/** Moon 视频新系列允许的输出比例；Wan 不支持 21:9，底价模型不支持 adaptive。 */
const RATIOS = ["21:9", "16:9", "4:3", "1:1", "3:4", "9:16", "adaptive"];
/** H3 普通和超分工作流允许的固定比例。 */
const H3_RATIOS = ["16:9", "9:16", "1:1", "2:3", "3:2", "3:4", "4:3", "21:9"];
/** H3 普通工作流；超分工作流使用相同语义的 cf-* 变体。 */
const H3_STANDARD_WORKFLOWS = ["text-to-video", "multi-reference", "multi-reference-4", "lh-multi-reference", "fl2v", "mj"];
/** H3 超分工作流仅接受 2K、4K 和显式比例。 */
const H3_CF_WORKFLOWS = ["cf-multi-reference", "cf-fl2v", "cf-mj"];
/** H3 普通工作流的精确尺寸及对应计费档位。 */
const H3_SIZE_RESOLUTIONS = {
  "864x480": "480p",
  "1376x768": "768p",
  "1920x1088": "1080p",
  "480x864": "480p",
  "768x1376": "768p",
  "1088x1920": "1080p",
  "640x640": "480p",
  "1024x1024": "768p",
  "1440x1440": "1080p",
  "544x800": "480p",
  "832x1248": "768p",
  "1184x1760": "1080p",
  "800x544": "480p",
  "1248x832": "768p",
  "1760x1184": "1080p",
  "576x736": "480p",
  "896x1184": "768p",
  "1248x1664": "1080p",
  "736x576": "480p",
  "1184x896": "768p",
  "1664x1248": "1080p",
  "992x416": "480p",
  "1568x672": "768p",
  "2208x960": "1080p",
};
/** Canvas 分辨率和比例到 H3 精确尺寸的映射。 */
const H3_CANVAS_SIZES = {
  "16:9": { "480p": "864x480", "768p": "1376x768", "1080p": "1920x1088" },
  "9:16": { "480p": "480x864", "768p": "768x1376", "1080p": "1088x1920" },
  "1:1": { "480p": "640x640", "768p": "1024x1024", "1080p": "1440x1440" },
  "2:3": { "480p": "544x800", "768p": "832x1248", "1080p": "1184x1760" },
  "3:2": { "480p": "800x544", "768p": "1248x832", "1080p": "1760x1184" },
  "3:4": { "480p": "576x736", "768p": "896x1184", "1080p": "1248x1664" },
  "4:3": { "480p": "736x576", "768p": "1184x896", "1080p": "1664x1248" },
  "21:9": { "480p": "992x416", "768p": "1568x672", "1080p": "2208x960" },
};
/** 布尔参数只能传 JSON 布尔值，显式 false 必须保留。 */
const TOKEN_BOOLEAN_FIELDS = ["generate_audio", "watermark", "return_last_frame", "web_search", "camera_fixed"];
/** Wan 创建任务的公开字段。 */
const WAN_FIELDS = [
  "model",
  "prompt",
  "resolution",
  "ratio",
  "aspect_ratio",
  "duration",
  "seconds",
  "prompt_extend",
  "reference_images",
  "reference_videos",
  "reference_audios",
  "input",
];
/** Seedance Token 模型公开字段；其他参数必须拒绝，不能静默丢弃。 */
const TOKEN_FIELDS = [
  "model",
  "prompt",
  "content",
  "images",
  "image_urls",
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
/** MiniMax H3 创建任务的公开字段；duration、resolution 和 ratio 不是原生字段。 */
const H3_FIELDS = [
  "model",
  "prompt",
  "seconds",
  "workflow_id",
  "size",
  "aspect_ratio",
  "prompt_enhance",
  "mode",
  "input_reference",
  "images",
  "reference_video",
  "reference_audio",
  "reference_videos",
  "reference_audios",
];
/** Grok 仅接收文生视频或 URL 图片参考，其他素材和未知参数不得透传。 */
const GROK_FIELDS = ["model", "prompt", "seconds", "duration", "size", "resolution", "aspect_ratio", "ratio", "reference_images", "input_reference"];
/** 官转素材入口互斥；字段名遵守 Moon 官转合同。 */
const PT_FIELDS = [
  "model",
  "prompt",
  "content",
  "materials",
  "reference_images",
  "reference_videos",
  "reference_audios",
  "input_reference",
  "duration",
  "seconds",
  "resolution",
  "size",
  "ratio",
  "aspect_ratio",
  "generate_audio",
  "generateAudio",
];
/** 底价渠道公开的 JSON 参数，不接收上传、内联媒体或隐式转换。 */
const BUDGET_FIELDS = [
  "model",
  "prompt",
  "seconds",
  "duration",
  "resolution",
  "size",
  "ratio",
  "aspect_ratio",
  "images",
  "image_urls",
  "image_refs",
  "reference_images",
  "input_reference",
  "videos",
  "video_urls",
  "video_refs",
  "audios",
  "audio_urls",
  "audio_refs",
];
/** 底价渠道各模型的分辨率、时长及素材上限。 */
const BUDGET_LIMITS = {
  sd2mini: { resolutions: { "480p": [5, 15], "720p": [5, 12] }, images: 9, videos: 3, audios: 3, total: 15 },
  "sd2-930-face": { resolutions: { "720p": [4, 30] }, images: 9, videos: 0, audios: 3, total: 12 },
  "sd2.5-30-10-face": { resolutions: { "720p": [4, 30] }, images: 30, videos: 0, audios: 10, total: 40 },
  "sd2-930-fast": { resolutions: { "720p": [5, 15] }, images: 9, videos: 3, audios: 3, total: 15 },
  "sd2.5-30-10-10-480": { resolutions: { "480p": [4, 30] }, images: 30, videos: 10, audios: 10, total: 50 },
  "sd2.5-30-10-10": { resolutions: { "720p": [4, 30] }, images: 30, videos: 10, audios: 10, total: 50 },
  "sd2-930-no-face": { resolutions: { "720p": [4, 15] }, images: 9, videos: 3, audios: 3, total: 15 },
  "sd2.5-30-10-10-per-request": { resolutions: { "720p": [5, 30] }, images: 30, videos: 3, audios: 3, total: 36 },
};

/** 引用素材按文件数提供可选的输入单价；只计算已校验的引用条目，不读取文件内容。 */
const INPUT_USAGE_SCHEMA = {
  image_input_count: {
    type: "number",
    unit: "count",
    unitLabel: { en: "image", zh: "张" },
    description: { en: "Image input unit price", zh: "图片输入单价" },
  },
  video_input_count: {
    type: "number",
    unit: "count",
    unitLabel: { en: "video", zh: "段" },
    description: { en: "Video input unit price", zh: "视频输入单价" },
  },
};

/** Grok 的生成次数、请求秒数及图片引用分别可定价；上游按次成本不决定下游售价。 */
const GROK_USAGE_SCHEMA = {
  ...INPUT_USAGE_SCHEMA,
  video_count: {
    type: "number",
    unit: "count",
    description: { en: "Video generation unit price", zh: "视频生成单价" },
  },
  seconds: {
    type: "number",
    unit: "second",
    description: { en: "Video generation unit price", zh: "视频生成单价" },
  },
};

/** Moon Seedance 的 Token 用量与输入素材数可分别定价；tokens 不是金额。 */
const TOKEN_USAGE_SCHEMA = {
  ...INPUT_USAGE_SCHEMA,
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

/** Wan 的原秒数事实保留输出加参考视频时长；输入素材数可另行定价。 */
const WAN_USAGE_SCHEMA = {
  ...INPUT_USAGE_SCHEMA,
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

/** 官转的秒数事实只含输出时长；参考素材另按文件数计。 */
const PT_USAGE_SCHEMA = {
  ...INPUT_USAGE_SCHEMA,
  seconds: { type: "number", unit: "second", description: { en: "Video generation unit price", zh: "视频生成单价" } },
  resolution: {
    enum: ["480p", "720p"],
    enumLabels: { "480p": { en: "480p", zh: "480p" }, "720p": { en: "720p", zh: "720p" } },
    description: { en: "Output video resolution", zh: "输出视频分辨率" },
  },
};
/** 底价按次模型的生成事实固定为一次；输入素材可单独定价。 */
const BUDGET_PER_REQUEST_USAGE_SCHEMA = {
  ...INPUT_USAGE_SCHEMA,
  video_count: { type: "number", unit: "count", description: { en: "Video generation unit price", zh: "视频生成单价" } },
  resolution: PT_USAGE_SCHEMA.resolution,
};
/** 底价按秒模型的秒数事实只含输出时长；输入素材按文件数另计。 */
const BUDGET_PER_SECOND_USAGE_SCHEMA = {
  ...INPUT_USAGE_SCHEMA,
  seconds: { type: "number", unit: "second", description: { en: "Video generation unit price", zh: "视频生成单价" } },
  resolution: PT_USAGE_SCHEMA.resolution,
};
/** H3 暴露输出秒数和输入素材数；不把 Moon 积分换算为美元。 */
const H3_USAGE_SCHEMA = {
  ...INPUT_USAGE_SCHEMA,
  seconds: {
    type: "number",
    unit: "second",
    description: { en: "Video generation unit price", zh: "视频生成单价" },
  },
  resolution: {
    enum: H3_RESOLUTIONS,
    enumLabels: {
      "480p": { en: "480p", zh: "480p" },
      "768p": { en: "768p", zh: "768p" },
      "1080p": { en: "1080p", zh: "1080p" },
      "2k": { en: "2K", zh: "2K" },
      "4k": { en: "4K", zh: "4K" },
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
    en: "Moon video generation for Wan, Seedance, ArtsDance, PT, budget, MiniMax H3, and Grok models",
    zh: "Moon Wan、Seedance、ArtsDance、官转、底价、MiniMax H3 与 Grok 视频生成",
  },
  version: "1.5.0",
  author: { name: "QuantumNous" },
  models: MODELS,
  modelDiscovery: { protocol: "openai", path: "/v1/models" },
  fetchMode: "per_task",
  requiredCapabilities: ["task-submit-no-retry@1"],
  usageSchema: TOKEN_USAGE_SCHEMA,
  usageProfiles: [
    { models: WAN_MODELS, schema: WAN_USAGE_SCHEMA },
    { models: PT_MODELS, schema: PT_USAGE_SCHEMA },
    { models: BUDGET_PER_REQUEST_MODELS, schema: BUDGET_PER_REQUEST_USAGE_SCHEMA },
    { models: BUDGET_PER_SECOND_MODELS, schema: BUDGET_PER_SECOND_USAGE_SCHEMA },
    { models: [H3_MODEL], schema: H3_USAGE_SCHEMA },
    { models: [GROK_MODEL], schema: GROK_USAGE_SCHEMA },
  ],
  usageExamples: [
    { label: "5s · 720p", facts: { tokens: 108000, resolution: "720p", video_input: "none", image_input_count: 0, video_input_count: 0 } },
    { label: "5s · 1080p", facts: { tokens: 243000, resolution: "1080p", video_input: "none", image_input_count: 0, video_input_count: 0 } },
    { label: "5s · 4k", facts: { tokens: 972000, resolution: "4k", video_input: "none", image_input_count: 0, video_input_count: 0 } },
    {
      label: "2.0 · 5s · 720p · 1 image · 1 video",
      facts: { tokens: 432000, resolution: "720p", video_input: "video", image_input_count: 1, video_input_count: 1 },
    },
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

/** 判断是否走 Moon Wan 的已确认原生合同。 */
function isWan(model) {
  return WAN_MODELS.includes(model);
}

/** 判断是否必须等待真实 token 用量后才能完成任务。 */
function isTokenModel(model) {
  return TOKEN_MODELS.includes(model);
}

/** 官转不使用 Token 结算，按照提交时的输出秒数和分辨率定价。 */
function isPT(model) {
  return PT_MODELS.includes(model);
}

/** 底价渠道保留各自的时长、素材上限和上游计费类别。 */
function isBudget(model) {
  return BUDGET_MODELS.includes(model);
}

/** 判断是否走 Moon 原生 MiniMax H3 合同。 */
function isH3(model) {
  return model === H3_MODEL;
}

/** 判断是否使用 Moon 独立的 Grok 参数与按次计费合同。 */
function isGrok(model) {
  return model === GROK_MODEL;
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

/** 校验 Wan 参考视频时长；公开协议允许小数秒。 */
function numericReferenceDuration(value, field) {
  if (typeof value !== "number" || !Number.isFinite(value) || value < 2 || value > 15) throw new Error(field + " must be a number between 2 and 15");
  return value;
}

/** 统一 duration/seconds；Grok 默认 6 秒，其余默认 5 秒，只有 Seedance 接受 -1 自动时长。 */
function normalizeDuration(req, model) {
  const hasDuration = Object.prototype.hasOwnProperty.call(req, "duration");
  const hasSeconds = Object.prototype.hasOwnProperty.call(req, "seconds");
  if (Object.prototype.hasOwnProperty.call(req, "auto_duration")) {
    if (!isTokenModel(model) || req.auto_duration !== true || hasDuration || hasSeconds) throw new Error("invalid internal auto duration marker");
    return -1;
  }
  if (hasDuration && hasSeconds && numericDuration(req.duration, "duration") !== numericDuration(req.seconds, "seconds"))
    throw new Error("duration and seconds conflict");
  const raw = hasDuration ? req.duration : hasSeconds ? req.seconds : isGrok(model) ? 6 : 5;
  const duration = numericDuration(raw, hasDuration ? "duration" : "seconds");
  if (isWan(model)) {
    if (duration < 2 || duration > 30) throw new Error("duration must be an integer between 2 and 30 for Moon Wan models");
  } else if (isH3(model)) {
    if (!hasSeconds || hasDuration || duration < 4 || duration > 15) throw new Error("seconds must be an integer between 4 and 15 for Moon MiniMax H3");
  } else if (isGrok(model)) {
    if (duration < 4 || duration > 15) throw new Error("seconds must be an integer between 4 and 15 for Moon Grok");
  } else if (isPT(model)) {
    const max = model === "seedance2.5-30-10-10-PT" ? 30 : 15;
    if (duration < 5 || duration > max) throw new Error("Moon PT duration must be an integer between 5 and " + max);
  } else if (isBudget(model)) {
    if (duration < 4 || duration > 30) throw new Error("Moon budget duration must be an integer between 4 and 30");
  } else if (duration !== -1 && (duration < 4 || duration > (model === "seedance-2-5-official" ? 30 : 15))) {
    throw new Error("duration is outside the documented Moon Seedance range");
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
  if (isGrok(model) && !["720p", "1080p"].includes(raw)) throw new Error("Moon Grok supports only 720p or 1080p");
  if (isPT(model) && !["480p", "720p"].includes(raw)) throw new Error("Moon PT supports only 480p or 720p");
  if (isBudget(model) && !Object.prototype.hasOwnProperty.call(BUDGET_LIMITS[model].resolutions, raw))
    throw new Error("resolution is not supported by the Moon budget model");
  if (!isPT(model) && !isBudget(model) && !TOKEN_RESOLUTIONS.includes(raw)) throw new Error("resolution is not supported by the Moon model");
  if (LIMITED_TOKEN_MODELS.includes(model) && !["480p", "720p"].includes(raw)) throw new Error("this Moon model supports only 480p or 720p");
  if (model === "seedance-2-5-official" && !["720p", "1080p"].includes(raw)) throw new Error("Moon Seedance 2.5 supports only 720p or 1080p");
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
  if (isGrok(model) && !["16:9", "9:16", "1:1", "4:3", "3:4"].includes(ratio)) throw new Error("ratio is not supported by Moon Grok");
  if (isBudget(model) && !["16:9", "9:16", "1:1", "4:3", "3:4"].includes(ratio)) throw new Error("ratio is not supported by Moon budget models");
  return ratio;
}

/** 合并 H3 Canvas 比例别名；普通尺寸和超分都不能从 adaptive 推导。 */
function normalizeH3CanvasRatio(req) {
  const hasRatio = Object.prototype.hasOwnProperty.call(req, "ratio");
  const hasAspectRatio = Object.prototype.hasOwnProperty.call(req, "aspect_ratio");
  if (hasRatio && hasAspectRatio && trimmed(req.ratio) !== trimmed(req.aspect_ratio)) throw new Error("ratio and aspect_ratio conflict");
  const ratio = trimmed(hasRatio ? req.ratio : req.aspect_ratio);
  if (!H3_RATIOS.includes(ratio)) throw new Error("Moon MiniMax H3 requires a fixed documented ratio");
  return ratio;
}

/** 校验引用数组的 URL 与固定角色，未知子字段（含客户端自报时长）不得透传。 */
function validateReferenceList(value, field, role, maxCount, allowFrames = false) {
  if (value === undefined) return 0;
  if (!Array.isArray(value) || value.length > maxCount) throw new Error(field + " must contain at most " + maxCount + " references");
  for (const item of value) {
    if (
      !item ||
      typeof item !== "object" ||
      Array.isArray(item) ||
      !isHTTPURL(item.url) ||
      (item.role !== role && !(allowFrames && ["first_frame", "last_frame"].includes(item.role)))
    )
      throw new Error(field + " items must contain an HTTP(S) url and a supported role");
    for (const key of Object.keys(item)) if (key !== "url" && key !== "role") throw new Error("unsupported " + field + " reference field: " + key);
  }
  return value.length;
}

/** 校验两种素材表达并合并计数；参考视频不下载探测，按模型上限预留。 */
function validateTokenReferences(req, duration, ratio, model) {
  const is25 = model === "seedance-2-5-official";
  const images = tokenImages(req, model);
  let imageCount = validateReferenceList(images, "images", "reference_image", is25 ? 30 : 9, is25);
  let videoCount = validateReferenceList(req.videos, "videos", "reference_video", is25 ? 10 : 3);
  let audioCount = validateReferenceList(req.audios, "audios", "reference_audio", is25 ? 10 : 3);
  const frames = is25 ? (images || []).filter((item) => item.role === "first_frame" || item.role === "last_frame").map((item) => item.role) : [];
  let text = req.prompt || "";
  for (const field of ["first_frame", "last_frame"]) {
    if (req[field] === undefined) continue;
    if (!isHTTPURL(req[field])) throw new Error(field + " must be an HTTP(S) URL");
    imageCount++;
    if (is25) frames.push(field);
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
        if (is25 && ["first_frame", "last_frame"].includes(item.role)) frames.push(item.role);
      } else if (item.type === "video_url") {
        if (item.role !== "reference_video") throw new Error("content video role must be reference_video");
        videoCount++;
      } else {
        if (item.role !== "reference_audio") throw new Error("content audio role must be reference_audio");
        audioCount++;
      }
    }
  }
  if (imageCount > (is25 ? 30 : 9) || videoCount > (is25 ? 10 : 3) || audioCount > (is25 ? 10 : 3) || imageCount + videoCount + audioCount > (is25 ? 50 : 15))
    throw new Error("Moon Seedance reference limit exceeded");
  if (!is25 && audioCount > 0 && imageCount === 0 && videoCount === 0) throw new Error("audio-only references are not supported");
  if (is25 && frames.length) {
    if (
      frames.filter((role) => role === "first_frame").length > 1 ||
      frames.filter((role) => role === "last_frame").length > 1 ||
      (frames.includes("last_frame") && !frames.includes("first_frame"))
    )
      throw new Error("Moon Seedance 2.5 requires at most one first and last frame in order");
    if (ratio !== "adaptive") throw new Error("Moon Seedance 2.5 frame mode requires adaptive ratio");
  }
  if (text.length > 20000) throw new Error("prompt and content text must not exceed 20000 characters");
  if (/--(?:duration|resolution)\b/i.test(text)) throw new Error("inline duration and resolution overrides are not supported");
  if (req.omni_reference_task_type !== undefined && !["auto", "edit", "extend"].includes(req.omni_reference_task_type))
    throw new Error("omni_reference_task_type must be auto, edit, or extend");
  if ((req.omni_reference_task_type === "edit" || req.omni_reference_task_type === "extend") && videoCount === 0)
    throw new Error("edit and extend require a reference video");
  if ((req.omni_reference_task_type === "edit" || req.omni_reference_task_type === "extend") && ratio !== "adaptive")
    throw new Error("edit and extend require adaptive ratio");
  if (req.omni_reference_task_type === "edit" && duration !== -1) throw new Error("edit requires duration -1");
  return { imageCount, videoCount, hasReference: imageCount + videoCount + audioCount > 0 };
}

/** 将 image_urls 的 URL 或图片对象归一为已确认的 images 合同；两个字段不得同时出现。 */
function tokenImages(req, model) {
  if (req.image_urls !== undefined && req.images !== undefined) throw new Error("images and image_urls are mutually exclusive");
  const images = req.image_urls === undefined ? req.images : req.image_urls;
  if (images === undefined) return undefined;
  if (!Array.isArray(images)) throw new Error("images and image_urls must be arrays");
  if (model !== "seedance-2-5-official" && req.image_urls === undefined) return images;
  return images.map((item, index) => {
    const frame = model === "seedance-2-5-official" && images.length === 2 && images.every((image) => typeof image === "string");
    if (typeof item === "string") return { url: item, role: frame ? (index === 0 ? "first_frame" : "last_frame") : "reference_image" };
    if (model === "seedance-2-5-official" && item && typeof item === "object" && !Array.isArray(item) && item.role === undefined)
      return Object.assign({}, item, { role: "reference_image" });
    return item;
  });
}

/** 检查请求对象和顶层字段，内部自动时长标记只能由驱动读取。 */
function validateTopLevelFields(req, allowed, allowAutoDuration) {
  if (!req || typeof req !== "object" || Array.isArray(req)) throw new Error("request body must be an object");
  for (const key of Object.keys(req)) {
    if (allowAutoDuration && key === "auto_duration") continue;
    if (!allowed.includes(key)) throw new Error("unsupported Moon request field: " + key);
  }
}

/** 校验 Wan URL/file_id 素材，并返回数量及参考视频总时长。 */
function validateWanReferenceList(value, field, maxCount) {
  if (value === undefined) return { count: 0, videoSeconds: 0 };
  if (!Array.isArray(value) || value.length > maxCount) throw new Error(field + " must contain at most " + maxCount + " references");
  let videoSeconds = 0;
  for (const item of value) {
    if (!item || typeof item !== "object" || Array.isArray(item)) throw new Error(field + " items must be objects");
    const hasURL = Object.prototype.hasOwnProperty.call(item, "url");
    const hasFileID = Object.prototype.hasOwnProperty.call(item, "file_id");
    if (hasURL === hasFileID) throw new Error(field + " items must contain exactly one of url or file_id");
    if ((hasURL && !isHTTPURL(item.url)) || (hasFileID && !trimmed(item.file_id)))
      throw new Error(field + " items must contain a valid HTTP(S) url or file_id");
    const allowed = ["url", "file_id"];
    if (field === "reference_images") allowed.push("role");
    if (field === "reference_videos") allowed.push("duration");
    for (const key of Object.keys(item)) if (!allowed.includes(key)) throw new Error("unsupported " + field + " reference field: " + key);
    if (field === "reference_images" && item.role !== undefined && !["first_frame", "last_frame"].includes(item.role))
      throw new Error("reference image role must be first_frame or last_frame");
    if (field === "reference_videos") {
      if (!Object.prototype.hasOwnProperty.call(item, "duration")) throw new Error("reference video duration is required");
      videoSeconds += numericReferenceDuration(item.duration, "reference video duration");
    }
  }
  return { count: value.length, videoSeconds: videoSeconds };
}

/** 校验 Moon Wan 原生请求并返回计费事实。 */
function validateWanRequest(req, model) {
  validateTopLevelFields(req, WAN_FIELDS, false);
  if (typeof req.prompt !== "string" || !req.prompt.trim()) throw new Error("prompt must be a non-empty string");
  if (req.prompt.length > 20000) throw new Error("prompt must not exceed 20000 characters");
  if (/--(?:duration|resolution)\b/i.test(req.prompt)) throw new Error("inline duration and resolution overrides are not supported");
  if (req.prompt_extend !== undefined) ensureBoolean(req.prompt_extend, "prompt_extend");
  const duration = normalizeDuration(req, model);
  const resolution = normalizeResolution(req.resolution === undefined ? "720P" : req.resolution, model);
  const ratio = normalizeRatio(req, model);
  const images = validateWanReferenceList(req.reference_images, "reference_images", 10);
  const videos = validateWanReferenceList(req.reference_videos, "reference_videos", 5);
  const audios = validateWanReferenceList(req.reference_audios, "reference_audios", 5);
  let documentCount = 0;
  if (req.input !== undefined) {
    if (!req.input || typeof req.input !== "object" || Array.isArray(req.input)) throw new Error("Moon Wan input must be an object");
    for (const key of Object.keys(req.input)) if (key !== "media") throw new Error("unsupported Moon Wan input field: " + key);
    const media = req.input.media;
    if (!Array.isArray(media) || media.length > 1) throw new Error("Moon Wan input.media supports at most one file or link");
    for (const item of media) {
      if (!item || typeof item !== "object" || Array.isArray(item) || !["file", "link"].includes(item.type) || !isHTTPURL(item.url))
        throw new Error("Moon Wan document media must contain type file or link and an HTTP(S) url");
      for (const key of Object.keys(item)) if (key !== "type" && key !== "url") throw new Error("unsupported Moon Wan document field: " + key);
    }
    documentCount = media.length;
  }
  const frames = (req.reference_images || []).filter((item) => item.role !== undefined);
  if (frames.length) {
    const firstFrames = frames.filter((item) => item.role === "first_frame").length;
    const lastFrames = frames.filter((item) => item.role === "last_frame").length;
    if (firstFrames > 1 || lastFrames > 1) throw new Error("Moon Wan supports at most one first_frame and one last_frame");
    if (images.count !== frames.length || videos.count || audios.count || documentCount)
      throw new Error("Moon Wan frames cannot be combined with other reference media");
  }
  const referenceCount = images.count + videos.count + audios.count;
  if (referenceCount > 12) throw new Error("Moon Wan supports at most 12 total references");
  if (videos.videoSeconds > 15) throw new Error("Moon Wan reference video duration must not exceed 15 seconds in total");
  return {
    duration: duration,
    resolution: resolution,
    ratio: ratio,
    referenceVideoSeconds: videos.videoSeconds,
    videoInput: videos.count > 0 ? "video" : "none",
    imageCount: images.count,
    videoCount: videos.count,
    hasReference: referenceCount + documentCount > 0,
  };
}

/** 校验 Moon Seedance 原生请求并返回计费事实。 */
function validateTokenRequest(req, model, allowAutoDuration) {
  validateTopLevelFields(req, TOKEN_FIELDS, allowAutoDuration);
  if (typeof req.prompt !== "undefined" && (typeof req.prompt !== "string" || !req.prompt.trim())) throw new Error("prompt must be a non-empty string");
  const hasContent = req.content !== undefined;
  if (
    hasContent &&
    (req.prompt !== undefined ||
      req.images !== undefined ||
      req.image_urls !== undefined ||
      req.videos !== undefined ||
      req.audios !== undefined ||
      req.first_frame !== undefined ||
      req.last_frame !== undefined)
  )
    throw new Error("content cannot be combined with prompt or reference arrays");
  const hasPrompt = typeof req.prompt === "string" && req.prompt.trim() !== "";
  if (!hasPrompt && !hasContent) throw new Error("prompt or content is required");
  const duration = normalizeDuration(req, model);
  const resolution = normalizeResolution(req.resolution === undefined ? "720p" : req.resolution, model);
  const ratio = normalizeRatio(req, model);
  const references = validateTokenReferences(req, duration, ratio, model);
  for (const field of TOKEN_BOOLEAN_FIELDS) if (req[field] !== undefined) ensureBoolean(req[field], field);
  if (req.output_format !== undefined && !["mp4", "mov"].includes(req.output_format)) throw new Error("output_format must be mp4 or mov");
  if (req.seed !== undefined && (!Number.isInteger(req.seed) || req.seed < -1 || req.seed > 2147483647))
    throw new Error("seed must be between -1 and 2147483647");
  return { duration, resolution, ratio, videoInput: references.videoCount > 0 ? "video" : "none", ...references };
}

/** 新系列的素材仅接受公网 HTTP(S) 直链；DNS 可访问性仍由供应商验证。 */
function isPublicMediaURL(value) {
  if (!isHTTPURL(value)) return false;
  const host = /^https?:\/\/(\[[^\]]+\]|[^:/?#]+)/i.exec(value)[1].toLowerCase();
  if (host === "localhost" || host.endsWith(".localhost") || host.endsWith(".local") || host === "[::1]") return false;
  if (/^(?:0|10|127|169\.254|192\.168)\./.test(host)) return false;
  const private172 = /^172\.(\d+)\./.exec(host);
  if (private172 && Number(private172[1]) >= 16 && Number(private172[1]) <= 31) return false;
  return true;
}

/** 校验官转与底价渠道的尺寸别名，同时保留显式分辨率和比例冲突。 */
function resolutionFromSize(req, model, ratio) {
  const sizes = {
    "854x480": ["480p", "16:9"],
    "832x480": ["480p", "16:9"],
    "864x480": ["480p", "16:9"],
    "480x854": ["480p", "9:16"],
    "1280x720": ["720p", "16:9"],
    "720x1280": ["720p", "9:16"],
    "480p": ["480p", ""],
    "720p": ["720p", ""],
  };
  const size = req.size === undefined ? null : sizes[trimmed(req.size).toLowerCase()];
  if (req.size !== undefined && !size) throw new Error("unsupported Moon video size");
  const resolution = normalizeResolution(req.resolution === undefined ? (size ? size[0] : "720p") : req.resolution, model);
  if (size && (size[0] !== resolution || (size[1] && size[1] !== ratio))) throw new Error("size conflicts with resolution or ratio");
  return resolution;
}

/** 官转视频和音频逐段时长用于素材校验，不参与输出秒数计价。 */
function ptReferenceDuration(value) {
  if (typeof value !== "number" || !Number.isFinite(value) || value < 2 || value > 30)
    throw new Error("Moon PT reference durationSeconds must be a number between 2 and 30");
}

/** 官转四种素材入口互斥，并按实际分辨率/模型限制计数。 */
function validatePTRequest(req, model) {
  validateTopLevelFields(req, PT_FIELDS, false);
  const duration = normalizeDuration(req, model);
  const ratio = normalizeRatio(req, model);
  const resolution = resolutionFromSize(req, model, ratio);
  if (req.generate_audio !== undefined) ensureBoolean(req.generate_audio, "generate_audio");
  if (req.generateAudio !== undefined) ensureBoolean(req.generateAudio, "generateAudio");
  if (req.generate_audio !== undefined && req.generateAudio !== undefined && req.generate_audio !== req.generateAudio)
    throw new Error("generate_audio and generateAudio conflict");
  const hasArrays = ["reference_images", "reference_videos", "reference_audios"].some((key) => req[key] !== undefined);
  const entries = [req.content !== undefined, req.materials !== undefined, hasArrays, req.input_reference !== undefined].filter(Boolean);
  if (entries.length > 1) throw new Error("Moon PT reference input forms are mutually exclusive");
  if (req.prompt !== undefined && (typeof req.prompt !== "string" || !req.prompt.trim())) throw new Error("prompt must be a non-empty string");
  let text = req.prompt || "";
  const counts = { images: 0, videos: 0, audios: 0 };
  const frames = [];
  const addMedia = (type, role, url, durationSeconds) => {
    if (!isPublicMediaURL(url)) throw new Error("Moon PT reference must be a public HTTP(S) URL");
    if (type === "image") {
      if (!["first_frame", "last_frame", "reference_image"].includes(role)) throw new Error("unsupported Moon PT image role");
      counts.images++;
      if (role !== "reference_image") frames.push(role);
    } else if (type === "video" || type === "audio") {
      if (role !== "reference_" + type) throw new Error("unsupported Moon PT reference role");
      ptReferenceDuration(durationSeconds);
      counts[type === "video" ? "videos" : "audios"]++;
    } else throw new Error("unsupported Moon PT reference type");
  };
  if (req.content !== undefined) {
    if (!Array.isArray(req.content) || !req.content.length) throw new Error("Moon PT content must be a non-empty array");
    let contentText = "";
    for (const item of req.content) {
      if (!item || typeof item !== "object" || Array.isArray(item)) throw new Error("invalid Moon PT content item");
      if (item.type === "text") {
        if (typeof item.text !== "string" || !item.text.trim() || Object.keys(item).some((key) => !["type", "text"].includes(key)))
          throw new Error("invalid Moon PT content text");
        contentText += item.text;
        continue;
      }
      const type = { image_url: "image", video_url: "video", audio_url: "audio" }[item.type];
      if (!type) throw new Error("unsupported Moon PT content type");
      const media = item[item.type];
      if (
        !media ||
        typeof media !== "object" ||
        Array.isArray(media) ||
        Object.keys(media).some((key) => key !== "url") ||
        Object.keys(item).some((key) => !["type", item.type, "role", "durationSeconds"].includes(key))
      )
        throw new Error("invalid Moon PT content reference");
      if (type === "image" && item.durationSeconds !== undefined) throw new Error("image durationSeconds is not supported");
      addMedia(type, item.role || (type === "image" ? "reference_image" : "reference_" + type), media.url, item.durationSeconds);
    }
    if (!contentText.trim() || (req.prompt !== undefined && req.prompt !== contentText)) throw new Error("Moon PT prompt and content text conflict");
    text = contentText;
  }
  if (req.materials !== undefined) {
    if (!Array.isArray(req.materials)) throw new Error("Moon PT materials must be an array");
    for (const item of req.materials) {
      if (
        !item ||
        typeof item !== "object" ||
        Array.isArray(item) ||
        Object.keys(item).some((key) => !["type", "role", "url", "durationSeconds"].includes(key))
      )
        throw new Error("invalid Moon PT material");
      if (item.type === "image" && item.durationSeconds !== undefined) throw new Error("image durationSeconds is not supported");
      addMedia(item.type, item.role || (item.type === "image" ? "reference_image" : "reference_" + item.type), item.url, item.durationSeconds);
    }
  }
  if (hasArrays) {
    for (const [field, type] of [
      ["reference_images", "image"],
      ["reference_videos", "video"],
      ["reference_audios", "audio"],
    ]) {
      if (req[field] === undefined) continue;
      if (!Array.isArray(req[field])) throw new Error(field + " must be an array");
      for (const item of req[field]) {
        const media = typeof item === "string" ? { url: item } : item;
        if (
          !media ||
          typeof media !== "object" ||
          Array.isArray(media) ||
          Object.keys(media).some((key) => !["url", ...(type === "image" ? ["role"] : ["durationSeconds"])].includes(key))
        )
          throw new Error("invalid Moon PT " + field + " reference");
        addMedia(type, type === "image" ? media.role || "reference_image" : "reference_" + type, media.url, media.durationSeconds);
      }
    }
  }
  if (req.input_reference !== undefined) addMedia("image", "first_frame", req.input_reference);
  if (!text.trim()) throw new Error("Moon PT prompt or content text is required");
  const is25 = model === "seedance2.5-30-10-10-PT";
  const limits = is25 ? [30, 10, 10, 50] : model === "seedance2.0-fast-PT" ? [9, 0, 3, 12] : [9, 3, 3, 15];
  if (text.length > (model === "seedance2.0-fast-PT" ? 4000 : is25 ? 10000 : 6000)) throw new Error("Moon PT prompt exceeds model limit");
  if (counts.images > limits[0] || counts.videos > limits[1] || counts.audios > limits[2] || counts.images + counts.videos + counts.audios > limits[3])
    throw new Error("Moon PT reference limit exceeded");
  if (
    frames.filter((role) => role === "first_frame").length > 1 ||
    frames.filter((role) => role === "last_frame").length > 1 ||
    (frames.includes("last_frame") && !frames.includes("first_frame")) ||
    (frames.length && counts.images + counts.videos + counts.audios !== frames.length)
  )
    throw new Error("Moon PT first/last frames cannot be mixed with other references");
  return {
    duration,
    resolution,
    ratio,
    imageCount: counts.images,
    videoCount: counts.videos,
    hasReference: counts.images + counts.videos + counts.audios > 0,
  };
}

/** 底价渠道允许素材字段原序透传，但每一项仍须为公网 URL 且符合模型上限。 */
function validateBudgetRequest(req, model) {
  validateTopLevelFields(req, BUDGET_FIELDS, false);
  if (typeof req.prompt !== "string" || !req.prompt.trim() || req.prompt.length > 5000) throw new Error("Moon budget prompt must contain 1 to 5000 characters");
  if (/@image\d+/i.test(req.prompt)) throw new Error("Moon budget image mentions must use @图片N");
  const duration = normalizeDuration(req, model);
  const ratio = normalizeRatio(req, model);
  const resolution = resolutionFromSize(req, model, ratio);
  const limits = BUDGET_LIMITS[model];
  const range = limits.resolutions[resolution];
  if (duration < range[0] || duration > range[1]) throw new Error("Moon budget duration is outside the model and resolution range");
  const counts = { images: 0, videos: 0, audios: 0 };
  for (const [fields, type] of [
    [["images", "image_urls", "image_refs", "reference_images"], "images"],
    [["videos", "video_urls", "video_refs"], "videos"],
    [["audios", "audio_urls", "audio_refs"], "audios"],
  ]) {
    for (const field of fields) {
      if (req[field] === undefined) continue;
      if (!Array.isArray(req[field])) throw new Error(field + " must be an array");
      for (const item of req[field]) {
        const media = typeof item === "string" ? { url: item } : item;
        if (
          !media ||
          typeof media !== "object" ||
          Array.isArray(media) ||
          !isPublicMediaURL(media.url) ||
          Object.keys(media).some((key) => !["url", ...(type === "images" ? ["role"] : ["durationSeconds"])].includes(key))
        )
          throw new Error("Moon budget references require public HTTP(S) URLs");
        if (type === "images" && media.role !== undefined && !["first_frame", "last_frame", "reference_image"].includes(media.role))
          throw new Error("unsupported Moon budget image role");
        if (media.durationSeconds !== undefined) ptReferenceDuration(media.durationSeconds);
        counts[type]++;
      }
    }
  }
  if (req.input_reference !== undefined) {
    if (!isPublicMediaURL(req.input_reference)) throw new Error("Moon budget input_reference requires a public HTTP(S) URL");
    if (["images", "image_urls", "image_refs", "reference_images"].some((field) => req[field] !== undefined))
      throw new Error("Moon budget input_reference conflicts with other image inputs");
    counts.images++;
  }
  if (
    counts.images > limits.images ||
    counts.videos > limits.videos ||
    counts.audios > limits.audios ||
    counts.images + counts.videos + counts.audios > limits.total
  )
    throw new Error("Moon budget reference limit exceeded");
  return {
    duration,
    resolution,
    ratio,
    model,
    imageCount: counts.images,
    videoCount: counts.videos,
    hasReference: counts.images + counts.videos + counts.audios > 0,
  };
}

/** 校验 H3 URL 数组，素材只能使用可直接访问的 HTTP(S) 地址。 */
function validateH3URLList(value, field, maxCount) {
  if (value === undefined) return 0;
  if (!Array.isArray(value) || value.length > maxCount) throw new Error(field + " must contain at most " + maxCount + " URLs");
  for (const url of value) if (!isHTTPURL(url)) throw new Error(field + " items must be HTTP(S) URLs");
  return value.length;
}

/** 校验 Moon MiniMax H3 原生请求、工作流和精确尺寸。 */
function validateH3Request(req, model) {
  validateTopLevelFields(req, H3_FIELDS, false);
  if (typeof req.prompt !== "string" || !req.prompt.trim()) throw new Error("prompt must be a non-empty string");
  const duration = normalizeDuration(req, model);
  const workflow = req.workflow_id;
  if (!H3_STANDARD_WORKFLOWS.includes(workflow) && !H3_CF_WORKFLOWS.includes(workflow)) throw new Error("unsupported Moon MiniMax H3 workflow_id");
  let resolution;
  if (H3_CF_WORKFLOWS.includes(workflow)) {
    if (!["2K", "4K"].includes(req.size)) throw new Error("Moon MiniMax H3 super-resolution size must be 2K or 4K");
    if (!H3_RATIOS.includes(req.aspect_ratio)) throw new Error("Moon MiniMax H3 super-resolution requires a fixed aspect_ratio");
    resolution = req.size.toLowerCase();
  } else {
    if (req.aspect_ratio !== undefined) throw new Error("Moon MiniMax H3 standard workflows do not accept aspect_ratio");
    resolution = H3_SIZE_RESOLUTIONS[req.size];
    if (!resolution) throw new Error("Moon MiniMax H3 standard workflows require a documented exact size");
  }
  if (req.prompt_enhance !== undefined) ensureBoolean(req.prompt_enhance, "prompt_enhance");
  if (req.mode !== undefined && req.mode !== "first_last_frame") throw new Error("mode must be first_last_frame");
  if (req.mode !== undefined && !["fl2v", "cf-fl2v"].includes(workflow)) throw new Error("mode is supported only by fl2v workflows");
  let imageCount = validateH3URLList(req.images, "images", 9);
  if (req.input_reference !== undefined) {
    if (!isHTTPURL(req.input_reference)) throw new Error("input_reference must be an HTTP(S) URL");
    imageCount++;
  }
  if (imageCount > 9) throw new Error("input_reference and images support at most 9 URLs in total");
  if (req.reference_video !== undefined && req.reference_videos !== undefined) throw new Error("reference_video and reference_videos are mutually exclusive");
  if (req.reference_audio !== undefined && req.reference_audios !== undefined) throw new Error("reference_audio and reference_audios are mutually exclusive");
  let videoCount = validateH3URLList(req.reference_videos, "reference_videos", 3);
  let audioCount = validateH3URLList(req.reference_audios, "reference_audios", 3);
  if (req.reference_video !== undefined) {
    if (!isHTTPURL(req.reference_video)) throw new Error("reference_video must be an HTTP(S) URL");
    videoCount = 1;
  }
  if (req.reference_audio !== undefined) {
    if (!isHTTPURL(req.reference_audio)) throw new Error("reference_audio must be an HTTP(S) URL");
    audioCount = 1;
  }
  const hasReference = imageCount + videoCount + audioCount > 0;
  if (workflow === "text-to-video" && hasReference) throw new Error("text-to-video does not accept reference media");
  if (["multi-reference", "multi-reference-4", "cf-multi-reference"].includes(workflow) && !hasReference)
    throw new Error(workflow + " requires reference media");
  if (workflow === "lh-multi-reference") {
    if (imageCount === 0 || imageCount > 4 || videoCount > 0 || audioCount > 0)
      throw new Error("lh-multi-reference requires 1 to 4 images and no reference audio or video");
    if (duration > 10 || !["480p", "768p"].includes(resolution)) throw new Error("lh-multi-reference supports at most 10 seconds and at most 768p");
  }
  if (["fl2v", "cf-fl2v"].includes(workflow) && (imageCount < 1 || imageCount > 2 || videoCount > 0 || audioCount > 0))
    throw new Error(workflow + " requires 1 to 2 ordered images and no reference audio or video");
  if (workflow === "mj" && !["480p", "768p"].includes(resolution)) throw new Error("mj supports at most 768p");
  return {
    duration: duration,
    resolution: resolution,
    videoInput: videoCount > 0 ? "video" : "none",
    imageCount: imageCount,
    videoCount: videoCount,
    hasReference: hasReference,
    h3: true,
  };
}

/** 校验 Moon Grok 图片、别名和尺寸组合，返回有界的请求时长等规范参数。 */
function validateGrokRequest(req, model) {
  validateTopLevelFields(req, GROK_FIELDS, false);
  if (typeof req.prompt !== "string" || !req.prompt.trim() || req.prompt.length > 32000) throw new Error("Moon Grok prompt must contain 1 to 32000 characters");
  const duration = normalizeDuration(req, model);
  const ratio = normalizeRatio(req, model);
  const exactSize = typeof req.size === "string" && Object.prototype.hasOwnProperty.call(GROK_SIZES, req.size) ? GROK_SIZES[req.size] : null;
  const sizeResolution = req.size === undefined ? undefined : exactSize ? exactSize.resolution : normalizeResolution(req.size, model);
  const resolution = normalizeResolution(req.resolution === undefined ? sizeResolution || "720p" : req.resolution, model);
  if (sizeResolution && resolution !== sizeResolution) throw new Error("resolution and size conflict");
  if (exactSize && exactSize.ratio !== ratio) throw new Error("Moon Grok exact size must match aspect_ratio");
  if (req.input_reference !== undefined && req.reference_images !== undefined) throw new Error("input_reference and reference_images are mutually exclusive");
  if (req.input_reference !== undefined && !isHTTPURL(req.input_reference)) throw new Error("input_reference must be an HTTP(S) URL");
  if (req.reference_images !== undefined) {
    if (!Array.isArray(req.reference_images) || req.reference_images.length < 1 || req.reference_images.length > 7)
      throw new Error("Moon Grok reference_images must contain 1 to 7 images");
    for (const item of req.reference_images) {
      if (!item || typeof item !== "object" || Array.isArray(item) || !isHTTPURL(item.url))
        throw new Error("Moon Grok reference_images items must contain an HTTP(S) url");
      for (const key of Object.keys(item)) if (key !== "url" && key !== "role") throw new Error("unsupported Moon Grok reference image field: " + key);
      if (item.role !== undefined && !["reference_image", "first_frame"].includes(item.role)) throw new Error("unsupported Moon Grok reference image role");
      if (item.role === "first_frame" && req.reference_images.length !== 1)
        throw new Error("Moon Grok first_frame requires exactly one image and cannot be combined with other references");
    }
  }
  return {
    duration,
    resolution,
    ratio,
    grok: true,
    imageCount: req.input_reference !== undefined ? 1 : req.reference_images ? req.reference_images.length : 0,
    videoCount: 0,
    hasReference: req.input_reference !== undefined || req.reference_images !== undefined,
  };
}

/** 按模型系列选择独立请求合同。 */
function validateKnownFields(req, model, allowAutoDuration = false) {
  if (isWan(model)) return validateWanRequest(req, model);
  if (isTokenModel(model)) return validateTokenRequest(req, model, allowAutoDuration);
  if (isPT(model)) return validatePTRequest(req, model);
  if (isBudget(model)) return validateBudgetRequest(req, model);
  if (isH3(model)) return validateH3Request(req, model);
  if (isGrok(model)) return validateGrokRequest(req, model);
  throw new Error("unsupported Moon model");
}

/** 解析 Canvas 内容数组并保留文本与 URL 素材的明确角色。 */
function canvasContent(content, family) {
  if (!Array.isArray(content) || content.length === 0) throw new Error("Moon " + family + " metadata.content must be a non-empty array");
  const parsed = { text: "", textCount: 0, images: [], videos: [], audios: [], firstFrame: "", lastFrame: "" };
  for (const item of content) {
    if (!item || typeof item !== "object" || Array.isArray(item)) throw new Error("Moon " + family + " metadata.content items must be objects");
    if (item.type === "text") {
      if (typeof item.text !== "string" || !item.text.trim()) throw new Error("Moon " + family + " metadata.content text must be non-empty");
      for (const key of Object.keys(item)) if (key !== "type" && key !== "text") throw new Error("unsupported Moon " + family + " text field: " + key);
      parsed.text += item.text;
      parsed.textCount++;
      continue;
    }
    if (!["image_url", "video_url", "audio_url"].includes(item.type)) throw new Error("unsupported Moon " + family + " content item type");
    const reference = item[item.type];
    if (!reference || typeof reference !== "object" || Array.isArray(reference) || !isHTTPURL(reference.url))
      throw new Error("Moon " + family + " content media must contain an HTTP(S) url");
    for (const key of Object.keys(reference)) if (key !== "url") throw new Error("unsupported Moon " + family + " content URL field: " + key);
    for (const key of Object.keys(item))
      if (key !== "type" && key !== item.type && key !== "role") throw new Error("unsupported Moon " + family + " content media field: " + key);
    if (item.type === "image_url") {
      if (!["first_frame", "last_frame", "reference_image"].includes(item.role)) throw new Error("unsupported Moon " + family + " image role");
      if (item.role === "first_frame") {
        if (parsed.firstFrame) throw new Error("Moon " + family + " content has duplicate first_frame items");
        parsed.firstFrame = reference.url;
      } else if (item.role === "last_frame") {
        if (parsed.lastFrame) throw new Error("Moon " + family + " content has duplicate last_frame items");
        parsed.lastFrame = reference.url;
      } else parsed.images.push(reference.url);
    } else if (item.type === "video_url") {
      if (item.role !== "reference_video") throw new Error("Moon " + family + " content video role must be reference_video");
      parsed.videos.push(reference.url);
    } else {
      if (item.role !== "reference_audio") throw new Error("Moon " + family + " content audio role must be reference_audio");
      parsed.audios.push(reference.url);
    }
  }
  if (parsed.textCount !== 1) throw new Error("Moon " + family + " metadata.content must contain exactly one text item");
  return parsed;
}

/** 把 Canvas OpenAI Video 封装转换为 Moon 对应模型系列的原生合同。 */
function normalizeCanvasTextVideoRequest(source, model, clientModel) {
  const request = Object.assign({}, source, { model: clientModel });
  if (!Object.prototype.hasOwnProperty.call(source, "metadata")) return request;
  const metadata = source.metadata;
  if (!metadata || typeof metadata !== "object" || Array.isArray(metadata)) throw new Error("metadata must be an object");

  if (isWan(model)) {
    for (const key of Object.keys(metadata))
      if (!["input", "reference_video_durations"].includes(key)) throw new Error("unsupported Moon Wan metadata field: " + key);
    if (!Object.prototype.hasOwnProperty.call(metadata, "input")) throw new Error("Moon Wan metadata must include input");
    const input = metadata.input;
    if (!input || typeof input !== "object" || Array.isArray(input)) throw new Error("Moon Wan metadata.input must be an object");
    for (const key of Object.keys(input)) if (key !== "media") throw new Error("unsupported Moon Wan metadata.input field: " + key);
    const media = input.media === undefined ? [] : input.media;
    if (!Array.isArray(media)) throw new Error("Moon Wan metadata.input.media must be an array");
    const durations = metadata.reference_video_durations;
    if (durations !== undefined && !Array.isArray(durations)) throw new Error("Moon Wan reference_video_durations must be an array");
    const referenceImages = [];
    const referenceVideos = [];
    const referenceAudios = [];
    const documentMedia = [];
    let videoIndex = 0;
    for (const item of media) {
      if (!item || typeof item !== "object" || Array.isArray(item) || !isHTTPURL(item.url))
        throw new Error("Moon Wan metadata media must contain an HTTP(S) url");
      for (const key of Object.keys(item)) if (key !== "type" && key !== "url") throw new Error("unsupported Moon Wan metadata media field: " + key);
      if (["first_frame", "last_frame", "reference_image"].includes(item.type)) {
        const image = { url: item.url };
        if (item.type !== "reference_image") image.role = item.type;
        referenceImages.push(image);
      } else if (item.type === "reference_video") {
        if (!durations || videoIndex >= durations.length) throw new Error("Moon Wan reference video duration sidecar is required");
        referenceVideos.push({ url: item.url, duration: numericReferenceDuration(durations[videoIndex], "reference video duration") });
        videoIndex++;
      } else if (item.type === "reference_audio") referenceAudios.push({ url: item.url });
      else if (item.type === "file" || item.type === "link") documentMedia.push({ type: item.type, url: item.url });
      else throw new Error("unsupported Moon Wan metadata media type");
    }
    if ((durations || []).length !== videoIndex) throw new Error("Moon Wan reference_video_durations must match reference videos in order");
    for (const field of ["reference_images", "reference_videos", "reference_audios", "input"])
      if (Object.prototype.hasOwnProperty.call(request, field)) throw new Error(field + " conflicts with Moon Wan metadata.input.media");
    if (referenceImages.length) request.reference_images = referenceImages;
    if (referenceVideos.length) request.reference_videos = referenceVideos;
    if (referenceAudios.length) request.reference_audios = referenceAudios;
    if (documentMedia.length) request.input = { media: documentMedia };
    delete request.metadata;
    return request;
  }

  if (isTokenModel(model)) {
    for (const key of Object.keys(metadata))
      if (!["content", "resolution", "ratio", "omni_reference_task_type"].includes(key)) throw new Error("unsupported Moon Seedance metadata field: " + key);
    if (
      !Object.prototype.hasOwnProperty.call(metadata, "content") ||
      !Object.prototype.hasOwnProperty.call(metadata, "resolution") ||
      !Object.prototype.hasOwnProperty.call(metadata, "ratio")
    )
      throw new Error("Moon Seedance metadata must include content, resolution, and ratio");
    const content = canvasContent(metadata.content, "Seedance");
    if (Object.prototype.hasOwnProperty.call(request, "content")) throw new Error("content conflicts with metadata.content");
    if (request.prompt !== content.text) throw new Error("prompt conflicts with metadata.content text");
    const resolution = normalizeResolution(metadata.resolution, model);
    if (Object.prototype.hasOwnProperty.call(request, "resolution") && normalizeResolution(request.resolution, model) !== resolution)
      throw new Error("resolution conflicts with metadata.resolution");
    const ratio = normalizeRatio({ ratio: metadata.ratio }, model);
    if (
      (Object.prototype.hasOwnProperty.call(request, "ratio") || Object.prototype.hasOwnProperty.call(request, "aspect_ratio")) &&
      normalizeRatio(request, model) !== ratio
    )
      throw new Error("ratio conflicts with metadata.ratio");
    let taskType = metadata.omni_reference_task_type;
    if (taskType === "reference") taskType = "auto";
    if (request.omni_reference_task_type !== undefined && taskType !== undefined && request.omni_reference_task_type !== taskType)
      throw new Error("omni_reference_task_type conflicts with metadata");
    request.content = metadata.content.map((item) => Object.assign({}, item));
    request.resolution = resolution;
    request.ratio = ratio;
    if (taskType !== undefined) request.omni_reference_task_type = taskType;
    delete request.prompt;
    delete request.metadata;
    return request;
  }

  if (isPT(model)) {
    for (const key of Object.keys(metadata))
      if (!["content", "resolution", "ratio", "reference_video_durations", "reference_audio_durations"].includes(key))
        throw new Error("unsupported Moon PT metadata field: " + key);
    if (!Array.isArray(metadata.content) || !metadata.content.length) throw new Error("Moon PT metadata.content must be a non-empty array");
    if (
      request.content !== undefined ||
      request.materials !== undefined ||
      request.reference_images !== undefined ||
      request.reference_videos !== undefined ||
      request.reference_audios !== undefined ||
      request.input_reference !== undefined
    )
      throw new Error("Moon PT metadata.content conflicts with reference inputs");
    const durations = { video_url: metadata.reference_video_durations, audio_url: metadata.reference_audio_durations };
    const indexes = { video_url: 0, audio_url: 0 };
    for (const kind of ["video_url", "audio_url"])
      if (durations[kind] !== undefined && !Array.isArray(durations[kind])) throw new Error("Moon PT reference durations must be arrays");
    request.content = metadata.content.map((item) => {
      if (!item || typeof item !== "object" || Array.isArray(item)) throw new Error("invalid Moon PT metadata content");
      const copy = Object.assign({}, item);
      if (item.type === "video_url" || item.type === "audio_url") {
        const index = indexes[item.type]++;
        if (copy.durationSeconds === undefined && durations[item.type] && index < durations[item.type].length)
          copy.durationSeconds = durations[item.type][index];
      }
      return copy;
    });
    for (const kind of ["video_url", "audio_url"])
      if (durations[kind] && durations[kind].length !== indexes[kind]) throw new Error("Moon PT reference durations must match media in order");
    if (metadata.resolution !== undefined) request.resolution = metadata.resolution;
    if (metadata.ratio !== undefined) request.ratio = metadata.ratio;
    delete request.metadata;
    return request;
  }

  if (isBudget(model)) {
    for (const key of Object.keys(metadata))
      if (!["content", "resolution", "ratio"].includes(key)) throw new Error("unsupported Moon budget metadata field: " + key);
    const content = canvasContent(metadata.content, "budget");
    if (request.prompt !== undefined && request.prompt !== content.text) throw new Error("prompt conflicts with metadata.content text");
    for (const field of [
      "images",
      "image_urls",
      "image_refs",
      "reference_images",
      "input_reference",
      "videos",
      "video_urls",
      "video_refs",
      "audios",
      "audio_urls",
      "audio_refs",
    ])
      if (request[field] !== undefined) throw new Error(field + " conflicts with Moon budget metadata.content");
    request.prompt = content.text;
    const images = content.images.map((url) => ({ url, role: "reference_image" }));
    if (content.firstFrame) images.unshift({ url: content.firstFrame, role: "first_frame" });
    if (content.lastFrame) images.push({ url: content.lastFrame, role: "last_frame" });
    if (images.length) request.images = images;
    if (content.videos.length) request.videos = content.videos.slice();
    if (content.audios.length) request.audios = content.audios.slice();
    if (metadata.resolution !== undefined) request.resolution = metadata.resolution;
    if (metadata.ratio !== undefined) request.ratio = metadata.ratio;
    delete request.metadata;
    return request;
  }

  if (isGrok(model)) {
    for (const key of Object.keys(metadata))
      if (!["content", "resolution", "ratio"].includes(key)) throw new Error("unsupported Moon Grok metadata field: " + key);
    const content = canvasContent(metadata.content, "Grok");
    if (request.prompt !== undefined && request.prompt !== content.text) throw new Error("prompt conflicts with metadata.content text");
    request.prompt = content.text;
    if (content.lastFrame || content.videos.length || content.audios.length) throw new Error("Moon Grok supports only first-frame or image references");
    if (request.reference_images !== undefined || request.input_reference !== undefined)
      throw new Error("Moon Grok reference fields conflict with metadata.content");
    if (metadata.resolution !== undefined) {
      const resolution = normalizeResolution(metadata.resolution, model);
      if (request.resolution !== undefined && normalizeResolution(request.resolution, model) !== resolution)
        throw new Error("resolution conflicts with metadata.resolution");
      request.resolution = resolution;
    }
    if (metadata.ratio !== undefined) {
      const ratio = normalizeRatio({ ratio: metadata.ratio }, model);
      if ((request.ratio !== undefined || request.aspect_ratio !== undefined) && normalizeRatio(request, model) !== ratio)
        throw new Error("ratio conflicts with metadata.ratio");
      request.ratio = ratio;
    }
    const images = content.images.map((url) => ({ url, role: "reference_image" }));
    if (content.firstFrame) images.unshift({ url: content.firstFrame, role: "first_frame" });
    if (images.length) request.reference_images = images;
    delete request.metadata;
    return request;
  }

  for (const key of Object.keys(metadata))
    if (!["content", "resolution", "ratio"].includes(key)) throw new Error("unsupported Moon MiniMax H3 metadata field: " + key);
  if (
    !Object.prototype.hasOwnProperty.call(metadata, "content") ||
    !Object.prototype.hasOwnProperty.call(metadata, "resolution") ||
    !Object.prototype.hasOwnProperty.call(metadata, "ratio")
  )
    throw new Error("Moon MiniMax H3 metadata must include content, resolution, and ratio");
  const content = canvasContent(metadata.content, "MiniMax H3");
  if (request.prompt !== content.text) throw new Error("prompt conflicts with metadata.content text");
  const hasDuration = Object.prototype.hasOwnProperty.call(request, "duration");
  const hasSeconds = Object.prototype.hasOwnProperty.call(request, "seconds");
  if (!hasDuration && !hasSeconds) throw new Error("seconds must be an integer between 4 and 15 for Moon MiniMax H3");
  const duration = numericDuration(hasDuration ? request.duration : request.seconds, hasDuration ? "duration" : "seconds");
  if (hasDuration && hasSeconds && duration !== numericDuration(request.seconds, "seconds")) throw new Error("duration and seconds conflict");
  if (duration < 4 || duration > 15) throw new Error("seconds must be an integer between 4 and 15 for Moon MiniMax H3");
  request.seconds = duration;
  delete request.duration;
  const resolution = trimmed(metadata.resolution).toLowerCase();
  if (!H3_RESOLUTIONS.includes(resolution)) throw new Error("unsupported Moon MiniMax H3 Canvas resolution");
  if (Object.prototype.hasOwnProperty.call(request, "resolution") && trimmed(request.resolution).toLowerCase() !== resolution)
    throw new Error("resolution conflicts with metadata.resolution");
  const ratio = normalizeH3CanvasRatio({ ratio: metadata.ratio });
  if (
    (Object.prototype.hasOwnProperty.call(request, "ratio") || Object.prototype.hasOwnProperty.call(request, "aspect_ratio")) &&
    normalizeH3CanvasRatio(request) !== ratio
  )
    throw new Error("ratio conflicts with metadata.ratio");
  for (const field of ["input_reference", "images", "reference_video", "reference_audio", "reference_videos", "reference_audios"])
    if (Object.prototype.hasOwnProperty.call(request, field)) throw new Error(field + " conflicts with Moon MiniMax H3 metadata.content");
  const frameImages = [];
  if (content.firstFrame) frameImages.push(content.firstFrame);
  if (content.lastFrame) frameImages.push(content.lastFrame);
  const onlyFrames = frameImages.length > 0 && content.images.length === 0 && content.videos.length === 0 && content.audios.length === 0;
  let workflow =
    frameImages.length + content.images.length + content.videos.length + content.audios.length === 0
      ? "text-to-video"
      : onlyFrames
        ? "fl2v"
        : "multi-reference";
  let size;
  if (resolution === "2k" || resolution === "4k") {
    if (workflow === "text-to-video") throw new Error("Moon MiniMax H3 has no documented super-resolution text-to-video workflow");
    workflow = workflow === "fl2v" ? "cf-fl2v" : "cf-multi-reference";
    size = resolution.toUpperCase();
  } else {
    size = H3_CANVAS_SIZES[ratio] && H3_CANVAS_SIZES[ratio][resolution];
    if (!size) throw new Error("Moon MiniMax H3 Canvas resolution and ratio do not map to a documented size");
  }
  if (request.size !== undefined && request.size !== size) throw new Error("size conflicts with metadata resolution and ratio");
  if (request.workflow_id !== undefined && request.workflow_id !== workflow) throw new Error("workflow_id conflicts with metadata.content");
  request.size = size;
  request.workflow_id = workflow;
  if (resolution === "2k" || resolution === "4k") request.aspect_ratio = ratio;
  else delete request.aspect_ratio;
  if (onlyFrames) request.images = frameImages;
  else if (frameImages.length || content.images.length) request.images = frameImages.concat(content.images);
  if (content.videos.length) request.reference_videos = content.videos;
  if (content.audios.length) request.reference_audios = content.audios;
  delete request.resolution;
  delete request.ratio;
  delete request.metadata;
  return request;
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
  if (facts.h3) request.seconds = facts.duration;
  if (facts.grok || isBudget(facts.model)) {
    request.seconds = facts.duration;
    delete request.duration;
  }
  return request;
}

/** 生成各模型系列的规范上游 JSON，不修改宿主传入对象。 */
function normalizeRequest(req, model, facts) {
  const values = Object.assign({}, req || {});
  values.model = model;
  if (isPT(model) || isBudget(model)) {
    delete values.size;
    delete values.aspect_ratio;
    values.resolution = facts.resolution;
    values.ratio = facts.ratio;
    if (isPT(model)) {
      values.duration = facts.duration;
      delete values.seconds;
      if (values.generateAudio !== undefined) {
        values.generate_audio = values.generateAudio;
        delete values.generateAudio;
      }
    } else {
      values.seconds = facts.duration;
      delete values.duration;
    }
    return values;
  }
  if (isGrok(model)) {
    values.seconds = facts.duration;
    values.size = facts.resolution;
    values.aspect_ratio = facts.ratio;
    delete values.duration;
    delete values.resolution;
    delete values.ratio;
    if (values.reference_images !== undefined)
      values.reference_images = values.reference_images.map((item) => ({ url: item.url, role: item.role === undefined ? "reference_image" : item.role }));
    return values;
  }
  if (isH3(model)) {
    values.seconds = normalizeDuration(req, model);
    for (const field of ["images", "reference_videos", "reference_audios"]) if (values[field] !== undefined) values[field] = values[field].slice();
    return values;
  }
  delete values.auto_duration;
  delete values.seconds;
  values.duration = normalizeDuration(req, model);
  values.resolution = normalizeResolution(req.resolution === undefined ? "720p" : req.resolution, model).toLowerCase();
  if (isWan(model)) {
    delete values.ratio;
    values.aspect_ratio = normalizeRatio(req, model);
    for (const field of ["reference_images", "reference_videos", "reference_audios"])
      if (values[field] !== undefined) values[field] = values[field].map((item) => Object.assign({}, item));
    if (values.input !== undefined) values.input = { media: values.input.media.map((item) => ({ type: item.type, url: item.url })) };
    return values;
  }
  delete values.aspect_ratio;
  values.ratio = normalizeRatio(req, model);
  if (values.image_urls !== undefined || (model === "seedance-2-5-official" && values.images !== undefined)) {
    values.images = tokenImages(req, model).map((item) => Object.assign({}, item));
    delete values.image_urls;
  }
  if (values.content !== undefined) values.content = values.content.map((item) => Object.assign({}, item));
  return values;
}

/** 把 Responses 文本和 URL 图片转成中立输入；各模型系列随后映射其原生引用字段。 */
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
            result.images.push(url);
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
  const upstreamModel = trimmed(ctx.upstreamModel || model);
  if (!MODELS.includes(upstreamModel)) throw new Error("unsupported Moon model");
  const fields = isWan(upstreamModel)
    ? WAN_FIELDS
    : isTokenModel(upstreamModel)
      ? TOKEN_FIELDS
      : isPT(upstreamModel)
        ? PT_FIELDS
        : isBudget(upstreamModel)
          ? BUDGET_FIELDS
          : isGrok(upstreamModel)
            ? GROK_FIELDS
            : H3_FIELDS;
  for (const key of Object.keys(source)) {
    if (!fields.includes(key) && !["input", "stream", "background"].includes(key)) throw new Error("unsupported Moon Responses field: " + key);
  }
  if (source.stream !== undefined) ensureBoolean(source.stream, "stream");
  if (source.background !== undefined) ensureBoolean(source.background, "background");
  if (source.input !== undefined && (source.prompt !== undefined || source.content !== undefined))
    throw new Error("input cannot be combined with prompt or content");
  const input = responsesInput(source);
  const request = { model: model };
  if (input.prompt) request.prompt = input.prompt;
  for (const key of fields) if (key !== "model" && key !== "input" && source[key] !== undefined) request[key] = source[key];
  if (input.images.length) {
    const imageField = isWan(upstreamModel) || isPT(upstreamModel) || isGrok(upstreamModel) ? "reference_images" : "images";
    if (request[imageField] !== undefined && !Array.isArray(request[imageField])) throw new Error(imageField + " must be an array");
    const images = isWan(upstreamModel)
      ? input.images.map((url) => ({ url: url }))
      : isTokenModel(upstreamModel) || isGrok(upstreamModel)
        ? input.images.map((url) => ({ url: url, role: "reference_image" }))
        : input.images.slice();
    request[imageField] = images.concat(request[imageField] || []);
  }
  const facts = validateKnownFields(request, upstreamModel);
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
  candidates.push(body.video_url, body.url, body.output && body.output.video_url, body.output && body.output.content_url, body.metadata && body.metadata.url);
  for (const candidate of candidates) if (isHTTPURL(candidate)) return candidate;
  return "";
}

/** 构造一次性 JSON 提交描述；H3 上限 64 MiB，其余系列上限 256 KiB。 */
export function buildSubmitRequest(ctx) {
  const request = ctx.requestBody || {};
  const model = modelName(ctx, request);
  if (!MODELS.includes(model)) throw new Error("unsupported Moon model");
  const facts = validateKnownFields(request, model, true);
  const normalized = normalizeRequest(request, model, facts);
  let bodyBytes = 0;
  for (const character of JSON.stringify(normalized)) {
    const point = character.codePointAt(0);
    bodyBytes += point < 128 ? 1 : point < 2048 ? 2 : point < 65536 ? 3 : 4;
  }
  const maxBodyBytes = isH3(model) ? 67108864 : 262144;
  if (bodyBytes > maxBodyBytes)
    throw new Error(isH3(model) ? "Moon MiniMax H3 JSON request must not exceed 64 MiB" : "Moon JSON request must not exceed 256 KiB");
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

/** 按 24 × 宽 × 高 / 1024 估算 tokens；自动输出和每段参考视频按模型最大时长预留。 */
function estimateTokens(duration, resolution, videoInputCount, model) {
  const maxSeconds = model === "seedance-2-5-official" ? 30 : 15;
  const seconds = duration === -1 ? maxSeconds : duration;
  const totalSeconds = seconds + videoInputCount * maxSeconds;
  const pixels = resolutionPixels(resolution);
  return (totalSeconds * pixels[0] * pixels[1] * 24) / 1024;
}

/** 将已校验的请求素材条目转换为输入计费事实；零素材也返回 0，重复引用不去重。 */
function inputUsageFacts(facts) {
  return {
    image_input_count: facts.imageCount || 0,
    video_input_count: facts.videoCount || 0,
  };
}

/** 返回宿主用量事实；billing_ratios 不伪造 Moon 的币值或倍率。 */
export function extractUsage(ctx) {
  const request = ctx.requestBody || {};
  const model = modelName(ctx, request);
  if (!MODELS.includes(model)) throw new Error("unsupported Moon model");
  if (ctx.usagePurpose === "billing_ratios") return null;
  const facts = validateKnownFields(request, model, true);
  if (isGrok(model)) return { video_count: 1, seconds: facts.duration, ...inputUsageFacts(facts) };
  if (isPT(model)) return { seconds: facts.duration, resolution: facts.resolution, ...inputUsageFacts(facts) };
  if (isBudget(model)) {
    return BUDGET_PER_REQUEST_MODELS.includes(model)
      ? { video_count: 1, resolution: facts.resolution, ...inputUsageFacts(facts) }
      : { seconds: facts.duration, resolution: facts.resolution, ...inputUsageFacts(facts) };
  }
  if (isWan(model)) return { seconds: facts.duration + facts.referenceVideoSeconds, resolution: facts.resolution, ...inputUsageFacts(facts) };
  if (isH3(model)) return { seconds: facts.duration, resolution: facts.resolution, ...inputUsageFacts(facts) };
  return {
    tokens: estimateTokens(facts.duration, facts.resolution, facts.videoCount, model),
    resolution: facts.resolution,
    video_input: facts.videoInput,
    ...inputUsageFacts(facts),
  };
}

/** 提取 H3 完成响应中的有界秒数和规范分辨率；积分字段不参与美元定价。 */
function actualH3Usage(body) {
  const billing = body && body.billing;
  if (!billing || typeof billing !== "object" || Array.isArray(billing)) return null;
  const facts = {};
  const rawSeconds = billing.seconds;
  let seconds = null;
  if (typeof rawSeconds === "number") seconds = rawSeconds;
  else if (typeof rawSeconds === "string" && /^\d+$/.test(rawSeconds.trim())) seconds = Number(rawSeconds);
  if (Number.isInteger(seconds) && seconds >= 4 && seconds <= 15) facts.seconds = seconds;
  const resolution = trimmed(billing.resolution).toLowerCase();
  if (H3_RESOLUTIONS.includes(resolution)) facts.resolution = resolution;
  return Object.keys(facts).length ? facts : null;
}

/** Seedance 使用实际 tokens，H3 采纳有界事实；Wan/Grok 请求秒数保留于提交快照，不从积分或未确认字段推测。 */
export function extractUsageOnComplete(task, result, body) {
  if (!result || result.status !== "SUCCESS" || !body || typeof body !== "object") return null;
  const source = taskPayload(body);
  const model = modelName(task, source);
  if (isGrok(model)) return { video_count: 1 };
  if (isH3(model)) return actualH3Usage(source);
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
    funding_pending: "IN_PROGRESS",
    reserving: "IN_PROGRESS",
    reservation_unknown: "IN_PROGRESS",
    refund_pending: "IN_PROGRESS",
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
    const model = modelName(ctx, source);
    if ((isGrok(model) || isPT(model) || isBudget(model)) && !url) return { status: "IN_PROGRESS", reason: "waiting for a valid Moon video URL" };
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

/** 原始成片 URL 无密钥直连；限时 Moon 公播链接改用所有者鉴权的 /content。 */
export function buildContentRequest(ctx) {
  if (!ctx || ctx.artifactKey !== "video") throw new Error("artifact_not_found");
  const direct = artifactURL(ctx.data);
  if (direct && !/\/v1\/videos\/public\//.test(direct)) return { url: direct, method: ctx.clientRequest.method, credentialless: true };
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
      const upstreamModel = trimmed(ctx.upstreamModel || model);
      if (!MODELS.includes(upstreamModel)) throw new Error("unsupported Moon model");
      const request = normalizeCanvasTextVideoRequest(source, upstreamModel, model);
      const facts = validateKnownFields(request, upstreamModel);
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
