package gobatch

import "errors"

var ErrConcurrentGoCalls = errors.New("Concurrent calls to Go are not allowed")

type ProcessorError struct {
	err error
}

func (e ProcessorError) Error() string {
	return e.err.Error()
}

func (e ProcessorError) Original() error {
	return e.err
}

type SourceError struct {
	err error
}

func newSourceError(err error) {
	return &SourceError{
		err: err,
	}
}

func (e SourceError) Error() string {
	return e.err.Error()
}

func (e SourceError) Original() error {
	return e.err
}
