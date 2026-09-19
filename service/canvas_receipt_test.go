package service

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// TestCanvasReceiptAccountingMatrix 在隔离数据库验证成功、退款和异步部分失败的真实记账。
func TestCanvasReceiptAccountingMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector
			switch dialect {
			case "sqlite":
				driver = sqlite.Open(filepath.Join(t.TempDir(), "accounting.db"))
			case "mysql", "postgres":
				env := "TEST_MYSQL_DSN"
				if dialect == "postgres" {
					env = "TEST_POSTGRES_DSN"
				}
				dsn := os.Getenv(env)
				if dsn == "" {
					if os.Getenv("CANVAS_REQUIRE_DATABASE_MATRIX") == "true" {
						t.Fatal(env + " is required")
					}
					t.Skip(env + " is not configured")
				}
				require.Contains(t, dsn, "canvas_bridge_test")
				driver = mysql.Open(dsn)
				if dialect == "postgres" {
					driver = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
				}
			}
			db, err := gorm.Open(driver, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: fmt.Sprintf("canvas_accounting_test_%d_", time.Now().UnixNano())}})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			previousDB, previousLogDB := model.DB, model.LOG_DB
			previousBatch, previousRedis, previousLogging := common.BatchUpdateEnabled, common.RedisEnabled, common.LogConsumeEnabled
			model.DB, model.LOG_DB = db, db
			common.BatchUpdateEnabled, common.RedisEnabled, common.LogConsumeEnabled = true, false, false
			t.Setenv("CANVAS_BRIDGE_ENABLED", "true")
			require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Task{}, &model.CanvasReceipt{}, &model.Log{}))
			t.Cleanup(func() {
				require.NoError(t, db.Migrator().DropTable(&model.CanvasReceipt{}, &model.Task{}, &model.Token{}, &model.User{}, &model.Log{}))
				model.DB, model.LOG_DB = previousDB, previousLogDB
				common.BatchUpdateEnabled, common.RedisEnabled, common.LogConsumeEnabled = previousBatch, previousRedis, previousLogging
				require.NoError(t, sqlDB.Close())
			})
			for _, scenario := range []string{"sync", "async", "async_refund", "async_token_failure", "async_funding_failure", "sync_token_failure", "per_call", "immediate", "immediate_failure", "immediate_unverified", "free", "tiered_unverified"} {
				t.Run(scenario, func(t *testing.T) { verifyCanvasAccounting(t, db, scenario) })
			}
		})
	}
}

