package batch_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
)

type textSource struct {
	texts []string
}

func (s *textSource) Read(ctx context.Context) (<-chan interface{}, <-chan error) {
	out := make(chan interface{})
	errs := make(chan error)

	go func() {
		defer close(out)
		defer close(errs)
		for _, text := range s.texts {
			select {
			case <-ctx.Done():
				return
			case out <- text:
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()

	return out, errs
}

type validateProcessor struct {
	minLength int
	maxLength int
}

func (p *validateProcessor) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	fmt.Println("Validation:")
	for i, item := range items {
		if item.Error != nil {
			continue
		}
		if text, ok := item.Data.(string); ok {
			switch {
			case len(text) < p.minLength:
				item.Error = fmt.Errorf("too short (min %d)", p.minLength)
			case len(text) > p.maxLength:
				item.Error = fmt.Errorf("too long (max %d)", p.maxLength)
			}
		} else {
			item.Error = errors.New("not a string")
		}
		status := "ok"
		if item.Error != nil {
			status = "error: " + item.Error.Error()
		}
		fmt.Printf("  Item %d: %v (%s)\n", i, item.Data, status)
	}
	return items, nil
}

type formatProcessor struct{}

func (p *formatProcessor) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	fmt.Println("Format:")
	for i, item := range items {
		if item.Error != nil {
			continue
		}
		if text, ok := item.Data.(string); ok {
			item.Data = "[" + strings.ToUpper(text) + "]"
			fmt.Printf("  Item %d: formatted to %v\n", i, item.Data)
		}
	}
	return items, nil
}

type enrichProcessor struct {
	metadata map[string]string
}

func (p *enrichProcessor) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	fmt.Println("Enrich:")
	for i, item := range items {
		if item.Error != nil {
			continue
		}
		if text, ok := item.Data.(string); ok {
			key := strings.ToLower(strings.Trim(text, "[]"))
			if meta, exists := p.metadata[key]; exists {
				item.Data = struct {
					Text     string
					Metadata string
				}{
					Text:     text,
					Metadata: meta,
				}
				fmt.Printf("  Item %d: enriched with %q\n", i, meta)
			} else {
				fmt.Printf("  Item %d: no metadata\n", i)
			}
		}
	}
	return items, nil
}

type displayProcessor struct{}

func (p *displayProcessor) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	fmt.Println("Results:")
	for i, item := range items {
		if item.Error != nil {
			fmt.Printf("  Item %d: ERROR: %v\n", i, item.Error)
		} else {
			fmt.Printf("  Item %d: OK: %v\n", i, item.Data)
		}
	}
	return items, nil
}

func Example_processorChain() {
	src := &textSource{
		texts: []string{
			"hello",
			"a", // too short
			"world",
			"processing",
			"thisisaverylongstringthatwillexceedthemaximumlength",
			"batch",
		},
	}

	meta := map[string]string{
		"hello":      "English greeting",
		"world":      "Planet Earth",
		"processing": "Act of handling data",
		"batch":      "Group of items processed together",
	}

	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 4,
		MaxItems: 3,
	})
	b := batch.New(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fmt.Println("=== Processor Chain Example ===")

	errs := batch.RunBatchAndWait(ctx, b, src,
		&validateProcessor{minLength: 3, maxLength: 15},
		&formatProcessor{},
		&enrichProcessor{metadata: meta},
		&displayProcessor{},
	)

	if len(errs) > 0 {
		fmt.Printf("Total errors: %d\n", len(errs))
	}

	// Output:
	// === Processor Chain Example ===
	// Validation:
	//   Item 0: hello (ok)
	//   Item 1: a (error: too short (min 3))
	//   Item 2: world (ok)
	// Format:
	//   Item 0: formatted to [HELLO]
	//   Item 2: formatted to [WORLD]
	// Enrich:
	//   Item 0: enriched with "English greeting"
	//   Item 2: enriched with "Planet Earth"
	// Results:
	//   Item 0: OK: {[HELLO] English greeting}
	//   Item 1: ERROR: too short (min 3)
	//   Item 2: OK: {[WORLD] Planet Earth}
	// Validation:
	//   Item 0: processing (ok)
	//   Item 1: thisisaverylongstringthatwillexceedthemaximumlength (error: too long (max 15))
	//   Item 2: batch (ok)
	// Format:
	//   Item 0: formatted to [PROCESSING]
	//   Item 2: formatted to [BATCH]
	// Enrich:
	//   Item 0: enriched with "Act of handling data"
	//   Item 2: enriched with "Group of items processed together"
	// Results:
	//   Item 0: OK: {[PROCESSING] Act of handling data}
	//   Item 1: ERROR: too long (max 15)
	//   Item 2: OK: {[BATCH] Group of items processed together}
	// Total errors: 2
}
