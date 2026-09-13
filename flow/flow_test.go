package flow_test

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
	"github.com/MasterOfBinary/gobatch/flow"
	"github.com/MasterOfBinary/gobatch/source"
)

func wait(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("flow did not reach barrier")
	}
}
func result(t *testing.T, ch <-chan error) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(3 * time.Second):
		t.Fatal("flow did not join")
		return nil
	}
}

// Reordering or running a later step after failure changes the recorded effects.
func TestSerialDeclarationOrderStopsOnError(t *testing.T) {
	for _, nested := range []bool{false, true} {
		t.Run(map[bool]string{false: "execute", true: "sequence"}[nested], func(t *testing.T) {
			var order []int
			failure := errors.New("write failed")
			steps := []flow.Step{
				func(context.Context) error { order = append(order, 1); return nil },
				func(context.Context) error { order = append(order, 2); return failure },
				func(context.Context) error { order = append(order, 3); return nil },
			}
			if nested {
				steps = []flow.Step{flow.Sequence(steps...)}
			}
			if err := flow.Execute(context.Background(), steps...); !errors.Is(err, failure) {
				t.Errorf("failure lost: %v", err)
			}
			if !reflect.DeepEqual(order, []int{1, 2}) {
				t.Errorf("effects=%v; want [1 2]", order)
			}
		})
	}
}

// A runtime header identifies the actual library worker, so its absence proves
// completion rather than merely that a callback reached its last statement.
// Both Go 1.18.10 and Go 1.26.3 use "goroutine <id> [<state>]:" headers.
func goroutineHeader() string {
	buf := make([]byte, 1024)
	n := runtime.Stack(buf, false)
	return strings.SplitN(string(buf[:n]), " [", 2)[0] + " ["
}

func awaitGoroutineExit(t *testing.T, header string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		if n == len(buf) {
			t.Fatal("goroutine snapshot truncated")
		}
		if !strings.Contains(string(buf[:n]), header) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("later-declared branch did not finish")
		}
		runtime.Gosched()
	}
}

// Completion order is forced: the second worker exits before the first is
// released to fail. A third branch owns its result until all branches join.
func TestParallelStartsAllJoinsAndSelectsDeclarationError(t *testing.T) {
	entered := make(chan struct{}, 3)
	secondWorker := make(chan string, 1)
	firstRelease, release := make(chan struct{}), make(chan struct{})
	var firstOnce sync.Once
	defer firstOnce.Do(func() { close(firstRelease) })
	var once sync.Once
	defer once.Do(func() { close(release) })
	firstErr, secondErr := errors.New("first"), errors.New("second")
	values := [3]int{}
	done := make(chan error, 1)
	go func() {
		done <- flow.Execute(context.Background(), flow.Parallel(
			func(ctx context.Context) error { entered <- struct{}{}; <-firstRelease; values[0] = 1; return firstErr },
			func(ctx context.Context) error {
				entered <- struct{}{}
				values[1] = 2
				secondWorker <- goroutineHeader()
				return secondErr
			},
			func(ctx context.Context) error { entered <- struct{}{}; <-release; values[2] = 3; return ctx.Err() },
		))
	}()
	for i := 0; i < 3; i++ {
		wait(t, entered)
	}
	var worker string
	select {
	case worker = <-secondWorker:
	case <-time.After(3 * time.Second):
		t.Fatal("second branch did not start")
	}
	awaitGoroutineExit(t, worker)
	firstOnce.Do(func() { close(firstRelease) })
	select {
	case err := <-done:
		t.Fatalf("returned before held branch finished: %v", err)
	default:
	}
	once.Do(func() { close(release) })
	if err := result(t, done); !errors.Is(err, firstErr) {
		t.Errorf("declaration-order failure=%v", err)
	}
	if values != [3]int{1, 2, 3} {
		t.Errorf("branches not joined: %v", values)
	}
}

