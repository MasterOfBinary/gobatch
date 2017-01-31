package source

import "context"

type Source interface {
	Read(ctx context.Context) ([]interface{}, error)
}
