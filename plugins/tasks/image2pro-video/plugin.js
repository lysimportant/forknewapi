export const meta = {
  apiVersion: 1,
  key: "image2pro-video",
  name: "Image2Pro Video",
  description: {
    en: "Image2Pro video generation through the OpenAI video API",
    zh: "通过 OpenAI 视频接口接入 Image2Pro 视频生成",
  },
  version: "1.2.0",
  author: { name: "lysimportant/forknewapi" },
  website: "https://image2pro.top/api-docs",
  baseUrl: "https://api.image2pro.top/v1",
  allowedHosts: ["api.image2pro.top:443"],
  models: ["Seedance2.0", "无限制-MAX-Video"],
  modelRouting: "channel",
  modelMediaTypes: { "Seedance2.0": "video", "无限制-MAX-Video": "video" },
  modelDiscovery: { protocol: "openai", path: "/v1/models" },
  fetchMode: "per_task",
  requiredCapabilities: ["task-submit-no-retry@1"],
  upstreams: ["vendor", "new_api"],
  protocols: ["openai_video"],
  usageSchema: {
    count: {
      type: "number",
      unit: "count",
      unitLabel: { en: "video", zh: "个" },
      description: { en: "Video generation unit price", zh: "视频生成单价" },
    },
    input_images: {
      type: "number",
      unit: "count",
      unitLabel: { en: "image", zh: "张" },
      description: { en: "Input image unit price", zh: "输入图片单价" },
    },
    input_audios: {
      type: "number",
      unit: "count",
      unitLabel: { en: "audio", zh: "段" },
      description: { en: "Input audio unit price", zh: "输入音频单价" },
    },
    input_videos: {
      type: "number",
      unit: "count",
      unitLabel: { en: "video", zh: "段" },
      description: { en: "Input video unit price", zh: "输入视频单价" },
    },
    input_video_seconds: {
      type: "number",
      unit: "second",
      description: { en: "Input video unit price", zh: "输入视频单价" },
    },
  },
  usageExamples: [
    { label: "text", facts: { count: 1, input_images: 0, input_audios: 0, input_videos: 0, input_video_seconds: 0 } },
    { label: "reference", facts: { count: 1, input_images: 2, input_audios: 1, input_videos: 1, input_video_seconds: 0 } },
  ],
  usageProfiles: [
    {
      models: ["无限制-MAX-Video"],
      schema: {
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
        input_audios: {
          type: "number",
          unit: "count",
          unitLabel: { en: "audio", zh: "段" },
          description: { en: "Input audio unit price", zh: "输入音频单价" },
        },
      },
    },
    {
      models: ["Seedance2.0"],
      schema: {
        count: {
          type: "number",
          unit: "count",
          unitLabel: { en: "video", zh: "个" },
          description: { en: "Video generation unit price", zh: "视频生成单价" },
        },
        input_images: {
          type: "number",
          unit: "count",
          unitLabel: { en: "image", zh: "张" },
          description: { en: "Input image unit price", zh: "输入图片单价" },
        },
        input_audios: {
          type: "number",
          unit: "count",
          unitLabel: { en: "audio", zh: "段" },
          description: { en: "Input audio unit price", zh: "输入音频单价" },
        },
      },
    },
  ],
};

const MAX_MODEL = "无限制-MAX-Video";
const SEEDANCE_MODEL = "Seedance2.0";
const MODELS = new Set([MAX_MODEL, SEEDANCE_MODEL]);
const MAX_FIELDS = new Set([
  "model",
  "prompt",
  "input",
  "duration",
  "seconds",
  "durationSeconds",
  "ratio",
  "aspect_ratio",
  "aspectRatio",
  "images",
  "input_image",
  "input_reference",
  "image",
  "reference_images",
  "audios",
  "audio",
  "reference_audios",
  "content",
  "metadata",
  "mode",
  "first_frame",
  "last_frame",
  "seed",
  "size",
  "resolution",
  "n",
  "client_request_id",
]);
const METADATA_FIELDS = new Set([
  "prompt",
  "duration",
  "seconds",
  "durationSeconds",
  "ratio",
  "aspect_ratio",
  "aspectRatio",
  "images",
  "input_image",
  "input_reference",
  "image",
  "reference_images",
  "audios",
  "audio",
  "reference_audios",
  "mode",
  "first_frame",
  "last_frame",
  "seed",
  "size",
  "resolution",
  "n",
]);

