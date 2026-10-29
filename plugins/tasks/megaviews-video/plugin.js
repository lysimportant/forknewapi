/**
 * MegaAi（megaviews.top）统一视频 API 协议适配（宿主 openai_video 协议）。
 *
 * 本插件把宿主的 OpenAI 视频入口翻译成 MegaAi 平台公开的视频异步任务接口：
 *   POST /v1/videos
 *   GET  /v1/videos/{task_id}
 *   GET  /v1/videos/{task_id}/content
 *
 * 平台把多个厂商的视频模型（Seedance、Wan、MiniMax H3、SD 等）统一为同一套参数：
 * duration（秒）、ratio（16:9）、resolution（480p 等）、seed、referenceImages、
 * referenceVideos、referenceAudios 与模型专属 metadata。插件入口同时接受宿主
 * OpenAI 视频字段（seconds/size/input_reference）与 Canvas 传入的 metadata.content，
 * 统一转换成上述平台字段；无法用统一字段表达的模型专属参数放进 metadata 透传。
 * 参考素材必须是上游可访问的公网 HTTP(S) 直链，本地文件上传无法提交给该接口。
 *
 * 模型不在插件里写死：渠道使用 modelRouting "channel"，管理员在类型 61 渠道里读取
 * 上游 /v1/models 目录后自行勾选视频模型。售价由管理员在本站配置，插件只报告
 * 请求时长、清晰度等计费用量事实；上游按“生成时长 + 参考素材时长”计费的模型，
 * 查询响应会报告与实际计费一致的 billing_seconds，完成时按该值结算实际秒数。
 */

export const meta = {
  apiVersion: 1,
  key: "megaviews-video",
  name: "MegaAi 视频",
  description: {
    en: "MegaAi unified video generation gateway",
    zh: "MegaAi 统一视频生成网关",
  },
  version: "1.0.0",
  author: { name: "lysimportant/forknewapi" },
  website: "https://megaviews.top/docs",
  baseUrl: "https://megaviews.top",
  allowedHosts: ["megaviews.top:443", "www.megaviews.top:443", "newapi.megaviews.top:443", "newapi.megabyai.cc:443"],
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
      unitLabel: { en: "video", zh: "条" },
      description: { en: "Video generation unit price", zh: "视频生成单价" },
    },
    seconds: {
      type: "number",
      unit: "second",
      description: { en: "Video generation unit price", zh: "视频生成单价" },
    },
    video_resolution: {
      enum: ["480p", "720p", "768p", "1080p", "4k"],
      description: { en: "Output video resolution", zh: "输出视频分辨率" },
      enumLabels: { "768p": { en: "768P", zh: "768P" }, "4k": { en: "4K", zh: "4K" } },
    },
  },
  usageExamples: [
    { label: "480p · 5s", facts: { count: 1, seconds: 5, video_resolution: "480p" } },
    { label: "720p · 5s", facts: { count: 1, seconds: 5, video_resolution: "720p" } },
    { label: "768p · 5s", facts: { count: 1, seconds: 5, video_resolution: "768p" } },
    { label: "1080p · 5s", facts: { count: 1, seconds: 5, video_resolution: "1080p" } },
    { label: "4k · 5s", facts: { count: 1, seconds: 5, video_resolution: "4k" } },
  ],
};

/** 与宿主 MaxTaskDurationSeconds 保持一致的安全上限，不代表模型支持的时长。 */
const MIN_SECONDS = 1;
const MAX_SECONDS = 3600;
/** 平台随机种子范围为 0 到 2147483647；0 是明确指定的种子，不是缺省值。 */
const MAX_SEED = 2147483647;
/** 计费用量事实只使用五档规范清晰度；上游按模型可能写 720P、2K 等。 */
const RESOLUTIONS = ["480p", "720p", "768p", "1080p", "4k"];

