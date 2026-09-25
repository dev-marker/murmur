package jobs

import (
	"testing"
	"time"
)

func TestBackoff(t *testing.T) {
	tests := []struct {
		attempt int
		r       float64
		want    time.Duration
	}{
		{1, 0, 500 * time.Millisecond},
		{1, 0.5, 750 * time.Millisecond},
		{2, 0, time.Second},
		{3, 0.5, 3 * time.Second},
		{4, 0, 4 * time.Second},
		{7, 0, 30 * time.Second},
		{100, 0.5, 45 * time.Second},
	}
	for _, tt := range tests {
		if got := backoff(tt.attempt, tt.r); got != tt.want {
			t.Errorf("backoff(%d, %v) = %v, want %v", tt.attempt, tt.r, got, tt.want)
		}
	}
}