// A failed sibling must not cancel a successful sibling's context. All branches
// enter even with a canceled parent; each callback decides how to cooperate.
func TestParallelCancellationIsParentOwned(t *testing.T) {
	failure := errors.New("sibling failed")
	failed := make(chan struct{})
	var siblingErr error
	err := flow.Execute(context.Background(), flow.Parallel(
		func(context.Context) error { close(failed); return failure },
		func(ctx context.Context) error { <-failed; siblingErr = ctx.Err(); return siblingErr },
	))
	if !errors.Is(err, failure) || siblingErr != nil {
		t.Fatalf("err=%v sibling context=%v", err, siblingErr)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered, observed, release := make(chan struct{}, 2), make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	done := make(chan error, 1)
	var completed bool
	go func() {
		done <- flow.Execute(ctx, flow.Parallel(
			func(ctx context.Context) error {
				entered <- struct{}{}
				<-ctx.Done()
				close(observed)
				return ctx.Err()
			},
			func(context.Context) error { entered <- struct{}{}; <-release; completed = true; return nil },
		))
	}()
	wait(t, entered)
	wait(t, entered)
	cancel()
	wait(t, observed)
	select {
	case err := <-done:
		t.Fatalf("abandoned noncooperative branch: %v", err)
	default:
	}
	once.Do(func() { close(release) })
	if err := result(t, done); !errors.Is(err, context.Canceled) || !completed {
		t.Fatalf("err=%v completed=%v", err, completed)
	}
	calls := 0
	if err := flow.Execute(ctx, func(got context.Context) error { calls++; return got.Err() }); !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("canceled parent not passed to callback: %v calls=%d", err, calls)
	}
}

// Wrapping any context would break callers that store request-scoped values or
// compare the context they passed to the callback.
func TestCallbacksReceiveOriginalCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for name, compose := range map[string]func(flow.Step) flow.Step{
		"execute":  func(step flow.Step) flow.Step { return step },
		"sequence": func(step flow.Step) flow.Step { return flow.Sequence(step) },
		"parallel": func(step flow.Step) flow.Step { return flow.Parallel(step, step) },
	} {
		t.Run(name, func(t *testing.T) {
			wantCalls := 1
			if name == "parallel" {
				wantCalls = 2
			}
			seen := make(chan context.Context, wantCalls)
			step := func(got context.Context) error {
				seen <- got
				return nil
			}
			if err := flow.Execute(ctx, compose(step)); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			for i := 0; i < wantCalls; i++ {
				if got := <-seen; got != ctx || got.Err() != context.Canceled {
					t.Fatalf("callback context = %v (err %v); want original canceled parent", got, got.Err())
				}
			}
		})
	}
}

func TestEmptyAndNilSteps(t *testing.T) {
	for _, step := range []flow.Step{flow.Sequence(), flow.Parallel()} {
		if err := flow.Execute(context.Background(), step); err != nil {
			t.Error(err)
		}
	}
	if err := flow.Execute(context.Background()); err != nil {
		t.Error(err)
	}
	for name, step := range map[string]flow.Step{"execute": nil, "sequence": flow.Sequence(nil), "parallel": flow.Parallel(nil)} {
		t.Run(name, func(t *testing.T) {
			later := false
			err := flow.Execute(context.Background(), step, func(context.Context) error { later = true; return nil })
			if !errors.Is(err, flow.ErrNilStep) || later {
				t.Fatalf("nil step failure=%v later=%v", err, later)
			}
		})
	}
	calls := 0
	err := flow.Execute(context.Background(), flow.Parallel(nil, func(context.Context) error { calls++; return nil }))
	if !errors.Is(err, flow.ErrNilStep) || calls != 1 {
		t.Fatalf("nil branch suppressed sibling: %v calls=%d", err, calls)
	}
}

type hostilePanic struct{}

func (hostilePanic) Error() string { panic("panic value must never be formatted") }

func TestRecoveredPanicsStopSerialAndJoinParallel(t *testing.T) {
	for name, step := range map[string]flow.Step{
		"execute":  func(context.Context) error { panic(hostilePanic{}) },
		"sequence": flow.Sequence(func(context.Context) error { panic(nil) }),
	} {
		t.Run(name, func(t *testing.T) {
			later := false
			err := flow.Execute(context.Background(), step, func(context.Context) error { later = true; return nil })
			var pe *flow.PanicError
			if !errors.As(err, &pe) || later {
				t.Fatalf("panic failure=%v later=%v", err, later)
			}
			if pe.Boundary != "flow.step" || len(pe.Stack()) == 0 || len(pe.Stack()) > 16384 {
				t.Fatalf("missing bounded diagnostic: %v", pe)
			}
		})
	}
	for _, abnormal := range []bool{false, true} {
		t.Run(map[bool]string{false: "parallel_panic", true: "parallel_goexit"}[abnormal], func(t *testing.T) {
			entered, panicked, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var once sync.Once
			defer once.Do(func() { close(release) })
			done := make(chan error, 1)
			value := 0
			go func() {
				done <- flow.Execute(context.Background(), flow.Parallel(
					func(context.Context) error {
						<-entered
						defer close(panicked)
						if abnormal {
							runtime.Goexit()
						}
						panic(hostilePanic{})
					},
					func(context.Context) error { close(entered); <-release; value = 42; return nil },
				))
			}()
			wait(t, panicked)
			select {
			case err := <-done:
				t.Fatalf("panic abandoned sibling: %v", err)
			default:
			}
			once.Do(func() { close(release) })
			err := result(t, done)
			var pe *flow.PanicError
			if !errors.As(err, &pe) || value != 42 {
				t.Fatalf("panic=%v sibling value=%d", err, value)
			}
		})
	}
}

