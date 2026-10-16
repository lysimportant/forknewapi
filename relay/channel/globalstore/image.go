package globalstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// Polling budget, mirroring ali's asyncTaskWait shape with the defects fixed:
// the whole wait is bounded by a context deadline, the number of consecutive
// transport/HTTP failures is capped, a non-2xx poll response is an error rather
// than a task, and an empty or unrecognized status is an error rather than an
// empty success. 5s + 19*10s = 195s of sleeping inside a 210s (3.5 min) budget.
const (
	globalStorePollTimeout              = 210 * time.Second
	globalStoreFirstPollDelay           = 5 * time.Second
	globalStorePollInterval             = 10 * time.Second
	globalStorePollMaxAttempts          = 20
	globalStorePollMaxConsecutiveErrors = 3
)

// globalStorePollPolicy bounds one asynchronous wait. The production values live
// in globalStoreDefaultPollPolicy; tests shrink them so the polling semantics can
// be exercised without real-time sleeps.
type globalStorePollPolicy struct {
	timeout              time.Duration
	firstDelay           time.Duration
	interval             time.Duration
	maxAttempts          int
	maxConsecutiveErrors int
}

// globalStoreDefaultPollPolicy keeps the documented 3.5 minute budget: 5s first
// poll, then 19 further 10s polls, all inside a 210s context deadline.
var globalStoreDefaultPollPolicy = globalStorePollPolicy{
	timeout:              globalStorePollTimeout,
	firstDelay:           globalStoreFirstPollDelay,
	interval:             globalStorePollInterval,
	maxAttempts:          globalStorePollMaxAttempts,
	maxConsecutiveErrors: globalStorePollMaxConsecutiveErrors,
}

// globalStoreMaxMediaItems mirrors plugin.js MAX_MEDIA_ITEMS.
const globalStoreMaxMediaItems = 60

// globalStoreHTTPURLPattern mirrors plugin.js isHTTPURL(): the upstream only
// reads publicly reachable HTTP(S) media, so anything else is rejected here.
var globalStoreHTTPURLPattern = regexp.MustCompile(`(?i)^https?://\S+$`)

// globalStoreSizePattern mirrors the /^\d+x\d+$/ check of plugin.js.
var globalStoreSizePattern = regexp.MustCompile(`^\d+x\d+$`)

// Size resolution tuning. A documented ratio must match the requested pixel ratio
// within this relative tolerance, the ratio error outweighs the long-edge distance
// when both are known, and the epsilon keeps "the first documented entry wins"
// deterministic.
const (
	globalStoreRatioTolerance = 0.02
	globalStoreRatioWeight    = 4.0
	globalStoreScoreEpsilon   = 1e-9
)

// globalStoreBaseURL normalizes the configured channel base URL. Only trailing
// slashes are removed; a configured API prefix is preserved because the operator
// set it explicitly.
func globalStoreBaseURL(baseURL string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/")
}

// globalStoreUpstreamModel resolves the model name to send upstream, applying
// plugin.js UPSTREAM_MODEL_NAMES.
func globalStoreUpstreamModel(model string) string {
	if upstream, ok := upstreamModelNames[model]; ok {
		return upstream
	}
	return model
}

// globalStoreExtraString reads an OpenAI image parameter that dto.ImageRequest
// has no dedicated field for; the ingress keeps unknown JSON keys in Extra.
func globalStoreExtraString(request dto.ImageRequest, key string) (string, error) {
	raw, ok := request.Extra[key]
	if !ok {
		return "", nil
	}
	var value string
	if err := common.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("%s must be a string", key)
	}
	return strings.TrimSpace(value), nil
}

