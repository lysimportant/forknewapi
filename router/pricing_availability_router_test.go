package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPricingAvailabilityRoutesRespectAccess 验证两个状态接口均继承广场访问限制和禁缓存响应头。
func TestPricingAvailabilityRoutesRespectAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousRateLimit := common.GlobalApiRateLimitEnable
	common.GlobalApiRateLimitEnable = false
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	previous, existed := common.OptionMap["HeaderNavModules"]
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.GlobalApiRateLimitEnable = previousRateLimit
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		if existed {
			common.OptionMap["HeaderNavModules"] = previous
		} else {
			delete(common.OptionMap, "HeaderNavModules")
		}
	})
	engine := gin.New()
	SetApiRouter(engine)
	for _, test := range []struct {
		config string
		status int
	}{
		{`{"pricing":{"enabled":false}}`, http.StatusForbidden},
		{`{"pricing":{"enabled":true,"requireAuth":true}}`, http.StatusUnauthorized},
	} {
		common.OptionMapRWMutex.Lock()
		common.OptionMap["HeaderNavModules"] = test.config
		common.OptionMapRWMutex.Unlock()
		for _, path := range []string{"/api/pricing/yuanliu-availability", "/api/pricing/seedance-availability"} {
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
			require.Equal(t, test.status, recorder.Code, path)
			assert.Contains(t, recorder.Header().Get("Cache-Control"), "no-store")
		}
	}
}
