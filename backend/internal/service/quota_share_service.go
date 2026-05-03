package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"golang.org/x/sync/singleflight"
)

var (
	ErrQuotaShare5hExceeded = errors.New("quota_share: 5h window limit exceeded")
	ErrQuotaShare7dExceeded = errors.New("quota_share: 7d window limit exceeded")
)

const quotaShareWindowEndDriftTolerance = 2 * time.Minute

func IsQuotaShareExceededError(err error) bool {
	return errors.Is(err, ErrQuotaShare5hExceeded) || errors.Is(err, ErrQuotaShare7dExceeded)
}

func quotaShareWindowEndChanged(existingEnd, newEnd int64) bool {
	if existingEnd == newEnd {
		return false
	}
	if existingEnd == 0 || newEnd == 0 {
		return true
	}
	diff := existingEnd - newEnd
	if diff < 0 {
		diff = -diff
	}
	return diff > int64(quotaShareWindowEndDriftTolerance/time.Second)
}

func normalizeQuotaShareWindow(existingStart, existingEnd int64, resetAfterSeconds int, windowMinutes *int) (start, end int64) {
	if resetAfterSeconds <= 0 {
		return existingStart, existingEnd
	}
	now := time.Now()
	newEnd := now.Add(time.Duration(resetAfterSeconds) * time.Second).Unix()
	if existingEnd > 0 && !quotaShareWindowEndChanged(existingEnd, newEnd) {
		if existingStart > 0 {
			return existingStart, existingEnd
		}
		if windowMinutes != nil && *windowMinutes > 0 {
			windowDur := time.Duration(*windowMinutes) * time.Minute
			return existingEnd - int64(windowDur/time.Second), existingEnd
		}
		return existingStart, existingEnd
	}
	if windowMinutes == nil || *windowMinutes <= 0 {
		return existingStart, newEnd
	}
	windowDur := time.Duration(*windowMinutes) * time.Minute
	return newEnd - int64(windowDur/time.Second), newEnd
}

// QuotaShareCache is the interface for the quota_share-specific Redis cache layer.
type QuotaShareCache interface {
	// GetGroupState returns the cached global window state for a quota_share group.
	GetGroupState(ctx context.Context, groupID int64) (*QuotaShareGroupState, error)
	// SetGroupState persists the global window state.
	SetGroupState(ctx context.Context, groupID int64, state *QuotaShareGroupState) error
	// GetKeyUsage returns the cached usage for a key within the current windows.
	GetKeyUsage(ctx context.Context, groupID, keyID int64) (*QuotaShareKeyUsage, error)
	// IncrKeyUsage atomically increments key usage, resetting if the window has changed.
	IncrKeyUsage(ctx context.Context, groupID, keyID int64, cost float64, window5hEnd, window7dEnd int64) (*QuotaShareKeyUsage, error)
	// GetTotalWeight returns the cached sum of all key weights for a group.
	GetTotalWeight(ctx context.Context, groupID int64) (int, error)
	// SetTotalWeight caches the total weight for a group.
	SetTotalWeight(ctx context.Context, groupID int64, total int) error
	// DeleteTotalWeight removes the cached total weight, forcing a reload on next access.
	DeleteTotalWeight(ctx context.Context, groupID int64) error
	// IncrLocalUSD atomically increments the local USD spent for a specific window since last calibration.
	IncrLocalUSD(ctx context.Context, groupID int64, window string, cost float64) error
	// GetAndResetLocalUSD reads and resets the local USD counter for a specific window.
	GetAndResetLocalUSD(ctx context.Context, groupID int64, window string) (float64, error)
}

// QuotaShareGroupState holds the global window state stored in Redis.
// Estimated limits are co-located here so CheckLimits needs only one Redis read.
type QuotaShareGroupState struct {
	Window5hStart int64   `json:"w5s"` // unix seconds
	Window5hEnd   int64   `json:"w5e"`
	Window7dStart int64   `json:"w7s"`
	Window7dEnd   int64   `json:"w7e"`
	Upstream5hPct float64 `json:"u5p"`
	Upstream7dPct float64 `json:"u7p"`
	UpdatedAt     int64   `json:"uat"` // unix seconds

	Estimated5hLimitUSD float64 `json:"e5"`
	Estimated7dLimitUSD float64 `json:"e7"`
}

