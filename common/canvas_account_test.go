package common

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCanvasAccountLegacyConfig 验证未配置附加客户端时保留原部署字段和规范化行为。
func TestCanvasAccountLegacyConfig(t *testing.T) {
	setCanvasAccountPrimaryEnv(t)

	config, err := GetCanvasAccountConfig()
	require.NoError(t, err)
	assert.Equal(t, CanvasAccountConfig{
		Issuer:       "https://newapi.example.com",
		ClientID:     "canvas-production",
		InstanceID:   "production",
		RedirectURI:  "https://canvas.example.com/api/auth/callback/new-api",
		ClientSecret: "primary-secret",
	}, config)

	configs, err := GetCanvasAccountConfigs()
	require.NoError(t, err)
	require.Len(t, configs, 1)
	assert.Equal(t, config, configs[0])
}

// TestCanvasAccountConfigsAdditionalClients 验证附加实例继承 issuer 且按精确身份查找。
func TestCanvasAccountConfigsAdditionalClients(t *testing.T) {
	setCanvasAccountPrimaryEnv(t)
	t.Setenv("CANVAS_ACCOUNT_ADDITIONAL_CLIENTS", `[
		{"client_id":"canvas-local","instance_id":"local","redirect_uri":"http://localhost:5173/api/auth/callback/new-api"},
		{"client_id":"canvas-server","instance_id":"server","redirect_uri":"https://server.canvas.example/api/auth/callback/new-api","client_secret":"server-secret"}
	]`)

	configs, err := GetCanvasAccountConfigs()
	require.NoError(t, err)
	require.Len(t, configs, 3)
	assert.Equal(t, "https://newapi.example.com", configs[1].Issuer)
	assert.Empty(t, configs[1].ClientSecret)
	assert.Equal(t, "server-secret", configs[2].ClientSecret)

	local, found := FindCanvasAccountConfig(configs, "canvas-local", "local")
	require.True(t, found)
	assert.Equal(t, "http://localhost:5173/api/auth/callback/new-api", local.RedirectURI)
	_, found = FindCanvasAccountConfig(configs, " canvas-local", "local")
	assert.False(t, found)
}

// TestCanvasAccountConfigsRejectDuplicatePair 拒绝与主实例或其他附加实例重叠的身份。
func TestCanvasAccountConfigsRejectDuplicatePair(t *testing.T) {
	setCanvasAccountPrimaryEnv(t)

	tests := map[string]string{
		"primary pair": `[{"client_id":"canvas-production","instance_id":"production","redirect_uri":"https://other.example/callback"}]`,
		"additional pair": `[
			{"client_id":"canvas-local","instance_id":"local","redirect_uri":"http://localhost:5173/callback"},
			{"client_id":"canvas-local","instance_id":"local","redirect_uri":"https://canvas.example/callback"}
		]`,
	}
	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			t.Setenv("CANVAS_ACCOUNT_ADDITIONAL_CLIENTS", value)
			_, err := GetCanvasAccountConfigs()
			require.ErrorIs(t, err, errCanvasAccountConfiguration)
		})
	}
}

// TestCanvasAccountConfigsRejectInvalidJSONShape 拒绝歧义、缺字段和类型错误的配置。
func TestCanvasAccountConfigsRejectInvalidJSONShape(t *testing.T) {
	setCanvasAccountPrimaryEnv(t)

	tests := map[string]string{
		"malformed":          `{`,
		"top level object":   `{"client_id":"canvas-local"}`,
		"array item is null": `[null]`,
		"missing field":      `[{"client_id":"canvas-local","instance_id":"local"}]`,
		"null field":         `[{"client_id":null,"instance_id":"local","redirect_uri":"http://localhost:5173/callback"}]`,
		"nonstring field":    `[{"client_id":1,"instance_id":"local","redirect_uri":"http://localhost:5173/callback"}]`,
		"unknown field":      `[{"client_id":"canvas-local","instance_id":"local","redirect_uri":"http://localhost:5173/callback","name":"local"}]`,
		"duplicate key":      `[{"client_id":"canvas-local","client_id":"canvas-other","instance_id":"local","redirect_uri":"http://localhost:5173/callback"}]`,
	}
	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			t.Setenv("CANVAS_ACCOUNT_ADDITIONAL_CLIENTS", value)
			_, err := GetCanvasAccountConfigs()
			require.ErrorIs(t, err, errCanvasAccountConfiguration)
		})
	}
}

