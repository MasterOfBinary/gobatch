// Package processor contains several implementations of the batch.Processor
// interface for common processing scenarios, including:
//
// - Error: For simulating errors with configurable failure rates
// - Filter: For filtering items based on custom predicates
// - Nil: For testing timing behavior without modifying items
// - Transform: For transforming item data values
// - Channel: For writing item data to an output channel
//
// Each processor implementation follows a consistent error handling pattern and
// respects context cancellation.
//
// Basic usage of the Transform processor:
//
//	p := &Transform{Func: func(v interface{}) (interface{}, error) {
//	    if n, ok := v.(int); ok {
//	        return n * 2, nil
//	    }
//	    return v, nil
//	}}
//
//	items := []*batch.Item{{Data: 1}, {Data: 2}}
//	res, _ := p.Process(context.Background(), items)
//	fmt.Println(res[0].Data, res[1].Data)
//
// Output:
//
//	2 4
package processor
