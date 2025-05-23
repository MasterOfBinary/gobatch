package processor

import (
	"errors"
	"fmt"
)

// TransformConfig provides configuration options for creating a Transform processor.
type TransformConfig struct {
	// Func is the transformation function to apply to each item's Data field.
	// This field is required.
	Func TransformFunc

	// ContinueOnError determines whether to continue processing items after a transformation error.
	// If true, items with transformation errors will have their Error field set but processing continues.
	// If false, the processor will return an error and stop after the first transformation failure.
	// Default is true (continue processing).
	ContinueOnError bool
}

// Validate checks if the TransformConfig is valid.
func (c TransformConfig) Validate() error {
	if c.Func == nil {
		return errors.New("transformation function cannot be nil")
	}
	return nil
}

// NewTransform creates a new Transform processor with the given configuration.
// It validates the configuration and returns an error if invalid.
//
// Example:
//
//	proc, err := processor.NewTransform(processor.TransformConfig{
//		Func: func(data interface{}) (interface{}, error) {
//			// Transform logic here
//			return data, nil
//		},
//		ContinueOnError: true,
//	})
//	if err != nil {
//		// handle error
//	}
func NewTransform(config TransformConfig) (*Transform, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid transform config: %w", err)
	}

	return &Transform{
		Func:            config.Func,
		ContinueOnError: config.ContinueOnError,
	}, nil
}

// FilterConfig provides configuration options for creating a Filter processor.
type FilterConfig struct {
	// Predicate is a function that returns true for items that should be kept
	// and false for items that should be filtered out.
	// This field is required.
	Predicate FilterFunc

	// InvertMatch inverts the predicate logic: if true, items matching the predicate
	// will be removed instead of kept.
	// Default is false (keep matching items).
	InvertMatch bool
}

// Validate checks if the FilterConfig is valid.
func (c FilterConfig) Validate() error {
	if c.Predicate == nil {
		return errors.New("predicate function cannot be nil")
	}
	return nil
}

// NewFilter creates a new Filter processor with the given configuration.
// It validates the configuration and returns an error if invalid.
//
// Example:
//
//	proc, err := processor.NewFilter(processor.FilterConfig{
//		Predicate: func(item *batch.Item) bool {
//			// Return true to keep the item, false to filter it out
//			return item.Data != nil
//		},
//		InvertMatch: false,
//	})
//	if err != nil {
//		// handle error
//	}
func NewFilter(config FilterConfig) (*Filter, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid filter config: %w", err)
	}

	return &Filter{
		Predicate:   config.Predicate,
		InvertMatch: config.InvertMatch,
	}, nil
}

// ErrorConfig provides configuration options for creating an Error processor.
type ErrorConfig struct {
	// Err is the error to apply to each item.
	// If nil, a default "processor error" will be used.
	Err error

	// FailFraction controls what fraction of items should have errors applied.
	// Value range is 0.0 to 1.0, where:
	// - 0.0 means no items will have errors (processor becomes a pass-through) - this is the zero value default
	// - 1.0 means all items will have errors
	// - 0.5 means approximately half the items will have errors
	// Note: If you want all items to fail, explicitly set FailFraction to 1.0
	FailFraction float64
}

// NewError creates a new Error processor with the given configuration.
// Unlike other processors, this doesn't require validation as nil error is valid.
//
// Example:
//
//	proc := processor.NewError(processor.ErrorConfig{
//		Err:          errors.New("processing failed"),
//		FailFraction: 1.0, // All items will fail
//	})
func NewError(config ErrorConfig) *Error {
	return &Error{
		Err:          config.Err,
		FailFraction: config.FailFraction,
	}
}

// NewNil creates a new Nil processor.
// The Nil processor passes through all items unchanged.
//
// Example:
//
//	proc := processor.NewNil()
func NewNil() *Nil {
	return &Nil{}
}
