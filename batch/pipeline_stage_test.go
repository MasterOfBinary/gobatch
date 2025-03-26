package batch

import "testing"

func TestPipelineStage_Close(t *testing.T) {
	out := make(chan *Item[any])
	errs := make(chan error)
	ps := &PipelineStage[any, any]{
		Output: out,
		Errors: errs,
	}

	ps.Close()

	select {
	case <-out:
	default:
		t.Error("ps.Output was not closed")
	}

	select {
	case <-errs:
	default:
		t.Error("ps.Errors was not closed")
	}
}