// QuotaShareKeyUsage holds per-key usage within the current windows.
type QuotaShareKeyUsage struct {
	Usage5h float64 `json:"u5"`
	Usage7d float64 `json:"u7"`
}

// QuotaShareGroupRepository is the minimal repository interface used by QuotaShareService.
type QuotaShareGroupRepository interface {
	GetByIDLite(ctx context.Context, id int64) (*Group, error)
	UpdateQuotaShareEstimates(ctx context.Context, groupID int64, est5h, est7d float64, calState *domain.QuotaShareCalibrationState) error
}

// QuotaShareKeyRepository provides weight information.
type QuotaShareKeyRepository interface {
	GetTotalQuotaWeight(ctx context.Context, groupID int64) (int, error)
}

// QuotaShareUsageRepository provides DB-backed actual_cost aggregation.
type QuotaShareUsageRepository interface {
	SumAPIKeyActualCostInWindow(ctx context.Context, apiKeyID int64, startTime, endTime time.Time) (float64, error)
}

type QuotaShareService struct {
	cache          QuotaShareCache
	groupRepo      QuotaShareGroupRepository
	keyRepo        QuotaShareKeyRepository
	usageRepo      QuotaShareUsageRepository
	sfCal          singleflight.Group
	mu             sync.Mutex // protects calibration writes
	accountGroupID sync.Map   // accountID(int64) → groupID(int64) lazy cache
}

func NewQuotaShareService(cache QuotaShareCache, groupRepo QuotaShareGroupRepository, keyRepo QuotaShareKeyRepository, usageRepo QuotaShareUsageRepository) *QuotaShareService {
	return &QuotaShareService{
		cache:     cache,
		groupRepo: groupRepo,
		keyRepo:   keyRepo,
		usageRepo: usageRepo,
	}
}

// CheckLimits verifies the API key hasn't exceeded its quota_share allocation.
// Returns nil if the request should proceed, or an error if limits are exceeded.
func (s *QuotaShareService) CheckLimits(ctx context.Context, apiKey *APIKey, group *Group) error {
	if group == nil || !group.IsQuotaShareType() {
		return nil
	}

	state, err := s.cache.GetGroupState(ctx, group.ID)
	if err != nil {
		slog.Warn("quota_share: failed to get group state, allowing request", "group_id", group.ID, "error", err)
		return nil
	}

	now := time.Now().Unix()

	// No window initialized yet — allow all requests (D4)
	if state == nil || (state.Window5hEnd == 0 && state.Window7dEnd == 0) {
		return nil
	}

	window5hActive := state.Window5hEnd > 0 && now <= state.Window5hEnd
	window7dActive := state.Window7dEnd > 0 && now <= state.Window7dEnd

	// Both windows absent or expired — allow (new window will be established from next response)
	if !window5hActive && !window7dActive {
		return nil
	}

	redisUsage, err := s.cache.GetKeyUsage(ctx, group.ID, apiKey.ID)
	if err != nil {
		slog.Warn("quota_share: failed to get key usage, allowing request", "key_id", apiKey.ID, "error", err)
		return nil
	}
	if redisUsage == nil {
		redisUsage = &QuotaShareKeyUsage{}
	}

	totalWeight, err := s.getTotalWeight(ctx, group.ID)
	if err != nil || totalWeight <= 0 {
		slog.Warn("quota_share: failed to get total weight, allowing request", "group_id", group.ID, "error", err)
		return nil
	}

	weight := apiKey.QuotaWeight
	if weight <= 0 {
		weight = 1
	}
	ratio := float64(weight) / float64(totalWeight)

	// Use estimates from Redis state (co-located with windows), not from auth cache group
	est5h := state.Estimated5hLimitUSD
	est7d := state.Estimated7dLimitUSD

	// Check 5h window
	if window5hActive && est5h > 0 {
		limit5h := est5h * ratio
		redis5h, db5h, effective5h, source5h := s.effectiveKeyWindowUsage(ctx, apiKey.ID, state.Window5hStart, state.Window5hEnd, redisUsage.Usage5h)
		if effective5h >= limit5h {
			slog.Info("quota_share: 5h window limit exceeded",
				"api_key_id", apiKey.ID,
				"group_id", group.ID,
				"redis_usage", redis5h,
				"db_usage", db5h,
				"effective_usage", effective5h,
				"limit", limit5h,
				"source", source5h,
			)
			return ErrQuotaShare5hExceeded
		}
	}

	// Check 7d window
	if window7dActive && est7d > 0 {
		limit7d := est7d * ratio
		redis7d, db7d, effective7d, source7d := s.effectiveKeyWindowUsage(ctx, apiKey.ID, state.Window7dStart, state.Window7dEnd, redisUsage.Usage7d)
		if effective7d >= limit7d {
			slog.Info("quota_share: 7d window limit exceeded",
				"api_key_id", apiKey.ID,
				"group_id", group.ID,
				"redis_usage", redis7d,
				"db_usage", db7d,
				"effective_usage", effective7d,
				"limit", limit7d,
				"source", source7d,
			)
			return ErrQuotaShare7dExceeded
		}
	}

	return nil
}

