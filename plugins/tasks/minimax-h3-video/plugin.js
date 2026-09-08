/** 第三方 OpenAI 视频兼容 MiniMax H3 插件；清晰度为渠道能力选项，不代表供应商保证支持。 */
export const meta = {
  apiVersion: 1,
  key: "minimax-h3-video",
  name: "MiniMax H3 (OpenAI compatible)",
  icon: "Hailuo.Color",
  description: {
    en: "Third-party OpenAI-compatible MiniMax H3 video generation, billed by seconds and model resolution. Not the native MiniMax API.",
    zh: "第三方 OpenAI 视频兼容 MiniMax H3，按秒和模型清晰度计费；不是 MiniMax 官方原生接口。",
  },
  version: "1.0.0",
  author: { name: "lysimportant/forknewapi" },
  models: ["minimax_h3-1080p", "minimax_h3-2K", "minimax_h3-768p"],
  fetchMode: "per_task",
  protocols: ["openai_video", { name: "openai_responses", supports: ["stream", "sync", "background"] }],
  usageSchema: {
    seconds: {
      type: "number",
      unit: "second",
      description: {
        en: "Video duration in seconds; seconds or duration is required, greater than zero and at most 3600.",
        zh: "视频时长（秒）；请求必须提供seconds或duration，大于0且不超过3600。",
      },
    },
    resolution: {
      enum: ["768P", "1080P", "2K"],
      description: {
        en: "Resolution follows the upstream model suffix: 768P, 1080P or 2K. Explicit resolution and size must match the selected model.",
        zh: "清晰度按上游模型后缀识别为768P、1080P或2K；显式resolution和size必须与所选模型一致。",
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
function validateRequest(req, model) {
  if (!req || typeof req !== "object" || Array.isArray(req)) throw new Error("request body must be an object");
  if (typeof req.prompt !== "string" || !req.prompt.trim()) throw new Error("prompt is required");
  if (req.seconds === undefined && req.duration === undefined) throw new Error("seconds or duration is required for per-second billing");
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
  // 计费参数只接受顶层值，防止透传对象另带时长、批量或清晰度绕过预扣校验。
  for (const container of ["metadata", "parameters"]) {
    if (req[container] === undefined) continue;
    let extra = req[container];
    if (typeof extra === "string") extra = JSON.parse(extra);
    if (!extra || typeof extra !== "object" || Array.isArray(extra)) throw new Error(container + " must be a JSON object");
    for (const key of ["seconds", "duration", "resolution", "size", "n", "count", "batch_size"]) {
      if (Object.prototype.hasOwnProperty.call(extra, key))
        throw new Error(container + "." + key + " is not supported; provide billing parameters at the top level");
    }
  }
  resolutionFact(req, model);
}

/** 从模型后缀确定计费清晰度；校验显式参数，避免模型档位与价格事实不一致。 */
function resolutionFact(req, model) {
  const resolutions = { "minimax_h3-1080p": "1080P", "minimax_h3-2K": "2K", "minimax_h3-768p": "768P" };
  const modelName = model || req.model;
  if (!Object.prototype.hasOwnProperty.call(resolutions, modelName)) throw new Error("unsupported MiniMax H3 compatible model");
  const expected = resolutions[modelName];
  for (const key of ["resolution", "size"]) {
    if (req[key] === undefined) continue;
    if (typeof req[key] !== "string") throw new Error(key + " must be a resolution label or pixel dimensions");
    const value = req[key].toUpperCase();
    let actual = value;
    if (!meta.usageSchema.resolution.enum.includes(value)) {
      const dimensions = /^(\d{1,4})X(\d{1,4})$/.exec(value);
      const shortSide = dimensions ? Math.min(Number(dimensions[1]), Number(dimensions[2])) : 0;
      actual = { 768: "768P", 1080: "1080P", 1440: "2K" }[shortSide];
    }
    if (actual !== expected) throw new Error(key + " conflicts with model resolution " + expected);
  }
  return expected;
}
/** 构造第三方 /videos 请求；有文件时复用宿主 multipart 文件引用，不读取本机文件。 */
export function buildSubmitRequest(ctx) {
  if (ctx.action && !["text_to_video", "image_to_video"].includes(ctx.action)) throw new Error("unsupported video action");
  const req = ctx.requestBody;
  validateRequest(req, ctx.upstreamModel || ctx.model);
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
  const taskId = body.id || body.task_id;
  if (typeof taskId !== "string" || !taskId.trim()) throw new Error("upstream video id is missing");
  return { taskId, taskData: body };
}

/** 报告已校验秒数与模型清晰度；旧倍率入口返回空对象，防止额外乘价。 */
export function extractUsage(ctx) {
  validateRequest(ctx.requestBody, ctx.upstreamModel || ctx.model);
  if (ctx.usagePurpose === "billing_ratios") return {};
  const req = ctx.requestBody;
  return { seconds: Number(req.seconds === undefined ? req.duration : req.seconds), resolution: resolutionFact(req, ctx.upstreamModel || ctx.model) };
}

/** 成功时按上游明确提供的有效秒数结算；缺失时保留预扣秒数，模型清晰度不变，错误秒数显式报错。 */
export function extractUsageOnComplete(_task, result, body) {
  if (!result || result.status !== "SUCCESS") return null;
  const response = body || {};
  if (response.seconds === undefined && response.duration === undefined) return null;
  for (const key of ["seconds", "duration"]) {
    const raw = response[key];
    if (raw === undefined) continue;
    if (
      (typeof raw !== "number" && typeof raw !== "string") ||
      String(raw).trim() === "" ||
      !Number.isFinite(Number(raw)) ||
      Number(raw) <= 0 ||
      Number(raw) > 3600
    )
      throw new Error("upstream " + key + " must be greater than 0 and at most 3600");
  }
  if (response.seconds !== undefined && response.duration !== undefined && Number(response.seconds) !== Number(response.duration))
    throw new Error("upstream seconds and duration conflict");
  return { seconds: Number(response.seconds === undefined ? response.duration : response.seconds) };
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
  const url = data.video_url || data.url || (data.video && data.video.url);
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
      validateRequest(req, ctx.upstreamModel || ctx.model);
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
      validateRequest(body, ctx.upstreamModel || ctx.model);
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
        metadata: { vendor: "minimax-h3-video" },
      };
    },
  },
};
