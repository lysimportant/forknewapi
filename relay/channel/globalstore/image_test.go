package globalstore

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// globalStoreTestContext builds the minimal gin context the polling loop logs to.
func globalStoreTestContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/images/generations", nil)
	return c
}

// globalStoreTestPolicy keeps the production shape with millisecond delays so the
// polling semantics can be asserted without real-time sleeps.
func globalStoreTestPolicy(maxAttempts int) globalStorePollPolicy {
	return globalStorePollPolicy{
		timeout:              time.Second,
		firstDelay:           time.Millisecond,
		interval:             time.Millisecond,
		maxAttempts:          maxAttempts,
		maxConsecutiveErrors: 2,
	}
}

// globalStoreExtraValues encodes the parameters that dto.ImageRequest keeps in
// Extra (aspect_ratio, ratio and resolution).
func globalStoreExtraValues(t *testing.T, pairs map[string]string) map[string]json.RawMessage {
	t.Helper()
	extra := make(map[string]json.RawMessage, len(pairs))
	for key, value := range pairs {
		raw, err := common.Marshal(value)
		require.NoError(t, err)
		extra[key] = raw
	}
	return extra
}

// globalStoreReferenceList builds a JSON array of public reference URLs, as the
// canvas would send them after signing its own asset addresses.
func globalStoreReferenceList(t *testing.T, count int) json.RawMessage {
	t.Helper()
	urls := make([]string, 0, count)
	for index := 0; index < count; index++ {
		urls = append(urls, fmt.Sprintf("https://cdn.example/%d.png", index))
	}
	raw, err := common.Marshal(urls)
	require.NoError(t, err)
	return raw
}

