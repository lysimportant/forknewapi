/**
 * 灵动（lingdongapi.com）统一视频异步任务协议适配（宿主 openai_video 协议）。
 *
 * 插件把宿主的 OpenAI 视频入口翻译成灵动平台公开的视频任务接口：
 *   POST /v1/video/generations
 *   GET  /v1/video/generations/{task_id}
 *   GET  /v1/videos/{task_id}/content
 *
 * 平台把多个厂商的视频模型（CVK、MiniMax H3、SD 等）统一为同一套参数：
 * prompt、images（参考图，按顺序对应 @图片1、@图片2）、videos、audios、ratio、
 * duration（秒）、resolution 与 watermark。插件入口同时接受宿主 OpenAI 视频字段
 * （seconds/size/input_reference）与 Canvas 传入的 metadata.content，并翻译成上述
 * 平台字段。平台没有尾帧与模型专属参数入口：首帧按参考图顺序提交，尾帧与无法用
 * 统一字段表达的参数直接报错，而不是静默丢弃。
 * 参考素材必须是上游可访问的公网 HTTP(S) 直链，本地文件上传无法提交给该接口。
 *
 * 模型不在插件里写死：渠道使用 modelRouting "channel"，管理员在类型 61 渠道里读取
 * 上游 /v1/models 目录后自行勾选视频模型。售价由管理员在本站配置，插件报告计费用量
 * 事实：生成次数（count）、生成时长（seconds）、清晰度、参考素材数量
 * （input_images/input_videos/input_audios，首帧不计入参考图片）与参考视频秒数
 * （input_video_seconds）。平台按模型区分按次与按秒计费，由管理员按模型选择计费
 * 表达式；上游查询响应不报告用量数字，完成钩子不覆盖提交时上报的事实。
 */

export const meta = {
  apiVersion: 1,
  key: "lingdong-video",
  name: "灵动视频",
  description: {
    en: "Lingdong unified video generation API",
    zh: "灵动统一视频生成接口",
  },
  version: "1.0.0",
  author: { name: "lysimportant/forknewapi" },
  website: "https://www.lingdongapi.com/docs/api/",
  baseUrl: "https://www.lingdongapi.com",
  allowedHosts: ["www.lingdongapi.com:443", "lingdongapi.com:443"],
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
    input_video_seconds: {
      type: "number",
      unit: "second",
      description: { en: "Input video unit price", zh: "输入视频单价" },
    },
    video_resolution: {
      enum: ["480p", "720p", "1080p"],
      description: { en: "Output video resolution", zh: "输出视频分辨率" },
    },
  },
  usageExamples: [
    {
      label: "480p · 5s",
      facts: {
        count: 1,
        seconds: 5,
        input_images: 0,
        input_videos: 0,
        input_audios: 0,
        input_video_seconds: 0,
        video_resolution: "480p",
      },
    },
    {
      label: "720p · 5s",
      facts: {
        count: 1,
        seconds: 5,
        input_images: 0,
        input_videos: 0,
        input_audios: 0,
        input_video_seconds: 0,
        video_resolution: "720p",
      },
    },
    {
      label: "1080p · 5s",
      facts: {
        count: 1,
        seconds: 5,
        input_images: 0,
        input_videos: 0,
        input_audios: 0,
        input_video_seconds: 0,
        video_resolution: "1080p",
      },
    },
  ],
};

/** 与宿主 MaxTaskDurationSeconds 保持一致的安全上限，不代表模型支持的时长。 */
const MIN_SECONDS = 1;
const MAX_SECONDS = 3600;
/** 参考素材数量的安全上限，避免异常请求把计费事实放大到无意义的量级。 */
const MAX_INPUT_MEDIA = 128;
/** 计费用量事实只使用站点模型目录公开的三档清晰度；其余写法不写用量事实。 */
const RESOLUTIONS = ["480p", "720p", "1080p"];

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
  "watermark",
  "n",
  "image",
  "input_reference",
  "first_frame",
  "first_frame_url",
  "start_frame",
  "last_frame",
  "last_frame_url",
  "end_frame",
  "images",
  "image_urls",
  "reference_images",
  "referenceImages",
  "videos",
  "video_urls",
  "reference_videos",
  "referenceVideos",
  "audios",
  "audio_urls",
  "reference_audios",
  "referenceAudios",
  "metadata",
]);

