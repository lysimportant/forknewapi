package model

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// TestCanvasReceiptDatabaseMatrix 验证三种数据库的新建、代表性旧表升级及重复迁移。
func TestCanvasReceiptDatabaseMatrix(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		open func(string) gorm.Dialector
	}{
		{"sqlite", filepath.Join(t.TempDir(), "receipts.db"), func(s string) gorm.Dialector { return sqlite.Open(s) }},
		{"mysql", os.Getenv("TEST_MYSQL_DSN"), func(s string) gorm.Dialector { return mysql.Open(s) }},
		{"postgres", os.Getenv("TEST_POSTGRES_DSN"), func(s string) gorm.Dialector {
			return postgres.New(postgres.Config{DSN: s, PreferSimpleProtocol: true})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.dsn == "" {
				if os.Getenv("CANVAS_REQUIRE_DATABASE_MATRIX") == "true" {
					t.Fatal("required isolated database DSN is missing: " + test.name)
				}
				t.Skip("isolated database DSN is not configured: " + test.name)
			}
			if test.name != "sqlite" {
				require.Contains(t, test.dsn, "canvas_bridge_test", "only the isolated Canvas database may be used")
			}
			for _, upgrade := range []bool{false, true} {
				t.Run(fmt.Sprintf("upgrade_%t", upgrade), func(t *testing.T) {
					recorder := &migrationSQLRecorder{}
					prefix := fmt.Sprintf("canvas_receipt_test_%d_", time.Now().UnixNano())
					db, err := gorm.Open(test.open(test.dsn), &gorm.Config{Logger: recorder, NamingStrategy: schema.NamingStrategy{TablePrefix: prefix}})
					require.NoError(t, err)
					sqlDB, err := db.DB()
					require.NoError(t, err)
					sqlDB.SetMaxOpenConns(1)
					previousDB := DB
					DB = db
					t.Cleanup(func() {
						require.NoError(t, db.Migrator().DropTable(&CanvasReceipt{}, &Task{}, &Token{}, &User{}))
						DB = previousDB
						require.NoError(t, sqlDB.Close())
					})
					if upgrade {
						// 旧版本没有回执表；按现有发布结构预置钱包、令牌和任务数据。
						require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &Task{}))
						require.NoError(t, db.Create(&User{Id: 951, Username: "receipt-existing", Quota: 700, AffCode: "receipt-old"}).Error)
						require.NoError(t, db.Create(&Token{Id: 952, UserId: 951, Key: "syntheticreceiptupgrade", RemainQuota: 600}).Error)
						require.NoError(t, db.Create(&Task{TaskID: "old-task", UserId: 951, Quota: 100}).Error)
					}
					require.NoError(t, db.AutoMigrate(&CanvasReceipt{}))
					recorder.reset()
					require.NoError(t, db.AutoMigrate(&CanvasReceipt{}))
					assert.Empty(t, recorder.schemaMutations(), "second receipt migration must not change the schema")
					if upgrade {
						var user User
						var token Token
						var task Task
						require.NoError(t, db.First(&user, 951).Error)
						require.NoError(t, db.First(&token, 952).Error)
						require.NoError(t, db.Where("task_id = ?", "old-task").First(&task).Error)
						assert.Equal(t, 700, user.Quota)
						assert.Equal(t, 600, token.RemainQuota)
						assert.Equal(t, 100, task.Quota)
					}
					verifyCanvasReceiptLifecycle(t)
				})
			}
		})
	}
}

