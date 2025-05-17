package processor_test

import (
	"context"
	"fmt"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
	"github.com/MasterOfBinary/gobatch/processor"
	"github.com/MasterOfBinary/gobatch/source"
)

// This example demonstrates how to use the ResultCollector processor to collect
// processed items from a batch operation.
func Example_resultCollector() {
	// Create a source with integer data
	ch := make(chan interface{})
	go func() {
		defer close(ch)
		for i := 1; i <= 10; i++ {
			ch <- i
			time.Sleep(10 * time.Millisecond)
		}
	}()
	src := &source.Channel{Input: ch}

	// Create a processor that squares the numbers
	squareProcessor := &processor.Transform{
		Func: func(data interface{}) (interface{}, error) {
			if num, ok := data.(int); ok {
				return num * num, nil
			}
			return data, nil
		},
	}

	// Create a filter processor that keeps only even numbers
	evenFilter := &processor.Filter{
		Predicate: func(item *batch.Item) bool {
			if num, ok := item.Data.(int); ok {
				return num%2 == 0
			}
			return false
		},
	}

	// Create a result collector
	collector := &processor.ResultCollector{}

	// Create a batch with simple configuration
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 2,
		MaxItems: 4,
	})
	b := batch.New(config)

	// Run the batch process
	ctx := context.Background()
	fmt.Println("Starting batch processing...")
	batch.RunBatchAndWait(ctx, b, src, squareProcessor, evenFilter, collector)

	// Get all the collected results (without resetting)
	fmt.Println("\nAll collected results:")
	for _, item := range collector.Results(false) {
		fmt.Printf("Item ID %d: %v\n", item.ID, item.Data)
	}

	// Extract only the integers using the type-safe ExtractData function
	// and reset the collector at the same time
	integers := processor.ExtractData[int](collector, true)
	fmt.Println("\nExtracted integers:")
	fmt.Println(integers)

	// Output:
	// Starting batch processing...
	//
	// All collected results:
	// Item ID 1: 4
	// Item ID 3: 16
	// Item ID 5: 36
	// Item ID 7: 64
	// Item ID 9: 100
	//
	// Extracted integers:
	// [4 16 36 64 100]
}

// This example shows how to use the ResultCollector with custom filter options
// to selectively collect items from the processing pipeline.
func Example_resultCollectorWithOptions() {
	// Create a source with string data
	ch := make(chan interface{})
	go func() {
		defer close(ch)
		words := []string{"hello", "world", "batch", "processing", "is", "awesome"}
		for _, word := range words {
			ch <- word
			time.Sleep(10 * time.Millisecond)
		}
	}()
	src := &source.Channel{Input: ch}

	// Create a processor that transforms strings to uppercase
	uppercaseProcessor := &processor.Transform{
		Func: func(data interface{}) (interface{}, error) {
			if str, ok := data.(string); ok {
				return fmt.Sprintf("[%s]", str), nil
			}
			return data, nil
		},
	}

	// Create a result collector that only collects items with length > 6
	collector := &processor.ResultCollector{
		Filter: func(item *batch.Item) bool {
			if str, ok := item.Data.(string); ok {
				return len(str) > 8
			}
			return false
		},
		MaxItems: 3, // Limit to 3 items
	}

	// Create a batch with simple configuration
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 1,
		MaxItems: 10,
	})
	b := batch.New(config)

	// Run the batch process
	ctx := context.Background()
	fmt.Println("Starting filtered collection...")
	batch.RunBatchAndWait(ctx, b, src, uppercaseProcessor, collector)

	// Get all the collected results (and reset the collector)
	fmt.Println("\nCollected items (length > 8, max 3 items):")
	for _, item := range collector.Results(true) {
		fmt.Printf("Item ID %d: %v\n", item.ID, item.Data)
	}

	// Output:
	// Starting filtered collection...
	//
	// Collected items (length > 8, max 3 items):
	// Item ID 3: [processing]
	// Item ID 5: [awesome]
}