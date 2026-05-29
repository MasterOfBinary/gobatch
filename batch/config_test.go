package batch

import (
	"math"
	"sync"
	"testing"
	"time"
)

func TestNewConstantConfig_NilValues(t *testing.T) {
	cfg := NewConstantConfig(nil)
	got := cfg.Get()

	if got.MinItems != 0 || got.MaxItems != 0 || got.MinTime != 0 || got.MaxTime != 0 {
		t.Errorf("expected default zero values, got %+v", got)
	}
}

func TestNewConstantConfig_WithValues(t *testing.T) {
	expected := ConfigValues{
		MinItems: 10,
		MaxItems: 100,
		MinTime:  5 * time.Second,
		MaxTime:  30 * time.Second,
	}

	cfg := NewConstantConfig(&expected)
	got := cfg.Get()

	if got != expected {
		t.Errorf("expected %+v, got %+v", expected, got)
	}
}

func TestNewDynamicConfig_NilValues(t *testing.T) {
	cfg := NewDynamicConfig(nil)
	got := cfg.Get()

	if got.MinItems != 0 || got.MaxItems != 0 || got.MinTime != 0 || got.MaxTime != 0 {
		t.Errorf("expected default zero values, got %+v", got)
	}
}

func TestNewDynamicConfig_WithValues(t *testing.T) {
	expected := ConfigValues{
		MinItems: 5,
		MaxItems: 50,
		MinTime:  1 * time.Second,
		MaxTime:  10 * time.Second,
	}

	cfg := NewDynamicConfig(&expected)
	got := cfg.Get()

	if got != expected {
		t.Errorf("expected %+v, got %+v", expected, got)
	}
}

func TestDynamicConfig_UpdateBatchSize(t *testing.T) {
	cfg := NewDynamicConfig(nil)
	cfg.UpdateBatchSize(20, 200)

	got := cfg.Get()

	if got.MinItems != 20 || got.MaxItems != 200 {
		t.Errorf("expected MinItems=20, MaxItems=200, got %+v", got)
	}
}

func TestDynamicConfig_UpdateTiming(t *testing.T) {
	cfg := NewDynamicConfig(nil)
	cfg.UpdateTiming(2*time.Second, 15*time.Second)

	got := cfg.Get()

	if got.MinTime != 2*time.Second || got.MaxTime != 15*time.Second {
		t.Errorf("expected MinTime=2s, MaxTime=15s, got %+v", got)
	}
}

func TestDynamicConfig_Update(t *testing.T) {
	cfg := NewDynamicConfig(nil)
	update := ConfigValues{
		MinItems: 7,
		MaxItems: 70,
		MinTime:  3 * time.Second,
		MaxTime:  20 * time.Second,
	}
	cfg.Update(update)

	got := cfg.Get()

	if got != update {
		t.Errorf("expected %+v, got %+v", update, got)
	}
}

func TestDynamicConfig_ConcurrentAccess(t *testing.T) {
	cfg := NewDynamicConfig(&ConfigValues{
		MinItems: 1,
		MaxItems: 10,
		MinTime:  1 * time.Second,
		MaxTime:  5 * time.Second,
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			cfg.UpdateBatchSize(uint64(i), uint64(i*10))
		}(i)
		go func() {
			defer wg.Done()
			_ = cfg.Get()
		}()
	}
	wg.Wait()
}

// --- Contract-locking tests ---
//
// The tests below assert behavior that already exists; they lock the
// documented contract (config.go) so future changes that would silently
// break it are caught. They are expected to pass immediately because no
// behavioral code changed alongside them.

// TestConstantConfig_Get_RoundTripZeroValues locks the contract that
// ConstantConfig.Get faithfully returns explicitly-supplied zero values.
// Zero values carry documented "unset/disabled" meaning (see ConfigValues
// godoc); the config type itself must not silently rewrite them — that
// interpretation belongs to the engine, not the config holder.
func TestConstantConfig_Get_RoundTripZeroValues(t *testing.T) {
	zero := ConfigValues{
		MinItems: 0,
		MaxItems: 0,
		MinTime:  0,
		MaxTime:  0,
	}

	got := NewConstantConfig(&zero).Get()

	if got != zero {
		t.Errorf("ConstantConfig.Get altered zero values: expected %+v, got %+v", zero, got)
	}
}

