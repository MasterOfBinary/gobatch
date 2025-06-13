package batch

import (
	"testing"
	"time"
)

func TestFixConfig(t *testing.T) {
	tests := []struct {
		name     string
		input    ConfigValues
		expected ConfigValues
	}{
		{
			name:     "MinItems is 0",
			input:    ConfigValues{MinItems: 0, MaxItems: 10, MinTime: 0, MaxTime: 0},
			expected: ConfigValues{MinItems: 1, MaxItems: 10, MinTime: 0, MaxTime: 0},
		},
		{
			name:     "MinItems is 5 (no change)",
			input:    ConfigValues{MinItems: 5, MaxItems: 10, MinTime: 0, MaxTime: 0},
			expected: ConfigValues{MinItems: 5, MaxItems: 10, MinTime: 0, MaxTime: 0},
		},
		{
			name:     "MinTime greater than MaxTime",
			input:    ConfigValues{MinItems: 1, MaxItems: 10, MinTime: 10 * time.Second, MaxTime: 5 * time.Second},
			expected: ConfigValues{MinItems: 1, MaxItems: 10, MinTime: 5 * time.Second, MaxTime: 5 * time.Second},
		},
		{
			name:     "MinTime less than MaxTime (no change)",
			input:    ConfigValues{MinItems: 1, MaxItems: 10, MinTime: 5 * time.Second, MaxTime: 10 * time.Second},
			expected: ConfigValues{MinItems: 1, MaxItems: 10, MinTime: 5 * time.Second, MaxTime: 10 * time.Second},
		},
		{
			name:     "MinItems greater than MaxItems",
			input:    ConfigValues{MinItems: 100, MaxItems: 10, MinTime: 0, MaxTime: 0},
			expected: ConfigValues{MinItems: 10, MaxItems: 10, MinTime: 0, MaxTime: 0},
		},
		{
			name:     "MinItems less than MaxItems (no change)",
			input:    ConfigValues{MinItems: 10, MaxItems: 100, MinTime: 0, MaxTime: 0},
			expected: ConfigValues{MinItems: 10, MaxItems: 100, MinTime: 0, MaxTime: 0},
		},
		{
			name:     "MinItems is 0 and MinTime greater than MaxTime",
			input:    ConfigValues{MinItems: 0, MaxItems: 10, MinTime: 10 * time.Second, MaxTime: 5 * time.Second},
			expected: ConfigValues{MinItems: 1, MaxItems: 10, MinTime: 5 * time.Second, MaxTime: 5 * time.Second},
		},
		{
			name:     "MinItems greater than MaxItems and MinTime greater than MaxTime",
			input:    ConfigValues{MinItems: 100, MaxItems: 10, MinTime: 10 * time.Second, MaxTime: 5 * time.Second},
			expected: ConfigValues{MinItems: 10, MaxItems: 10, MinTime: 5 * time.Second, MaxTime: 5 * time.Second},
		},
		{
			name:     "Only MaxTime set",
			input:    ConfigValues{MaxTime: 5 * time.Second},
			expected: ConfigValues{MinItems: 1, MaxTime: 5 * time.Second},
		},
		{
			name:     "Only MaxItems set",
			input:    ConfigValues{MaxItems: 5},
			expected: ConfigValues{MinItems: 1, MaxItems: 5},
		},
		{
			name:     "All zero values",
			input:    ConfigValues{},
			expected: ConfigValues{MinItems: 1},
		},
		{
			name:     "No changes needed (MinItems=1, valid times and items)",
			input:    ConfigValues{MinItems: 1, MaxItems: 10, MinTime: 1 * time.Second, MaxTime: 2 * time.Second},
			expected: ConfigValues{MinItems: 1, MaxItems: 10, MinTime: 1 * time.Second, MaxTime: 2 * time.Second},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fixConfig(tt.input)
			// Compare individual fields for better error messages if we used a more complex comparison.
			// However, for ConfigValues, direct struct comparison is fine for test logic.
			if got.MinItems != tt.expected.MinItems ||
				got.MaxItems != tt.expected.MaxItems ||
				got.MinTime != tt.expected.MinTime ||
				got.MaxTime != tt.expected.MaxTime {
				t.Errorf("fixConfig(%+v) = %+v, want %+v", tt.input, got, tt.expected)
			}
		})
	}
}
