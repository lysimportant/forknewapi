package controller

import (
	"net/http"
	"strconv"

	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

// GetPerfMetricsSummary 仅使用当前角色可见的分组生成模型性能汇总，避免隐藏样本进入聚合。
func GetPerfMetricsSummary(c *gin.Context) {
	hours := 24
	if rawHours := c.Query("hours"); rawHours != "" {
		if parsed, err := strconv.Atoi(rawHours); err == nil {
			hours = parsed
		}
	}

	activeGroups := []string{"auto"}
	for group := range ratio_setting.GetGroupRatioCopy() {
		if service.IsGroupVisible(group, c.GetInt("role")) {
			activeGroups = append(activeGroups, group)
		}
	}
	result, err := perfmetrics.QuerySummaryAll(hours, activeGroups)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// GetPerfMetrics 返回模型的可见分组统计；显式查询不可见分组时返回空分组列表。
func GetPerfMetrics(c *gin.Context) {
	modelName := c.Query("model")
	if modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "model is required",
		})
		return
	}

	hours := 24
	if rawHours := c.Query("hours"); rawHours != "" {
		if parsed, err := strconv.Atoi(rawHours); err == nil {
			hours = parsed
		}
	}

	result, err := perfmetrics.Query(perfmetrics.QueryParams{
		Model: modelName,
		Group: c.Query("group"),
		Hours: hours,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	result.Groups = filterActiveGroups(result.Groups, c.GetInt("role"))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// filterActiveGroups 保留仍配置且对当前角色可见的分组，自动分组沿用原有展示规则。
func filterActiveGroups(groups []perfmetrics.GroupResult, role int) []perfmetrics.GroupResult {
	activeRatios := ratio_setting.GetGroupRatioCopy()
	return lo.Filter(groups, func(g perfmetrics.GroupResult, _ int) bool {
		_, ok := activeRatios[g.Group]
		return (ok || g.Group == "auto") && service.IsGroupVisible(g.Group, role)
	})
}
