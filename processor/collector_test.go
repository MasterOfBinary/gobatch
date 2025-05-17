package processor

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/MasterOfBinary/gobatch/batch"
)

func TestResultCollector_Process(t *testing.T) {
	items := []*batch.Item{
		{ID: 1, Data: "test1"},
		{ID: 2, Data: "test2"},
		{ID: 3, Data: "test3", Error: errors.New("error3")},
		{ID: 4, Data: "test4"},
		{ID: 5, Data: 42}, // Different type
	}

	tests := []struct {
		name          string
		collector     *ResultCollector
		expectedCount int
		expectedData  []interface{}
	}{
		{
			name:          "default settings",
			collector:     &ResultCollector{},
			expectedCount: 4,
			expectedData:  []interface{}{"test1", "test2", "test4", 42},
		},
		{
			name:          "collect errors",
			collector:     &ResultCollector{CollectErrors: true},
			expectedCount: 5,
			expectedData:  []interface{}{"test1", "test2", "test3", "test4", 42},
		},
		{
			name: "with filter",
			collector: &ResultCollector{
				Filter: func(item *batch.Item) bool {
					// Only collect even IDs
					return item.ID%2 == 0
				},
			},
			expectedCount: 2,
			expectedData:  []interface{}{"test2", "test4"},
		},
		{
			name: "max items",
			collector: &ResultCollector{
				MaxItems: 2,
			},
			expectedCount: 2,
			expectedData:  []interface{}{"test1", "test2"},
		},
		{
			name: "filter with type check",
			collector: &ResultCollector{
				Filter: func(item *batch.Item) bool {
					_, isString := item.Data.(string)
					return isString
				},
				MaxItems:      10,
				CollectErrors: true,
			},
			expectedCount: 4,
			expectedData:  []interface{}{"test1", "test2", "test3", "test4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Process the items
			result, err := tt.collector.Process(context.Background(), items)
			
			// Check that the returned error is nil
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			
			// Check that the returned items are the same as the input
			if !reflect.DeepEqual(result, items) {
				t.Errorf("Process() items = %v, want %v", result, items)
			}
			
			// Check the number of collected items
			if count := tt.collector.Count(); count != tt.expectedCount {
				t.Errorf("Count() = %v, want %v", count, tt.expectedCount)
			}
			
			// Check the collected items
			collected := tt.collector.Results()
			if len(collected) != tt.expectedCount {
				t.Errorf("Results() returned %d items, want %d", len(collected), tt.expectedCount)
			}
			
			// Check the content of collected items
			dataList := make([]interface{}, 0, len(collected))
			for _, item := range collected {
				if s, ok := item.Data.(string); ok {
					dataList = append(dataList, s)
				} else if n, ok := item.Data.(int); ok {
					dataList = append(dataList, n)
				}
			}
			
			if !reflect.DeepEqual(dataList, tt.expectedData) {
				t.Errorf("Results data = %v, want %v", dataList, tt.expectedData)
			}
		})
	}
}

func TestResultCollector_Reset(t *testing.T) {
	items := []*batch.Item{
		{ID: 1, Data: "test1"},
		{ID: 2, Data: "test2"},
	}
	
	collector := &ResultCollector{}
	
	// Process items
	collector.Process(context.Background(), items)
	
	// Verify items were collected
	if count := collector.Count(); count != 2 {
		t.Errorf("Initial count = %d, want 2", count)
	}
	
	// Reset the collector
	collector.Reset()
	
	// Verify no items remain
	if count := collector.Count(); count != 0 {
		t.Errorf("Count after reset = %d, want 0", count)
	}
	
	if results := collector.Results(); len(results) != 0 {
		t.Errorf("Results after reset = %v, want empty slice", results)
	}
}

func TestResultCollector_Concurrency(t *testing.T) {
	collector := &ResultCollector{}
	var wg sync.WaitGroup
	
	// Number of concurrent goroutines and items per goroutine
	goroutines := 10
	itemsPerGoroutine := 100
	
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(routineID int) {
			defer wg.Done()
			
			items := make([]*batch.Item, itemsPerGoroutine)
			for j := 0; j < itemsPerGoroutine; j++ {
				items[j] = &batch.Item{
					ID:   uint64(routineID*itemsPerGoroutine + j),
					Data: routineID*itemsPerGoroutine + j,
				}
			}
			
			_, _ = collector.Process(context.Background(), items)
		}(i)
	}
	
	wg.Wait()
	
	// Verify all items were collected
	expectedCount := goroutines * itemsPerGoroutine
	if count := collector.Count(); count != expectedCount {
		t.Errorf("Count after concurrent processing = %d, want %d", count, expectedCount)
	}
}

func TestExtractData(t *testing.T) {
	collector := &ResultCollector{}
	
	items := []*batch.Item{
		{ID: 1, Data: "string1"},
		{ID: 2, Data: 42},
		{ID: 3, Data: "string2"},
		{ID: 4, Data: 3.14},
		{ID: 5, Data: "string3", Error: errors.New("error")},
		{ID: 6, Data: true},
	}
	
	// Process with collector that includes errors
	collector.CollectErrors = true
	collector.Process(context.Background(), items)
	
	// Extract strings
	strings := ExtractData[string](collector)
	expectedStrings := []string{"string1", "string2"}
	if !reflect.DeepEqual(strings, expectedStrings) {
		t.Errorf("ExtractData[string] = %v, want %v", strings, expectedStrings)
	}
	
	// Extract ints
	ints := ExtractData[int](collector)
	expectedInts := []int{42}
	if !reflect.DeepEqual(ints, expectedInts) {
		t.Errorf("ExtractData[int] = %v, want %v", ints, expectedInts)
	}
	
	// Extract floats
	floats := ExtractData[float64](collector)
	expectedFloats := []float64{3.14}
	if !reflect.DeepEqual(floats, expectedFloats) {
		t.Errorf("ExtractData[float64] = %v, want %v", floats, expectedFloats)
	}
	
	// Extract bools
	bools := ExtractData[bool](collector)
	expectedBools := []bool{true}
	if !reflect.DeepEqual(bools, expectedBools) {
		t.Errorf("ExtractData[bool] = %v, want %v", bools, expectedBools)
	}
}

func TestResultCollector_ItemCopying(t *testing.T) {
	// Create a collector
	collector := &ResultCollector{}
	
	// Create test items
	items := []*batch.Item{
		{ID: 1, Data: "test1"},
	}
	
	// Process the items
	collector.Process(context.Background(), items)
	
	// Get the collected results
	results := collector.Results()
	
	// Modify the original items
	items[0].Data = "modified"
	
	// Verify that the collected items were not affected
	if results[0].Data != "test1" {
		t.Errorf("Collected item was modified, got %v, want %v", results[0].Data, "test1")
	}
	
	// Modify the results
	results[0].Data = "modified result"
	
	// Get a fresh copy of the results
	internalResults := collector.Results()
	
	// Verify that modifying the result doesn't affect the internal collection
	if internalResults[0].Data != "test1" {
		t.Errorf("Internal collection was modified, got %v, want %v", internalResults[0].Data, "test1")
	}
	
	// Verify that the two result slices are distinct
	if reflect.ValueOf(results[0]).Pointer() == reflect.ValueOf(internalResults[0]).Pointer() {
		t.Error("Results() did not return a deep copy of items")
	}
}