func TestGlobalstoreSubmitRequestBody(t *testing.T) {
	tooManyImages := uint(2)

	t.Run("uses the documented banana_c1 defaults", func(t *testing.T) {
		body, err := oaiImage2GlobalStoreRequest(
			&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "banana_c1"}},
			dto.ImageRequest{Model: "banana_c1", Prompt: "  一只猫  "},
		)
		require.NoError(t, err)
		assert.Equal(t, "banana_c1", body.Model)
		assert.Equal(t, "一只猫", body.Prompt)
		// banana_c1 documents one tier and a 1024x1024 default size, so a request
		// that chooses neither carries those exact pixels, never the tier name.
		assert.Equal(t, "1024x1024", body.Size)
		assert.Equal(t, "1:1", body.AspectRatio)
		assert.Empty(t, body.Resolution)
		assert.Equal(t, "standard", body.Quality)
		assert.Empty(t, body.ReferenceImages)
	})

	t.Run("maps the fork-only gpt-image-2 alias upstream", func(t *testing.T) {
		body, err := oaiImage2GlobalStoreRequest(
			&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "globalstore/gpt-image-2"}},
			dto.ImageRequest{Model: "globalstore/gpt-image-2", Prompt: "a cat", Quality: "HIGH"},
		)
		require.NoError(t, err)
		assert.Equal(t, "gpt-image-2", body.Model)
		assert.Equal(t, "high", body.Quality)
		assert.Equal(t, "2048x2048", body.Size)
		assert.Equal(t, "1:1", body.AspectRatio)
		assert.Empty(t, body.Resolution)
	})

	t.Run("prefers the channel mapped upstream model name", func(t *testing.T) {
		body, err := oaiImage2GlobalStoreRequest(
			&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "seedream-5.0"}},
			dto.ImageRequest{Model: "banana2", Prompt: "a cat"},
		)
		require.NoError(t, err)
		assert.Equal(t, "seedream-5.0", body.Model)
	})

	t.Run("sends a size when the caller chose one", func(t *testing.T) {
		body, err := oaiImage2GlobalStoreRequest(
			&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "banana2"}},
			dto.ImageRequest{Model: "banana2", Prompt: "a cat", Size: "1024X1024"},
		)
		require.NoError(t, err)
		assert.Equal(t, "1024x1024", body.Size)
		assert.Equal(t, "1:1", body.AspectRatio)
		assert.Empty(t, body.Resolution)
	})

	t.Run("resolves a tier and ratio into the documented pixels", func(t *testing.T) {
		for _, test := range []struct {
			name          string
			model         string
			extra         map[string]string
			expectedSize  string
			expectedRatio string
		}{
			{
				name:          "banana2 narrows the 4k tier to the requested ratio",
				model:         "banana2",
				extra:         map[string]string{"resolution": "4K", "aspect_ratio": "9:16"},
				expectedSize:  "3072x5504",
				expectedRatio: "9:16",
			},
			{
				name:          "gpt-image-2 keeps the requested tier",
				model:         "globalstore/gpt-image-2",
				extra:         map[string]string{"resolution": "2K", "aspect_ratio": "3:2"},
				expectedSize:  "2496x1664",
				expectedRatio: "3:2",
			},
			{
				name:          "banana_c1 falls back to its documented default",
				model:         "banana_c1",
				extra:         map[string]string{"aspect_ratio": "1:1"},
				expectedSize:  "1024x1024",
				expectedRatio: "1:1",
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				body, err := oaiImage2GlobalStoreRequest(
					&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: test.model}},
					dto.ImageRequest{
						Model:  test.model,
						Prompt: "a cat",
						Extra:  globalStoreExtraValues(t, test.extra),
					},
				)
				require.NoError(t, err)
				assert.Equal(t, test.expectedSize, body.Size)
				assert.Equal(t, test.expectedRatio, body.AspectRatio)
				assert.Empty(t, body.Resolution)
			})
		}
	})

	t.Run("maps a requested pixel size onto the documented enumeration", func(t *testing.T) {
		// The canvas resolves its own clarity tier into pixels, so 1k 16:9
		// arrives as 1024x576, which is not one of the documented sizes. The
		// channel keeps the intent and sends the documented size of that ratio.
		body, err := oaiImage2GlobalStoreRequest(
			&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "banana2"}},
			dto.ImageRequest{Model: "banana2", Prompt: "a cat", Size: "1024x576"},
		)
		require.NoError(t, err)
		assert.Equal(t, "1376x768", body.Size)
		assert.Equal(t, "16:9", body.AspectRatio)
		assert.Empty(t, body.Resolution)
	})

	t.Run("keeps the tier name only for models without a size list", func(t *testing.T) {
		body, err := oaiImage2GlobalStoreRequest(
			&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "banana_c2"}},
			dto.ImageRequest{Model: "banana_c2", Prompt: "a cat", Size: "2048x1152"},
		)
		require.NoError(t, err)
		assert.Equal(t, "2K", body.Resolution)
		assert.Equal(t, "16:9", body.AspectRatio)
		assert.Empty(t, body.Size)

		// seedream-5.0 documents a size without an enumeration, so the exact
		// pixels pass through untouched.
		seedream, err := oaiImage2GlobalStoreRequest(
			&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "seedream-5.0"}},
			dto.ImageRequest{Model: "seedream-5.0", Prompt: "a cat", Size: "1024x1024"},
		)
		require.NoError(t, err)
		assert.Equal(t, "1024x1024", seedream.Size)
		assert.Equal(t, "1:1", seedream.AspectRatio)
		assert.Empty(t, seedream.Resolution)
	})

	t.Run("forwards the documented auto framing instead of forcing a ratio", func(t *testing.T) {
		// banana2 documents auto as an aspect ratio, so the caller's "you frame it"
		// intent survives; gpt-image-2.5 does not, so it keeps its documented default
		// instead of receiving a value its vendor page does not list.
		banana, err := oaiImage2GlobalStoreRequest(
			&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "banana2"}},
			dto.ImageRequest{
				Model:  "banana2",
				Prompt: "a cat",
				Extra:  globalStoreExtraValues(t, map[string]string{"aspect_ratio": "AUTO"}),
			},
		)
		require.NoError(t, err)
		assert.Equal(t, "auto", banana.AspectRatio)
		assert.Empty(t, banana.Size)
		assert.Empty(t, banana.Resolution)

		autoSize, err := oaiImage2GlobalStoreRequest(
			&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "banana2"}},
			dto.ImageRequest{Model: "banana2", Prompt: "a cat", Size: "auto"},
		)
		require.NoError(t, err)
		assert.Equal(t, "auto", autoSize.AspectRatio)
		assert.Empty(t, autoSize.Size)

		// banana_c2 has no pixel size contract, so auto still carries a tier.
		tierOnly, err := oaiImage2GlobalStoreRequest(
			&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "banana_c2"}},
			dto.ImageRequest{
				Model:  "banana_c2",
				Prompt: "a cat",
				Extra:  globalStoreExtraValues(t, map[string]string{"aspect_ratio": "auto", "resolution": "2K"}),
			},
		)
		require.NoError(t, err)
		assert.Equal(t, "auto", tierOnly.AspectRatio)
		assert.Equal(t, "2K", tierOnly.Resolution)
		assert.Empty(t, tierOnly.Size)

		undeclared, err := oaiImage2GlobalStoreRequest(
			&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-image-2.5"}},
			dto.ImageRequest{Model: "gpt-image-2.5", Prompt: "a cat", Size: "auto"},
		)
		require.NoError(t, err)
		assert.Equal(t, "1024x1024", undeclared.Size)
		assert.Equal(t, "1:1", undeclared.AspectRatio)
		assert.Empty(t, undeclared.Resolution)
	})

	t.Run("accepts public reference image URLs", func(t *testing.T) {
		body, err := oaiImage2GlobalStoreRequest(
			&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "banana2"}},
			dto.ImageRequest{
				Model:  "banana2",
				Prompt: "a cat",
				Image:  json.RawMessage(`["https://cdn.example/one.png","http://cdn.example/two.jpg"]`),
			},
		)
		require.NoError(t, err)
		assert.Equal(t, []string{"https://cdn.example/one.png", "http://cdn.example/two.jpg"}, body.ReferenceImages)
	})

	t.Run("accepts the images array and url objects", func(t *testing.T) {
		body, err := oaiImage2GlobalStoreRequest(
			&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "banana2"}},
			dto.ImageRequest{
				Model:  "banana2",
				Prompt: "a cat",
				Images: json.RawMessage(`[{"image_url":{"url":"https://cdn.example/nested.png"}}]`),
			},
		)
		require.NoError(t, err)
		assert.Equal(t, []string{"https://cdn.example/nested.png"}, body.ReferenceImages)
	})

	t.Run("never sends quality to a model that does not declare it", func(t *testing.T) {
		body, err := oaiImage2GlobalStoreRequest(
			&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "banana2"}},
			dto.ImageRequest{Model: "banana2", Prompt: "a cat", Quality: "ultra"},
		)
		require.NoError(t, err)
		assert.Empty(t, body.Quality)
	})

	t.Run("rejects requests the upstream contract forbids", func(t *testing.T) {
		for _, test := range []struct {
			name      string
			model     string
			request   dto.ImageRequest
			expected  string
			assertion func(t *testing.T, err error)
		}{
			{
				name:     "unknown model",
				model:    "not-a-globalstore-model",
				request:  dto.ImageRequest{Prompt: "a cat"},
				expected: "unsupported GlobalStore image model",
			},
			{
				name:     "missing prompt",
				model:    "banana2",
				request:  dto.ImageRequest{},
				expected: "prompt is required",
			},
			{
				name:     "more than one image per task",
				model:    "banana2",
				request:  dto.ImageRequest{Prompt: "a cat", N: &tooManyImages},
				expected: "n greater than 1",
			},
			{
				name:     "base64 response format",
				model:    "banana2",
				request:  dto.ImageRequest{Prompt: "a cat", ResponseFormat: "b64_json"},
				expected: "response_format must be url",
			},
			{
				name:  "tier the model does not declare",
				model: "banana2",
				request: dto.ImageRequest{
					Prompt: "a cat",
					Size:   "1024x1024",
					Extra:  globalStoreExtraValues(t, map[string]string{"resolution": "8K"}),
				},
				expected: "resolution must be one of",
			},
			{
				name:     "malformed size",
				model:    "banana2",
				request:  dto.ImageRequest{Prompt: "a cat", Size: "1024"},
				expected: "size must be WxH",
			},
			{
				// banana_c2 documents 4:1 and 8:1 but no 7:1, and the model has no
				// size field that could carry the requested frame instead.
				name:     "aspect ratio a tier-only model cannot express",
				model:    "banana_c2",
				request:  dto.ImageRequest{Prompt: "a cat", Size: "700x100"},
				expected: "aspect ratio of size",
			},
			{
				// ratiosI1 already covers 5:4, so 5:7 is the missing sibling.
				name:  "ratio the model does not declare",
				model: "banana2",
				request: dto.ImageRequest{
					Prompt: "a cat",
					Extra:  globalStoreExtraValues(t, map[string]string{"aspect_ratio": "5:7"}),
				},
				expected: "aspect_ratio must be one of",
			},
			{
				name:  "conflicting ratio aliases",
				model: "banana2",
				request: dto.ImageRequest{
					Prompt: "a cat",
					Extra:  globalStoreExtraValues(t, map[string]string{"aspect_ratio": "1:1", "ratio": "16:9"}),
				},
				expected: "aspect_ratio conflicts with ratio",
			},
			{
				name:  "resolution the model does not declare",
				model: "banana2",
				request: dto.ImageRequest{
					Prompt: "a cat",
					Extra:  globalStoreExtraValues(t, map[string]string{"resolution": "8K"}),
				},
				expected: "resolution must be one of",
			},
			{
				name:  "quality the model does not declare",
				model: "banana_c1",
				request: dto.ImageRequest{
					Prompt:  "a cat",
					Quality: "ultra",
				},
				expected: "quality must be one of",
			},
			{
				name:     "base64 upload instead of a public url",
				model:    "banana2",
				request:  dto.ImageRequest{Prompt: "a cat", Image: json.RawMessage(`"data:image/png;base64,AAAA"`)},
				expected: "public HTTP(S) URLs",
			},
			{
				name:     "file path instead of a public url",
				model:    "banana2",
				request:  dto.ImageRequest{Prompt: "a cat", Image: json.RawMessage(`["/v1/assets/x/versions/1/content"]`)},
				expected: "public HTTP(S) URLs",
			},
			{
				name:     "more references than the model allows",
				model:    "seedream-5.0",
				request:  dto.ImageRequest{Prompt: "a cat", Image: globalStoreReferenceList(t, 11)},
				expected: "cannot exceed 10 for seedream-5.0",
			},
			{
				name:     "more media items than the upstream accepts",
				model:    "banana2",
				request:  dto.ImageRequest{Prompt: "a cat", Image: globalStoreReferenceList(t, globalStoreMaxMediaItems+1)},
				expected: "cannot exceed 60 items",
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				body, err := oaiImage2GlobalStoreRequest(
					&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: test.model}},
					test.request,
				)
				require.Error(t, err, "expected the local contract check to reject this request")
				assert.Nil(t, body)
				assert.Contains(t, err.Error(), test.expected)
			})
		}
	})
}

