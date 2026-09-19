package model

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Canvas 回执只允许从待结算转为一个不可修改的终态。
const (
	CanvasReceiptPending  = "pending"
	CanvasReceiptSettled  = "settled"
	CanvasReceiptRefunded = "refunded"
)

// canvasReceiptRequestID 限制网关请求标识的长度和字符，不接受截断后的标识。
var canvasReceiptRequestID = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)

// CanvasReceipt 保存实际记账结果；身份字段只供服务端鉴权，不直接序列化返回。
// Quota 使用上游额度单位，QuotaPerUnit 是创建时每货币单位对应额度的十进制快照。
type CanvasReceipt struct {
	ID               int64  `gorm:"primaryKey"`
	TokenID          int    `gorm:"not null;uniqueIndex:idx_canvas_receipt_token_request,priority:1"`
	RequestID        string `gorm:"type:varchar(64);not null;uniqueIndex:idx_canvas_receipt_token_request,priority:2"`
	UserID           int    `gorm:"not null;index"`
	TaskID           string `gorm:"type:varchar(191)"`
	ModelName        string `gorm:"column:model;type:varchar(512);not null"`
	Group            string `gorm:"type:varchar(191);not null"`
	Status           string `gorm:"type:varchar(16);not null"`
	Quota            int64  `gorm:"not null"`
	QuotaPerUnit     string `gorm:"type:varchar(64);not null"`
	PricingVersion   string `gorm:"type:varchar(80)"`
	AccountingReady  bool   `gorm:"not null"`
	AccountingFailed bool   `gorm:"not null"`
	CreatedAt        int64  `gorm:"not null"`
	UpdatedAt        int64  `gorm:"not null"`
	SettledAt        int64  `gorm:"not null"`
}

// ValidCanvasReceiptRequestID 校验创建请求标识，不接受客户端轮询请求的替代标识。
func ValidCanvasReceiptRequestID(requestID string) bool {
	return requestID != "." && requestID != ".." && canvasReceiptRequestID.MatchString(requestID)
}

// EnsureCanvasReceipt 幂等创建待结算回执；同一令牌和请求只能有一条记录。
// 返回数据库错误或身份冲突；已有终态和换算快照不会被后续调用覆盖。
func EnsureCanvasReceipt(receipt *CanvasReceipt) (*CanvasReceipt, error) {
	unit, validUnit := new(big.Rat).SetString(receipt.QuotaPerUnit)
	if receipt.TokenID <= 0 || receipt.UserID <= 0 || !ValidCanvasReceiptRequestID(receipt.RequestID) ||
		!validUnit || unit.Sign() <= 0 || len(receipt.QuotaPerUnit) > 64 || receipt.ModelName == "" ||
		len(receipt.ModelName) > 512 || len(receipt.TaskID) > 191 || len(receipt.Group) > 191 || len(receipt.PricingVersion) > 80 {
		return nil, errors.New("invalid canvas receipt identity or quota unit")
	}
	now := time.Now().UnixMilli()
	candidate := *receipt
	candidate.ID = 0
	candidate.Status = CanvasReceiptPending
	candidate.Quota, candidate.SettledAt = 0, 0
	candidate.AccountingReady, candidate.AccountingFailed = false, false
	candidate.CreatedAt, candidate.UpdatedAt = now, now
	if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&candidate).Error; err != nil {
		return nil, err
	}
	existing, err := GetCanvasReceipt(receipt.TokenID, receipt.UserID, receipt.RequestID)
	if err != nil {
		return nil, err
	}
	if existing.ModelName != receipt.ModelName || (existing.TaskID != "" && receipt.TaskID != "" && existing.TaskID != receipt.TaskID) {
		return nil, errors.New("canvas receipt identity conflict")
	}
	if existing.Status == CanvasReceiptPending {
		updates := map[string]any{"group": receipt.Group, "updated_at": now}
		// Legacy 结算仍读取运行中换算设置，期间变化时保留原快照并要求人工核对。
		if existing.QuotaPerUnit != receipt.QuotaPerUnit {
			updates["accounting_failed"] = true
		}
		if receipt.TaskID != "" {
			updates["task_id"] = receipt.TaskID
		}
		if err := DB.Model(&CanvasReceipt{}).Where("id = ? AND status = ?", existing.ID, CanvasReceiptPending).Updates(updates).Error; err != nil {
			return nil, err
		}
	}
	return GetCanvasReceipt(receipt.TokenID, receipt.UserID, receipt.RequestID)
}

