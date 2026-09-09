/** 第三方 OpenAI 视频兼容 Grok 插件；清晰度为渠道能力选项，不代表供应商保证支持。 */
export const meta = {
  apiVersion: 1,
  key: "grok-video",
  name: "Grok Video (OpenAI compatible)",
  icon: "Grok.Color",
  description: {
    en: "Third-party OpenAI-compatible Grok video generation, billed per request. Resolution support depends on the provider; not the native xAI API.",
    zh: "第三方 OpenAI 视频兼容 Grok，按次计费。清晰度需上游支持；不是 xAI 官方原生接口。",
  },
  version: "1.1.1",
  author: { name: "lysimportant/forknewapi" },
  models: ["grok-imagine-video-1.5", "grok-imagine-video-1.5.1", "grok-imagine-video"],
  fetchMode: "per_task",
  protocols: ["openai_video", { name: "openai_responses", supports: ["stream", "sync", "background"] }],
  usageSchema: {
    count: {
      type: "number",
      unit: "count",
      description: { en: "One video generation request, independent of duration.", zh: "每次视频生成计为一次，不按时长相乘。" },
    },
    resolution: {
      enum: ["unspecified", "480p", "720p", "1080p", "4k"],
      description: {
        en: "Shared pricing options: 480p, 720p and 1080p; availability depends on the upstream model. unspecified preserves the provider default; 4k is retained only for existing third-party configurations, not documented xAI support.",
        zh: "共享定价选项为480p、720p、1080p，实际可用档位取决于上游模型。unspecified 保留上游默认；4k 仅兼容既有第三方配置，不表示 xAI 官方支持。",
      },
    },
  },
};

