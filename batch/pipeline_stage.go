package batch

type PipelineStage interface {
	Input() <-chan *Item
	Output() chan<- *Item
	//Retry() chan<- *Item
	Error() chan<- error

	Close()
}

type pipelineStage struct {
	in  chan *Item
	out chan *Item
	err chan error

	closed bool
}

func (p *pipelineStage) Input() <-chan *Item {
	return p.in
}

func (p *pipelineStage) Output() chan<- *Item {
	return p.out
}

func (p *pipelineStage) Error() chan<- error {
	return p.err
}

func (p *pipelineStage) Close() {
	close(p.out)
	close(p.err)
}