function trimmed(value) {
  return typeof value === "string" ? value.trim() : String(value || "").trim();
}

function apiBase(ctx) {
  let base = trimmed((ctx && ctx.baseUrl) || meta.baseUrl).replace(/\/+$/, "");
  if (!/^https:\/\/[^/?#@\\\s]+(?:\/[^?#\\\s]*)?$/.test(base))
    throw new Error("channel base URL must be an HTTPS API base without credentials, query or fragment");
  base = base.replace(/\/v1(?:\/videos)?$/, "");
  return base + "/v1";
}

function publicURL(value, field) {
  if (typeof value !== "string" || !/^https:\/\/[^/?#@\\\s]+(?:[/?#][^\\\s]*)?$/i.test(value.trim()))
    throw new Error(field + " must be a public HTTPS URL");
  return value.trim();
}

function mediaURL(value, field) {
  let url = value;
  if (url && typeof url === "object" && !Array.isArray(url)) url = url.url;
  return publicURL(url, field);
}

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
  return values.map((item) => mediaURL(item, field));
}

function metadataValue(request, metadata, key) {
  if (request[key] !== undefined && metadata[key] !== undefined && JSON.stringify(request[key]) !== JSON.stringify(metadata[key]))
    throw new Error(key + " conflicts with metadata." + key);
  return request[key] === undefined ? metadata[key] : request[key];
}

function decodeBody(body) {
  if (!body || !["json", "form", "multipart"].includes(body.kind)) throw new Error("JSON, form, or multipart body required");
  if (body.kind === "json") {
    if (!body.value || typeof body.value !== "object" || Array.isArray(body.value)) throw new Error("JSON object required");
    return { ...body.value };
  }
  if ((body.files || []).length) throw new Error("Image2Pro requires public HTTPS media URLs; upload media before submitting");
  const request = {};
  for (const key of Object.keys(body.fields || {})) {
    const values = body.fields[key];
    if (!Array.isArray(values) || values.length !== 1) throw new Error(key + " must be provided once");
    let value = values[0];
    if (["content", "metadata", "images", "audios", "videos", "reference_images", "reference_audios", "reference_videos"].includes(key)) {
      try {
        value = JSON.parse(value);
      } catch {
        throw new Error(key + " must be valid JSON");
      }
    }
    request[key] = value;
  }
  return request;
}

function contentInputs(request, metadata) {
  let supplied = metadataValue(request, metadata, "content");
  if (typeof supplied === "string") {
    try {
      supplied = JSON.parse(supplied);
    } catch {
      throw new Error("content must be a JSON array");
    }
  }
  if (supplied === undefined) return { texts: [], images: [], audios: [], first: undefined, last: undefined };
  if (!Array.isArray(supplied)) throw new Error("content must be an array");
  const result = { texts: [], images: [], audios: [], first: undefined, last: undefined };
  for (const item of supplied) {
    if (typeof item === "string") {
      result.texts.push(item);
      continue;
    }
    if (!item || typeof item !== "object" || Array.isArray(item)) throw new Error("invalid content item");
    const type = item.type === "input_image" ? "image_url" : item.type === "input_audio" ? "audio_url" : item.type;
    if (type === "text" || type === "input_text") {
      if (typeof item.text !== "string") throw new Error("content text must be a string");
      result.texts.push(item.text);
      continue;
    }
    if (type !== "image_url" && type !== "audio_url") throw new Error("unsupported content type");
    const value = mediaURL(item[type], "content " + type);
    const role = item.role;
    if (type === "image_url") {
      if (role === "first_frame") {
        if (result.first) throw new Error("first_frame must be provided once");
        result.first = value;
      } else if (role === "last_frame") {
        if (result.last) throw new Error("last_frame must be provided once");
        result.last = value;
      }
      else if (role === undefined || role === "reference_image") result.images.push(value);
      else throw new Error("unsupported image content role");
    } else if (role === undefined || role === "reference_audio") result.audios.push(value);
    else throw new Error("unsupported audio content role");
  }
  return result;
}

// The host authorizes exact channel models. Keep the two legacy wire formats,
// while catalog models use the provider's native video fields without an ID list.
function normalizeCatalogRequest(request, model, publicTaskId) {
  if (typeof model !== "string" || !model.trim() || model !== model.trim() || model.includes(",") || /\p{Cc}/u.test(model))
    throw new Error("model must be a canonical non-empty string");
  let metadata = request.metadata;
  if (typeof metadata === "string") {
    try {
      metadata = JSON.parse(metadata);
    } catch {
      throw new Error("metadata must be a JSON object");
    }
  }
  if (metadata === undefined || metadata === null) metadata = {};
  if (typeof metadata !== "object" || Array.isArray(metadata)) throw new Error("metadata must be a JSON object");
  const allowed = new Set([...MAX_FIELDS, "videos", "reference_videos"]);
  for (const key of Object.keys(request)) if (!allowed.has(key)) throw new Error("unsupported video request field: " + key);
  for (const key of Object.keys(metadata)) {
    if (!allowed.has(key) || ["model", "metadata", "client_request_id"].includes(key)) throw new Error("unsupported metadata field: " + key);
  }
  const source = {};
  for (const key of allowed) {
    if (!["model", "metadata", "client_request_id"].includes(key)) source[key] = metadataValue(request, metadata, key);
  }
  const content = contentInputs(request, metadata);
  if (source.prompt !== undefined && typeof source.prompt !== "string") throw new Error("prompt must be a string");
  if (source.input !== undefined && typeof source.input !== "string") throw new Error("input must be a string");
  if (source.prompt && source.input && source.prompt.trim() !== source.input.trim()) throw new Error("prompt conflicts with input");
  const prompt = trimmed(source.prompt || source.input || content.texts.filter((value) => trimmed(value)).join("\n"));
  if (!prompt) throw new Error("prompt is required");
  const body = { model, prompt };
  // Native contracts differ in defaults (Omni 1.2 always generates 15 seconds).
  // Bound forwarded durations, but never turn these hints into billed seconds.
  const durations = [source.duration, source.seconds, source.durationSeconds].filter((value) => value !== undefined);
  if (durations.length) {
    const numbers = durations.map((value) => {
      if ((typeof value !== "number" && typeof value !== "string") || String(value).trim() === "") throw new Error("duration must be a number");
      return Number(typeof value === "string" ? value.trim().replace(/s$/i, "") : value);
    });
    if (numbers.some((value) => !Number.isSafeInteger(value) || value < 1 || value > 3600 || value !== numbers[0]))
      throw new Error("duration aliases conflict or duration is outside 1-3600 seconds");
    body.duration = numbers[0];
  }
  const ratios = [source.ratio, source.aspect_ratio, source.aspectRatio].filter((value) => value !== undefined);
  if (ratios.some((value) => typeof value !== "string" || !/^\d+:\d+$/.test(value) || value !== ratios[0]))
    throw new Error("ratio aliases conflict or are invalid");
  if (source.ratio !== undefined) body.ratio = source.ratio;
  if (source.aspect_ratio !== undefined || source.aspectRatio !== undefined)
    body.aspect_ratio = source.aspect_ratio === undefined ? source.aspectRatio : source.aspect_ratio;
  for (const field of ["size", "resolution", "mode"]) {
    const value = source[field];
    if (value !== undefined) {
      if (typeof value !== "string" || !value.trim() || value.length > 128 || /\p{Cc}/u.test(value)) throw new Error(field + " must be a non-empty string");
      body[field] = value;
    }
  }
  if (source.seed !== undefined) {
    if (
      (typeof source.seed !== "number" && typeof source.seed !== "string") ||
      String(source.seed).trim() === "" ||
      !Number.isSafeInteger(Number(source.seed)) ||
      Number(source.seed) < 0 ||
      Number(source.seed) > 2147483647
    )
      throw new Error("seed must be an integer between 0 and 2147483647");
    body.seed = Number(source.seed);
  }
  if (source.n !== undefined) {
    if ((typeof source.n !== "number" && typeof source.n !== "string") || Number(source.n) !== 1) throw new Error("n only supports integer 1");
    body.n = 1;
  }
  const facts = { model, count: 1, input_images: 0, input_videos: 0, input_audios: 0, input_video_seconds: 0 };
  let first;
  let last;
  for (const field of ["first_frame", "last_frame"]) {
    if (source[field] !== undefined) body[field] = mediaURL(source[field], field);
  }
  first = body.first_frame;
  last = body.last_frame;
  for (const field of ["images", "input_image", "reference_images"]) {
    let supplied = source[field];
    if (supplied === undefined || supplied === null || supplied === "") continue;
    if (typeof supplied === "string" && supplied.trim().startsWith("[")) {
      try {
        supplied = JSON.parse(supplied);
      } catch {
        throw new Error(field + " must be a JSON array");
      }
    }
    const values = Array.isArray(supplied) ? supplied : [supplied];
    if (values.length > 128) throw new Error("at most 128 reference images are supported");
    const images = values.map((item) => {
      if (item && typeof item === "object" && !Array.isArray(item) && Object.keys(item).some((key) => !["url", "role"].includes(key)))
        throw new Error("unsupported image reference field");
      const url = mediaURL(item, field);
      if (item && typeof item === "object" && !Array.isArray(item) && item.role !== undefined) {
        if (field !== "reference_images" || !["first_frame", "last_frame"].includes(item.role)) throw new Error("unsupported image content role");
        if (item.role === "first_frame") {
          if (first) throw new Error("first_frame must be provided once");
          first = url;
        } else {
          if (last) throw new Error("last_frame must be provided once");
          last = url;
        }
        return { url, role: item.role };
      }
      facts.input_images += 1;
      return typeof item === "string" ? url : { url };
    });
    body[field] = field === "input_image" && !Array.isArray(supplied) ? images[0] : images;
  }
  const frameAliases = [source.image, source.input_reference, content.first].filter((value) => value !== undefined);
  for (const supplied of frameAliases) {
    const url = mediaURL(supplied, "input_reference");
    if (first && first !== url) throw new Error("first_frame conflicts with input_reference");
    if (!first) {
      body.reference_images = [].concat(body.reference_images || [], { url, role: "first_frame" });
      first = url;
    }
  }
  if (content.last) {
    if (last && last !== content.last) throw new Error("last_frame conflicts with content");
    if (!last) body.reference_images = [].concat(body.reference_images || [], { url: content.last, role: "last_frame" });
    last = content.last;
  }
  if (content.images.length) body.reference_images = [].concat(body.reference_images || [], content.images);
  facts.input_images += content.images.length;
  const audios = content.audios.slice();
  for (const field of ["audios", "audio", "reference_audios"]) {
    const values = mediaList(source[field], field);
    if (audios.length + values.length > 128) throw new Error("at most 128 reference audios are supported");
    audios.push(...values);
  }
  if (audios.length) body.audios = audios;
  facts.input_audios = audios.length;
  for (const field of ["videos", "reference_videos"]) {
    const supplied = source[field];
    if (supplied === undefined || supplied === null) continue;
    if (!Array.isArray(supplied) || supplied.length > 128) throw new Error(field + " must be an array of at most 128 videos");
    body[field] = supplied.map((item) => {
      const rawURL = item && typeof item === "object" && !Array.isArray(item) ? item.url : item;
      const url =
        field === "reference_videos" && typeof rawURL === "string" && /^extend:\/\/[A-Za-z0-9_.-]{1,200}$/.test(rawURL) ? rawURL : publicURL(rawURL, field);
      if (typeof item === "string") return url;
      if (Object.keys(item).some((key) => !["url", "start_seconds", "require_audio"].includes(key))) throw new Error("unsupported video reference field");
      const video = { url };
      if (item.start_seconds !== undefined) {
        if (typeof item.start_seconds !== "number" || !Number.isFinite(item.start_seconds) || item.start_seconds < 0 || item.start_seconds > 3600)
          throw new Error("start_seconds must be between 0 and 3600");
        video.start_seconds = item.start_seconds;
      }
      if (item.require_audio !== undefined) {
        if (typeof item.require_audio !== "boolean") throw new Error("require_audio must be a boolean");
        video.require_audio = item.require_audio;
      }
      return video;
    });
    facts.input_videos += supplied.length;
  }
  if (facts.input_images > 128 || facts.input_audios > 128 || facts.input_videos > 128) throw new Error("reference media count exceeds 128");
  if (last && !first) throw new Error("last_frame requires first_frame");
  if ((first || last) && (facts.input_images || facts.input_audios || facts.input_videos)) throw new Error("keyframes cannot be mixed with reference media");
  if (publicTaskId !== undefined) {
    if (request.client_request_id !== undefined && request.client_request_id !== publicTaskId)
      throw new Error("client_request_id is reserved for the public task id");
    body.client_request_id = publicTaskId;
  }
  return { body, facts };
}

function normalizeRequest(request, model, publicTaskId) {
  if (!request || typeof request !== "object" || Array.isArray(request)) throw new Error("video request must be a JSON object");
  if (!MODELS.has(model)) return normalizeCatalogRequest(request, model, publicTaskId);
  for (const key of Object.keys(request)) if (!MAX_FIELDS.has(key)) throw new Error("unsupported video request field: " + key);
  let metadata = request.metadata;
  if (metadata === undefined || metadata === null) metadata = {};
  if (typeof metadata === "string") {
    try {
      metadata = JSON.parse(metadata);
    } catch {
      throw new Error("metadata must be a JSON object");
    }
  }
  if (!metadata || typeof metadata !== "object" || Array.isArray(metadata)) throw new Error("metadata must be a JSON object");
  for (const key of Object.keys(metadata)) if (!METADATA_FIELDS.has(key) && key !== "content") throw new Error("unsupported metadata field: " + key);

  const content = contentInputs(request, metadata);
  const promptValue = metadataValue(request, metadata, "prompt");
  const input = metadataValue(request, metadata, "input");
  if (promptValue !== undefined && typeof promptValue !== "string") throw new Error("prompt must be a string");
  let prompt = promptValue === undefined ? "" : promptValue;
  if (input !== undefined) {
    if (typeof input !== "string") throw new Error("input must be a string");
    if (prompt && prompt.trim() !== input.trim()) throw new Error("prompt conflicts with input");
    prompt = input;
  }
  if (!prompt.trim() && content.texts.some((value) => trimmed(value))) prompt = content.texts.filter((value) => trimmed(value)).join("\n");

  const durationValues = ["duration", "seconds", "durationSeconds"].map((key) => metadataValue(request, metadata, key)).filter((value) => value !== undefined);
  let duration;
  if (durationValues.length) {
    if (durationValues.some((value) => (typeof value !== "number" && typeof value !== "string") || String(value).trim() === "")) throw new Error("duration must be a number");
    const numbers = durationValues.map((value) => Number(value));
    if (numbers.some((value) => !Number.isSafeInteger(value)) || numbers.some((value) => value !== numbers[0])) throw new Error("duration aliases conflict or are invalid");
    duration = numbers[0];
  }
  if (model === MAX_MODEL && (duration === undefined || duration < 4 || duration > 12)) throw new Error("无限制-MAX-Video duration must be an integer between 4 and 12");
  if (model === SEEDANCE_MODEL && duration !== undefined && duration <= 0) throw new Error("Seedance2.0 duration must be a positive integer");

  const ratioValues = ["ratio", "aspect_ratio", "aspectRatio"].map((key) => metadataValue(request, metadata, key)).filter((value) => value !== undefined);
  let ratio;
  if (ratioValues.length) {
    if (ratioValues.some((value) => typeof value !== "string") || ratioValues.some((value) => value !== ratioValues[0])) throw new Error("ratio aliases conflict or are invalid");
    ratio = ratioValues[0].trim();
    if (!/^\d+:\d+$/.test(ratio)) throw new Error("ratio must use width:height notation");
    if (model === MAX_MODEL && !["21:9", "16:9", "4:3", "1:1", "3:4", "9:16"].includes(ratio)) throw new Error("unsupported ratio for 无限|制-MAX-Video".replace("|", ""));
  }

  const images = content.images.slice();
  for (const key of ["images", "input_image", "reference_images"]) images.push(...mediaList(metadataValue(request, metadata, key), key));
  const audios = content.audios.slice();
  for (const key of ["audios", "audio", "reference_audios"]) audios.push(...mediaList(metadataValue(request, metadata, key), key));
  let first = content.first;
  let last = content.last;
  for (const key of ["image", "input_reference"]) {
    const supplied = metadataValue(request, metadata, key);
    if (supplied !== undefined) {
      const value = mediaURL(supplied, key);
      if (first && first !== value) throw new Error("first_frame conflicts with " + key);
      first = value;
    }
  }
  const firstValue = metadataValue(request, metadata, "first_frame");
  const lastValue = metadataValue(request, metadata, "last_frame");
  if (firstValue !== undefined) {
    const value = mediaURL(firstValue, "first_frame");
    if (first && first !== value) throw new Error("first_frame conflicts with content");
    first = value;
  }
  if (lastValue !== undefined) {
    const value = mediaURL(lastValue, "last_frame");
    if (last && last !== value) throw new Error("last_frame conflicts with content");
    last = value;
  }
  if (images.length > (model === MAX_MODEL ? 5 : 9)) throw new Error("too many reference images for this model");
  if (audios.length > 3) throw new Error("at most 3 reference audios are supported");
  if (new Set(audios).size !== audios.length) throw new Error("reference audios cannot be repeated");
  if ((first || last) && (images.length || audios.length)) throw new Error("keyframes cannot be mixed with reference images or audio");
  if (last && !first) throw new Error("last_frame requires first_frame");
  if (model === SEEDANCE_MODEL && last) throw new Error("Seedance2.0 does not support last_frame");
  if (model === MAX_MODEL && !prompt.trim()) throw new Error("prompt is required for 无限制-MAX-Video");
  if (model === SEEDANCE_MODEL && !prompt.trim() && !images.length && !first) throw new Error("prompt or an image is required for Seedance2.0");

  const body = { model };
  if (prompt.trim()) body.prompt = prompt.trim();
  if (duration !== undefined) body.duration = duration;
  if (ratio !== undefined) body.ratio = ratio;
  if (images.length) body.images = images.slice();
  if (audios.length) body.audios = audios.slice();
  if (model === MAX_MODEL) {
    const inferredMode = first || last ? "keyframe" : images.length || audios.length ? "reference" : "text";
    const mode = metadataValue(request, metadata, "mode");
    if (mode !== undefined && mode !== inferredMode) throw new Error("mode does not match the supplied media");
    body.mode = mode === undefined ? inferredMode : mode;
    if (first) body.first_frame = first;
    if (last) body.last_frame = last;
    const seed = metadataValue(request, metadata, "seed");
    if (seed !== undefined) {
      if ((typeof seed !== "number" && typeof seed !== "string") || String(seed).trim() === "" || !Number.isSafeInteger(Number(seed)) || Number(seed) < 0 || Number(seed) > 2147483647) throw new Error("seed must be an integer between 0 and 2147483647");
      body.seed = Number(seed);
    }
    const sizeValue = metadataValue(request, metadata, "size");
    const resolution = metadataValue(request, metadata, "resolution");
    if (sizeValue !== undefined && resolution !== undefined && String(sizeValue).toUpperCase() !== String(resolution).toUpperCase()) throw new Error("size conflicts with resolution");
    const size = sizeValue === undefined ? resolution : sizeValue;
    if (size !== undefined) {
      if (typeof size !== "string" || size.toUpperCase() !== "720P") throw new Error("size only supports 720P for 无限制-MAX-Video");
      body.size = "720P";
    }
  } else {
    if (metadataValue(request, metadata, "mode") !== undefined || metadataValue(request, metadata, "seed") !== undefined || metadataValue(request, metadata, "size") !== undefined || metadataValue(request, metadata, "resolution") !== undefined)
      throw new Error("mode, seed, and size are unsupported for Seedance2.0");
    if (first) body.images = [first].concat(body.images || []);
  }
  const n = metadataValue(request, metadata, "n");
  if (n !== undefined) {
    if ((typeof n !== "number" && typeof n !== "string") || Number(n) !== 1) throw new Error("n only supports integer 1");
    body.n = 1;
  }
  if (publicTaskId !== undefined) {
    const requestedId = metadataValue(request, metadata, "client_request_id");
    if (requestedId !== undefined && requestedId !== publicTaskId) throw new Error("client_request_id is reserved for the public task id");
    body.client_request_id = publicTaskId;
  }
  return { body, facts: { model, seconds: duration, count: 1, input_images: images.length, input_audios: audios.length } };
}

function sanitizeReason(value, apiKey) {
  let reason = trimmed(value) || "upstream video generation failed";
  if (apiKey) reason = reason.split(String(apiKey)).join("[redacted]");
  return reason.replace(/Bearer\s+\S+|\bsk-[A-Za-z0-9_-]+/gi, "[redacted]");
}

function taskId(value, ctx) {
  const pattern = ctx && ctx.upstream && ctx.upstream.kind === "new_api" ? /^task_[A-Za-z0-9_-]{1,186}$/ : /^[0-9a-f]{16,80}$/i;
  return typeof value === "string" && pattern.test(value) ? value : "";
}

function resultURL(body) {
  if (!body || typeof body !== "object" || Array.isArray(body)) return "";
  const candidates = [body.url, Array.isArray(body.output) && body.output[0] && body.output[0].url, body.metadata && body.metadata.url];
  for (const value of candidates) {
    try {
      return publicURL(value, "video URL");
    } catch {
      // Another documented result location may carry the usable URL.
    }
  }
  return "";
}

export function buildSubmitRequest(ctx) {
  if (typeof ctx.publicTaskId !== "string" || !ctx.publicTaskId) throw new Error("public task id is required for idempotency");
  const built = normalizeRequest(ctx.requestBody, ctx.upstreamModel || ctx.model, ctx.publicTaskId);
  // Each gateway assigns its own public ID; only the direct vendor accepts it in the body.
  if (ctx.upstream && ctx.upstream.kind === "new_api") delete built.body.client_request_id;
  return {
    url: apiBase(ctx) + "/videos",
    method: "POST",
    headers: { Authorization: "Bearer " + ctx.apiKey, "Content-Type": "application/json", Accept: "application/json", "Idempotency-Key": ctx.publicTaskId },
    body: built.body,
    noRetry: true,
  };
}

export function parseSubmitResponse(ctx, response) {
  const body = response && response.body;
  if (!response || response.statusCode < 200 || response.statusCode >= 300 || !body || typeof body !== "object" || Array.isArray(body)) throw new Error("invalid upstream video creation response");
  const id = taskId(body.id, ctx);
  if (!id) throw new Error("upstream video id must be 16-80 hexadecimal characters");
  const result = parseTaskResult(ctx, body);
  if (result.status === "UNKNOWN") throw new Error(result.reason || "invalid upstream video result");
  const parsed = { taskId: id, taskData: body, state: { model: ctx && (ctx.upstreamModel || ctx.model) } };
  if (result.status === "SUCCESS" || result.status === "FAILURE") parsed.immediate = result;
  return parsed;
}

export function parseTaskResult(ctx, body) {
  if (!body || typeof body !== "object" || Array.isArray(body)) return { status: "UNKNOWN", reason: "invalid upstream video task response" };
  if (body.status === "queued") return { status: "QUEUED" };
  if (body.status === "in_progress") return { status: "IN_PROGRESS" };
  if (ctx && ctx.upstream && ctx.upstream.kind === "new_api") {
    if (body.status === "completed") return { status: "SUCCESS", progress: "100%" };
  }
  if (body.status === "processing") return { status: "IN_PROGRESS" };
  if (body.status === "completed") {
    const url = resultURL(body);
    return url ? { status: "SUCCESS", progress: "100%", url } : { status: "UNKNOWN", reason: "completed video URL is missing or invalid" };
  }
  if (body.status === "failed") return { status: "FAILURE", reason: sanitizeReason(body.error && (body.error.message || body.error), ctx && ctx.apiKey) };
  return { status: "UNKNOWN", reason: "unrecognized upstream video task status" };
}

export function buildQueryRequest(ctx) {
  const id = taskId(ctx.taskId, ctx);
  if (!id) throw new Error("upstream video id must be 16-80 hexadecimal characters");
  return { url: apiBase(ctx) + "/videos/" + encodeURIComponent(id), method: "GET", headers: { Authorization: "Bearer " + ctx.apiKey, Accept: "application/json" } };
}

export function extractUsage(ctx) {
  const built = normalizeRequest(ctx.requestBody, ctx.upstreamModel || ctx.model);
  if (ctx.usagePurpose === "billing_ratios") return {};
  // Catalog contracts are priced per generation. A requested duration is not
  // necessarily generated duration, so it must not create a per-second charge.
  if (!MODELS.has(built.facts.model)) {
    return { count: 1, input_images: built.facts.input_images, input_videos: built.facts.input_videos, input_audios: built.facts.input_audios, input_video_seconds: 0 };
  }
  // 只统计参考素材；首帧、尾帧是生成条件，不计费。
  const counts = { input_images: built.facts.input_images, input_audios: built.facts.input_audios };
  return built.facts.model === MAX_MODEL ? (built.facts.seconds === undefined ? counts : Object.assign({ seconds: built.facts.seconds }, counts)) : Object.assign({ count: 1 }, counts);
}

function completedDuration(body, model) {
  const value = body && body.duration;
  if ((typeof value !== "number" && typeof value !== "string") || String(value).trim() === "") return undefined;
  const seconds = Number(value);
  const catalogModel = typeof model === "string" && model !== "" && !MODELS.has(model);
  return Number.isSafeInteger(seconds) && seconds >= (catalogModel ? 1 : 4) && seconds <= (catalogModel ? 3600 : 12) ? seconds : undefined;
}

export function extractUsageOnComplete(task, result, body) {
  if (!result || result.status !== "SUCCESS") return null;
  const model = task && task.state && task.state.model;
  if (model && !MODELS.has(model)) {
    const usage = body && body.usage;
    if (!usage || typeof usage !== "object" || Array.isArray(usage)) return null;
    const facts = {};
    // Only explicitly identified reference usage can replace submitted counts;
    // output duration and keyframe counts must not become input charges.
    const fields = {
      input_images: ["reference_image_count"],
      input_videos: ["reference_video_count"],
      input_audios: ["reference_audio_count"],
      input_video_seconds: ["reference_video_seconds", "input_seconds"],
    };
    for (const key of Object.keys(fields)) {
      for (const field of fields[key]) {
        const value = usage[field];
        if ((typeof value !== "number" && typeof value !== "string") || String(value).trim() === "") continue;
        const quantity = Number(value);
        const seconds = key === "input_video_seconds";
        if (!Number.isFinite(quantity) || quantity < 0 || quantity > (seconds ? 3600 : 128) || (!seconds && !Number.isSafeInteger(quantity))) continue;
        facts[key] = quantity;
        break;
      }
    }
    return Object.keys(facts).length ? facts : null;
  }
  const seconds = completedDuration(body);
  if (model === MAX_MODEL && seconds !== undefined) return { seconds };
  return model === MAX_MODEL ? null : { count: 1 };
}

export function listArtifacts(task) {
  return task.status === "SUCCESS" ? [{ key: "video", type: "video", mimeType: "video/mp4" }] : [];
}

export function buildContentRequest(ctx) {
  if (ctx.artifactKey === "video" && ctx.upstream && ctx.upstream.kind === "new_api") {
    const id = taskId(ctx.upstreamTaskId, ctx);
    if (!id) throw new Error("invalid upstream gateway task id");
    return { url: apiBase(ctx) + "/videos/" + encodeURIComponent(id) + "/content", method: ctx.clientRequest.method, headers: { Authorization: "Bearer " + ctx.apiKey } };
  }
  if (ctx.artifactKey !== "video" || !resultURL(ctx.data)) throw new Error("artifact_not_found");
  return { url: resultURL(ctx.data), method: ctx.clientRequest.method, credentialless: true };
}

export const protocols = {
  openai_video: {
    decodeRequest: function (ctx) {
      const request = decodeBody(ctx.body);
      const model = ctx.upstreamModel || ctx.model;
      const built = normalizeRequest(request, model);
      const hasMedia =
        built.body.first_frame ||
        (built.body.images || []).length ||
        built.body.input_image ||
        (built.body.reference_images || []).length ||
        (built.body.audios || []).length ||
        (built.body.videos || []).length ||
        (built.body.reference_videos || []).length;
      const action = hasMedia ? "image_to_video" : "text_to_video";
      return { kind: "submit", model: ctx.model, action, requestBody: Object.assign({}, request, { model: ctx.model }) };
    },
    render: function (_ctx, task) {
      const statuses = { NOT_START: "queued", SUBMITTED: "queued", QUEUED: "queued", IN_PROGRESS: "in_progress", SUCCESS: "completed", FAILURE: "failed" };
      const output = { id: task.task_id, object: "video", model: (task.properties || {}).origin_model_name, status: statuses[task.status] || "unknown" };
      const properties = task.properties || {};
      const model = properties.upstream_model_name || (task.data && task.data.model) || properties.origin_model_name;
      const duration = completedDuration(task.data, model);
      if (duration !== undefined) output.duration = duration;
      if (task.status === "FAILURE") output.error = { code: "video_generation_failed", message: "Video generation failed" };
      return output;
    },
  },
};
