package model

import (
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenAutoGroupsRoundTripThroughRedisHashCache(t *testing.T) {
	useUserCacheMiniRedis(t)
	token := Token{
		Id:         42,
		UserId:     7,
		Key:        "token-auto-groups-cache-key",
		Name:       "auto-cache",
		Group:      "auto",
		AutoGroups: `["vip","default"]`,
	}

	require.NoError(t, cacheSetTokenForTest(token))
	cached, err := cacheGetTokenByKey(token.Key)
	require.NoError(t, err)
	assert.Equal(t, token.AutoGroups, cached.AutoGroups)
	groups, err := cached.GetAutoGroups()
	require.NoError(t, err)
	assert.Equal(t, []string{"vip", "default"}, groups)
}

func TestTokenUpdateSynchronouslyNarrowsPreheatedAutoGroupsCache(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)
	token := Token{
		UserId:          7,
		Key:             "token-auto-groups-update-cache-key",
		Name:            "auto-cache-update",
		Status:          common.TokenStatusEnabled,
		ExpiredTime:     -1,
		UnlimitedQuota:  true,
		Group:           "auto",
		CrossGroupRetry: true,
		AutoGroups:      `["default","vip"]`,
	}
	require.NoError(t, token.Insert())
	require.NoError(t, cacheSetTokenForTest(token))

	preheated, err := cacheGetTokenByKey(token.Key)
	require.NoError(t, err)
	assert.JSONEq(t, `["default","vip"]`, preheated.AutoGroups)

	require.NoError(t, token.SetAutoGroups([]string{"vip"}))
	require.NoError(t, token.Update())
	// Update 是限制性变更：写库前删除缓存并设置 fence。缓存不再提供旧的
	// 宽分组值，下一次读取必须看到收紧后的分组。
	_, cacheErr := cacheGetTokenByKey(token.Key)
	require.Error(t, cacheErr, "the pre-update cache entry must be invalidated")
	reloaded, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.JSONEq(t, `["vip"]`, reloaded.AutoGroups)
}

// TestCanvasManagedMutationsRollbackWhenCacheInvalidationFails 验证限制性变更不会在缓存失效失败时部分提交。
func TestCanvasManagedMutationsRollbackWhenCacheInvalidationFails(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&Option{}, &CanvasGrant{}, &CanvasManagedToken{}, &CanvasIdempotencyOperation{}))
	t.Cleanup(func() {
		DB.Exec("DELETE FROM canvas_idempotency_operations")
		DB.Exec("DELETE FROM canvas_managed_tokens")
		DB.Exec("DELETE FROM canvas_grants")
		DB.Exec("DELETE FROM options")
	})
	server := useUserCacheMiniRedis(t)

	now := time.Now().Unix()
	user := User{Username: "canvas-cache-rollback", Password: "test-password", Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, DB.Create(&user).Error)
	rawGrant := "canvas-cache-rollback-grant"
	grant := CanvasGrant{
		ID: "22222222-2222-4222-8222-222222222222", ClientID: "canvas", InstanceID: "cache-test", UserID: user.Id,
		TokenHash: CanvasGrantTokenHash(rawGrant), Scopes: strings.Join(common.CanvasAccountScopes(), " "), Status: CanvasGrantStatusActive,
		IssuedAt: now, ExpiresAt: now + 3600, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, DB.Create(&grant).Error)
	token := Token{
		UserId: user.Id, Key: "canvas-cache-rollback-token", Name: "Canvas-cache-auto", Status: common.TokenStatusEnabled,
		CreatedTime: now, AccessedTime: now, ExpiredTime: grant.ExpiresAt, UnlimitedQuota: true, Group: "auto",
	}
	require.NoError(t, token.SetAutoGroups([]string{"default", "vip"}))
	require.NoError(t, DB.Create(&token).Error)
	managed := CanvasManagedToken{
		GrantID: grant.ID, UserID: user.Id, GroupID: "auto", TokenID: token.Id, CredentialRevision: 1,
		AutoGroups: token.AutoGroups, Status: CanvasManagedTokenStatusActive, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, DB.Create(&managed).Error)
	_, err := ReconcileCanvasManagedAuthority(managed.ID)
	require.NoError(t, err)

	server.Close()
	_, err = EnsureCanvasManagedToken(CanvasManagedTokenInput{
		GrantID: grant.ID, GroupID: "auto", OperationID: "scope-update", AutoGroups: []string{"vip"},
	})
	require.ErrorContains(t, err, "invalidate managed token cache")
	require.NoError(t, DB.First(&token, token.Id).Error)
	assert.JSONEq(t, `["default","vip"]`, token.AutoGroups)
	require.NoError(t, DB.First(&managed, managed.ID).Error)
	assert.JSONEq(t, `["default","vip"]`, managed.AutoGroups)
	var operationCount int64
	require.NoError(t, DB.Model(&CanvasIdempotencyOperation{}).Where("grant_id = ? AND operation_id = ?", grant.ID, "scope-update").Count(&operationCount).Error)
	assert.Zero(t, operationCount)

	_, err = RevokeCanvasGrant(rawGrant)
	require.ErrorContains(t, err, "invalidate managed token cache")
	require.NoError(t, DB.First(&grant, "id = ?", grant.ID).Error)
	assert.Equal(t, CanvasGrantStatusActive, grant.Status)
	require.NoError(t, DB.First(&token, token.Id).Error)
	assert.Equal(t, common.TokenStatusEnabled, token.Status)
	require.NoError(t, DB.First(&managed, managed.ID).Error)
	assert.Equal(t, CanvasManagedTokenStatusActive, managed.Status)
}

// cacheSetTokenForTest 以测试身份写入完整 token 缓存（含额度字段），
// 模拟“已水合”的缓存状态。
func cacheSetTokenForTest(token Token) error {
	_, err := cacheInitToken(token)
	return err
}
