package batch

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestConfigValues_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  ConfigValues
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config with all values",
			config: ConfigValues{
				MinItems: 10,
				MaxItems: 100,
				MinTime:  1 * time.Second,
				MaxTime:  5 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "valid config with only MinItems",
			config: ConfigValues{
				MinItems: 1,
			},
			wantErr: false,
		},
		{
			name: "valid - MinItems is zero (process immediately)",
			config: ConfigValues{
				MinItems: 0,
			},
			wantErr: false,
		},
		{
			name: "invalid - MinItems greater than MaxItems",
			config: ConfigValues{
				MinItems: 100,
				MaxItems: 10,
			},
			wantErr: true,
			errMsg:  "MinItems (100) cannot be greater than MaxItems (10)",
		},
		{
			name: "valid - MaxItems is zero (no limit)",
			config: ConfigValues{
				MinItems: 10,
				MaxItems: 0,
			},
			wantErr: false,
		},
		{
			name: "invalid - MinTime is negative",
			config: ConfigValues{
				MinItems: 1,
				MinTime:  -1 * time.Second,
			},
			wantErr: true,
			errMsg:  "MinTime cannot be negative",
		},
		{
			name: "invalid - MaxTime is negative",
			config: ConfigValues{
				MinItems: 1,
				MaxTime:  -1 * time.Second,
			},
			wantErr: true,
			errMsg:  "MaxTime cannot be negative",
		},
		{
			name: "invalid - MinTime greater than MaxTime",
			config: ConfigValues{
				MinItems: 1,
				MinTime:  5 * time.Second,
				MaxTime:  1 * time.Second,
			},
			wantErr: true,
			errMsg:  "MinTime (5s) cannot be greater than MaxTime (1s)",
		},
		{
			name: "valid - MaxTime is zero (no limit)",
			config: ConfigValues{
				MinItems: 1,
				MinTime:  1 * time.Second,
				MaxTime:  0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
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

func TestBatch_GoWithInvalidConfig(t *testing.T) {
	// Test that Go returns an error for invalid configuration
	b := New(NewConstantConfig(&ConfigValues{
		MinItems: 10,
		MaxItems: 5, // Invalid: MaxItems < MinItems
	}))

	// Create a simple test source
	src := SourceFunc(func(ctx context.Context) (<-chan interface{}, <-chan error) {
		out := make(chan interface{})
		errs := make(chan error)
		go func() {
			defer close(out)
			defer close(errs)
			for i := 1; i <= 3; i++ {
				select {
				case <-ctx.Done():
					return
				case out <- i:
				}
			}
		}()
		return out, errs
	})

	// Create a simple test processor
	proc := ProcessorFunc(func(ctx context.Context, items []*Item) ([]*Item, error) {
		return items, nil
	})

	errs := b.Go(context.Background(), src, proc)
	
	// Should get configuration error immediately
	err := <-errs
	if err == nil {
		t.Fatal("expected configuration error")
	}
	
	if !strings.Contains(err.Error(), "invalid configuration") {
		t.Errorf("expected invalid configuration error, got: %v", err)
	}
	
	// Ensure batch is marked as not running
	b.mu.Lock()
	if b.running {
		t.Error("batch should not be marked as running after configuration error")
	}
	b.mu.Unlock()
}
