package batch

import (
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"
	"time"
)

// ResourceLimits defines limits on system resources used by batch processing.
// These limits help prevent resource exhaustion and ensure stable operation
// under heavy load.
type ResourceLimits struct {
	// MaxConcurrentBatches limits the number of batches that can be processed
	// concurrently. A value of 0 means no limit.
	MaxConcurrentBatches int

	// MaxMemoryPerBatch limits the estimated memory usage per batch in bytes.
	// This is a soft limit based on item count and average item size.
	// A value of 0 means no limit.
	MaxMemoryPerBatch int64

	// MaxProcessingTime limits the maximum time a single batch can take to process.
	// If exceeded, the batch processing is considered failed.
	// A value of 0 means no limit.
	MaxProcessingTime time.Duration

	// MaxTotalMemory limits the total memory that can be used by all batches.
	// This is a soft limit that triggers backpressure when approached.
	// A value of 0 means no limit.
	MaxTotalMemory int64
}

// DefaultResourceLimits returns sensible default resource limits based on
// the system's available resources.
func DefaultResourceLimits() ResourceLimits {
	numCPU := runtime.NumCPU()
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	
	// Use 80% of system memory as total limit
	totalMem := int64(float64(memStats.Sys) * 0.8)
	
	return ResourceLimits{
		MaxConcurrentBatches: numCPU * 2,           // 2 batches per CPU
		MaxMemoryPerBatch:    totalMem / int64(numCPU*4), // Divide available memory
		MaxProcessingTime:    5 * time.Minute,       // 5 minutes max per batch
		MaxTotalMemory:       totalMem,
	}
}

// Validate checks if the resource limits are valid.
func (r ResourceLimits) Validate() error {
	if r.MaxConcurrentBatches < 0 {
		return errors.New("MaxConcurrentBatches cannot be negative")
	}
	
	if r.MaxMemoryPerBatch < 0 {
		return errors.New("MaxMemoryPerBatch cannot be negative")
	}
	
	if r.MaxProcessingTime < 0 {
		return errors.New("MaxProcessingTime cannot be negative")
	}
	
	if r.MaxTotalMemory < 0 {
		return errors.New("MaxTotalMemory cannot be negative")
	}
	
	if r.MaxTotalMemory > 0 && r.MaxMemoryPerBatch > 0 && r.MaxMemoryPerBatch > r.MaxTotalMemory {
		return fmt.Errorf("MaxMemoryPerBatch (%d) cannot be greater than MaxTotalMemory (%d)",
			r.MaxMemoryPerBatch, r.MaxTotalMemory)
	}
	
	return nil
}

// resourceTracker tracks resource usage for enforcing limits.
type resourceTracker struct {
	activeBatches    int32
	estimatedMemory  int64
	limits           ResourceLimits
}

// newResourceTracker creates a new resource tracker with the given limits.
func newResourceTracker(limits ResourceLimits) *resourceTracker {
	return &resourceTracker{
		limits: limits,
	}
}

// canStartBatch checks if a new batch can be started given current resource usage.
func (rt *resourceTracker) canStartBatch(estimatedSize int64) error {
	if rt.limits.MaxConcurrentBatches > 0 {
		current := atomic.LoadInt32(&rt.activeBatches)
		if int(current) >= rt.limits.MaxConcurrentBatches {
			return fmt.Errorf("maximum concurrent batches limit reached (%d)", rt.limits.MaxConcurrentBatches)
		}
	}
	
	if rt.limits.MaxTotalMemory > 0 {
		currentMem := atomic.LoadInt64(&rt.estimatedMemory)
		if currentMem+estimatedSize > rt.limits.MaxTotalMemory {
			return fmt.Errorf("would exceed total memory limit (current: %d, requested: %d, limit: %d)",
				currentMem, estimatedSize, rt.limits.MaxTotalMemory)
		}
	}
	
	if rt.limits.MaxMemoryPerBatch > 0 && estimatedSize > rt.limits.MaxMemoryPerBatch {
		return fmt.Errorf("batch size %d exceeds per-batch memory limit %d",
			estimatedSize, rt.limits.MaxMemoryPerBatch)
	}
	
	return nil
}

// startBatch marks a batch as started and updates resource counters.
func (rt *resourceTracker) startBatch(estimatedSize int64) {
	atomic.AddInt32(&rt.activeBatches, 1)
	atomic.AddInt64(&rt.estimatedMemory, estimatedSize)
}

// finishBatch marks a batch as completed and updates resource counters.
func (rt *resourceTracker) finishBatch(estimatedSize int64) {
	atomic.AddInt32(&rt.activeBatches, -1)
	atomic.AddInt64(&rt.estimatedMemory, -estimatedSize)
}

// getUsage returns current resource usage statistics.
func (rt *resourceTracker) getUsage() (activeBatches int32, estimatedMemory int64) {
	return atomic.LoadInt32(&rt.activeBatches), atomic.LoadInt64(&rt.estimatedMemory)
}