// The canvas posts a JSON image edit body, so reference URLs have to arrive in
// the OpenAI image/images fields. dto.ImageRequest has no "image[]" field, so a
// multipart style plural key would be parked in Extra and dropped here, which
// would silently degrade an image edit into a text-to-image call.
func TestGlobalstoreReadsJSONEditReferences(t *testing.T) {
	upstreamInfo := func() *relaycommon.RelayInfo {
		return &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "banana2"},
		}
	}

	t.Run("collects both reference fields into one upstream array", func(t *testing.T) {
		var request dto.ImageRequest
		require.NoError(t, common.Unmarshal([]byte(`{
			"model": "banana2",
			"prompt": "a cat",
			"image": "https://cdn.example/one.png",
			"images": ["https://cdn.example/two.png", "https://cdn.example/three.png"]
		}`), &request))

		body, err := oaiImage2GlobalStoreRequest(upstreamInfo(), request)
		require.NoError(t, err)
		assert.Equal(t, []string{
			"https://cdn.example/one.png",
			"https://cdn.example/two.png",
			"https://cdn.example/three.png",
		}, body.ReferenceImages)
	})

	t.Run("drops a multipart style image[] key", func(t *testing.T) {
		var request dto.ImageRequest
		require.NoError(t, common.Unmarshal([]byte(`{
			"model": "banana2",
			"prompt": "a cat",
			"image[]": ["https://cdn.example/one.png"]
		}`), &request))

		body, err := oaiImage2GlobalStoreRequest(upstreamInfo(), request)
		require.NoError(t, err)
		assert.Empty(t, body.ReferenceImages, "the canvas must send images, not image[]")
	})
}