// normalizeSpecValue matches a value case-insensitively and returns the exact
// spelling the model declares (plugin.js normalizeResolution).
func normalizeSpecValue(value string, supported []string, field string) (string, error) {
	trimmedValue := strings.TrimSpace(value)
	for _, candidate := range supported {
		if strings.EqualFold(candidate, trimmedValue) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%s must be one of %s", field, strings.Join(supported, ", "))
}

// normalizeSpecLiteral matches a value exactly (plugin.js normalizeAspectRatio
// is case-sensitive) and returns it unchanged.
func normalizeSpecLiteral(value string, supported []string, field string) (string, error) {
	trimmedValue := strings.TrimSpace(value)
	if slices.Contains(supported, trimmedValue) {
		return trimmedValue, nil
	}
	return "", fmt.Errorf("%s must be one of %s", field, strings.Join(supported, ", "))
}

// globalStoreDeclaredValue returns the documented spelling of value when the
// model declares it, or "" when it does not.
func globalStoreDeclaredValue(supported []string, value string) string {
	for _, candidate := range supported {
		if strings.EqualFold(candidate, value) {
			return candidate
		}
	}
	return ""
}

// globalStoreSizeInput is the resolved size contract of one submit body. At most
// one of size and resolution is set: size for the models whose vendor page
// documents pixel sizes, resolution only for the models that document nothing but
// a tier name.
type globalStoreSizeInput struct {
	size       string
	ratio      string
	resolution string
}

// globalStoreTierNames returns the documented tiers of one size table in a stable
// order so that equal scores resolve deterministically.
func globalStoreTierNames(sizeTiers map[string][]string) []string {
	names := make([]string, 0, len(sizeTiers))
	for name := range sizeTiers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// globalStoreDocumentedSize reports whether size is one of the documented pixels
// of the model; the documented spelling is returned.
func globalStoreDocumentedSize(sizeTiers map[string][]string, size string) (string, bool) {
	if size == "" {
		return "", false
	}
	for _, name := range globalStoreTierNames(sizeTiers) {
		for _, candidate := range sizeTiers[name] {
			if strings.EqualFold(candidate, size) {
				return candidate, true
			}
		}
	}
	return "", false
}

// globalStoreSizeDimensions parses a WxH size into positive pixel counts.
func globalStoreSizeDimensions(size string) (int, int, bool) {
	parts := strings.SplitN(strings.ToLower(strings.TrimSpace(size)), "x", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	width, errWidth := strconv.Atoi(strings.TrimSpace(parts[0]))
	height, errHeight := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errWidth != nil || errHeight != nil || width <= 0 || height <= 0 {
		return 0, 0, false
	}
	return width, height, true
}

// globalStoreRatioValue parses a "horizontal:vertical" ratio into its value.
func globalStoreRatioValue(ratio string) (float64, bool) {
	parts := strings.SplitN(strings.TrimSpace(ratio), ":", 2)
	if len(parts) != 2 {
		return 0, false
	}
	horizontal, errHorizontal := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	vertical, errVertical := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if errHorizontal != nil || errVertical != nil || horizontal <= 0 || vertical <= 0 {
		return 0, false
	}
	return horizontal / vertical, true
}

// globalStoreSizeRatio returns the pixel ratio of a WxH size.
func globalStoreSizeRatio(size string) (float64, bool) {
	width, height, ok := globalStoreSizeDimensions(size)
	if !ok {
		return 0, false
	}
	return float64(width) / float64(height), true
}

// globalStoreSizeLongEdge returns the longer edge of a WxH size.
func globalStoreSizeLongEdge(size string) (int, bool) {
	width, height, ok := globalStoreSizeDimensions(size)
	if !ok {
		return 0, false
	}
	return max(width, height), true
}

// globalStoreRatioForSize returns the declared ratio whose value matches the pixel
// ratio of size within the documented tolerance, or "" when none does.
func globalStoreRatioForSize(ratios []string, size string) string {
	want, ok := globalStoreSizeRatio(size)
	if !ok {
		return ""
	}
	best, bestError := "", 0.0
	for _, candidate := range ratios {
		value, ok := globalStoreRatioValue(candidate)
		if !ok {
			continue
		}
		relativeError := math.Abs(value-want) / want
		if relativeError > globalStoreRatioTolerance {
			continue
		}
		if best == "" || relativeError < bestError {
			best, bestError = candidate, relativeError
		}
	}
	return best
}

// globalStoreTierForSize returns the declared tier whose documented long edge is
// closest, in log space, to the longer edge of size. It maps a pixel request back
// onto the tier name of a model whose vendor page accepts no pixel size at all.
func globalStoreTierForSize(tiers []string, size string) string {
	longEdge, ok := globalStoreSizeLongEdge(size)
	if !ok {
		return ""
	}
	best, bestDistance := "", 0.0
	for _, candidate := range tiers {
		edge, ok := tierLongEdges[strings.ToLower(candidate)]
		if !ok {
			continue
		}
		distance := math.Abs(math.Log(float64(longEdge) / float64(edge)))
		if best == "" || distance < bestDistance {
			best, bestDistance = candidate, distance
		}
	}
	return best
}

// globalStoreEnumeratedSize maps the caller's intent onto the documented pixel
// enumeration. A documented size is kept verbatim; every other request picks the
// documented size whose pixel ratio is closest, with the long-edge distance as the
// tie breaker so the requested tier is preserved. The returned ratio is the
// declared ratio that matches the chosen size, or "" when none does.
func globalStoreEnumeratedSize(spec modelSpec, requested, tier, ratio string) (string, string, bool) {
	if documented, ok := globalStoreDocumentedSize(spec.sizeTiers, requested); ok {
		return documented, globalStoreRatioForSize(spec.ratios, documented), true
	}
	if requested == "" && tier == "" && ratio == "" {
		return spec.defaultSize, globalStoreRatioForSize(spec.ratios, spec.defaultSize), true
	}

	want, hasRatio := globalStoreRatioValue(ratio)
	if !hasRatio {
		want, hasRatio = globalStoreSizeRatio(requested)
	}
	// Without an explicit size the documented default is the reference the long
	// edge is compared against, which keeps a ratio-only request on the tier of
	// the documented default.
	reference := requested
	if reference == "" {
		reference = spec.defaultSize
	}
	longEdge, hasEdge := globalStoreSizeLongEdge(reference)

	best, bestRatio, bestScore := "", "", 0.0
	for _, name := range globalStoreTierNames(spec.sizeTiers) {
		if tier != "" && !strings.EqualFold(name, tier) {
			continue
		}
		for _, candidate := range spec.sizeTiers[name] {
			score := 0.0
			if hasRatio && want > 0 {
				if value, ok := globalStoreSizeRatio(candidate); ok {
					score += math.Abs(value-want) / want * globalStoreRatioWeight
				}
			}
			if hasEdge {
				if edge, ok := globalStoreSizeLongEdge(candidate); ok {
					score += math.Abs(math.Log(float64(edge) / float64(longEdge)))
				}
			}
			if best == "" || score < bestScore-globalStoreScoreEpsilon {
				best, bestScore = candidate, score
				bestRatio = globalStoreRatioForSize(spec.ratios, candidate)
			}
		}
	}
	return best, bestRatio, best != ""
}

// globalStoreResolveSizeInput turns the caller's pixel size, tier name and ratio
// hints into the upstream size contract. The upstream body never carries a tier
// name while the model's vendor page documents pixel sizes for it.
func globalStoreResolveSizeInput(spec modelSpec, requested, tier, ratio string) (globalStoreSizeInput, error) {
	if spec.acceptsSize && len(spec.sizeTiers) > 0 {
		size, matchedRatio, ok := globalStoreEnumeratedSize(spec, requested, tier, ratio)
		if !ok {
			return globalStoreSizeInput{}, fmt.Errorf(
				"size %q cannot be expressed by the documented sizes of this model", requested)
		}
		return globalStoreSizeInput{size: size, ratio: matchedRatio}, nil
	}
	if spec.acceptsSize && requested != "" {
		// The vendor page documents a size field without an enumeration, so the
		// requested pixels are forwarded exactly as they were asked for.
		return globalStoreSizeInput{
			size:  requested,
			ratio: globalStoreRatioForSize(spec.ratios, requested),
		}, nil
	}

	// The model documents no usable pixel size for this request, so the tier name
	// is the only remaining upstream control.
	tierHint := tier
	if tierHint == "" && requested != "" {
		tierHint = globalStoreTierForSize(spec.resolutions, requested)
	}
	if tierHint == "" {
		tierHint = spec.resolutions[0]
	}
	normalizedTier, err := normalizeSpecValue(tierHint, spec.resolutions, "resolution")
	if err != nil {
		return globalStoreSizeInput{}, err
	}

	normalizedRatio := ratio
	if normalizedRatio == "" && requested != "" {
		// A tier-only model cannot express an aspect ratio it does not declare,
		// and silently substituting another one would change the requested frame.
		normalizedRatio = globalStoreRatioForSize(spec.ratios, requested)
		if normalizedRatio == "" {
			return globalStoreSizeInput{}, fmt.Errorf(
				"aspect ratio of size %q is not supported by this model", requested)
		}
	}
	if normalizedRatio == "" {
		normalizedRatio = spec.defaultRatio
	}
	if normalizedRatio == "" {
		normalizedRatio = "1:1"
	}
	if normalizedRatio, err = normalizeSpecLiteral(normalizedRatio, spec.ratios, "aspect_ratio"); err != nil {
		return globalStoreSizeInput{}, err
	}
	return globalStoreSizeInput{ratio: normalizedRatio, resolution: normalizedTier}, nil
}

// oaiImage2GlobalStoreRequest converts an OpenAI dto.ImageRequest into the
// GlobalStore submit body.
//
// The upstream body field names and the per-model defaults come from plugin.js
// upstreamBody()/normalizeImageRequest(); the plugin's validation is mirrored so
// an unsupported value fails locally instead of being rejected by the upstream
// after the task has already been billed.
func oaiImage2GlobalStoreRequest(info *relaycommon.RelayInfo, request dto.ImageRequest) (*GlobalStoreSubmitRequest, error) {
	// GetUpstreamModelName is nil-safe; the promoted field would panic when a
	// caller builds a RelayInfo without channel metadata.
	model := strings.TrimSpace(info.GetUpstreamModelName())
	if model == "" {
		model = strings.TrimSpace(request.Model)
	}
	spec, ok := modelSpecs[model]
	if !ok {
		return nil, fmt.Errorf("unsupported GlobalStore image model: %s", model)
	}

	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" {
		return nil, errors.New("prompt is required")
	}
	if request.N != nil && *request.N > 1 {
		return nil, errors.New("n greater than 1 is not supported by GlobalStore image models")
	}
	if responseFormat := strings.TrimSpace(request.ResponseFormat); responseFormat != "" && responseFormat != "url" {
		return nil, fmt.Errorf("response_format must be url for GlobalStore image models, got %q", responseFormat)
	}

	references, err := globalStoreReferenceImages(request)
	if err != nil {
		return nil, err
	}
	if len(references) > spec.maxReferences {
		return nil, fmt.Errorf("reference_images cannot exceed %d for %s", spec.maxReferences, model)
	}

	body := &GlobalStoreSubmitRequest{
		Model:  globalStoreUpstreamModel(model),
		Prompt: prompt,
	}
	if len(references) > 0 {
		body.ReferenceImages = references
	}

	size := strings.ToLower(strings.TrimSpace(request.Size))
	autoSize := size == "auto"
	if autoSize {
		size = ""
	}
	if size != "" && !globalStoreSizePattern.MatchString(size) {
		return nil, fmt.Errorf("size must be WxH, got %q", size)
	}
	resolution, err := globalStoreExtraString(request, "resolution")
	if err != nil {
		return nil, err
	}
	aspectRatio, err := globalStoreExtraString(request, "aspect_ratio")
	if err != nil {
		return nil, err
	}
	ratioAlias, err := globalStoreExtraString(request, "ratio")
	if err != nil {
		return nil, err
	}
	if aspectRatio != "" && ratioAlias != "" && aspectRatio != ratioAlias {
		return nil, errors.New("aspect_ratio conflicts with ratio")
	}

	// The tier name is an input hint only: the upstream body carries the exact
	// pixels the vendor documents for the resolved tier and ratio.
	tier := ""
	if resolution != "" {
		normalizedTier, err := normalizeSpecValue(resolution, spec.resolutions, "resolution")
		if err != nil {
			return nil, err
		}
		tier = strings.ToLower(normalizedTier)
	}
	ratio := strings.TrimSpace(aspectRatio)
	if ratio == "" {
		ratio = strings.TrimSpace(ratioAlias)
	}
	autoRatio := strings.EqualFold(ratio, "auto")
	if autoRatio {
		ratio = ""
	}
	if ratio != "" {
		normalizedRatio, err := normalizeSpecLiteral(ratio, spec.ratios, "aspect_ratio")
		if err != nil {
			return nil, err
		}
		ratio = normalizedRatio
	}

	// "auto" asks the upstream to frame the image. Only the models that document it
	// as an aspect ratio get that intent; the others fall back to their documented
	// default geometry rather than a silently substituted ratio.
	autoRatioValue := ""
	if autoSize || autoRatio {
		autoRatioValue = globalStoreDeclaredValue(spec.ratios, "auto")
	}
	switch {
	case autoRatioValue != "":
		body.AspectRatio = autoRatioValue
		if len(spec.sizeTiers) == 0 && len(spec.resolutions) > 0 {
			// A model without a pixel size contract must still name a tier.
			order := tier
			if order == "" {
				order = spec.resolutions[0]
			}
			normalizedTier, err := normalizeSpecValue(order, spec.resolutions, "resolution")
			if err != nil {
				return nil, err
			}
			// The documented spelling of the tier is what the vendor page lists.
			body.Resolution = normalizedTier
		}
	default:
		sizeInput, err := globalStoreResolveSizeInput(spec, size, tier, ratio)
		if err != nil {
			return nil, err
		}
		body.Size = sizeInput.size
		body.AspectRatio = sizeInput.ratio
		body.Resolution = sizeInput.resolution
	}

	// quality is only part of the contract for the models that declare it.
	if len(spec.qualities) > 0 {
		quality := strings.TrimSpace(request.Quality)
		if quality == "" {
			quality = spec.qualities[0]
		}
		normalizedQuality, err := normalizeSpecValue(quality, spec.qualities, "quality")
		if err != nil {
			return nil, err
		}
		body.Quality = strings.ToLower(normalizedQuality)
	}

	return body, nil
}

// globalStoreReferenceImages collects the public reference image URLs from the
// OpenAI image edit fields (image / images), mirroring plugin.js collectInputs().
func globalStoreReferenceImages(request dto.ImageRequest) ([]string, error) {
	var references []string
	for _, raw := range []json.RawMessage{request.Image, request.Images} {
		urls, err := globalStoreMediaURLs(raw)
		if err != nil {
			return nil, err
		}
		references = append(references, urls...)
	}
	return references, nil
}

// globalStoreMediaURLs mirrors plugin.js mediaList(): a single value, an array,
// or a JSON-encoded array of URL strings or URL-carrying objects.
func globalStoreMediaURLs(raw json.RawMessage) ([]string, error) {
	trimmedRaw := strings.TrimSpace(string(raw))
	if trimmedRaw == "" || trimmedRaw == "null" {
		return nil, nil
	}

	var value any
	if err := common.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("reference images must be public HTTP(S) URLs: %w", err)
	}
	if text, ok := value.(string); ok && strings.HasPrefix(strings.TrimSpace(text), "[") {
		var decoded any
		if err := common.Unmarshal([]byte(strings.TrimSpace(text)), &decoded); err != nil {
			return nil, errors.New("reference images must be a JSON array")
		}
		value = decoded
	}

	items, ok := value.([]any)
	if !ok {
		items = []any{value}
	}
	if len(items) > globalStoreMaxMediaItems {
		return nil, fmt.Errorf("reference images cannot exceed %d items", globalStoreMaxMediaItems)
	}

	urls := make([]string, 0, len(items))
	for _, item := range items {
		url, err := globalStoreMediaURL(item)
		if err != nil {
			return nil, err
		}
		urls = append(urls, url)
	}
	return urls, nil
}

// globalStoreMediaURL mirrors plugin.js mediaURL(): it accepts a plain URL
// string, a {image_url} / {url} object, or a nested {url:{url}} object.
func globalStoreMediaURL(value any) (string, error) {
	var url string
	switch typed := value.(type) {
	case string:
		url = typed
	case map[string]any:
		raw, ok := typed["image_url"]
		if !ok {
			raw = typed["url"]
		}
		switch inner := raw.(type) {
		case string:
			url = inner
		case map[string]any:
			if nested, ok := inner["url"].(string); ok {
				url = nested
			}
		}
	}

	url = strings.TrimSpace(url)
	if !globalStoreHTTPURLPattern.MatchString(url) {
		return "", errors.New("reference images must be public HTTP(S) URLs; file uploads are unsupported")
	}
	return url, nil
}

// upstreamErrorText mirrors plugin.js errorMessage(): the upstream error field
// is either a plain string or an object carrying message/code.
func upstreamErrorText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := common.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text)
	}
	var object struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	}
	if err := common.Unmarshal(raw, &object); err != nil {
		return ""
	}
	if message := strings.TrimSpace(object.Message); message != "" {
		return message
	}
	return strings.TrimSpace(object.Code)
}

