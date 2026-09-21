package model

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// canvasGrantMigrationBaseline 表示引入 Revision 字段前的授权表结构。
type canvasGrantMigrationBaseline struct {
	ID         string `gorm:"type:varchar(36);primaryKey"`
	ClientID   string `gorm:"type:varchar(64);not null;uniqueIndex:idx_canvas_grant_identity,priority:1"`
	InstanceID string `gorm:"type:varchar(128);not null;uniqueIndex:idx_canvas_grant_identity,priority:2"`
	UserID     int    `gorm:"not null;index;uniqueIndex:idx_canvas_grant_identity,priority:3"`
	TokenHash  string `gorm:"type:char(64);not null;uniqueIndex"`
	Scopes     string `gorm:"type:varchar(255);not null"`
	Status     string `gorm:"type:varchar(16);not null;index"`
	IssuedAt   int64  `gorm:"type:bigint;not null"`
	ExpiresAt  int64  `gorm:"type:bigint;not null;index"`
	RevokedAt  int64  `gorm:"type:bigint;not null"`
	CreatedAt  int64  `gorm:"type:bigint;not null"`
	UpdatedAt  int64  `gorm:"type:bigint;not null"`
}

// TableName 返回代表性旧版授权表名。
func (canvasGrantMigrationBaseline) TableName() string { return "canvas_grants" }

// canvasManagedTokenMigrationBaseline 表示引入 AutoGroups 字段前的管理 Token 表结构。
type canvasManagedTokenMigrationBaseline struct {
	ID                    int64  `gorm:"primaryKey"`
	GrantID               string `gorm:"type:varchar(36);not null;index;uniqueIndex:idx_canvas_managed_group,priority:1"`
	UserID                int    `gorm:"not null;index"`
	GroupID               string `gorm:"type:varchar(191);not null;uniqueIndex:idx_canvas_managed_group,priority:2"`
	TokenID               int    `gorm:"not null;uniqueIndex"`
	CredentialRevision    int64  `gorm:"type:bigint;not null"`
	PermissionRevision    int64  `gorm:"type:bigint;not null"`
	PermissionFingerprint string `gorm:"type:char(64);not null"`
	Status                string `gorm:"type:varchar(16);not null;index"`
	ReplacedByID          *int64 `gorm:"index"`
	CreatedAt             int64  `gorm:"type:bigint;not null"`
	UpdatedAt             int64  `gorm:"type:bigint;not null"`
	RevokedAt             int64  `gorm:"type:bigint;not null"`
}

// TableName 返回代表性旧版管理 Token 表名。
func (canvasManagedTokenMigrationBaseline) TableName() string { return "canvas_managed_tokens" }

// TestCanvasAccountDatabaseMatrix 验证账号合同在三种数据库上的新建、升级和重复迁移。
func TestCanvasAccountDatabaseMatrix(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
	}{
		{name: "sqlite"},
		{name: "mysql", dsn: strings.TrimSpace(os.Getenv("TEST_MYSQL_DSN"))},
		{name: "postgres", dsn: strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.name != "sqlite" && test.dsn == "" {
				if os.Getenv("CANVAS_REQUIRE_DATABASE_MATRIX") == "true" {
					t.Fatal("required isolated database DSN is missing: " + test.name)
				}
				t.Skip("isolated database DSN is not configured: " + test.name)
			}
			if test.name != "sqlite" {
				require.Contains(t, strings.ToLower(test.dsn), "canvas_account_test", "only the isolated Canvas account database may be used")
			}

			const dsnEnv = "CANVAS_ACCOUNT_MIGRATION_TEST_DSN"
			previousSQLitePath := common.SQLitePath
			if test.name == "sqlite" {
				common.SQLitePath = filepath.Join(t.TempDir(), "canvas-account.db")
				t.Setenv(dsnEnv, "")
			} else {
				t.Setenv(dsnEnv, test.dsn)
			}
			t.Cleanup(func() { common.SQLitePath = previousSQLitePath })

			db, databaseType, err := chooseDB(dsnEnv, false)
			require.NoError(t, err)
			previousType := common.MainDatabaseType()
			common.SetMainDatabaseType(databaseType)
			initCol()
			t.Cleanup(func() { common.SetMainDatabaseType(previousType); initCol() })
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			versionQuery := "SELECT version()"
			if test.name == "sqlite" {
				versionQuery = "SELECT sqlite_version()"
			}
			var databaseVersion string
			require.NoError(t, db.Raw(versionQuery).Scan(&databaseVersion).Error)
			t.Logf("database version: %s", databaseVersion)

			recorder := &migrationSQLRecorder{}
			db = db.Session(&gorm.Session{Logger: recorder})
			t.Run("fresh", func(t *testing.T) {
				testCanvasAccountFreshMigration(t, db, recorder)
			})
			t.Run("upgrade", func(t *testing.T) {
				testCanvasAccountBaselineUpgrade(t, db, recorder)
			})
			t.Run("token-configuration", func(t *testing.T) {
				testCanvasTokenConfiguration(t, db)
			})
			t.Run("controlled-rotation", func(t *testing.T) {
				testCanvasControlledRotation(t, db)
			})
		})
	}
}

