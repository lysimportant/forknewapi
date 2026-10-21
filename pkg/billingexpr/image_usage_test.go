package billingexpr_test

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncImageUsageCountPricing(t *testing.T) {
	const expression = `tier("base", u("count") * 3)`
	for _, count := range []int{1, 3, dto.MaxImageN} {
		request := billingexpr.RequestInput{ImageCount: &count, Usage: map[string]any{"count": float64(99)}}
		for _, byHash := range []bool{false, true} {
			var cost float64
			var trace billingexpr.TraceResult
			var err error
			if byHash {
				cost, trace, err = billingexpr.RunExprByHashWithRequest(expression, billingexpr.ExprHashString(expression), billingexpr.TokenParams{}, request)
			} else {
				cost, trace, err = billingexpr.RunExprWithRequest(expression, billingexpr.TokenParams{}, request)
			}
			require.NoError(t, err)
			assert.Equal(t, float64(count)*3*1_000_000, cost)
			assert.Equal(t, cost, trace.Cost)
			assert.Equal(t, billingexpr.BillingUnitRequest, trace.BillingUnit)
			assert.Equal(t, "base", trace.MatchedTier)
			require.NotNil(t, trace.ImageCount)
			assert.Equal(t, count, *trace.ImageCount)
			assert.Nil(t, trace.FixedPrice)
			assert.Equal(t, float64(99), request.Usage["count"], "the host fact must not mutate caller usage")
		}
	}
}

func TestSyncImageUsageCountQuotaAndRequestMultiplier(t *testing.T) {
	const expression = `tier("base", u("count") * 3) * (param("fast") == true ? 2 : 1)`
	for _, tc := range []struct {
		name  string
		count int
		group float64
		want  int
	}{
		{"default one image", 1, 1, 3_000_000},
		{"multiple images and group", 3, 1.5, 13_500_000},
		{"free group", 3, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := billingexpr.RequestInput{ImageCount: &tc.count, Body: []byte(`{"fast":true}`)}
			cost, trace, err := billingexpr.RunExprWithRequest(expression, billingexpr.TokenParams{}, request)
			require.NoError(t, err)
			assert.Equal(t, float64(tc.count)*6*1_000_000, cost)
			assert.Equal(t, float64(tc.count)*3*1_000_000, trace.Cost, "tier cost uses the existing scaled fixed-price unit before request multipliers")
			require.Len(t, trace.RequestRules, 1)
			assert.True(t, trace.RequestRules[0].Matched)
			snapshot := &billingexpr.BillingSnapshot{ExprString: expression, ExprHash: billingexpr.ExprHashString(expression), GroupRatio: tc.group, QuotaPerUnit: 500_000, EstimatedTier: "base"}
			result, err := billingexpr.ComputeTieredQuotaWithRequest(snapshot, billingexpr.TokenParams{}, request)
			require.NoError(t, err)
			assert.Equal(t, tc.want, result.ActualQuotaAfterGroup)
			assert.Equal(t, float64(tc.count)*6*500_000, result.ActualQuotaBeforeGroup)
			assert.Equal(t, billingexpr.BillingUnitRequest, result.BillingUnit)
			assert.Equal(t, &tc.count, result.ImageCount)
		})
	}
}

func TestSyncImageUsageRejectsInvalidCounts(t *testing.T) {
	for _, count := range []int{-1, 0, dto.MaxImageN + 1} {
		_, _, err := billingexpr.RunExprWithRequest(`tier("base", u("count") * 3)`, billingexpr.TokenParams{}, billingexpr.RequestInput{ImageCount: &count})
		require.ErrorContains(t, err, "image_count")
	}
}

