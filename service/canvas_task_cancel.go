package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

var ErrCanvasTaskCompleted = errors.New("video task has already completed")
var ErrCanvasTaskNotVideo = errors.New("task is not a video task")

// CancelCanvasVideoTask shares the terminal-status CAS and receipt accounting
// barrier with polling. A losing poll cannot revive or charge a cancelled task.
// Only the receipt, never the refund helper's funding-only bool, proves a refund.
func CancelCanvasVideoTask(ctx context.Context, task *model.Task, reason string) error {
	if task.PrivateData.Execution == nil || !isCanvasVideoPath(task.PrivateData.Execution.RequestPath) {
		return ErrCanvasTaskNotVideo
	}
	if task.Status == model.TaskStatusSuccess {
		return ErrCanvasTaskCompleted
	}
	// Keep the intent even when the creation charge has not committed yet.
	// Stale poll updates cannot clear this marker or publish a late success.
	if err := model.DB.Model(&model.Task{}).
		Where("id = ? AND status <> ? AND canvas_cancel_reason = ?", task.ID, model.TaskStatusSuccess, "").
		Update("canvas_cancel_reason", reason).Error; err != nil {
		return err
	}
	if err := model.DB.First(task, task.ID).Error; err != nil {
		return err
	}
	if task.Status == model.TaskStatusSuccess {
		return ErrCanvasTaskCompleted
	}
	if !canvasTaskSubmissionReady(ctx, task) {
		return nil // The durable marker is resumed by the polling sweep.
	}
	wonTransition := false
	if task.Status != model.TaskStatusFailure {
		fromStatus := task.Status
		task.Status, task.Progress, task.FailReason = model.TaskStatusFailure, "100%", reason
		task.FinishTime = time.Now().Unix()
		won, err := task.UpdateWithStatus(fromStatus)
		if err != nil {
			return err
		}
		if !won {
			return nil // Re-read by the controller; a concurrent terminal transition won.
		}
		wonTransition = true
	}
	if !wonTransition {
		// Only a receipt's atomic accounting barrier can safely retry a refund.
		// Legacy tasks without receipts are refunded solely by the CAS winner.
		if !common.CanvasBridgeEnabled() || task.PrivateData.Execution == nil {
			return nil
		}
		receipt, err := model.GetCanvasReceipt(task.PrivateData.TokenId, task.UserId, task.PrivateData.Execution.RequestID)
		if err != nil || receipt.TaskID != task.TaskID || receipt.Status != model.CanvasReceiptPending ||
			!receipt.AccountingReady || receipt.AccountingFailed {
			return nil
		}
	}
	RefundTaskQuota(ctx, task, reason)
	return nil
}

// Resume cancellations independently of upstream availability and task terminal
// status, including a crash between the failure CAS and refund accounting.
func sweepCanvasTaskCancellations(ctx context.Context) {
	if !common.CanvasBridgeEnabled() {
		return
	}
	var tasks []*model.Task
	readyReceipts := model.DB.Model(&model.CanvasReceipt{}).Select("task_id").
		Where("status = ? AND accounting_ready = ? AND accounting_failed = ?", model.CanvasReceiptPending, true, false)
	unreadyReceipts := model.DB.Model(&model.CanvasReceipt{}).Select("task_id").
		Where("status = ? AND accounting_ready = ? AND accounting_failed = ?", model.CanvasReceiptPending, false, false)
	if model.DB.Where("canvas_cancel_reason <> ? AND status <> ?", "", model.TaskStatusSuccess).
		Where("status <> ? OR task_id IN (?)", model.TaskStatusFailure, readyReceipts).
		Where("task_id NOT IN (?)", unreadyReceipts).
		Order("updated_at ASC, id ASC").Limit(100).Find(&tasks).Error != nil {
		return
	}
	for _, task := range tasks {
		if ctx.Err() != nil {
			return
		}
		_ = CancelCanvasVideoTask(ctx, task, task.CanvasCancelReason)
	}
}

func isCanvasVideoPath(path string) bool {
	return path == "/v1/videos" || strings.HasPrefix(path, "/v1/videos/") ||
		path == "/v1/video/generations" || strings.HasPrefix(path, "/v1/video/generations/")
}