func (s *QuotaShareService) effectiveKeyWindowUsage(ctx context.Context, apiKeyID int64, windowStart, windowEnd int64, redisUsage float64) (redisValue, dbValue, effective float64, source string) {
	redisValue = redisUsage
	dbValue = 0
	effective = redisUsage
	source = "redis"
	if s.usageRepo == nil {
		return redisValue, dbValue, effective, source
	}

	dbUsage, err := s.sumKeyWindowActualCost(ctx, apiKeyID, windowStart, windowEnd)
	if err != nil {
		slog.Warn("quota_share: failed to get DB-backed key usage, falling back to Redis",
			"api_key_id", apiKeyID,
			"window_start", windowStart,
			"window_end", windowEnd,
			"redis_usage", redisUsage,
			"error", err,
		)
		return redisValue, dbValue, effective, source
	}

	dbValue = dbUsage
	if dbUsage > redisUsage {
		effective = dbUsage
		source = "db"
		return redisValue, dbValue, effective, source
	}
	if dbUsage == redisUsage {
		source = "redis_db_equal"
	}
	return redisValue, dbValue, effective, source
}

// RecordUsage records the cost of a completed request against the key's quota_share usage.
func (s *QuotaShareService) RecordUsage(ctx context.Context, apiKey *APIKey, group *Group, cost float64) {
	if group == nil || cost <= 0 {
		return
	}

	state, err := s.cache.GetGroupState(ctx, group.ID)
	if err != nil || state == nil {
		slog.Warn("quota_share: failed to get group state for usage recording", "group_id", group.ID, "error", err)
		return
	}

	_, err = s.cache.IncrKeyUsage(ctx, group.ID, apiKey.ID, cost, state.Window5hEnd, state.Window7dEnd)
	if err != nil {
		slog.Error("quota_share: failed to increment key usage", "key_id", apiKey.ID, "error", err)
	}

	if err := s.cache.IncrLocalUSD(ctx, group.ID, "5h", cost); err != nil {
		slog.Warn("quota_share: failed to increment local USD counter (5h)", "group_id", group.ID, "error", err)
	}
	if err := s.cache.IncrLocalUSD(ctx, group.ID, "7d", cost); err != nil {
		slog.Warn("quota_share: failed to increment local USD counter (7d)", "group_id", group.ID, "error", err)
	}
}

// UpdateGlobalWindow updates the group's global window state based on upstream codex headers.
func (s *QuotaShareService) UpdateGlobalWindow(ctx context.Context, group *Group, snapshot *OpenAICodexUsageSnapshot) {
	if group == nil || snapshot == nil || !group.IsQuotaShareType() {
		return
	}

	normalized := snapshot.Normalize()
	if normalized == nil {
		return
	}

	// Read existing state and apply incremental updates so a partial snapshot
	// cannot wipe the other window.
	existing, _ := s.cache.GetGroupState(ctx, group.ID)

	now := time.Now()
	state := &QuotaShareGroupState{}
	if existing != nil {
		*state = *existing
	}
	state.UpdatedAt = now.Unix()

	// Override with group DB values if available (e.g. from GetByIDLite)
	if group.Estimated5hLimitUSD > 0 {
		state.Estimated5hLimitUSD = group.Estimated5hLimitUSD
	}
	if group.Estimated7dLimitUSD > 0 {
		state.Estimated7dLimitUSD = group.Estimated7dLimitUSD
	}

	if normalized.Reset5hSeconds != nil && normalized.Used5hPercent != nil {
		if start, end := normalizeQuotaShareWindow(state.Window5hStart, state.Window5hEnd, *normalized.Reset5hSeconds, normalized.Window5hMinutes); end > 0 {
			state.Window5hStart = start
			state.Window5hEnd = end
		}
		state.Upstream5hPct = *normalized.Used5hPercent
	}

	if normalized.Reset7dSeconds != nil && normalized.Used7dPercent != nil {
		if start, end := normalizeQuotaShareWindow(state.Window7dStart, state.Window7dEnd, *normalized.Reset7dSeconds, normalized.Window7dMinutes); end > 0 {
			state.Window7dStart = start
			state.Window7dEnd = end
		}
		state.Upstream7dPct = *normalized.Used7dPercent
	}

	if err := s.cache.SetGroupState(ctx, group.ID, state); err != nil {
		slog.Error("quota_share: failed to update global window", "group_id", group.ID, "error", err)
	}
}

