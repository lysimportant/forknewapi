package ratio_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCustomCompletionRatiosSurviveUpgrade 保护管理员覆盖内置输出倍率的定制契约，避免升级后重新锁定价格。
func TestCustomCompletionRatiosSurviveUpgrade(t *testing.T) {
	original := CompletionRatio2JSONString()
	t.Cleanup(func() { require.NoError(t, UpdateCompletionRatioByJSONString(original)) })
	require.NoError(t, UpdateCompletionRatioByJSONString(`{"gpt-4o-2024-05-13":7.25,"claude-3-5-sonnet-20241022":0,"vendor/custom-model":4.5}`))
	for _, tc := range []struct {
		model string
		ratio float64
	}{
		{"gpt-4o-2024-05-13", 7.25},
		{"claude-3-5-sonnet-20241022", 0},
		{"vendor/custom-model", 4.5},
	} {
		t.Run(tc.model, func(t *testing.T) {
			assert.Equal(t, tc.ratio, GetCompletionRatio(tc.model))
			info := GetCompletionRatioInfo(tc.model)
			assert.Equal(t, tc.ratio, info.Ratio)
			assert.False(t, info.Locked)
		})
	}
	require.NoError(t, UpdateCompletionRatioByJSONString(`{}`))
	assert.Equal(t, 3.0, GetCompletionRatio("gpt-4o-2024-05-13"))
	assert.False(t, GetCompletionRatioInfo("gpt-4o-2024-05-13").Locked)
}
