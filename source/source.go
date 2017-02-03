package source

import "context"

type Source interface {
	Read(ctx context.Context, items chan<- interface{}, errs chan<- error)
}

//go:generate mockery -name=Source -case=underscore