/** 规范化根地址或 /v1 地址；不改变供应商自定义路径前缀。 */
function apiBase(ctx) {
  const base = String(ctx.baseUrl || "").replace(/\/+$/, "");
  if (!/^https?:\/\/[^/?#]+(?:\/[^?#]*)?$/.test(base)) throw new Error("channel base URL must be an HTTP(S) API base without query or fragment");
  return /\/v1$/.test(base) ? base : base + "/v1";
}

/** 校验共享生成参数，保留显式值；3600 秒只是宿主安全上限，供应商可有更严格限制。 */
function validateRequest(req) {
  if (!req || typeof req !== "object" || Array.isArray(req)) throw new Error("request body must be an object");
  if (typeof req.prompt !== "string" || !req.prompt.trim()) throw new Error("prompt is required");
  for (const key of ["seconds", "duration"]) {
    if (req[key] !== undefined) {
      const value = req[key];
      if (
        (typeof value !== "string" && typeof value !== "number") ||
        String(value).trim() === "" ||
        !Number.isFinite(Number(value)) ||
        Number(value) <= 0 ||
        Number(value) > 3600
      )
        throw new Error(key + " must be greater than 0 and at most 3600");
    }
  }
  if (req.seconds !== undefined && req.duration !== undefined && Number(req.seconds) !== Number(req.duration)) throw new Error("seconds and duration conflict");
  for (const key of ["n", "count", "batch_size"]) {
    if (req[key] !== undefined && ![1, "1"].includes(req[key])) throw new Error("only one video per request is supported");
  }
  if (req.resolution !== undefined && (typeof req.resolution !== "string" || !meta.usageSchema.resolution.enum.slice(1).includes(req.resolution.toLowerCase())))
    throw new Error("resolution must be 480p, 720p or 1080p; 4k is accepted only for legacy third-party compatibility");
  resolutionFact(req);
}

/** 统一清晰度和像素尺寸的计费档，拒绝冲突及未知尺寸，防止低档计费而向上游请求高档。 */
function resolutionFact(req) {
  let sizeResolution;
  if (req.size !== undefined) {
    if (typeof req.size !== "string") throw new Error("size must be a resolution label or pixel dimensions");
    const size = req.size.toLowerCase();
    if (meta.usageSchema.resolution.enum.slice(1).includes(size)) sizeResolution = size;
    else {
      const dimensions = /^(\d{1,4})x(\d{1,4})$/.exec(size);
      const shortSide = dimensions ? Math.min(Number(dimensions[1]), Number(dimensions[2])) : 0;
      sizeResolution = { 480: "480p", 720: "720p", 1080: "1080p", 2160: "4k" }[shortSide];
      if (!sizeResolution) throw new Error("size has an unsupported resolution");
    }
  }
  const resolution = req.resolution === undefined ? sizeResolution : String(req.resolution).toLowerCase();
  if (sizeResolution && sizeResolution !== resolution) throw new Error("resolution and size conflict");
  return resolution || "unspecified";
}

/** 构造第三方 /videos 请求；有文件时复用宿主 multipart 文件引用，不读取本机文件。 */
export function buildSubmitRequest(ctx) {
  if (ctx.action && !["text_to_video", "image_to_video"].includes(ctx.action)) throw new Error("unsupported video action");
  const req = ctx.requestBody;
  validateRequest(req);
  const body = Object.assign({}, req, { model: ctx.upstreamModel });
  const headers = { Authorization: "Bearer " + ctx.apiKey };
  const descriptor = { url: apiBase(ctx) + "/videos", method: "POST", headers };
  if ((ctx.files || []).length) {
    const parts = [];
    for (const key of Object.keys(body)) {
      if (body[key] !== undefined && body[key] !== null)
        parts.push({ name: key, value: typeof body[key] === "object" ? JSON.stringify(body[key]) : body[key] });
    }
    for (const file of ctx.files) {
      if (file.field !== "input_reference") throw new Error("only input_reference file is supported");
      parts.push({ name: file.field, fileRef: file.ref, filename: file.filename });
    }
    return Object.assign(descriptor, { bodyType: "multipart", parts });
  }
  headers["Content-Type"] = "application/json";
  return Object.assign(descriptor, { body });
}

/** 提取标准顶层任务 ID；不把错误响应或不明包装格式视为提交成功。 */
export function parseSubmitResponse(_ctx, resp) {
  const body = resp.body || {};
  if (body.error) throw new Error("upstream rejected video creation");
  const taskId = body.request_id || body.id || body.task_id;
  if (typeof taskId !== "string" || !taskId.trim()) throw new Error("upstream video id is missing");
  return { taskId, taskData: body };
}

/** 按次报告用量；旧倍率入口返回空对象，防止时长或清晰度重复乘价。 */
export function extractUsage(ctx) {
  validateRequest(ctx.requestBody);
  if (ctx.usagePurpose === "billing_ratios") return {};
  return { count: 1, resolution: resolutionFact(ctx.requestBody) };
}

/** 成功仍只计一次；不使用上游时长或任意计数字段改变收费次数，保留请求清晰度分档。 */
export function extractUsageOnComplete(_task, result) {
  return result && result.status === "SUCCESS" ? { count: 1 } : null;
}

/** 查询上游任务；路径参数编码，避免任务 ID 改写请求路径。 */
export function buildQueryRequest(ctx) {
  return { url: apiBase(ctx) + "/videos/" + encodeURIComponent(ctx.taskId), method: "GET", headers: { Authorization: "Bearer " + ctx.apiKey } };
}

/** 明确映射 OpenAI 视频状态；未知响应不能默认为处理中或成功。 */
export function parseTaskResult(_ctx, body) {
  if (!body || typeof body !== "object" || Array.isArray(body)) return { status: "UNKNOWN", reason: "invalid video response" };
  const statuses = {
    queued: "QUEUED",
    pending: "QUEUED",
    processing: "IN_PROGRESS",
    in_progress: "IN_PROGRESS",
    completed: "SUCCESS",
    done: "SUCCESS",
    failed: "FAILURE",
    cancelled: "FAILURE",
    canceled: "FAILURE",
    expired: "FAILURE",
  };
  const status = body.error ? "FAILURE" : statuses[body.status];
  if (!status) return { status: "UNKNOWN", reason: "unrecognized video status" };
  const result = { status };
  if (status === "FAILURE") result.reason = "upstream video generation failed";
  if (status === "SUCCESS") result.progress = "100%";
  else if (typeof body.progress === "number" && Number.isFinite(body.progress) && body.progress >= 0 && body.progress < 100)
    result.progress = body.progress + "%";
  return result;
}

/** 成功后声明视频制品；下载通过宿主受控代理，不向客户端暴露渠道密钥。 */
export function listArtifacts(task) {
  return task.status === "SUCCESS" ? [{ key: "video", type: "video" }] : [];
}

/** 标准接口使用鉴权 content 路径；明确返回的 CDN 地址采用无凭据下载。 */
export function buildContentRequest(ctx) {
  if (ctx.artifactKey !== "video") throw new Error("artifact_not_found");
  let data = ctx.data || {};
  if (data.data && data.data.task_id && data.data.data) data = data.data.data;
  const url = data.video_url || data.url || (data.video && data.video.url) || (data.data && data.data.video && data.data.video.url);
  if (url !== undefined) {
    if (typeof url !== "string" || !/^https?:\/\//.test(url)) throw new Error("invalid video artifact URL");
    return { url, method: ctx.clientRequest.method, credentialless: true };
  }
  return {
    url: apiBase(ctx) + "/videos/" + encodeURIComponent(ctx.upstreamTaskId) + "/content",
    method: ctx.clientRequest.method,
    headers: { Authorization: "Bearer " + ctx.apiKey },
  };
}

/** 生成宿主制品的 HTML 视频输出；地址来自宿主授权后的制品链接。 */
function videoOutput(ctx) {
  const artifact = ctx.artifacts && ctx.artifacts.video;
  if (!artifact || !artifact.url) throw new Error("video artifact is unavailable");
  const escaped = String(artifact.url).replace(/&/g, "&amp;").replace(/"/g, "&quot;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  return '<video controls src="' + escaped + '"></video>';
}

/** 注册 OpenAI 视频及纯文本 Responses 生成入口，沿用宿主任务状态、鉴权和结算。 */
export const protocols = {
  openai_video: {
    /** 解码 JSON 或 multipart；校验生成数量和时长，不猜测供应商默认参数。 */
    decodeRequest: function (ctx) {
      let req;
      if (ctx.body && ctx.body.kind === "json") {
        req = ctx.body.value;
      } else if (ctx.body && ctx.body.kind === "multipart") {
        req = {};
        for (const key of Object.keys(ctx.body.fields || {})) {
          const values = ctx.body.fields[key];
          if (values.length !== 1) throw new Error(key + " must be provided once");
          req[key] = values[0];
        }
        const files = ctx.body.files || [];
        if (
          files.length > 1 ||
          files.some(function (file) {
            return file.field !== "input_reference";
          })
        )
          throw new Error("only one input_reference file is supported");
      } else throw new Error("JSON or multipart body required");
      validateRequest(req);
      return {
        kind: "submit",
        model: ctx.model,
        action: req.input_reference || req.image || (ctx.body.files || []).length ? "image_to_video" : "text_to_video",
        requestBody: Object.assign({}, req, { model: ctx.model }),
      };
    },
    /** 返回标准视频任务表示；宿主负责写入最终公共状态与生命周期字段。 */
    render: function (_ctx, task) {
      const statuses = { NOT_START: "queued", SUBMITTED: "queued", QUEUED: "queued", IN_PROGRESS: "in_progress", SUCCESS: "completed", FAILURE: "failed" };
      return { id: task.task_id, object: "video", model: (task.properties || {}).origin_model_name || "", status: statuses[task.status] || "unknown" };
    },
  },
  openai_responses: {
    /** 仅接受单轮文本 input；视频专用参数显式复制，不静默丢弃工具或历史消息。 */
    decodeRequest: function (ctx) {
      const req = ctx.body && ctx.body.kind === "json" && ctx.body.value;
      if (!req || typeof req !== "object" || Array.isArray(req) || typeof req.input !== "string")
        throw new Error("video Responses requires a text input string");
      for (const key of ["tools", "tool_choice", "previous_response_id", "conversation", "instructions", "images", "image", "input_reference"]) {
        if (req[key] !== undefined && req[key] !== null) throw new Error("video Responses does not support " + key);
      }
      const body = { model: ctx.model, prompt: req.input };
      for (const key of ["seconds", "duration", "resolution", "size", "aspect_ratio", "n", "count", "batch_size"]) {
        if (req[key] !== undefined) body[key] = req[key];
      }
      validateRequest(body);
      return { kind: "submit", model: ctx.model, action: "text_to_video", requestBody: body };
    },
    /** 输出进度与唯一终态；失败事件交由宿主完成标准 Responses 封装。 */
    renderEvents: function (ctx, task, previousState) {
      const status = task.status || "UNKNOWN";
      const state = { status, progress: task.progress || "" };
      if (status === "SUCCESS")
        return { events: previousState && previousState.status === status ? [] : [{ type: "output", data: videoOutput(ctx) }], state, done: true };
      if (status === "FAILURE") return { events: [{ type: "error", code: "video_generation_failed", message: "Video generation failed" }], state, done: true };
      return {
        events:
          previousState && previousState.status === status && previousState.progress === state.progress
            ? []
            : [{ type: "progress", message: status.toLowerCase() }],
        state,
        done: false,
      };
    },
    /** 返回成功的纯视频消息；不直接输出供应商地址或密钥。 */
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
        metadata: { vendor: "grok-video" },
      };
    },
  },
};