// TryCalibrate attempts to calibrate the estimated limits using upstream percentage changes.
// Uses singleflight to prevent concurrent calibrations for the same group.
func (s *QuotaShareService) TryCalibrate(ctx context.Context, group *Group, snapshot *OpenAICodexUsageSnapshot) {
	if group == nil || snapshot == nil || !group.IsQuotaShareType() {
		return
	}

	normalized := snapshot.Normalize()
	if normalized == nil {
		return
	}

	calState := group.CalibrationState
	if calState == nil {
		calState = &domain.QuotaShareCalibrationState{}
	}

	// Initialize window calibration state from current upstream data if needed
	if calState.Window5h == nil && normalized.Used5hPercent != nil {
		calState.Window5h = &domain.QuotaShareCalibrationWindowState{
			CurrentEstimateUSD: group.Estimated5hLimitUSD,
			LastUpstreamPct:    *normalized.Used5hPercent,
			EMAAlpha:           calibrationDefaultAlpha,
		}
	}
	if calState.Window7d == nil && normalized.Used7dPercent != nil {
		calState.Window7d = &domain.QuotaShareCalibrationWindowState{
			CurrentEstimateUSD: group.Estimated7dLimitUSD,
			LastUpstreamPct:    *normalized.Used7dPercent,
			EMAAlpha:           calibrationDefaultAlpha,
		}
	}

	// Singleflight: only one calibration per group at a time (Trap 13)
	sfKey := fmt.Sprintf("calibrate:%d", group.ID)
	_, _, _ = s.sfCal.Do(sfKey, func() (interface{}, error) {
		s.doCalibrate(ctx, group, normalized, calState)
		return nil, nil
	})
}

const (
	calibrationMinInterval  = 30 * time.Minute
	calibrationMinDeltaPct  = 3.0
	calibrationDefaultAlpha = 0.3
	calibrationMaxDeviation = 3.0 // skip if E_new / E_old > this
)

func (s *QuotaShareService) doCalibrate(ctx context.Context, group *Group, norm *NormalizedCodexLimits, calState *domain.QuotaShareCalibrationState) {
	now := time.Now()
	updated := false

	// Calibrate 5h window (uses its own independent localUSD counter)
	if norm.Used5hPercent != nil && calState.Window5h != nil {
		w := calState.Window5h
		if s.shouldCalibrate(now, w.LastCalibrationAt, *norm.Used5hPercent, w.LastUpstreamPct) {
			localUSD5h, _ := s.cache.GetAndResetLocalUSD(ctx, group.ID, "5h")
			if newEst := s.computeCalibration(w, *norm.Used5hPercent, group.Estimated5hLimitUSD, localUSD5h); newEst > 0 {
				group.Estimated5hLimitUSD = newEst
				w.CurrentEstimateUSD = newEst
				w.LastCalibrationAt = &now
				w.LastUpstreamPct = *norm.Used5hPercent
				w.CalibrationCount++
				updated = true
			}
		}
	}

	// Calibrate 7d window (uses its own independent localUSD counter)
	if norm.Used7dPercent != nil && calState.Window7d != nil {
		w := calState.Window7d
		if s.shouldCalibrate(now, w.LastCalibrationAt, *norm.Used7dPercent, w.LastUpstreamPct) {
			localUSD7d, _ := s.cache.GetAndResetLocalUSD(ctx, group.ID, "7d")
			if newEst := s.computeCalibration(w, *norm.Used7dPercent, group.Estimated7dLimitUSD, localUSD7d); newEst > 0 {
				group.Estimated7dLimitUSD = newEst
				w.CurrentEstimateUSD = newEst
				w.LastCalibrationAt = &now
				w.LastUpstreamPct = *norm.Used7dPercent
				w.CalibrationCount++
				updated = true
			}
		}
	}

	if updated {
		// Update Redis state estimates so CheckLimits picks up new values immediately
		if state, err := s.cache.GetGroupState(ctx, group.ID); err == nil && state != nil {
			state.Estimated5hLimitUSD = group.Estimated5hLimitUSD
			state.Estimated7dLimitUSD = group.Estimated7dLimitUSD
			_ = s.cache.SetGroupState(ctx, group.ID, state)
		}

		if s.groupRepo != nil {
			go func() {
				updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := s.groupRepo.UpdateQuotaShareEstimates(updateCtx, group.ID, group.Estimated5hLimitUSD, group.Estimated7dLimitUSD, calState); err != nil {
					slog.Error("quota_share: failed to persist calibration", "group_id", group.ID, "error", err)
				}
			}()
		}
	}
}

