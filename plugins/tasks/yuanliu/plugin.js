/** 源流开放 API 已核对的固定画幅；自适应名称按各渠道目录保留。 */
const FIXED_RATIOS = ["16:9", "4:3", "1:1", "3:4", "9:16", "21:9"];

/** 2026-10-08 模型目录快照；冲突能力采用字段与说明的保守交集，不复制人民币报价。 */
const MODEL_SPECS = {
  "seedance-2.5-guanfang-anmiao": {
    alias: "Yuan-Seedance-2.5-Official",
    min: 4,
    max: 30,
    resolutions: ["480p", "720p", "1080p"],
    ratios: ["adaptive", ...FIXED_RATIOS],
    images: 30,
    videos: 0,
    audios: 10,
    prompt: 16000,
    billing: "per_second",
  },
  yl_g7zy_seedance_v2_0_std: {
    alias: "Yuan-Seedance-2.0-LJ",
    min: 4,
    max: 15,
    resolutions: ["720p"],
    ratios: ["auto", ...FIXED_RATIOS],
    images: 9,
    videos: 0,
    audios: 0,
    prompt: 16000,
    billing: "per_call",
  },
  yl_g7zy_seedance_v2_0_std_full: {
    alias: "Yuan-Seedance-2.0-LJ-Full",
    min: 5,
    max: 15,
    resolutions: ["720p"],
    ratios: ["auto", ...FIXED_RATIOS],
    images: 9,
    videos: 3,
    audios: 3,
    total: 15,
    prompt: 16000,
    billing: "per_call",
  },
  yl_g7zy_seedance_v2_5: {
    alias: "Yuan-Seedance-2.5-LJ",
    min: 4,
    max: 30,
    resolutions: ["720p"],
    ratios: ["auto", ...FIXED_RATIOS],
    images: 30,
    videos: 0,
    audios: 0,
    prompt: 16000,
    billing: "per_call",
  },
  yl_g7zy_seedance_v2_5_full: {
    alias: "Yuan-Seedance-2.5-LJ-Full",
    min: 5,
    max: 30,
    resolutions: ["720p"],
    ratios: ["auto", ...FIXED_RATIOS],
    images: 30,
    videos: 10,
    audios: 10,
    total: 50,
    prompt: 16000,
    billing: "per_second",
  },
  "yl_seedance-2-0_ba0687ff09f2": {
    alias: "Yuan-Seedance-2.0-HD",
    min: 5,
    max: 15,
    durations: [5, 10, 15],
    resolutions: ["720p"],
    ratios: ["adaptive", ...FIXED_RATIOS],
    images: 9,
    videos: 0,
    audios: 0,
    prompt: 10000,
    billing: "per_call",
  },
  "yl_seedance-2-5_6caffaca7390": {
    alias: "Yuan-Seedance-2.5-HD",
    min: 4,
    max: 30,
    resolutions: ["720p"],
    ratios: ["adaptive", ...FIXED_RATIOS],
    images: 30,
    videos: 0,
    audios: 0,
    prompt: 16000,
    billing: "per_call",
  },
  "yl_seedance-2-5_0fab2f1b1f10": {
    alias: "Yuan-Seedance-2.5-HD-Full",
    min: 10,
    max: 30,
    resolutions: ["720p"],
    ratios: ["adaptive", ...FIXED_RATIOS],
    images: 30,
    videos: 10,
    audios: 10,
    prompt: 16000,
    billing: "per_call",
  },
  "yl_seedance-2-5_750271498003": {
    alias: "Yuan-Seedance-2.5-HD-PerSecond",
    min: 10,
    max: 30,
    resolutions: ["720p"],
    ratios: ["adaptive", ...FIXED_RATIOS],
    images: 30,
    videos: 10,
    audios: 10,
    prompt: 16000,
    billing: "per_second",
  },
  yl_api_hmstudio_seedance_v2_5_101010_7d58bbb217e6: {
    alias: "Yuan-Seedance-2.5-YS-Full",
    min: 4,
    max: 30,
    resolutions: ["720p"],
    ratios: FIXED_RATIOS,
    images: 10,
    videos: 10,
    audios: 10,
    prompt: 16000,
    billing: "per_call",
  },
  yl_api_hmstudio_seedance_v2_5_dc729300ff39: {
    alias: "Yuan-Seedance-2.5-YS",
    min: 4,
    max: 30,
    resolutions: ["720p"],
    ratios: FIXED_RATIOS,
    images: 10,
    videos: 0,
    audios: 0,
    prompt: 16000,
    billing: "per_call",
  },
  "yl_video-30_76dbb7993f8e": {
    alias: "Yuan-Seedance-2.5-YL1",
    min: 30,
    max: 30,
    resolutions: ["720p"],
    ratios: FIXED_RATIOS,
    images: 9,
    videos: 0,
    audios: 0,
    prompt: 8000,
    billing: "per_call",
  },
  yl_api_hmstudio_seedance_v2_0_514a65db713b: {
    alias: "Yuan-Seedance-2.0-YS",
    min: 4,
    max: 15,
    resolutions: ["720p"],
    ratios: FIXED_RATIOS,
    images: 9,
    videos: 0,
    audios: 0,
    prompt: 16000,
    billing: "per_call",
  },
};