// verifyCanvasAccounting 从预扣开始验证最终余额与回执，不依赖消费日志。
func verifyCanvasAccounting(t *testing.T, db *gorm.DB, scenario string) {
	t.Helper()
	user := &model.User{Username: "receipt_" + scenario, AffCode: scenario, Quota: 1000, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(user).Error)
	token := &model.Token{UserId: user.Id, Key: "synthetic" + scenario, RemainQuota: 1000, Status: common.TokenStatusEnabled}
	require.NoError(t, db.Create(token).Error)
	info := &relaycommon.RelayInfo{UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, OriginModelName: "test-model", UsingGroup: "default", RequestId: "submit-" + scenario, ForcePreConsume: true}
	info.UserSetting.BillingPreference = "wallet_only"
	async := scenario != "sync" && scenario != "sync_token_failure" && scenario != "free" && scenario != "tiered_unverified"
	if async {
		info.TaskRelayInfo = &relaycommon.TaskRelayInfo{PublicTaskID: "task-" + scenario}
	}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/videos/generations", nil)
	if scenario != "free" {
		require.Nil(t, PreConsumeBilling(ctx, 100, info))
	}
	if scenario == "tiered_unverified" {
		info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprString: "invalid(", QuotaPerUnit: 500000, GroupRatio: 1, EstimatedQuotaAfterGroup: 100}
		ok, quota, result := TryTieredSettle(info, billingexpr.TokenParams{P: 1})
		assert.True(t, ok)
		assert.Equal(t, 100, quota)
		assert.Nil(t, result)
	}
	var task *model.Task
	if async {
		task = makeTask(user.Id, 0, 100, token.Id, BillingSourceWallet, 0)
		task.TaskID = info.TaskRelayInfo.PublicTaskID
		task.PrivateData.Execution = &model.TaskExecutionSnapshot{RequestID: info.RequestId}
		if scenario == "immediate" || scenario == "immediate_unverified" {
			task.Status = model.TaskStatusSuccess
		}
		if scenario == "immediate_failure" {
			task.Status = model.TaskStatusFailure
			task.Quota = 0
		}
		require.NoError(t, db.Create(task).Error)
	}
	if scenario == "immediate_unverified" {
		MarkCanvasReceiptUnverified(info)
	}
	if scenario == "sync_token_failure" {
		require.NoError(t, db.Delete(token).Error)
	}
	actual := 150
	if async {
		actual = 100
	}
	if scenario == "immediate_failure" || scenario == "free" {
		actual = 0
	}
	if scenario == "tiered_unverified" {
		actual = 100
	}
	err := SettleBilling(ctx, info, actual)
	if scenario == "sync_token_failure" {
		require.Error(t, err)
		require.Error(t, SettleBilling(ctx, info, actual), "a repeated session must preserve the original accounting error")
	} else if scenario == "tiered_unverified" {
		require.Error(t, err)
	} else {
		require.NoError(t, err)
	}
	if scenario == "immediate" || scenario == "immediate_failure" || scenario == "immediate_unverified" {
		FinalizeCanvasImmediateTaskReceipt(ctx, info, task)
	}
	receipt, err := model.GetCanvasReceipt(token.Id, user.Id, info.RequestId)
	require.NoError(t, err)
	deferred := async && scenario != "immediate" && scenario != "immediate_failure" && scenario != "immediate_unverified"
	if deferred {
		assert.Equal(t, model.CanvasReceiptPending, receipt.Status)
	}
	if deferred {
		task.Status = model.TaskStatusSuccess
		switch scenario {
		case "async_token_failure":
			require.NoError(t, db.Delete(token).Error)
			RecalculateTaskQuota(context.Background(), task, 150, "receipt test")
			RecalculateTaskQuota(context.Background(), task, 150, "receipt retry")
		case "async_funding_failure":
			require.NoError(t, db.Delete(user).Error)
			RecalculateTaskQuota(context.Background(), task, 150, "receipt test")
		case "async_refund":
			task.Status = model.TaskStatusFailure
			require.True(t, RefundTaskQuota(context.Background(), task, "receipt test"))
		case "per_call":
			task.PrivateData.BillingContext.PerCallBilling = true
			assert.False(t, settleTaskBillingOnComplete(context.Background(), &mockAdaptor{}, task, &relaycommon.TaskInfo{}))
		default:
			RecalculateTaskQuota(context.Background(), task, 150, "receipt test")
		}
	}
	receipt, err = model.GetCanvasReceipt(token.Id, user.Id, info.RequestId)
	require.NoError(t, err)
	assert.Equal(t, info.RequestId, receipt.RequestID)
	assert.Equal(t, "500000", receipt.QuotaPerUnit)
	switch scenario {
	case "sync_token_failure", "async_token_failure", "async_funding_failure", "immediate_unverified", "tiered_unverified":
		assert.Equal(t, model.CanvasReceiptPending, receipt.Status)
		assert.True(t, receipt.AccountingFailed)
		assert.Zero(t, receipt.SettledAt)
	case "async_refund", "immediate_failure":
		assert.Equal(t, model.CanvasReceiptRefunded, receipt.Status)
		assert.Zero(t, receipt.Quota)
	default:
		assert.Equal(t, model.CanvasReceiptSettled, receipt.Status)
		wantQuota := int64(150)
		if scenario == "per_call" || scenario == "immediate" {
			wantQuota = 100
		}
		if scenario == "free" {
			wantQuota = 0
		}
		assert.Equal(t, wantQuota, receipt.Quota)
		assert.Positive(t, receipt.SettledAt)
	}
	if scenario != "async_funding_failure" {
		var storedUser model.User
		require.NoError(t, db.First(&storedUser, user.Id).Error)
		wantBalance := 850
		if scenario == "async_refund" || scenario == "immediate_failure" || scenario == "free" {
			wantBalance = 1000
		}
		if scenario == "per_call" || scenario == "immediate" || scenario == "immediate_unverified" || scenario == "tiered_unverified" {
			wantBalance = 900
		}
		assert.Equal(t, wantBalance, storedUser.Quota)
	}
}

