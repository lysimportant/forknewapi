/**
 * NSKeven 统一视频 API 协议适配（宿主 openai_video 协议）。
 *
 * 本插件把宿主的 OpenAI 视频入口翻译成 NSKeven 的统一视频接口：
 *   POST /v1/video/generations
 *   GET  /v1/video/generations/{task_id}
 *   GET  /v1/videos/{task_id}/content
 *
 * 模型不在插件里写死：渠道使用 modelRouting "channel"，管理员在类型 61 渠道里读取
 * 上游 /v1/models 目录后自行勾选任意视频模型；运行时按上游模型名识别参数家族
 * （豆包 metadata 格式、快线 4.3 格式、其余扁平格式）。
 */

export const meta = {
  apiVersion: 1,
  key: "nskeven-video",
  name: "NSKeven 视频",
  description: {
    en: "NSKeven unified video generation gateway",
    zh: "NSKeven 统一视频生成网关",
  },
  version: "1.1.0",
  author: { name: "lysimportant/forknewapi" },
  website: "https://docs.nskeven.uk/",
  baseUrl: "https://newapi.nskeven.uk",
  allowedHosts: ["newapi.nskeven.uk:443"],
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
      unitLabel: { en: "video", zh: "个" },
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
    { label: "480p · 5s", facts: { count: 1, seconds: 5, input_images: 0, input_videos: 0, input_audios: 0, video_resolution: "480p" } },
    { label: "720p · 5s", facts: { count: 1, seconds: 5, input_images: 0, input_videos: 0, input_audios: 0, video_resolution: "720p" } },
    { label: "1080p · 5s", facts: { count: 1, seconds: 5, input_images: 0, input_videos: 0, input_audios: 0, video_resolution: "1080p" } },
    { label: "4k · 5s", facts: { count: 1, seconds: 5, input_images: 0, input_videos: 0, input_audios: 0, video_resolution: "4k" } },
    { label: "720p · 5s · 参考素材", facts: { count: 1, seconds: 5, input_images: 1, input_videos: 1, input_audios: 1, video_resolution: "720p" } },
  ],
};

const MAX_SECONDS = 3600;
const MIN_SECONDS = 1;
const DEFAULT_SECONDS = 5;
const RESOLUTIONS = ["480p", "720p", "1080p", "4k"];
const ASPECT_RATIOS = ["16:9", "9:16", "1:1", "4:3", "3:4", "21:9"];
/** 参考素材按件计费，宿主对 count 用量事实的上限为 128。 */
const MAX_INPUT_MEDIA = 128;

const REQUEST_FIELDS = new Set([
  "model",
  "prompt",
  "input",
  "duration",
  "seconds",
  "durationSeconds",
  "resolution",
  "video_resolution",
  "videoResolution",
  "size",
  "quality",
  "aspect_ratio",
  "ratio",
  "aspectRatio",
  "image",
  "input_reference",
  "image_urls",
  "images",
  "reference_images",
  "referenceImages",
  "videos",
  "reference_videos",
  "referenceVideos",
  "audios",
  "reference_audios",
  "referenceAudios",
  "first_frame",
  "first_frame_url",
  "start_frame",
  "last_frame",
  "last_frame_url",
  "end_frame",
  "generate_audio",
  "watermark",
  "seed",
  "negative_prompt",
  "n",
  "metadata",
]);

const METADATA_FIELDS = new Set([
  "content",
  "resolution",
  "ratio",
  "aspect_ratio",
  "videos",
  "audios",
  "generate_audio",
  "watermark",
  "seed",
]);

const FIRST_FRAME_FIELDS = ["image", "input_reference", "first_frame", "first_frame_url", "start_frame"];
const LAST_FRAME_FIELDS = ["last_frame", "last_frame_url", "end_frame"];
const IMAGE_LIST_FIELDS = ["images", "image_urls", "reference_images", "referenceImages"];
const VIDEO_LIST_FIELDS = ["videos", "reference_videos", "referenceVideos"];
const AUDIO_LIST_FIELDS = ["audios", "reference_audios", "referenceAudios"];