/** 仅接收已确认的入口字段及平台别名，未知字段直接报错而不是静默丢弃。 */
const REQUEST_FIELDS = new Set([
  "model",
  "prompt",
  "seconds",
  "duration",
  "duration_seconds",
  "durationSeconds",
  "video_duration",
  "ratio",
  "aspect_ratio",
  "resolution",
  "video_resolution",
  "videoResolution",
  "size",
  "seed",
  "n",
  "image",
  "input_reference",
  "first_frame",
  "first_frame_url",
  "start_frame",
  "last_frame",
  "last_frame_url",
  "end_frame",
  "end_frame_url",
  "images",
  "image_urls",
  "reference_images",
  "referenceImages",
  "reference_image_urls",
  "videos",
  "video_urls",
  "reference_videos",
  "referenceVideos",
  "reference_video_urls",
  "audios",
  "audio_urls",
  "reference_audios",
  "referenceAudios",
  "reference_audio_urls",
  "quality",
  "negative_prompt",
  "generate_audio",
  "watermark",
  "metadata",
]);

const FIRST_FRAME_FIELDS = ["image", "input_reference", "first_frame", "first_frame_url", "start_frame"];
const LAST_FRAME_FIELDS = ["last_frame", "last_frame_url", "end_frame", "end_frame_url"];
const IMAGE_LIST_FIELDS = ["images", "image_urls", "reference_images", "referenceImages", "reference_image_urls"];
const VIDEO_LIST_FIELDS = ["videos", "video_urls", "reference_videos", "referenceVideos", "reference_video_urls"];
const AUDIO_LIST_FIELDS = ["audios", "audio_urls", "reference_audios", "referenceAudios", "reference_audio_urls"];
const SECONDS_FIELDS = ["duration", "seconds", "duration_seconds", "durationSeconds", "video_duration"];
const RATIO_FIELDS = ["ratio", "aspect_ratio"];
const RESOLUTION_FIELDS = ["resolution", "video_resolution", "videoResolution"];
const METADATA_MEDIA_FIELDS = new Set([...IMAGE_LIST_FIELDS, ...VIDEO_LIST_FIELDS, ...AUDIO_LIST_FIELDS, "metadata"]);
const NUMERIC_FORM_FIELDS = new Set([...SECONDS_FIELDS, "seed", "n"]);
/** 平台 metadata 是模型专属扩展；这些入口字段不能由 metadata 重写，防止绕过计费校验。 */
const METADATA_CONTROL_FIELDS = new Set(["model", "prompt", "seconds", "duration", "duration_seconds", "durationSeconds", "video_duration", "seed", "n"]);
/** 统一接口没有对应字段的宿主可选参数，放进 metadata 交给具体模型解释。 */
const METADATA_PASSTHROUGH_FIELDS = ["quality", "negative_prompt", "generate_audio", "watermark"];

function fail(message) {
  throw new Error(message);
}

function trimmed(value) {
  return typeof value === "string" ? value.trim() : "";
}

