package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	// groupVisibilitySecretGroup 是本组输出可见性测试使用的精确隐藏分组名。
	groupVisibilitySecretGroup = "神秘分组"
	// groupVisibilitySimilarGroup 用于验证相近名称不会被误过滤。
	groupVisibilitySimilarGroup = "神秘分组相近"
)

// groupVisibilityPricingResponse 描述定价接口中与分组可见性相关的响应字段。
type groupVisibilityPricingResponse struct {
	Success     bool               `json:"success"`
	Data        []model.Pricing    `json:"data"`
	GroupRatio  map[string]float64 `json:"group_ratio"`
	UsableGroup map[string]string  `json:"usable_group"`
	AutoGroups  []string           `json:"auto_groups"`
}

// groupVisibilityUserGroupsResponse 描述用户可选分组接口响应。
type groupVisibilityUserGroupsResponse struct {
	Success bool                      `json:"success"`
	Data    map[string]map[string]any `json:"data"`
}

// groupVisibilityPerfResponse 描述性能接口的统一响应包装。
type groupVisibilityPerfResponse[T any] struct {
	Success bool `json:"success"`
	Data    T    `json:"data"`
}

// groupVisibilityTokenAutoGroupsResponse 描述 Token Auto 分组选项接口响应。
type groupVisibilityTokenAutoGroupsResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Groups   []string `json:"groups"`
		MaxCount int      `json:"max_count"`
	} `json:"data"`
}

// setupGroupVisibilityControllerTest 初始化分组可见性接口所需的隔离数据和全局设置。
func setupGroupVisibilityControllerTest(t *testing.T) (*gorm.DB, map[int]int) {
	t.Helper()

	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalGroupRatios := ratio_setting.GroupRatio2JSONString()
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalMemoryCacheEnabled := common.MemoryCacheEnabled

	db := setupModelListControllerTestDB(t)
	common.MemoryCacheEnabled = false
	require.NoError(t, db.AutoMigrate(&model.PerfMetric{}, &model.UserSession{}))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(fmt.Sprintf(
		`{"auto":"Auto","default":"Default","vip":"VIP","%s":"Secret","%s":"Similar"}`,
		groupVisibilitySecretGroup,
		groupVisibilitySimilarGroup,
	)))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(fmt.Sprintf(
		`{"default":1,"vip":2,"%s":3,"%s":4}`,
		groupVisibilitySecretGroup,
		groupVisibilitySimilarGroup,
	)))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(fmt.Sprintf(
		`["%s","default","%s"]`,
		groupVisibilitySecretGroup,
		groupVisibilitySimilarGroup,
	)))

	roles := []int{
		common.RoleGuestUser,
		common.RoleCommonUser,
		common.RoleAdminUser,
		common.RoleRootUser,
	}
	userIDs := make(map[int]int, len(roles))
	for _, role := range roles {
		user := model.User{
			Username: fmt.Sprintf("group-visibility-%d", role),
			Password: "password",
			Role:     role,
			Status:   common.UserStatusEnabled,
			Group:    "default",
			AffCode:  fmt.Sprintf("gv-%d", role),
		}
		require.NoError(t, db.Create(&user).Error)
		userIDs[role] = user.Id
	}

	channels := []model.Channel{
		{Id: 9101, Type: constant.ChannelTypeOpenAI, Key: "default-key", Status: common.ChannelStatusEnabled, Name: "default", Group: "default", Models: "visibility-default,visibility-shared"},
		{Id: 9102, Type: constant.ChannelTypeOpenAI, Key: "secret-key", Status: common.ChannelStatusEnabled, Name: "secret", Group: groupVisibilitySecretGroup, Models: "visibility-secret-only,visibility-shared"},
		{Id: 9103, Type: constant.ChannelTypeOpenAI, Key: "similar-key", Status: common.ChannelStatusEnabled, Name: "similar", Group: groupVisibilitySimilarGroup, Models: "visibility-similar"},
	}
	require.NoError(t, db.Create(&channels).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: "default", Model: "visibility-default", ChannelId: 9101, Enabled: true},
		{Group: "default", Model: "visibility-shared", ChannelId: 9101, Enabled: true},
		{Group: groupVisibilitySecretGroup, Model: "visibility-secret-only", ChannelId: 9102, Enabled: true},
		{Group: groupVisibilitySecretGroup, Model: "visibility-shared", ChannelId: 9102, Enabled: true},
		{Group: groupVisibilitySimilarGroup, Model: "visibility-similar", ChannelId: 9103, Enabled: true},
	}).Error)
	model.InvalidatePricingCache()

	t.Cleanup(func() {
		model.InvalidatePricingCache()
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios))
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
	})

	return db, userIDs
}

