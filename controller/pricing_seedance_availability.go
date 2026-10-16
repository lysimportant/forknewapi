package controller

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// 独立缓存避免与 Yuanliu 的相同渠道 ID 或插件切换结果交叉污染。
var pricingSeedanceCatalogCache = yuanliuCatalogCache{entries: make(map[int]yuanliuCatalogSnapshot)}

func fetchSeedanceModelNames(channel *model.Channel) (map[string]string, error) {
	return fetchPricingTaskPluginModelNames(channel, "seedance-video", true)
}

// GetSeedanceAvailability 返回当前用户可见 Seedance Unified Video 渠道的目录状态。
// 有效目录缺少模型（包括空目录）为 disabled；失败为 unknown；不会修改渠道或模型。
// data.models 使用公开模型名，status/checked_at、缓存、超时和多 Key 保守判断沿用 Yuanliu。
func GetSeedanceAvailability(c *gin.Context) {
	getPricingTaskPluginAvailability(c, "seedance-video", &pricingSeedanceCatalogCache, fetchSeedanceModelNames)
}
