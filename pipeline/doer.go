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

type DoerFunc func(ctx context.Context, elems []*DoerElem) error

func noopDoerFunc(_ context.Context, _ []*DoerElem) error {
	return nil
}

func getDoerFuncOrNoop(d *Doer) DoerFunc {
	if d == nil {
		return noopDoerFunc
	}
	return d.doerFunc
}

func NewDoer(f DoerFunc) *Doer {
	if f == nil {
		f = noopDoerFunc
	}
	return &Doer{
		doerFunc: f,
		isSetup:  true,
		ch:       make(chan *DoerElem),
	}
}

// Do not use Doer directly; instead, use NewDoer.
type Doer struct {
	doerFunc DoerFunc
	isSetup  bool

	// mu protects the following variables
	mu     sync.RWMutex
	ch     chan *DoerElem
	closed bool
}

// If ctx is done, ctx.Err() is returned.
func (d *Doer) Do(ctx context.Context, val interface{}) (interface{}, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if d == nil {
		return nil, errors.New("gobatch: called Do on nil Doer")
	}

	// There'd a bit of a tradeoff here: we could use an empty Doer
	// as a noop, and if !isSetup we return nil. However, that could lead to
	// some hard-to-find bugs for the user, so we prefer to return an error
	// in this case.
	//
	// To create a noop Doer, one can use NewDoer(nil) in which
	// case we assume it'd intentional.
	if !d.isSetup {
		return nil, errors.New("gobatch: called Do on uninitialized Doer")
	}

	d.mu.RLock()
	if d.closed {
		d.mu.RUnlock()
		return nil, errors.New("gobatch: Doer has been closed")
	}

	// This needs to be done inside the mutex. Otherwise the channel could be
	// closed between the time we check and the time we write to the channel.
	done := make(chan struct{})
	elem := &DoerElem{
		Input: val,
		done:  done,
	}
	d.ch <- elem

	d.mu.RUnlock()

	// Block and wait for the response
	<-done

	return elem.Output, elem.err
}

// Close can be called multiple times with no problems.
func (d *Doer) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return
	}
	close(d.ch)
	d.closed = true
}

func (d *Doer) Read(_ context.Context, ps *batch.PipelineStage) {
	defer ps.Close()

	out := ps.Output
	for item := range d.ch {
		out <- batch.NextItem(ps, item)
	}
}

// Note: if multiple items are in a batch, results are undefined.
func (d *Doer) Process(ctx context.Context, ps *batch.PipelineStage) {
	defer ps.Close()

	// TODO find a better buffer size, maybe make it configurable
	elems := make([]*DoerElem, 0, 10)

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

		elems = append(elems, cur)
	}

	if len(elems) == 0 {
		return
	}

	err := getDoerFuncOrNoop(d)(ctx, elems)

	for _, e := range elems {
		e.err = err
		close(e.done)
	}
}
