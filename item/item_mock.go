package item

import "sync"

// MockGenerator generates mock Items with unique IDs. Items are generated in a
// separate goroutine and added to a channel, which can be retrieved by calling
// GetCh.
type MockGenerator struct {
	closeOnce sync.Once
	done      chan struct{}
	ch        chan Item

	mu     sync.Mutex
	nextID uint64
}

// NewMockGenerator returns a new MockGenerator.
//
// After using it, call Close to prevent a goroutine leak.
func NewMockGenerator() *MockGenerator {
	m := &MockGenerator{
		done: make(chan struct{}),
		ch:   make(chan Item),
	}

	go func() {
		id := uint64(0)
		nextItem := &mockItemImpl{
			id: id,
		}
		for {
			select {
			case m.ch <- nextItem:
				id++
				nextItem = &mockItemImpl{
					id: id,
				}

			case <-m.done:
				return
			}
		}
	}()

	return m
}

// Close stops a MockGenerator's goroutine
func (m *MockGenerator) Close() {
	m.closeOnce.Do(func() {
		close(m.done)
	})
}

// GetCh returns a channel of Items with unique IDs.
func (m *MockGenerator) GetCh() <-chan Item {
	return m.ch
}

// mockItemImpl is an implementation of the source.Item interface that can
// be used for mocking.
type mockItemImpl struct {
	id   uint64
	item interface{}
}

// GetID implements the Item interface.
func (i *mockItemImpl) GetID() uint64 {
	return i.id
}

// Get implements the Item interface.
func (i *mockItemImpl) Get() interface{} {
	return i.item
}

// Set implements the Item interface.
func (i *mockItemImpl) Set(item interface{}) {
	i.item = item
}
