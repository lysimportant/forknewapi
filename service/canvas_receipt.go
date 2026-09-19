package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"gorm.io/gorm"
)

// beginCanvasReceipt 在移动额度前保存创建请求身份和换算快照；关闭桥接时无副作用。
// Playground 没有 API 令牌，不属于对外回执的结算范围。
func beginCanvasReceipt(info *relaycommon.RelayInfo) error {
	if !common.CanvasBridgeEnabled() || info == nil || info.IsPlayground {
		return nil
	}
	quotaPerUnit, pricingVersion := common.QuotaPerUnit, ""
	if snap := info.TieredBillingSnapshot; snap != nil {
		quotaPerUnit, pricingVersion = snap.QuotaPerUnit, snap.ExprHash
	}
	receipt := &model.CanvasReceipt{TokenID: info.TokenId, UserID: info.UserId, RequestID: info.RequestId,
		ModelName: info.OriginModelName, Group: info.UsingGroup,
		QuotaPerUnit: strconv.FormatFloat(quotaPerUnit, 'f', -1, 64), PricingVersion: pricingVersion}
	if info.TaskRelayInfo != nil {
		receipt.TaskID = info.TaskRelayInfo.PublicTaskID
	}
	_, err := model.EnsureCanvasReceipt(receipt)
	return err
}

// PrepareCanvasTaskReceipt 在任务持久化并可被轮询前建立提交记账屏障。
// 免费任务也需要调用；初始化失败时提交入口必须阻止任务进入可轮询状态。
func PrepareCanvasTaskReceipt(info *relaycommon.RelayInfo) error {
	return beginCanvasReceipt(info)
}

// MarkCanvasReceiptUnverified 标记实际计费无法确认的请求；日志和预估额度不能成为最终回执。
// 仅桥接启用时写入失败屏障，保留原有调用和计费错误处理。
func MarkCanvasReceiptUnverified(info *relaycommon.RelayInfo) {
	if !common.CanvasBridgeEnabled() || info == nil || info.IsPlayground {
		return
	}
	if err := beginCanvasReceipt(info); err != nil {
		common.SysLog("Canvas 待核对回执初始化失败: " + err.Error())
		return
	}
	if err := model.MarkCanvasReceiptAccounting(info.TokenId, info.UserId, info.RequestId, false); err != nil {
		common.SysLog("Canvas 待核对回执写入失败: " + err.Error())
	}
}

// finishCanvasRelayReceipt 只在资金与令牌记账都成功后调用；任务提交仅设置记账屏障。
// 不使用数据库中的任务状态推断立即完成，避免轮询早于提交记账完成时误结算。
func finishCanvasRelayReceipt(info *relaycommon.RelayInfo, quota int, accountingErr error) error {
	if !common.CanvasBridgeEnabled() || info == nil || info.IsPlayground {
		return nil
	}
	if err := model.MarkCanvasReceiptAccounting(info.TokenId, info.UserId, info.RequestId, accountingErr == nil); err != nil {
		return err
	}
	if accountingErr != nil {
		return nil
	}
	if info.TaskRelayInfo != nil {
		return nil
	}
	return model.FinalizeCanvasReceipt(info.TokenId, info.UserId, info.RequestId, model.CanvasReceiptSettled, int64(quota))
}

// FinalizeCanvasImmediateTaskReceipt 仅由明确收到同步终态的提交入口在成功记账后调用。
// 表达式失败或记账屏障未完成时保留 pending，不根据后续轮询状态推断实际扣费。
func FinalizeCanvasImmediateTaskReceipt(ctx context.Context, info *relaycommon.RelayInfo, task *model.Task) {
	if !common.CanvasBridgeEnabled() || info == nil || info.IsPlayground || task == nil {
		return
	}
	status := model.CanvasReceiptSettled
	if task.Status == model.TaskStatusFailure && task.Quota == 0 {
		status = model.CanvasReceiptRefunded
	} else if task.Status != model.TaskStatusSuccess {
		return
	}
	if err := model.FinalizeCanvasReceipt(info.TokenId, info.UserId, info.RequestId, status, int64(task.Quota)); err != nil {
		logger.LogError(ctx, fmt.Sprintf("Canvas 立即完成回执需核对 task %s: %v", task.TaskID, err))
	}
}

// finishCanvasTaskReceipt 根据原始提交身份完成异步回执；缺失回执的旧任务不补造记账证明。
// accounted 为 false 时永久保留 pending，避免部分成功后的重试误报完整记账。
func finishCanvasTaskReceipt(ctx context.Context, task *model.Task, status string, quota int, accounted bool) {
	if !common.CanvasBridgeEnabled() || task == nil || task.PrivateData.Execution == nil || task.PrivateData.TokenId <= 0 {
		return
	}
	requestID := task.PrivateData.Execution.RequestID
	receipt, err := model.GetCanvasReceipt(task.PrivateData.TokenId, task.UserId, requestID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return
	}
	if err == nil && receipt.TaskID != task.TaskID {
		err = fmt.Errorf("canvas receipt task identity mismatch")
	}
	if err == nil {
		err = model.MarkCanvasReceiptAccounting(task.PrivateData.TokenId, task.UserId, requestID, accounted)
		if err == nil && accounted {
			err = model.FinalizeCanvasReceipt(task.PrivateData.TokenId, task.UserId, requestID, status, int64(quota))
		}
	}
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("Canvas 回执保持待核对 task %s: %v", task.TaskID, err))
	}
}

// beginCanvasTaskReceipt 在异步调整前关闭记账屏障；旧任务无需回执，继续原有行为。
// 已开始但未完成的记账不会自动重放，避免在部分成功后再次移动资金。
func beginCanvasTaskReceipt(ctx context.Context, task *model.Task) bool {
	if !common.CanvasBridgeEnabled() || task == nil || task.PrivateData.Execution == nil || task.PrivateData.TokenId <= 0 {
		return true
	}
	requestID := task.PrivateData.Execution.RequestID
	receipt, err := model.GetCanvasReceipt(task.PrivateData.TokenId, task.UserId, requestID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true
	}
	if err == nil && receipt.TaskID != task.TaskID {
		err = fmt.Errorf("canvas receipt task identity mismatch")
	}
	if err == nil {
		err = model.BeginCanvasReceiptAdjustment(task.PrivateData.TokenId, task.UserId, requestID)
	}
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("Canvas 任务记账需核对 task %s: %v", task.TaskID, err))
		return false
	}
	return true
}

// canvasTaskSubmissionReady 在任务落终态前等待提交记账完成，避免漏掉之后的差额结算或退款。
// 旧任务无回执时保持原行为；提交已明确失败时允许保存任务结果，但回执仍待核对。
func canvasTaskSubmissionReady(ctx context.Context, task *model.Task) bool {
	if !common.CanvasBridgeEnabled() || task == nil || task.PrivateData.Execution == nil || task.PrivateData.TokenId <= 0 {
		return true
	}
	receipt, err := model.GetCanvasReceipt(task.PrivateData.TokenId, task.UserId, task.PrivateData.Execution.RequestID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true
	}
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("Canvas 提交记账状态读取失败 task %s: %v", task.TaskID, err))
		return false
	}
	return receipt.TaskID == task.TaskID && (receipt.AccountingReady || receipt.AccountingFailed)
}
