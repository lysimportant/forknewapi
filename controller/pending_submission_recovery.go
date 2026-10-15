package controller

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaychannel "github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const pendingSubmissionRecoveryInterval = time.Minute

// StartPendingTaskRecovery continuously repairs the crash window between a
// provider accepting a task and the normal task row being finalized.
func StartPendingTaskRecovery(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	go func() {
		RecoverPendingTaskSubmissions(ctx)
		ticker := time.NewTicker(pendingSubmissionRecoveryInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				RecoverPendingTaskSubmissions(ctx)
			}
		}
	}()
}

// RecoverPendingTaskSubmissions claims and replays at most one bounded batch.
// Recovery is deliberately conservative: only an explicit non-2xx rejection
// is refundable; transport and parse ambiguity remains needs_review.
func RecoverPendingTaskSubmissions(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	tasks, err := model.GetPendingSubmissionTasks(100)
	if err != nil {
		logger.LogError(ctx, "query pending task submissions failed: "+err.Error())
		return
	}
	for _, task := range tasks {
		if ctx.Err() != nil {
			return
		}
		claimed, ok, claimErr := claimPendingSubmission(task.ID)
		if claimErr != nil {
			logger.LogError(ctx, fmt.Sprintf("claim pending task submission %d failed: %v", task.ID, claimErr))
			continue
		}
		if !ok {
			continue
		}
		recoverPendingTaskSubmission(ctx, claimed)
	}
}

func claimPendingSubmission(id int64) (*model.Task, bool, error) {
	if model.DB == nil {
		return nil, false, nil
	}
	var claimed *model.Task
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var task model.Task
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&task, id).Error; err != nil {
			return err
		}
		submission := task.PrivateData.Submission
		if submission == nil {
			return nil
		}
		now := time.Now().UnixMilli()
		if submission.Status == model.TaskSubmissionRecovering && submission.UpdatedAt >= now-5*60*1000 {
			return nil
		}
		if submission.Status != model.TaskSubmissionPending && submission.Status != model.TaskSubmissionRecovering {
			return nil
		}
		if task.Status == model.TaskStatusSuccess || (task.CanvasCancelReason != "" && task.Status == model.TaskStatusFailure) {
			return nil
		}
		submission.Status = model.TaskSubmissionRecovering
		submission.UpdatedAt = now
		if err := tx.Save(&task).Error; err != nil {
			return err
		}
		claimed = &task
		return nil
	})
	return claimed, claimed != nil, err
}

func recoverPendingTaskSubmission(ctx context.Context, task *model.Task) {
	if task == nil || task.PrivateData.Submission == nil {
		return
	}
	submission := task.PrivateData.Submission
	adaptor := relay.GetTaskAdaptorByPluginKey(submission.PluginKey)
	if adaptor == nil {
		markSubmissionNeedsReview(ctx, task, "task plugin unavailable during recovery")
		return
	}
	channelModel, err := model.CacheGetChannel(task.ChannelId)
	if err != nil || channelModel == nil {
		markSubmissionNeedsReview(ctx, task, "task channel unavailable during recovery")
		return
	}
	recoverable, ok := adaptor.(relaychannel.TaskSubmissionRecoveryAdaptor)
	if !ok {
		markSubmissionNeedsReview(ctx, task, "task plugin does not support submission recovery")
		return
	}
	baseURL := channelModel.GetBaseURL()
	if baseURL == "" {
		baseURL = constant.GetChannelBaseURL(channelModel.Type)
	}
	info := &relaycommon.RelayInfo{
		UserId:          task.UserId,
		UsingGroup:      task.Group,
		OriginModelName: task.Properties.OriginModelName,
		BillingSource:   task.PrivateData.BillingSource,
		SubscriptionId:  task.PrivateData.SubscriptionId,
		TokenId:         task.PrivateData.TokenId,
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{Action: task.Action, PublicTaskID: task.TaskID},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:          channelModel.Type,
			ChannelId:            channelModel.Id,
			UpstreamModelName:    task.Properties.UpstreamModelName,
			ChannelBaseUrl:       baseURL,
			ApiKey:               channelModel.Key,
			ChannelSetting:       channelModel.GetSetting(),
			ChannelOtherSettings: channelModel.GetOtherSettings(),
			ParamOverride:        channelModel.GetParamOverride(),
			HeadersOverride:      channelModel.GetHeaderOverride(),
		},
	}
	if task.PrivateData.Execution != nil {
		info.RequestId = task.PrivateData.Execution.RequestID
		info.RequestURLPath = task.PrivateData.Execution.RequestPath
	}
	if bc := task.PrivateData.BillingContext; bc != nil {
		info.PriceData.ModelPrice = bc.ModelPrice
		info.PriceData.ModelRatio = bc.ModelRatio
		info.PriceData.GroupRatioInfo.GroupRatio = bc.GroupRatio
		info.PriceData.ReplaceOtherRatios(bc.OtherRatios)
		info.TieredBillingSnapshot = bc.TieredSnapshot
	}
	snapshot := &relaychannel.TaskSubmissionSnapshot{
		PluginKey: submission.PluginKey, PluginVersion: submission.PluginVersion,
		URL: submission.URL, Method: submission.Method, Headers: submission.Headers,
		ResponseType: submission.ResponseType, NoRetry: submission.NoRetry,
		Context: submission.Context,
	}
	if err := recoverable.RestoreSubmissionSnapshot(snapshot); err != nil {
		markSubmissionNeedsReview(ctx, task, "restore task submission descriptor failed: "+err.Error())
		return
	}
	requestPath := info.RequestURLPath
	if requestPath == "" {
		requestPath = "/v1/videos"
	}
	recoveryCtx := newTaskRecoveryContext(ctx, submission.Method, requestPath)
	resp, err := recoverable.DoRequest(recoveryCtx, info, bytes.NewReader(submission.Body))
	if err != nil {
		markSubmissionNeedsReview(ctx, task, "recovery request outcome unknown: "+err.Error())
		return
	}
	if resp == nil {
		markSubmissionNeedsReview(ctx, task, "recovery returned no response")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusRequestTimeout && resp.StatusCode != http.StatusTooManyRequests {
			markSubmissionRejected(ctx, task, fmt.Sprintf("provider rejected recovered submission (HTTP %d)", resp.StatusCode))
			return
		}
		markSubmissionNeedsReview(ctx, task, fmt.Sprintf("recovery response may be ambiguous (HTTP %d)", resp.StatusCode))
		return
	}
	parsed, taskErr := recoverable.ParseResponse(recoveryCtx, resp, info)
	if taskErr != nil || parsed == nil || strings.TrimSpace(parsed.UpstreamTaskID) == "" {
		reason := "recovery response could not be parsed"
		if taskErr != nil {
			reason += ": " + taskErr.Message
		}
		markSubmissionNeedsReview(ctx, task, reason)
		return
	}
	finishRecoveredTask(ctx, task, parsed)
}

