// SPDX-License-Identifier: ice License 1.0

package pool

// Pool is a generic resource pool that maintains up to a desired number of items.
// It is thread-safe and has unlimited capacity in the sense that Get() will always
// return an item, creating a new one if necessary.
type Pool[T any] struct {
	ch        chan T
	new       func() T
	beforeGet func(T) T
}

type config[T any] struct {
	DesiredNumberOfItems int
	PreFill              bool
	BeforeGet            func(T) T
}

type Option[T any] func(*config[T])

// WithDesiredNumberOfItems sets the desired number of items to keep persistently in the pool.
func WithDesiredNumberOfItems[T any](n int) Option[T] {
	return func(c *config[T]) {
		c.DesiredNumberOfItems = n
	}
}

// WithPreFill indicates whether to pre-fill the pool with the desired number of items upon creation.
func WithPreFill[T any](preFill bool) Option[T] {
	return func(c *config[T]) {
		c.PreFill = preFill
	}
}

// WithBeforeGet sets a function to be called on an item before it is returned by Get().
func WithBeforeGet[T any](fn func(T) T) Option[T] {
	return func(c *config[T]) {
		c.BeforeGet = fn
	}
}

// New creates a new Pool with the given constructor function and desired number of items to keep persistently.
func New[T any](constructor func() T, opts ...Option[T]) *Pool[T] {
	var cfg config[T]

	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.BeforeGet == nil {
		cfg.BeforeGet = func(item T) T { return item }
	}

	if cfg.DesiredNumberOfItems < 0 {
		cfg.DesiredNumberOfItems = 0
	}

	pool := &Pool[T]{
		ch:        make(chan T, cfg.DesiredNumberOfItems),
		new:       constructor,
		beforeGet: cfg.BeforeGet,
	}

	if cfg.PreFill {
		for range cfg.DesiredNumberOfItems {
			pool.ch <- constructor()
		}
	}

	return pool
}

func (p *Pool[T]) get() T {
	select {
	case item := <-p.ch:
		return item
	default:
		return p.new()
	}
}

// Get retrieves an item from the pool if available, otherwise creates a new one using the constructor.
func (p *Pool[T]) Get() T {
	return p.beforeGet(p.get())
}

// Put attempts to return an item to the pool. If the pool is full, the item is discarded.
func (p *Pool[T]) Put(item T) {
	select {
	case p.ch <- item:
	default:
		// Discard the item.
	}
}
