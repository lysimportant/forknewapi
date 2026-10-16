package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchSeedanceModelNames(t *testing.T) {
	plugin, ok := jsplugin.DefaultRegistry.Get("seedance-video")
	require.True(t, ok)
	require.Empty(t, plugin.Meta.Models)
	for _, test := range []struct {
		name      string
		body      string
		status    int
		want      map[string]string
		wantError bool
	}{
		{name: "dynamic model ID", body: `{"data":[{"id":"upstream-seedance","name":"可用"}]}`, want: map[string]string{"upstream-seedance": "可用"}},
		{name: "OpenAI ID only", body: `{"data":[{"id":"upstream-seedance"}]}`, want: map[string]string{"upstream-seedance": "upstream-seedance"}},
		{name: "sold out", body: `{"data":[{"id":"upstream-seedance","name":"今日售罄"}]}`, want: map[string]string{"upstream-seedance": "今日售罄"}},
		{name: "valid empty catalog", body: `{"data":[]}`, want: map[string]string{}},
		{name: "missing data", body: `{}`, wantError: true},
		{name: "null data", body: `{"data":null}`, wantError: true},
		{name: "invalid body", body: `not-json`, wantError: true},
		{name: "blank ID", body: `{"data":[{"id":" "}]}`, wantError: true},
		{name: "duplicate ID", body: `{"data":[{"id":"same"},{"id":"same"}]}`, wantError: true},
		{name: "upstream failure", status: http.StatusBadGateway, body: `{"secret":"private-upstream-body"}`, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "/v1/models", r.URL.Path)
				assert.Equal(t, "Bearer fixture-private-key", r.Header.Get("Authorization"))
				if test.status != 0 {
					w.WriteHeader(test.status)
				}
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			baseURL, setting := server.URL+"/v1", `{"task_plugin_key":"seedance-video"}`
			names, err := fetchSeedanceModelNames(&model.Channel{Key: "fixture-private-key", BaseURL: &baseURL, Setting: &setting})
			if test.wantError {
				require.Error(t, err)
				assert.NotContains(t, err.Error(), "private-upstream-body")
				assert.NotContains(t, err.Error(), "fixture-private-key")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, names)
		})
	}
}

