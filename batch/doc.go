// Package batch contains the core batch processing functionality.
// The main type is Batch, which can be created using New. It reads from a
// Source implementation and processes items in batches using one or more
// Processor implementations. Some Source and Processor implementations are
// provided in related packages, or you can create your own based on your needs.
//
// Batch uses MinTime, MinItems, MaxTime, and MaxItems from Config to determine
// when and how many items are processed at once.
//
// These parameters may conflict; for example, during slow periods,
// MaxTime may be reached before MinItems are collected. In these cases,
// the following priority order is used (EOF means end of input data):
//
//	MaxTime = MaxItems > EOF > MinTime > MinItems
//
// A few examples:
//
// - MinTime = 2s. After 1s the input channel is closed. The items are processed right away.
// - MinItems = 10, MinTime = 2s. After 1s, 10 items have been read. They are not processed until 2s has passed.
// - MaxItems = 10, MinTime = 2s. After 1s, 10 items have been read. They are not processed until 2s has passed.
//
// Timers and counters are relative to when the previous batch finished processing.
// Each batch starts a new MinTime/MaxTime window and counts new items from zero.
//
// Processors can be chained together. Each processor receives the output items
// from the previous processor:
//
//	b.Go(ctx, source, processor1, processor2, processor3)
//
// The configuration is reloaded before each batch is collected. This allows
// dynamic Config implementations to update batch behavior during processing.
package batch
