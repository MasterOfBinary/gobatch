// Package source contains several implementations of the batch.Source
// interface for common data source scenarios, including:
//
// - Channel: For using existing channels as batch sources
// - Error: For simulating error-only sources without data
// - Nil: For testing timing behavior without emitting data
//
// Each source implementation handles context cancellation properly and
// ensures channels are closed appropriately.
package source
