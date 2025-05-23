package source

import (
	"errors"
	"fmt"
)

// ChannelConfig provides configuration options for creating a Channel source.
type ChannelConfig struct {
	// Input is the channel from which this source will read data.
	// This field is required.
	Input <-chan interface{}

	// BufferSize controls the size of the output buffer.
	// If zero or negative, defaultChannelBuffer is used.
	BufferSize int
}

// Validate checks if the ChannelConfig is valid.
func (c ChannelConfig) Validate() error {
	if c.Input == nil {
		return errors.New("input channel cannot be nil")
	}
	return nil
}

// NewChannel creates a new Channel source with the given configuration.
// It validates the configuration and returns an error if invalid.
//
// Example:
//
//	input := make(chan interface{}, 10)
//	src, err := source.NewChannel(source.ChannelConfig{
//		Input:      input,
//		BufferSize: 100,
//	})
//	if err != nil {
//		// handle error
//	}
func NewChannel(config ChannelConfig) (*Channel, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid channel config: %w", err)
	}

	return &Channel{
		Input:      config.Input,
		BufferSize: config.BufferSize,
	}, nil
}

// ErrorConfig provides configuration options for creating an Error source.
type ErrorConfig struct {
	// Errs is the channel from which this source will read errors.
	// This field is required.
	Errs <-chan error

	// BufferSize controls the size of the error buffer.
	// If zero or negative, defaultErrorBuffer is used.
	BufferSize int
}

// Validate checks if the ErrorConfig is valid.
func (c ErrorConfig) Validate() error {
	if c.Errs == nil {
		return errors.New("error channel cannot be nil")
	}
	return nil
}

// NewError creates a new Error source with the given configuration.
// It validates the configuration and returns an error if invalid.
//
// Example:
//
//	errCh := make(chan error, 5)
//	src, err := source.NewError(source.ErrorConfig{
//		Errs:       errCh,
//		BufferSize: 10,
//	})
//	if err != nil {
//		// handle error
//	}
func NewError(config ErrorConfig) (*Error, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid error config: %w", err)
	}

	return &Error{
		Errs:       config.Errs,
		BufferSize: config.BufferSize,
	}, nil
}

// NewNil creates a new Nil source.
// The Nil source immediately closes its channels without providing any data.
//
// Example:
//
//	src := source.NewNil()
func NewNil() *Nil {
	return &Nil{}
}