const JSON_FORM_FIELDS = new Set([
  "metadata",
  "images",
  "image_urls",
  "reference_images",
  "referenceImages",
  "videos",
  "reference_videos",
  "referenceVideos",
  "audios",
  "reference_audios",
  "referenceAudios",
  "first_frame",
  "last_frame",
]);
const BOOLEAN_FORM_FIELDS = new Set(["generate_audio", "watermark"]);

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
  if (/^asset:\/\/ast-[A-Za-z0-9_-]+$/.test(url)) return true;
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
  fail(field + " must be a public HTTP(S) URL or a platform asset URL");
}

function singleMediaURL(value, field) {
  if (Array.isArray(value)) fail(field + " must be a single media URL");
  return mediaURL(value, field);
}

function conflictURL(current, url, label) {
  if (current && current !== url) fail("conflicting " + label + " references");
  return url;
}

function referenceItems(value, field) {
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
      return referenceItems(parsed, field);
    }
    return [text];
  }
  if (Array.isArray(value)) return value;
  return [value];
}

function collectMediaURLs(value, field, target) {
  const items = referenceItems(value, field);
  for (let index = 0; index < items.length; index += 1) {
    target.push(mediaURL(items[index], field));
  }
}

function collectImages(value, field, target) {
  const items = referenceItems(value, field);
  for (let index = 0; index < items.length; index += 1) {
    const item = items[index];
    if (typeof item === "string") {
      target.images.push(mediaURL(item, field));
      continue;
    }
    if (!isObject(item)) fail(field + " must contain media URLs");
    const keys = Object.keys(item);
    for (let keyIndex = 0; keyIndex < keys.length; keyIndex += 1) {
      const key = keys[keyIndex];
      if (key !== "url" && key !== "role") fail("unsupported " + field + " item field: " + key);
    }
    const url = mediaURL(item.url, field);
    const role = item.role === undefined || item.role === null ? "reference_image" : item.role;
    if (role === "reference_image") target.images.push(url);
    else if (role === "first_frame") target.first = conflictURL(target.first, url, "first frame");
    else if (role === "last_frame") target.last = conflictURL(target.last, url, "last frame");
    else fail("unsupported " + field + " role: " + String(role));
  }
}

function parseDuration(value) {
  let text = value;
  if (typeof text === "number") {
    if (!Number.isInteger(text)) fail("seconds must be a whole number between 1 and 3600");
  } else if (typeof text === "string") {
    text = text.trim();
    const match = /^([0-9]+)(?:s|S|秒)?$/.exec(text);
    if (!match) fail("seconds must be a whole number between 1 and 3600");
    text = match[1];
  } else {
    fail("seconds must be a whole number between 1 and 3600");
  }
  const seconds = Number(text);
  if (!Number.isInteger(seconds) || seconds < MIN_SECONDS || seconds > MAX_SECONDS)
    fail("seconds must be a whole number between 1 and 3600");
  return seconds;
}

function resolutionFromSize(value) {
  const text = trimmed(value).toLowerCase();
  const match = /^([0-9]{3,4})[x*]([0-9]{3,4})$/.exec(text);
  if (!match) return "";
  const height = Math.min(Number(match[1]), Number(match[2]));
  if (height >= 2160) return "4k";
  if (height >= 1080) return "1080p";
  if (height >= 720) return "720p";
  if (height >= 480) return "480p";
  return "";
}

function isDoubao(model) {
  return /^doubao-seedance/i.test(model);
}

function isExpress(model) {
  return /^sd-2-/i.test(model) || /^sd-2\.5-/i.test(model) || /^wan-3\.0-/i.test(model);
}

function isASeries(model) {
  return /^even-sd2\.[05]-a/i.test(model);
}

