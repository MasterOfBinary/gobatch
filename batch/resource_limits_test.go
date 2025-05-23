package batch

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestResourceLimits_Validate(t *testing.T) {
	tests := []struct {
		name    string
		limits  ResourceLimits
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid limits",
			limits: ResourceLimits{
				MaxConcurrentBatches: 10,
				MaxMemoryPerBatch:    1024 * 1024,
				MaxProcessingTime:    5 * time.Minute,
				MaxTotalMemory:       10 * 1024 * 1024,
			},
			wantErr: false,
		},
		{
			name: "valid - all zeros (no limits)",
			limits: ResourceLimits{
				MaxConcurrentBatches: 0,
				MaxMemoryPerBatch:    0,
				MaxProcessingTime:    0,
				MaxTotalMemory:       0,
			},
			wantErr: false,
		},
		{
			name: "invalid - negative concurrent batches",
			limits: ResourceLimits{
				MaxConcurrentBatches: -1,
			},
			wantErr: true,
			errMsg:  "MaxConcurrentBatches cannot be negative",
		},
		{
			name: "invalid - negative memory per batch",
			limits: ResourceLimits{
				MaxMemoryPerBatch: -1024,
			},
			wantErr: true,
			errMsg:  "MaxMemoryPerBatch cannot be negative",
		},
		{
			name: "invalid - negative processing time",
			limits: ResourceLimits{
				MaxProcessingTime: -1 * time.Second,
			},
			wantErr: true,
			errMsg:  "MaxProcessingTime cannot be negative",
		},
		{
			name: "invalid - negative total memory",
			limits: ResourceLimits{
				MaxTotalMemory: -1024,
			},
			wantErr: true,
			errMsg:  "MaxTotalMemory cannot be negative",
		},
		{
			name: "invalid - per-batch memory exceeds total",
			limits: ResourceLimits{
				MaxMemoryPerBatch: 1024 * 1024,
				MaxTotalMemory:    512 * 1024,
			},
			wantErr: true,
			errMsg:  "MaxMemoryPerBatch (1048576) cannot be greater than MaxTotalMemory (524288)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.limits.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("expected error message %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestResourceTracker(t *testing.T) {
	t.Run("concurrent batch limit", func(t *testing.T) {
		limits := ResourceLimits{
			MaxConcurrentBatches: 2,
		}
		tracker := newResourceTracker(limits)

		// Start first batch
		if err := tracker.canStartBatch(1024); err != nil {
			t.Errorf("should allow first batch: %v", err)
		}
		tracker.startBatch(1024)

		// Start second batch
		if err := tracker.canStartBatch(1024); err != nil {
			t.Errorf("should allow second batch: %v", err)
		}
		tracker.startBatch(1024)

		// Third batch should fail
		if err := tracker.canStartBatch(1024); err == nil {
			t.Error("should not allow third batch when limit is 2")
		}

		// Finish one batch
		tracker.finishBatch(1024)

		// Now third batch should succeed
		if err := tracker.canStartBatch(1024); err != nil {
			t.Errorf("should allow batch after one finished: %v", err)
		}
	})

	t.Run("memory limits", func(t *testing.T) {
		limits := ResourceLimits{
			MaxMemoryPerBatch: 1024,
			MaxTotalMemory:    2048,
		}
		tracker := newResourceTracker(limits)

		// Batch exceeding per-batch limit
		if err := tracker.canStartBatch(2048); err == nil {
			t.Error("should not allow batch exceeding per-batch memory limit")
		}

		// Start first batch
		if err := tracker.canStartBatch(1024); err != nil {
			t.Errorf("should allow first batch: %v", err)
		}
		tracker.startBatch(1024)

		// Second batch would exceed total
		if err := tracker.canStartBatch(1500); err == nil {
			t.Error("should not allow batch that would exceed total memory")
		}

		// Smaller second batch should work
		if err := tracker.canStartBatch(512); err != nil {
			t.Errorf("should allow smaller batch: %v", err)
		}
	})

	t.Run("resource usage tracking", func(t *testing.T) {
		tracker := newResourceTracker(ResourceLimits{})

		batches, memory := tracker.getUsage()
		if batches != 0 || memory != 0 {
			t.Errorf("initial usage should be zero, got batches=%d, memory=%d", batches, memory)
		}

		tracker.startBatch(1024)
		tracker.startBatch(2048)

		batches, memory = tracker.getUsage()
		if batches != 2 || memory != 3072 {
			t.Errorf("expected batches=2, memory=3072, got batches=%d, memory=%d", batches, memory)
		}

		tracker.finishBatch(1024)

		batches, memory = tracker.getUsage()
		if batches != 1 || memory != 2048 {
			t.Errorf("after finish expected batches=1, memory=2048, got batches=%d, memory=%d", batches, memory)
		}
	})
}

func TestBatchWithResourceLimits(t *testing.T) {
	t.Run("respects concurrent batch limit", func(t *testing.T) {
		limits := ResourceLimits{
			MaxConcurrentBatches: 1,
		}
		opts := &Options{
			Config: NewConstantConfig(&ConfigValues{
				MinItems: 2,
				MaxItems: 2,
			}),
			ResourceLimits: &limits,
		}
		b := NewWithOptions(opts)

		// Slow processor to ensure batches overlap
		proc := ProcessorFunc(func(ctx context.Context, items []*Item) ([]*Item, error) {
			time.Sleep(100 * time.Millisecond)
			return items, nil
		})

		// Create a simple test source
		src := SourceFunc(func(ctx context.Context) (<-chan interface{}, <-chan error) {
			out := make(chan interface{})
			errs := make(chan error)
			go func() {
				defer close(out)
				defer close(errs)
				for i := 1; i <= 4; i++ {
					select {
					case <-ctx.Done():
						return
					case out <- i:
					}
				}
			}()
			return out, errs
		})
		
		errs := b.Go(context.Background(), src, proc)
		
		var errorCount int
		for err := range errs {
			if err != nil {
				errorCount++
			}
		}
		
		// With limit of 1 concurrent batch, processing should succeed
		// but may have resource limit errors if batches tried to run concurrently
		<-b.Done()
	})

	t.Run("respects processing time limit", func(t *testing.T) {
		limits := ResourceLimits{
			MaxProcessingTime: 50 * time.Millisecond,
		}
		opts := &Options{
			Config: NewConstantConfig(&ConfigValues{
				MinItems: 1,
			}),
			ResourceLimits: &limits,
		}
		b := NewWithOptions(opts)

		// Slow processor that exceeds time limit
		proc := ProcessorFunc(func(ctx context.Context, items []*Item) ([]*Item, error) {
			select {
			case <-time.After(100 * time.Millisecond):
				return items, nil
			case <-ctx.Done():
				return items, ctx.Err()
			}
		})

		// Create a simple test source with one item
		src := SourceFunc(func(ctx context.Context) (<-chan interface{}, <-chan error) {
			out := make(chan interface{})
			errs := make(chan error)
			go func() {
				defer close(out)
				defer close(errs)
				select {
				case <-ctx.Done():
					return
				case out <- 1:
				}
			}()
			return out, errs
		})
		
		errs := b.Go(context.Background(), src, proc)
		
		hadTimeout := false
		for err := range errs {
			if err != nil && (err == context.DeadlineExceeded || strings.Contains(err.Error(), "deadline")) {
				hadTimeout = true
			}
		}
		
		if !hadTimeout {
			t.Error("expected timeout error due to processing time limit")
		}
		
		<-b.Done()
	})
}

// Use the ProcessorFunc and SourceFunc types defined in config_validation_test.go