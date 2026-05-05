package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type quotaShareCacheStub struct {
	state         *QuotaShareGroupState
	saved         *QuotaShareGroupState
	keyUsage      *QuotaShareKeyUsage
	totalWeight   int
	localUSD      map[string]float64
	resetLocalUSD []string
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
	if c.localUSD != nil {
		c.localUSD[window] += cost
	}
	return nil
}

func (c *quotaShareCacheStub) GetAndResetLocalUSD(ctx context.Context, groupID int64, window string) (float64, error) {
	if c.localUSD == nil {
		return 0, nil
	}
	value := c.localUSD[window]
	c.localUSD[window] = 0
	return value, nil
}

func (c *quotaShareCacheStub) ResetLocalUSD(ctx context.Context, groupID int64, window string) error {
	c.resetLocalUSD = append(c.resetLocalUSD, window)
	if c.localUSD != nil {
		c.localUSD[window] = 0
	}
	return nil
}

type quotaShareGroupRepoStub struct {
	group           *Group
	savedCalState   *domain.QuotaShareCalibrationState
	savedEst5h      float64
	savedEst7d      float64
	updateCalls     int
	updateStateOnly int
}

func (r *quotaShareGroupRepoStub) GetByIDLite(ctx context.Context, id int64) (*Group, error) {
	if r.group == nil {
		return nil, ErrGroupNotFound
	}
	copyGroup := *r.group
	return &copyGroup, nil
}

func (r *quotaShareGroupRepoStub) UpdateQuotaShareEstimates(ctx context.Context, groupID int64, est5h, est7d float64, calState *domain.QuotaShareCalibrationState) error {
	r.savedEst5h = est5h
	r.savedEst7d = est7d
	r.savedCalState = calState
	r.updateCalls++
	return nil
}

func (r *quotaShareGroupRepoStub) UpdateQuotaShareCalibrationState(ctx context.Context, groupID int64, calState *domain.QuotaShareCalibrationState) error {
	r.savedCalState = calState
	r.updateStateOnly++
	return nil
}

type quotaShareUsageRepoStub struct {
	calls        []windowQuery
	usageByStart map[int64]float64
	err          error
}

type windowQuery struct {
	apiKeyID int64
	start    time.Time
	end      time.Time
}

func (r *quotaShareUsageRepoStub) SumAPIKeyActualCostInWindow(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) (float64, error) {
	r.calls = append(r.calls, windowQuery{apiKeyID: apiKeyID, start: startTime, end: endTime})
	if r.err != nil {
		return 0, r.err
	}
	if r.usageByStart != nil {
		return r.usageByStart[startTime.Unix()], nil
	}
	switch startTime.Unix() {
	case 1719990000:
		return 1.25, nil
	case 1720594800:
		return 4.75, nil
	default:
		return 0.5, nil
	}
}

