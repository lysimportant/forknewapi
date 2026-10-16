package globalstore

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	relaychannel "github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
)

// onlyImagesError is returned by every conversion path this channel does not
// serve. GlobalStore exposes a single asynchronous model-center task API, so
// there is no OpenAI-compatible fallback to degrade to.
var onlyImagesError = errors.New("globalstore channel only serves image generation")

type Adaptor struct {
	client http.Client
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	switch info.RelayMode {
	case relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits:
		return fmt.Sprintf("%s/v2/model-center/tasks", globalStoreBaseURL(info.ChannelBaseUrl)), nil
	default:
		return "", fmt.Errorf("%w: unsupported relay mode %d", onlyImagesError, info.RelayMode)
	}
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	relaychannel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	req.Set("Content-Type", "application/json")
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return nil, onlyImagesError
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, onlyImagesError
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, onlyImagesError
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, onlyImagesError
}

// ConvertImageRequest builds the GlobalStore model-center submit body. File
// uploads and data URIs are rejected: the upstream reads public HTTP(S) media
// only.
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	if err := assertNoUploadedReferenceBytes(c, request); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	globalStoreRequest, err := oaiImage2GlobalStoreRequest(info, request)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	return globalStoreRequest, nil
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, onlyImagesError
}
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return relaychannel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	switch info.RelayMode {
	case relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits:
		return globalStoreImageHandler(c, resp, info)
	default:
		return nil, types.NewError(
			fmt.Errorf("%w: unsupported relay mode %d", onlyImagesError, info.RelayMode),
			types.ErrorCodeInvalidRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	return nil, onlyImagesError
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return nil, onlyImagesError
}

// assertNoUploadedReferenceBytes rejects a multipart image edit that carries its
// reference image as uploaded file bytes.
//
// The shared image-edit parser (relay/helper.GetAndValidOpenAIImageRequest) keeps
// only the text form value "image"; file parts stay behind in
// Request.MultipartForm.File and never reach dto.ImageRequest. Without this guard
// the reference would be dropped silently and the caller would receive an
// unrelated text-to-image result. GlobalStore reads server-reachable HTTP(S)
// media, so the bytes cannot be forwarded either.
func assertNoUploadedReferenceBytes(c *gin.Context, request dto.ImageRequest) error {
	if c == nil || c.Request == nil || c.Request.MultipartForm == nil {
		return nil
	}
	if len(c.Request.MultipartForm.File) == 0 {
		return nil
	}
	if len(request.Image) > 0 || len(request.Images) > 0 {
		// A usable public URL was supplied as well, so the dropped file parts do
		// not change the request the upstream receives.
		return nil
	}
	return errors.New(
		"globalstore image edits require public HTTP(S) reference image URLs; uploaded file bytes cannot be forwarded to the upstream")
}

// compile-time assertion that the adaptor honours the relay contract.
var _ relaychannel.Adaptor = (*Adaptor)(nil)
