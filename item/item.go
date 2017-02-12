package item

// Item is the interface for an item in the batch pipeline.
type Item interface {
	// GetID returns the ID of the item. It is unique among all items
	// that have been added to the pipeline.
	GetID() uint64

	// Get returns the item.
	Get() interface{}

	// Set sets the item.
	Set(item interface{})
}
