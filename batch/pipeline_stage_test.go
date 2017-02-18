package batch

import "testing"

func TestPipelineStage_Close(t *testing.T) {
	ps := &pipelineStage{
		out: make(chan *Item),
		err: make(chan error),
	}

	ps.Close()

	select {
	case <-ps.out:
	default:
		t.Error("ps.out was not closed")
	}

	select {
	case <-ps.err:
	default:
		t.Error("ps.err was not closed")
	}
}