func TestSyncImageUsageRejectsFixedPricingIncludingUnselectedBranch(t *testing.T) {
	count := 1
	for _, expression := range []string{
		`param("fixed") == true ? tier("fixed", fixed(0.01)) : tier("image", u("count") * 3)`,
		`true ? tier("image", u("count") * 3) : tier("fixed", fixed(0.01))`,
	} {
		t.Run(expression, func(t *testing.T) {
			_, err := billingexpr.CompileFromCache(expression)
			require.NoError(t, err)
			_, _, err = billingexpr.RunExprWithRequest(expression, billingexpr.TokenParams{}, billingexpr.RequestInput{ImageCount: &count})
			require.ErrorContains(t, err, "fixed pricing is not supported for synchronous image usage expressions")
		})
	}
}

func TestSyncImageUsageDoesNotDefaultUnknownUsage(t *testing.T) {
	count := 1
	_, _, err := billingexpr.RunExprWithRequest(`tier("base", u("count") * 3 + u("unknown") * 2)`, billingexpr.TokenParams{}, billingexpr.RequestInput{ImageCount: &count})
	require.Error(t, err)
	_, _, err = billingexpr.RunExpr(`tier("base", u("count") * 3)`, billingexpr.TokenParams{})
	require.Error(t, err, "count without validated image context must remain missing")
}

func TestImageUsageContextPreservesExistingPricingModes(t *testing.T) {
	count := 3
	for _, tc := range []struct {
		name       string
		expression string
		cost       float64
		unit       billingexpr.BillingUnit
		imageCount *int
	}{
		{"token", `tier("tokens", p * 2 + c * 8)`, 2400, billingexpr.BillingUnitToken, nil},
		{"fixed", `tier("fixed", fixed(0.01))`, 10_000, billingexpr.BillingUnitRequest, nil},
		{"image count", `tier("image", fixed(0.04)) * image_count`, 120_000, billingexpr.BillingUnitRequest, &count},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cost, trace, err := billingexpr.RunExprWithRequest(tc.expression, billingexpr.TokenParams{P: 1000, C: 50}, billingexpr.RequestInput{ImageCount: &count})
			require.NoError(t, err)
			assert.Equal(t, tc.cost, cost)
			assert.Equal(t, tc.unit, trace.BillingUnit)
			assert.Equal(t, tc.imageCount, trace.ImageCount)
		})
	}
}

func TestTaskUsageCountWithoutImageContextKeepsDollarOutput(t *testing.T) {
	const expression = `tier("base", u("count") * 3)`
	request := billingexpr.RequestInput{Usage: map[string]any{"count": float64(3)}}
	cost, trace, err := billingexpr.RunExprWithRequest(expression, billingexpr.TokenParams{}, request)
	require.NoError(t, err)
	assert.Equal(t, float64(9), cost)
	assert.Equal(t, cost, trace.Cost)
	assert.Equal(t, billingexpr.BillingUnitToken, trace.BillingUnit)
	assert.Nil(t, trace.ImageCount)
	snapshot := &billingexpr.BillingSnapshot{ExprString: expression, ExprHash: billingexpr.ExprHashString(expression), TaskUsageBilling: true, GroupRatio: 1.5, QuotaPerUnit: 500_000}
	result, err := billingexpr.ComputeTieredQuotaWithRequest(snapshot, billingexpr.TokenParams{}, request)
	require.NoError(t, err)
	assert.Equal(t, float64(4_500_000), result.ActualQuotaBeforeGroup)
	assert.Equal(t, 6_750_000, result.ActualQuotaAfterGroup)
}

func TestDynamicUsageKeyDoesNotEnableSyncImagePricing(t *testing.T) {
	count := 1
	request := billingexpr.RequestInput{ImageCount: &count, Body: []byte(`{"usage_key":"count"}`), Usage: map[string]any{"count": float64(3)}}
	cost, trace, err := billingexpr.RunExprWithRequest(`tier("base", u(param("usage_key")) * 3)`, billingexpr.TokenParams{}, request)
	require.NoError(t, err)
	assert.Equal(t, float64(9), cost)
	assert.Equal(t, billingexpr.BillingUnitToken, trace.BillingUnit)
	assert.Nil(t, trace.ImageCount)
}
