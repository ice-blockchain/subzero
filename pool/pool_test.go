// SPDX-License-Identifier: ice License 1.0

package pool

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("default configuration", func(t *testing.T) {
		constructor := func() int { return 42 }
		pool := New(constructor)

		require.NotNil(t, pool)
		require.NotNil(t, pool.new)
		require.Equal(t, 0, cap(pool.ch))
		require.Equal(t, 0, len(pool.ch))
	})

	t.Run("with desired number of items", func(t *testing.T) {
		constructor := func() string { return "test" }
		pool := New(constructor, WithDesiredNumberOfItems[string](5))

		require.Equal(t, 5, cap(pool.ch))
		require.Equal(t, 0, len(pool.ch))
	})

	t.Run("with pre-fill", func(t *testing.T) {
		constructor := func() int { return 123 }
		pool := New(constructor, WithDesiredNumberOfItems[int](3), WithPreFill[int](true))

		require.Equal(t, 3, len(pool.ch))
		require.Equal(t, 3, cap(pool.ch))
	})

	t.Run("negative desired items", func(t *testing.T) {
		constructor := func() int { return 0 }
		pool := New(constructor, WithDesiredNumberOfItems[int](-5))

		require.Equal(t, 0, cap(pool.ch))
	})

	t.Run("zero desired items", func(t *testing.T) {
		constructor := func() bool { return true }
		pool := New(constructor, WithDesiredNumberOfItems[bool](0))

		require.Equal(t, 0, cap(pool.ch))
		require.Equal(t, 0, len(pool.ch))
	})

	t.Run("multiple options", func(t *testing.T) {
		constructor := func() float64 { return 3.14 }
		pool := New(constructor,
			WithDesiredNumberOfItems[float64](10),
			WithPreFill[float64](false),
			WithDesiredNumberOfItems[float64](5)) // Should use the last value.

		require.Equal(t, 5, cap(pool.ch))
		require.Equal(t, 0, len(pool.ch))
	})
}

func TestGet(t *testing.T) {
	t.Parallel()

	t.Run("get from empty pool", func(t *testing.T) {
		constructor := func() int { return 42 }
		pool := New(constructor)

		item := pool.Get()
		require.Equal(t, 42, item)
	})

	t.Run("get from pre-filled pool", func(t *testing.T) {
		constructor := func() string { return "new" }
		pool := New(constructor, WithDesiredNumberOfItems[string](2), WithPreFill[string](true))

		item := pool.Get()
		require.Equal(t, "new", item)
		require.Equal(t, 1, len(pool.ch))
	})

	t.Run("get multiple items from pre-filled pool", func(t *testing.T) {
		callCount := 0
		constructor := func() int {
			callCount++
			return callCount * 10
		}
		pool := New(constructor, WithDesiredNumberOfItems[int](3), WithPreFill[int](true))

		// Get all pre-filled items.
		item1 := pool.Get()
		item2 := pool.Get()
		item3 := pool.Get()

		require.Contains(t, []int{10, 20, 30}, item1)
		require.Contains(t, []int{10, 20, 30}, item2)
		require.Contains(t, []int{10, 20, 30}, item3)
		require.Equal(t, 0, len(pool.ch))

		// Next get should create a new item.
		item4 := pool.Get()
		require.Equal(t, 40, item4)
	})

	t.Run("get from pool with struct type", func(t *testing.T) {
		type testStruct struct {
			ID   int
			Name string
		}
		constructor := func() testStruct {
			return testStruct{ID: 1, Name: "test"}
		}
		pool := New(constructor, WithDesiredNumberOfItems[testStruct](1), WithPreFill[testStruct](true))

		item := pool.Get()
		require.Equal(t, testStruct{ID: 1, Name: "test"}, item)
	})
}

func TestPut(t *testing.T) {
	t.Parallel()

	t.Run("put to pool with capacity", func(t *testing.T) {
		constructor := func() int { return 0 }
		pool := New(constructor, WithDesiredNumberOfItems[int](2))

		pool.Put(100)
		require.Equal(t, 1, len(pool.ch))

		item := pool.Get()
		require.Equal(t, 100, item)
	})

	t.Run("put to full pool", func(t *testing.T) {
		constructor := func() int { return 0 }
		pool := New(constructor, WithDesiredNumberOfItems[int](1), WithPreFill[int](true))

		pool.Put(200)
		require.Equal(t, 1, len(pool.ch))
	})

	t.Run("put multiple items", func(t *testing.T) {
		constructor := func() int { return -1 }
		pool := New(constructor, WithDesiredNumberOfItems[int](3))

		pool.Put(10)
		pool.Put(20)
		pool.Put(30)
		require.Equal(t, 3, len(pool.ch))

		// Pool is full, this should be discarded.
		pool.Put(40)
		require.Equal(t, 3, len(pool.ch))
	})

	t.Run("put to zero capacity pool", func(t *testing.T) {
		constructor := func() string { return "default" }
		pool := New(constructor) // Default capacity is 0.

		pool.Put("test")
		require.Equal(t, 0, len(pool.ch))

		// Should still get from constructor.
		item := pool.Get()
		require.Equal(t, "default", item)
	})

	t.Run("put and get cycle", func(t *testing.T) {
		constructor := func() int { return 999 }
		pool := New(constructor, WithDesiredNumberOfItems[int](2))

		// Put then get.
		pool.Put(123)
		item := pool.Get()
		require.Equal(t, 123, item)

		// Pool should be empty now.
		require.Equal(t, 0, len(pool.ch))

		// Next get should use constructor.
		item2 := pool.Get()
		require.Equal(t, 999, item2)
	})
}

