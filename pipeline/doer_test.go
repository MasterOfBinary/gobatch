package pipeline

import (
	"context"
	"errors"
	"math/rand"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
)

const (
	minItems  = 10
	randomKVs = 100
)

var cache basicCache

type basicCache map[string]string

func (b basicCache) Set(_ context.Context, key, value string) {
	b[key] = value
}

func (b basicCache) BatchGet(_ context.Context, keys []string) []string {
	vals := make([]string, len(keys))
	for i, k := range keys {
		vals[i] = b[k]
	}
	return vals
}

func cacheGetter(ctx context.Context, elems []*DoerElem) error {
	keys := make([]string, len(elems))
	for i, e := range elems {
		keys[i] = e.Input.(string)
	}

	vals := cache.BatchGet(ctx, keys)

	for i, e := range elems {
		e.Output = vals[i]
	}

	return nil
}

type kv struct {
	key string
	val string
}

func TestDoer(t *testing.T) {
	cache = make(map[string]string)

	ctx := context.Background()

	// First create our batch pipeline
	doer := NewDoer(cacheGetter)
	b := batch.New(batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: minItems, // process 10 at a time for this simple example
		MaxTime:  time.Millisecond * 10,
	}))

	// Set some random keys and values
	randoms := make([]*kv, randomKVs)

	rand.Seed(100)
	for i := 0; i < randomKVs; i++ {
		k := strconv.FormatInt(rand.Int63(), 10)
		v := strconv.FormatInt(rand.Int63(), 10)
		cache.Set(ctx, k, v)
		randoms[i] = &kv{
			key: k,
			val: v,
		}
	}

	b.Go(ctx, doer, doer)

	var wg sync.WaitGroup

	// Loop, getting random keys each time. Number of loops doesn't necessarily
	// have to be a multiple of minItems, because we've specified a max time
	for i := 0; i < minItems*50-5; /**100000-5*/ i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			r := rand.Intn(randomKVs)
			key := randoms[r].key
			val, err := doer.Do(ctx, key)
			if err != nil {
				panic(err)
			}

			if v, ok := val.(string); !ok || v != randoms[r].val {
				t.Fatalf("doer.Do(ctx, %v) was %v, want %v", key, v, randoms[r].val)
			}
		}()
	}

	wg.Wait()

	doer.Close()
	<-b.Done()
}

func assertDoer(t *testing.T, d *Doer) {
	if d == nil {
		t.Fatal("NewDoer(nil) == nil")
	}
	if !d.isSetup {
		t.Error("NewDoer(nil).isSetup == false")
	}
	if d.doerFunc == nil {
		t.Error("NewDoer(nil).doerFunc == nil")
	}
	if d.ch == nil {
		t.Error("NewDoer(nil).ch == nil")
	}
}

func TestNewDoer(t *testing.T) {
	t.Run("nil-func", func(t *testing.T) {
		t.Parallel()

		d := NewDoer(nil)
		assertDoer(t, d)
	})

	t.Run("with-func", func(t *testing.T) {
		t.Parallel()

		f := DoerFunc(func(ctx context.Context, elems []*DoerElem) error {
			return errors.New("yo")
		})

		d := NewDoer(f)
		assertDoer(t, d)
	})
}
