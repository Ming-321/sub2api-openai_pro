package repository

import "testing"

func TestShouldResetQuotaShareWindowEnd(t *testing.T) {
	tests := []struct {
		name       string
		storedEnd  int64
		currentEnd int64
		tolerance  int64
		want       bool
	}{
		{name: "same end", storedEnd: 1000, currentEnd: 1000, tolerance: 120, want: false},
		{name: "small drift", storedEnd: 1000, currentEnd: 1008, tolerance: 120, want: false},
		{name: "large drift", storedEnd: 1000, currentEnd: 1400, tolerance: 120, want: true},
		{name: "missing stored", storedEnd: 0, currentEnd: 1400, tolerance: 120, want: true},
		{name: "missing current", storedEnd: 1000, currentEnd: 0, tolerance: 120, want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldResetQuotaShareWindowEnd(tc.storedEnd, tc.currentEnd, tc.tolerance)
			if got != tc.want {
				t.Fatalf("shouldResetQuotaShareWindowEnd(%d, %d, %d) = %v, want %v", tc.storedEnd, tc.currentEnd, tc.tolerance, got, tc.want)
			}
		})
	}
}
