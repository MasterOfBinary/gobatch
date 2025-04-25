package batch_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
)

// textSource generates sample text data for processing
type textSource struct {
	texts []string
}

// Read implements the Source interface by sending text items to the output channel
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
				// Text sent successfully
				time.Sleep(time.Millisecond * 10) // Consistent 10ms delay for deterministic batching
			}
		}
	}()

	return out, errs
}

// validateProcessor validates text items and marks invalid ones with errors
type validateProcessor struct {
	minLength int
	maxLength int
}

// Process implements the Processor interface by validating text items
func (p *validateProcessor) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	fmt.Println("1. VALIDATION STAGE:")
	fmt.Printf("   Processing batch of %d items\n", len(items))

	for i, item := range items {
		// Skip items that already have errors
		if item.Error != nil {
			continue
		}

		// Process only string data
		if text, ok := item.Data.(string); ok {
			fmt.Printf("   - Item %d: %q", i, text)

			if len(text) < p.minLength {
				item.Error = fmt.Errorf("text too short (min: %d)", p.minLength)
				fmt.Printf(" → FAILED: %v\n", item.Error)
			} else if len(text) > p.maxLength {
				item.Error = fmt.Errorf("text too long (max: %d)", p.maxLength)
				fmt.Printf(" → FAILED: %v\n", item.Error)
			} else {
				fmt.Printf(" → VALID\n")
			}
		} else {
			item.Error = errors.New("not a string")
			fmt.Printf(" → FAILED: %v\n", item.Error)
		}
	}

	return items, nil
}

// formatProcessor transforms text items (uppercase and bracket addition)
type formatProcessor struct{}

// Process implements the Processor interface by transforming text items
func (p *formatProcessor) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	fmt.Println("2. TRANSFORMATION STAGE:")
	fmt.Printf("   Processing batch of %d items\n", len(items))

	validCount := 0
	for i, item := range items {
		// Skip items that already have errors
		if item.Error != nil {
			fmt.Printf("   - Item %d: Skipped (has error)\n", i)
			continue
		}

		// Process only string data
		if text, ok := item.Data.(string); ok {
			// Apply transformations: uppercase and add brackets
			transformed := "[" + strings.ToUpper(text) + "]"
			item.Data = transformed

			fmt.Printf("   - Item %d: %q → %q\n", i, text, transformed)
			validCount++
		}
	}
	fmt.Printf("   Transformed %d items\n", validCount)

	return items, nil
}

// enrichProcessor adds metadata to text items
type enrichProcessor struct {
	metadata map[string]string
}

// Process implements the Processor interface by enriching items with metadata
func (p *enrichProcessor) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	fmt.Println("3. ENRICHMENT STAGE:")
	fmt.Printf("   Processing batch of %d items\n", len(items))

	enrichedCount := 0
	for i, item := range items {
		// Skip items that already have errors
		if item.Error != nil {
			fmt.Printf("   - Item %d: Skipped (has error)\n", i)
			continue
		}

		// Process only string data
		if text, ok := item.Data.(string); ok {
			// Get the original text (remove brackets and lowercase)
			originalText := strings.ToLower(strings.Trim(text, "[]"))

			fmt.Printf("   - Item %d: %q", i, text)

			// Check if we have metadata for this text
			if metadata, exists := p.metadata[originalText]; exists {
				// Create a struct with the text and metadata
				item.Data = struct {
					Text     string
					Metadata string
				}{
					Text:     text,
					Metadata: metadata,
				}
				fmt.Printf(" → Added metadata: %q\n", metadata)
				enrichedCount++
			} else {
				fmt.Printf(" → No metadata available\n")
			}
		}
	}
	fmt.Printf("   Enriched %d items\n", enrichedCount)

	return items, nil
}

// displayProcessor outputs the final processed items
type displayProcessor struct{}

// Process implements the Processor interface by displaying processed items
func (p *displayProcessor) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	fmt.Println("4. RESULTS STAGE:")
	fmt.Printf("   Processing batch of %d items\n", len(items))

	successCount := 0
	errorCount := 0

	for i, item := range items {
		if item.Error != nil {
			fmt.Printf("   - Item %d: ERROR: %v\n", i, item.Error)
			errorCount++
		} else {
			fmt.Printf("   - Item %d: SUCCESS: %v\n", i, item.Data)
			successCount++
		}
	}

	fmt.Printf("   BATCH SUMMARY: %d successful, %d failed\n", successCount, errorCount)
	return items, nil
}

