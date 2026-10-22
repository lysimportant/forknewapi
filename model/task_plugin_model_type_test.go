package model

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

func TestTaskPluginModelType(t *testing.T) {
	plugin := &jsplugin.LoadedPlugin{Meta: jsplugin.Meta{ModelMediaTypes: map[string]string{"raw": "image", "Static-Video": "video"}}}
	setting := dto.ChannelSettings{TaskPluginModelTypes: map[string]string{"raw": "video", "dynamic-image": "image", "invalid": "audio"}}
	for _, tt := range []struct{ id, want string }{
		{"raw", "video"}, {"dynamic-image", "image"}, {"Static-Video", "video"},
		{"static-video", ""}, {" raw", ""}, {"unknown-image-video", ""}, {"invalid", ""},
	} {
		if got := TaskPluginModelType(plugin, setting, tt.id); got != tt.want {
			t.Errorf("TaskPluginModelType(%q) = %q, want %q", tt.id, got, tt.want)
		}
	}
	if got := TaskPluginModelType(nil, dto.ChannelSettings{}, "raw"); got != "" {
		t.Fatalf("nil plugin = %q", got)
	}
}
