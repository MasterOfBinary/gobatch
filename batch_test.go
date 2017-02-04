package gobatch

import (
	"errors"
	"testing"
	"time"
)

func TestMust(t *testing.T) {
	batch, _ := New(nil)

	if Must(batch, nil) != batch {
		t.Error("Must(batch, nil) != batch")
	}

	var panics bool
	func() {
		defer func() {
			if p := recover(); p != nil {
				panics = true
			}
		}()
		_ = Must(batch, errors.New("error"))
	}()

	if !panics {
		t.Error("Must(batch, err) doesn't panic")
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		config  *BatchConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: false,
		},
		{
			name:    "empty config",
			config:  &BatchConfig{},
			wantErr: false,
		},
		{
			name: "good config",
			config: &BatchConfig{
				MinItems:        5,
				MaxItems:        10,
				MinTime:         time.Second,
				MaxTime:         2 * time.Second,
				ReadConcurrency: 5,
			},
			wantErr: false,
		},
		{
			name: "min time only",
			config: &BatchConfig{
				MinTime: time.Second,
			},
			wantErr: false,
		},
		{
			name: "min items only",
			config: &BatchConfig{
				MinItems: 5,
			},
			wantErr: false,
		},
		{
			name: "bad items",
			config: &BatchConfig{
				MinItems: 10,
				MaxItems: 5,
			},
			wantErr: true,
		},
		{
			name: "bad times",
			config: &BatchConfig{
				MinTime: 2 * time.Second,
				MaxTime: time.Second,
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			batch, err := New(test.config)
			if test.wantErr && err == nil {
				t.Error("New(config) returns nil error, want not nil")
			} else if !test.wantErr {
				if err != nil {
					t.Errorf("New(config) returns error %v, want nil", err)
				}
				if batch == nil {
					t.Error("New(config) returns nil batch, want not nil")
				}
			}
		})
	}
}
