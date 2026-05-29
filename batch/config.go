// Package batch provides a flexible batch processing pipeline for handling data.
package batch

import (
	"sync"
	"time"
)

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
	// The batch engine clamps conflicting min/max values, but only when the
	// corresponding maximum is greater than zero (non-zero). Specifically:
	// if MaxItems > 0 and MinItems > MaxItems, MinItems is reduced to
	// MaxItems; if MaxTime > 0 and MinTime > MaxTime, MinTime is reduced to
	// MaxTime. When the maximum is zero it is treated as unset (see
	// ConfigValues.MaxItems and ConfigValues.MaxTime), so no clamping occurs
	// and the min value is used as-is.
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
	// A value of 0 means there is no minimum-time wait: a batch becomes
	// eligible as soon as MinItems is satisfied, without waiting for any
	// time to elapse.
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
	// A value of 0 is treated by the engine as 1: at least one item is
	// processed per batch. MinItems is therefore never effectively zero at
	// runtime.
	//
	// This parameter helps optimize processing by ensuring batches are
	// large enough to amortize the overhead of processing across multiple items.
	MinItems uint64 `json:"minItems"`

	// MaxTime specifies that a maximum amount of time should pass before
	// processing. Once that time has been reached, items will be processed
	// whether or not MinItems items are available.
	//
	// A value of 0 means unset / disabled: there is NO maximum-time flush
	// and the underlying timer is not started. This is counter-intuitive
	// (zero may read like "flush immediately"), but a zero MaxTime imposes
	// no upper bound on how long a batch may wait for MinItems.
	//
	// This parameter ensures that items don't wait in the queue for too long,
	// which is important for latency-sensitive applications.
	MaxTime time.Duration `json:"maxTime"`

	// MaxItems specifies that a maximum number of items should be available
	// before processing. Once that number of items is available, they will
	// be processed whether or not MinTime has been reached.
	//
	// A value of 0 means unset / disabled: there is NO maximum-count
	// trigger. This is counter-intuitive (zero may read like "flush on the
	// first item"), but a zero MaxItems imposes no upper bound on batch
	// size; batches are then bounded only by MinItems, MaxTime, or the
	// source being exhausted.
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

// NewDynamicConfig creates a configuration that can be adjusted at runtime.
// It is thread-safe and suitable for use in environments where batch processing
// parameters need to change dynamically in response to system conditions.
//
// If values is nil, the default values are used as described in Batch.
//
// This is useful for:
// - Systems that need to adapt to changing workloads
// - Services that implement backpressure mechanisms
// - Applications that tune batch parameters based on performance metrics
func NewDynamicConfig(values *ConfigValues) *DynamicConfig {
	if values == nil {
		return &DynamicConfig{}
	}

	return &DynamicConfig{
		minItems: values.MinItems,
		maxItems: values.MaxItems,
		minTime:  values.MinTime,
		maxTime:  values.MaxTime,
	}
}

// DynamicConfig implements the Config interface with values that can be
// modified at runtime. It provides thread-safe access to configuration values
// and methods to update batch size and timing parameters.
//
// Unlike ConstantConfig, DynamicConfig allows changing batch parameters while
// the system is running, enabling dynamic adaptation to varying conditions.
type DynamicConfig struct {
	mu       sync.RWMutex
	minItems uint64
	maxItems uint64
	minTime  time.Duration
	maxTime  time.Duration
}

// Get implements the Config interface by returning the current configuration values.
// It uses a read lock to ensure thread safety when accessing the values.
func (c *DynamicConfig) Get() ConfigValues {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return ConfigValues{
		MinItems: c.minItems,
		MaxItems: c.maxItems,
		MinTime:  c.minTime,
		MaxTime:  c.maxTime,
	}
}

// UpdateBatchSize updates the batch size parameters.
// This method is thread-safe and can be called while batch processing is active.
func (c *DynamicConfig) UpdateBatchSize(minItems, maxItems uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.minItems = minItems
	c.maxItems = maxItems
}

// UpdateTiming updates the timing parameters.
// This method is thread-safe and can be called while batch processing is active.
func (c *DynamicConfig) UpdateTiming(minTime, maxTime time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.minTime = minTime
	c.maxTime = maxTime
}

// Update replaces all configuration values at once.
// This method is thread-safe and can be called while batch processing is active.
func (c *DynamicConfig) Update(config ConfigValues) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.minItems = config.MinItems
	c.maxItems = config.MaxItems
	c.minTime = config.MinTime
	c.maxTime = config.MaxTime
}