func TestGlobalstorePollTaskSemantics(t *testing.T) {
	/** newPollServer answers every status query with the caller's response. */
	newPollServer := func(t *testing.T, response func(attempt int32) (int, string)) (*httptest.Server, *atomic.Int32, *atomic.Value) {
		t.Helper()
		var attempts atomic.Int32
		var authorization atomic.Value
		authorization.Store("")
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			authorization.Store(request.Header.Get("Authorization"))
			status, body := response(attempts.Add(1))
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(status)
			_, _ = writer.Write([]byte(body))
		}))
		t.Cleanup(server.Close)
		return server, &attempts, &authorization
	}

	poll := func(t *testing.T, server *httptest.Server, policy globalStorePollPolicy) (*GlobalStoreTaskResponse, []byte, error) {
		t.Helper()
		c := globalStoreTestContext(t)
		ctx, cancel := context.WithTimeout(context.Background(), policy.timeout)
		defer cancel()
		return globalStorePollTask(c, ctx, server.Client(), server.URL, "sk-globalstore", policy)
	}

	t.Run("settles on a completed task", func(t *testing.T) {
		server, attempts, authorization := newPollServer(t, func(attempt int32) (int, string) {
			if attempt == 1 {
				return http.StatusOK, `{"id":"task-1","status":"processing"}`
			}
			return http.StatusOK, `{"id":"task-1","status":"completed","image_url":"https://cdn.example/out.png","progress":100}`
		})

		task, body, err := poll(t, server, globalStoreTestPolicy(4))
		require.NoError(t, err)
		require.NotNil(t, task)
		assert.Equal(t, globalStoreStatusCompleted, task.Status)
		assert.Equal(t, "https://cdn.example/out.png", globalStoreResultURL(task))
		assert.Contains(t, string(body), "completed")
		assert.Equal(t, int32(2), attempts.Load(), "polling must stop at the first terminal status")
		assert.Equal(t, "Bearer sk-globalstore", authorization.Load())
	})

	t.Run("settles in the queued then completed order", func(t *testing.T) {
		server, attempts, _ := newPollServer(t, func(attempt int32) (int, string) {
			if attempt < 3 {
				return http.StatusOK, `{"id":"task-1","status":"QUEUED"}`
			}
			return http.StatusOK, `{"id":"task-1","status":"Completed","result_url":"https://cdn.example/fallback.png"}`
		})

		task, _, err := poll(t, server, globalStoreTestPolicy(5))
		require.NoError(t, err)
		require.NotNil(t, task)
		assert.Equal(t, "https://cdn.example/fallback.png", globalStoreResultURL(task))
		assert.Equal(t, int32(3), attempts.Load())
	})

	t.Run("returns a failed task as a terminal result", func(t *testing.T) {
		server, _, _ := newPollServer(t, func(int32) (int, string) {
			return http.StatusOK, `{"id":"task-1","status":"failed","error":{"message":"prompt rejected","code":"content_filter"}}`
		})

		task, _, err := poll(t, server, globalStoreTestPolicy(4))
		// A failed task is a terminal outcome, not a transport error: the handler
		// decides how to surface it and never bills usage for it.
		require.NoError(t, err)
		require.NotNil(t, task)
		assert.Equal(t, globalStoreStatusFailed, task.Status)
		// errorMessage() in plugin.js prefers the message and only falls back to
		// the code, so the code is deliberately not appended here.
		assert.Equal(t, "prompt rejected", upstreamErrorText(task.Error))
	})

	t.Run("falls back to the error code when the message is missing", func(t *testing.T) {
		server, _, _ := newPollServer(t, func(int32) (int, string) {
			return http.StatusOK, `{"id":"task-1","status":"failed","error":{"code":"content_filter"}}`
		})

		task, _, err := poll(t, server, globalStoreTestPolicy(4))
		require.NoError(t, err)
		require.NotNil(t, task)
		assert.Equal(t, "content_filter", upstreamErrorText(task.Error))
	})

	t.Run("treats an empty status as an error", func(t *testing.T) {
		server, _, _ := newPollServer(t, func(int32) (int, string) {
			return http.StatusOK, `{"id":"task-1"}`
		})

		task, _, err := poll(t, server, globalStoreTestPolicy(4))
		require.Error(t, err)
		assert.Nil(t, task)
		assert.Contains(t, err.Error(), "unrecognized status")
	})

	t.Run("reports a task the upstream no longer has", func(t *testing.T) {
		server, _, _ := newPollServer(t, func(int32) (int, string) {
			return http.StatusNotFound, `{"error":"not found"}`
		})

		task, _, err := poll(t, server, globalStoreTestPolicy(4))
		require.Error(t, err)
		assert.Nil(t, task)
		assert.Contains(t, err.Error(), "not found upstream")
	})

	t.Run("never parses a non-2xx body as a task", func(t *testing.T) {
		// A 500 body that happens to look like a completed task must not be billed.
		server, _, _ := newPollServer(t, func(int32) (int, string) {
			return http.StatusInternalServerError, `{"id":"task-1","status":"completed","image_url":"https://cdn.example/out.png"}`
		})

		task, _, err := poll(t, server, globalStoreTestPolicy(4))
		require.Error(t, err)
		assert.Nil(t, task)
		assert.Contains(t, err.Error(), "HTTP status 500")
	})

	t.Run("aborts after consecutive upstream failures", func(t *testing.T) {
		server, attempts, _ := newPollServer(t, func(int32) (int, string) {
			return http.StatusServiceUnavailable, `{"error":"upstream busy"}`
		})

		policy := globalStoreTestPolicy(8)
		task, _, err := poll(t, server, policy)
		require.Error(t, err)
		assert.Nil(t, task)
		assert.Contains(t, err.Error(), "consecutive errors")
		assert.Equal(t, int32(policy.maxConsecutiveErrors), attempts.Load(),
			"the loop must stop at the consecutive error cap instead of exhausting the budget")
	})

	t.Run("recovers when a transient failure is followed by success", func(t *testing.T) {
		server, attempts, _ := newPollServer(t, func(attempt int32) (int, string) {
			if attempt == 1 {
				return http.StatusBadGateway, `{"error":"bad gateway"}`
			}
			return http.StatusOK, `{"id":"task-1","status":"completed","image_url":"https://cdn.example/out.png"}`
		})

		task, _, err := poll(t, server, globalStoreTestPolicy(4))
		require.NoError(t, err)
		require.NotNil(t, task)
		assert.Equal(t, globalStoreStatusCompleted, task.Status)
		assert.Equal(t, int32(2), attempts.Load())
	})

	t.Run("gives up when the task never settles", func(t *testing.T) {
		server, attempts, _ := newPollServer(t, func(int32) (int, string) {
			return http.StatusOK, `{"id":"task-1","status":"processing"}`
		})

		policy := globalStoreTestPolicy(3)
		task, _, err := poll(t, server, policy)
		require.Error(t, err)
		assert.Nil(t, task)
		assert.Contains(t, err.Error(), "did not reach a terminal state")
		assert.Equal(t, int32(policy.maxAttempts), attempts.Load())
	})
}

func TestGlobalstoreResultURLPreference(t *testing.T) {
	assert.Equal(t, "https://cdn.example/image.png",
		globalStoreResultURL(&GlobalStoreTaskResponse{ImageURL: "https://cdn.example/image.png", ResultURL: "https://cdn.example/result.png"}))
	assert.Equal(t, "https://cdn.example/result.png",
		globalStoreResultURL(&GlobalStoreTaskResponse{ResultURL: "https://cdn.example/result.png"}))
	assert.Empty(t, globalStoreResultURL(&GlobalStoreTaskResponse{}))
}

func TestGlobalstoreTaskURLComposition(t *testing.T) {
	// The documented media-task entry carries a path prefix that must survive.
	assert.Equal(t,
		"https://saas.globalstore.ai/kyyReactApiServer/v2/model-center/tasks/task%2F1",
		globalStoreTaskURL("  https://saas.globalstore.ai/kyyReactApiServer/  ", "task/1"))
	assert.Equal(t,
		"https://saas.globalstore.ai/kyyReactApiServer/v2/model-center/tasks/task-1",
		globalStoreTaskURL("https://saas.globalstore.ai/kyyReactApiServer", "task-1"))
}