// groupVisibilityRequest 使用真实 Gin Context 调用 controller 并返回响应记录器。
func groupVisibilityRequest(t *testing.T, path string, role int, userID int, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, path, nil)
	context.Set("role", role)
	if userID != 0 {
		context.Set("id", userID)
	}
	handler(context)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	return recorder
}

// TestGroupVisibilityPricingRoleMatrixAndCacheIsolation 验证定价字段过滤及按角色读取不会污染全局缓存。
func TestGroupVisibilityPricingRoleMatrixAndCacheIsolation(t *testing.T) {
	_, userIDs := setupGroupVisibilityControllerTest(t)

	for _, test := range []struct {
		name          string
		role          int
		secretVisible bool
	}{
		{name: "admin first", role: common.RoleAdminUser, secretVisible: true},
		{name: "guest", role: common.RoleGuestUser},
		{name: "common after admin", role: common.RoleCommonUser},
		{name: "root", role: common.RoleRootUser, secretVisible: true},
		{name: "admin after common", role: common.RoleAdminUser, secretVisible: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := groupVisibilityRequest(t, "/api/pricing", test.role, userIDs[test.role], GetPricing)
			var response groupVisibilityPricingResponse
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.True(t, response.Success)

			pricing := pricingByModelName(response.Data)
			assert.Contains(t, pricing, "visibility-default")
			assert.Contains(t, pricing, "visibility-shared")
			assert.Contains(t, pricing, "visibility-similar")
			assert.Equal(t, test.secretVisible, pricing["visibility-secret-only"].ModelName != "")
			assert.Equal(t, test.secretVisible, slices.Contains(pricing["visibility-shared"].EnableGroup, groupVisibilitySecretGroup))
			assert.Contains(t, pricing["visibility-shared"].EnableGroup, "default")

			_, usableHasSecret := response.UsableGroup[groupVisibilitySecretGroup]
			_, ratioHasSecret := response.GroupRatio[groupVisibilitySecretGroup]
			assert.Equal(t, test.secretVisible, usableHasSecret)
			assert.Equal(t, test.secretVisible, ratioHasSecret)
			assert.Equal(t, test.secretVisible, slices.Contains(response.AutoGroups, groupVisibilitySecretGroup))
			assert.Contains(t, response.UsableGroup, groupVisibilitySimilarGroup)
			assert.Contains(t, response.GroupRatio, groupVisibilitySimilarGroup)
			assert.Contains(t, response.AutoGroups, groupVisibilitySimilarGroup)
		})
	}

	globalPricing := pricingByModelName(model.GetPricing())
	require.Contains(t, globalPricing, "visibility-secret-only")
	assert.ElementsMatch(t, []string{"default", groupVisibilitySecretGroup}, globalPricing["visibility-shared"].EnableGroup)
}