// verifyCanvasReceiptLifecycle 覆盖唯一性、归属、不可变终态和失败记账屏障。
func verifyCanvasReceiptLifecycle(t *testing.T) {
	t.Helper()
	receipt := &CanvasReceipt{TokenID: 11, UserID: 21, RequestID: "Original-Request", TaskID: "video-task", ModelName: "video-model", Group: "default", QuotaPerUnit: "500000"}
	stored, err := EnsureCanvasReceipt(receipt)
	require.NoError(t, err)
	assert.Equal(t, CanvasReceiptPending, stored.Status)
	assert.Zero(t, stored.Quota)
	_, err = EnsureCanvasReceipt(receipt)
	require.NoError(t, err)
	var count int64
	require.NoError(t, DB.Model(&CanvasReceipt{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	duplicate := *stored
	duplicate.ID = 0
	assert.Error(t, DB.Create(&duplicate).Error)
	for _, owner := range [][2]int{{12, 21}, {11, 22}} {
		_, err = GetCanvasReceipt(owner[0], owner[1], receipt.RequestID)
		assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	}
	_, err = GetCanvasReceipt(11, 21, strings.ToLower(receipt.RequestID))
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	assert.ErrorIs(t, MarkCanvasReceiptAccounting(11, 21, strings.ToLower(receipt.RequestID), true), gorm.ErrRecordNotFound)
	assert.ErrorIs(t, FinalizeCanvasReceipt(11, 21, strings.ToLower(receipt.RequestID), CanvasReceiptSettled, 0), gorm.ErrRecordNotFound)
	assert.Error(t, FinalizeCanvasReceipt(11, 21, receipt.RequestID, CanvasReceiptSettled, 90))
	require.NoError(t, MarkCanvasReceiptAccounting(11, 21, receipt.RequestID, true))
	require.NoError(t, BeginCanvasReceiptAdjustment(11, 21, receipt.RequestID))
	assert.Error(t, BeginCanvasReceiptAdjustment(11, 21, receipt.RequestID))
	assert.Error(t, FinalizeCanvasReceipt(11, 21, receipt.RequestID, CanvasReceiptSettled, 90))
	require.NoError(t, MarkCanvasReceiptAccounting(11, 21, receipt.RequestID, true))
	require.NoError(t, FinalizeCanvasReceipt(11, 21, receipt.RequestID, CanvasReceiptSettled, 90))
	require.NoError(t, FinalizeCanvasReceipt(11, 21, receipt.RequestID, CanvasReceiptSettled, 90))
	assert.Error(t, FinalizeCanvasReceipt(11, 21, receipt.RequestID, CanvasReceiptRefunded, 0))
	assert.Error(t, FinalizeCanvasReceipt(11, 21, receipt.RequestID, CanvasReceiptSettled, 91))
	receipt.QuotaPerUnit, receipt.Group = "1", "other"
	stored, err = EnsureCanvasReceipt(receipt)
	require.NoError(t, err)
	assert.Equal(t, "500000", stored.QuotaPerUnit)
	assert.Equal(t, "default", stored.Group)
	assert.Positive(t, stored.SettledAt)
	receipt.RequestID = "unit-changed"
	_, err = EnsureCanvasReceipt(receipt)
	require.NoError(t, err)
	receipt.QuotaPerUnit = "100000"
	stored, err = EnsureCanvasReceipt(receipt)
	require.NoError(t, err)
	assert.Equal(t, "1", stored.QuotaPerUnit)
	assert.True(t, stored.AccountingFailed)
	assert.False(t, ValidCanvasReceiptRequestID("."))
	assert.False(t, ValidCanvasReceiptRequestID(".."))
	for _, status := range []string{CanvasReceiptSettled, CanvasReceiptRefunded} {
		receipt.RequestID = "failed-" + status
		_, err = EnsureCanvasReceipt(receipt)
		require.NoError(t, err)
		require.NoError(t, MarkCanvasReceiptAccounting(11, 21, receipt.RequestID, false))
		require.NoError(t, MarkCanvasReceiptAccounting(11, 21, receipt.RequestID, true))
		assert.Error(t, FinalizeCanvasReceipt(11, 21, receipt.RequestID, status, 0))
		stored, err = GetCanvasReceipt(11, 21, receipt.RequestID)
		require.NoError(t, err)
		assert.Equal(t, CanvasReceiptPending, stored.Status)
	}
}

// TestCanvasReceiptBatchAccounting 开关关闭时保持旧队列行为，开启后钱包和令牌同步写库。
func TestCanvasReceiptBatchAccounting(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "batch.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	previousDB, previousBatch := DB, common.BatchUpdateEnabled
	DB, common.BatchUpdateEnabled = db, true
	t.Cleanup(func() {
		DB, common.BatchUpdateEnabled = previousDB, previousBatch
		for _, kind := range []int{BatchUpdateTypeUserQuota, BatchUpdateTypeTokenQuota} {
			batchUpdateLocks[kind].Lock()
			delete(batchUpdateStores[kind], 981)
			batchUpdateLocks[kind].Unlock()
		}
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.Create(&User{Id: 981, Username: "batch-receipt", Quota: 1000}).Error)
	require.NoError(t, db.Create(&Token{Id: 981, UserId: 981, Key: "syntheticbatchreceipt", RemainQuota: 1000}).Error)
	for _, enabled := range []bool{false, true} {
		t.Setenv("CANVAS_BRIDGE_ENABLED", fmt.Sprint(enabled))
		require.NoError(t, DecreaseUserQuota(981, 100, false))
		require.NoError(t, DecreaseTokenQuota(981, "syntheticbatchreceipt", 100))
		var user User
		var token Token
		require.NoError(t, db.First(&user, 981).Error)
		require.NoError(t, db.First(&token, 981).Error)
		want := 1000
		if enabled {
			want = 900
		}
		assert.Equal(t, want, user.Quota)
		assert.Equal(t, want, token.RemainQuota)
	}
	require.NoError(t, IncreaseUserQuota(981, 100, false))
	require.NoError(t, IncreaseTokenQuota(981, "syntheticbatchreceipt", 100))
	require.NoError(t, persistUserQuotaDelta(981, -25))
	require.NoError(t, persistTokenQuotaDelta(981, -25))
	var user User
	var token Token
	require.NoError(t, db.First(&user, 981).Error)
	require.NoError(t, db.First(&token, 981).Error)
	assert.Equal(t, 975, user.Quota)
	assert.Equal(t, 975, token.RemainQuota)
	assert.ErrorIs(t, DecreaseUserQuota(982, 1, false), gorm.ErrRecordNotFound)
	assert.ErrorIs(t, DecreaseTokenQuota(982, "missing", 1), gorm.ErrRecordNotFound)
}