// TestCanvasReceiptPollingWaitsForSubmission 验证所有终态入口等待提交记账，然后继续结算或退款。
func TestCanvasReceiptPollingWaitsForSubmission(t *testing.T) {
	t.Setenv("CANVAS_BRIDGE_ENABLED", "true")
	require.NoError(t, model.DB.AutoMigrate(&model.CanvasReceipt{}))
	previousTimeout := constant.TaskTimeoutMinutes
	constant.TaskTimeoutMinutes = 1
	t.Cleanup(func() { constant.TaskTimeoutMinutes = previousTimeout })
	for _, mode := range []string{"single_success", "single_failure", "batch", "fail", "sweep", "free_async", "free_immediate"} {
		t.Run(mode, func(t *testing.T) {
			truncate(t)
			seedUser(t, 970, 1000)
			seedToken(t, 970, 970, "syntheticpollreceipt", 1000)
			seedChannel(t, 970)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1/videos", nil)
			info := &relaycommon.RelayInfo{UserId: 970, TokenId: 970, TokenKey: "syntheticpollreceipt", OriginModelName: "test-model", UsingGroup: "default", RequestId: "poll-submit-" + mode,
				ForcePreConsume: true, TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "poll-task-" + mode}}
			info.UserSetting.BillingPreference = "wallet_only"
			quota := 100
			if mode == "free_async" || mode == "free_immediate" {
				quota = 0
			} else {
				require.Nil(t, PreConsumeBilling(ctx, quota, info))
			}
			require.NoError(t, PrepareCanvasTaskReceipt(info))
			task := makeTask(970, 970, quota, 970, BillingSourceWallet, 0)
			task.TaskID = info.PublicTaskID
			task.SubmitTime = time.Now().Add(-2 * time.Minute).Unix()
			task.PrivateData.Execution = &model.TaskExecutionSnapshot{RequestID: info.RequestId}
			task.PrivateData.BillingContext.PerCallBilling = true
			if mode == "free_immediate" {
				task.Status = model.TaskStatusSuccess
			}
			require.NoError(t, model.DB.Create(task).Error)
			t.Cleanup(func() {
				require.NoError(t, model.DB.Where("request_id = ?", info.RequestId).Delete(&model.CanvasReceipt{}).Error)
			})
			if mode == "free_immediate" {
				require.NoError(t, SettleBilling(ctx, info, 0))
				FinalizeCanvasImmediateTaskReceipt(ctx, info, task)
				receipt, err := model.GetCanvasReceipt(970, 970, info.RequestId)
				require.NoError(t, err)
				assert.Equal(t, model.CanvasReceiptSettled, receipt.Status)
				assert.Zero(t, receipt.Quota)
				return
			}
			status := model.TaskStatusSuccess
			if mode != "single_success" && mode != "batch" && mode != "free_async" {
				status = model.TaskStatusFailure
			}
			adaptor := &scriptedPollingAdaptor{parse: &relaycommon.TaskInfo{Status: string(status)}}
			poll := func() {
				switch mode {
				case "single_success", "single_failure", "free_async":
					require.NoError(t, updateVideoSingleTask(ctx, adaptor, &model.Channel{Id: 970}, task.TaskID, map[string]*model.Task{task.TaskID: task}))
				case "batch":
					batch := &scriptedBatchPollingAdaptor{results: map[string]*BatchTaskResult{task.TaskID: {TaskInfo: *adaptor.parse}}}
					require.NoError(t, updateBatchTasks(ctx, batch, 970, []string{task.TaskID}, map[string]*model.Task{task.TaskID: task}))
				case "fail":
					require.NoError(t, failTaskFromPoll(ctx, adaptor, task, task.Status, "not found"))
				case "sweep":
					sweepTimedOutTasks(ctx)
				}
			}
			poll()
			var stored model.Task
			require.NoError(t, model.DB.First(&stored, task.ID).Error)
			assert.Equal(t, model.TaskStatusInProgress, string(stored.Status))
			assert.Equal(t, quota, stored.Quota)
			require.NoError(t, SettleBilling(ctx, info, quota))
			poll()
			require.NoError(t, model.DB.First(&stored, task.ID).Error)
			assert.Equal(t, string(status), string(stored.Status))
			receipt, err := model.GetCanvasReceipt(970, 970, info.RequestId)
			require.NoError(t, err)
			if status == model.TaskStatusSuccess {
				assert.Equal(t, model.CanvasReceiptSettled, receipt.Status)
			} else {
				assert.Equal(t, model.CanvasReceiptRefunded, receipt.Status)
			}
		})
	}
}