// testCanvasControlledRotation 在三种实际数据库中验证回滚、原操作重试及人工修改边界。
func testCanvasControlledRotation(t *testing.T, db *gorm.DB) {
	t.Helper()
	previousDB, previousRedis := DB, common.RedisEnabled
	DB, common.RedisEnabled = db, false
	t.Cleanup(func() { DB, common.RedisEnabled = previousDB, previousRedis })
	models := []any{&User{}, &Token{}, &Channel{}, &Ability{}, &Option{}, &CanvasGrant{}, &CanvasManagedToken{}, &CanvasIdempotencyOperation{}}
	require.NoError(t, db.AutoMigrate(models...))
	t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(models...)) })
	initCol()
	user := User{Id: 7301, Username: "canvas-rotation-test", Group: "default", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	var grant *CanvasGrant
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var err error
		grant, err = UpsertCanvasGrantWithTx(tx, "canvas-test", "rotation-test", user.Id, strings.Repeat("b", 64), "tokens:manage", time.Now().Unix()+3600)
		return err
	}))
	initial := CanvasManagedTokenInput{GrantID: grant.ID, GroupID: "default", OperationID: "initial"}
	old, err := EnsureCanvasManagedToken(initial)
	require.NoError(t, err)
	fingerprint := sha256.Sum256([]byte(old.Token.GetFullKey()))
	rotation := initial
	rotation.OperationID = "rotate-1"
	rotation.Rotation = &CanvasManagedTokenRotation{TokenID: old.Token.Id, CredentialRevision: 1, KeyFingerprint: hex.EncodeToString(fingerprint[:])}
	const callback = "canvas_test:reject_rotation_intent"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "canvas_idempotency_operations" {
			tx.AddError(ErrCanvasIdempotencyConflict)
		}
	}))
	_, err = EnsureCanvasManagedToken(rotation)
	require.NoError(t, db.Callback().Create().Remove(callback))
	require.ErrorIs(t, err, ErrCanvasIdempotencyConflict)
	unchanged, err := EnsureCanvasManagedToken(initial)
	require.NoError(t, err)
	assert.Equal(t, old.Token.Key, unchanged.Token.Key)
	assert.Equal(t, int64(1), unchanged.Managed.CredentialRevision)
	rotated, err := EnsureCanvasManagedToken(rotation)
	require.NoError(t, err)
	assert.Equal(t, old.Token.Id, rotated.Token.Id)
	assert.NotEqual(t, old.Token.Key, rotated.Token.Key)
	assert.Equal(t, int64(2), rotated.Managed.CredentialRevision)
	assert.Greater(t, rotated.Managed.PermissionRevision, old.Managed.PermissionRevision)
	retried, err := EnsureCanvasManagedToken(rotation)
	require.NoError(t, err)
	assert.Equal(t, rotated.Token.Key, retried.Token.Key)
	assert.Equal(t, int64(2), retried.Managed.CredentialRevision)
	_, err = GetTokenByKey(old.Token.Key, true)
	assert.Error(t, err)
	conflict := rotation
	conflict.OperationID = "different-operation-same-version"
	_, err = EnsureCanvasManagedToken(conflict)
	assert.ErrorIs(t, err, ErrCanvasManagedTokenChanged)
	var count int64
	require.NoError(t, db.Model(&Token{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
	require.NoError(t, db.Model(&Token{}).Where("id = ?", rotated.Token.Id).Update("key", "manually-replaced-key").Error)
	_, err = EnsureCanvasManagedToken(rotation)
	assert.ErrorIs(t, err, ErrCanvasManagedTokenChanged, "replay cannot adopt a manual key change")
	require.NoError(t, db.Model(&Token{}).Where("id = ?", rotated.Token.Id).Update("key", rotated.Token.Key).Error)
	rotated.Token.ExpiredTime = -1
	require.NoError(t, rotated.Token.Update())
	_, err = EnsureCanvasManagedToken(rotation)
	assert.ErrorIs(t, err, ErrCanvasManagedTokenChanged, "rotation cannot reverse a manual expiry change")
}

// testCanvasTokenConfiguration 验证普通令牌更新及人工改期与管理状态的事务一致性。
func testCanvasTokenConfiguration(t *testing.T, db *gorm.DB) {
	t.Helper()
	previousDB, previousRedis := DB, common.RedisEnabled
	DB, common.RedisEnabled = db, false
	t.Cleanup(func() { DB, common.RedisEnabled = previousDB, previousRedis })
	require.NoError(t, db.AutoMigrate(&Token{}, &CanvasManagedToken{}))
	t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(&CanvasManagedToken{}, &Token{})) })
	token := Token{UserId: 7101, Key: "canvas-expiry-matrix", Name: "Canvas-expiry", ExpiredTime: time.Now().Unix() + 3600}
	require.NoError(t, db.Create(&token).Error)
	token.Name = "ordinary-token-renamed"
	require.NoError(t, token.Update(), "ordinary tokens must remain editable")
	managed := CanvasManagedToken{GrantID: "expiry-grant", UserID: token.UserId, TokenID: token.Id, GroupID: "default", Status: CanvasManagedTokenStatusActive}
	require.NoError(t, db.Create(&managed).Error)
	token.Name = "managed-token-renamed"
	require.NoError(t, token.Update())
	require.NoError(t, db.First(&managed, managed.ID).Error)
	assert.Equal(t, CanvasManagedTokenStatusActive, managed.Status, "renaming must not revoke management")

	originalExpiry := token.ExpiredTime
	token.ExpiredTime = time.Now().Unix() - 1
	const callback = "canvas_test:reject_token_update"
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "tokens" {
			tx.AddError(ErrCanvasManagedTokenChanged)
		}
	}))
	updateErr := token.Update()
	require.NoError(t, db.Callback().Update().Remove(callback))
	require.ErrorIs(t, updateErr, ErrCanvasManagedTokenChanged)
	var stored Token
	require.NoError(t, db.First(&stored, token.Id).Error)
	require.NoError(t, db.First(&managed, managed.ID).Error)
	assert.Equal(t, originalExpiry, stored.ExpiredTime)
	assert.Equal(t, CanvasManagedTokenStatusActive, managed.Status, "failed token updates must roll back management status")

	require.NoError(t, token.Update())
	require.NoError(t, db.First(&stored, token.Id).Error)
	require.NoError(t, db.First(&managed, managed.ID).Error)
	assert.Equal(t, token.ExpiredTime, stored.ExpiredTime)
	assert.Equal(t, CanvasManagedTokenStatusChanged, managed.Status)
}