// globalStoreResultURL mirrors plugin.js resultURL(): the image-specific field
// wins, result_url is the fallback.
func globalStoreResultURL(task *GlobalStoreTaskResponse) string {
	if url := strings.TrimSpace(task.ImageURL); globalStoreHTTPURLPattern.MatchString(url) {
		return url
	}
	if url := strings.TrimSpace(task.ResultURL); globalStoreHTTPURLPattern.MatchString(url) {
		return url
	}
	return ""
}

// sleepWithContext waits for delay, or returns early once ctx is done so the
// polling budget cannot be overshot by a long sleep.
func sleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// globalStoreFetchTask performs one task-status query. A non-2xx response is an
// error carrying the upstream body, never a task.
func globalStoreFetchTask(ctx context.Context, client *http.Client, requestURL string, apiKey string) (*GlobalStoreTaskResponse, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build globalstore task request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("globalstore task request failed: %w", err)
	}
	defer service.CloseResponseBodyGracefully(resp)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read globalstore task response body: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return nil, body, fmt.Errorf("globalstore task was not found upstream (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, body, fmt.Errorf("globalstore task HTTP status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var task GlobalStoreTaskResponse
	if err := common.Unmarshal(body, &task); err != nil {
		return nil, body, fmt.Errorf("failed to decode globalstore task response: %w", err)
	}
	return &task, body, nil
}

