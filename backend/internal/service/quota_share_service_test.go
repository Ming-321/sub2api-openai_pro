package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type quotaShareCacheStub struct {
	state       *QuotaShareGroupState
	saved       *QuotaShareGroupState
	keyUsage    *QuotaShareKeyUsage
	totalWeight int
}

func (c *quotaShareCacheStub) GetGroupState(ctx context.Context, groupID int64) (*QuotaShareGroupState, error) {
	return c.state, nil
}

func (c *quotaShareCacheStub) SetGroupState(ctx context.Context, groupID int64, state *QuotaShareGroupState) error {
	if state == nil {
		c.saved = nil
		return nil
	}
	copyState := *state
	c.saved = &copyState
	c.state = &copyState
	return nil
}

func (c *quotaShareCacheStub) GetKeyUsage(ctx context.Context, groupID, keyID int64) (*QuotaShareKeyUsage, error) {
	if c.keyUsage == nil {
		return nil, nil
	}
	copyUsage := *c.keyUsage
	return &copyUsage, nil
}

func (c *quotaShareCacheStub) IncrKeyUsage(ctx context.Context, groupID, keyID int64, cost float64, window5hEnd, window7dEnd int64) (*QuotaShareKeyUsage, error) {
	return c.GetKeyUsage(ctx, groupID, keyID)
}

func (c *quotaShareCacheStub) GetTotalWeight(ctx context.Context, groupID int64) (int, error) {
	return c.totalWeight, nil
}

func (c *quotaShareCacheStub) SetTotalWeight(ctx context.Context, groupID int64, total int) error {
	c.totalWeight = total
	return nil
}

func (c *quotaShareCacheStub) DeleteTotalWeight(ctx context.Context, groupID int64) error {
	c.totalWeight = 0
	return nil
}

func (c *quotaShareCacheStub) IncrLocalUSD(ctx context.Context, groupID int64, window string, cost float64) error {
	return nil
}

func (c *quotaShareCacheStub) GetAndResetLocalUSD(ctx context.Context, groupID int64, window string) (float64, error) {
	return 0, nil
}

type quotaShareUsageRepoStub struct {
	calls []windowQuery
}

type windowQuery struct {
	apiKeyID int64
	start    time.Time
	end      time.Time
}

func (r *quotaShareUsageRepoStub) SumAPIKeyActualCostInWindow(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) (float64, error) {
	r.calls = append(r.calls, windowQuery{apiKeyID: apiKeyID, start: startTime, end: endTime})
	switch startTime.Unix() {
	case 1719990000:
		return 1.25, nil
	case 1720594800:
		return 4.75, nil
	default:
		return 0.5, nil
	}
}

func TestQuotaShareServiceGetKeyUsageUsesDBWindowTotals(t *testing.T) {
	cache := &quotaShareCacheStub{
		state: &QuotaShareGroupState{
			Window5hStart: 1719990000,
			Window5hEnd:   1720008000,
			Window7dStart: 1720594800,
			Window7dEnd:   1721203200,
		},
	}
	usageRepo := &quotaShareUsageRepoStub{}
	svc := NewQuotaShareService(cache, nil, nil, usageRepo)

	apiKey := &APIKey{ID: 88}
	group := &Group{ID: 7, SubscriptionType: SubscriptionTypeQuotaShare}

	usage, err := svc.GetKeyUsage(context.Background(), apiKey, group)
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.InDelta(t, 1.25, usage.Usage5h, 1e-9)
	require.InDelta(t, 4.75, usage.Usage7d, 1e-9)
	require.Len(t, usageRepo.calls, 2)
	require.Equal(t, int64(88), usageRepo.calls[0].apiKeyID)
	require.Equal(t, time.Unix(1719990000, 0), usageRepo.calls[0].start)
	require.Equal(t, time.Unix(1720008000, 0), usageRepo.calls[0].end)
	require.Equal(t, time.Unix(1720594800, 0), usageRepo.calls[1].start)
	require.Equal(t, time.Unix(1721203200, 0), usageRepo.calls[1].end)
}

func TestQuotaShareServiceUpdateGlobalWindowKeepsSmallDriftWindow(t *testing.T) {
	now := time.Now()
	existingEnd := now.Add(5*time.Hour + 10*time.Second).Unix()
	existingStart := existingEnd - int64((5*time.Hour)/time.Second)
	cache := &quotaShareCacheStub{
		state: &QuotaShareGroupState{
			Window5hStart: existingStart,
			Window5hEnd:   existingEnd,
			Upstream5hPct: 12.5,
		},
	}
	svc := NewQuotaShareService(cache, nil, nil, nil)
	group := &Group{
		ID:                  9,
		SubscriptionType:    SubscriptionTypeQuotaShare,
		Estimated5hLimitUSD: 100,
		Estimated7dLimitUSD: 200,
	}
	pct := 17.5
	resetAfter := 5 * 60 * 60
	windowMinutes := 300
	snapshot := &OpenAICodexUsageSnapshot{
		PrimaryUsedPercent:       &pct,
		PrimaryResetAfterSeconds: &resetAfter,
		PrimaryWindowMinutes:     &windowMinutes,
	}

	svc.UpdateGlobalWindow(context.Background(), group, snapshot)

	require.NotNil(t, cache.saved)
	require.Equal(t, existingStart, cache.saved.Window5hStart)
	require.Equal(t, existingEnd, cache.saved.Window5hEnd)
	require.InDelta(t, pct, cache.saved.Upstream5hPct, 1e-9)
}
