package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateTextOtherInfoRecordsModelAuditContract(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tt := range []struct {
		name             string
		requestedModel   string
		billingModel     string
		sentModel        string
		responseModel    string
		isMapped         bool
		wantRequested    bool
		wantSent         bool
		wantResponse     bool
		wantMismatch     bool
		mismatchExpected bool
	}{
		{
			name:             "response mismatch without configured mapping",
			requestedModel:   "virtual-request-model",
			billingModel:     "billing-model",
			sentModel:        "provider-model",
			responseModel:    "provider-runtime-model",
			wantRequested:    true,
			wantSent:         true,
			wantResponse:     true,
			wantMismatch:     true,
			mismatchExpected: true,
		},
		{
			name:           "matching response records explicit false",
			requestedModel: "provider-model",
			sentModel:      "provider-model",
			responseModel:  "provider-model",
			wantSent:       true,
			wantResponse:   true,
			wantMismatch:   true,
		},
		{
			name:           "missing response remains unknown",
			requestedModel: "alias-model",
			sentModel:      "provider-model",
			isMapped:       true,
			wantSent:       true,
		},
		{
			name:           "parameter override difference without response model",
			requestedModel: "requested-model",
			sentModel:      "overridden-model",
			wantSent:       true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			start := time.Unix(1_700_000_000, 0)
			info := &relaycommon.RelayInfo{
				OriginModelName:   tt.requestedModel,
				BillingModelName:  tt.billingModel,
				StartTime:         start,
				FirstResponseTime: start.Add(time.Second),
				ChannelMeta: &relaycommon.ChannelMeta{
					UpstreamModelName: tt.sentModel,
					IsModelMapped:     tt.isMapped,
				},
			}
			info.SetSentUpstreamModelName(tt.sentModel)
			info.ObserveUpstreamResponseModel(tt.responseModel, true)

			generated := GenerateTextOtherInfo(ctx, info, 1, 1, 1, 0, 1, 0, 1)
			require.NotNil(t, generated)
			other := generated.Snapshot()

			if tt.wantRequested {
				assert.Equal(t, tt.requestedModel, other["requested_model_name"])
			} else {
				assert.NotContains(t, other, "requested_model_name")
			}
			if tt.wantSent {
				assert.Equal(t, tt.sentModel, other["upstream_model_name"])
			} else {
				assert.NotContains(t, other, "upstream_model_name")
			}
			if tt.wantResponse {
				assert.Equal(t, tt.responseModel, other["upstream_response_model"])
			} else {
				assert.NotContains(t, other, "upstream_response_model")
			}
			if tt.wantMismatch {
				assert.Equal(t, tt.mismatchExpected, other["upstream_model_mismatch"])
			} else {
				assert.NotContains(t, other, "upstream_model_mismatch")
			}
			assert.Equal(t, tt.isMapped, other["is_model_mapped"] == true)
		})
	}
}

func TestAppendRelayModelLogInfoHandlesMissingChannelMeta(t *testing.T) {
	info := &relaycommon.RelayInfo{OriginModelName: "requested-model"}
	info.ObserveUpstreamResponseModel("response-model", true)
	other := model.NewLogOther()

	require.NotPanics(t, func() {
		AppendRelayModelLogInfo(info, other, "requested-model")
	})
	snapshot := other.Snapshot()
	assert.Equal(t, "response-model", snapshot["upstream_response_model"])
	assert.NotContains(t, snapshot, "upstream_model_name")
	assert.NotContains(t, snapshot, "upstream_model_mismatch")
}
