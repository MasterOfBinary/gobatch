// Package source contains several implementations of the batch.Source
// interface for common data source scenarios, including:
//
//   - Channel: For using existing channels as batch sources
//   - Error: For simulating error-only sources without data
//   - Nil: For testing timing behavior without emitting data
//
// Each source implementation handles context cancellation properly and
// ensures channels are closed appropriately.
//
// Basic usage of the Channel source:
//
//	input := make(chan int, 2)
//	input <- 1
//	input <- 2
//	close(input)
//
//	src := &Channel[int]{Input: input}
//	out, errs := src.Read(context.Background())
//	for item := range out {
//	    fmt.Println(item)
//	}
//	for range errs {
//	}
//
// Output:
//
//	1
//	2
package source
