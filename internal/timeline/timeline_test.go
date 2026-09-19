package timeline

import (
	"slices"
	"testing"
)

func TestMerge(t *testing.T) {
	tests := []struct {
		name string
		a, b []int64
		n    int
		want []int64
	}{
		{"only the timeline", []int64{9, 5, 2}, nil, 3, []int64{9, 5, 2}},
		{"only pulled", nil, []int64{8, 4}, 3, []int64{8, 4}},
		{"cut at n", []int64{9, 5, 2}, []int64{8, 4}, 3, []int64{9, 8, 5}},
		{"the same post in both", []int64{9, 8, 5}, []int64{8, 4}, 3, []int64{9, 8, 5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := merge(tt.a, tt.b, tt.n); !slices.Equal(got, tt.want) {
				t.Errorf("merge(%v, %v, %d) = %v, want %v", tt.a, tt.b, tt.n, got, tt.want)
			}
		})
	}
}
