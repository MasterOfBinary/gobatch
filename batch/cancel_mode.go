package batch

// CancelMode controls how a Batch reacts to context cancellation.
//
// The zero value is CancelDrain, which preserves the historical behavior:
// items already read from the Source are still processed and the Batch relies
// on the Source to stop producing and close its channels.
type CancelMode int

const (
	// CancelDrain (the default) keeps processing items already read from the
	// Source when the context is canceled, relying on the Source to stop
	// producing and close its channels. Nothing already read into the pipeline
	// is dropped. (Internal error sends remain context-aware so a full,
	// undrained error channel still cannot deadlock the pipeline.)
	CancelDrain CancelMode = iota

	// CancelStop makes the Batch stop reading promptly when the context is
	// canceled instead of waiting for the Source. Items already buffered in the
	// pipeline are still processed, but items not yet read from the Source may
	// be dropped.
	CancelStop
)