// TestGroupVisibilityUserGroupsRoleMatrix 验证用户可选分组按可信角色过滤且保留相近名称。
func TestGroupVisibilityUserGroupsRoleMatrix(t *testing.T) {
	_, userIDs := setupGroupVisibilityControllerTest(t)

	for _, test := range []struct {
		name          string
		role          int
		secretVisible bool
	}{
		{name: "guest", role: common.RoleGuestUser},
		{name: "common", role: common.RoleCommonUser},
		{name: "admin", role: common.RoleAdminUser, secretVisible: true},
		{name: "root", role: common.RoleRootUser, secretVisible: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := groupVisibilityRequest(t, "/api/user/groups", test.role, userIDs[test.role], GetUserGroups)
			var response groupVisibilityUserGroupsResponse
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.True(t, response.Success)
			_, hasSecret := response.Data[groupVisibilitySecretGroup]
			assert.Equal(t, test.secretVisible, hasSecret)
			assert.Contains(t, response.Data, "default")
			assert.Contains(t, response.Data, "auto")
			assert.Contains(t, response.Data, groupVisibilitySimilarGroup)
		})
	}

	adminGroupsRecorder := groupVisibilityRequest(t, "/api/group", common.RoleAdminUser, userIDs[common.RoleAdminUser], GetGroups)
	var adminGroups struct {
		Success bool     `json:"success"`
		Data    []string `json:"data"`
	}
	require.NoError(t, common.Unmarshal(adminGroupsRecorder.Body.Bytes(), &adminGroups))
	require.True(t, adminGroups.Success)
	assert.Contains(t, adminGroups.Data, groupVisibilitySecretGroup)
}

// TestGroupVisibilityTokenAutoGroupsRoleMatrix 验证 Token 编辑选项隐藏低权限角色不可见的自动分组。
func TestGroupVisibilityTokenAutoGroupsRoleMatrix(t *testing.T) {
	_, userIDs := setupGroupVisibilityControllerTest(t)

	for _, test := range []struct {
		name          string
		role          int
		secretVisible bool
	}{
		{name: "guest", role: common.RoleGuestUser},
		{name: "common", role: common.RoleCommonUser},
		{name: "admin", role: common.RoleAdminUser, secretVisible: true},
		{name: "root", role: common.RoleRootUser, secretVisible: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := groupVisibilityRequest(t, "/api/token/auto-groups", test.role, userIDs[test.role], GetTokenAutoGroups)
			var response groupVisibilityTokenAutoGroupsResponse
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.True(t, response.Success)
			assert.Equal(t, test.secretVisible, slices.Contains(response.Data.Groups, groupVisibilitySecretGroup))
			assert.Contains(t, response.Data.Groups, "default")
			assert.Contains(t, response.Data.Groups, groupVisibilitySimilarGroup)
			assert.Equal(t, setting.GetMaxTokenAutoGroups(), response.Data.MaxCount)
		})
	}
}

