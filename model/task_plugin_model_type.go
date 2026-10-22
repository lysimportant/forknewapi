package model

import (
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// TaskPluginModelType resolves an exact upstream ID. Saved catalog types take
// precedence over the plugin's compatibility metadata; unknown IDs stay unknown.
func TaskPluginModelType(plugin *jsplugin.LoadedPlugin, setting dto.ChannelSettings, upstreamModel string) string {
	mediaType, exists := setting.TaskPluginModelTypes[upstreamModel]
	if !exists && plugin != nil {
		mediaType = plugin.Meta.ModelMediaTypes[upstreamModel]
	}
	if mediaType == "image" || mediaType == "video" {
		return mediaType
	}
	return ""
}
