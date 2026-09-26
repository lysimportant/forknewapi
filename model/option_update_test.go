package model

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// openOptionUpdateTestDB 打开本用例专用数据库；MySQL/PostgreSQL 使用验收脚本
// 提供的隔离 DSN，SQLite 使用临时文件，避免触碰运行中的站点数据。
func openOptionUpdateTestDB(t *testing.T, dialect string) *gorm.DB {
	t.Helper()
	var db *gorm.DB
	var err error
	switch dialect {
	case "sqlite":
		db, err = gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "options.db")), &gorm.Config{})
	case "mysql":
		dsn := strings.TrimSpace(os.Getenv("TEST_MYSQL_DSN"))
		if dsn == "" {
			t.Skip("TEST_MYSQL_DSN is not configured")
		}
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	case "postgres":
		dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
		if dsn == "" {
			t.Skip("TEST_POSTGRES_DSN is not configured")
		}
		db, err = gorm.Open(postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true}), &gorm.Config{})
	default:
		t.Fatalf("unsupported dialect %q", dialect)
	}
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&Option{}))
	var version string
	versionQuery := "SELECT VERSION()"
	if dialect == "sqlite" {
		versionQuery = "SELECT sqlite_version()"
	}
	require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
	t.Logf("database: %s %s", dialect, version)
	return db
}

// withOptionUpdateTestState 隔离当前用例使用的数据库与公告内存配置，并在结束后恢复。
func withOptionUpdateTestState(t *testing.T, db *gorm.DB, dbType common.DatabaseType) {
	t.Helper()
	previousDB := DB
	previousType := common.MainDatabaseType()
	previousOptions := common.OptionMap
	previousAnnouncements := console_setting.GetConsoleSetting().Announcements
	DB = db
	common.SetMainDatabaseType(dbType)
	common.OptionMap = map[string]string{}
	console_setting.GetConsoleSetting().Announcements = ""
	t.Cleanup(func() {
		DB = previousDB
		common.SetMainDatabaseType(previousType)
		common.OptionMap = previousOptions
		console_setting.GetConsoleSetting().Announcements = previousAnnouncements
	})
}

// TestUpdateOptionPersistsAndPropagatesDatabaseErrors 验证公告配置持久化、失败回滚与内存发布边界。
func TestUpdateOptionPersistsAndPropagatesDatabaseErrors(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := openOptionUpdateTestDB(t, dialect)
			dbType := common.DatabaseTypeSQLite
			if dialect == "mysql" {
				dbType = common.DatabaseTypeMySQL
			} else if dialect == "postgres" {
				dbType = common.DatabaseTypePostgreSQL
			}
			withOptionUpdateTestState(t, db, dbType)

			const key = "console_setting.announcements"
			newValue := `[{"id":1,"content":"new announcement","publishDate":"2026-09-17T00:00:00Z"}]`
			updatedValue := `[{"id":1,"content":"updated announcement","publishDate":"2026-09-17T00:00:00Z","pinned":true}]`
			// 新增、更新、重复保存与清空都必须同时持久化并发布到实际公告配置。
			for _, value := range []string{newValue, updatedValue, updatedValue, ""} {
				require.NoError(t, UpdateOption(key, value))
				assert.Equal(t, value, common.OptionMap[key])
				assert.Equal(t, value, console_setting.GetConsoleSetting().Announcements)
				var saved Option
				require.NoError(t, db.First(&saved, Option{Key: key}).Error)
				assert.Equal(t, value, saved.Value)
			}
			require.NoError(t, UpdateOption(key, updatedValue))
			// Save 失败时返回原始错误、内存不变，事务也回滚而不损坏旧值。
			saveErr := errors.New("option update save failed")
			callbackName := "option-update-test-save-failure"
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
				tx.AddError(saveErr)
			}))
			err := UpdateOption(key, newValue)
			require.ErrorIs(t, err, saveErr)
			assert.Equal(t, updatedValue, common.OptionMap[key])
			assert.Equal(t, updatedValue, console_setting.GetConsoleSetting().Announcements)
			var saved Option
			require.NoError(t, db.First(&saved, Option{Key: key}).Error)
			assert.Equal(t, updatedValue, saved.Value)
			err = UpdateOption("option-update-test-new-save", "should-not-publish")
			require.ErrorIs(t, err, saveErr)
			assert.NotContains(t, common.OptionMap, "option-update-test-new-save")
			var empty Option
			assert.ErrorIs(t, db.First(&empty, Option{Key: "option-update-test-new-save"}).Error, gorm.ErrRecordNotFound)
			// 首次插入失败时同样返回原始错误，内存不更新且不留下空配置行。
			insertErr := errors.New("option insert failed")
			insertCallbackName := "option-update-test-insert-failure"
			require.NoError(t, db.Callback().Create().Before("gorm:create").Register(insertCallbackName, func(tx *gorm.DB) {
				tx.AddError(insertErr)
			}))
			err = UpdateOption("option-update-test-new", "should-not-publish")
			require.ErrorIs(t, err, insertErr)
			assert.NotContains(t, common.OptionMap, "option-update-test-new")
			var missing Option
			assert.ErrorIs(t, db.First(&missing, Option{Key: "option-update-test-new"}).Error, gorm.ErrRecordNotFound)
			// 关闭真实连接池，验证数据库不可用时错误上抛且不发布尚未落库的公告。
			sqlDB, err := db.DB()
			require.NoError(t, err)
			require.NoError(t, sqlDB.Close())
			err = UpdateOption(key, newValue)
			require.ErrorContains(t, err, "database is closed")
			assert.Equal(t, updatedValue, common.OptionMap[key])
			assert.Equal(t, updatedValue, console_setting.GetConsoleSetting().Announcements)
		})
	}
}

