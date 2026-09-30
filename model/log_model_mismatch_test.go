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
	"gorm.io/driver/clickhouse"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// openModelMismatchLogDB 为日志查询创建独立测试表，并在结束后恢复数据库及列名全局状态。
// 外部 DSN 必须指向名称以 model_mismatch_ 开头且不含 logs 表的专用数据库；不清空已有表。
// 主库始终使用临时 SQLite，以同时验证独立日志库的渠道名称回填。
func openModelMismatchLogDB(t *testing.T, dialect string) {
	t.Helper()
	var driver gorm.Dialector
	versionQuery := "SELECT version()"
	databaseQuery := ""
	if dialect == "sqlite" {
		driver = sqlite.Open(filepath.Join(t.TempDir(), "logs.db"))
		versionQuery = "SELECT sqlite_version()"
	} else {
		envName := "TEST_" + strings.ToUpper(dialect) + "_DSN"
		dsn := strings.TrimSpace(os.Getenv(envName))
		if dsn == "" {
			t.Skip(envName + " 未配置，真实数据库验证未执行")
		}
		switch dialect {
		case "mysql":
			driver = mysql.Open(dsn)
			databaseQuery = "SELECT DATABASE()"
		case "postgres":
			driver = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
			databaseQuery = "SELECT current_database()"
		case "clickhouse":
			driver = clickhouse.Open(dsn)
			databaseQuery = "SELECT currentDatabase()"
		default:
			t.Fatalf("不支持的测试方言: %s", dialect)
		}
	}
	config := &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)}
	logDB, err := gorm.Open(driver, config)
	require.NoError(t, err)
	logSQL, err := logDB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, logSQL.Close()) })
	logSQL.SetMaxOpenConns(1)
	if databaseQuery != "" {
		var databaseName string
		require.NoError(t, logDB.Raw(databaseQuery).Scan(&databaseName).Error)
		require.True(t, strings.HasPrefix(databaseName, "model_mismatch_"), "只能使用 model_mismatch_ 前缀的隔离数据库")
	}
	require.False(t, logDB.Migrator().HasTable(&Log{}), "隔离数据库已存在 logs 表，请新建测试数据库")
	var version string
	require.NoError(t, logDB.Raw(versionQuery).Scan(&version).Error)
	t.Logf("真实日志数据库: %s %s", dialect, version)

	mainDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "main.db")), config)
	require.NoError(t, err)
	mainSQL, err := mainDB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, mainSQL.Close()) })
	channels := []struct {
		Id   int
		Name string
	}{{11, "primary-channel"}, {22, "alternate-channel"}}
	require.NoError(t, mainDB.Table("channels").AutoMigrate(&channels))
	require.NoError(t, mainDB.Table("channels").Create(&channels).Error)

	previousDB, previousLogDB := DB, LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	previousCache := common.MemoryCacheEnabled
	previousColumns := [6]string{commonGroupCol, commonKeyCol, commonTrueVal, commonFalseVal, logKeyCol, logGroupCol}
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
		common.MemoryCacheEnabled = previousCache
		commonGroupCol, commonKeyCol = previousColumns[0], previousColumns[1]
		commonTrueVal, commonFalseVal = previousColumns[2], previousColumns[3]
		logKeyCol, logGroupCol = previousColumns[4], previousColumns[5]
	})
	DB, LOG_DB = mainDB, logDB
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseType(dialect))
	common.MemoryCacheEnabled = false
	initCol()
	if dialect == "clickhouse" {
		require.NoError(t, logDB.Exec(clickHouseLogCreateTableSQL(0)).Error)
	} else {
		require.NoError(t, logDB.AutoMigrate(&Log{}))
	}
}

