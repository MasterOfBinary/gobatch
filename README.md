GoBatch
=======

[![Build Status](https://travis-ci.org/MasterOfBinary/gobatch.svg?branch=master)](https://travis-ci.org/MasterOfBinary/gobatch)
[![Coverage Status](https://coveralls.io/repos/github/MasterOfBinary/gobatch/badge.svg?branch=master)](https://coveralls.io/github/MasterOfBinary/gobatch?branch=master)
[![GoDoc](https://godoc.org/github.com/MasterOfBinary/gobatch?status.svg)](https://godoc.org/github.com/MasterOfBinary/gobatch)
[![BADGINATOR](https://badginator.herokuapp.com/MasterOfBinary/gobatch.svg)](https://github.com/defunctzombie/badginator)

GoBatch is a batch processing library for Go. By using interfaces for the
reader (`source.Source`) and processor (`processor.Processor`), the actual
data input and processing of a batch of items is done by the user, while the
`Batch` structure provided by the GoBatch library handles the rest of the
pipeline.

The batch pipeline consists of several stages:

1. Reading from the source, which could be a channel, disk, Redis, or pretty
much anywhere. All that's needed is a `Source` implementation that knows how to
do it.
2. The data is written to an interface channel and passed to `Batch`, which queues
the items and prepares them for processing. It decides how many items to batch
together based on its configuration.
3. `Batch` sends the items in batches, as an interface slice, to the `Processor`
which does whatever is necessary with the data.

Features
--------

* Complete control over the number of items to process at once.
* Errors are returned over a channel, so that they can be logged or otherwise
handled.
* Channels are used throughout the library, not just for errors. Everything is
(or can be, depending on its config) highly concurrent.

Documentation
-------------

See the [GoDocs](https://godoc.org/github.com/MasterOfBinary/gobatch).

Installation
------------

To download, run

    go get github.com/MasterOfBinary/gobatch

GoBatch doesn't require any dependencies except Go 1.7 and its standard library.

Example
-------

The [GoDocs](https://godoc.org/github.com/MasterOfBinary/gobatch) provide a different
example that includes error handling and a `Source` provided by GoBatch. This one is
the bare minimum to get GoBatch to work with a custom `Source` and `Processor`.

```go
package main

import (
	"context"
	"fmt"

	"github.com/MasterOfBinary/gobatch"
)

// oneItemSource implements the source.Source interface.
type oneItemSource struct{}

// Read reads a single item and finishes.
func (s *oneItemSource) Read(ctx context.Context, items chan<- interface{}, errs chan<- error) {
	items <- 1

	// Read needs to close the channels after it's done
	close(items)
	close(errs)
}

// printProcessor implements the processor.Processor interface.
type printProcessor struct{}

// Process processes items in batches (although in this example it's batches
// of 1 item).
func (p printProcessor) Process(ctx context.Context, items []interface{}, errs chan<- error) {
	// This processor prints all the items in a line
	fmt.Println(items)

	// Process needs to close the error channel after it's done
	close(errs)
}

func main() {
	// Use default config
	b := gobatch.New(nil, 1)
	p := &printProcessor{}
	s := &oneItemSource{}

	// Go runs in the background while the main goroutine processes errors. IgnoreErrors
	// reads the errors from the error channel and just ignores them.
	gobatch.IgnoreErrors(b.Go(context.Background(), s, p))

	// Wait for it to finish
	<-b.Done()

	fmt.Println("Finished processing.")
}
```

Output:

```
[1]
Finished processing.
```

License
-------

GoBatch is provided under the MIT licence. See LICENSE for more details.