// legacyOptionWithoutPrimaryKey 模拟旧 options 表缺少键唯一约束的数据库结构。
type legacyOptionWithoutPrimaryKey struct {
	Key   string `gorm:"size:191"`
	Value string
}

// TestOptionPrimaryKeyMigrationPreservesRows 验证健康表保持不变、旧表修复保留原始备份及重复启动稳定。
func TestOptionPrimaryKeyMigrationPreservesRows(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := openOptionUpdateTestDB(t, dialect)
			before, err := db.Migrator().GetTables()
			require.NoError(t, err)
			require.NoError(t, migrateOptionPrimaryKey(db))
			after, err := db.Migrator().GetTables()
			require.NoError(t, err)
			assert.ElementsMatch(t, before, after, "健康 options 表不得被重建")

			require.NoError(t, db.Migrator().DropTable(&Option{}))
			require.NoError(t, db.Table("options").Migrator().CreateTable(&legacyOptionWithoutPrimaryKey{}))
			rows := []Option{{Key: "About", Value: "preserved"}, {Key: "About", Value: "preserved"}, {Key: "Notice", Value: "retained"}, {Key: "", Value: "empty-key-backup"}}
			require.NoError(t, db.Table("options").Create(&rows).Error)
			require.NoError(t, migrateOptionPrimaryKey(db))
			var saved []Option
			require.NoError(t, db.Find(&saved).Error)
			assert.ElementsMatch(t, []Option{{Key: "About", Value: "preserved"}, {Key: "Notice", Value: "retained"}}, saved)
			assert.Error(t, db.Create(&Option{Key: "About", Value: "duplicate"}).Error, "修复后数据库必须拒绝重复配置键")
			tables, err := db.Migrator().GetTables()
			require.NoError(t, err)
			var backups []string
			for _, table := range tables {
				if strings.HasPrefix(table, optionLegacyTablePrefix) {
					backups = append(backups, table)
				}
			}
			require.Len(t, backups, 1)
			var original []Option
			require.NoError(t, db.Table(backups[0]).Find(&original).Error)
			assert.ElementsMatch(t, rows, original, "含重复或空键的原始记录必须完整保留")
			require.NoError(t, migrateOptionPrimaryKey(db))
			repeated, err := db.Migrator().GetTables()
			require.NoError(t, err)
			assert.ElementsMatch(t, tables, repeated, "重复启动不得新增备份或重建 options 表")
		})
	}
}

// TestOptionPrimaryKeyFailureStopsMigration 验证配置主键修复失败会中止启动，不带错误配置表继续服务。
func TestOptionPrimaryKeyFailureStopsMigration(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := openOptionUpdateTestDB(t, dialect)
			dbType := common.DatabaseTypeSQLite
			if dialect == "mysql" {
				dbType = common.DatabaseTypeMySQL
			} else if dialect == "postgres" {
				dbType = common.DatabaseTypePostgreSQL
			}
			withOptionUpdateTestState(t, db, dbType)
			require.NoError(t, db.Migrator().DropTable(&Option{}))
			require.NoError(t, db.Table("options").Migrator().CreateTable(&legacyOptionWithoutPrimaryKey{}))
			require.NoError(t, db.Create(&Option{Key: "About", Value: "preserved-on-failure"}).Error)
			readErr := errors.New("options migration read failed")
			const callbackName = "option-migration-read-failure"
			require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
				if tx.Statement.Table == "options" {
					tx.AddError(readErr)
				}
			}))
			require.ErrorIs(t, migrateDB(), readErr)
			require.NoError(t, db.Callback().Query().Remove(callbackName))
			var saved Option
			require.NoError(t, db.First(&saved, Option{Key: "About"}).Error)
			assert.Equal(t, "preserved-on-failure", saved.Value)
		})
	}
}
