package controller

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupPendingSubmissionDB creates an isolated SQLite database holding only the
// task table used by the pending submission recovery loop, and restores the
// previous global connections afterwards.
func setupPendingSubmissionDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "pending_submission.sqlite")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
	})
	return db
}

func seedPendingSubmission(t *testing.T, db *gorm.DB, updatedAt int64, taskStatus model.TaskStatus) *model.Task {
	t.Helper()
	task := &model.Task{
		CreatedAt: updatedAt,
		UpdatedAt: updatedAt,
		TaskID:    fmt.Sprintf("task_pending_%d", updatedAt),
		Platform:  constant.TaskPlatform("test"),
		UserId:    1,
		Group:     "default",
		Status:    taskStatus,
		PrivateData: model.TaskPrivateData{
			Submission: &model.TaskSubmissionState{
				Status:    model.TaskSubmissionPending,
				PluginKey: "test-plugin",
				UpdatedAt: updatedAt,
			},
		},
	}
	require.NoError(t, db.Create(task).Error)
	return task
}

// TestClaimPendingSubmissionSkipsLiveSubmission keeps a submission that is
// still owned by a live request away from the recovery loop: replaying it would
// create a second upstream task and a second charge.
func TestClaimPendingSubmissionSkipsLiveSubmission(t *testing.T) {
	db := setupPendingSubmissionDB(t)
	now := time.Now()
	task := seedPendingSubmission(t, db, now.Add(-10*time.Second).UnixMilli(), model.TaskStatusNotStart)

	claimed, ok, err := claimPendingSubmission(task.ID)
	require.NoError(t, err)
	assert.False(t, ok, "a submission younger than the in-flight grace must not be claimed")
	assert.Nil(t, claimed)

	stored := &model.Task{}
	require.NoError(t, db.First(stored, task.ID).Error)
	require.NotNil(t, stored.PrivateData.Submission)
	assert.Equal(t, model.TaskSubmissionPending, stored.PrivateData.Submission.Status)
}

// TestClaimPendingSubmissionClaimsStaleSubmission verifies that a submission
// left behind by a crashed process is still recoverable after the grace period.
func TestClaimPendingSubmissionClaimsStaleSubmission(t *testing.T) {
	db := setupPendingSubmissionDB(t)
	now := time.Now()
	task := seedPendingSubmission(t, db, now.Add(-10*time.Minute).UnixMilli(), model.TaskStatusNotStart)

	claimed, ok, err := claimPendingSubmission(task.ID)
	require.NoError(t, err)
	assert.True(t, ok)
	require.NotNil(t, claimed)
	require.NotNil(t, claimed.PrivateData.Submission)
	assert.Equal(t, model.TaskSubmissionRecovering, claimed.PrivateData.Submission.Status)

	stored := &model.Task{}
	require.NoError(t, db.First(stored, task.ID).Error)
	require.NotNil(t, stored.PrivateData.Submission)
	assert.Equal(t, model.TaskSubmissionRecovering, stored.PrivateData.Submission.Status)
}

// TestClaimPendingSubmissionSkipsTerminalTasks keeps finished tasks out of
// recovery even when their submission record is stale.
func TestClaimPendingSubmissionSkipsTerminalTasks(t *testing.T) {
	db := setupPendingSubmissionDB(t)
	now := time.Now()
	task := seedPendingSubmission(t, db, now.Add(-10*time.Minute).UnixMilli(), model.TaskStatusSuccess)

	claimed, ok, err := claimPendingSubmission(task.ID)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Nil(t, claimed)
}

// TestClaimPendingSubmissionSkipsRecentRecovery keeps a second instance from
// claiming a submission that is already being replayed by someone else.
func TestClaimPendingSubmissionSkipsRecentRecovery(t *testing.T) {
	db := setupPendingSubmissionDB(t)
	now := time.Now()
	task := seedPendingSubmission(t, db, now.Add(-10*time.Minute).UnixMilli(), model.TaskStatusNotStart)
	task.PrivateData.Submission.Status = model.TaskSubmissionRecovering
	task.PrivateData.Submission.UpdatedAt = now.Add(-30 * time.Second).UnixMilli()
	require.NoError(t, db.Save(task).Error)

	claimed, ok, err := claimPendingSubmission(task.ID)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Nil(t, claimed)
}
