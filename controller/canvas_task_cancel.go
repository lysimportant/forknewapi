package controller

import (
	"errors"
	"net"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// CancelCanvasVideoTask only operates on the authenticated token's own video.
// Missing tasks and foreign tasks deliberately have the same response.
func CancelCanvasVideoTask(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !common.CanvasBridgeEnabled() {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "canvas_bridge_disabled", "message": "Canvas bridge is disabled"}})
		return
	}
	token, err := model.GetTokenById(c.GetInt("token_id"))
	if err != nil || token.UserId != c.GetInt("id") || token.Status == common.TokenStatusDisabled {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "invalid_token", "message": "Invalid API token"}})
		return
	}
	if allowed := token.GetIpLimits(); len(allowed) > 0 {
		ip := net.ParseIP(c.ClientIP())
		if ip == nil || !common.IsIpInCIDRList(ip, allowed) {
			c.JSON(http.StatusForbidden, gin.H{"error": gin.H{"code": "ip_not_allowed", "message": "IP address is not allowed"}})
			return
		}
	}
	var input struct {
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_reason", "message": "Invalid cancellation reason"}})
		return
	}
	reasons := map[string]string{"user_cancelled": "用户取消视频生成", "task_not_found": "视频查询未找到任务", "task_failed": "视频平台任务失败"}
	reason, valid := reasons[input.Reason]
	if !valid {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_reason", "message": "Invalid cancellation reason"}})
		return
	}
	task, exists, err := model.GetByTaskId(c.GetInt("id"), c.Param("taskId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "task_unavailable", "message": "Task unavailable"}})
		return
	}
	if !exists || task.TaskID != c.Param("taskId") || task.PrivateData.TokenId != token.Id {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "task_not_found", "message": "Task not found; refund requires reconciliation"}})
		return
	}
	err = service.CancelCanvasVideoTask(c.Request.Context(), task, reason)
	if errors.Is(err, service.ErrCanvasTaskCompleted) || errors.Is(err, service.ErrCanvasTaskNotVideo) {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "task_not_cancellable", "message": "Task has completed or is not a video task"}})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "cancel_failed", "message": "Cancellation failed; refund requires reconciliation"}})
		return
	}
	task, exists, err = model.GetByTaskId(c.GetInt("id"), c.Param("taskId"))
	if err != nil || !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "task_unavailable", "message": "Task unavailable"}})
		return
	}
	refundStatus := model.CanvasReceiptPending
	if execution := task.PrivateData.Execution; execution != nil {
		if receipt, err := model.GetCanvasReceipt(task.PrivateData.TokenId, task.UserId, execution.RequestID); err == nil && receipt.TaskID == task.TaskID {
			refundStatus = receipt.Status
		}
	}
	status := http.StatusAccepted
	if refundStatus == model.CanvasReceiptRefunded && task.Status == model.TaskStatusFailure {
		status = http.StatusOK
	}
	if task.Status == model.TaskStatusSuccess {
		status = http.StatusConflict
	}
	c.JSON(status, gin.H{"task_id": task.TaskID, "status": task.Status.ToVideoStatus(), "refund_status": refundStatus})
}
