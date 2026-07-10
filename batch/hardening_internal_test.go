package batch

import "testing"

// TestClampPreallocCap verifies that the pre-allocation capacity helper returns
// a sane, bounded value. A huge MinItems (reachable via DynamicConfig with
// MaxItems==0) must never be used directly as a slice capacity, since that
// would trigger a multi-TB allocation and crash the process.
func TestClampPreallocCap(t *testing.T) {
	tests := []struct {
		name     string
		minItems uint64
		maxItems uint64
		want     int
	}{
		{
			name:     "small min, no max - exact",
			minItems: 8,
			maxItems: 0,
			want:     8,
		},
		{
			name:     "zero min, no max - zero",
			minItems: 0,
			maxItems: 0,
			want:     0,
		},
		{
			name:     "min at the cap boundary - exact",
			minItems: maxPreallocCap,
			maxItems: 0,
			want:     maxPreallocCap,
		},
		{
			name:     "huge min, no max - clamped to cap",
			minItems: 1 << 40, // ~1 trillion: a real make() of this would OOM
			maxItems: 0,
			want:     maxPreallocCap,
		},
		{
			name:     "max uint64 min, no max - clamped to cap",
			minItems: ^uint64(0),
			maxItems: 0,
			want:     maxPreallocCap,
		},
		{
			name:     "maxItems caps below minItems",
			minItems: 1000,
			maxItems: 16,
			want:     16,
		},
		{
			name:     "maxItems caps a huge minItems",
			minItems: 1 << 40,
			maxItems: 32,
			want:     32,
		},
		{
			// maxItems only ever caps downward; it must not inflate the
			// pre-allocation. With a small minItems the batch starts small and
			// grows via append, so we pre-allocate just minItems here.
			name:     "huge maxItems does not inflate prealloc - uses min",
			minItems: 10,
			maxItems: 1 << 40,
			want:     10,
		},
		{
			name:     "both small, max above min - uses min",
			minItems: 4,
			maxItems: 64,
			want:     4,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got := clampPreallocCap(tt.minItems, tt.maxItems)
			if got != tt.want {
				t.Errorf("clampPreallocCap(%d, %d) = %d, want %d",
					tt.minItems, tt.maxItems, got, tt.want)
			}
			if got < 0 {
				t.Errorf("clampPreallocCap returned negative capacity %d", got)
			}
			if got > maxPreallocCap {
				t.Errorf("clampPreallocCap returned %d, which exceeds maxPreallocCap %d", got, maxPreallocCap)
			}
		})
	}
}
