package globalstore

import (
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
)

// globalStoreEditContext builds the gin context of an image edit request.
// uploadedFileParts reproduces what the shared parser leaves behind when the
// caller uploads the reference image instead of naming a public URL.
func globalStoreEditContext(t *testing.T, uploadedFileParts bool) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	if uploadedFileParts {
		c.Request.MultipartForm = &multipart.Form{
			File: map[string][]*multipart.FileHeader{
				"image": {{Filename: "reference.png"}},
			},
		}
	}
	return c
}

// globalStoreErrorStatus reads the HTTP status the relay will answer with.
// Client input errors must stay 4xx so callers do not retry them as outages.
func globalStoreErrorStatus(t *testing.T, err error) int {
	t.Helper()
	var newAPIErr *types.NewAPIError
	require.ErrorAs(t, err, &newAPIErr)
	assert.Equal(t, types.ErrorCodeInvalidRequest, newAPIErr.GetErrorCode())
	return newAPIErr.StatusCode
}

func TestGlobalstoreRejectsUploadedReferenceBytes(t *testing.T) {
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits}

	t.Run("rejects an upload whose bytes the upstream cannot read", func(t *testing.T) {
		request := dto.ImageRequest{Model: "banana2", Prompt: "a cat"}
		body, err := adaptor.ConvertImageRequest(globalStoreEditContext(t, true), info, request)
		require.Error(t, err)
		assert.Nil(t, body)
		assert.Contains(t, err.Error(), "public HTTP(S) reference image URLs")
		assert.Equal(t, http.StatusBadRequest, globalStoreErrorStatus(t, err))
	})

	t.Run("reports model side validation failures as bad requests", func(t *testing.T) {
		count := uint(2)
		request := dto.ImageRequest{Model: "banana2", Prompt: "a cat", N: &count}
		body, err := adaptor.ConvertImageRequest(globalStoreEditContext(t, false), info, request)
		require.Error(t, err)
		assert.Nil(t, body)
		assert.Contains(t, err.Error(), "n greater than 1 is not supported")
		assert.Equal(t, http.StatusBadRequest, globalStoreErrorStatus(t, err))
	})

	t.Run("accepts an edit that also names a public reference", func(t *testing.T) {
		request := dto.ImageRequest{
			Model:  "banana2",
			Prompt: "a cat",
			Image:  []byte(`"https://cdn.example/reference.png"`),
		}
		body, err := adaptor.ConvertImageRequest(globalStoreEditContext(t, true), info, request)
		require.NoError(t, err)
		require.NotNil(t, body)
		submit, ok := body.(*GlobalStoreSubmitRequest)
		require.True(t, ok, "the channel must build a GlobalStore submit body")
		assert.Equal(t, []string{"https://cdn.example/reference.png"}, submit.ReferenceImages)
	})

	t.Run("accepts a form value carrying the url", func(t *testing.T) {
		request := dto.ImageRequest{
			Model:  "banana2",
			Prompt: "a cat",
			Image:  []byte(`"https://cdn.example/reference.png"`),
		}
		body, err := adaptor.ConvertImageRequest(globalStoreEditContext(t, false), info, request)
		require.NoError(t, err)
		require.NotNil(t, body)
	})

	t.Run("leaves text to image requests untouched", func(t *testing.T) {
		request := dto.ImageRequest{Model: "banana2", Prompt: "a cat"}
		body, err := adaptor.ConvertImageRequest(globalStoreEditContext(t, false), info, request)
		require.NoError(t, err)
		require.NotNil(t, body)
		submit, ok := body.(*GlobalStoreSubmitRequest)
		require.True(t, ok)
		assert.Empty(t, submit.ReferenceImages)
	})
}

func TestGlobalstoreAdaptorRouting(t *testing.T) {
	adaptor := &Adaptor{}

	t.Run("submits both generation and edit modes to the model center", func(t *testing.T) {
		for _, mode := range []int{relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits} {
			url, err := adaptor.GetRequestURL(&relaycommon.RelayInfo{
				RelayMode:   mode,
				ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://saas.globalstore.ai/kyyReactApiServer/"},
			})
			require.NoError(t, err)
			assert.Equal(t, "https://saas.globalstore.ai/kyyReactApiServer/v2/model-center/tasks", url)
		}
	})

	t.Run("refuses relay modes it cannot serve", func(t *testing.T) {
		_, err := adaptor.GetRequestURL(&relaycommon.RelayInfo{
			RelayMode:   relayconstant.RelayModeChatCompletions,
			ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://saas.globalstore.ai"},
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, onlyImagesError)

		_, responseErr := adaptor.DoResponse(
			globalStoreEditContext(t, false),
			&http.Response{},
			&relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeChatCompletions},
		)
		require.Error(t, responseErr)
	})

	t.Run("authenticates with the channel key as a bearer token", func(t *testing.T) {
		c := globalStoreEditContext(t, false)
		header := http.Header{}
		info := &relaycommon.RelayInfo{
			RelayMode:   relayconstant.RelayModeImagesGenerations,
			ChannelMeta: &relaycommon.ChannelMeta{ApiKey: "sk-globalstore"},
		}
		require.NoError(t, adaptor.SetupRequestHeader(c, &header, info))
		assert.Equal(t, "Bearer sk-globalstore", header.Get("Authorization"))
		assert.Equal(t, "application/json", header.Get("Content-Type"))
	})

	t.Run("publishes the channel identity and model list", func(t *testing.T) {
		assert.Equal(t, "globalstore", adaptor.GetChannelName())
		assert.Equal(t, []string{
			"banana2",
			"bananapro",
			"banana_c1",
			"banana_c2",
			"globalstore/gpt-image-2",
			"gpt-image-2.5",
			"seedream-5.0",
			"seedream_5.0Pro",
		}, adaptor.GetModelList())
	})

	t.Run("degrades no other conversion path", func(t *testing.T) {
		c := globalStoreEditContext(t, false)
		info := &relaycommon.RelayInfo{}

		general, generalErr := adaptor.ConvertOpenAIRequest(c, info, &dto.GeneralOpenAIRequest{})
		require.ErrorIs(t, generalErr, onlyImagesError)
		assert.Nil(t, general)

		responses, responsesErr := adaptor.ConvertOpenAIResponsesRequest(c, info, dto.OpenAIResponsesRequest{})
		require.ErrorIs(t, responsesErr, onlyImagesError)
		assert.Nil(t, responses)

		claude, claudeErr := adaptor.ConvertClaudeRequest(c, info, &dto.ClaudeRequest{})
		require.ErrorIs(t, claudeErr, onlyImagesError)
		assert.Nil(t, claude)

		gemini, geminiErr := adaptor.ConvertGeminiRequest(c, info, &dto.GeminiChatRequest{})
		require.ErrorIs(t, geminiErr, onlyImagesError)
		assert.Nil(t, gemini)

		embedding, embeddingErr := adaptor.ConvertEmbeddingRequest(c, info, dto.EmbeddingRequest{})
		require.ErrorIs(t, embeddingErr, onlyImagesError)
		assert.Nil(t, embedding)

		rerank, rerankErr := adaptor.ConvertRerankRequest(c, relayconstant.RelayModeRerank, dto.RerankRequest{})
		require.ErrorIs(t, rerankErr, onlyImagesError)
		assert.Nil(t, rerank)

		audio, audioErr := adaptor.ConvertAudioRequest(c, info, dto.AudioRequest{})
		require.ErrorIs(t, audioErr, onlyImagesError)
		assert.Nil(t, audio)
	})
}
