package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractRedemptionKey(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "plain key", raw: "abc123", want: "abc123"},
		{name: "trims spaces", raw: "  abc123  ", want: "abc123"},
		{name: "uses first line from multiline copy", raw: "abc123\ndef456", want: "abc123"},
		{name: "strips legacy name tab prefix", raw: "batch-name\tabc123", want: "abc123"},
		{name: "empty", raw: "\n", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractRedemptionKey(tt.raw))
		})
	}
}