// TestCanvasAccountConfigsRejectUnsafeValues 验证回调来源、字段字符和长度限制。
func TestCanvasAccountConfigsRejectUnsafeValues(t *testing.T) {
	setCanvasAccountPrimaryEnv(t)

	tests := map[string]string{
		"remote http":        `[{"client_id":"canvas-local","instance_id":"local","redirect_uri":"http://canvas.example/callback"}]`,
		"credentials":        `[{"client_id":"canvas-local","instance_id":"local","redirect_uri":"https://user:pass@canvas.example/callback"}]`,
		"query":              `[{"client_id":"canvas-local","instance_id":"local","redirect_uri":"https://canvas.example/callback?code=1"}]`,
		"fragment":           `[{"client_id":"canvas-local","instance_id":"local","redirect_uri":"https://canvas.example/callback#fragment"}]`,
		"wildcard":           `[{"client_id":"canvas-*","instance_id":"local","redirect_uri":"https://canvas.example/callback"}]`,
		"control character":  `[{"client_id":"canvas-local","instance_id":"local\u000aother","redirect_uri":"https://canvas.example/callback"}]`,
		"overlong client id": fmt.Sprintf(`[{"client_id":"%s","instance_id":"local","redirect_uri":"https://canvas.example/callback"}]`, strings.Repeat("a", 65)),
		"overlong secret":    fmt.Sprintf(`[{"client_id":"canvas-local","instance_id":"local","redirect_uri":"https://canvas.example/callback","client_secret":"%s"}]`, strings.Repeat("s", canvasAccountClientSecretMaxLength+1)),
	}
	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			t.Setenv("CANVAS_ACCOUNT_ADDITIONAL_CLIENTS", value)
			_, err := GetCanvasAccountConfigs()
			require.ErrorIs(t, err, errCanvasAccountConfiguration)
		})
	}
}

// TestCanvasAccountConfigsLimitsAndSecretErrors 验证配置总量上限且错误不回显 secret。
func TestCanvasAccountConfigsLimitsAndSecretErrors(t *testing.T) {
	setCanvasAccountPrimaryEnv(t)

	entries := make([]string, canvasAccountAdditionalClientsMaxCount+1)
	for i := range entries {
		entries[i] = fmt.Sprintf(`{"client_id":"canvas-%d","instance_id":"local","redirect_uri":"https://canvas.example/callback"}`, i)
	}
	t.Setenv("CANVAS_ACCOUNT_ADDITIONAL_CLIENTS", `[`+strings.Join(entries, ",")+`]`)
	_, err := GetCanvasAccountConfigs()
	require.ErrorIs(t, err, errCanvasAccountConfiguration)

	secret := "synthetic-secret-must-not-appear"
	t.Setenv("CANVAS_ACCOUNT_ADDITIONAL_CLIENTS", fmt.Sprintf(`[{"client_id":"canvas-local","instance_id":"local","redirect_uri":"http://remote.example/callback","client_secret":"%s"}]`, secret))
	_, err = GetCanvasAccountConfigs()
	require.Error(t, err)
	assert.NotContains(t, err.Error(), secret)

	t.Setenv("CANVAS_ACCOUNT_ADDITIONAL_CLIENTS", strings.Repeat(" ", canvasAccountAdditionalClientsMaxSize+1))
	_, err = GetCanvasAccountConfigs()
	require.ErrorIs(t, err, errCanvasAccountConfiguration)
}

// TestCanvasAccountAdditionalSecretIsOpaque 保留附加客户端 secret 的原始字符语义。
func TestCanvasAccountAdditionalSecretIsOpaque(t *testing.T) {
	setCanvasAccountPrimaryEnv(t)
	secret := " synthetic*secret\t "
	encoded, err := Marshal([]map[string]string{{
		"client_id": "canvas-local", "instance_id": "local",
		"redirect_uri": "http://localhost:8080/v1/auth/newapi/callback", "client_secret": secret,
	}})
	require.NoError(t, err)
	t.Setenv("CANVAS_ACCOUNT_ADDITIONAL_CLIENTS", string(encoded))
	configs, err := GetCanvasAccountConfigs()
	require.NoError(t, err)
	require.Len(t, configs, 2)
	assert.Equal(t, secret, configs[1].ClientSecret)
}

// setCanvasAccountPrimaryEnv 设置各测试共享的旧单客户端部署配置。
func setCanvasAccountPrimaryEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CANVAS_ACCOUNT_ISSUER", " https://newapi.example.com/ ")
	t.Setenv("CANVAS_ACCOUNT_CLIENT_ID", " canvas-production ")
	t.Setenv("CANVAS_ACCOUNT_INSTANCE_ID", " production ")
	t.Setenv("CANVAS_ACCOUNT_REDIRECT_URI", " https://canvas.example.com/api/auth/callback/new-api ")
	t.Setenv("CANVAS_ACCOUNT_CLIENT_SECRET", "primary-secret")
	t.Setenv("CANVAS_ACCOUNT_ADDITIONAL_CLIENTS", "")
}
