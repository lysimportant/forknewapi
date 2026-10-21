package helper

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelPriceHelperImageUsageCount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(saved)) })
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode":    `{"image-usage-test":"tiered_expr"}`,
		"billing_setting.billing_expr":    `{"image-usage-test":"tier(\"base\", u(\"count\") * 3)"}`,
		"group_ratio_setting.group_ratio": `{"default":1.5}`,
	}))

	for _, tc := range []struct {
		name, body  string
		mode, count int
		channel     int
		multipart   bool
	}{
		{name: "generations default count", body: `{"model":"image-usage-test","prompt":"private fixture prompt"}`, mode: relayconstant.RelayModeImagesGenerations, count: 1},
		{name: "generations multiple images", body: `{"model":"image-usage-test","n":3}`, mode: relayconstant.RelayModeImagesGenerations, count: 3},
		{name: "JSON edits", body: `{"model":"image-usage-test","n":2,"image":"private fixture image"}`, mode: relayconstant.RelayModeImagesEdits, count: 2},
		{name: "multipart edits default count", mode: relayconstant.RelayModeImagesEdits, count: 1, multipart: true},
		{name: "Ali validated nested count", body: `{"model":"image-usage-test","n":1,"parameters":{"n":4}}`, mode: relayconstant.RelayModeImagesGenerations, count: 4, channel: constant.ChannelTypeAli},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(tc.body)
			contentType := "application/json"
			path := "/v1/images/generations"
			if tc.mode == relayconstant.RelayModeImagesEdits {
				path = "/v1/images/edits"
			}
			if tc.multipart {
				var form bytes.Buffer
				writer := multipart.NewWriter(&form)
				require.NoError(t, writer.WriteField("model", "image-usage-test"))
				require.NoError(t, writer.WriteField("prompt", "private fixture prompt"))
				part, err := writer.CreateFormFile("image", "fixture.png")
				require.NoError(t, err)
				_, err = part.Write([]byte("private fixture image"))
				require.NoError(t, err)
				require.NoError(t, writer.Close())
				body, contentType = form.Bytes(), writer.FormDataContentType()
			}
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
			ctx.Request.Header.Set("Content-Type", contentType)
			common.SetContextKey(ctx, constant.ContextKeyChannelType, tc.channel)
			request, err := GetAndValidOpenAIImageRequest(ctx, tc.mode)
			require.NoError(t, err)
			info := &relaycommon.RelayInfo{OriginModelName: "image-usage-test", UserGroup: "default", UsingGroup: "default", Request: request}
			price, err := ModelPriceHelper(ctx, info, 0, &types.TokenCountMeta{})
			require.NoError(t, err)
			// $3 per image, 500000 quota/$, and group ratio 1.5.
			assert.Equal(t, tc.count*2_250_000, price.QuotaToPreConsume)
			require.NotNil(t, info.TieredBillingSnapshot)
			assert.Equal(t, billingexpr.BillingUnitRequest, info.TieredBillingSnapshot.EstimatedBillingUnit)
			assert.Equal(t, tc.count, *info.TieredBillingSnapshot.EstimatedImageCount)
			assert.False(t, info.TieredBillingSnapshot.TaskUsageBilling)
			require.NotNil(t, info.BillingRequestInput)
			assert.NotContains(t, string(info.BillingRequestInput.Body), "private fixture")
			replayed, err := io.ReadAll(ctx.Request.Body)
			require.NoError(t, err)
			assert.Equal(t, body, replayed, "pricing must not consume or rewrite the image request")
		})
	}

	t.Run("non-image request has no implicit usage count", func(t *testing.T) {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		info := &relaycommon.RelayInfo{OriginModelName: "image-usage-test", UserGroup: "default", UsingGroup: "default", Request: &dto.GeneralOpenAIRequest{}}
		_, err := ModelPriceHelper(ctx, info, 0, &types.TokenCountMeta{})
		require.ErrorContains(t, err, "tiered expr run failed")
		assert.Nil(t, info.TieredBillingSnapshot)
	})
}
