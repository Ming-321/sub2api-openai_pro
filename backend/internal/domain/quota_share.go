package domain

import "time"

// QuotaShareCalibrationWindowState holds calibration state for a single window (5h or 7d).
type QuotaShareCalibrationWindowState struct {
	CurrentEstimateUSD float64    `json:"current_estimate_usd"`
	LastCalibrationAt  *time.Time `json:"last_calibration_at,omitempty"`
	LastUpstreamPct    float64    `json:"last_upstream_pct"`
	LastLocalUSD       float64    `json:"last_local_usd"`
	EMAAlpha           float64    `json:"ema_alpha"`
	CalibrationCount   int        `json:"calibration_count"`
}

// QuotaShareCalibrationState holds calibration state for both windows.
type QuotaShareCalibrationState struct {
	Window5h *QuotaShareCalibrationWindowState `json:"5h,omitempty"`
	Window7d *QuotaShareCalibrationWindowState `json:"7d,omitempty"`
}