func (s *QuotaShareService) shouldCalibrate(now time.Time, lastCal *time.Time, currentPct, lastPct float64) bool {
	if lastCal != nil && now.Sub(*lastCal) < calibrationMinInterval {
		return false
	}
	deltaPct := math.Abs(currentPct - lastPct)
	return deltaPct >= calibrationMinDeltaPct
}

func (s *QuotaShareService) computeCalibration(w *domain.QuotaShareCalibrationWindowState, currentUpstreamPct, currentEstimate, localUSD float64) float64 {
	deltaPct := currentUpstreamPct - w.LastUpstreamPct
	if deltaPct < 0.5 {
		return 0 // too small to be reliable
	}

	if localUSD <= 0 {
		return 0
	}

	eNew := localUSD / (deltaPct / 100.0)

	// Sanity check: reject wild deviations
	if currentEstimate > 0 {
		deviation := eNew / currentEstimate
		if deviation > calibrationMaxDeviation || deviation < 1.0/calibrationMaxDeviation {
			slog.Warn("quota_share: calibration deviation too large, skipping",
				"e_new", eNew, "e_old", currentEstimate, "deviation", deviation)
			return 0
		}
	}

	alpha := w.EMAAlpha
	if alpha <= 0 || alpha > 1 {
		alpha = calibrationDefaultAlpha
	}

	if currentEstimate <= 0 {
		return eNew
	}
	return alpha*eNew + (1-alpha)*currentEstimate
}

func (s *QuotaShareService) getTotalWeight(ctx context.Context, groupID int64) (int, error) {
	total, err := s.cache.GetTotalWeight(ctx, groupID)
	if err == nil && total > 0 {
		return total, nil
	}

	// Cache miss — load from DB and cache
	if s.keyRepo == nil {
		return 0, errors.New("quota_share: key repository not configured")
	}
	total, err = s.keyRepo.GetTotalQuotaWeight(ctx, groupID)
	if err != nil {
		return 0, err
	}
	if total > 0 {
		_ = s.cache.SetTotalWeight(ctx, groupID, total)
	}
	return total, nil
}

// InvalidateTotalWeight removes the cached total weight for a group, forcing recalculation.
func (s *QuotaShareService) InvalidateTotalWeight(ctx context.Context, groupID int64) {
	_ = s.cache.DeleteTotalWeight(ctx, groupID)
}

// GetKeyLimits returns the computed limits for a key based on group estimates and weight.
func (s *QuotaShareService) GetKeyLimits(ctx context.Context, apiKey *APIKey, group *Group) (limit5h, limit7d float64, err error) {
	if group == nil || !group.IsQuotaShareType() {
		return 0, 0, nil
	}

	totalWeight, err := s.getTotalWeight(ctx, group.ID)
	if err != nil || totalWeight <= 0 {
		return 0, 0, err
	}

	weight := apiKey.QuotaWeight
	if weight <= 0 {
		weight = 1
	}
	ratio := float64(weight) / float64(totalWeight)

	// Read estimates from Redis state (not from auth-cache group)
	est5h, est7d := group.Estimated5hLimitUSD, group.Estimated7dLimitUSD
	if state, stateErr := s.cache.GetGroupState(ctx, group.ID); stateErr == nil && state != nil {
		if state.Estimated5hLimitUSD > 0 {
			est5h = state.Estimated5hLimitUSD
		}
		if state.Estimated7dLimitUSD > 0 {
			est7d = state.Estimated7dLimitUSD
		}
	}

	return est5h * ratio, est7d * ratio, nil
}