// Changing the caller's construction slice must not replace the declared work.
func TestCompositionFreezesDeclaredSteps(t *testing.T) {
	for name, compose := range map[string]func(...flow.Step) flow.Step{"sequence": flow.Sequence, "parallel": flow.Parallel} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			steps := []flow.Step{func(context.Context) error { calls++; return nil }}
			step := compose(steps...)
			steps[0] = nil
			for i := 0; i < 2; i++ {
				if err := flow.Execute(context.Background(), step); err != nil {
					t.Fatal(err)
				}
			}
			if calls != 2 {
				t.Fatalf("calls=%d", calls)
			}
		})
	}
}

type batchProcessor func(context.Context, []*batch.Item[int]) ([]*batch.Item[int], error)

func (p batchProcessor) Process(ctx context.Context, items []*batch.Item[int]) ([]*batch.Item[int], error) {
	return p(ctx, items)
}

// Real small-buffer batches produce more errors than fit, finish their final
// worker, then return to flow. A failed write must suppress the dependent step.
func TestBatchStepDrainsAndJoinsBeforeDependent(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			input := make(chan int, 4)
			for i := 0; i < 4; i++ {
				input <- i
			}
			close(input)
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			defer once.Do(func() { close(release) })
			var (
				values   []int
				valuesMu sync.Mutex
			)
			sortedValues := func() []int {
				valuesMu.Lock()
				defer valuesMu.Unlock()
				got := append([]int(nil), values...)
				sort.Ints(got)
				return got
			}
			var failures []error
			dependent := false
			writeErr := errors.New("write failed")
			b := batch.New[int](nil).WithBufferConfig(batch.BufferConfig{ItemBufferSize: 1, ErrorBufferSize: 1})
			done := make(chan error, 1)
			go func() {
				done <- flow.Execute(context.Background(),
					func(ctx context.Context) error {
						failures = batch.RunBatchAndWait[int](ctx, b, &source.Channel[int]{Input: input}, batchProcessor(func(_ context.Context, items []*batch.Item[int]) ([]*batch.Item[int], error) {
							for _, item := range items {
								if item.Data == 3 {
									close(entered)
									<-release
								}
								valuesMu.Lock()
								values = append(values, item.Data)
								valuesMu.Unlock()
							}
							if fail {
								return items, writeErr
							}
							return items, nil
						}))
						if len(failures) > 0 {
							return failures[0]
						}
						return nil
					},
					func(context.Context) error {
						dependent = true
						select {
						case <-b.Done():
						default:
							return errors.New("batch not joined")
						}
						if !reflect.DeepEqual(sortedValues(), []int{0, 1, 2, 3}) {
							return errors.New("incomplete batch")
						}
						return nil
					},
				)
			}()
			wait(t, entered)
			select {
			case err := <-done:
				t.Fatalf("flow returned with held processor: %v", err)
			default:
			}
			once.Do(func() { close(release) })
			err := result(t, done)
			if fail {
				if !errors.Is(err, writeErr) || dependent || len(failures) != 4 {
					t.Fatalf("err=%v dependent=%v failures=%d", err, dependent, len(failures))
				}
			} else if err != nil || !dependent {
				t.Fatalf("err=%v dependent=%v", err, dependent)
			}
			if got := sortedValues(); !reflect.DeepEqual(got, []int{0, 1, 2, 3}) {
				t.Errorf("unfinished batch: %v", got)
			}
		})
	}
}