func newTaskRecoveryContext(parent context.Context, method, path string) *gin.Context {
	if method == "" {
		method = http.MethodPost
	}
	if path == "" {
		path = "/v1/videos"
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(strings.ToUpper(method), path, nil).WithContext(parent)
	return c
}

func finishRecoveredTask(ctx context.Context, task *model.Task, result *relaychannel.TaskSubmitResponse) {
	oldStatus := task.Status
	task.PrivateData.UpstreamTaskID = result.UpstreamTaskID
	task.Data = result.TaskData
	if len(result.PluginState) > 0 {
		task.PrivateData.PluginState = result.PluginState
	}
	if result.Immediate != nil {
		task.Status = model.TaskStatus(result.Immediate.Status)
		task.Progress = result.Immediate.Progress
		if result.Immediate.Status == model.TaskStatusFailure {
			task.Status = model.TaskStatusFailure
			task.FailReason = result.Immediate.Reason
			task.Progress = "100%"
		}
		if result.Immediate.Status == model.TaskStatusSuccess || result.Immediate.Status == model.TaskStatusFailure {
			task.FinishTime = time.Now().Unix()
		}
	}
	if task.PrivateData.Submission != nil {
		task.PrivateData.Submission.Status = model.TaskSubmissionCompleted
		task.PrivateData.Submission.LastError = ""
		task.PrivateData.Submission.UpdatedAt = time.Now().UnixMilli()
		task.PrivateData.Submission.Body = nil
		task.PrivateData.Submission.Headers = nil
		task.PrivateData.Submission.Context = nil
	}
	if won, err := task.UpdateWithStatus(oldStatus); err != nil || !won {
		if err != nil {
			logger.LogError(ctx, "persist recovered task failed: "+err.Error())
		}
		return
	}
	if task.Status == model.TaskStatusFailure {
		service.RefundTaskQuota(ctx, task, task.FailReason)
	}
	logger.LogInfo(ctx, fmt.Sprintf("recovered task submission task=%s upstream=%s", task.TaskID, result.UpstreamTaskID))
}

func markSubmissionNeedsReview(ctx context.Context, task *model.Task, reason string) {
	if task == nil || task.PrivateData.Submission == nil {
		return
	}
	task.PrivateData.Submission.Status = model.TaskSubmissionNeedsReview
	task.PrivateData.Submission.LastError = reason
	task.PrivateData.Submission.UpdatedAt = time.Now().UnixMilli()
	if err := task.Update(); err != nil {
		logger.LogError(ctx, "persist needs_review task submission failed: "+err.Error())
	}
}

func markSubmissionRejected(ctx context.Context, task *model.Task, reason string) {
	if task == nil || task.PrivateData.Submission == nil {
		return
	}
	oldStatus := task.Status
	task.Status = model.TaskStatusFailure
	task.Progress = "100%"
	task.FailReason = reason
	task.FinishTime = time.Now().Unix()
	task.PrivateData.Submission.Status = model.TaskSubmissionFailed
	task.PrivateData.Submission.LastError = reason
	task.PrivateData.Submission.UpdatedAt = time.Now().UnixMilli()
	if won, err := task.UpdateWithStatus(oldStatus); err != nil || !won {
		if err != nil {
			logger.LogError(ctx, "persist rejected task submission failed: "+err.Error())
		}
		return
	}
	service.RefundTaskQuota(ctx, task, reason)
}
