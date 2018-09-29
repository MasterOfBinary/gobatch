package pipeline

import (
	"context"
	"errors"
	"sync"

	"github.com/MasterOfBinary/gobatch/batch"
)

type DoerElem struct {
	Input  interface{}
	Output interface{}
	err    error

	done chan<- struct{}
}

type DoerFunc func(ctx context.Context, es []*DoerElem) error

func getDoerFuncOrNoop(s *Doer) DoerFunc {
	if s == nil || s.doerFunc == nil {
		return func(ctx context.Context, es []*DoerElem) error { return nil }
	}
	return s.doerFunc
}

func NewDoer(f DoerFunc) *Doer {
	return &Doer{
		doerFunc: f,
		isSetup:  true,
		ch:       make(chan *DoerElem),
	}
}

// Do not use BatchSetter directly; instead, use NewBatchSetter.
type Doer struct {
	doerFunc DoerFunc
	isSetup  bool

	// mu protects the following variables
	mu     sync.RWMutex
	ch     chan *DoerElem
	closed bool
}

// If ctx is done, ctx.Err() is returned.
func (s *Doer) Do(ctx context.Context, val interface{}) (interface{}, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if s == nil {
		return nil, errors.New("gobatch: called Set on nil BatchSetter")
	}

	// There's a bit of a tradeoff here: we could use an empty BatchSetter
	// as a noop, and if !isSetup we return nil. However, that could lead to
	// some hard-to-find bugs for the user, so we prefer to return an error
	// in this case.
	//
	// To create a noop BatchSetter, one can use NewBatchSetter(nil) in which
	// case we assume it's intentional.
	if !s.isSetup {
		return nil, errors.New("gobatch: called Set on uninitialized BatchSetter")
	}

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, errors.New("gobatch: BatchSetter has been closed")
	}

	// This needs to be done inside the mutex. Otherwise the channel could be
	// closed between the time we check and the time we write to the channel
	done := make(chan struct{})
	elem := &DoerElem{
		Input: val,
		done:  done,
	}
	s.ch <- elem

	s.mu.RUnlock()

	// Block and wait for the response
	<-done

	return elem.Output, elem.err
}

// Close can be called multiple times with no problems.
func (s *Doer) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	close(s.ch)
	s.closed = true
}

func (s *Doer) Read(ctx context.Context, ps *batch.PipelineStage) {
	defer ps.Close()

	out := ps.Output
	for item := range s.ch {
		out <- batch.NextItem(ps, item)
	}
}

// Note: if multiple items are in a batch, results are undefined.
func (s *Doer) Process(ctx context.Context, ps *batch.PipelineStage) {
	defer ps.Close()

	es := make([]*DoerElem, 0)

	for in := range ps.Input {
		var (
			cur *DoerElem
			ok  bool
		)
		if cur, ok = in.Get().(*DoerElem); !ok {
			// This shouldn't happen under normal circumstances
			ps.Errors <- errors.New("gobatch: unknown input type")
			continue
		}

		es = append(es, cur)
	}

	if len(es) == 0 {
		return
	}

	err := getDoerFuncOrNoop(s)(ctx, es)

	for _, e := range es {
		e.err = err
		close(e.done)
	}
}
