package pipeline_test

import (
	"context"
	"math/rand"
	"strconv"

	"fmt"
	"sync"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
	"github.com/MasterOfBinary/gobatch/pipeline"
)

const (
	minItems  = 10
	randomKVs = 100
)

var cache BasicCache

type BasicCache map[string]string

func (b BasicCache) Set(ctx context.Context, key, value string) {
	b[key] = value
}

func (b BasicCache) BatchGet(ctx context.Context, keys []string) []string {
	vals := make([]string, len(keys))
	for i, k := range keys {
		vals[i] = b[k]
	}
	return vals
}

func CacheGetter(ctx context.Context, es []*pipeline.DoerElem) error {
	keys := make([]string, len(es))
	for i, e := range es {
		keys[i] = e.Input.(string)
	}

	vals := cache.BatchGet(ctx, keys)

	for i, e := range es {
		e.Output = vals[i]
	}

	return nil
}

type kv struct {
	key string
	val string
}

// ExampleDoer shows how Doer can be used to batch together cache calls.
func ExampleDoer() {
	cache = make(map[string]string)

	ctx := context.Background()

	// First create our batch pipeline
	doer := pipeline.NewDoer(CacheGetter)
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
	for i := 0; i < minItems; /**100000-5*/ i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			r := rand.Intn(randomKVs)
			val, err := doer.Do(ctx, randoms[r].key)
			if err != nil {
				panic(err)
			}

			if v, ok := val.(string); !ok || v != randoms[r].val {
				fmt.Printf("Unexpected value %v returned for key %v\n", v, randoms[r].key)
			}
		}()
	}

	wg.Wait()

	fmt.Println("Finished")

	// Output:
	// Finished
}