// TestDynamicConfig_Get_RoundTripZeroValues locks the same zero-value
// round-trip contract for DynamicConfig.Get.
func TestDynamicConfig_Get_RoundTripZeroValues(t *testing.T) {
	zero := ConfigValues{
		MinItems: 0,
		MaxItems: 0,
		MinTime:  0,
		MaxTime:  0,
	}

	got := NewDynamicConfig(&zero).Get()

	if got != zero {
		t.Errorf("DynamicConfig.Get altered zero values: expected %+v, got %+v", zero, got)
	}
}

// TestDynamicConfig_Mutators_AcceptZeroAndLargeValues is a backwards-compat
// guard: Update, UpdateBatchSize, and UpdateTiming must accept zero values
// and extreme (max-uint64 / very large duration) values without panicking or
// rejecting them, and Get must round-trip exactly what was set. This locks the
// hard constraint that config setters perform no validation that could break
// existing callers.
func TestDynamicConfig_Mutators_AcceptZeroAndLargeValues(t *testing.T) {
	cases := []struct {
		name   string
		values ConfigValues
	}{
		{
			name:   "all zero",
			values: ConfigValues{},
		},
		{
			name: "max uint64 counts",
			values: ConfigValues{
				MinItems: math.MaxUint64,
				MaxItems: math.MaxUint64,
				MinTime:  0,
				MaxTime:  0,
			},
		},
		{
			name: "max duration times",
			values: ConfigValues{
				MinItems: 0,
				MaxItems: 0,
				MinTime:  time.Duration(math.MaxInt64),
				MaxTime:  time.Duration(math.MaxInt64),
			},
		},
		{
			name: "min greater than max (no clamping at config layer)",
			values: ConfigValues{
				MinItems: 1000,
				MaxItems: 1,
				MinTime:  10 * time.Second,
				MaxTime:  1 * time.Second,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Update: replaces all values at once.
			cfgUpdate := NewDynamicConfig(nil)
			cfgUpdate.Update(tc.values)
			if got := cfgUpdate.Get(); got != tc.values {
				t.Errorf("Update round-trip mismatch: expected %+v, got %+v", tc.values, got)
			}

			// UpdateBatchSize + UpdateTiming: the split setters must reach the
			// same state, with no clamping or rejection at the config layer.
			cfgSplit := NewDynamicConfig(nil)
			cfgSplit.UpdateBatchSize(tc.values.MinItems, tc.values.MaxItems)
			cfgSplit.UpdateTiming(tc.values.MinTime, tc.values.MaxTime)
			if got := cfgSplit.Get(); got != tc.values {
				t.Errorf("UpdateBatchSize/UpdateTiming round-trip mismatch: expected %+v, got %+v", tc.values, got)
			}
		})
	}
}

// TestDynamicConfig_ConcurrentReadWrite_AllMutators locks thread-safety of
// DynamicConfig across every exported mutator (not just UpdateBatchSize) under
// the race detector. Run with -race to exercise the contract.
func TestDynamicConfig_ConcurrentReadWrite_AllMutators(t *testing.T) {
	cfg := NewDynamicConfig(&ConfigValues{
		MinItems: 1,
		MaxItems: 10,
		MinTime:  1 * time.Second,
		MaxTime:  5 * time.Second,
	})

	const iterations = 200
	var wg sync.WaitGroup

	for i := 0; i < iterations; i++ {
		wg.Add(4)
		go func(i int) {
			defer wg.Done()
			cfg.UpdateBatchSize(uint64(i), uint64(i*10))
		}(i)
		go func(i int) {
			defer wg.Done()
			cfg.UpdateTiming(time.Duration(i)*time.Millisecond, time.Duration(i*2)*time.Millisecond)
		}(i)
		go func(i int) {
			defer wg.Done()
			cfg.Update(ConfigValues{
				MinItems: uint64(i),
				MaxItems: uint64(i + 1),
				MinTime:  time.Duration(i) * time.Microsecond,
				MaxTime:  time.Duration(i+1) * time.Microsecond,
			})
		}(i)
		go func() {
			defer wg.Done()
			_ = cfg.Get()
		}()
	}

	wg.Wait()
}