// globalStoreTaskWait polls the task endpoint until the task reaches a terminal
// state. It returns the terminal task and the raw body of the last successful
// poll; every timeout, transport failure, non-2xx response and unrecognized
// status is returned as an error so no usage is ever billed for it.
func globalStoreTaskWait(c *gin.Context, info *relaycommon.RelayInfo, taskID string) (*GlobalStoreTaskResponse, []byte, error) {
	client, err := service.GetHttpClientWithProxySettings(info.ChannelSetting.Proxy, info.ChannelSetting)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create globalstore polling http client: %w", err)
	}

	requestURL := globalStoreTaskURL(info.ChannelBaseUrl, taskID)
	policy := globalStoreDefaultPollPolicy
	ctx, cancel := context.WithTimeout(c.Request.Context(), policy.timeout)
	defer cancel()

	return globalStorePollTask(c, ctx, client, requestURL, info.ApiKey, policy)
}

// globalStoreTaskURL composes the documented status endpoint from the channel
// base URL.
func globalStoreTaskURL(baseURL string, taskID string) string {
	return fmt.Sprintf("%s/v2/model-center/tasks/%s", globalStoreBaseURL(baseURL), url.PathEscape(taskID))
}

// globalStorePollTask is the polling loop behind globalStoreTaskWait. It returns
// the last terminal task with a nil error, and every other outcome — budget
// exhaustion, consecutive transport failures, non-2xx responses and unrecognized
// statuses — as an error, so no usage is ever billed for them.
func globalStorePollTask(c *gin.Context, ctx context.Context, client *http.Client, requestURL string, apiKey string, policy globalStorePollPolicy) (*GlobalStoreTaskResponse, []byte, error) {
	var (
		lastTask          *GlobalStoreTaskResponse
		lastBody          []byte
		consecutiveErrors int
	)

	for attempt := 1; attempt <= policy.maxAttempts; attempt++ {
		delay := policy.interval
		if attempt == 1 {
			delay = policy.firstDelay
		}
		if err := sleepWithContext(ctx, delay); err != nil {
			return lastTask, lastBody, fmt.Errorf("globalstore task did not finish within %s: %w", policy.timeout, err)
		}

		logger.LogDebug(c, "globalstore asyncTaskWait step %d/%d", attempt, policy.maxAttempts)

		task, body, err := globalStoreFetchTask(ctx, client, requestURL, apiKey)
		if err != nil {
			consecutiveErrors++
			logger.LogWarn(c, fmt.Sprintf("globalstore asyncTaskWait poll error (%d/%d): %s", consecutiveErrors, policy.maxConsecutiveErrors, err.Error()))
			if consecutiveErrors >= policy.maxConsecutiveErrors {
				return nil, nil, fmt.Errorf("globalstore task polling aborted after %d consecutive errors: %w", consecutiveErrors, err)
			}
			continue
		}
		consecutiveErrors = 0
		lastTask = task
		lastBody = body

		switch strings.ToLower(strings.TrimSpace(task.Status)) {
		case globalStoreStatusCompleted, globalStoreStatusFailed:
			return task, body, nil
		case globalStoreStatusQueued, globalStoreStatusProcessing:
			continue
		default:
			return nil, nil, fmt.Errorf("globalstore task returned an unrecognized status %q", task.Status)
		}
	}

	return nil, nil, fmt.Errorf("globalstore task did not reach a terminal state within %s", policy.timeout)
}