const FIRST_FRAME_FIELDS = ["image", "input_reference", "first_frame", "first_frame_url", "start_frame"];
const LAST_FRAME_FIELDS = ["last_frame", "last_frame_url", "end_frame"];
const IMAGE_LIST_FIELDS = ["images", "image_urls", "reference_images", "referenceImages"];
const VIDEO_LIST_FIELDS = ["videos", "video_urls", "reference_videos", "referenceVideos"];
const AUDIO_LIST_FIELDS = ["audios", "audio_urls", "reference_audios", "referenceAudios"];
const SECONDS_FIELDS = ["seconds", "duration", "duration_seconds", "durationSeconds", "video_duration"];
const RATIO_FIELDS = ["ratio", "aspect_ratio"];
const RESOLUTION_FIELDS = ["resolution", "video_resolution", "videoResolution"];

const METADATA_MEDIA_FIELDS = new Set(
  IMAGE_LIST_FIELDS.concat(VIDEO_LIST_FIELDS, AUDIO_LIST_FIELDS, ["metadata"]),
);
const NUMERIC_FORM_FIELDS = new Set(SECONDS_FIELDS.concat(["n"]));

function fail(message) {
  throw new Error(message);
}

function trimmed(value) {
  return typeof value === "string" ? value.trim() : "";
}