func TestGetSeedanceAvailabilityVisibilityMappingAndCache(t *testing.T) {
	db, userIDs := setupGroupVisibilityControllerTest(t)
	var primaryCalls, secondaryCalls, failedCalls, privateCalls atomic.Int32
	var primaryBody atomic.Value
	primaryBody.Store(`{"data":[{"id":"up-active"},{"id":"up-shared","name":"售罄"},{"id":"up-sold","name":"告罄"},{"id":"up-uncertain","name":"售罄"}]}`)
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryCalls.Add(1)
		_, _ = w.Write([]byte(primaryBody.Load().(string)))
	}))
	defer primary.Close()
	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondaryCalls.Add(1)
		_, _ = w.Write([]byte(`{"data":[{"id":"up-shared"}]}`))
	}))
	defer secondary.Close()
	failed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		failedCalls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"secret":"private-upstream-body"}`))
	}))
	defer failed.Close()
	private := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		privateCalls.Add(1)
		_, _ = w.Write([]byte(`{"data":[{"id":"up-private"}]}`))
	}))
	defer private.Close()
	setting, otherSetting := `{"task_plugin_key":"seedance-video"}`, `{"task_plugin_key":"yuanliu"}`
	primaryURL, secondaryURL, failedURL, privateURL := primary.URL, secondary.URL, failed.URL, private.URL
	primaryMapping := `{"Seed-active":"up-active","Seed-shared":"up-shared","Seed-sold":"up-sold","Seed-missing":"up-missing","Seed-chain":"bridge","bridge":"up-active","Seed-uncertain":"up-uncertain","Seed-cycle":"cycle","cycle":"Seed-cycle"}`
	secondaryMapping := `{"Seed-shared":"up-shared"}`
	failedMapping := `{"Seed-uncertain":"up-uncertain"}`
	privateMapping := `{"Seed-secret":"up-private","Seed-own-group":"up-private"}`
	channels := []model.Channel{
		{Id: 9401, Type: constant.ChannelTypeTaskPlugin, Key: "fixture-primary", Status: common.ChannelStatusEnabled, Name: "primary", Group: "default", Models: "Seed-active,Seed-shared,Seed-sold,Seed-missing,Seed-chain,Seed-uncertain,Seed-cycle", BaseURL: &primaryURL, Setting: &setting, ModelMapping: &primaryMapping},
		{Id: 9402, Type: constant.ChannelTypeTaskPlugin, Key: "fixture-secondary", Status: common.ChannelStatusEnabled, Name: "secondary", Group: "default", Models: "Seed-shared", BaseURL: &secondaryURL, Setting: &setting, ModelMapping: &secondaryMapping},
		{Id: 9403, Type: constant.ChannelTypeTaskPlugin, Key: "fixture-failed", Status: common.ChannelStatusEnabled, Name: "failed", Group: "default", Models: "Seed-uncertain", BaseURL: &failedURL, Setting: &setting, ModelMapping: &failedMapping},
		{Id: 9404, Type: constant.ChannelTypeTaskPlugin, Key: "fixture-secret", Status: common.ChannelStatusEnabled, Name: "secret", Group: groupVisibilitySecretGroup, Models: "Seed-secret", BaseURL: &privateURL, Setting: &setting, ModelMapping: &privateMapping},
		{Id: 9405, Type: constant.ChannelTypeTaskPlugin, Key: "fixture-own", Status: common.ChannelStatusEnabled, Name: "own group", Group: "owner-only", Models: "Seed-own-group", BaseURL: &privateURL, Setting: &setting, ModelMapping: &privateMapping},
		{Id: 9406, Type: constant.ChannelTypeTaskPlugin, Key: "fixture-disabled", Status: common.ChannelStatusManuallyDisabled, Name: "disabled", Group: "default", Models: "Seed-disabled-channel", BaseURL: &privateURL, Setting: &setting},
		{Id: 9407, Type: constant.ChannelTypeTaskPlugin, Key: "fixture-other", Status: common.ChannelStatusEnabled, Name: "another plugin", Group: "default", Models: "Seed-other-plugin", BaseURL: &privateURL, Setting: &otherSetting},
		{Id: 9408, Type: constant.ChannelTypeTaskPlugin, Key: "fixture-multi-a\nfixture-multi-b", Status: common.ChannelStatusEnabled, Name: "multi key", Group: "default", Models: "Seed-multi-missing", BaseURL: &primaryURL, Setting: &setting, ChannelInfo: model.ChannelInfo{IsMultiKey: true}},
	}
	require.NoError(t, db.Create(&channels).Error)
	var abilities []model.Ability
	for _, channel := range channels {
		for _, name := range strings.Split(channel.Models, ",") {
			abilities = append(abilities, model.Ability{Group: channel.Group, Model: name, ChannelId: channel.Id, Enabled: true})
		}
	}
	require.NoError(t, db.Create(&abilities).Error)
	model.InvalidatePricingCache()
	owner := model.User{Username: "seedance-owner", Password: "fixture-password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "owner-only", AffCode: "seed-own"}
	require.NoError(t, db.Create(&owner).Error)
	read := func(role, userID int) map[string]yuanliuAvailability {
		t.Helper()
		recorder := groupVisibilityRequest(t, "/api/pricing/seedance-availability", role, userID, GetSeedanceAvailability)
		var response struct {
			Success bool `json:"success"`
			Data    struct {
				Models map[string]yuanliuAvailability `json:"models"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		require.True(t, response.Success)
		for _, secret := range []string{"fixture-primary", "fixture-secondary", "fixture-failed", "private-upstream-body", primary.URL, private.URL, "up-active", "up-private"} {
			assert.NotContains(t, recorder.Body.String(), secret)
		}
		return response.Data.Models
	}
	commonModels := read(common.RoleCommonUser, userIDs[common.RoleCommonUser])
	statuses := make(map[string]string)
	for name, item := range commonModels {
		statuses[name] = item.Status
		if item.Status == "unknown" {
			assert.Nil(t, item.CheckedAt)
		} else {
			require.NotNil(t, item.CheckedAt)
			_, err := time.Parse(time.RFC3339, *item.CheckedAt)
			assert.NoError(t, err)
		}
	}
	assert.Equal(t, map[string]string{
		"Seed-active": "available", "Seed-shared": "available", "Seed-sold": "sold_out",
		"Seed-missing": "disabled", "Seed-chain": "available", "Seed-uncertain": "unknown", "Seed-multi-missing": "unknown",
	}, statuses)
	assert.Equal(t, int32(0), privateCalls.Load())
	adminModels := read(common.RoleAdminUser, userIDs[common.RoleAdminUser])
	assert.Equal(t, "available", adminModels["Seed-secret"].Status)
	assert.NotContains(t, adminModels, "Seed-own-group")
	ownerModels := read(common.RoleCommonUser, owner.Id)
	assert.Equal(t, "available", ownerModels["Seed-own-group"].Status)
	assert.NotContains(t, ownerModels, "Seed-secret")
	assert.NotContains(t, read(common.RoleCommonUser, userIDs[common.RoleCommonUser]), "Seed-secret")
	assert.Equal(t, int32(2), primaryCalls.Load()) // primary + multi-key channel, then cached across users
	assert.Equal(t, int32(1), secondaryCalls.Load())
	assert.Equal(t, int32(1), failedCalls.Load())
	assert.Equal(t, int32(2), privateCalls.Load()) // separate admin and owner channels only

	// Valid empty directory replaces the cached snapshot after expiry, retaining public model cards.
	primaryBody.Store(`{"data":[]}`)
	assert.Equal(t, "available", read(common.RoleCommonUser, userIDs[common.RoleCommonUser])["Seed-active"].Status)
	pricingSeedanceCatalogCache.Lock()
	snapshot := pricingSeedanceCatalogCache.entries[9401]
	snapshot.expiresAt = time.Now().Add(-time.Second)
	pricingSeedanceCatalogCache.entries[9401] = snapshot
	pricingSeedanceCatalogCache.Unlock()
	refreshed := read(common.RoleCommonUser, userIDs[common.RoleCommonUser])
	assert.Equal(t, "disabled", refreshed["Seed-active"].Status)
	assert.Equal(t, "disabled", refreshed["Seed-chain"].Status)
	assert.Equal(t, "available", refreshed["Seed-shared"].Status)
	assert.Equal(t, int32(3), primaryCalls.Load())
	var reloaded model.Channel
	require.NoError(t, db.First(&reloaded, 9401).Error)
	assert.Equal(t, channels[0].Models, reloaded.Models)
	assert.Equal(t, common.ChannelStatusEnabled, reloaded.Status)
}

func TestTaskPluginCatalogCacheSeparatesPluginIdentity(t *testing.T) {
	cache := yuanliuCatalogCache{entries: make(map[int]yuanliuCatalogSnapshot)}
	channel := &model.Channel{Id: 9501, Key: "fixture-key"}
	plugin := jsplugin.Meta{Key: "yuanliu", Version: "1.0.0"}
	var calls atomic.Int32
	fetch := func(*model.Channel) (map[string]string, error) {
		return map[string]string{"target": fmt.Sprint(calls.Add(1))}, nil
	}
	assert.Equal(t, "1", cache.get(context.Background(), channel, plugin, fetch).names["target"])
	plugin.Key = "seedance-video"
	assert.Equal(t, "2", cache.get(context.Background(), channel, plugin, fetch).names["target"])
	plugin.ModelRouting = "channel"
	assert.Equal(t, "3", cache.get(context.Background(), channel, plugin, fetch).names["target"])
	assert.Equal(t, "3", cache.get(context.Background(), channel, plugin, fetch).names["target"])
}
