package batch

import (
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

func TestFixConfigClampNegativeDurations(t *testing.T) {
	tests := []struct {
		name   string
		input  ConfigValues
		expect ConfigValues
	}{
		{
			name: "negative min time",
			input: ConfigValues{
				MinItems: 1,
				MinTime:  -5 * time.Second,
			},
			expect: ConfigValues{
				MinItems: 1,
				MinTime:  0,
			},
		},
		{
			name: "negative max time",
			input: ConfigValues{
				MinItems: 1,
				MinTime:  2 * time.Second,
				MaxTime:  -3 * time.Second,
			},
			expect: ConfigValues{
				MinItems: 1,
				MinTime:  2 * time.Second,
				MaxTime:  0,
			},
		},
		{
			name: "both negative",
			input: ConfigValues{
				MinItems: 1,
				MinTime:  -1 * time.Second,
				MaxTime:  -2 * time.Second,
			},
			expect: ConfigValues{
				MinItems: 1,
				MinTime:  0,
				MaxTime:  0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fixConfig(tt.input)
			if got != tt.expect {
				t.Errorf("expected %+v, got %+v", tt.expect, got)
			}
		})
	}
}
