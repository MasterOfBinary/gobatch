package batch

// Options contains optional configuration for creating a new Batch.
// It allows specifying both configuration and resource limits.
type Options struct {
	// Config provides the batching configuration.
	// If nil, default configuration is used.
	Config Config

	// ResourceLimits defines resource constraints.
	// If nil, no resource limits are enforced.
	ResourceLimits *ResourceLimits
}

// WithDefaults returns Options with default values where not specified.
func (o *Options) WithDefaults() *Options {
	if o == nil {
		o = &Options{}
	}

	if o.Config == nil {
		o.Config = NewConstantConfig(nil)
	}

	return o
}

// NewWithOptions creates a new Batch with the given options.
// This is the recommended way to create a Batch with resource limits.
//
// Example:
//
//	limits := batch.DefaultResourceLimits()
//	opts := &batch.Options{
//		Config: batch.NewConstantConfig(&batch.ConfigValues{
//			MinItems: 10,
//			MaxItems: 100,
//		}),
//		ResourceLimits: &limits,
//	}
//	b := batch.NewWithOptions(opts)
func NewWithOptions(opts *Options) *Batch {
	opts = opts.WithDefaults()

	b := &Batch{
		config: opts.Config,
	}

	if opts.ResourceLimits != nil {
		b.resourceTracker = newResourceTracker(*opts.ResourceLimits)
	}

	return b
}