function resolutionRuleOf(model) {
  const lower = trimmed(model).toLowerCase();
  if (/^video-ultra-720p/.test(lower) || /^video-ultra2\.5-720p/.test(lower)) return { fixed: "720p" };
  if (/^even--seedance-2\.0-480p$/.test(lower)) return { fixed: "480p" };
  if (/^even--seedance-2\.0-720p$/.test(lower)) return { fixed: "720p" };
  if (/^even--seedance-2\.0-1080p$/.test(lower)) return { fixed: "1080p" };
  if (/^even--seedance-2\.0-4k$/.test(lower)) return { fixed: "4k" };
  if (/^sd-2-(e4|e5|e6|e8)$/.test(lower) || /^sd-2\.5-e1$/.test(lower)) return { fixed: "720p" };
  if (/^doubao-seedance-2-5/.test(lower)) return { allowed: ["480p", "720p", "1080p"], fallback: "720p" };
  if (/^doubao-seedance-2-0-(fast|mini)/.test(lower)) return { allowed: ["480p", "720p"], fallback: "720p" };
  if (/^doubao-seedance-2-0/.test(lower)) return { allowed: ["480p", "720p", "1080p", "4k"], fallback: "720p" };
  if (/^even-sd2\.0-a$/i.test(trimmed(model))) return { allowed: ["480p", "720p"], fallback: "480p" };
  if (/^even-sd2\.5-a$/i.test(trimmed(model))) return { allowed: ["480p", "720p", "1080p"], fallback: "480p" };
  return null;
}

function durationRuleOf(model) {
  const lower = trimmed(model).toLowerCase();
  if (/^doubao-seedance-2-5/.test(lower)) return { min: 4, max: 30, fallback: 4 };
  if (/^doubao-seedance/.test(lower)) return { min: 4, max: 15, fallback: 4 };
  if (/^video-ultra2\.5/.test(lower)) return { min: 4, max: 30, fallback: 5 };
  if (/^video-ultra-720p/.test(lower)) return { min: 4, max: 15, fallback: 5 };
  if (/^even--seedance-2\.0-4k$/.test(lower)) return null;
  if (/^even--seedance/.test(lower)) return { min: 4, max: 15, fallback: 5 };
  if (/^sd-2-(e4|e5|e6)$/.test(lower)) return { min: 4, max: 15, fallback: 5 };
  return null;
}

function resolutionFor(model, resolution) {
  const rule = resolutionRuleOf(model);
  if (rule && rule.fixed) {
    if (resolution && resolution !== rule.fixed)
      fail("model " + model + " only supports resolution " + rule.fixed);
    return rule.fixed;
  }
  let value = resolution || "";
  if (!value && rule) value = rule.fallback;
  if (rule && value && rule.allowed.indexOf(value) < 0)
    fail("model " + model + " supports resolution " + rule.allowed.join("/") + " only");
  return value;
}

function translateReferenceTokens(prompt) {
  let result = prompt;
  result = result.replace(/\bImage\s+([0-9]+)/gi, "@图$1");
  result = result.replace(/\bVideo\s+([0-9]+)/gi, "@视频$1");
  result = result.replace(/\bAudio\s+([0-9]+)/gi, "@音频$1");
  result = result.replace(/@?图\s*([0-9]+)/g, "@图$1");
  result = result.replace(/@?视频\s*([0-9]+)/g, "@视频$1");
  result = result.replace(/@?音频\s*([0-9]+)/g, "@音频$1");
  return result;
}

