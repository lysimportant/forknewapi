package common_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
)

func TestWanEndpointsDistinguishImagesFromVideos(t *testing.T) {
	for _, name := range []string{
		"wan2.7-image-pro", "wan2.7-image", "wan2.6-image", "wan2.6-t2i",
		"wan2.5-t2i-preview", "wan2.2-t2i-flash", "wan2.2-t2i-plus",
		"wanx2.1-t2i-turbo", "wanx2.1-t2i-plus", "wanx2.0-t2i-turbo",
	} {
		t.Run(name, func(t *testing.T) {
			assert.Contains(t, common.GetEndpointTypesByChannelType(constant.ChannelTypeAli, name), constant.EndpointTypeImageGeneration)
		})
	}
	for _, name := range []string{
		"wanx2.1-t2v-plus", "wanx2.1-t2v-turbo", "wanx2.1-i2v-plus", "wanx2.1-i2v-turbo",
	} {
		t.Run(name, func(t *testing.T) {
			assert.NotContains(t, common.GetEndpointTypesByChannelType(constant.ChannelTypeAli, name), constant.EndpointTypeImageGeneration)
		})
	}
}

// TestGlobalStoreImageFamiliesUseImageEndpoints 锁定 GlobalStore 模型中心的图片家族识别。
// 漏识别会让原生渠道把图片模型按 /v1/chat/completions 暴露给 Canvas，并让定价面板丢掉 image_count。
func TestGlobalStoreImageFamiliesUseImageEndpoints(t *testing.T) {
	// 非插件原生渠道是 GlobalStore 图片模型的落点，与 Ali 同形（无专属 case，落到 default）。
	for _, name := range []string{
		"gpt-image-2", "gpt-image-2.5",
		"banana2", "bananapro", "banana_c1", "banana_c2",
		"seedream-5.0", "seedream_5.0Pro",
	} {
		t.Run(name, func(t *testing.T) {
			assert.True(t, common.IsImageGenerationModel(name))
			assert.Contains(t, common.GetEndpointTypesByChannelType(constant.ChannelTypeAli, name), constant.EndpointTypeImageGeneration)
		})
	}
	for _, name := range []string{
		"grok_video", "grok_1.5", "omni_720P", "veo3.1-fast",
		"wan3.0", "wan3.0-Prime", "sd_2.0_discount", "sd_2.5_special",
		"seedance_2.0_nsfw_c1", "seedance_2_0_x1_official",
	} {
		t.Run(name, func(t *testing.T) {
			assert.False(t, common.IsImageGenerationModel(name))
			assert.NotContains(t, common.GetEndpointTypesByChannelType(constant.ChannelTypeAli, name), constant.EndpointTypeImageGeneration)
		})
	}
}

// /v1/models reports the DeepSeek Search and Fetch routing names with their
// own endpoint on the channels that relay them; chat models keep their list.
func TestDeepSeekToolRoutingNamesReportTheirEndpoint(t *testing.T) {
	for _, channelType := range []int{constant.ChannelTypeDeepSeek, constant.ChannelTypeNewAPI} {
		assert.Equal(t, []constant.EndpointType{constant.EndpointTypeDeepSeekToolsSearch}, common.GetEndpointTypesByChannelType(channelType, constant.DeepSeekSearchModel))
		assert.Equal(t, []constant.EndpointType{constant.EndpointTypeDeepSeekToolsFetch}, common.GetEndpointTypesByChannelType(channelType, constant.DeepSeekFetchModel))
	}
	assert.Equal(t, []constant.EndpointType{constant.EndpointTypeOpenAI}, common.GetEndpointTypesByChannelType(constant.ChannelTypeDeepSeek, "deepseek-flash"))
}