// testCanvasAccountFreshMigration 校验全新表的默认值、索引、唯一性和幂等迁移。
func testCanvasAccountFreshMigration(t *testing.T, db *gorm.DB, recorder *migrationSQLRecorder) {
	t.Helper()
	dropCanvasAccountMigrationTables(t, db)
	defer dropCanvasAccountMigrationTables(t, db)

	require.NoError(t, db.AutoMigrate(&CanvasGrant{}, &CanvasManagedToken{}, &CanvasIdempotencyOperation{}))
	assert.True(t, db.Migrator().HasIndex(&CanvasGrant{}, "idx_canvas_grant_identity"))
	assert.True(t, db.Migrator().HasIndex(&CanvasGrant{}, db.NamingStrategy.IndexName("canvas_grants", "token_hash")))
	assert.True(t, db.Migrator().HasIndex(&CanvasManagedToken{}, "idx_canvas_managed_group"))
	assert.True(t, db.Migrator().HasIndex(&CanvasManagedToken{}, db.NamingStrategy.IndexName("canvas_managed_tokens", "token_id")))
	assert.True(t, db.Migrator().HasIndex(&CanvasIdempotencyOperation{}, "idx_canvas_operation"))

	now := time.Now().Unix()
	grantValues := canvasGrantMigrationValues("fresh-grant", "fresh-client", "fresh-instance", 1001, strings.Repeat("a", 64), now)
	require.NoError(t, db.Table("canvas_grants").Create(grantValues).Error)
	var grant CanvasGrant
	require.NoError(t, db.First(&grant, "id = ?", "fresh-grant").Error)
	assert.EqualValues(t, 1, grant.Revision)

	managed := CanvasManagedToken{
		GrantID: "fresh-grant", UserID: 1001, GroupID: "auto", TokenID: 2001,
		CredentialRevision: 1, PermissionRevision: 1, PermissionFingerprint: strings.Repeat("b", 64),
		AutoGroups: `["default","vip"]`, Status: CanvasManagedTokenStatusActive, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, db.Create(&managed).Error)
	require.NoError(t, db.Create(&CanvasIdempotencyOperation{
		GrantID: "fresh-grant", OperationID: "fresh-operation", RequestDigest: strings.Repeat("c", 64),
		ManagedTokenID: managed.ID, CreatedAt: now,
	}).Error)
	var saved CanvasManagedToken
	require.NoError(t, db.First(&saved, managed.ID).Error)
	assert.Equal(t, managed.AutoGroups, saved.AutoGroups)

	assert.Error(t, db.Table("canvas_grants").Create(canvasGrantMigrationValues("duplicate-identity", "fresh-client", "fresh-instance", 1001, strings.Repeat("d", 64), now)).Error)
	duplicateGroup := managed
	duplicateGroup.ID, duplicateGroup.TokenID = 0, 2002
	assert.Error(t, db.Create(&duplicateGroup).Error)
	duplicateOperation := CanvasIdempotencyOperation{
		GrantID: "fresh-grant", OperationID: "fresh-operation", RequestDigest: strings.Repeat("e", 64),
		ManagedTokenID: managed.ID, CreatedAt: now,
	}
	assert.Error(t, db.Create(&duplicateOperation).Error)

	recorder.reset()
	require.NoError(t, db.AutoMigrate(&CanvasGrant{}, &CanvasManagedToken{}, &CanvasIdempotencyOperation{}))
	assert.Empty(t, recorder.schemaMutations(), "second Canvas account migration must not change the schema")
}