function isObject(value) {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function clone(value) {
  return JSON.parse(JSON.stringify(value));
}

/** 渠道 Base URL 统一到平台 /v1 前缀；只允许 HTTPS，不拼接凭据。 */
function apiBase(ctx) {
  const raw = trimmed((ctx && ctx.baseUrl) || meta.baseUrl);
  if (!raw) fail("channel base URL is required");
  const base = raw.replace(/\/+$/, "");
  if (!/^https:\/\/[^/?#@\s\\]+(?:\/[^?#\s\\]*)?$/.test(base)) fail("channel base URL must be an HTTPS URL");
  return base.replace(/\/v1$/, "") + "/v1";
}

/** 参考素材必须是上游可访问的公网 HTTP(S) 直链；拒绝本机与内网地址。 */
function mediaURL(value, field) {
  if (isObject(value)) {
    if (value.url === undefined) fail(field + " must be a public HTTP(S) URL");
    return mediaURL(value.url, field);
  }
  if (typeof value !== "string") fail(field + " must be a public HTTP(S) URL");
  const url = value.trim();
  const match = /^(https?):\/\/([A-Za-z0-9.-]+(?::[0-9]{1,5})?)(?:[/?#]|$)/i.exec(url);
  if (!match) fail(field + " must be a public HTTP(S) URL");
  const host = match[2].toLowerCase().split(":")[0];
  if (
    host === "localhost" ||
    host === "0.0.0.0" ||
    host.indexOf(".") < 0 ||
    host.endsWith(".localhost") ||
    host.endsWith(".local") ||
    host.startsWith("127.") ||
    host.startsWith("10.") ||
    host.startsWith("192.168.") ||
    host.startsWith("169.254.") ||
    /^172\.(1[6-9]|2[0-9]|3[0-1])\./.test(host)
  )
    fail(field + " must be a public HTTP(S) URL");
  return url;
}

function conflictURL(current, next, label) {
  if (current && current !== next) fail(label + " conflicts with another reference");
  return next;
}

function mediaItems(value, field) {
  if (value === undefined || value === null) return [];
  let list = value;
  if (typeof value === "string") {
    const text = value.trim();
    if (!text) return [];
    if (text.charAt(0) === "[") {
      try {
        list = JSON.parse(text);
      } catch (error) {
        fail(field + " must be a JSON array of media URLs");
      }
    } else {
      list = [text];
    }
  }
  if (!Array.isArray(list)) fail(field + " must be an array of media URLs");
  return list;
}

function mediaItem(value, field) {
  if (typeof value === "string") return { url: mediaURL(value, field) };
  if (!isObject(value)) fail(field + " must contain media URL objects");
  const keys = Object.keys(value);
  for (let index = 0; index < keys.length; index += 1) {
    if (keys[index] !== "url" && keys[index] !== "duration_seconds")
      fail("unsupported " + field + " item field: " + keys[index]);
  }
  const item = { url: mediaURL(value.url, field) };
  if (value.duration_seconds !== undefined && value.duration_seconds !== null) {
    item.duration = parseSeconds(value.duration_seconds, field + ".duration_seconds");
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

function parseWatermark(value, field) {
  if (typeof value === "boolean") return value;
  if (value === "true") return true;
  if (value === "false") return false;
  fail(field + " must be a boolean");
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

/** OpenAIVideo 的 size 没有档位概念，只能按短边归一到平台目录里的清晰度档。 */
function sizeResolution(size) {
  const shortEdge = Math.min(size.width, size.height);
  if (shortEdge >= 1000) return "1080p";
  if (shortEdge >= 640) return "720p";
  if (shortEdge >= 400) return "480p";
  return "";
}

/** 把上游清晰度写法归一到计费枚举；无法识别时返回空串（不写用量事实）。 */
function resolutionFact(value) {
  const text = trimmed(value).toLowerCase();
  if (RESOLUTIONS.indexOf(text) >= 0) return text;
  if (text === "1080" || text === "fhd") return "1080p";
  if (text === "720" || text === "hd") return "720p";
  if (text === "480" || text === "sd") return "480p";
  return "";
}

function mergeMetadataScalar(source, metadata, field) {
  if (metadata[field] === undefined) return;
  if (source[field] !== undefined && JSON.stringify(source[field]) !== JSON.stringify(metadata[field]))
    fail("metadata." + field + " conflicts with " + field);
  source[field] = metadata[field];
}

/**
 * Canvas 以 metadata.content 传递按编号引用的素材；平台按 images/videos/audios
 * 的顺序对应 @图片N、@视频N、@音频N，因此按类型保持提交顺序即可。
 */
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
    if (type !== "image_url" && type !== "video_url" && type !== "audio_url")
      fail("unsupported metadata.content item type: " + String(type));
    const keys = Object.keys(item);
    for (let keyIndex = 0; keyIndex < keys.length; keyIndex += 1) {
      const key = keys[keyIndex];
      if (key !== "type" && key !== "role" && key !== type) fail("unsupported metadata.content item field: " + key);
    }
    const url = mediaURL(item[type], "metadata.content." + type);
    const role = item.role === undefined || item.role === null ? "" : trimmed(item.role);
    if (type === "image_url") {
      if (role === "first_frame") target.firstFrame = conflictURL(target.firstFrame, url, "first frame");
      else if (role === "last_frame") fail("last frame is not supported by the Lingdong unified video API");
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
 * 把宿主入口请求归一成平台统一参数；返回的 body 是驱动钩子与计费事实的唯一来源，
 * usage 汇总可计费的参考素材数量与输入视频秒数（首帧不计入参考素材）。
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

  const metadata = source.metadata;
  if (metadata !== undefined) {
    if (!isObject(metadata)) fail("metadata must be an object");
    const metadataFields = Object.keys(metadata);
    for (let index = 0; index < metadataFields.length; index += 1) {
      const field = metadataFields[index];
      if (field === "content" || field === "resolution" || field === "ratio" || field === "aspect_ratio" || field === "watermark")
        continue;
      fail("unsupported metadata field: " + field);
    }
    mergeMetadataScalar(source, metadata, "resolution");
    mergeMetadataScalar(source, metadata, "ratio");
    mergeMetadataScalar(source, metadata, "aspect_ratio");
    mergeMetadataScalar(source, metadata, "watermark");
    collectContent(metadata, prompt, target);
  }

  const imageItems = collectMedia(source, "images", IMAGE_LIST_FIELDS);
  const videoItems = collectMedia(source, "videos", VIDEO_LIST_FIELDS);
  const audioItems = collectMedia(source, "audios", AUDIO_LIST_FIELDS);
  const images = target.images.slice();
  for (let index = 0; index < imageItems.length; index += 1) images.push(imageItems[index].url);
  const videos = target.videos.slice();
  for (let index = 0; index < videoItems.length; index += 1) videos.push(videoItems[index].url);
  const audios = target.audios.slice();
  for (let index = 0; index < audioItems.length; index += 1) audios.push(audioItems[index].url);
  let inputVideoSeconds = 0;
  for (let index = 0; index < videoItems.length; index += 1) {
    if (videoItems[index].duration !== undefined) inputVideoSeconds += videoItems[index].duration;
  }
  inputVideoSeconds = Math.min(inputVideoSeconds, MAX_SECONDS);

  for (let index = 0; index < LAST_FRAME_FIELDS.length; index += 1) {
    if (source[LAST_FRAME_FIELDS[index]] !== undefined && source[LAST_FRAME_FIELDS[index]] !== null)
      fail("last frame is not supported by the Lingdong unified video API");
  }

  let firstFrame = target.firstFrame;
  for (let index = 0; index < FIRST_FRAME_FIELDS.length; index += 1) {
    const field = FIRST_FRAME_FIELDS[index];
    if (source[field] === undefined || source[field] === null) continue;
    firstFrame = conflictURL(firstFrame, firstFrameURL(source[field], field), "first frame");
  }

  const duration = collectScalar(source, "seconds", SECONDS_FIELDS, parseSeconds);
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

  let watermark;
  if (source.watermark !== undefined && source.watermark !== null) watermark = parseWatermark(source.watermark, "watermark");

  if (source.n !== undefined && source.n !== null) {
    const count = Number(source.n);
    if (count !== 1) fail("n must be 1");
  }

  const body = { prompt: prompt, images: images, videos: videos, audios: audios };
  if (firstFrame) body.input_reference = firstFrame;
  if (duration !== undefined) body.duration = duration;
  if (ratio !== undefined) body.ratio = ratio;
  if (resolution !== undefined) body.resolution = resolution;
  if (watermark !== undefined) body.watermark = watermark;

  const usage = {
    input_images: Math.min(images.length, MAX_INPUT_MEDIA),
    input_videos: Math.min(videos.length, MAX_INPUT_MEDIA),
    input_audios: Math.min(audios.length, MAX_INPUT_MEDIA),
    input_video_seconds: inputVideoSeconds,
  };
  return { body: body, usage: usage };
}

/** 平台请求体：首帧作为第一张参考图提交，其余参考素材保持提交顺序。 */
function upstreamBody(parsed, model) {
  const body = { model: model, prompt: parsed.prompt };
  const images = parsed.input_reference ? [parsed.input_reference].concat(parsed.images) : parsed.images.slice();
  if (images.length > 0) body.images = images;
  if (parsed.videos.length > 0) body.videos = parsed.videos.slice();
  if (parsed.audios.length > 0) body.audios = parsed.audios.slice();
  if (parsed.duration !== undefined) body.duration = parsed.duration;
  if (parsed.ratio !== undefined) body.ratio = parsed.ratio;
  if (parsed.resolution !== undefined) body.resolution = parsed.resolution;
  if (parsed.watermark !== undefined) body.watermark = parsed.watermark;
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
    fail("Lingdong requires public HTTP(S) media URLs; upload the media elsewhere and submit its public URL");
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
    const raw = decodeFormField(values, field);
    if (NUMERIC_FORM_FIELDS.has(field)) {
      const number = Number(raw.trim());
      if (!Number.isFinite(number)) fail("form field " + field + " must be a number");
      request[field] = number;
      continue;
    }
    request[field] = raw;
  }
  return request;
}

function upstreamTaskID(value, label) {
  const id = trimmed(value);
  if (!/^[A-Za-z0-9_.:-]{1,200}$/.test(id)) fail(label + " task id is invalid");
  return id;
}

function envelopeData(body) {
  if (!isObject(body)) return body;
  if (isObject(body.data) && body.id === undefined && body.task_id === undefined) return body.data;
  return body;
}

function progressValue(value) {
  if (typeof value === "number") {
    if (!Number.isFinite(value) || value < 0 || value > 100) return "";
    return Math.round(value) + "%";
  }
  const text = trimmed(value);
  if (/^[0-9]{1,3}(?:\.[0-9]+)?%$/.test(text)) return text;
  return "";
}

/** 失败原因展示给用户前去掉渠道密钥与鉴权头，并截断异常长文本。 */
function sanitizeReason(value, ctx) {
  let text = "";
  if (typeof value === "string") text = value.trim();
  else if (typeof value === "number") text = String(value);
  if (text) {
    const apiKey = trimmed(ctx && ctx.apiKey);
    if (apiKey) text = text.split(apiKey).join("[redacted]");
    text = text.replace(/Bearer\s+\S+/gi, "Bearer [redacted]").replace(/sk-[A-Za-z0-9_-]+/g, "sk-[redacted]");
  }
  if (text.length > 500) text = text.slice(0, 500);
  return text || "Video generation failed";
}

function failureReason(data, ctx) {
  const error = data.error;
  if (isObject(error)) return sanitizeReason(error.message || error.code, ctx);
  if (error !== undefined) return sanitizeReason(error, ctx);
  return sanitizeReason(data.fail_reason || data.message || data.reason, ctx);
}

function submitHeaders(ctx, apiKey) {
  return {
    Authorization: "Bearer " + apiKey,
    "Content-Type": "application/json",
    Accept: "application/json",
    "X-Idempotency-Key": ctx.publicTaskId.trim(),
  };
}

export function buildSubmitRequest(ctx) {
  const model = trimmed(ctx && ctx.upstreamModel) || trimmed(ctx && ctx.model);
  if (!model) fail("upstream model is required");
  const apiKey = trimmed(ctx && ctx.apiKey);
  if (!apiKey) fail("channel API key is required");
  if (!ctx || typeof ctx.publicTaskId !== "string" || !ctx.publicTaskId.trim()) fail("public task id is required");
  const parsed = normalizeRequest(ctx.requestBody);
  return {
    url: apiBase(ctx) + "/video/generations",
    method: "POST",
    headers: submitHeaders(ctx, apiKey),
    body: upstreamBody(parsed.body, model),
    noRetry: true,
  };
}

export function parseTaskResult(ctx, body) {
  if (!isObject(body)) return { status: "UNKNOWN" };
  const data = isObject(body.data) && body.data.status !== undefined ? body.data : body;
  if (!isObject(data)) return { status: "UNKNOWN" };
  if (data.error) return { status: "FAILURE", reason: failureReason(data, ctx) };
  const status = trimmed(data.status).toLowerCase();
  if (status === "completed" || status === "succeeded" || status === "success")
    return { status: "SUCCESS", progress: "100%" };
  if (status === "failed" || status === "failure" || status === "canceled" || status === "cancelled")
    return { status: "FAILURE", reason: failureReason(data, ctx) };
  if (status === "queued" || status === "pending" || status === "submitted" || status === "not_start")
    return { status: "QUEUED" };
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
    fail("Lingdong video submission failed");
  const body = response.body;
  if (!isObject(body)) fail("Lingdong submit response must be a JSON object");
  const data = envelopeData(body);
  if (isObject(body.error) && body.id === undefined && body.task_id === undefined)
    fail(sanitizeReason(body.error.message || body.error.code, ctx));
  if (isObject(data.error) && data.id === undefined && data.task_id === undefined)
    fail(sanitizeReason(data.error.message || data.error.code, ctx));
  const rawID = data.id !== undefined ? data.id : data.task_id;
  const submitted = {
    taskId: upstreamTaskID(rawID, "Lingdong submit response"),
    taskData: data,
    state: { model: trimmed(ctx && (ctx.upstreamModel || ctx.model)) },
  };
  const result = parseTaskResult(ctx, data);
  const status = result.status;
  if (status === "SUCCESS" || status === "FAILURE") submitted.immediate = result;
  return submitted;
}

export function buildQueryRequest(ctx) {
  const id = upstreamTaskID(ctx && ctx.taskId, "Lingdong query response");
  const apiKey = trimmed(ctx && ctx.apiKey);
  if (!apiKey) fail("channel API key is required");
  return {
    url: apiBase(ctx) + "/video/generations/" + encodeURIComponent(id),
    method: "GET",
    headers: {
      Authorization: "Bearer " + apiKey,
      Accept: "application/json",
    },
  };
}

export function extractUsage(ctx) {
  if (!ctx || ctx.usagePurpose === "billing_ratios") return {};
  let parsed;
  try {
    parsed = normalizeRequest(ctx.requestBody);
  } catch (error) {
    return {};
  }
  const facts = {
    count: 1,
    // 只统计参考素材；首帧 input_reference 是生成条件，不计费。
    input_images: parsed.usage.input_images,
    input_videos: parsed.usage.input_videos,
    input_audios: parsed.usage.input_audios,
    input_video_seconds: parsed.usage.input_video_seconds,
  };
  if (parsed.body.duration !== undefined) facts.seconds = parsed.body.duration;
  const resolution = parsed.body.resolution !== undefined ? resolutionFact(parsed.body.resolution) : "";
  if (resolution) facts.video_resolution = resolution;
  return facts;
}

/** 平台查询响应不报告生成或参考素材的用量数字，保留提交时上报的事实。 */
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
        ? ctx.data.id !== undefined
          ? ctx.data.id
          : ctx.data.task_id
        : undefined;
  const id = upstreamTaskID(rawID, "Lingdong content response");
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
      const model = trimmed(ctx && ctx.model) || trimmed(request && request.model);
      if (!model) fail("model is required");
      const parsed = normalizeRequest(request);
      const action =
        parsed.body.videos.length > 0 || parsed.body.audios.length > 0 || parsed.body.images.length > 1
          ? "reference_to_video"
          : parsed.body.input_reference || parsed.body.images.length > 0
            ? "image_to_video"
            : "text_to_video";
      const alias = trimmed(ctx && ctx.model) || model;
      const requestBody = Object.assign({ model: alias }, parsed.body);
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
      const status = statuses[task && task.status] || "unknown";
      const output = {
        id: task && task.task_id,
        object: "video",
        model: (task && task.properties && task.properties.origin_model_name) || meta.key,
        status: status,
      };
      if (status === "failed") output.error = { code: "video_generation_failed", message: "Video generation failed" };
      return output;
    },
  },
};
