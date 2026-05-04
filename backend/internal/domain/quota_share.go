package domain

import "time"

const (
	QuotaShareCalibrationSuggestionStatusPending          = "pending"
	QuotaShareCalibrationSuggestionStatusInsufficientData = "insufficient_data"
	QuotaShareCalibrationSuggestionStatusRejected         = "rejected"
	QuotaShareCalibrationSuggestionStatusApplied          = "applied"
	QuotaShareCalibrationSuggestionStatusDiscarded        = "discarded"
)

// QuotaShareCalibrationSuggestion holds a semi-automatic calibration proposal.
// The proposal is informational until an administrator applies it.
type QuotaShareCalibrationSuggestion struct {
	Window               string     `json:"window"`
	Status               string     `json:"status"`
	Reason               string     `json:"reason,omitempty"`
	SuggestedEstimateUSD float64    `json:"suggested_estimate_usd,omitempty"`
	CurrentEstimateUSD   float64    `json:"current_estimate_usd"`
	LocalUSD             float64    `json:"local_usd,omitempty"`
	UpstreamPctStart     float64    `json:"upstream_pct_start"`
	UpstreamPctCurrent   float64    `json:"upstream_pct_current"`
	UpstreamPctDelta     float64    `json:"upstream_pct_delta"`
	EMAAlpha             float64    `json:"ema_alpha,omitempty"`
	CalculatedAt         *time.Time `json:"calculated_at,omitempty"`
	AppliedAt            *time.Time `json:"applied_at,omitempty"`
	DiscardedAt          *time.Time `json:"discarded_at,omitempty"`
}

// QuotaShareCalibrationWindowState holds calibration state for a single window (5h or 7d).
type QuotaShareCalibrationWindowState struct {
	CurrentEstimateUSD float64                          `json:"current_estimate_usd"`
	LastCalibrationAt  *time.Time                       `json:"last_calibration_at,omitempty"`
	LastUpstreamPct    float64                          `json:"last_upstream_pct"`
	LastLocalUSD       float64                          `json:"last_local_usd"`
	EMAAlpha           float64                          `json:"ema_alpha"`
	CalibrationCount   int                              `json:"calibration_count"`
	PendingSuggestion  *QuotaShareCalibrationSuggestion `json:"pending_suggestion,omitempty"`
}

// QuotaShareCalibrationState holds calibration state for both windows.
type QuotaShareCalibrationState struct {
	Window5h *QuotaShareCalibrationWindowState `json:"5h,omitempty"`
	Window7d *QuotaShareCalibrationWindowState `json:"7d,omitempty"`
}
