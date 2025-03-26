package batch

import "sync"

// MockItemGenerator generates mock Items with unique IDs. Items are generated in a
// separate goroutine and added to a channel, which can be retrieved by calling
// GetCh.
type MockItemGenerator struct {
	closeOnce sync.Once
	done      chan struct{}
	ch        chan *Item[int]
}

// NewMockItemGenerator returns a new MockItemGenerator.
//
// After using it, call Close to prevent a goroutine leak.
func NewMockItemGenerator() *MockItemGenerator {
	m := &MockItemGenerator{
		done: make(chan struct{}),
		ch:   make(chan *Item[int]),
	}

	go func() {
		id := uint64(0)
		nextItem := &Item[int]{
			id: id,
		}
		for {
			select {
			case m.ch <- nextItem:
				id++
				nextItem = &Item[int]{
					id: id,
				}

			case <-m.done:
				return
			}
		}
	}()

	return m
}

// Close stops a MockItemGenerator's goroutine
func (m *MockItemGenerator) Close() {
	m.closeOnce.Do(func() {
		close(m.done)
	})
}

// GetCh returns a channel of Items with unique IDs.
func (m *MockItemGenerator) GetCh() <-chan *Item[int] {
	return m.ch
}
