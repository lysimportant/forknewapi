package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanvasVideoCancellationAccounting(t *testing.T) {
	t.Setenv("CANVAS_BRIDGE_ENABLED", "true")
	require.NoError(t, model.DB.AutoMigrate(&model.CanvasReceipt{}))
	for _, scenario := range []string{"cancel", "pending_submission", "already_failed", "completed", "non_video", "partial_token", "legacy"} {
		t.Run(scenario, func(t *testing.T) {
			truncate(t)
			t.Cleanup(func() { model.DB.Exec("DELETE FROM canvas_receipts") })
			user := &model.User{Id: 71}
			token := &model.Token{Id: 72}
			channel := &model.Channel{Id: 73}
			seedUser(t, user.Id, 900)
			seedToken(t, token.Id, user.Id, "synthetic-cancel-token", 900)
			seedChannel(t, channel.Id)
			task := makeTask(user.Id, channel.Id, 100, token.Id, BillingSourceWallet, 0)
			task.TaskID = "cancel-" + scenario
			task.Status = model.TaskStatusInProgress
			task.PrivateData.Execution = &model.TaskExecutionSnapshot{RequestID: "create-" + scenario, RequestPath: "/v1/video/generations"}
			if scenario == "non_video" {
				task.PrivateData.Execution.RequestPath = "/v1/images/generations"
			}
			if scenario == "completed" {
				task.Status = model.TaskStatusSuccess
			}
			if scenario == "already_failed" {
				task.Status = model.TaskStatusFailure
			}
			require.NoError(t, model.DB.Create(task).Error)
			seedChargedAccounting(t, user.Id, channel.Id, token.Id, 100, 1)
			if scenario != "legacy" {
				_, err := model.EnsureCanvasReceipt(&model.CanvasReceipt{TokenID: token.Id, UserID: user.Id, RequestID: task.PrivateData.Execution.RequestID, TaskID: task.TaskID, ModelName: "test-model", Group: "default", QuotaPerUnit: "500000"})
				require.NoError(t, err)
				if scenario != "pending_submission" {
					require.NoError(t, model.MarkCanvasReceiptAccounting(token.Id, user.Id, task.PrivateData.Execution.RequestID, true))
				}
			}
			if scenario == "partial_token" {
				require.NoError(t, model.DB.Delete(token).Error)
			}
			stalePoll := *task
			err := CancelCanvasVideoTask(context.Background(), task, "用户取消视频生成")
			if scenario == "completed" || scenario == "non_video" {
				if scenario == "completed" {
					require.ErrorIs(t, err, ErrCanvasTaskCompleted)
				} else {
					require.ErrorIs(t, err, ErrCanvasTaskNotVideo)
				}
				var current model.User
				require.NoError(t, model.DB.First(&current, user.Id).Error)
				assert.Equal(t, 900, current.Quota)
				return
			}
			require.NoError(t, err)
			stalePoll.Status = model.TaskStatusSuccess
			won, err := stalePoll.UpdateWithStatus(model.TaskStatusInProgress)
			require.NoError(t, err)
			assert.False(t, won, "a stale success must not overwrite cancellation intent")
			if scenario == "pending_submission" {
				var current model.User
				require.NoError(t, model.DB.First(&current, user.Id).Error)
				assert.Equal(t, 900, current.Quota, "wait for original charge to commit")
				require.NotEmpty(t, task.CanvasCancelReason)
				require.NoError(t, model.MarkCanvasReceiptAccounting(token.Id, user.Id, task.PrivateData.Execution.RequestID, true))
				sweepCanvasTaskCancellations(context.Background())
			}
			for range 3 {
				require.NoError(t, CancelCanvasVideoTask(context.Background(), task, "重复取消"))
			}
			var persisted model.Task
			require.NoError(t, model.DB.First(&persisted, task.ID).Error)
			assert.EqualValues(t, model.TaskStatusFailure, persisted.Status)
			assert.Zero(t, persisted.Quota)
			var current model.User
			require.NoError(t, model.DB.First(&current, user.Id).Error)
			assert.Equal(t, 1000, current.Quota, "repeat cancellation never refunds twice")
			if scenario != "partial_token" {
				var currentToken model.Token
				require.NoError(t, model.DB.First(&currentToken, token.Id).Error)
				assert.Equal(t, 1000, currentToken.RemainQuota)
			}
			if scenario != "legacy" {
				receipt, err := model.GetCanvasReceipt(token.Id, user.Id, task.PrivateData.Execution.RequestID)
				require.NoError(t, err)
				if scenario == "partial_token" {
					assert.Equal(t, model.CanvasReceiptPending, receipt.Status)
				} else {
					assert.Equal(t, model.CanvasReceiptRefunded, receipt.Status)
				}
			}
		})
	}
}

func TestCanvasFailedVideoNeverSettlesUsage(t *testing.T) {
	task := &model.Task{Status: model.TaskStatusFailure}
	assert.False(t, settleTaskBillingOnComplete(context.Background(), nil, task, &relaycommon.TaskInfo{TotalTokens: 500}))
}
