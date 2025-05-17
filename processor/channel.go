package processor

import (
    "context"

    "github.com/MasterOfBinary/gobatch/batch"
)

// Channel is a Processor that sends the Data field of each item to an output
// channel. Items with existing errors are ignored. The channel is not closed by
// the processor.
type Channel struct {
    // Output is the channel that receives each item's Data value.
    // If nil, the processor does nothing.
    Output chan<- interface{}
}

// Process implements the Processor interface by forwarding item data to the
// Output channel until the context is canceled.
func (p *Channel) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
    if len(items) == 0 || p.Output == nil {
        return items, nil
    }

    for _, item := range items {
        if item.Error != nil {
            continue
        }

        select {
        case <-ctx.Done():
            return items, ctx.Err()
        case p.Output <- item.Data:
        }
    }

    return items, nil
}