func (r *quotaShareUsageRepoStub) GetAccountQuotaShareStatsInWindow(ctx context.Context, accountID int64, startTime, endTime time.Time) (*usagestats.AccountStats, error) {
	return &usagestats.AccountStats{}, r.err
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

func TestQuotaShareServiceCheckLimitsUsesMaxOfRedisAndDBFor5h(t *testing.T) {
	now := time.Now().Unix()
	state := &QuotaShareGroupState{
		Window5hStart:       now - int64(time.Hour/time.Second),
		Window5hEnd:         now + int64(time.Hour/time.Second),
		Window7dStart:       now - int64((2*time.Hour)/time.Second),
		Window7dEnd:         now + int64(time.Hour/time.Second),
		Estimated5hLimitUSD: 10,
		Estimated7dLimitUSD: 100,
	}
	group := &Group{ID: 7, SubscriptionType: SubscriptionTypeQuotaShare}
	apiKey := &APIKey{ID: 88, QuotaWeight: 1}

	t.Run("db_exceeds_when_redis_is_lower", func(t *testing.T) {
		cache := &quotaShareCacheStub{
			state:       state,
			keyUsage:    &QuotaShareKeyUsage{Usage5h: 5, Usage7d: 1},
			totalWeight: 1,
		}
		usageRepo := &quotaShareUsageRepoStub{usageByStart: map[int64]float64{
			state.Window5hStart: 10,
			state.Window7dStart: 1,
		}}
		svc := NewQuotaShareService(cache, nil, nil, usageRepo)

		err := svc.CheckLimits(context.Background(), apiKey, group)
		require.ErrorIs(t, err, ErrQuotaShare5hExceeded)
		require.Len(t, usageRepo.calls, 1)
	})

	t.Run("redis_exceeds_when_db_is_lower", func(t *testing.T) {
		cache := &quotaShareCacheStub{
			state:       state,
			keyUsage:    &QuotaShareKeyUsage{Usage5h: 10, Usage7d: 1},
			totalWeight: 1,
		}
		usageRepo := &quotaShareUsageRepoStub{usageByStart: map[int64]float64{
			state.Window5hStart: 5,
			state.Window7dStart: 1,
		}}
		svc := NewQuotaShareService(cache, nil, nil, usageRepo)

		err := svc.CheckLimits(context.Background(), apiKey, group)
		require.ErrorIs(t, err, ErrQuotaShare5hExceeded)
		require.Len(t, usageRepo.calls, 1)
	})

	t.Run("both_below_limit_allows", func(t *testing.T) {
		cache := &quotaShareCacheStub{
			state:       state,
			keyUsage:    &QuotaShareKeyUsage{Usage5h: 4, Usage7d: 1},
			totalWeight: 1,
		}
		usageRepo := &quotaShareUsageRepoStub{usageByStart: map[int64]float64{
			state.Window5hStart: 6,
			state.Window7dStart: 1,
		}}
		svc := NewQuotaShareService(cache, nil, nil, usageRepo)

		err := svc.CheckLimits(context.Background(), apiKey, group)
		require.NoError(t, err)
		require.Len(t, usageRepo.calls, 2)
	})
}

func TestQuotaShareServiceCheckLimitsDBFailureFallsBackToRedis(t *testing.T) {
	now := time.Now().Unix()
	state := &QuotaShareGroupState{
		Window5hStart:       now - int64(time.Hour/time.Second),
		Window5hEnd:         now + int64(time.Hour/time.Second),
		Estimated5hLimitUSD: 10,
	}
	group := &Group{ID: 7, SubscriptionType: SubscriptionTypeQuotaShare}
	apiKey := &APIKey{ID: 88, QuotaWeight: 1}
	dbErr := errors.New("db temporarily unavailable")

	t.Run("redis_below_allows", func(t *testing.T) {
		cache := &quotaShareCacheStub{
			state:       state,
			keyUsage:    &QuotaShareKeyUsage{Usage5h: 9},
			totalWeight: 1,
		}
		svc := NewQuotaShareService(cache, nil, nil, &quotaShareUsageRepoStub{err: dbErr})

		err := svc.CheckLimits(context.Background(), apiKey, group)
		require.NoError(t, err)
	})

	t.Run("redis_exceeds_blocks", func(t *testing.T) {
		cache := &quotaShareCacheStub{
			state:       state,
			keyUsage:    &QuotaShareKeyUsage{Usage5h: 10},
			totalWeight: 1,
		}
		svc := NewQuotaShareService(cache, nil, nil, &quotaShareUsageRepoStub{err: dbErr})

		err := svc.CheckLimits(context.Background(), apiKey, group)
		require.ErrorIs(t, err, ErrQuotaShare5hExceeded)
	})
}

func TestQuotaShareServiceCheckLimitsUsesMaxOfRedisAndDBFor7d(t *testing.T) {
	now := time.Now().Unix()
	state := &QuotaShareGroupState{
		Window5hStart:       now - int64(time.Hour/time.Second),
		Window5hEnd:         now + int64(time.Hour/time.Second),
		Window7dStart:       now - int64((2*time.Hour)/time.Second),
		Window7dEnd:         now + int64(time.Hour/time.Second),
		Estimated5hLimitUSD: 100,
		Estimated7dLimitUSD: 10,
	}
	group := &Group{ID: 7, SubscriptionType: SubscriptionTypeQuotaShare}
	apiKey := &APIKey{ID: 88, QuotaWeight: 1}
	cache := &quotaShareCacheStub{
		state:       state,
		keyUsage:    &QuotaShareKeyUsage{Usage5h: 1, Usage7d: 5},
		totalWeight: 1,
	}
	usageRepo := &quotaShareUsageRepoStub{usageByStart: map[int64]float64{
		state.Window5hStart: 1,
		state.Window7dStart: 10,
	}}
	svc := NewQuotaShareService(cache, nil, nil, usageRepo)

	err := svc.CheckLimits(context.Background(), apiKey, group)
	require.ErrorIs(t, err, ErrQuotaShare7dExceeded)
	require.Len(t, usageRepo.calls, 2)
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

func TestQuotaShareServiceTryCalibrateCreatesPendingSuggestion(t *testing.T) {
	sampleStartedAt := time.Now().Add(-time.Hour)
	cache := &quotaShareCacheStub{
		localUSD: map[string]float64{"5h": 2},
	}
	groupRepo := &quotaShareGroupRepoStub{
		group: &Group{
			ID:                  11,
			Name:                "quota-share",
			SubscriptionType:    SubscriptionTypeQuotaShare,
			Estimated5hLimitUSD: 10,
		},
	}
	svc := NewQuotaShareService(cache, groupRepo, nil, nil)
	group := &Group{
		ID:                  11,
		Name:                "quota-share",
		SubscriptionType:    SubscriptionTypeQuotaShare,
		Estimated5hLimitUSD: 10,
		CalibrationState: &domain.QuotaShareCalibrationState{
			Window5h: &domain.QuotaShareCalibrationWindowState{
				CurrentEstimateUSD: 10,
				LastUpstreamPct:    10,
				SampleStartedAt:    &sampleStartedAt,
				EMAAlpha:           0.3,
			},
		},
	}
	primaryPct := 20.0
	snapshot := &OpenAICodexUsageSnapshot{
		SecondaryUsedPercent: &primaryPct,
	}

	svc.TryCalibrate(context.Background(), group, snapshot)

	require.NotNil(t, groupRepo.savedCalState)
	require.NotNil(t, groupRepo.savedCalState.Window5h)
	require.NotNil(t, groupRepo.savedCalState.Window5h.PendingSuggestion)
	require.Equal(t, domain.QuotaShareCalibrationSuggestionStatusPending, groupRepo.savedCalState.Window5h.PendingSuggestion.Status)
	require.InDelta(t, 13.0, groupRepo.savedCalState.Window5h.PendingSuggestion.SuggestedEstimateUSD, 1e-9)
	require.InDelta(t, 10.0, group.Estimated5hLimitUSD, 1e-9)
	require.Equal(t, 1, groupRepo.updateStateOnly)
}

func TestQuotaShareServiceTryCalibrateInitializesBaselineWithoutSuggestion(t *testing.T) {
	cache := &quotaShareCacheStub{
		localUSD: map[string]float64{"5h": 9, "7d": 12},
	}
	groupRepo := &quotaShareGroupRepoStub{}
	svc := NewQuotaShareService(cache, groupRepo, nil, nil)
	group := &Group{
		ID:                  11,
		Name:                "quota-share",
		SubscriptionType:    SubscriptionTypeQuotaShare,
		Estimated5hLimitUSD: 10,
		Estimated7dLimitUSD: 70,
	}
	pct5h := 8.0
	pct7d := 21.0
	reset5h := 3600
	reset7d := 86400
	window5h := 300
	window7d := 10080
	snapshot := &OpenAICodexUsageSnapshot{
		PrimaryUsedPercent:         &pct5h,
		PrimaryResetAfterSeconds:   &reset5h,
		PrimaryWindowMinutes:       &window5h,
		SecondaryUsedPercent:       &pct7d,
		SecondaryResetAfterSeconds: &reset7d,
		SecondaryWindowMinutes:     &window7d,
	}

	svc.TryCalibrate(context.Background(), group, snapshot)

	require.NotNil(t, groupRepo.savedCalState)
	require.NotNil(t, groupRepo.savedCalState.Window5h)
	require.NotNil(t, groupRepo.savedCalState.Window7d)
	require.Nil(t, groupRepo.savedCalState.Window5h.PendingSuggestion)
	require.Nil(t, groupRepo.savedCalState.Window7d.PendingSuggestion)
	require.NotNil(t, groupRepo.savedCalState.Window5h.SampleStartedAt)
	require.NotNil(t, groupRepo.savedCalState.Window7d.SampleStartedAt)
	require.InDelta(t, pct5h, groupRepo.savedCalState.Window5h.LastUpstreamPct, 1e-9)
	require.InDelta(t, pct7d, groupRepo.savedCalState.Window7d.LastUpstreamPct, 1e-9)
	require.Equal(t, []string{"5h", "7d"}, cache.resetLocalUSD)
	require.Zero(t, cache.localUSD["5h"])
	require.Zero(t, cache.localUSD["7d"])
	require.Equal(t, 1, groupRepo.updateStateOnly)
}

func TestQuotaShareServiceTryCalibrateSkipsSmallPositiveDelta(t *testing.T) {
	sampleStartedAt := time.Now().Add(-time.Hour)
	cache := &quotaShareCacheStub{localUSD: map[string]float64{"5h": 2}}
	groupRepo := &quotaShareGroupRepoStub{}
	svc := NewQuotaShareService(cache, groupRepo, nil, nil)
	group := &Group{
		ID:                  11,
		Name:                "quota-share",
		SubscriptionType:    SubscriptionTypeQuotaShare,
		Estimated5hLimitUSD: 10,
		CalibrationState: &domain.QuotaShareCalibrationState{
			Window5h: &domain.QuotaShareCalibrationWindowState{
				CurrentEstimateUSD: 10,
				LastUpstreamPct:    10,
				SampleStartedAt:    &sampleStartedAt,
				EMAAlpha:           0.3,
			},
		},
	}
	pct := 12.9
	snapshot := &OpenAICodexUsageSnapshot{SecondaryUsedPercent: &pct}

	svc.TryCalibrate(context.Background(), group, snapshot)

	require.Nil(t, groupRepo.savedCalState)
	require.InDelta(t, 2.0, cache.localUSD["5h"], 1e-9)
}

func TestQuotaShareServiceTryCalibrateResetsBaselineOnWindowChangeAndRollback(t *testing.T) {
	sampleStartedAt := time.Now().Add(-time.Hour)
	now := time.Now()
	cache := &quotaShareCacheStub{localUSD: map[string]float64{"5h": 2, "7d": 3}}
	groupRepo := &quotaShareGroupRepoStub{}
	svc := NewQuotaShareService(cache, groupRepo, nil, nil)
	group := &Group{
		ID:                  11,
		Name:                "quota-share",
		SubscriptionType:    SubscriptionTypeQuotaShare,
		Estimated5hLimitUSD: 10,
		Estimated7dLimitUSD: 70,
		CalibrationState: &domain.QuotaShareCalibrationState{
			Window5h: &domain.QuotaShareCalibrationWindowState{
				CurrentEstimateUSD: 10,
				LastUpstreamPct:    40,
				WindowStart:        now.Add(-5 * time.Hour).Unix(),
				WindowEnd:          now.Add(time.Hour).Unix(),
				SampleStartedAt:    &sampleStartedAt,
				PendingSuggestion: &domain.QuotaShareCalibrationSuggestion{
					Window: "5h",
					Status: domain.QuotaShareCalibrationSuggestionStatusPending,
				},
				EMAAlpha: 0.3,
			},
			Window7d: &domain.QuotaShareCalibrationWindowState{
				CurrentEstimateUSD: 70,
				LastUpstreamPct:    80,
				SampleStartedAt:    &sampleStartedAt,
				PendingSuggestion: &domain.QuotaShareCalibrationSuggestion{
					Window: "7d",
					Status: domain.QuotaShareCalibrationSuggestionStatusPending,
				},
				EMAAlpha: 0.3,
			},
		},
	}
	pct5h := 45.0
	pct7d := 20.0
	reset5h := 3 * 3600
	window5h := 300
	snapshot := &OpenAICodexUsageSnapshot{
		SecondaryUsedPercent:       &pct5h,
		SecondaryResetAfterSeconds: &reset5h,
		SecondaryWindowMinutes:     &window5h,
		PrimaryUsedPercent:         &pct7d,
	}

	svc.TryCalibrate(context.Background(), group, snapshot)

	require.NotNil(t, groupRepo.savedCalState)
	require.Nil(t, groupRepo.savedCalState.Window5h.PendingSuggestion)
	require.Nil(t, groupRepo.savedCalState.Window7d.PendingSuggestion)
	require.InDelta(t, pct5h, groupRepo.savedCalState.Window5h.LastUpstreamPct, 1e-9)
	require.InDelta(t, pct7d, groupRepo.savedCalState.Window7d.LastUpstreamPct, 1e-9)
	require.ElementsMatch(t, []string{"5h", "7d"}, cache.resetLocalUSD)
}

func TestQuotaShareServiceTryCalibrateRejectsLargeDeviation(t *testing.T) {
	sampleStartedAt := time.Now().Add(-time.Hour)
	cache := &quotaShareCacheStub{
		localUSD: map[string]float64{"5h": 100},
	}
	groupRepo := &quotaShareGroupRepoStub{}
	svc := NewQuotaShareService(cache, groupRepo, nil, nil)
	group := &Group{
		ID:                  11,
		Name:                "quota-share",
		SubscriptionType:    SubscriptionTypeQuotaShare,
		Estimated5hLimitUSD: 10,
		CalibrationState: &domain.QuotaShareCalibrationState{
			Window5h: &domain.QuotaShareCalibrationWindowState{
				CurrentEstimateUSD: 10,
				LastUpstreamPct:    10,
				SampleStartedAt:    &sampleStartedAt,
				EMAAlpha:           0.3,
			},
		},
	}
	pct := 20.0
	snapshot := &OpenAICodexUsageSnapshot{SecondaryUsedPercent: &pct}

	svc.TryCalibrate(context.Background(), group, snapshot)

	require.NotNil(t, groupRepo.savedCalState)
	require.NotNil(t, groupRepo.savedCalState.Window5h.PendingSuggestion)
	require.Equal(t, domain.QuotaShareCalibrationSuggestionStatusRejected, groupRepo.savedCalState.Window5h.PendingSuggestion.Status)
	require.Equal(t, 0, groupRepo.updateCalls)
	require.Equal(t, 1, groupRepo.updateStateOnly)
}

func TestQuotaShareServiceApplyAndDiscardCalibrationSuggestion(t *testing.T) {
	now := time.Now()
	pending := &domain.QuotaShareCalibrationSuggestion{
		Window:               "5h",
		Status:               domain.QuotaShareCalibrationSuggestionStatusPending,
		SuggestedEstimateUSD: 18,
		CurrentEstimateUSD:   10,
		CalculatedAt:         &now,
	}
	group := &Group{
		ID:                  21,
		Name:                "quota-share",
		SubscriptionType:    SubscriptionTypeQuotaShare,
		Estimated5hLimitUSD: 10,
		Estimated7dLimitUSD: 30,
		CalibrationState: &domain.QuotaShareCalibrationState{
			Window5h: &domain.QuotaShareCalibrationWindowState{
				CurrentEstimateUSD: 10,
				LastUpstreamPct:    10,
				PendingSuggestion:  pending,
			},
			Window7d: &domain.QuotaShareCalibrationWindowState{
				CurrentEstimateUSD: 30,
				LastUpstreamPct:    20,
			},
		},
	}
	groupRepo := &quotaShareGroupRepoStub{group: group}
	svc := NewQuotaShareService(&quotaShareCacheStub{}, groupRepo, nil, nil)

	applied, err := svc.ApplyCalibrationSuggestion(context.Background(), group.ID, "5h")
	require.NoError(t, err)
	require.NotNil(t, applied)
	require.InDelta(t, 18.0, groupRepo.savedEst5h, 1e-9)
	require.InDelta(t, 30.0, groupRepo.savedEst7d, 1e-9)
	require.Equal(t, domain.QuotaShareCalibrationSuggestionStatusApplied, groupRepo.savedCalState.Window5h.PendingSuggestion.Status)

	discardPending := &domain.QuotaShareCalibrationSuggestion{
		Window:               "7d",
		Status:               domain.QuotaShareCalibrationSuggestionStatusPending,
		SuggestedEstimateUSD: 50,
		CurrentEstimateUSD:   30,
	}
	groupRepo.group = &Group{
		ID:               22,
		Name:             "quota-share-2",
		SubscriptionType: SubscriptionTypeQuotaShare,
		CalibrationState: &domain.QuotaShareCalibrationState{
			Window7d: &domain.QuotaShareCalibrationWindowState{
				CurrentEstimateUSD: 30,
				PendingSuggestion:  discardPending,
			},
		},
	}
	groupRepo.savedCalState = nil

	discarded, err := svc.DiscardCalibrationSuggestion(context.Background(), 22, "7d", "")
	require.NoError(t, err)
	require.NotNil(t, discarded)
	require.Equal(t, domain.QuotaShareCalibrationSuggestionStatusDiscarded, groupRepo.savedCalState.Window7d.PendingSuggestion.Status)
	require.NotEmpty(t, groupRepo.savedCalState.Window7d.PendingSuggestion.Reason)
}
