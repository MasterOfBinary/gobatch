package batch_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
)

type errorSource struct {
	items     []int
	errorRate int
}

func (s *errorSource) Read(ctx context.Context) (<-chan interface{}, <-chan error) {
	out := make(chan interface{})
	errs := make(chan error)

	go func() {
		defer close(out)
		defer close(errs)

		for i, val := range s.items {
			select {
			case <-ctx.Done():
				errs <- fmt.Errorf("source interrupted: %w", ctx.Err())
				return
			default:
				if s.errorRate > 0 && i > 0 && i%s.errorRate == 0 {
					errs <- fmt.Errorf("source error at item %d", i)
					time.Sleep(10 * time.Millisecond)
					continue
				}
				out <- val
				time.Sleep(5 * time.Millisecond)
			}
		}
	}()

	return out, errs
}

type validationProcessor struct {
	maxValue int
}

func (p *validationProcessor) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	for _, item := range items {
		if item.Error != nil {
			continue
		}
		select {
		case <-ctx.Done():
			return items, ctx.Err()
		default:
		}

		if num, ok := item.Data.(int); ok {
			if num > p.maxValue {
				item.Error = fmt.Errorf("value %d exceeds maximum %d", num, p.maxValue)
			}
		} else {
			item.Error = errors.New("expected int")
		}
	}
	return items, nil
}

type errorProneProcessor struct {
	failOnBatch int
	batchCount  int32
}

func (p *errorProneProcessor) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	batchNum := atomic.AddInt32(&p.batchCount, 1)

	if int(batchNum) == p.failOnBatch {
		return items, fmt.Errorf("processor failed on batch %d", batchNum)
	}

	for _, item := range items {
		if item.Error != nil {
			continue
		}
		select {
		case <-ctx.Done():
			return items, ctx.Err()
		default:
		}

		if num, ok := item.Data.(int); ok {
			item.Data = "Item: " + strconv.Itoa(num)
		}
	}

	return items, nil
}

type errorLogger struct{}

func (p *errorLogger) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	fmt.Println("Batch:")
	errorCount := 0

	for _, item := range items {
		if item.Error != nil {
			fmt.Printf("- Item %d error: %v\n", item.ID, item.Error)
			errorCount++
		} else {
			fmt.Printf("- Item %d: %v\n", item.ID, item.Data)
		}
	}

	if errorCount > 0 {
		fmt.Printf("Batch had %d error(s)\n", errorCount)
	}

	return items, nil
}

func Example_errorHandling() {
	src := &errorSource{
		items:     []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 15, 20, 25},
		errorRate: 5,
	}

	validator := &validationProcessor{maxValue: 10}
	transformer := &errorProneProcessor{failOnBatch: 2}
	logger := &errorLogger{}

	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 3,
		MaxItems: 5,
	})
	b := batch.New(config)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fmt.Println("=== Error Handling Example ===")

	errs := batch.RunBatchAndWait(ctx, b, src, validator, transformer, logger)

	fmt.Println("\nSummary:")
	if len(errs) == 0 {
		fmt.Println("No errors")
	} else {
		for i, err := range errs {
			var srcErr *batch.SourceError
			var procErr *batch.ProcessorError

			switch {
			case errors.As(err, &srcErr):
				fmt.Printf("%d. Source error: %v\n", i+1, srcErr.Unwrap())
			case errors.As(err, &procErr):
				fmt.Printf("%d. Processor error: %v\n", i+1, procErr.Unwrap())
			default:
				fmt.Printf("%d. Other error: %v\n", i+1, err)
			}
		}
	}

	// Output:
	// === Error Handling Example ===
	// Batch:
	// - Item 0: Item: 1
	// - Item 1: Item: 2
	// - Item 2: Item: 3
	// Batch:
	// - Item 3: 4
	// - Item 4: 5
	// - Item 5: 7
	// Batch:
	// - Item 6: Item: 8
	// - Item 7: Item: 9
	// - Item 8: Item: 10
	// Batch:
	// - Item 9 error: value 12 exceeds maximum 10
	// - Item 10 error: value 15 exceeds maximum 10
	// - Item 11 error: value 20 exceeds maximum 10
	// Batch had 3 error(s)
	// Batch:
	// - Item 12 error: value 25 exceeds maximum 10
	// Batch had 1 error(s)
	//
	// Summary:
	// 1. Source error: source error at item 5
	// 2. Processor error: processor failed on batch 2
	// 3. Source error: source error at item 10
	// 4. Processor error: value 12 exceeds maximum 10
	// 5. Processor error: value 15 exceeds maximum 10
	// 6. Processor error: value 20 exceeds maximum 10
	// 7. Processor error: value 25 exceeds maximum 10
}