// GetCanvasReceipt 同时约束令牌、用户和创建请求；越权与缺失统一返回 ErrRecordNotFound。
func GetCanvasReceipt(tokenID, userID int, requestID string) (*CanvasReceipt, error) {
	var receipt CanvasReceipt
	err := DB.Where("token_id = ? AND user_id = ? AND request_id = ?", tokenID, userID, requestID).First(&receipt).Error
	if err == nil && receipt.RequestID != requestID {
		return nil, gorm.ErrRecordNotFound
	}
	return &receipt, err
}

// MarkCanvasReceiptAccounting 记录记账屏障；任一次失败都会阻止自动生成终态回执。
// success 只在资金来源和令牌写入都成功后传 true，不可用日志或任务状态代替。
func MarkCanvasReceiptAccounting(tokenID, userID int, requestID string, success bool) error {
	receipt, err := GetCanvasReceipt(tokenID, userID, requestID)
	if err != nil {
		return err
	}
	updates := map[string]any{"updated_at": time.Now().UnixMilli()}
	if success {
		updates["accounting_ready"] = true
	} else {
		updates["accounting_failed"] = true
	}
	return DB.Model(&CanvasReceipt{}).
		Where("id = ? AND status = ?", receipt.ID, CanvasReceiptPending).
		Updates(updates).Error
}

// FinalizeCanvasReceipt 在已成功记账且从未失败时原子写入终态；重复相同结果幂等。
// 终态金额、换算快照和身份不可修改；未过记账屏障或相反结果返回错误。
func FinalizeCanvasReceipt(tokenID, userID int, requestID, status string, quota int64) error {
	if quota < 0 || (status != CanvasReceiptSettled && status != CanvasReceiptRefunded) || (status == CanvasReceiptRefunded && quota != 0) {
		return errors.New("invalid canvas receipt settlement")
	}
	receipt, err := GetCanvasReceipt(tokenID, userID, requestID)
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	result := DB.Model(&CanvasReceipt{}).
		Where("id = ? AND status = ? AND accounting_ready = ? AND accounting_failed = ?",
			receipt.ID, CanvasReceiptPending, true, false).
		Updates(map[string]any{"status": status, "quota": quota, "settled_at": now, "updated_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	receipt, err = GetCanvasReceipt(tokenID, userID, requestID)
	if err != nil {
		return err
	}
	if receipt.Status == status && receipt.Quota == quota {
		return nil
	}
	return fmt.Errorf("canvas receipt is not ready or is immutable: %s", receipt.Status)
}

// BeginCanvasReceiptAdjustment 原子关闭异步记账屏障，防止部分写入后被重试误判成功。
// 只允许已完成提交记账的 pending 回执开始一次调整；失败或并发调用返回错误。
func BeginCanvasReceiptAdjustment(tokenID, userID int, requestID string) error {
	receipt, err := GetCanvasReceipt(tokenID, userID, requestID)
	if err != nil {
		return err
	}
	result := DB.Model(&CanvasReceipt{}).
		Where("id = ? AND status = ? AND accounting_ready = ? AND accounting_failed = ?",
			receipt.ID, CanvasReceiptPending, true, false).
		Updates(map[string]any{"accounting_ready": false, "updated_at": time.Now().UnixMilli()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("canvas receipt accounting is incomplete or already finalized")
	}
	return nil
}
