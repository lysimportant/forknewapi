package controller

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// canvasReceiptResponse 只公开调用方自己的请求结果，不公开用户、令牌或渠道凭据。
type canvasReceiptResponse struct {
	Version        int    `json:"version"`
	RequestID      string `json:"request_id"`
	TaskID         string `json:"task_id,omitempty"`
	Model          string `json:"model"`
	Group          string `json:"group"`
	Status         string `json:"status"`
	Quota          string `json:"quota"`
	QuotaPerUnit   string `json:"quota_per_unit"`
	PricingVersion string `json:"pricing_version,omitempty"`
	SettledAt      string `json:"settled_at,omitempty"`
}

// GetCanvasReceipt 返回 TokenAuthReadOnly 已验证令牌自己的权威回执。
// 过期或耗尽额度的令牌可读；禁用、归属不符和 IP 限制在服务端重新校验。
func GetCanvasReceipt(c *gin.Context) {
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
	requestID := c.Param("requestId")
	if !model.ValidCanvasReceiptRequestID(requestID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request_id", "message": "Invalid request ID"}})
		return
	}
	receipt, err := model.GetCanvasReceipt(token.Id, token.UserId, requestID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "receipt_not_found", "message": "Receipt not found"}})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "receipt_unavailable", "message": "Receipt is unavailable"}})
		return
	}
	response := canvasReceiptResponse{Version: 1, RequestID: receipt.RequestID, TaskID: receipt.TaskID,
		Model: receipt.ModelName, Group: receipt.Group, Status: receipt.Status,
		Quota: strconv.FormatInt(receipt.Quota, 10), QuotaPerUnit: receipt.QuotaPerUnit, PricingVersion: receipt.PricingVersion}
	if receipt.SettledAt > 0 {
		response.SettledAt = time.UnixMilli(receipt.SettledAt).UTC().Format(time.RFC3339Nano)
	}
	c.JSON(http.StatusOK, response)
}
