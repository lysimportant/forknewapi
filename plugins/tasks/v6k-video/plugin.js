/**
 * V6K 统一视频 API 协议适配（宿主 openai_video 协议）。
 *
 * 本插件把宿主的 OpenAI 视频入口翻译成 V6K 站点公开的统一视频接口：
 *   POST /v1/videos
 *   GET  /v1/videos/{id}
 *   GET|HEAD /v1/videos/{id}/content
 *
 * 模型不在插件里写死，也不按型号拆分参数模板：渠道使用 modelRouting "channel"，
 * 管理员在类型 61 渠道里读取上游 /v1/models 目录后自行勾选任意视频模型。
 * 各模型支持的时长、分辨率、素材数量上限由上游按模型校验，插件只做协议字段
 * 归一化、公共媒体 URL 校验和计费用量事实提取。
 */

export const meta = {
  apiVersion: 1,
  key: "v6k-video",
  name: "V6K 视频",
  description: {
    en: "V6K site OpenAI-compatible video generation",
    zh: "通过 V6K 站点的 OpenAI 兼容视频接口生成视频",
  },
  version: "1.1.0",
  author: { name: "lysimportant/forknewapi" },
  website: "https://www.v6k.top",
  baseUrl: "https://www.v6k.top",
  allowedHosts: ["www.v6k.top:443"],
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
      enum: ["480p", "720p", "1080p", "4k"],
      description: { en: "Output video resolution", zh: "输出视频分辨率" },
    },
  },
  usageExamples: [
    { label: "480p · 5 秒", facts: { count: 1, seconds: 5, input_images: 0, input_videos: 0, input_audios: 0, video_resolution: "480p" } },
    { label: "720p · 5 秒", facts: { count: 1, seconds: 5, input_images: 0, input_videos: 0, input_audios: 0, video_resolution: "720p" } },
    { label: "1080p · 5 秒", facts: { count: 1, seconds: 5, input_images: 0, input_videos: 0, input_audios: 0, video_resolution: "1080p" } },
    { label: "4k · 5 秒", facts: { count: 1, seconds: 5, input_images: 0, input_videos: 0, input_audios: 0, video_resolution: "4k" } },
    { label: "720p · 5 秒 · 参考素材", facts: { count: 1, seconds: 5, input_images: 1, input_videos: 1, input_audios: 1, video_resolution: "720p" } },
  ],
};

const MIN_SECONDS = 1;
const MAX_SECONDS = 3600;
/** 计费用量事实只使用四档规范清晰度；上游按模型可能写 720P/1080P 等。 */
const RESOLUTIONS = ["480p", "720p", "1080p", "4k"];
/** 参考素材按件计费，宿主对 count 用量事实的上限为 128。 */
const MAX_INPUT_MEDIA = 128;

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
  "quality",
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
  "image",
  "input_reference",
  "first_frame",
  "first_frame_url",
  "start_frame",
  "last_frame",
  "last_frame_url",
  "end_frame",
  "negative_prompt",
  "n",
  "metadata",
]);

const FIRST_FRAME_FIELDS = ["image", "input_reference", "first_frame", "first_frame_url", "start_frame"];
const LAST_FRAME_FIELDS = ["last_frame", "last_frame_url", "end_frame"];
const IMAGE_LIST_FIELDS = ["images", "image_urls", "reference_images", "referenceImages", "reference_image_urls"];
const VIDEO_LIST_FIELDS = ["videos", "video_urls", "reference_videos", "referenceVideos", "reference_video_urls"];
const AUDIO_LIST_FIELDS = ["audios", "audio_urls", "reference_audios", "referenceAudios", "reference_audio_urls"];
const SECONDS_FIELDS = ["seconds", "duration", "duration_seconds", "durationSeconds", "video_duration"];
const RATIO_FIELDS = ["ratio", "aspect_ratio"];
const RESOLUTION_FIELDS = ["resolution", "video_resolution", "videoResolution"];
const METADATA_MEDIA_FIELDS = new Set([...IMAGE_LIST_FIELDS, ...VIDEO_LIST_FIELDS, ...AUDIO_LIST_FIELDS, "metadata"]);
const NUMERIC_FORM_FIELDS = new Set([...SECONDS_FIELDS, "n"]);

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

