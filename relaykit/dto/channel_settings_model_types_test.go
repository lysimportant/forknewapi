package dto

import (
	"strings"
	"testing"
)

func TestChannelSettingsTaskPluginModelTypes(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		mediaType string
		valid     bool
	}{
		{"raw image", "无限制-Seedream-5-Pro-2K", "image", true},
		{"raw video", "provider/raw-video", "video", true},
		{"empty ID", "", "image", false},
		{"leading whitespace", " raw", "image", false},
		{"trailing whitespace", "raw ", "image", false},
		{"control", "raw\nmodel", "image", false},
		{"delimiter", "raw,model", "image", false},
		{"oversized", strings.Repeat("a", 201), "image", false},
		{"invalid UTF8", string([]byte{0xff}), "image", false},
		{"empty type", "raw", "", false},
		{"unknown type", "raw", "audio", false},
		{"noncanonical type", "raw", "Image", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setting := &ChannelSettings{TaskPluginModelTypes: map[string]string{tt.key: tt.mediaType}}
			if err := setting.ValidateHTTPTransport(); (err == nil) != tt.valid {
				t.Fatalf("ValidateHTTPTransport() = %v, valid = %t", err, tt.valid)
			}
		})
	}
	if err := (&ChannelSettings{}).ValidateHTTPTransport(); err != nil {
		t.Fatalf("legacy empty setting: %v", err)
	}
}
