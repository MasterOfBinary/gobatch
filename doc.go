// Package gobatch contains a batch processor. The main class is Batch,
// which can be created using New. It reads from an implementation of the
// source.Source interface, and items are processed in batches by an
// implementation of the processor.Processor interface. Some Source and
// Processor implementations are provided in the source and processor
// packages, respectively, or you can create your own custom one.
//
// Batch uses the MinTime, MinItems, MaxTime, and MaxItems configuration
// parameters in BatchConfig to determine when and how many items are
// processed at once.
//
// These parameters may conflict, however; for example, during a slow time,
// MaxTime may be reached before MinItems are read. Thus it is necessary
// to prioritize the parameters in some way. They are prioritized as follows
// (with EOF signifying the end of the input data):
//
//    MaxTime = MaxItems > EOF > MinTime > MinItems
//
// A few examples:
//
// MinTime = 2s. After 1s the input channel is closed. The items are
// processed right away.
//
// MinItems = 10, MinTime = 2s. After 1s, 10 items have been read. They are
// not processed until 2s has passed (along with all other items that have
// been read up to the 2s mark).
//
// MaxItems = 10, MinTime = 2s. After 1s, 10 items have been read. They aren't
// processed until 2s has passed.
//
// Note that the time and item counters are relative to when the last batch
// started processing.
package gobatch