function normalizeRequest(request, model) {
  if (!isObject(request)) fail("video request body must be a JSON object");
  const upstreamModel = trimmed(model);
  if (!upstreamModel) fail("model is required");
  const fields = Object.keys(request);
  for (let index = 0; index < fields.length; index += 1) {
    if (!REQUEST_FIELDS.has(fields[index])) fail("unsupported video request field: " + fields[index]);
  }
  const metadata = request.metadata === undefined || request.metadata === null ? {} : request.metadata;
  if (!isObject(metadata)) fail("metadata must be a JSON object");
  const metadataFields = Object.keys(metadata);
  for (let index = 0; index < metadataFields.length; index += 1) {
    if (!METADATA_FIELDS.has(metadataFields[index])) fail("unsupported video metadata field: " + metadataFields[index]);
  }

  const target = { images: [], first: "", last: "" };
  const contentVideos = [];
  const contentAudios = [];
  const contentTexts = [];
  if (metadata.content !== undefined) {
    if (!Array.isArray(metadata.content)) fail("metadata.content must be an array");
    for (let index = 0; index < metadata.content.length; index += 1) {
      const item = metadata.content[index];
      if (!isObject(item)) fail("metadata.content items must be JSON objects");
      if (item.type === "text" || item.type === "input_text") {
        if (typeof item.text !== "string") fail("metadata.content text items must contain text");
        contentTexts.push(item.text);
        continue;
      }
      const type = item.type;
      if (type !== "image_url" && type !== "input_image" && type !== "video_url" && type !== "audio_url")
        fail("unsupported metadata.content item type: " + String(type));
      const urlField = type === "input_image" ? "image_url" : type;
      const itemKeys = Object.keys(item);
      for (let keyIndex = 0; keyIndex < itemKeys.length; keyIndex += 1) {
        const key = itemKeys[keyIndex];
        if (key !== "type" && key !== "role" && key !== urlField)
          fail("unsupported metadata.content item field: " + key);
      }
      const url = mediaURL(item[urlField], "metadata.content " + type);
      if (type === "image_url" || type === "input_image") {
        const role = item.role === undefined || item.role === null ? "reference_image" : item.role;
        if (role === "reference_image") target.images.push(url);
        else if (role === "first_frame") target.first = conflictURL(target.first, url, "first frame");
        else if (role === "last_frame") target.last = conflictURL(target.last, url, "last frame");
        else fail("unsupported metadata.content image role: " + String(role));
      } else if (type === "video_url") {
        if (item.role !== undefined && item.role !== null && item.role !== "reference_video" && item.role !== "content")
          fail("unsupported metadata.content video role: " + String(item.role));
        contentVideos.push(url);
      } else {
        if (item.role !== undefined && item.role !== null && item.role !== "reference_audio" && item.role !== "audioTrack")
          fail("unsupported metadata.content audio role: " + String(item.role));
        contentAudios.push(url);
      }
    }
  }
  if (metadata.videos !== undefined) collectMediaURLs(metadata.videos, "metadata.videos", contentVideos);
  if (metadata.audios !== undefined) collectMediaURLs(metadata.audios, "metadata.audios", contentAudios);

  const topLevelMediaFields = FIRST_FRAME_FIELDS.concat(LAST_FRAME_FIELDS)
    .concat(IMAGE_LIST_FIELDS)
    .concat(VIDEO_LIST_FIELDS)
    .concat(AUDIO_LIST_FIELDS);
  let hasTopLevelMedia = false;
  for (let index = 0; index < topLevelMediaFields.length; index += 1) {
    if (request[topLevelMediaFields[index]] !== undefined && request[topLevelMediaFields[index]] !== null) {
      hasTopLevelMedia = true;
      break;
    }
  }
  const hasContentMedia =
    target.images.length > 0 ||
    target.first !== "" ||
    target.last !== "" ||
    contentVideos.length > 0 ||
    contentAudios.length > 0;
  if (hasContentMedia && hasTopLevelMedia)
    fail("reference media must be provided either in metadata or in top-level fields, not both");

  for (let index = 0; index < IMAGE_LIST_FIELDS.length; index += 1) {
    if (request[IMAGE_LIST_FIELDS[index]] !== undefined)
      collectImages(request[IMAGE_LIST_FIELDS[index]], IMAGE_LIST_FIELDS[index], target);
  }
  for (let index = 0; index < FIRST_FRAME_FIELDS.length; index += 1) {
    const field = FIRST_FRAME_FIELDS[index];
    if (request[field] !== undefined)
      target.first = conflictURL(target.first, singleMediaURL(request[field], field), "first frame");
  }
  for (let index = 0; index < LAST_FRAME_FIELDS.length; index += 1) {
    const field = LAST_FRAME_FIELDS[index];
    if (request[field] !== undefined)
      target.last = conflictURL(target.last, singleMediaURL(request[field], field), "last frame");
  }
  const videos = contentVideos.slice();
  const audios = contentAudios.slice();
  for (let index = 0; index < VIDEO_LIST_FIELDS.length; index += 1) {
    if (request[VIDEO_LIST_FIELDS[index]] !== undefined)
      collectMediaURLs(request[VIDEO_LIST_FIELDS[index]], VIDEO_LIST_FIELDS[index], videos);
  }
  for (let index = 0; index < AUDIO_LIST_FIELDS.length; index += 1) {
    if (request[AUDIO_LIST_FIELDS[index]] !== undefined)
      collectMediaURLs(request[AUDIO_LIST_FIELDS[index]], AUDIO_LIST_FIELDS[index], audios);
  }

  let prompt = typeof request.prompt === "string" ? request.prompt.trim() : "";
  if (!prompt && typeof request.input === "string") prompt = request.input.trim();
  if (!prompt && contentTexts.length > 0) prompt = contentTexts.join("\n").trim();
  if (!prompt) fail("prompt is required");
  if (request.negative_prompt !== undefined && request.negative_prompt !== null && request.negative_prompt !== "")
    fail("negative_prompt is not supported; put exclusions into prompt");

  if (request.n !== undefined) {
    const n = Number(request.n);
    if (!Number.isInteger(n) || n !== 1) fail("n only supports 1");
  }

  const family = isDoubao(upstreamModel) ? "doubao" : isExpress(upstreamModel) ? "express" : "flat";
  if (family !== "doubao") {
    const media = target.images.concat(videos, audios);
    if (target.first) media.push(target.first);
    if (target.last) media.push(target.last);
    for (let index = 0; index < media.length; index += 1) {
      if (media[index].indexOf("asset://") === 0)
        fail("asset:// media URLs are supported only by doubao-seedance models");
    }
  }

  const frameCount = (target.first ? 1 : 0) + (target.last ? 1 : 0);
  if (family === "doubao" && frameCount > 0) {
    if (target.images.length + frameCount > 2)
      fail("doubao-seedance models allow at most 2 images when first/last frames are used");
    if (videos.length > 0) fail("doubao-seedance models cannot combine reference videos with first/last frames");
  }
  if (family === "express") {
    if (target.images.length > 9) fail("sd-2/wan-3.0 models support at most 9 reference images");
    if (videos.length > 3) fail("sd-2/wan-3.0 models support at most 3 reference videos");
    if (audios.length > 3) fail("sd-2/wan-3.0 models support at most 3 reference audios");
  }

  let resolution = "";
  const rawResolution =
    request.resolution !== undefined
      ? request.resolution
      : request.video_resolution !== undefined
        ? request.video_resolution
        : request.videoResolution !== undefined
          ? request.videoResolution
          : metadata.resolution;
  if (rawResolution !== undefined && rawResolution !== null) {
    if (typeof rawResolution !== "string") fail("resolution must be a string");
    resolution = rawResolution.trim().toLowerCase();
    if (RESOLUTIONS.indexOf(resolution) < 0) fail("resolution must be one of " + RESOLUTIONS.join(", "));
    const metadataResolution = metadata.resolution;
    if (
      metadataResolution !== undefined &&
      metadataResolution !== null &&
      String(metadataResolution).trim().toLowerCase() !== resolution
    )
      fail("metadata.resolution conflicts with resolution");
  } else if (request.size !== undefined) {
    resolution = resolutionFromSize(request.size);
  }
  resolution = resolutionFor(upstreamModel, resolution);

  let aspect =
    request.aspect_ratio !== undefined
      ? request.aspect_ratio
      : request.ratio !== undefined
        ? request.ratio
        : request.aspectRatio !== undefined
          ? request.aspectRatio
          : metadata.aspect_ratio !== undefined
            ? metadata.aspect_ratio
            : metadata.ratio;
  if (request.aspect_ratio !== undefined && request.ratio !== undefined && request.aspect_ratio !== request.ratio)
    fail("aspect_ratio conflicts with ratio");
  if (aspect !== undefined && aspect !== null) {
    if (typeof aspect !== "string") fail("aspect_ratio must be a string");
    aspect = aspect.trim();
    if (ASPECT_RATIOS.indexOf(aspect) < 0) fail("aspect_ratio must be one of " + ASPECT_RATIOS.join(", "));
  } else {
    aspect = "";
  }

  const durationRule = durationRuleOf(upstreamModel);
  const hasSeconds = request.seconds !== undefined && request.seconds !== null && request.seconds !== "";
  const hasDuration = request.duration !== undefined && request.duration !== null && request.duration !== "";
  const hasDurationAlias =
    request.durationSeconds !== undefined && request.durationSeconds !== null && request.durationSeconds !== "";
  let seconds;
  if (hasSeconds && (hasDuration || hasDurationAlias)) {
    const parsedSeconds = parseDuration(request.seconds);
    if (hasDuration && parseDuration(request.duration) !== parsedSeconds) fail("duration conflicts with seconds");
    if (hasDurationAlias && parseDuration(request.durationSeconds) !== parsedSeconds)
      fail("durationSeconds conflicts with seconds");
    seconds = parsedSeconds;
  } else if (hasSeconds) {
    seconds = parseDuration(request.seconds);
  } else if (hasDuration) {
    seconds = parseDuration(request.duration);
  } else if (hasDurationAlias) {
    seconds = parseDuration(request.durationSeconds);
  } else {
    seconds = durationRule ? durationRule.fallback : DEFAULT_SECONDS;
  }
  if (durationRule && (seconds < durationRule.min || seconds > durationRule.max))
    fail("seconds must be between " + durationRule.min + " and " + durationRule.max + " for model " + upstreamModel);

  let generateAudio;
  if (request.generate_audio !== undefined) {
    if (typeof request.generate_audio !== "boolean") fail("generate_audio must be a boolean");
    generateAudio = request.generate_audio;
  }
  if (metadata.generate_audio !== undefined) {
    if (typeof metadata.generate_audio !== "boolean") fail("metadata.generate_audio must be a boolean");
    if (generateAudio !== undefined && generateAudio !== metadata.generate_audio)
      fail("metadata.generate_audio conflicts with generate_audio");
    generateAudio = metadata.generate_audio;
  }

  let watermark;
  if (request.watermark !== undefined) {
    if (family !== "doubao") fail("watermark is supported only by doubao-seedance models");
    if (typeof request.watermark !== "boolean") fail("watermark must be a boolean");
    watermark = request.watermark;
  }
  if (metadata.watermark !== undefined) {
    if (family !== "doubao") fail("metadata.watermark is supported only by doubao-seedance models");
    if (typeof metadata.watermark !== "boolean") fail("metadata.watermark must be a boolean");
    if (watermark !== undefined && watermark !== metadata.watermark) fail("metadata.watermark conflicts with watermark");
    watermark = metadata.watermark;
  }

  let seed;
  if (request.seed !== undefined) {
    if (family !== "doubao") fail("seed is supported only by doubao-seedance models");
    seed = Number(request.seed);
    if (!Number.isInteger(seed) || seed < -1) fail("seed must be a non-negative integer or -1");
  }
  if (metadata.seed !== undefined) {
    if (family !== "doubao") fail("metadata.seed is supported only by doubao-seedance models");
    if (!Number.isInteger(Number(metadata.seed)) || Number(metadata.seed) < -1)
      fail("metadata.seed must be a non-negative integer or -1");
    if (seed !== undefined && seed !== Number(metadata.seed)) fail("metadata.seed conflicts with seed");
    seed = Number(metadata.seed);
  }

  if (isASeries(upstreamModel) && (target.images.length > 0 || videos.length > 0 || audios.length > 0))
    prompt = translateReferenceTokens(prompt);

  const parsed = {
    prompt: prompt,
    seconds: seconds,
    images: target.images,
    videos: videos,
    audios: audios,
  };
  if (resolution) parsed.resolution = resolution;
  if (aspect) parsed.aspect_ratio = aspect;
  if (target.first) parsed.first_frame_url = target.first;
  if (target.last) parsed.last_frame_url = target.last;
  if (generateAudio !== undefined) parsed.generate_audio = generateAudio;
  if (watermark !== undefined) parsed.watermark = watermark;
  if (seed !== undefined) parsed.seed = seed;
  return parsed;
}

