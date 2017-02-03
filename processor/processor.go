package processor

import "context"

type Processor interface {
	Process(ctx context.Context, items []interface{}) error
}