func Example_processorChain() {
	// Create a source with text data
	source := &textSource{
		texts: []string{
			"hello",
			"a", // too short
			"world",
			"processing",
			"thisisaverylongstringthatwillexceedthemaximumlength", // too long
			"batch",
		},
	}

	// Create metadata for enrichment
	metadata := map[string]string{
		"hello":      "English greeting",
		"world":      "Planet Earth",
		"processing": "Act of handling data",
		"batch":      "Group of items processed together",
	}

	// Create processors for each stage of the pipeline
	validator := &validateProcessor{minLength: 3, maxLength: 15}
	formatter := &formatProcessor{}
	enricher := &enrichProcessor{metadata: metadata}
	display := &displayProcessor{}

	// Create batch processor with configuration that demonstrates MaxItems has priority
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 4, // This would collect more items if it had higher priority
		MaxItems: 3, // This takes precedence over MinItems
	})
	b := batch.New(config)

	// Create context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fmt.Println("MULTI-STAGE PROCESSOR CHAIN EXAMPLE")
	fmt.Println("===================================")
	fmt.Println("This example demonstrates a four-stage processing pipeline:")
	fmt.Println("1. Validation: Checks text length requirements")
	fmt.Println("2. Transformation: Converts text to uppercase with brackets")
	fmt.Println("3. Enrichment: Adds metadata to valid items")
	fmt.Println("4. Results: Displays final processed items")
	fmt.Println("===================================")

	// Start the batch processing with processor chain
	errs := batch.RunBatchAndWait(ctx, b, source,
		validator, // Stage 1: Validate input
		formatter, // Stage 2: Transform valid items
		enricher,  // Stage 3: Add metadata to items
		display)   // Stage 4: Display results

	fmt.Println("===================================")
	fmt.Println("Processing complete")

	if len(errs) > 0 {
		fmt.Printf("Found %d errors in total\n", len(errs))
	}

	// Output:
	// MULTI-STAGE PROCESSOR CHAIN EXAMPLE
	// ===================================
	// This example demonstrates a four-stage processing pipeline:
	// 1. Validation: Checks text length requirements
	// 2. Transformation: Converts text to uppercase with brackets
	// 3. Enrichment: Adds metadata to valid items
	// 4. Results: Displays final processed items
	// ===================================
	// 1. VALIDATION STAGE:
	//    Processing batch of 3 items
	//    - Item 0: "hello" → VALID
	//    - Item 1: "a" → FAILED: text too short (min: 3)
	//    - Item 2: "world" → VALID
	// 2. TRANSFORMATION STAGE:
	//    Processing batch of 3 items
	//    - Item 0: "hello" → "[HELLO]"
	//    - Item 1: Skipped (has error)
	//    - Item 2: "world" → "[WORLD]"
	//    Transformed 2 items
	// 3. ENRICHMENT STAGE:
	//    Processing batch of 3 items
	//    - Item 0: "[HELLO]" → Added metadata: "English greeting"
	//    - Item 1: Skipped (has error)
	//    - Item 2: "[WORLD]" → Added metadata: "Planet Earth"
	//    Enriched 2 items
	// 4. RESULTS STAGE:
	//    Processing batch of 3 items
	//    - Item 0: SUCCESS: {[HELLO] English greeting}
	//    - Item 1: ERROR: text too short (min: 3)
	//    - Item 2: SUCCESS: {[WORLD] Planet Earth}
	//    BATCH SUMMARY: 2 successful, 1 failed
	// 1. VALIDATION STAGE:
	//    Processing batch of 3 items
	//    - Item 0: "processing" → VALID
	//    - Item 1: "thisisaverylongstringthatwillexceedthemaximumlength" → FAILED: text too long (max: 15)
	//    - Item 2: "batch" → VALID
	// 2. TRANSFORMATION STAGE:
	//    Processing batch of 3 items
	//    - Item 0: "processing" → "[PROCESSING]"
	//    - Item 1: Skipped (has error)
	//    - Item 2: "batch" → "[BATCH]"
	//    Transformed 2 items
	// 3. ENRICHMENT STAGE:
	//    Processing batch of 3 items
	//    - Item 0: "[PROCESSING]" → Added metadata: "Act of handling data"
	//    - Item 1: Skipped (has error)
	//    - Item 2: "[BATCH]" → Added metadata: "Group of items processed together"
	//    Enriched 2 items
	// 4. RESULTS STAGE:
	//    Processing batch of 3 items
	//    - Item 0: SUCCESS: {[PROCESSING] Act of handling data}
	//    - Item 1: ERROR: text too long (max: 15)
	//    - Item 2: SUCCESS: {[BATCH] Group of items processed together}
	//    BATCH SUMMARY: 2 successful, 1 failed
	// ===================================
	// Processing complete
	// Found 2 errors in total
}