function upstreamBody(parsed, model) {
  const family = isDoubao(model) ? "doubao" : isExpress(model) ? "express" : "flat";
  const body = {
    model: model,
    prompt: parsed.prompt,
    seconds: String(parsed.seconds),
  };
  if (family === "doubao") {
    const metadata = { resolution: parsed.resolution || "720p" };
    if (parsed.aspect_ratio) metadata.ratio = parsed.aspect_ratio;
    if (parsed.generate_audio !== undefined) metadata.generate_audio = parsed.generate_audio;
    if (parsed.watermark !== undefined) metadata.watermark = parsed.watermark;
    if (parsed.seed !== undefined) metadata.seed = parsed.seed;
    const content = [];
    if (parsed.first_frame_url)
      content.push({ type: "image_url", image_url: { url: parsed.first_frame_url }, role: "first_frame" });
    if (parsed.last_frame_url)
      content.push({ type: "image_url", image_url: { url: parsed.last_frame_url }, role: "last_frame" });
    for (let index = 0; index < parsed.images.length; index += 1) {
      content.push({ type: "image_url", image_url: { url: parsed.images[index] }, role: "reference_image" });
    }
    for (let index = 0; index < parsed.videos.length; index += 1) {
      content.push({ type: "video_url", video_url: { url: parsed.videos[index] }, role: "reference_video" });
    }
    for (let index = 0; index < parsed.audios.length; index += 1) {
      content.push({ type: "audio_url", audio_url: { url: parsed.audios[index] }, role: "reference_audio" });
    }
    if (content.length > 0) metadata.content = content;
    body.metadata = metadata;
    return body;
  }
  if (parsed.resolution) body.resolution = parsed.resolution;
  if (parsed.aspect_ratio) body.aspect_ratio = parsed.aspect_ratio;
  if (parsed.first_frame_url) body.first_frame_url = parsed.first_frame_url;
  if (parsed.last_frame_url) body.last_frame_url = parsed.last_frame_url;
  if (parsed.generate_audio !== undefined) body.generate_audio = parsed.generate_audio;
  if (family === "express") {
    if (parsed.images.length > 0) body.images = parsed.images.slice();
    const metadata = {};
    if (parsed.videos.length > 0) metadata.videos = parsed.videos.slice();
    if (parsed.audios.length > 0) metadata.audios = parsed.audios.slice();
    if (parsed.videos.length > 0 || parsed.audios.length > 0) body.metadata = metadata;
    return body;
  }
  if (parsed.images.length > 0) body.images = parsed.images.slice();
  if (parsed.videos.length > 0) body.videos = parsed.videos.slice();
  if (parsed.audios.length > 0) body.audios = parsed.audios.slice();
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
    fail("NSKeven requires public HTTP(S) media URLs; upload the media elsewhere and submit its public URL");
  if (!isObject(body.fields)) fail("video request fields are missing");
  const request = {};
  const fieldNames = Object.keys(body.fields);
  for (let index = 0; index < fieldNames.length; index += 1) {
    const field = fieldNames[index];
    const value = decodeFormField(body.fields[field], field);
    if (JSON_FORM_FIELDS.has(field)) {
      try {
        request[field] = JSON.parse(value);
      } catch (error) {
        fail("form field " + field + " must be valid JSON");
      }
      continue;
    }
    if (BOOLEAN_FORM_FIELDS.has(field)) {
      if (value === "true") request[field] = true;
      else if (value === "false") request[field] = false;
      else fail("form field " + field + " must be true or false");
      continue;
    }
    if (field === "n" || field === "duration" || field === "seed") {
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
  if (!/^task_[A-Za-z0-9_-]{1,186}$/.test(id)) fail(label + " task id is invalid");
  return id;
}

function progressValue(value) {
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

function upstreamStatus(data) {
  return trimmed(data.status).toUpperCase();
}

export function buildSubmitRequest(ctx) {
  const model = trimmed(ctx && ctx.upstreamModel) || trimmed(ctx && ctx.model);
  if (!model) fail("upstream model is required");
  const apiKey = trimmed(ctx && ctx.apiKey);
  if (!apiKey) fail("channel API key is required");
  if (!ctx || typeof ctx.publicTaskId !== "string" || !ctx.publicTaskId.trim()) fail("public task id is required");
  const request = ctx.requestBody;
  if (!isObject(request)) fail("video request body is missing");
  const parsed = normalizeRequest(request, model);
  const body = upstreamBody(parsed, model);
  return {
    url: apiBase(ctx) + "/video/generations",
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

export function parseTaskResult(ctx, body) {
  const data = isObject(body) && isObject(body.data) ? body.data : body;
  if (!isObject(data)) return { status: "UNKNOWN" };
  const status = upstreamStatus(data);
  if (status === "SUCCESS") {
    return { status: "SUCCESS", progress: "100%" };
  }
  if (status === "FAILURE" || status === "FAILED" || status === "CANCELED" || status === "CANCELLED") {
    return { status: "FAILURE", reason: sanitizeReason(data.fail_reason || data.reason, ctx) };
  }
  if (status === "NOT_START" || status === "SUBMITTED" || status === "QUEUED" || status === "QUEUING") {
    return { status: "QUEUED" };
  }
  if (status === "IN_PROGRESS" || status === "PROCESSING" || status === "RUNNING") {
    const result = { status: "IN_PROGRESS" };
    const progress = progressValue(data.progress);
    if (progress) result.progress = progress;
    return result;
  }
  return { status: "UNKNOWN" };
}

export function parseSubmitResponse(ctx, response) {
  if (!response || typeof response.statusCode !== "number" || response.statusCode < 200 || response.statusCode >= 300)
    fail("NSKeven submit request failed");
  const body = response.body;
  if (!isObject(body)) fail("NSKeven submit response must be a JSON object");
  if (body.error) {
    const message = isObject(body.error) ? body.error.message : body.error;
    fail(sanitizeReason(message, ctx));
  }
  if (body.code !== undefined && body.code !== null && body.code !== "success" && body.code !== "0" && body.code !== 0)
    fail(sanitizeReason(body.message || "NSKeven submit failed", ctx));
  const payload = isObject(body.data) && (body.data.task_id !== undefined || body.data.id !== undefined) ? body.data : body;
  const id = upstreamTaskID(payload.task_id !== undefined ? payload.task_id : payload.id, "NSKeven submit response");
  const result = parseTaskResult(ctx, body);
  if (result.status === "UNKNOWN") fail("NSKeven submit response did not include a usable task status");
  const submitted = {
    taskId: id,
    taskData: body,
    state: { model: trimmed(ctx && (ctx.upstreamModel || ctx.model)) },
  };
  if (result.status === "SUCCESS" || result.status === "FAILURE") submitted.immediate = result;
  return submitted;
}

export function buildQueryRequest(ctx) {
  const id = upstreamTaskID(ctx && ctx.taskId, "NSKeven query response");
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
  const model = trimmed(ctx.upstreamModel) || trimmed(ctx.model);
  let parsed;
  try {
    parsed = normalizeRequest(ctx.requestBody, model);
  } catch (error) {
    return {};
  }
  const facts = {
    count: 1,
    // 只统计参考素材；首帧、尾帧是生成条件，不计费。
    input_images: Math.min(parsed.images.length, MAX_INPUT_MEDIA),
    input_videos: Math.min(parsed.videos.length, MAX_INPUT_MEDIA),
    input_audios: Math.min(parsed.audios.length, MAX_INPUT_MEDIA),
  };
  if (parsed.seconds) facts.seconds = parsed.seconds;
  if (parsed.resolution && RESOLUTIONS.indexOf(parsed.resolution) >= 0) facts.video_resolution = parsed.resolution;
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
  const rawID = ctx.upstreamTaskId || (isObject(ctx.data) ? ctx.data.task_id : undefined);
  const id = upstreamTaskID(rawID, "NSKeven content response");
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
          : parsed.first_frame_url || parsed.images.length > 0
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
          message: sanitizeReason(task.fail_reason, { apiKey: "" }),
        };
      }
      return output;
    },
  },
};