// TestGroupVisibilityUsesAuthenticatedRole 验证公开目录只信任会话角色且禁止缓存角色相关响应。
func TestGroupVisibilityUsesAuthenticatedRole(t *testing.T) {
	db, userIDs := setupGroupVisibilityControllerTest(t)

	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	previousHeaderNavModules, hadHeaderNavModules := common.OptionMap["HeaderNavModules"]
	common.OptionMap["HeaderNavModules"] = `{"pricing":{"enabled":true,"requireAuth":false}}`
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		if hadHeaderNavModules {
			common.OptionMap["HeaderNavModules"] = previousHeaderNavModules
		} else {
			delete(common.OptionMap, "HeaderNavModules")
		}
	})

	commonBundle, err := service.CreateLoginSession(userIDs[common.RoleCommonUser], "password", "127.0.0.1", "group-visibility-test")
	require.NoError(t, err)
	adminBundle, err := service.CreateLoginSession(userIDs[common.RoleAdminUser], "password", "127.0.0.1", "group-visibility-test")
	require.NoError(t, err)
	rootBundle, err := service.CreateLoginSession(userIDs[common.RoleRootUser], "password", "127.0.0.1", "group-visibility-test")
	require.NoError(t, err)

	router := gin.New()
	router.GET("/pricing", middleware.DisableCache(), middleware.HeaderNavModuleAuth("pricing"), GetPricing)
	router.GET("/groups", middleware.DisableCache(), middleware.TryUserAuth(), GetUserGroups)

	for _, test := range []struct {
		name          string
		path          string
		accessToken   string
		secretVisible bool
	}{
		{name: "anonymous spoofed pricing role", path: "/pricing?role=100"},
		{name: "common spoofed pricing role", path: "/pricing?role=100", accessToken: commonBundle.AccessToken},
		{name: "admin pricing", path: "/pricing", accessToken: adminBundle.AccessToken, secretVisible: true},
		{name: "anonymous spoofed group role", path: "/groups?role=100"},
		{name: "admin groups", path: "/groups", accessToken: adminBundle.AccessToken, secretVisible: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			request.Header.Set("X-Role", "100")
			request.Header.Set("Role", "100")
			if test.accessToken != "" {
				request.Header.Set("Authorization", "Bearer "+test.accessToken)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			assert.Contains(t, recorder.Header().Get("Cache-Control"), "no-store")

			if request.URL.Path == "/pricing" {
				var response groupVisibilityPricingResponse
				require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
				_, hasSecret := pricingByModelName(response.Data)["visibility-secret-only"]
				assert.Equal(t, test.secretVisible, hasSecret)
				return
			}
			var response groupVisibilityUserGroupsResponse
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			_, hasSecret := response.Data[groupVisibilitySecretGroup]
			assert.Equal(t, test.secretVisible, hasSecret)
		})
	}

	require.NoError(t, db.Model(&model.User{}).
		Where("id = ?", userIDs[common.RoleAdminUser]).
		Update("role", common.RoleCommonUser).Error)
	downgradedRequest := httptest.NewRequest(http.MethodGet, "/pricing", nil)
	downgradedRequest.Header.Set("Authorization", "Bearer "+adminBundle.AccessToken)
	downgradedRecorder := httptest.NewRecorder()
	router.ServeHTTP(downgradedRecorder, downgradedRequest)
	require.Equal(t, http.StatusOK, downgradedRecorder.Code, downgradedRecorder.Body.String())
	var downgraded groupVisibilityPricingResponse
	require.NoError(t, common.Unmarshal(downgradedRecorder.Body.Bytes(), &downgraded))
	assert.NotContains(t, pricingByModelName(downgraded.Data), "visibility-secret-only")

	require.NoError(t, db.Model(&model.UserSession{}).
		Where("sid = ?", rootBundle.Session.SID).
		Update("expires_at", time.Now().Add(-time.Minute).Unix()).Error)
	expiredRequest := httptest.NewRequest(http.MethodGet, "/groups", nil)
	expiredRequest.Header.Set("Authorization", "Bearer "+rootBundle.AccessToken)
	expiredRecorder := httptest.NewRecorder()
	router.ServeHTTP(expiredRecorder, expiredRequest)
	assert.Equal(t, http.StatusUnauthorized, expiredRecorder.Code)
	assert.Contains(t, expiredRecorder.Header().Get("Cache-Control"), "no-store")
}

