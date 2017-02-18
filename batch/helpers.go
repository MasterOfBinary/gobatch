package batch

// NextItem retrieves the next source item from the source channel, sets its data,
// and returns it. It can be used in the Source.Read function:
//
//    func (s *source) Read(ctx context.Context, in <-chan *batch.Item, items chan<- *batch.Item, errs chan<- error) {
//      // Read data into myData...
//      items <- batch.NextItem(in, myData)
//      // ...
//    }
func NextItem(ps PipelineStage, data interface{}) *Item {
	i := <-ps.Input()
	i.Set(data)
	return i
}
