package batch

// Default buffer sizes for channels used in batch processing.
// These values can be tuned based on performance requirements.
const (
	// DefaultItemBufferSize is the default buffer size for the items channel.
	// This determines how many items can be queued between the reader and processor.
	DefaultItemBufferSize = 100

	// DefaultIDBufferSize is the default buffer size for the ID generator channel.
	// This should match or exceed the item buffer size to avoid blocking.
	DefaultIDBufferSize = 100

	// DefaultErrorBufferSize is the default buffer size for the error channel.
	// This should be large enough to handle bursts of errors without blocking.
	DefaultErrorBufferSize = 100
)