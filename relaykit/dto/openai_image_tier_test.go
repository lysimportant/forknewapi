package dto

import (
	"encoding/json"
	"fmt"
	"testing"

	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageRequestTierPreservesAspectRatio(t *testing.T) {
	for _, size := range []string{"1K", "2K", "3K", "4K", "1k", "2k", "3k", "4k", "", "auto", "AUTO"} {
		t.Run(size, func(t *testing.T) {
			var request ImageRequest
			input := fmt.Sprintf(`{"model":"provider-image","prompt":"test","size":%q,"aspect_ratio":"16:9","provider_option":false,"parameters":{"n":2}}`, size)
			require.NoError(t, kitutil.Unmarshal([]byte(input), &request))
			expectedExtra := map[string]json.RawMessage{
				"aspect_ratio":    json.RawMessage(`"16:9"`),
				"provider_option": json.RawMessage(`false`),
				"parameters":      json.RawMessage(`{"n":2}`),
			}
			require.Equal(t, expectedExtra, request.Extra)

			encoded, err := kitutil.Marshal(request)
			require.NoError(t, err)
			var payload map[string]json.RawMessage
			require.NoError(t, kitutil.Unmarshal(encoded, &payload))
			if size == "" {
				assert.NotContains(t, payload, "size")
			} else {
				assert.JSONEq(t, fmt.Sprintf("%q", size), string(payload["size"]))
			}
			assert.JSONEq(t, `"16:9"`, string(payload["aspect_ratio"]))
			assert.NotContains(t, payload, "provider_option")
			assert.NotContains(t, payload, "parameters")
			assert.Equal(t, size, request.Size)
			assert.Equal(t, expectedExtra, request.Extra)
		})
	}
}

func TestImageRequestTierDoesNotChangeOtherSizes(t *testing.T) {
	for _, size := range []string{"1024x1024", "1152x2048", "5K", " 1K "} {
		t.Run(size, func(t *testing.T) {
			request := ImageRequest{
				Model: "provider-image",
				Size:  size,
				Extra: map[string]json.RawMessage{"aspect_ratio": json.RawMessage(`"16:9"`)},
			}
			encoded, err := kitutil.Marshal(request)
			require.NoError(t, err)
			var payload map[string]json.RawMessage
			require.NoError(t, kitutil.Unmarshal(encoded, &payload))
			assert.NotContains(t, payload, "aspect_ratio")
			if size == "" {
				assert.NotContains(t, payload, "size")
			} else {
				assert.JSONEq(t, fmt.Sprintf("%q", size), string(payload["size"]))
			}
			assert.Equal(t, size, request.Size)
			assert.Equal(t, json.RawMessage(`"16:9"`), request.Extra["aspect_ratio"])
		})
	}
}

func TestImageRequestTierOmitsInvalidAspectRatio(t *testing.T) {
	cases := []struct {
		name string
		raw  json.RawMessage
	}{
		{name: "nil"},
		{name: "empty", raw: json.RawMessage(`""`)},
		{name: "whitespace", raw: json.RawMessage(`" \t\n"`)},
		{name: "null", raw: json.RawMessage(`null`)},
		{name: "number", raw: json.RawMessage(`1.5`)},
		{name: "boolean", raw: json.RawMessage(`true`)},
		{name: "object", raw: json.RawMessage(`{"ratio":"16:9"}`)},
		{name: "array", raw: json.RawMessage(`["16:9"]`)},
		{name: "malformed", raw: json.RawMessage(`"16:9`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			original := append(json.RawMessage(nil), tc.raw...)
			request := ImageRequest{
				Model: "provider-image",
				Size:  "2K",
				Extra: map[string]json.RawMessage{"aspect_ratio": tc.raw},
			}
			encoded, err := kitutil.Marshal(request)
			require.NoError(t, err)
			var payload map[string]json.RawMessage
			require.NoError(t, kitutil.Unmarshal(encoded, &payload))
			assert.NotContains(t, payload, "aspect_ratio")
			assert.Equal(t, original, request.Extra["aspect_ratio"])
		})
	}

	t.Run("absent", func(t *testing.T) {
		request := ImageRequest{Model: "provider-image", Size: "4K"}
		encoded, err := kitutil.Marshal(request)
		require.NoError(t, err)
		var payload map[string]json.RawMessage
		require.NoError(t, kitutil.Unmarshal(encoded, &payload))
		assert.NotContains(t, payload, "aspect_ratio")
		assert.Nil(t, request.Extra)
	})
}
