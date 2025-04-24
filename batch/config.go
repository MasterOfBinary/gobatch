// Package batch provides a flexible batch processing pipeline for handling data.
package batch

import "time"

// Config retrieves the config values used by Batch. If these values are
// constant, NewConstantConfig can be used to create an implementation
// of the interface.
//
// The Config interface allows for dynamic configuration of the batching behavior,
// which can be adjusted during runtime. This is useful for tuning the system
// under different load scenarios or adapting to changing performance requirements.
type Config interface {
	// Get returns the values for configuration.
	//
	// If MinItems > MaxItems or MinTime > MaxTime, the min value will be
	// set to the maximum value.
	//
	// If the config values may be modified during batch processing, Get
	// must properly handle concurrency issues.
	Get() ConfigValues
}

// ConfigValues is a struct that contains the Batch config values.
// These values control the timing and sizing behavior of batches in the pipeline.
// The batch system uses these parameters to determine when to process a batch
// based on time elapsed and number of items collected.
type ConfigValues struct {
	// MinTime specifies that a minimum amount of time that should pass
	// before processing items. The exception to this is if a max number
	// of items was specified and that number is reached before MinTime;
	// in that case those items will be processed right away.
	//
	// This parameter is useful to prevent processing very small batches
	// too frequently when items arrive at a slow but steady rate.
	MinTime time.Duration `json:"minTime"`

	// MinItems specifies that a minimum number of items should be
	// processed at a time. Items will not be processed until MinItems
	// items are ready for processing. The exceptions to that are if MaxTime
	// is specified and that time is reached before the minimum number of
	// items is available, or if all items have been read and are ready
	// to process.
	//
	// This parameter helps optimize processing by ensuring batches are
	// large enough to amortize the overhead of processing across multiple items.
	MinItems uint64 `json:"minItems"`

	// MaxTime specifies that a maximum amount of time should pass before
	// processing. Once that time has been reached, items will be processed
	// whether or not MinItems items are available.
	//
	// This parameter ensures that items don't wait in the queue for too long,
	// which is important for latency-sensitive applications.
	MaxTime time.Duration `json:"maxTime"`

	// MaxItems specifies that a maximum number of items should be available
	// before processing. Once that number of items is available, they will
	// be processed whether or not MinTime has been reached.
	//
	// This parameter prevents the system from accumulating too many items
	// in a single batch, which could lead to memory pressure or processing
	// spikes.
	MaxItems uint64 `json:"maxItems"`
}

// NewConstantConfig returns a Config with constant values. If values
// is nil, the default values are used as described in Batch.
//
// This is a convenience function for creating a configuration that doesn't
// change during the lifetime of the batch processing. It's the simplest
// way to provide configuration to the Batch system.
func NewConstantConfig(values *ConfigValues) *ConstantConfig {
	if values == nil {
		return &ConstantConfig{}
	}

	return &ConstantConfig{
		values: *values,
	}
}

// ConstantConfig is a Config with constant values. Create one with
// NewConstantConfig.
//
// This implementation is safe to use concurrently since the values
// never change after initialization.
type ConstantConfig struct {
	values ConfigValues
}

// Get implements the Config interface.
// Returns the constant configuration values stored in this ConstantConfig.
func (b *ConstantConfig) Get() ConfigValues {
	return b.values
}