function apiBase(ctx) {
  let base = trimmed(ctx && ctx.baseUrl) || meta.baseUrl;
  base = base.replace(/\/+$/, "");
  if (!/^https:\/\/[^\/?#@\s\\]+(?:\/[^?#\s\\]*)?$/.test(base))
    fail("channel base URL must be an HTTPS API base without credentials, query or fragment");
  base = base.replace(/\/v1$/, "");
  return base + "/v1";
}

function isMediaURL(value) {
  if (typeof value !== "string") return false;
  const url = value.trim();
  if (!url) return false;
  if (!/^https?:\/\/[^\/?#@\s\\]+(?:[/?#][^\s\\]*)?$/.test(url)) return false;
  const match = /^https?:\/\/([^\/?#@\s\\]+)/.exec(url);
  if (!match) return false;
  const host = match[1].toLowerCase();
  if (host === "localhost" || host === "::1" || host === "[::1]" || host.endsWith(".localhost") || host.endsWith(".local"))
    return false;
  if (/^0\.0\.0\.0/.test(host) || /^127\./.test(host) || /^10\./.test(host) || /^192\.168\./.test(host) || /^169\.254\./.test(host))
    return false;
  if (/^172\.(1[6-9]|2[0-9]|3[01])\./.test(host)) return false;
  return true;
}

function mediaURL(value, field) {
  if (isObject(value) && typeof value.url === "string") return mediaURL(value.url, field);
  if (typeof value === "string" && isMediaURL(value)) return value.trim();
  fail(field + " must be a public HTTP(S) URL");
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
      } catch (error) {
        fail(field + " must be a JSON array of media URLs");
      }
      return mediaItems(parsed, field);
    }
    return [text];
  }
  if (Array.isArray(value)) return value;
  return [value];
}

function mediaItem(value, field) {
  if (typeof value === "string") return mediaURL(value, field);
  if (!isObject(value)) fail(field + " must contain media URLs");
  const keys = Object.keys(value);
  for (let index = 0; index < keys.length; index += 1) {
    const key = keys[index];
    if (key !== "url" && key !== "duration_seconds") fail("unsupported " + field + " item field: " + key);
  }
  const item = { url: mediaURL(value.url, field) };
  if (value.duration_seconds !== undefined) {
    const duration = Number(value.duration_seconds);
    if (!Number.isInteger(duration) || duration < MIN_SECONDS || duration > MAX_SECONDS)
      fail(field + " duration_seconds must be an integer between 1 and 3600");
    item.duration_seconds = duration;
  }
  return item;
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
    const key = keys[index];
    if (key !== "url" && key !== "image_url") fail("unsupported " + field + " field: " + key);
  }
  if (value.url !== undefined) return mediaURL(value.url, field);
  if (value.image_url !== undefined) {
    if (typeof value.image_url === "string") return mediaURL(value.image_url, field);
    if (isObject(value.image_url) && value.image_url.url !== undefined) return mediaURL(value.image_url.url, field);
  }
  fail(field + " must be an image URL");
}

function parseSeconds(value, field) {
  let seconds;
  if (typeof value === "number") {
    seconds = value;
  } else if (typeof value === "string") {
    const text = value.trim();
    if (!/^[0-9]{1,4}$/.test(text)) fail(field + " must be an integer between 1 and 3600");
    seconds = Number(text);
  } else {
    fail(field + " must be an integer between 1 and 3600");
  }
  if (!Number.isInteger(seconds) || seconds < MIN_SECONDS || seconds > MAX_SECONDS)
    fail(field + " must be an integer between 1 and 3600");
  return seconds;
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

function parseRatio(value, field) {
  const text = trimmed(value);
  if (!/^[1-9][0-9]{0,3}:[1-9][0-9]{0,3}$/.test(text)) fail(field + " must be a ratio like 16:9");
  return text;
}

function parseResolution(value, field) {
  const text = trimmed(value);
  if (!text || !/^\S{1,32}$/.test(text)) fail(field + " must be a model resolution tier like 720p");
  return text;
}

function parseSize(value, field) {
  const text = trimmed(value);
  if (!/^[0-9]{2,5}x[0-9]{2,5}$/i.test(text)) fail(field + " must be a pixel size like 1280x720");
  return text;
}

/** 把上游分辨率写法归一到计费枚举；无法识别时返回空串（不写用量事实）。 */
function resolutionFact(value) {
  const text = trimmed(value).toLowerCase();
  if (RESOLUTIONS.indexOf(text) >= 0) return text;
  if (text === "2160p" || text === "2160") return "4k";
  if (text === "1080" || text === "fhd") return "1080p";
  if (text === "720" || text === "hd") return "720p";
  if (text === "480" || text === "sd") return "480p";
  return "";
}

/** 只提供像素尺寸的模型用短边归一到计费枚举。 */
function resolutionFactFromSize(size) {
  const match = /^([0-9]{2,5})x([0-9]{2,5})$/i.exec(trimmed(size));
  if (!match) return "";
  const shortEdge = Math.min(Number(match[1]), Number(match[2]));
  if (shortEdge >= 2160) return "4k";
  if (shortEdge >= 1080) return "1080p";
  if (shortEdge >= 720) return "720p";
  if (shortEdge >= 480) return "480p";
  return "";
}

function mergeMetadataScalar(source, metadata, field) {
  if (metadata[field] === undefined) return;
  if (source[field] !== undefined && JSON.stringify(source[field]) !== JSON.stringify(metadata[field]))
    fail("metadata." + field + " conflicts with " + field);
  source[field] = metadata[field];
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
        if (keys[keyIndex] !== "type" && keys[keyIndex] !== "text")
          fail("unsupported metadata.content text field: " + keys[keyIndex]);
      }
      if (typeof item.text !== "string" || item.text.trim() !== prompt)
        fail("metadata.content text must match prompt");
      continue;
    }
    if (type !== "image_url" && type !== "video_url" && type !== "audio_url")
      fail("unsupported metadata.content item type: " + String(type));
    const keys = Object.keys(item);
    for (let keyIndex = 0; keyIndex < keys.length; keyIndex += 1) {
      const key = keys[keyIndex];
      if (key !== "type" && key !== "role" && key !== type)
        fail("unsupported metadata.content item field: " + key);
    }
    const url = mediaURL(item[type], "metadata.content." + type);
    const role = item.role === undefined || item.role === null ? "" : trimmed(item.role);
    if (type === "image_url") {
      if (role === "first_frame") target.first = conflictURL(target.first, url, "first frame");
      else if (role === "last_frame") fail("last frame is not supported by the V6K unified video API");
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

function normalizeRequest(request, model) {
  if (!isObject(request)) fail("video request body must be a JSON object");
  const fields = Object.keys(request);
  for (let index = 0; index < fields.length; index += 1) {
    if (!REQUEST_FIELDS.has(fields[index])) fail("unsupported video request field: " + fields[index]);
  }
  const source = clone(request);
  if (source.model !== undefined && typeof source.model !== "string") fail("model must be a string");

  const prompt = trimmed(source.prompt);
  if (!prompt) fail("prompt is required");

  const target = { prompt: prompt, images: [], videos: [], audios: [] };

  const metadata = source.metadata;
  let metadataRest;
  if (metadata !== undefined) {
    if (!isObject(metadata)) fail("metadata must be an object");
    const metadataFields = Object.keys(metadata);
    const rest = {};
    for (let index = 0; index < metadataFields.length; index += 1) {
      const field = metadataFields[index];
      if (field === "content" || field === "resolution" || field === "ratio" || field === "aspect_ratio") continue;
      if (
        field === "model" ||
        field === "seconds" ||
        field === "duration" ||
        field === "duration_seconds" ||
        field === "durationSeconds" ||
        field === "video_duration"
      )
        fail("metadata must not override " + field);
      rest[field] = clone(metadata[field]);
    }
    mergeMetadataScalar(source, metadata, "resolution");
    mergeMetadataScalar(source, metadata, "ratio");
    mergeMetadataScalar(source, metadata, "aspect_ratio");
    collectContent(metadata, prompt, target);
    if (Object.keys(rest).length > 0) metadataRest = rest;
  }

  const images = target.images.concat(collectMedia(source, "images", IMAGE_LIST_FIELDS));
  const videos = target.videos.concat(collectMedia(source, "videos", VIDEO_LIST_FIELDS));
  const audios = target.audios.concat(collectMedia(source, "audios", AUDIO_LIST_FIELDS));

  for (let index = 0; index < LAST_FRAME_FIELDS.length; index += 1) {
    const field = LAST_FRAME_FIELDS[index];
    if (source[field] !== undefined && source[field] !== null)
      fail("last frame is not supported by the V6K unified video API");
  }
  let firstFrame;
  for (let index = 0; index < FIRST_FRAME_FIELDS.length; index += 1) {
    const field = FIRST_FRAME_FIELDS[index];
    if (source[field] === undefined || source[field] === null) continue;
    const url = firstFrameURL(source[field], field);
    firstFrame = conflictURL(firstFrame, url, "first frame");
  }
  if (target.first) firstFrame = conflictURL(firstFrame, target.first, "first frame");

  const seconds = collectScalar(source, "seconds", SECONDS_FIELDS, parseSeconds);
  const ratio = collectScalar(source, "ratio", RATIO_FIELDS, parseRatio);
  const resolution = collectScalar(source, "resolution", RESOLUTION_FIELDS, parseResolution);

  let size;
  if (source.size !== undefined) size = parseSize(source.size, "size");
  if (source.quality !== undefined && typeof source.quality !== "string") fail("quality must be a string");

  const negativePrompt = trimmed(source.negative_prompt);
  if (negativePrompt) fail("negative_prompt is not supported by the V6K unified video API");

  if (source.n !== undefined) {
    const count = Number(source.n);
    if (count !== 1) fail("n must be 1");
  }

  const parsed = { prompt: prompt, images: images, videos: videos, audios: audios };
  if (seconds !== undefined) parsed.seconds = seconds;
  if (ratio !== undefined) parsed.ratio = ratio;
  if (resolution !== undefined) parsed.resolution = resolution;
  if (size !== undefined) parsed.size = size;
  if (firstFrame) parsed.input_reference = firstFrame;
  if (metadataRest) parsed.metadata = metadataRest;
  return parsed;
}

function upstreamBody(parsed, model) {
  const body = { model: model, prompt: parsed.prompt };
  if (parsed.seconds !== undefined) body.seconds = parsed.seconds;
  if (parsed.ratio !== undefined) body.ratio = parsed.ratio;
  if (parsed.resolution !== undefined) body.resolution = parsed.resolution;
  if (parsed.size !== undefined) body.size = parsed.size;
  if (parsed.images.length > 0) body.images = parsed.images.slice();
  if (parsed.videos.length > 0) body.videos = parsed.videos.slice();
  if (parsed.audios.length > 0) body.audios = parsed.audios.slice();
  if (parsed.input_reference !== undefined) body.input_reference = parsed.input_reference;
  if (parsed.metadata !== undefined) body.metadata = clone(parsed.metadata);
  return body;
}

function decodeFormField(value, field) {
  if (Array.isArray(value)) {
    if (value.length !== 1) fail("form field " + field + " must be submitted once");
    value = value[0];
  }
  if (typeof value !== "string") fail("form field " + field + " must be a string");
  return value;
}

function decodedBody(body) {
  if (!isObject(body)) fail("video request body is missing");
  if (body.kind === "json") {
    if (!isObject(body.value)) fail("video request body must be a JSON object");
    return clone(body.value);
  }
  if (body.kind !== "form" && body.kind !== "multipart") fail("unsupported video request body kind");
  const files = body.files;
  if (Array.isArray(files) && files.length > 0)
    fail("V6K requires public HTTP(S) media URLs; upload the media elsewhere and submit its public URL");
  if (!isObject(body.fields)) fail("video request fields are missing");
  const request = {};
  const fieldNames = Object.keys(body.fields);
  for (let index = 0; index < fieldNames.length; index += 1) {
    const field = fieldNames[index];
    const values = body.fields[field];
    if (!Array.isArray(values) || values.length === 0) fail("form field " + field + " is missing");
    if (METADATA_MEDIA_FIELDS.has(field)) {
      if (values.length === 1 && trimmed(values[0]).charAt(0) === "[") {
        try {
          request[field] = JSON.parse(values[0]);
        } catch (error) {
          fail("form field " + field + " must be valid JSON");
        }
      } else {
        request[field] = values;
      }
      continue;
    }
    const value = decodeFormField(values, field);
    if (NUMERIC_FORM_FIELDS.has(field)) {
      const number = Number(value);
      if (!Number.isFinite(number)) fail("form field " + field + " must be a number");
      request[field] = number;
      continue;
    }
    request[field] = value;
  }
  return request;
}

function upstreamTaskID(value, label) {
  const id = typeof value === "string" ? value.trim() : typeof value === "number" ? String(value) : "";
  if (!/^[A-Za-z0-9_.:-]{1,200}$/.test(id)) fail(label + " task id is invalid");
  return id;
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

function upstreamStatus(body) {
  return trimmed(body.status).toLowerCase();
}

function failureReason(data, ctx) {
  if (isObject(data.error)) {
    const message = trimmed(data.error.message) || trimmed(data.error.code);
    if (message) return sanitizeReason(message, ctx);
  }
  return sanitizeReason(data.message || data.reason, ctx);
}

export function buildSubmitRequest(ctx) {
  const model = trimmed(ctx && ctx.upstreamModel) || trimmed(ctx && ctx.model);
  if (!model) fail("upstream model is required");
  const apiKey = trimmed(ctx && ctx.apiKey);
  if (!apiKey) fail("channel API key is required");
  if (!ctx || typeof ctx.publicTaskId !== "string" || !ctx.publicTaskId.trim()) fail("public task id is required");
  const request = ctx.requestBody;
  const parsed = normalizeRequest(request, model);
  const body = upstreamBody(parsed, model);
  return {
    url: apiBase(ctx) + "/videos",
    method: "POST",
    headers: {
      Authorization: "Bearer " + apiKey,
      "Content-Type": "application/json",
      Accept: "application/json",
      "Idempotency-Key": ctx.publicTaskId,
    },
    body: body,
    noRetry: true,
  };
}

export function parseTaskResult(ctx, body) {
  if (!isObject(body)) return { status: "UNKNOWN" };
  const data = isObject(body.data) && body.data.status !== undefined ? body.data : body;
  if (!isObject(data)) return { status: "UNKNOWN" };
  if (data.error) return { status: "FAILURE", reason: failureReason(data, ctx) };
  const status = upstreamStatus(data);
  if (status === "completed" || status === "succeeded" || status === "success") {
    return { status: "SUCCESS", progress: "100%" };
  }
  if (status === "failed" || status === "failure" || status === "canceled" || status === "cancelled") {
    return { status: "FAILURE", reason: failureReason(data, ctx) };
  }
  if (status === "queued" || status === "pending" || status === "submitted") {
    return { status: "QUEUED" };
  }
  if (status === "in_progress" || status === "processing" || status === "running") {
    const result = { status: "IN_PROGRESS" };
    const progress = progressValue(data.progress);
    if (progress) result.progress = progress;
    return result;
  }
  return { status: "UNKNOWN" };
}

export function parseSubmitResponse(ctx, response) {
  if (!response || typeof response.statusCode !== "number" || response.statusCode < 200 || response.statusCode >= 300)
    fail("V6K submit request failed");
  const body = response.body;
  if (!isObject(body)) fail("V6K submit response must be a JSON object");
  if (body.error && body.id === undefined && body.task_id === undefined) {
    const message = isObject(body.error) ? body.error.message || body.error.code : body.error;
    fail(sanitizeReason(message, ctx));
  }
  const rawID = body.id !== undefined ? body.id : body.task_id;
  const id = upstreamTaskID(rawID, "V6K submit response");
  const result = parseTaskResult(ctx, body);
  if (result.status === "UNKNOWN") fail("V6K submit response did not include a usable task status");
  const submitted = {
    taskId: id,
    taskData: body,
    state: { model: trimmed(ctx && (ctx.upstreamModel || ctx.model)) },
  };
  if (result.status === "SUCCESS" || result.status === "FAILURE") submitted.immediate = result;
  return submitted;
}

export function buildQueryRequest(ctx) {
  const id = upstreamTaskID(ctx && ctx.taskId, "V6K query response");
  const apiKey = trimmed(ctx && ctx.apiKey);
  if (!apiKey) fail("channel API key is required");
  return {
    url: apiBase(ctx) + "/videos/" + encodeURIComponent(id),
    method: "GET",
    headers: {
      Authorization: "Bearer " + apiKey,
      Accept: "application/json",
    },
  };
}

export function extractUsage(ctx) {
  if (!ctx || ctx.usagePurpose === "billing_ratios") return {};
  const model = trimmed(ctx.upstreamModel) || trimmed(ctx.model);
  let parsed;
  try {
    parsed = normalizeRequest(ctx.requestBody, model);
  } catch (error) {
    return {};
  }
  const facts = {
    count: 1,
    // 只统计参考素材；首帧 input_reference 是生成条件，不计费。
    input_images: Math.min(parsed.images.length, MAX_INPUT_MEDIA),
    input_videos: Math.min(parsed.videos.length, MAX_INPUT_MEDIA),
    input_audios: Math.min(parsed.audios.length, MAX_INPUT_MEDIA),
  };
  if (parsed.seconds !== undefined) facts.seconds = parsed.seconds;
  const resolution =
    parsed.resolution !== undefined
      ? resolutionFact(parsed.resolution)
      : parsed.size !== undefined
        ? resolutionFactFromSize(parsed.size)
        : "";
  if (resolution) facts.video_resolution = resolution;
  return facts;
}

export function extractUsageOnComplete(_task, _result, _body) {
  return null;
}

export function listArtifacts(task) {
  if (!task || task.status !== "SUCCESS") return [];
  return [{ key: "video", type: "video", mimeType: "video/mp4" }];
}

export function buildContentRequest(ctx) {
  if (!ctx || ctx.artifactKey !== "video") fail("artifact_not_found");
  const rawID =
    ctx.upstreamTaskId !== undefined && ctx.upstreamTaskId !== null
      ? ctx.upstreamTaskId
      : isObject(ctx.data)
        ? ctx.data.id
        : undefined;
  const id = upstreamTaskID(rawID, "V6K content response");
  const apiKey = trimmed(ctx && ctx.apiKey);
  if (!apiKey) fail("channel API key is required");
  const method =
    ctx.clientRequest && typeof ctx.clientRequest.method === "string" && ctx.clientRequest.method
      ? ctx.clientRequest.method
      : "GET";
  return {
    url: apiBase(ctx) + "/videos/" + encodeURIComponent(id) + "/content",
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
      const parsed = normalizeRequest(request, model);
      const action =
        parsed.videos.length > 0 || parsed.audios.length > 0 || parsed.images.length > 1
          ? "reference_to_video"
          : parsed.input_reference || parsed.images.length > 0
            ? "image_to_video"
            : "text_to_video";
      const alias = trimmed(ctx && ctx.model) || model;
      const requestBody = Object.assign({ model: alias }, parsed);
      return { kind: "submit", model: alias, action: action, requestBody: requestBody };
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
        output.error = {
          code: "video_generation_failed",
          message: "Video generation failed",
        };
      }
      return output;
    },
  },
};