func TestPoolConcurrency(t *testing.T) {
	t.Parallel()

	t.Run("concurrent get and put", func(t *testing.T) {
		constructor := func() int { return 42 }
		pool := New(constructor, WithDesiredNumberOfItems[int](10))

		var wg sync.WaitGroup
		const numGoroutines = 100

		for i := range numGoroutines {
			wg.Go(func() {
				item := pool.Get()
				pool.Put(item + i)
			})
		}

		wg.Wait()

		item := pool.Get()
		require.GreaterOrEqual(t, item, 42)
	})

	t.Run("concurrent puts to full pool", func(t *testing.T) {
		constructor := func() int { return 1 }
		pool := New(constructor, WithDesiredNumberOfItems[int](5), WithPreFill[int](true))

		var wg sync.WaitGroup
		const numGoroutines = 50

		for i := range numGoroutines {
			wg.Go(func() {
				pool.Put(i + 100)
			})
		}

		wg.Wait()

		// Pool should still have exactly 5 items.
		require.Equal(t, 5, len(pool.ch))
	})

	t.Run("concurrent gets from pre-filled pool", func(t *testing.T) {
		constructor := func() int { return 0 }
		pool := New(constructor, WithDesiredNumberOfItems[int](20), WithPreFill[int](true))

		var wg sync.WaitGroup
		const numGoroutines = 30
		results := make([]int, numGoroutines)

		for i := range numGoroutines {
			wg.Go(func() {
				results[i] = pool.Get()
			})
		}

		wg.Wait()

		// All gets should succeed.
		for _, result := range results {
			require.GreaterOrEqual(t, result, 0)
		}
	})
}

func TestOptions(t *testing.T) {
	t.Parallel()

	t.Run("WithDesiredNumberOfItems", func(t *testing.T) {
		opt := WithDesiredNumberOfItems[int](5)
		cfg := &config[int]{}
		opt(cfg)

		require.Equal(t, 5, cfg.DesiredNumberOfItems)
	})

	t.Run("WithPreFill true", func(t *testing.T) {
		opt := WithPreFill[int](true)
		cfg := &config[int]{}
		opt(cfg)

		require.True(t, cfg.PreFill)
	})

	t.Run("WithPreFill false", func(t *testing.T) {
		opt := WithPreFill[int](false)
		cfg := &config[int]{}
		opt(cfg)

		require.False(t, cfg.PreFill)
	})

	t.Run("multiple WithDesiredNumberOfItems", func(t *testing.T) {
		cfg := &config[int]{}

		opt1 := WithDesiredNumberOfItems[int](3)
		opt2 := WithDesiredNumberOfItems[int](7)

		opt1(cfg)
		require.Equal(t, 3, cfg.DesiredNumberOfItems)

		opt2(cfg)
		require.Equal(t, 7, cfg.DesiredNumberOfItems)
	})
}

func TestPoolWithDifferentTypes(t *testing.T) {
	t.Parallel()

	t.Run("string pool", func(t *testing.T) {
		pool := New(func() string { return "hello" }, WithDesiredNumberOfItems[string](2))

		pool.Put("world")
		item := pool.Get()
		require.Equal(t, "world", item)
	})

	t.Run("slice pool", func(t *testing.T) {
		pool := New(func() []int { return make([]int, 0, 10) }, WithDesiredNumberOfItems[[]int](1))

		slice := []int{1, 2, 3}
		pool.Put(slice)

		retrieved := pool.Get()
		require.Equal(t, slice, retrieved)
	})

	t.Run("pointer pool", func(t *testing.T) {
		type Data struct{ Value int }
		pool := New(func() *Data { return &Data{Value: 42} }, WithDesiredNumberOfItems[*Data](2))

		data := &Data{Value: 100}
		pool.Put(data)

		retrieved := pool.Get()
		require.Equal(t, 100, retrieved.Value)
	})

	t.Run("interface pool", func(t *testing.T) {
		pool := New(func() any { return "default" }, WithDesiredNumberOfItems[any](1))

		pool.Put(123)
		item := pool.Get()
		require.Equal(t, 123, item)
	})
}

func TestPoolEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("nil constructor panic", func(t *testing.T) {
		require.Panics(t, func() {
			New[int](nil, WithDesiredNumberOfItems[int](5), WithPreFill[int](true))
		})
	})

	t.Run("very large capacity", func(t *testing.T) {
		constructor := func() int { return 1 }
		pool := New(constructor, WithDesiredNumberOfItems[int](10000))

		require.Equal(t, 10000, cap(pool.ch))
		require.Equal(t, 0, len(pool.ch))
	})

	t.Run("pre-fill with zero capacity", func(t *testing.T) {
		constructor := func() int { return 5 }
		pool := New(constructor, WithDesiredNumberOfItems[int](0), WithPreFill[int](true))

		require.Equal(t, 0, len(pool.ch))
		require.Equal(t, 0, cap(pool.ch))

		// Should still work with constructor.
		item := pool.Get()
		require.Equal(t, 5, item)
	})
}

func TestPoolBeforeGet(t *testing.T) {
	t.Parallel()

	pool := New(func() int { return 1 },
		WithDesiredNumberOfItems[int](2),
		WithPreFill[int](true),
		WithBeforeGet(func(item int) int { return item * 10 }),
	)
	require.Equal(t, 2, len(pool.ch))

	item1 := pool.Get()
	require.Equal(t, 10, item1)
	require.Equal(t, 1, len(pool.ch))

	item2 := pool.Get()
	require.Equal(t, 10, item2)
	require.Equal(t, 0, len(pool.ch))
}