// TestLogModelMismatchQueries 验证查询参数 -1 仅匹配规范 JSON 的布尔标记，不修改持久化类型。
// 同一组真实数据库用例覆盖分页、组合筛选、用户隔离、角色投影及消费统计；不并行修改全局状态。
func TestLogModelMismatchQueries(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres", "clickhouse"} {
		t.Run(dialect, func(t *testing.T) {
			openModelMismatchLogDB(t, dialect)
			// 当前速率查询只有时间下界；将近期样本放到一小时后，避免慢速 CI 跨过 60 秒边界。
			recent := time.Now().Add(time.Hour).Unix()
			old := recent - 2*60*60
			metadata := []map[string]any{
				{"upstream_model_mismatch": true},
				{"upstream_model_mismatch": true},
				{"upstream_model_mismatch": false},
				{"upstream_model_name": "different-upstream-without-verification"},
				{"upstream_model_mismatch": "true"},
				{"is_model_mapped": true, "requested_model_name": "client-alias", "upstream_model_name": "mapped-model", "upstream_response_model": "mapped-model", "upstream_model_mismatch": false},
				{"upstreamXmodel_mismatch": true, "upstream_modelXmismatch": true, "upstreamXmodelXmismatch": true, "upstream_model_mismatch_detail": true, "prefix_upstream_model_mismatch": true},
				{"note": `embedded JSON: {"upstream_model_mismatch":true}`},
				{"upstream_model_mismatch": true},
				{"upstream_model_mismatch": true},
				{"upstream_model_mismatch": true},
				{"upstream_model_mismatch": true},
				{"upstream_model_mismatch": true},
			}
			quotas := []int{100, 10, 20, 30, 40, 50, 60, 70, 8000, 90, 110, 120, 13000}
			fixtures := make([]Log, len(metadata))
			for i, public := range metadata {
				other := NewLogOther()
				other.MergePublic(public)
				require.True(t, other.SetAdmin("diagnostic", "admin-fixture"))
				require.True(t, other.SetRoot("diagnostic", "root-fixture"))
				require.True(t, other.SetAudit("diagnostic", "audit-fixture"))
				fixtures[i] = Log{
					Id: i + 1, UserId: 101, CreatedAt: recent + int64(i+1), Type: LogTypeConsume,
					Username: "alice", TokenName: "token-a", ModelName: "target_model", ChannelId: 11, Group: "alpha",
					RequestId: fmt.Sprintf("req-%02d", i+1), UpstreamRequestId: fmt.Sprintf("up-%02d", i+1),
					Quota: quotas[i], PromptTokens: quotas[i] / 5, CompletionTokens: quotas[i] * 3 / 10,
					Other: other.JSONString(),
				}
			}
			fixtures[0].CreatedAt = old
			fixtures[8].Type = LogTypeError
			fixtures[9].UserId, fixtures[9].Username, fixtures[9].TokenName = 202, "bob", "token-b"
			fixtures[9].Group, fixtures[9].ChannelId, fixtures[9].ModelName = "beta", 22, "alternate-model"
			fixtures[10].Group = "beta"
			fixtures[11].ChannelId, fixtures[11].ModelName = 22, "alternate-model"
			fixtures[12].Type = LogTypeTopup
			require.NoError(t, LOG_DB.Create(&fixtures).Error)

			t.Run("lists", func(t *testing.T) {
				cases := []struct {
					name, model, username, token, group, request, upstream string
					user, logType, channel, offset, limit                  int
					start, end                                             int64
					want                                                   []int
					total                                                  int64
				}{
					{name: "canonical_true_only", logType: -1, want: []int{13, 12, 11, 10, 9, 2, 1}, total: 7},
					{name: "false", logType: -1, request: "req-03"},
					{name: "missing_flag_despite_different_model", logType: -1, request: "req-04"},
					{name: "string_true", logType: -1, request: "req-05"},
					{name: "normal_model_mapping", logType: -1, request: "req-06"},
					{name: "literal_underscores_and_complete_key", logType: -1, request: "req-07"},
					{name: "escaped_json_in_text", logType: -1, request: "req-08"},
					{name: "admin_page", logType: -1, offset: 2, limit: 2, want: []int{11, 10}, total: 7},
					{name: "admin_page_past_end", logType: -1, offset: 7, limit: 2, total: 7},
					{name: "group", logType: -1, group: "alpha", want: []int{13, 12, 9, 2, 1}, total: 5},
					{name: "channel", logType: -1, channel: 11, want: []int{13, 11, 9, 2, 1}, total: 5},
					{name: "inclusive_dates", logType: -1, start: recent + 2, end: recent + 9, want: []int{9, 2}, total: 2},
					{name: "model", logType: -1, model: "target_model", want: []int{13, 11, 9, 2, 1}, total: 5},
					{name: "model_wildcard", logType: -1, model: "target_%", want: []int{13, 11, 9, 2, 1}, total: 5},
					{name: "username", logType: -1, username: "alice", want: []int{13, 12, 11, 9, 2, 1}, total: 6},
					{name: "token", logType: -1, token: "token-a", want: []int{13, 12, 11, 9, 2, 1}, total: 6},
					{name: "request", logType: -1, request: "req-02", want: []int{2}, total: 1},
					{name: "upstream_request", logType: -1, upstream: "up-02", want: []int{2}, total: 1},
					{name: "all_filters_intersect", logType: -1, start: recent + 2, end: recent + 2, model: "target_model", username: "alice", token: "token-a", channel: 11, group: "alpha", request: "req-02", upstream: "up-02", want: []int{2}, total: 1},
					{name: "wrong_group", logType: -1, request: "req-02", group: "beta"},
					{name: "wrong_channel", logType: -1, request: "req-02", channel: 22},
					{name: "wrong_date", logType: -1, request: "req-02", start: recent + 3},
					{name: "wrong_model", logType: -1, request: "req-02", model: "alternate-model"},
					{name: "wrong_username", logType: -1, request: "req-02", username: "bob"},
					{name: "wrong_token", logType: -1, request: "req-02", token: "token-b"},
					{name: "wrong_upstream_request", logType: -1, request: "req-02", upstream: "up-03"},
					{name: "user_owns_only_six", user: 101, logType: -1, want: []int{13, 12, 11, 9, 2, 1}, total: 6},
					{name: "user_page", user: 101, logType: -1, offset: 2, limit: 2, want: []int{11, 9}, total: 6},
					{name: "user_page_past_end", user: 101, logType: -1, offset: 6, limit: 2, total: 6},
					{name: "other_user", user: 202, logType: -1, want: []int{10}, total: 1},
					{name: "unknown_user", user: 303, logType: -1},
					{name: "user_cannot_read_other_request", user: 101, logType: -1, request: "req-10"},
					{name: "user_all_filters_intersect", user: 101, logType: -1, start: recent + 2, end: recent + 2, model: "target_model", token: "token-a", group: "alpha", request: "req-02", upstream: "up-02", want: []int{2}, total: 1},
					{name: "user_group", user: 101, logType: -1, group: "beta", want: []int{11}, total: 1},
					{name: "user_model", user: 101, logType: -1, model: "alternate-model", want: []int{12}, total: 1},
					{name: "user_dates", user: 101, logType: -1, start: recent + 2, end: recent + 9, want: []int{9, 2}, total: 2},
					{name: "user_wrong_group", user: 101, logType: -1, request: "req-02", group: "beta"},
					{name: "user_wrong_date", user: 101, logType: -1, request: "req-02", start: recent + 3},
					{name: "user_wrong_model", user: 101, logType: -1, request: "req-02", model: "alternate-model"},
					{name: "user_wrong_token", user: 101, logType: -1, request: "req-02", token: "token-b"},
					{name: "user_wrong_upstream_request", user: 101, logType: -1, request: "req-02", upstream: "up-03"},
					{name: "legacy_all", logType: LogTypeUnknown, want: []int{13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}, total: 13},
					{name: "legacy_consume", logType: LogTypeConsume, want: []int{12, 11, 10, 8, 7, 6, 5, 4, 3, 2, 1}, total: 11},
					{name: "legacy_error", logType: LogTypeError, want: []int{9}, total: 1},
					{name: "legacy_user_all", user: 101, logType: LogTypeUnknown, want: []int{13, 12, 11, 9, 8, 7, 6, 5, 4, 3, 2, 1}, total: 12},
					{name: "legacy_user_consume", user: 101, logType: LogTypeConsume, want: []int{12, 11, 8, 7, 6, 5, 4, 3, 2, 1}, total: 10},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						limit := tc.limit
						if limit == 0 {
							limit = 50
						}
						var logs []*Log
						var total int64
						var err error
						if tc.user == 0 {
							logs, total, err = GetAllLogs(tc.logType, tc.start, tc.end, tc.model, tc.username, tc.token, tc.offset, limit, tc.channel, tc.group, tc.request, tc.upstream)
						} else {
							logs, total, err = GetUserLogs(tc.user, tc.logType, tc.start, tc.end, tc.model, tc.token, tc.offset, limit, tc.group, tc.request, tc.upstream)
						}
						require.NoError(t, err)
						assert.Equal(t, tc.total, total)
						require.Len(t, logs, len(tc.want))
						for i, expectedID := range tc.want {
							expected := fixtures[expectedID-1]
							assert.Equal(t, expected.RequestId, logs[i].RequestId)
							assert.Equal(t, expected.Type, logs[i].Type, "查询哨兵不能替换日志类型")
							if tc.user != 0 {
								assert.Equal(t, tc.user, logs[i].UserId)
								assert.Equal(t, tc.offset+i+1, logs[i].Id)
								assert.Empty(t, logs[i].ChannelName)
								other, err := common.StrToMap(logs[i].Other)
								require.NoError(t, err)
								assert.NotContains(t, other, "admin_info")
								assert.NotContains(t, other, "root_info")
								assert.NotContains(t, other, "audit_info")
								if tc.logType == -1 {
									assert.Equal(t, true, other["upstream_model_mismatch"])
								}
							} else {
								assert.Equal(t, expected.Other, logs[i].Other)
								assert.Equal(t, map[int]string{11: "primary-channel", 22: "alternate-channel"}[expected.ChannelId], logs[i].ChannelName)
							}
						}
					})
				}
			})

			t.Run("role_projection", func(t *testing.T) {
				logs, total, err := GetAllLogs(-1, 0, 0, "", "", "", 0, 10, 0, "", "req-02", "")
				require.NoError(t, err)
				require.EqualValues(t, 1, total)
				require.Len(t, logs, 1)
				rootLog, adminLog := *logs[0], *logs[0]
				FormatRootLogs([]*Log{&rootLog})
				FormatAdminLogs([]*Log{&adminLog})
				rootOther, err := common.StrToMap(rootLog.Other)
				require.NoError(t, err)
				adminOther, err := common.StrToMap(adminLog.Other)
				require.NoError(t, err)
				assert.Equal(t, true, rootOther["upstream_model_mismatch"])
				assert.Contains(t, rootOther, "root_info")
				assert.Contains(t, rootOther, "admin_info")
				assert.Equal(t, true, adminOther["upstream_model_mismatch"])
				assert.Contains(t, adminOther, "admin_info")
				assert.NotContains(t, adminOther, "root_info")
			})

			t.Run("stats", func(t *testing.T) {
				cases := []struct {
					name, model, username, token, group string
					logType, channel                    int
					start, end                          int64
					want                                Stat
				}{
					{name: "mismatch_consume_only", logType: -1, want: Stat{Quota: 430, Rpm: 4, Tpm: 165}},
					{name: "mismatch_username", logType: -1, username: "alice", want: Stat{Quota: 340, Rpm: 3, Tpm: 120}},
					{name: "mismatch_token", logType: -1, token: "token-a", want: Stat{Quota: 340, Rpm: 3, Tpm: 120}},
					{name: "mismatch_model", logType: -1, model: "target_model", want: Stat{Quota: 220, Rpm: 2, Tpm: 60}},
					{name: "mismatch_channel", logType: -1, channel: 11, want: Stat{Quota: 220, Rpm: 2, Tpm: 60}},
					{name: "mismatch_group", logType: -1, group: "alpha", want: Stat{Quota: 230, Rpm: 2, Tpm: 65}},
					{name: "mismatch_all_filters", logType: -1, username: "alice", token: "token-a", model: "target_%", channel: 11, group: "alpha", start: old, end: recent + 20, want: Stat{Quota: 110, Rpm: 1, Tpm: 5}},
					{name: "mismatch_recent_quota", logType: -1, start: recent, end: recent + 20, want: Stat{Quota: 330, Rpm: 4, Tpm: 165}},
					{name: "historical_quota_keeps_current_rates", logType: -1, start: old, end: old, want: Stat{Quota: 100, Rpm: 4, Tpm: 165}},
					{name: "empty_quota_window_keeps_current_rates", logType: -1, start: old + 1, end: old + 2, want: Stat{Rpm: 4, Tpm: 165}},
					{name: "empty_subset", logType: -1, token: "missing-token", want: Stat{}},
					{name: "legacy_all", logType: LogTypeUnknown, want: Stat{Quota: 700, Rpm: 10, Tpm: 300}},
					{name: "legacy_consume", logType: LogTypeConsume, want: Stat{Quota: 700, Rpm: 10, Tpm: 300}},
					{name: "legacy_error_parameter_is_still_consume_only", logType: LogTypeError, want: Stat{Quota: 700, Rpm: 10, Tpm: 300}},
					{name: "legacy_intersections", logType: LogTypeUnknown, username: "alice", token: "token-a", model: "target_model", channel: 11, group: "alpha", want: Stat{Quota: 380, Rpm: 7, Tpm: 140}},
					{name: "legacy_historical_quota_keeps_current_rates", logType: LogTypeConsume, start: old, end: old, want: Stat{Quota: 100, Rpm: 10, Tpm: 300}},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						stat, err := SumUsedQuota(tc.logType, tc.start, tc.end, tc.model, tc.username, tc.token, tc.channel, tc.group)
						require.NoError(t, err)
						assert.Equal(t, tc.want, stat)
					})
				}
			})

			t.Run("persisted_data_unchanged", func(t *testing.T) {
				var stored []Log
				require.NoError(t, LOG_DB.Order("id").Find(&stored).Error)
				assert.Equal(t, fixtures, stored, "列表、角色投影及统计查询不得重写日志数据")
			})
		})
	}
}
