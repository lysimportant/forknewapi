package router

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

// SetCanvasRouter 注册默认关闭的只读计费桥接；预估不进入中继、预扣或上游提交流程。
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
}