function isObject(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function clone(value) {
  return JSON.parse(JSON.stringify(value));
}

/** 兼容渠道根地址和 /v1 地址，保留自定义路径前缀并拒绝地址内的凭据、查询或片段。 */
function apiBase(ctx) {
  let base = trimmed(ctx && ctx.baseUrl) || meta.baseUrl;
  base = base.replace(/\/+$/, "");
  if (!/^https:\/\/[^/?#@\s\\]+(?:\/[^?#\s\\]*)?$/.test(base)) fail("channel base URL must be an HTTPS API base without credentials, query or fragment");
  return base.replace(/\/v1$/, "") + "/v1";
}

/** 校验绝对媒体地址；内网、回环与链路本地地址不允许作为上游参考素材。 */
function isMediaURL(value) {
  if (typeof value !== "string") return false;
  const url = value.trim();
  if (!url) return false;
  if (!/^https?:\/\/[^/?#@\s\\]+(?:[/?#][^\s\\]*)?$/.test(url)) return false;
  const match = /^https?:\/\/([^/?#@\s\\]+)/.exec(url);
  if (!match) return false;
  const host = match[1].toLowerCase();
  if (host === "localhost" || host === "::1" || host === "[::1]" || host.endsWith(".localhost") || host.endsWith(".local")) return false;
  if (/^0\.0\.0\.0/.test(host) || /^127\./.test(host) || /^10\./.test(host) || /^192\.168\./.test(host) || /^169\.254\./.test(host)) return false;
  if (/^172\.(1[6-9]|2[0-9]|3[01])\./.test(host)) return false;
  return true;
}

function mediaURL(value, field) {
  if (isObject(value) && typeof value.url === "string") return mediaURL(value.url, field);
  if (typeof value === "string" && isMediaURL(value)) return value.trim();
  fail(field + " must be a public HTTP(S) media URL");
}

function conflictURL(current, url, label) {
  if (current && current !== url) fail("conflicting " + label + " references");
  return url;
}

function mediaItems(value, field) {
  if (value === undefined || value === null) return [];
  if (typeof value === "string") {
    const text = value.trim();
    if (text.charAt(0) === "[") {
      let parsed;
      try {
        parsed = JSON.parse(text);
      } catch (e) {
        fail(field + " must be a JSON array of media URLs");
      }
      return mediaItems(parsed, field);
    }
    return [text];
  }
  if (Array.isArray(value)) return value;
  return [value];
}

/** 平台参考素材是 URL 字符串数组；画布传入的 {url, duration_seconds} 只取 URL。 */
function mediaItem(value, field) {
  if (typeof value === "string") return mediaURL(value, field);
  if (!isObject(value)) fail(field + " must contain media URLs");
  const keys = Object.keys(value);
  for (let index = 0; index < keys.length; index += 1) {
    if (keys[index] !== "url" && keys[index] !== "duration_seconds") fail("unsupported " + field + " item field: " + keys[index]);
  }
  const url = mediaURL(value.url, field);
  if (value.duration_seconds !== undefined) {
    const duration = Number(value.duration_seconds);
    if (!Number.isInteger(duration) || duration < MIN_SECONDS || duration > MAX_SECONDS)
      fail(field + " duration_seconds must be an integer between 1 and 3600");
  }
  return url;
}

function collectMedia(source, field, aliases) {
  let items;
  for (let index = 0; index < aliases.length; index += 1) {
    const alias = aliases[index];
    if (source[alias] === undefined || source[alias] === null) continue;
    const list = mediaItems(source[alias], alias);
    const normalized = [];
    for (let itemIndex = 0; itemIndex < list.length; itemIndex += 1) {
      normalized.push(mediaItem(list[itemIndex], alias));
    }
    if (items !== undefined) {
      if (JSON.stringify(items) !== JSON.stringify(normalized)) fail(alias + " conflicts with " + field);
      continue;
    }
    items = normalized;
  }
  return items || [];
}

function firstFrameURL(value, field) {
  if (Array.isArray(value)) {
    if (value.length !== 1) fail(field + " must contain a single image URL");
    return firstFrameURL(value[0], field);
  }
  if (typeof value === "string") return mediaURL(value, field);
  if (!isObject(value)) fail(field + " must be an image URL");
  const keys = Object.keys(value);
  for (let index = 0; index < keys.length; index += 1) {
    if (keys[index] !== "url" && keys[index] !== "image_url") fail("unsupported " + field + " field: " + keys[index]);
  }
  if (value.url !== undefined) return mediaURL(value.url, field);
  if (value.image_url !== undefined) {
    if (typeof value.image_url === "string") return mediaURL(value.image_url, field);
    if (isObject(value.image_url) && value.image_url.url !== undefined) return mediaURL(value.image_url.url, field);
  }
  fail(field + " must be an image URL");
}

function parseSeconds(value, field) {
  const seconds = typeof value === "string" && value.trim() !== "" ? Number(value.trim()) : value;
  if (typeof seconds !== "number" || !Number.isInteger(seconds) || seconds < MIN_SECONDS || seconds > MAX_SECONDS)
    fail(field + " must be an integer between 1 and 3600");
  return seconds;
}

function parseSeed(value, field) {
  const seed = typeof value === "string" && value.trim() !== "" ? Number(value.trim()) : value;
  if (typeof seed !== "number" || !Number.isInteger(seed) || seed < 0 || seed > MAX_SEED) fail(field + " must be an integer between 0 and 2147483647");
  return seed;
}

function parseRatio(value, field) {
  const text = trimmed(value);
  if (!/^[1-9][0-9]{0,3}:[1-9][0-9]{0,3}$/.test(text)) fail(field + " must be a ratio like 16:9");
  return text;
}

function parseResolution(value, field) {
  const text = trimmed(value).toLowerCase();
  if (!text || !/^\S{1,32}$/.test(text)) fail(field + " must be a resolution tier like 720p");
  return text;
}

function collectScalar(source, field, aliases, parser) {
  let value;
  for (let index = 0; index < aliases.length; index += 1) {
    const alias = aliases[index];
    if (source[alias] === undefined || source[alias] === null) continue;
    const parsed = parser(source[alias], alias);
    if (value !== undefined && value !== parsed) fail(alias + " conflicts with " + field);
    value = parsed;
  }
  return value;
}

function greatestCommonDivisor(left, right) {
  while (right) {
    const remainder = left % right;
    left = right;
    right = remainder;
  }
  return left;
}

function parseSize(value) {
  const match = /^([0-9]{2,5})x([0-9]{2,5})$/i.exec(trimmed(value));
  if (!match) fail("size must be a pixel size like 1280x720");
  return { width: Number(match[1]), height: Number(match[2]) };
}

function sizeRatio(size) {
  const divisor = greatestCommonDivisor(size.width, size.height);
  return size.width / divisor + ":" + size.height / divisor;
}

/** OpenAIVideo 的 size 没有档位概念，只能按短边归一到平台支持的清晰度档。 */
function sizeResolution(size) {
  const shortEdge = Math.min(size.width, size.height);
  if (shortEdge >= 2000) return "4k";
  if (shortEdge >= 1000) return "1080p";
  if (shortEdge >= 740) return "768p";
  if (shortEdge >= 640) return "720p";
  if (shortEdge >= 400) return "480p";
  return "";
}

/** 把上游清晰度写法归一到计费枚举；无法识别时返回空串（不写用量事实）。 */
function resolutionFact(value) {
  const text = trimmed(value).toLowerCase();
  if (RESOLUTIONS.indexOf(text) >= 0) return text;
  if (text === "2160p" || text === "2160") return "4k";
  if (text === "1080" || text === "fhd") return "1080p";
  if (text === "768") return "768p";
  if (text === "720" || text === "hd") return "720p";
  if (text === "480" || text === "sd") return "480p";
  return "";
}

function mergeMetadataScalar(source, metadata, field) {
  if (metadata[field] === undefined) return;
  if (source[field] !== undefined && JSON.stringify(source[field]) !== JSON.stringify(metadata[field])) fail("metadata." + field + " conflicts with " + field);
  source[field] = metadata[field];
}

/** 宿主可选参数没有平台顶层字段时放进 metadata；已有同名值时要求一致。 */
function putMetadata(metadata, field, value) {
  if (metadata[field] !== undefined && JSON.stringify(metadata[field]) !== JSON.stringify(value)) fail(field + " conflicts with metadata." + field);
  metadata[field] = value;
}

function collectContent(metadata, prompt, target) {
  const content = metadata.content;
  if (content === undefined) return;
  if (!Array.isArray(content)) fail("metadata.content must be an array");
  for (let index = 0; index < content.length; index += 1) {
    const item = content[index];
    if (!isObject(item)) fail("metadata.content must contain objects");
    const type = item.type;
    if (type === "text") {
      const keys = Object.keys(item);
      for (let keyIndex = 0; keyIndex < keys.length; keyIndex += 1) {
        if (keys[keyIndex] !== "type" && keys[keyIndex] !== "text") fail("unsupported metadata.content text field: " + keys[keyIndex]);
      }
      if (typeof item.text !== "string" || item.text.trim() !== prompt) fail("metadata.content text must match prompt");
      continue;
    }
    if (type !== "image_url" && type !== "video_url" && type !== "audio_url") fail("unsupported metadata.content item type: " + String(type));
    const keys = Object.keys(item);
    for (let keyIndex = 0; keyIndex < keys.length; keyIndex += 1) {
      const key = keys[keyIndex];
      if (key !== "type" && key !== "role" && key !== type) fail("unsupported metadata.content item field: " + key);
    }
    const url = mediaURL(item[type], "metadata.content." + type);
    const role = item.role === undefined || item.role === null ? "" : trimmed(item.role);
    if (type === "image_url") {
      if (role === "first_frame") target.firstFrame = conflictURL(target.firstFrame, url, "first frame");
      else if (role === "last_frame") fail("last frame is not supported by the MegaAi unified video API");
      else if (role === "" || role === "reference_image") target.images.push(url);
      else fail("unsupported metadata.content image role: " + role);
      continue;
    }
    if (type === "video_url") {
      if (role === "" || role === "reference_video" || role === "content") target.videos.push(url);
      else fail("unsupported metadata.content video role: " + role);
      continue;
    }
    if (role === "" || role === "reference_audio" || role === "audioTrack") target.audios.push(url);
    else fail("unsupported metadata.content audio role: " + role);
  }
}

/**
 * 把宿主入口请求归一成平台统一参数；返回的 requestBody 是驱动钩子与计费事实的唯一来源。
 * 不补默认时长、清晰度或素材数量：具体模型支持范围由上游按模型校验。
 */
function normalizeRequest(request) {
  if (!isObject(request)) fail("video request body must be a JSON object");
  const fields = Object.keys(request);
  for (let index = 0; index < fields.length; index += 1) {
    if (!REQUEST_FIELDS.has(fields[index])) fail("unsupported video request field: " + fields[index]);
  }
  const source = clone(request);
  if (source.model !== undefined && typeof source.model !== "string") fail("model must be a string");

  const prompt = trimmed(source.prompt);
  if (!prompt) fail("prompt is required");

  const target = { images: [], videos: [], audios: [], firstFrame: "" };
  let metadataRest;

  const metadata = source.metadata;
  if (metadata !== undefined) {
    if (!isObject(metadata)) fail("metadata must be an object");
    const rest = {};
    const metadataFields = Object.keys(metadata);
    for (let index = 0; index < metadataFields.length; index += 1) {
      const field = metadataFields[index];
      if (field === "content" || field === "resolution" || field === "ratio" || field === "aspect_ratio") continue;
      if (METADATA_CONTROL_FIELDS.has(field)) fail("metadata must not override " + field);
      rest[field] = clone(metadata[field]);
    }
    mergeMetadataScalar(source, metadata, "resolution");
    mergeMetadataScalar(source, metadata, "ratio");
    mergeMetadataScalar(source, metadata, "aspect_ratio");
    collectContent(metadata, prompt, target);
    if (Object.keys(rest).length > 0) metadataRest = rest;
  }

  const images = collectMedia(source, "images", IMAGE_LIST_FIELDS);
  const videos = collectMedia(source, "videos", VIDEO_LIST_FIELDS);
  const audios = collectMedia(source, "audios", AUDIO_LIST_FIELDS);

  for (let index = 0; index < LAST_FRAME_FIELDS.length; index += 1) {
    if (source[LAST_FRAME_FIELDS[index]] !== undefined) fail("last frame is not supported by the MegaAi unified video API");
  }

  let firstFrame = target.firstFrame;
  for (let index = 0; index < FIRST_FRAME_FIELDS.length; index += 1) {
    const field = FIRST_FRAME_FIELDS[index];
    if (source[field] === undefined || source[field] === null) continue;
    firstFrame = conflictURL(firstFrame, firstFrameURL(source[field], field), "first frame");
  }

  const duration = collectScalar(source, "duration", SECONDS_FIELDS, parseSeconds);
  let ratio = collectScalar(source, "ratio", RATIO_FIELDS, parseRatio);
  let resolution = collectScalar(source, "resolution", RESOLUTION_FIELDS, parseResolution);
  if (source.size !== undefined && source.size !== null) {
    if (typeof source.size !== "string") fail("size must be a pixel size like 1280x720");
    const size = parseSize(source.size);
    const derivedRatio = sizeRatio(size);
    if (ratio !== undefined && ratio !== derivedRatio) fail("ratio conflicts with size");
    ratio = derivedRatio;
    const derivedResolution = sizeResolution(size);
    if (derivedResolution) {
      const fact = resolutionFact(resolution);
      if (fact && fact !== derivedResolution) fail("resolution conflicts with size");
      if (resolution === undefined) resolution = derivedResolution;
    }
  }

  let seed;
  if (source.seed !== undefined && source.seed !== null) seed = parseSeed(source.seed, "seed");
  if (source.n !== undefined && source.n !== null) {
    if ((typeof source.n !== "number" && typeof source.n !== "string") || Number(source.n) !== 1) fail("n must be 1");
  }

  const passthroughMetadata = Object.assign({}, metadataRest || {});
  for (let index = 0; index < METADATA_PASSTHROUGH_FIELDS.length; index += 1) {
    const field = METADATA_PASSTHROUGH_FIELDS[index];
    if (source[field] === undefined || source[field] === null) continue;
    if (field === "generate_audio" && typeof source[field] !== "boolean") fail("generate_audio must be a boolean");
    putMetadata(passthroughMetadata, field, source[field]);
  }

  const normalized = { prompt: prompt };
  if (duration !== undefined) normalized.duration = duration;
  if (ratio !== undefined) normalized.ratio = ratio;
  if (resolution !== undefined) normalized.resolution = resolution;
  if (seed !== undefined) normalized.seed = seed;
  const referenceImages = (firstFrame ? [firstFrame] : []).concat(images, target.images);
  if (referenceImages.length > 0) normalized.referenceImages = referenceImages;
  const referenceVideos = videos.concat(target.videos);
  if (referenceVideos.length > 0) normalized.referenceVideos = referenceVideos;
  const referenceAudios = audios.concat(target.audios);
  if (referenceAudios.length > 0) normalized.referenceAudios = referenceAudios;
  if (Object.keys(passthroughMetadata).length > 0) normalized.metadata = passthroughMetadata;
  return normalized;
}

/** 读取后端已经解码的请求体；文件上传无法转成平台需要的公网素材地址。 */
function decodedBody(body) {
  if (!isObject(body)) fail("video request body is missing");
  if (body.kind === "json") {
    if (!isObject(body.value)) fail("video request body must be a JSON object");
    return clone(body.value);
  }
  if (body.kind !== "form" && body.kind !== "multipart") fail("unsupported video request body kind");
  const files = body.files;
  if (Array.isArray(files) && files.length > 0) fail("MegaAi requires public HTTP(S) reference media; upload the file elsewhere and pass its URL");
  if (!isObject(body.fields)) fail("video request fields are missing");
  const request = {};
  const fieldNames = Object.keys(body.fields);
  for (let index = 0; index < fieldNames.length; index += 1) {
    const field = fieldNames[index];
    const values = body.fields[field];
    if (!Array.isArray(values) || values.length === 0) fail("form field " + field + " is missing");
    if (field === "metadata") {
      if (values.length !== 1) fail("form field metadata must be submitted once");
      try {
        request.metadata = JSON.parse(values[0]);
      } catch (e) {
        fail("form field metadata must be valid JSON");
      }
      continue;
    }
    if (METADATA_MEDIA_FIELDS.has(field)) {
      if (values.length === 1 && trimmed(values[0]).charAt(0) === "[") {
        try {
          request[field] = JSON.parse(values[0]);
        } catch (e) {
          fail("form field " + field + " must be valid JSON");
        }
      } else {
        request[field] = values;
      }
      continue;
    }
    if (values.length !== 1) fail("form field " + field + " must be submitted once");
    request[field] = NUMERIC_FORM_FIELDS.has(field) ? Number(values[0]) : values[0];
  }
  return request;
}

function upstreamTaskID(value, label) {
  const id = typeof value === "string" ? value.trim() : typeof value === "number" ? String(value) : "";
  if (!/^[A-Za-z0-9_.:-]{1,200}$/.test(id)) fail(label + " task id is invalid");
  return id;
}

/** 兼容渠道可能把任务对象包在 data 里；只在外层没有任务 ID 时解包。 */
function envelopeData(body) {
  if (isObject(body) && isObject(body.data) && body.id === undefined && body.task_id === undefined) return body.data;
  return body;
}

function progressValue(value) {
  if (typeof value === "number" && Number.isFinite(value)) {
    const bounded = Math.max(0, Math.min(100, Math.round(value)));
    return bounded + "%";
  }
  const text = trimmed(value);
  return /^[0-9]{1,3}%$/.test(text) ? text : "";
}

function sanitizeReason(value, ctx) {
  let reason = typeof value === "string" ? value.trim() : "";
  if (!reason) reason = "Video generation failed";
  const apiKey = trimmed(ctx && ctx.apiKey);
  if (apiKey) reason = reason.split(apiKey).join("[redacted]");
  reason = reason.replace(/Bearer\s+\S+/gi, "[redacted]");
  reason = reason.replace(/\bsk-[A-Za-z0-9_-]{4,}/g, "[redacted]");
  return reason.slice(0, 500);
}

function failureReason(data, ctx) {
  if (isObject(data.error)) {
    const message = trimmed(data.error.message) || trimmed(data.error.code);
    if (message) return sanitizeReason(message, ctx);
  }
  return sanitizeReason(data.fail_reason || data.message || data.reason, ctx);
}

/**
 * 严格映射平台文档状态并保留失败原因；未知状态返回 UNKNOWN 交给宿主累计轮询失败，
 * 不把无法识别的响应当成进行中。成功结果统一从宿主内容入口 /content 代理获取。
 */
export function parseTaskResult(ctx, body) {
  const data = envelopeData(body);
  if (!isObject(data)) return { status: "UNKNOWN" };
  const status = trimmed(data.status).toLowerCase();
  if (status === "completed" || status === "succeeded" || status === "success") {
    return { status: "SUCCESS", progress: "100%" };
  }
  if (status === "failed" || status === "failure" || status === "canceled" || status === "cancelled" || status === "error") {
    return { status: "FAILURE", reason: failureReason(data, ctx) };
  }
  if (status === "queued" || status === "pending" || status === "submitted" || status === "not_start") {
    return { status: "QUEUED" };
  }
  if (status === "in_progress" || status === "processing" || status === "running") {
    const result = { status: "IN_PROGRESS" };
    const progress = progressValue(data.progress);
    if (progress) result.progress = progress;
    return result;
  }
  if (data.error) return { status: "FAILURE", reason: failureReason(data, ctx) };
  return { status: "UNKNOWN" };
}

/** 组装平台统一请求体；模型使用渠道映射后的精确上游 ID。 */
function upstreamBody(request, model) {
  const normalized = normalizeRequest(request);
  const body = { model: model, prompt: normalized.prompt };
  const scalarFields = ["duration", "ratio", "resolution", "seed"];
  for (let index = 0; index < scalarFields.length; index += 1) {
    const field = scalarFields[index];
    if (normalized[field] !== undefined) body[field] = normalized[field];
  }
  const mediaFields = ["referenceImages", "referenceVideos", "referenceAudios"];
  for (let index = 0; index < mediaFields.length; index += 1) {
    const field = mediaFields[index];
    if (normalized[field] !== undefined) body[field] = normalized[field].slice();
  }
  if (normalized.metadata !== undefined) body.metadata = clone(normalized.metadata);
  return body;
}

/**
 * 平台没有承诺通用幂等键；提交结果未知时由 noRetry 阻止宿主自动重发，避免重复计费。
 */
export function buildSubmitRequest(ctx) {
  const model = trimmed(ctx && ctx.upstreamModel) || trimmed(ctx && ctx.model);
  if (!model) fail("upstream model is required");
  const apiKey = trimmed(ctx && ctx.apiKey);
  if (!apiKey) fail("channel API key is required");
  const body = upstreamBody(ctx && ctx.requestBody, model);
  return {
    url: apiBase(ctx) + "/videos",
    method: "POST",
    headers: {
      Authorization: "Bearer " + apiKey,
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: body,
    noRetry: true,
  };
}

export function parseSubmitResponse(ctx, response) {
  if (!response || typeof response.statusCode !== "number" || response.statusCode < 200 || response.statusCode >= 300) fail("MegaAi video submission failed");
  const body = response.body;
  if (!isObject(body)) fail("MegaAi video submission response must be a JSON object");
  const data = envelopeData(body);
  if (data.error && data.id === undefined && data.task_id === undefined) {
    fail(failureReason(data, ctx));
  }
  const rawID = data.id !== undefined ? data.id : data.task_id;
  const taskId = upstreamTaskID(rawID, "MegaAi submit response");
  const result = parseTaskResult(ctx, body);
  if (result.status === "UNKNOWN") fail("MegaAi submit response did not include a usable task status");
  const submitted = {
    taskId: taskId,
    taskData: body,
    state: { model: trimmed(ctx && (ctx.upstreamModel || ctx.model)) },
  };
  if (result.status === "SUCCESS" || result.status === "FAILURE") submitted.immediate = result;
  return submitted;
}

export function buildQueryRequest(ctx) {
  const taskId = upstreamTaskID(ctx && ctx.taskId, "MegaAi query");
  const apiKey = trimmed(ctx && ctx.apiKey);
  if (!apiKey) fail("channel API key is required");
  return {
    url: apiBase(ctx) + "/videos/" + encodeURIComponent(taskId),
    method: "GET",
    headers: {
      Authorization: "Bearer " + apiKey,
      Accept: "application/json",
    },
  };
}

/**
 * 提交时按次计量始终保留；只有请求明确给出时长或可识别的清晰度时才报告按秒/按清晰度
 * 计费事实，不猜测上游默认值。倍率阶段不重复叠加用量计费。
 */
export function extractUsage(ctx) {
  if (!ctx || ctx.usagePurpose === "billing_ratios") return {};
  let parsed;
  try {
    parsed = normalizeRequest(ctx.requestBody);
  } catch (e) {
    return {};
  }
  const facts = { count: 1 };
  if (parsed.duration !== undefined) facts.seconds = parsed.duration;
  const resolution = parsed.resolution !== undefined ? resolutionFact(parsed.resolution) : "";
  if (resolution) facts.video_resolution = resolution;
  return facts;
}

/** 平台报告的实际计费秒数包含参考视频时长，用于把预扣时长结算到真实用量。 */
function billedSeconds(data) {
  const source = envelopeData(data);
  if (!isObject(source)) return undefined;
  const fields = ["billing_seconds", "generated_seconds"];
  for (let index = 0; index < fields.length; index += 1) {
    const value = source[fields[index]];
    const seconds = typeof value === "string" && value.trim() !== "" ? Number(value.trim()) : value;
    if (typeof seconds !== "number" || !Number.isInteger(seconds) || seconds <= 0 || seconds > MAX_SECONDS) continue;
    return seconds;
  }
  return undefined;
}

/** 成功任务按平台报告的计费秒数结算；没有可信秒数时沿用提交阶段冻结的时长与清晰度。 */
export function extractUsageOnComplete(_task, result, data) {
  if (!result || result.status !== "SUCCESS") return null;
  const seconds = billedSeconds(data);
  if (seconds === undefined) return null;
  return { count: 1, seconds: seconds };
}

/** 成功任务通过宿主内容入口代理平台鉴权下载，不向客户端暴露上游素材地址。 */
export function listArtifacts(task) {
  if (!task || task.status !== "SUCCESS") return [];
  return [{ key: "video", type: "video", mimeType: "video/mp4" }];
}

export function buildContentRequest(ctx) {
  if (!ctx || ctx.artifactKey !== "video") fail("artifact_not_found");
  const data = isObject(ctx.data) ? envelopeData(ctx.data) : null;
  const rawID =
    ctx.upstreamTaskId !== undefined && ctx.upstreamTaskId !== null ? ctx.upstreamTaskId : data ? (data.id !== undefined ? data.id : data.task_id) : undefined;
  const taskId = upstreamTaskID(rawID, "MegaAi content");
  const apiKey = trimmed(ctx.apiKey);
  if (!apiKey) fail("channel API key is required");
  const method = ctx.clientRequest && typeof ctx.clientRequest.method === "string" && ctx.clientRequest.method ? ctx.clientRequest.method : "GET";
  return {
    url: apiBase(ctx) + "/videos/" + encodeURIComponent(taskId) + "/content",
    method: method,
    headers: {
      Authorization: "Bearer " + apiKey,
      Accept: "video/*",
    },
  };
}

export const protocols = {
  openai_video: {
    decodeRequest: function (ctx) {
      const request = decodedBody(ctx && ctx.body);
      const model = trimmed(ctx && ctx.upstreamModel) || trimmed(ctx && ctx.model);
      if (!model) fail("upstream model is required");
      const parsed = normalizeRequest(request);
      const images = parsed.referenceImages || [];
      const videos = parsed.referenceVideos || [];
      const audios = parsed.referenceAudios || [];
      const action =
        videos.length > 0 || audios.length > 0 || images.length > 1 ? "reference_to_video" : images.length > 0 ? "image_to_video" : "text_to_video";
      const alias = trimmed(ctx && ctx.model) || model;
      return { kind: "submit", model: alias, action: action, requestBody: Object.assign({ model: alias }, parsed) };
    },
    render: function (_ctx, task) {
      const statuses = {
        NOT_START: "queued",
        SUBMITTED: "queued",
        QUEUED: "queued",
        IN_PROGRESS: "in_progress",
        SUCCESS: "completed",
        FAILURE: "failed",
      };
      const output = {
        id: task && task.task_id,
        object: "video",
        model: (task && task.properties && task.properties.origin_model_name) || meta.key,
        status: (task && statuses[task.status]) || "unknown",
      };
      if (task && task.status === "FAILURE") {
        output.error = { code: "video_generation_failed", message: "Video generation failed" };
      }
      return output;
    },
  },
};