// testCanvasAccountBaselineUpgrade 校验新增字段不会丢失既有授权、管理关系或幂等记录。
func testCanvasAccountBaselineUpgrade(t *testing.T, db *gorm.DB, recorder *migrationSQLRecorder) {
	t.Helper()
	dropCanvasAccountMigrationTables(t, db)
	defer dropCanvasAccountMigrationTables(t, db)

	require.NoError(t, db.AutoMigrate(&canvasGrantMigrationBaseline{}, &canvasManagedTokenMigrationBaseline{}, &CanvasIdempotencyOperation{}))
	now := time.Now().Unix()
	legacyGrant := canvasGrantMigrationBaseline{
		ID: "legacy-grant", ClientID: "legacy-client", InstanceID: "legacy-instance", UserID: 3001,
		TokenHash: strings.Repeat("f", 64), Scopes: "identity:read groups:read tokens:manage",
		Status: CanvasGrantStatusActive, IssuedAt: now, ExpiresAt: now + 3600, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, db.Create(&legacyGrant).Error)
	legacyManaged := canvasManagedTokenMigrationBaseline{
		GrantID: "legacy-grant", UserID: 3001, GroupID: "default", TokenID: 4001,
		CredentialRevision: 3, PermissionRevision: 4, PermissionFingerprint: strings.Repeat("1", 64),
		Status: CanvasManagedTokenStatusActive, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, db.Create(&legacyManaged).Error)
	legacyOperation := CanvasIdempotencyOperation{
		GrantID: "legacy-grant", OperationID: "legacy-operation", RequestDigest: strings.Repeat("2", 64),
		ManagedTokenID: legacyManaged.ID, CreatedAt: now,
	}
	require.NoError(t, db.Create(&legacyOperation).Error)

	require.NoError(t, db.AutoMigrate(&CanvasGrant{}, &CanvasManagedToken{}, &CanvasIdempotencyOperation{}))
	var grant CanvasGrant
	var managed CanvasManagedToken
	var operation CanvasIdempotencyOperation
	require.NoError(t, db.First(&grant, "id = ?", legacyGrant.ID).Error)
	require.NoError(t, db.First(&managed, legacyManaged.ID).Error)
	require.NoError(t, db.First(&operation, legacyOperation.ID).Error)
	assert.EqualValues(t, 1, grant.Revision)
	assert.Equal(t, legacyGrant.TokenHash, grant.TokenHash)
	assert.Equal(t, legacyManaged.PermissionFingerprint, managed.PermissionFingerprint)
	assert.Empty(t, managed.KeyFingerprint)
	assert.Empty(t, managed.AutoGroups)
	assert.Equal(t, legacyOperation.RequestDigest, operation.RequestDigest)

	require.NoError(t, db.Model(&CanvasManagedToken{}).Where("id = ?", managed.ID).Update("auto_groups", `["default","vip"]`).Error)
	require.NoError(t, db.First(&managed, legacyManaged.ID).Error)
	assert.Equal(t, `["default","vip"]`, managed.AutoGroups)
	// custom.16 已含 Revision/AutoGroups，仅缺轮换指纹列；升级时不能丢失已有非空范围。
	require.NoError(t, db.Migrator().DropColumn(&CanvasManagedToken{}, "KeyFingerprint"))
	require.NoError(t, db.AutoMigrate(&CanvasGrant{}, &CanvasManagedToken{}, &CanvasIdempotencyOperation{}))
	require.NoError(t, db.First(&managed, legacyManaged.ID).Error)
	assert.Empty(t, managed.KeyFingerprint)
	assert.Equal(t, `["default","vip"]`, managed.AutoGroups)
	assert.Equal(t, legacyManaged.CredentialRevision, managed.CredentialRevision)

	recorder.reset()
	require.NoError(t, db.AutoMigrate(&CanvasGrant{}, &CanvasManagedToken{}, &CanvasIdempotencyOperation{}))
	assert.Empty(t, recorder.schemaMutations(), "upgraded Canvas account schema must remain stable")
}

// canvasGrantMigrationValues 返回省略 Revision 的授权行，用于验证数据库默认值。
func canvasGrantMigrationValues(id, clientID, instanceID string, userID int, tokenHash string, now int64) map[string]any {
	return map[string]any{
		"id": id, "client_id": clientID, "instance_id": instanceID, "user_id": userID,
		"token_hash": tokenHash, "scopes": "identity:read groups:read tokens:manage", "status": CanvasGrantStatusActive,
		"issued_at": now, "expires_at": now + 3600, "revoked_at": 0, "created_at": now, "updated_at": now,
	}
}

// dropCanvasAccountMigrationTables 清理仅供迁移矩阵使用的隔离表。
func dropCanvasAccountMigrationTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Migrator().DropTable(&CanvasIdempotencyOperation{}, &CanvasManagedToken{}, &CanvasGrant{}))
}