// TestGroupVisibilityPerformanceRoleMatrix 验证性能明细和汇总不向低权限角色泄露神秘分组样本。
func TestGroupVisibilityPerformanceRoleMatrix(t *testing.T) {
	db, userIDs := setupGroupVisibilityControllerTest(t)
	bucketTs := time.Now().Add(-time.Minute).Unix()
	require.NoError(t, db.Create(&[]model.PerfMetric{
		{ModelName: "visibility-default", Group: "default", BucketTs: bucketTs, RequestCount: 2, SuccessCount: 2, TotalLatencyMs: 200, OutputTokens: 20, GenerationMs: 2000},
		{ModelName: "visibility-shared", Group: "default", BucketTs: bucketTs, RequestCount: 2, SuccessCount: 2, TotalLatencyMs: 200, OutputTokens: 20, GenerationMs: 2000},
		{ModelName: "visibility-shared", Group: groupVisibilitySecretGroup, BucketTs: bucketTs, RequestCount: 2, SuccessCount: 0, TotalLatencyMs: 1800, OutputTokens: 20, GenerationMs: 2000},
		{ModelName: "visibility-secret-only", Group: groupVisibilitySecretGroup, BucketTs: bucketTs, RequestCount: 3, SuccessCount: 0, TotalLatencyMs: 2700, OutputTokens: 30, GenerationMs: 3000},
		{ModelName: "visibility-similar", Group: groupVisibilitySimilarGroup, BucketTs: bucketTs, RequestCount: 1, SuccessCount: 1, TotalLatencyMs: 300, OutputTokens: 10, GenerationMs: 1000},
	}).Error)

	for _, test := range []struct {
		name          string
		role          int
		secretVisible bool
	}{
		{name: "guest", role: common.RoleGuestUser},
		{name: "common", role: common.RoleCommonUser},
		{name: "admin", role: common.RoleAdminUser, secretVisible: true},
		{name: "root", role: common.RoleRootUser, secretVisible: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			summaryRecorder := groupVisibilityRequest(t, "/api/perf-metrics/summary?hours=1", test.role, userIDs[test.role], GetPerfMetricsSummary)
			var summary groupVisibilityPerfResponse[perfmetrics.SummaryAllResult]
			require.NoError(t, common.Unmarshal(summaryRecorder.Body.Bytes(), &summary))
			require.True(t, summary.Success)
			byModel := make(map[string]perfmetrics.ModelSummary, len(summary.Data.Models))
			for _, item := range summary.Data.Models {
				byModel[item.ModelName] = item
			}
			assert.Contains(t, byModel, "visibility-default")
			assert.Contains(t, byModel, "visibility-shared")
			assert.Contains(t, byModel, "visibility-similar")
			_, hasSecretOnly := byModel["visibility-secret-only"]
			assert.Equal(t, test.secretVisible, hasSecretOnly)
			if test.secretVisible {
				assert.Equal(t, int64(500), byModel["visibility-shared"].AvgLatencyMs)
				assert.Equal(t, float64(50), byModel["visibility-shared"].SuccessRate)
			} else {
				assert.Equal(t, int64(100), byModel["visibility-shared"].AvgLatencyMs)
				assert.Equal(t, float64(100), byModel["visibility-shared"].SuccessRate)
			}

			detailRecorder := groupVisibilityRequest(t, "/api/perf-metrics?model=visibility-shared&hours=1", test.role, userIDs[test.role], GetPerfMetrics)
			var detail groupVisibilityPerfResponse[perfmetrics.QueryResult]
			require.NoError(t, common.Unmarshal(detailRecorder.Body.Bytes(), &detail))
			require.True(t, detail.Success)
			groups := make([]string, 0, len(detail.Data.Groups))
			for _, item := range detail.Data.Groups {
				groups = append(groups, item.Group)
			}
			assert.Contains(t, groups, "default")
			assert.Equal(t, test.secretVisible, slices.Contains(groups, groupVisibilitySecretGroup))

			explicitRecorder := groupVisibilityRequest(t, "/api/perf-metrics?model=visibility-shared&group="+groupVisibilitySecretGroup+"&hours=1", test.role, userIDs[test.role], GetPerfMetrics)
			var explicit groupVisibilityPerfResponse[perfmetrics.QueryResult]
			require.NoError(t, common.Unmarshal(explicitRecorder.Body.Bytes(), &explicit))
			require.True(t, explicit.Success)
			if test.secretVisible {
				require.Len(t, explicit.Data.Groups, 1)
				assert.Equal(t, groupVisibilitySecretGroup, explicit.Data.Groups[0].Group)
			} else {
				assert.Empty(t, explicit.Data.Groups)
			}
		})
	}
}