func (s *QuotaShareService) sumKeyWindowActualCost(ctx context.Context, apiKeyID int64, windowStart, windowEnd int64) (float64, error) {
	if s.usageRepo == nil {
		return 0, nil
	}
	if apiKeyID <= 0 || windowStart <= 0 || windowEnd <= 0 || windowEnd <= windowStart {
		return 0, nil
	}
	return s.usageRepo.SumAPIKeyActualCostInWindow(ctx, apiKeyID, time.Unix(windowStart, 0), time.Unix(windowEnd, 0))
}

// GetKeyUsage returns DB-backed actual_cost usage for the key in the current
// quota_share windows. Redis remains the hot-path limiter cache, but display
// should use usage_logs as the durable fact source.
func (s *QuotaShareService) GetKeyUsage(ctx context.Context, apiKey *APIKey, group *Group) (*QuotaShareKeyUsage, error) {
	if group == nil || apiKey == nil {
		return nil, nil
	}
	if s.usageRepo == nil {
		return s.cache.GetKeyUsage(ctx, group.ID, apiKey.ID)
	}

	state, err := s.cache.GetGroupState(ctx, group.ID)
	if err != nil {
		return nil, err
	}
	usage := &QuotaShareKeyUsage{}
	if state == nil {
		return usage, nil
	}

	if usage.Usage5h, err = s.sumKeyWindowActualCost(ctx, apiKey.ID, state.Window5hStart, state.Window5hEnd); err != nil {
		return nil, err
	}
	if usage.Usage7d, err = s.sumKeyWindowActualCost(ctx, apiKey.ID, state.Window7dStart, state.Window7dEnd); err != nil {
		return nil, err
	}
	return usage, nil
}

// GetGroupState returns the cached global window state for a quota_share group.
func (s *QuotaShareService) GetGroupState(ctx context.Context, groupID int64) (*QuotaShareGroupState, error) {
	return s.cache.GetGroupState(ctx, groupID)
}

// EnsureGroupStateEstimates updates the estimated limits in Redis state.
// Called by admin API when creating/updating a quota_share group's estimates.
func (s *QuotaShareService) EnsureGroupStateEstimates(ctx context.Context, groupID int64, est5h, est7d float64) {
	state, err := s.cache.GetGroupState(ctx, groupID)
	if err != nil {
		slog.Warn("quota_share: failed to read state for estimate update", "group_id", groupID, "error", err)
		state = &QuotaShareGroupState{}
	}
	if state == nil {
		state = &QuotaShareGroupState{}
	}
	state.Estimated5hLimitUSD = est5h
	state.Estimated7dLimitUSD = est7d
	state.UpdatedAt = time.Now().Unix()
	if err := s.cache.SetGroupState(ctx, groupID, state); err != nil {
		slog.Error("quota_share: failed to update estimates in Redis", "group_id", groupID, "error", err)
	}
}

// RegisterAccountGroup caches the association between an account and its quota_share group.
// Called during request processing when we know both the account and group.
func (s *QuotaShareService) RegisterAccountGroup(accountID, groupID int64) {
	s.accountGroupID.Store(accountID, groupID)
}

// UnregisterAccountGroup removes the cached account-to-group mapping.
// Should be called when an account is unbound from a quota_share group.
func (s *QuotaShareService) UnregisterAccountGroup(accountID int64) {
	s.accountGroupID.Delete(accountID)
}

// TryUpdateGlobalWindowByAccountID attempts to update global window state for a quota_share group
// associated with the given account. Returns silently if the account isn't in any quota_share group.
func (s *QuotaShareService) TryUpdateGlobalWindowByAccountID(ctx context.Context, accountID int64, snapshot *OpenAICodexUsageSnapshot) {
	if snapshot == nil {
		return
	}

	gidVal, ok := s.accountGroupID.Load(accountID)
	if !ok {
		return
	}
	groupID, ok := gidVal.(int64)
	if !ok || groupID <= 0 {
		return
	}

	group, err := s.groupRepo.GetByIDLite(ctx, groupID)
	if err != nil || group == nil || !group.IsQuotaShareType() {
		s.accountGroupID.Delete(accountID)
		return
	}

	s.UpdateGlobalWindow(ctx, group, snapshot)
	s.TryCalibrate(ctx, group, snapshot)
}
