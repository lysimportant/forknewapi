package router

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

// SetCanvasRouter 注册默认关闭的计费桥接；取消只使用原任务身份，预估不提交上游请求。
func SetCanvasRouter(router *gin.Engine) {
	bridge := router.Group("/v1/canvas", middleware.RouteTag("relay"), func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if !common.CanvasBridgeEnabled() {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "canvas_bridge_disabled", "message": "Canvas 计费桥接尚未启用"}})
			return
		}
		c.Next()
	}, middleware.GlobalAPIRateLimit())
	bridge.GET("/catalog", middleware.TokenAuth(), controller.GetCanvasCatalog)
	bridge.POST("/estimate", middleware.TokenAuth(), middleware.UserCriticalRateLimit("canvas_estimate"), controller.EstimateCanvasPrice)
	bridge.GET("/receipts/:requestId", middleware.TokenAuthReadOnly(), controller.GetCanvasReceipt)
	bridge.POST("/tasks/:taskId/cancel", middleware.TokenAuthReadOnly(), middleware.UserCriticalRateLimit("canvas_task_cancel"), controller.CancelCanvasVideoTask)
}