/** 按次模型每次请求只生成一个视频；单价由管理员以宿主货币配置。 */
const CALL_SCHEMA = {
  count: {
    type: "number",
    unit: "count",
    unitLabel: { en: "video", zh: "个" },
    description: { en: "Video generation unit price", zh: "视频生成单价" },
  },
};

/** 源流任务插件元信息；声明目录与精确别名，不占用其他厂商的原生路由。 */
export const meta = {
  apiVersion: 1,
  key: "yuanliu",
  name: "Yuanliu Video",
  icon: "text:源流",
  version: "1.0.0",
  author: { name: "Yuanliu" },
  description: {
    en: "Yuanliu Seedance video generation with public URL references",
    zh: "源流 Seedance 视频生成，支持公网 URL 参考素材",
  },
  baseUrl: "https://test.yuanliuai.tsyzai.com/openapi/v1",
  modelDiscovery: { protocol: "openai", path: "/openapi/v1/models" },
  models: Object.keys(MODEL_SPECS),
  modelAliases: Object.fromEntries(Object.entries(MODEL_SPECS).map(([model, spec]) => [spec.alias, model])),
  fetchMode: "per_task",
  protocols: ["openai_video"],
  requiredCapabilities: ["task-submit-no-retry@1"],
  usageSchema: CALL_SCHEMA,
  usageProfiles: Object.entries(MODEL_SPECS).map(([model, spec]) => ({
    models: [model],
    schema:
      spec.billing === "per_call"
        ? CALL_SCHEMA
        : {
            seconds: {
              type: "number",
              unit: "second",
              description: {
                en: "Video generation unit price",
                zh: "视频生成单价",
              },
            },
            resolution: {
              enum: spec.resolutions,
              description: {
                en: "Output video resolution",
                zh: "输出视频分辨率",
              },
            },
          },
  })),
};

/** 客户端字段仅覆盖已确认开放协议及 Canvas 普通参考桥接，未知语义必须显式拒绝。 */
const REQUEST_FIELDS = [
  "model",
  "prompt",
  "seconds",
  "duration",
  "resolution",
  "size",
  "aspect_ratio",
  "ratio",
  "images",
  "videos",
  "audios",
  "input_reference",
  "metadata",
  "mode",
];

/** 将精确展示别名解析为上游 ID；未知或未适配模型不继承相似型号能力。 */
function modelSpec(model) {
  const upstream = meta.modelAliases[model] || model;
  if (!Object.prototype.hasOwnProperty.call(MODEL_SPECS, upstream)) throw new Error("unsupported Yuanliu model");
  return MODEL_SPECS[upstream];
}

/** 验证整秒范围和离散档位；数字字符串只用于 multipart/seconds 兼容，不接受自动时长。 */
function validDuration(value, spec) {
  if ((typeof value !== "number" && typeof value !== "string") || (typeof value === "string" && !/^\d+$/.test(value)))
    throw new Error("duration must be an integer in the model range");
  const duration = Number(value);
  if (!Number.isSafeInteger(duration) || duration < spec.min || duration > spec.max || (spec.durations && !spec.durations.includes(duration)))
    throw new Error("duration must be an integer in the model range");
  return duration;
}

