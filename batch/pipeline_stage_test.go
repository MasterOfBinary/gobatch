package batch

import "testing"

func TestPipelineStage_Close(t *testing.T) {
	out := make(chan *Item)
	errs := make(chan error)
	ps := &PipelineStage{
		Output: out,
		Error:  errs,
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
		t.Error("ps.Error was not closed")
	}
}
