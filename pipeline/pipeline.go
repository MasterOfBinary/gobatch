// Package pipeline contains implementations that combine both Source and Processor
// functionality into unified components. This allows for creating higher-level
// abstractions that can simplify how users interact with the batch processing system.
//
// The Pipeline Concept:
//
// A Pipeline encapsulates both data sourcing and processing logic into a single component
// that presents a simplified, often synchronous API to the caller while leveraging
// the asynchronous batch processing capabilities of GoBatch internally.
//
// The key benefits of using a Pipeline are:
//
//  1. Simplified API: Callers can use straightforward, synchronous method calls
//     (like Get(key)) without having to understand or interact with the batch
//     processing system directly.
//
//  2. Batching Transparency: The Pipeline handles all batching logic internally,
//     automatically grouping operations for efficiency without requiring the caller
//     to manage batches.
//
//  3. Combined Interfaces: By implementing both Source and Processor interfaces,
//     a Pipeline can connect directly to the GoBatch system without needing
//     separate components.
//
// Current Implementations:
//
//   - RedisPipeline: Provides a synchronous API for Redis operations while
//     batching commands in the background for efficiency.
package pipeline
