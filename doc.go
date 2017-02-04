// Package gobatch contains a batch processor. The main class is Batch,
// which can be created using New.
//
// The MinTime, MinItems, MaxTime, and MaxItems configuration parameters
// in BatchConfig are used to specify when and how many items are processed
// at once.
//
// These parameters may conflict, however; for example, during a slow time,
// MaxTime may be reached before MinItems are read. Thus it is necessary
// to prioritize the parameters. They are prioritized as follows (with EOF
// signifying the end of the input data):
//
//    MaxTime = MaxItems > EOF > MinItems > MinTime
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
// MaxItems = 10, MinTime = 2s. After 1s, 10 items have been read. They are
// processed right away.
//
// Note that in all cases, the time and item counter is relative to the last
// batch.
package gobatch