/** 验证 HTTP(S) 参考地址，不接受文件占位符、Base64、反斜杠或 URL 内凭据；可达性由上游校验。 */
function referenceURL(value) {
  if (typeof value !== "string" || /[\s\\]/.test(value)) throw new Error("references require public HTTP(S) URLs");
  for (const character of value) {
    const code = character.charCodeAt(0);
    if (code <= 31 || code === 127) throw new Error("references require public HTTP(S) URLs");
  }
  const match = /^https?:\/\/([^/?#]+)(?:[/?][^#]*)?$/.exec(value);
  if (!match || match[1].includes("@")) throw new Error("references require public HTTP(S) URLs without credentials");
  return value;
}

/** 保留管理员配置的 API 路径前缀，拒绝会改变请求目标或混入凭据的渠道地址。 */
function apiBase(ctx) {
  const base = typeof ctx.baseUrl === "string" ? ctx.baseUrl.replace(/\/+$/, "") : "";
  if (!/^https?:\/\/[^/?#@\s\\]+(?:\/[^?#\s\\]*)?$/.test(base))
    throw new Error("Yuanliu channel base URL must be an HTTP(S) API base without credentials, query or fragment");
  return base;
}

/** 仅从宿主渠道凭据构造 Bearer 头，缺失或含换行的密钥在发送前明确失败。 */
function authorization(ctx) {
  if (typeof ctx.apiKey !== "string" || !ctx.apiKey.trim() || /[\r\n]/.test(ctx.apiKey)) throw new Error("Yuanliu channel API key is required");
  return "Bearer " + ctx.apiKey;
}

/** 合并同义参数时保留显式值，出现不同值则失败，避免按低档计费却发送高档参数。 */
function consistentValue(values, name, fallback) {
  const present = values.filter((value) => value !== undefined);
  if (present.some((value) => value !== present[0])) throw new Error(name + " fields conflict");
  return present.length ? present[0] : fallback;
}

/** 规范化普通参考请求；不改变素材重复次数、每类素材顺序或提示词引用编号。 */
function normalizeRequest(source, model) {
  if (!source || typeof source !== "object" || Array.isArray(source)) throw new Error("request body must be an object");
  for (const field of Object.keys(source)) if (!REQUEST_FIELDS.includes(field)) throw new Error("unsupported Yuanliu field: " + field);
  const spec = modelSpec(model);
  const metadata = source.metadata === undefined ? {} : source.metadata;
  if (!metadata || typeof metadata !== "object" || Array.isArray(metadata)) throw new Error("metadata must be an object");
  for (const field of Object.keys(metadata))
    if (!["content", "resolution", "ratio", "aspect_ratio", "mode", "omni_reference_task_type"].includes(field))
      throw new Error("unsupported Yuanliu metadata field: " + field);
  if (metadata.omni_reference_task_type !== undefined && metadata.omni_reference_task_type !== "reference")
    throw new Error("Yuanliu supports ordinary references, not video editing or extension");

  const durations = [source.duration, source.seconds].filter((value) => value !== undefined).map((value) => validDuration(value, spec));
  if (!durations.length) throw new Error("duration or seconds is required");
  const duration = consistentValue(durations, "duration", undefined);
  const rawResolutions = [source.resolution, source.size, metadata.resolution].map((value) => (typeof value === "string" ? value.toLowerCase() : value));
  const resolution = consistentValue(rawResolutions, "resolution", spec.resolutions[0]);
  if (!spec.resolutions.includes(resolution)) throw new Error("unsupported Yuanliu resolution");
  const aspectRatio = consistentValue([source.aspect_ratio, source.ratio, metadata.aspect_ratio, metadata.ratio], "aspect_ratio", "16:9");
  if (!spec.ratios.includes(aspectRatio)) throw new Error("unsupported Yuanliu aspect_ratio");
  const request = {
    model: source.model,
    prompt: source.prompt,
    duration,
    resolution,
    aspect_ratio: aspectRatio,
    images: [],
    videos: [],
    audios: [],
  };
  for (const type of ["images", "videos", "audios"]) {
    if (source[type] === undefined) continue;
    if (!Array.isArray(source[type])) throw new Error(type + " must be a URL array");
    for (const reference of source[type]) request[type].push(referenceURL(reference));
  }
  if (source.input_reference !== undefined) {
    let reference = source.input_reference;
    if (reference && typeof reference === "object" && !Array.isArray(reference)) {
      const keys = Object.keys(reference);
      if (keys.length !== 1 || !["url", "image_url"].includes(keys[0])) throw new Error("input_reference requires one image URL without frame roles");
      reference = reference[keys[0]];
      if (reference && typeof reference === "object" && !Array.isArray(reference) && Object.keys(reference).length === 1 && typeof reference.url === "string")
        reference = reference.url;
    }
    request.images.push(referenceURL(reference));
  }
  const texts = [];
  if (metadata.content !== undefined) {
    if (!Array.isArray(metadata.content)) throw new Error("metadata.content must be an array");
    for (const item of metadata.content) {
      if (!item || typeof item !== "object" || Array.isArray(item)) throw new Error("invalid metadata.content item");
      if (item.type === "text") {
        if (Object.keys(item).some((field) => !["type", "text"].includes(field)) || typeof item.text !== "string")
          throw new Error("invalid metadata.content text");
        texts.push(item.text);
        continue;
      }
      if (!["image_url", "video_url", "audio_url"].includes(item.type)) throw new Error("unsupported metadata.content type");
      const type = item.type.slice(0, -4);
      if (Object.keys(item).some((field) => !["type", "role", item.type].includes(field)) || (item.role !== undefined && item.role !== "reference_" + type))
        throw new Error("Yuanliu supports ordinary reference roles, not first or last frames");
      const reference = item[item.type];
      if (!reference || typeof reference !== "object" || Array.isArray(reference) || Object.keys(reference).length !== 1)
        throw new Error("metadata.content references require a URL object");
      request[type + "s"].push(referenceURL(reference.url));
    }
  }
  if (texts.length) request.prompt = consistentValue([source.prompt, texts.join("\n")], "prompt", undefined);
  if (typeof request.prompt !== "string" || !request.prompt.trim() || Array.from(request.prompt).length > spec.prompt)
    throw new Error("prompt is required and must fit the model limit");
  for (const type of ["images", "videos", "audios"])
    if (request[type].length > spec[type]) throw new Error("too many reference " + type + " for Yuanliu model");
  if (spec.total && request.images.length + request.videos.length + request.audios.length > spec.total) throw new Error("too many total Yuanliu references");
  const mode = consistentValue([source.mode, metadata.mode], "mode", undefined);
  const hasReferences = request.images.length + request.videos.length + request.audios.length > 0;
  if (mode !== undefined && !["text_to_video", "image_to_video", "reference_to_video", "omni_reference"].includes(mode))
    throw new Error("unsupported Yuanliu video mode");
  if (
    (mode === "text_to_video" && hasReferences) ||
    (mode === "image_to_video" && (!request.images.length || request.videos.length || request.audios.length)) ||
    (["reference_to_video", "omni_reference"].includes(mode) && !hasReferences)
  )
    throw new Error("Yuanliu video mode conflicts with references");
  if (metadata.omni_reference_task_type === "reference" && !hasReferences) throw new Error("reference mode requires reference media");
  const mentions = /@(Image|Video|Audio)(\d+)/g;
  for (let match; (match = mentions.exec(request.prompt)) !== null; ) {
    const count = request[match[1].toLowerCase() + "s"].length;
    if (Number(match[2]) < 1 || Number(match[2]) > count) throw new Error("prompt reference index exceeds supplied media");
  }
  return request;
}

/** 构造唯一计费提交；幂等号使用宿主已生成的任务 ID，禁止发送后自动重发或切换渠道。 */
export function buildSubmitRequest(ctx) {
  if ((ctx.files || []).length) throw new Error("Yuanliu references require public URLs; file uploads are unsupported");
  if (ctx.action && !["text_to_video", "image_to_video", "reference_to_video"].includes(ctx.action)) throw new Error("unsupported Yuanliu action");
  const model = meta.modelAliases[ctx.upstreamModel || ctx.model] || ctx.upstreamModel || ctx.model;
  const body = normalizeRequest(ctx.requestBody, model);
  if (typeof ctx.publicTaskId !== "string" || !ctx.publicTaskId.trim() || ctx.publicTaskId.length > 100)
    throw new Error("gateway task ID is required for Yuanliu idempotency");
  body.model = model;
  body.client_request_id = ctx.publicTaskId;
  return {
    url: apiBase(ctx) + "/videos",
    method: "POST",
    headers: {
      Authorization: authorization(ctx),
      "Content-Type": "application/json",
    },
    body,
    noRetry: true,
  };
}

/** 接受新建和幂等命中的任务；持久化计费维度，终态命中直接交给宿主结算。 */
export function parseSubmitResponse(ctx, response) {
  const body = response.body;
  if (
    (response.statusCode !== undefined && (response.statusCode < 200 || response.statusCode >= 300)) ||
    !body ||
    typeof body !== "object" ||
    Array.isArray(body) ||
    (body.error && body.status !== "failed")
  )
    throw new Error("upstream rejected Yuanliu video creation");
  const taskId = body.id || body.task_id;
  if (typeof taskId !== "string" || !taskId.trim() || (body.id && body.task_id && body.id !== body.task_id)) throw new Error("Yuanliu task ID is invalid");
  if (!["queued", "in_progress", "unknown", "completed", "failed"].includes(body.status)) throw new Error("unrecognized Yuanliu submission status");
  const request = normalizeRequest(ctx.requestBody, ctx.upstreamModel || ctx.model);
  const parsed = {
    taskId,
    taskData: body,
    state: { duration: request.duration, resolution: request.resolution },
  };
  if (body.status === "completed" || body.status === "failed") parsed.immediate = parseTaskResult(ctx, body);
  return parsed;
}

/** 返回次数或整秒用量；不把目录人民币价格或 billing.charged 当成宿主用量。 */
export function extractUsage(ctx) {
  const request = normalizeRequest(ctx.requestBody, ctx.upstreamModel || ctx.model);
  if (ctx.usagePurpose === "billing_ratios") return {};
  return modelSpec(ctx.upstreamModel || ctx.model).billing === "per_second" ? { seconds: request.duration, resolution: request.resolution } : { count: 1 };
}

/** 查询同渠道上游任务，编码 ID 防止路径注入；不存在额外批量或取消接口。 */
export function buildQueryRequest(ctx) {
  return {
    url: apiBase(ctx) + "/videos/" + encodeURIComponent(ctx.taskId),
    method: "GET",
    headers: { Authorization: authorization(ctx) },
  };
}

/** 明确的 unknown 是供应商等待态；只有陌生响应才返回宿主轮询失败哨兵 UNKNOWN。 */
export function parseTaskResult(ctx, body) {
  if (!body || typeof body !== "object" || Array.isArray(body)) return { status: "UNKNOWN", reason: "invalid Yuanliu task response" };
  const statuses = {
    queued: "QUEUED",
    in_progress: "IN_PROGRESS",
    unknown: "IN_PROGRESS",
    completed: "SUCCESS",
    failed: "FAILURE",
  };
  if (!Object.prototype.hasOwnProperty.call(statuses, body.status)) return { status: "UNKNOWN", reason: "unrecognized Yuanliu task status" };
  const status = statuses[body.status];
  const result = { status };
  if (status === "SUCCESS") result.progress = "100%";
  else if (typeof body.progress === "number" && Number.isFinite(body.progress) && body.progress >= 0 && body.progress <= 100)
    result.progress = body.progress + "%";
  if (status === "FAILURE") {
    let reason = body.error && typeof body.error.message === "string" && body.error.message.trim() ? body.error.message : "Yuanliu video generation failed";
    if (ctx && ctx.apiKey) reason = reason.split(ctx.apiKey).join("[redacted]");
    result.reason = reason.replace(/Bearer\s+\S+|\bsk-[A-Za-z0-9_-]+/gi, "[redacted]");
  }
  return result;
}

/** 完成时只接受该模型合法的整秒与清晰度，缺失值从提交 state 恢复；失败不产生成功用量。 */
export function extractUsageOnComplete(task, result, body) {
  if (!result || result.status !== "SUCCESS") return null;
  const spec = modelSpec(task.upstreamModel || task.model);
  const data = body || {};
  const state = task.state || {};
  const duration = consistentValue(
    [data.seconds, data.duration].filter((value) => value !== undefined).map((value) => validDuration(value, spec)),
    "upstream duration",
    state.duration
  );
  if (duration !== undefined) validDuration(duration, spec);
  if (spec.billing === "per_call") return { count: 1 };
  if (duration === undefined) return null;
  const resolution = consistentValue(
    [data.resolution, data.size].map((value) => (typeof value === "string" ? value.toLowerCase() : value)),
    "upstream resolution",
    state.resolution
  );
  if (!spec.resolutions.includes(resolution)) throw new Error("unsupported upstream Yuanliu resolution");
  return { seconds: duration, resolution };
}

/** 只有成功任务声明视频制品；制品始终由宿主鉴权代理交付。 */
export function listArtifacts(task) {
  return task.status === "SUCCESS" ? [{ key: "video", type: "video", mimeType: "video/mp4" }] : [];
}

/** 内容仅访问同渠道 /content；HEAD 由宿主裁剪 GET 响应，避免假设上游 HEAD 合同或向 CDN 发送密钥。 */
export function buildContentRequest(ctx) {
  if (ctx.artifactKey !== "video") throw new Error("artifact_not_found");
  return {
    url: apiBase(ctx) + "/videos/" + encodeURIComponent(ctx.upstreamTaskId) + "/content",
    method: "GET",
    headers: { Authorization: authorization(ctx) },
  };
}

/** OpenAI Video 入口仅接收 URL 参考；显示模型保留客户端别名，驱动单独使用上游 ID。 */
export const protocols = {
  openai_video: {
    /** 解码 JSON 或无文件 multipart；复用规范化器以便候选预检和实际提交重复调用。 */
    decodeRequest(ctx) {
      if (!ctx.body || !["json", "multipart"].includes(ctx.body.kind)) throw new Error("JSON or multipart body required");
      let source = ctx.body.value;
      if (ctx.body.kind === "multipart") {
        if ((ctx.body.files || []).length) throw new Error("Yuanliu references require public URLs; file uploads are unsupported");
        source = {};
        for (const field of Object.keys(ctx.body.fields || {})) {
          const values = ctx.body.fields[field];
          if (!Array.isArray(values) || !values.length) throw new Error("invalid multipart field: " + field);
          if (["images", "videos", "audios"].includes(field)) {
            source[field] = values.length === 1 && /^\s*\[/.test(values[0]) ? JSON.parse(values[0]) : values.slice();
          } else {
            if (values.length !== 1) throw new Error(field + " must be provided once");
            source[field] = field === "metadata" || (field === "input_reference" && /^\s*\{/.test(values[0])) ? JSON.parse(values[0]) : values[0];
          }
        }
      }
      const model = ctx.model || (source && source.model);
      if (typeof model !== "string" || !model.trim()) throw new Error("model is required");
      if (source && source.model !== undefined && source.model !== model) throw new Error("model conflicts with pinned model");
      const requestBody = normalizeRequest(source, ctx.upstreamModel || model);
      requestBody.model = model;
      const action =
        requestBody.videos.length || requestBody.audios.length ? "reference_to_video" : requestBody.images.length ? "image_to_video" : "text_to_video";
      return { kind: "submit", model, action, requestBody };
    },
    /** 返回网关任务视图与已确认尺寸，不暴露上游计费币种、私有 ID 或短时签名下载地址。 */
    render(_ctx, task) {
      const statuses = {
        NOT_START: "queued",
        SUBMITTED: "queued",
        QUEUED: "queued",
        IN_PROGRESS: "in_progress",
        SUCCESS: "completed",
        FAILURE: "failed",
      };
      const data = task.data || {};
      const output = {
        id: task.task_id,
        object: "video",
        model: (task.properties || {}).origin_model_name || "",
        status: statuses[task.status] || "unknown",
      };
      if (task.status === "SUCCESS") {
        if (data.seconds !== undefined) output.seconds = data.seconds;
        if (data.size !== undefined) output.size = data.size;
        if (data.aspect_ratio !== undefined) output.aspect_ratio = data.aspect_ratio;
      }
      if (task.status === "FAILURE")
        output.error = {
          message: task.fail_reason || "Yuanliu video generation failed",
        };
      return output;
    },
  },
};
