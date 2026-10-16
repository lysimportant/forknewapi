package globalstore

import "encoding/json"

// GlobalStore task status values as the upstream reports them (lowercase).
const (
	globalStoreStatusQueued     = "queued"
	globalStoreStatusProcessing = "processing"
	globalStoreStatusCompleted  = "completed"
	globalStoreStatusFailed     = "failed"
)

// GlobalStoreSubmitRequest is the body of POST {base}/v2/model-center/tasks.
//
// The field names mirror plugin.js upstreamBody() exactly: model, prompt,
// reference_images, size, aspect_ratio, resolution and quality. A request always
// carries the exact pixel size when the model's vendor page documents pixel sizes,
// together with the aspect ratio that size expresses; only the models that document
// no pixel size at all are steered by a resolution tier name. A field the model
// does not declare is never sent.
type GlobalStoreSubmitRequest struct {
	Model           string   `json:"model"`
	Prompt          string   `json:"prompt"`
	ReferenceImages []string `json:"reference_images,omitempty"`
	Size            string   `json:"size,omitempty"`
	AspectRatio     string   `json:"aspect_ratio,omitempty"`
	Resolution      string   `json:"resolution,omitempty"`
	Quality         string   `json:"quality,omitempty"`
}

// GlobalStoreSubmitResponse is the bare top-level JSON object the submit
// endpoint returns. The upstream task id lives in "id"; a failure may be
// reported in "error" as either a string or an object.
type GlobalStoreSubmitResponse struct {
	Id    string          `json:"id"`
	Error json.RawMessage `json:"error"`
}

// GlobalStoreTaskResponse is the bare top-level JSON object returned by
// GET {base}/v2/model-center/tasks/{id}.
//
// status is one of queued|processing|completed|failed. On success the generated
// image URL is reported in image_url, with result_url as the fallback the plugin
// also accepts. The video-only fields the upstream may emit (video_url, object)
// are deliberately not modeled: this channel serves image generation only.
type GlobalStoreTaskResponse struct {
	Id       string          `json:"id"`
	Status   string          `json:"status"`
	Progress json.RawMessage `json:"progress"`
	Error    json.RawMessage `json:"error"`
	ImageURL string          `json:"image_url"`
	// ResultURL is the generic fallback result address plugin.js resultURL()
	// accepts when no media-specific field is set.
	ResultURL string `json:"result_url"`
}
