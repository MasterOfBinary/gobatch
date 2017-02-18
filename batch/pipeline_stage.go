package batch

// PipelineStage contains the input and output channels for a single
// stage of the batch pipeline.
type PipelineStage struct {
	// Input contains the input items for a pipeline stage.
	Input <-chan *Item

	// Output is for the output of the pipeline stage.
	Output chan<- *Item

	//Retry chan<- *Item

	// Error is for any errors encountered during the pipeline stage.
	Errors chan<- error
}

// Close closes the pipeline stage.
//
// Note that it will also close the write channels. Do not close them separately
// or it will panic.
func (p *PipelineStage) Close() {
	close(p.Output)
	close(p.Errors)
}
