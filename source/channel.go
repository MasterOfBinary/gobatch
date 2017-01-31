package source

import "context"

type channelSource struct {
	ch <-chan interface{}
}

func Channel(ch <-chan interface{}) Source {
	return &channelSource{
		ch: ch,
	}
}

func (s *channelSource) Read(ctx context.Context) ([]interface{}, error) {
	select {
	case item, ok := <-s.ch:
		if ok {
			resp := make([]interface{}, 1)
			resp[0] = item
			return resp, nil
		}
	case <-ctx.Done():
	}

	return make([]interface{}, 0), nil
}