// globalStoreImageHandler reads the submit response, polls the asynchronous task
// and writes the OpenAI images response. Usage is produced only for a task that
// actually completed successfully.
func globalStoreImageHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	responseBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, types.NewOpenAIError(readErr, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, types.NewOpenAIError(
			fmt.Errorf("globalstore task submit returned HTTP status %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody))),
			types.ErrorCodeBadResponseStatusCode,
			resp.StatusCode,
		)
	}

	var submitResponse GlobalStoreSubmitResponse
	if unmarshalErr := common.Unmarshal(responseBody, &submitResponse); unmarshalErr != nil {
		return nil, types.NewOpenAIError(unmarshalErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if failure := upstreamErrorText(submitResponse.Error); failure != "" {
		return nil, types.NewError(errors.New(failure), types.ErrorCodeBadResponse)
	}
	taskID := strings.TrimSpace(submitResponse.Id)
	if taskID == "" {
		return nil, types.NewError(errors.New("globalstore submit response is missing the task id"), types.ErrorCodeBadResponse)
	}

	logger.LogDebug(c, "globalstore task submitted: %s", taskID)

	task, taskBody, waitErr := globalStoreTaskWait(c, info, taskID)
	if waitErr != nil {
		return nil, types.NewError(waitErr, types.ErrorCodeBadResponse)
	}

	if strings.ToLower(strings.TrimSpace(task.Status)) == globalStoreStatusFailed {
		message := upstreamErrorText(task.Error)
		if message == "" {
			message = "globalstore task failed upstream"
		}
		logger.LogError(c, "globalstore_async_task_failed: "+message)
		return nil, types.WithOpenAIError(types.OpenAIError{
			Message: message,
			Type:    "globalstore_error",
			Code:    "task_failed",
		}, http.StatusBadGateway)
	}

	imageURL := globalStoreResultURL(task)
	if imageURL == "" {
		return nil, types.NewError(errors.New("globalstore task completed without a result URL"), types.ErrorCodeBadResponse)
	}

	imageResponse := &dto.ImageResponse{
		Data: []dto.ImageData{
			{
				Url: imageURL,
			},
		},
		Created:  info.StartTime.Unix(),
		Metadata: taskBody,
	}

	// The task actually succeeded, so this is the only point where a billable
	// image count may be reported.
	info.UpdateImageCount(1)

	jsonResponse, marshalErr := common.Marshal(imageResponse)
	if marshalErr != nil {
		return nil, types.NewError(marshalErr, types.ErrorCodeBadResponseBody)
	}
	service.IOCopyBytesGracefully(c, resp, jsonResponse)

	return &dto.Usage{}, nil
}
