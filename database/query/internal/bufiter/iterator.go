// SPDX-License-Identifier: ice License 1.0

package bufiter

import (
	"iter"
)

type (
	Option[T any] func(*prebufferedIterator[T])

	prebufferedIterator[T any] struct {
		BufferSize int
		Buffer     []*T
		Next       func() (*T, error, bool)
		Map        func(*T) *T
		Stop       func()
	}
)

func (it *prebufferedIterator[T]) consumeBuffer() error {
	// Reset buffer length to 0 but keep capacity.
	it.Buffer = it.Buffer[:0]

	for i := 0; i < it.BufferSize; i++ {
		item, err, ok := it.Next()
		if err != nil {
			return err
		}

		if !ok {
			// End of iteration.
			// Release the iterator resources as early as possible.
			it.Stop()
			break
		}

		it.Buffer = append(it.Buffer, item)
	}

	return nil
}

func (it *prebufferedIterator[T]) Stream(yield func(*T, error) bool) {
	defer it.Stop()

	for {
		// Consume a buffer's worth of items.
		err := it.consumeBuffer()
		if err != nil {
			yield(nil, err)
			return
		}

		// If no items in buffer, we're done.
		if len(it.Buffer) == 0 {
			return
		}

		// Yield all items in the current buffer.
		for _, item := range it.Buffer {
			if !yield(it.Map(item), nil) {
				return
			}
		}

		// If buffer wasn't full, we've reached the end.
		if len(it.Buffer) < it.BufferSize {
			return
		}
	}
}

// WithMap allows to set a mapping function that transforms each item
// before yielding it to the consumer.
func WithMap[T any](mapper func(*T) *T) Option[T] {
	return func(it *prebufferedIterator[T]) {
		it.Map = mapper
	}
}

// New creates a buffered iterator that pre-fetches items from the source iterator
// in chunks of the specified buffer size.
func New[T any](source iter.Seq2[*T, error], bufferSize int, opts ...Option[T]) iter.Seq2[*T, error] {
	var it prebufferedIterator[T]

	it.BufferSize = bufferSize
	it.Buffer = make([]*T, 0, bufferSize)
	it.Map = func(item *T) *T { return item }
	it.Next, it.Stop = iter.Pull2(source)

	for _, opt := range opts {
		opt(&it)
	}

	return it.Stream
}
