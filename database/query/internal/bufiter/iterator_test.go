// SPDX-License-Identifier: ice License 1.0

package bufiter

import (
	"errors"
	"iter"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"
)

func helperNewIterator[T any](t *testing.T, err error, values ...T) iter.Seq2[*T, error] {
	t.Helper()

	var errorAfter int
	if err != nil {
		errorAfter = rand.IntN(len(values)) // Randomly decide when to return an error.
	}

	return func(yield func(*T, error) bool) {
		for i, v := range values {
			if i == errorAfter && err != nil {
				if !yield(nil, err) {
					return
				}
			}
			if !yield(&v, nil) {
				return
			}
		}
	}
}

func TestNewPrebufferedIterator_Empty(t *testing.T) {
	t.Parallel()

	buffered := New(helperNewIterator[int](t, nil), 3)

	var results []*int
	for item, err := range buffered {
		require.NoError(t, err)
		results = append(results, item)
	}

	require.Len(t, results, 0)
}

func TestNewPrebufferedIterator_SingleItem(t *testing.T) {
	t.Parallel()

	values := []int{42}
	buffered := New(helperNewIterator(t, nil, values...), 3)

	var results []*int
	for item, err := range buffered {
		require.NoError(t, err)
		results = append(results, item)
	}

	require.Len(t, results, 1)
	require.Equal(t, 42, *results[0])
}

func TestNewPrebufferedIterator_MultipleItemsLessThanBufferSize(t *testing.T) {
	t.Parallel()

	values := []int{1, 2}
	buffered := New(helperNewIterator(t, nil, values...), 5)

	var results []*int
	for item, err := range buffered {
		require.NoError(t, err)
		results = append(results, item)
	}

	require.Len(t, results, len(values))
	for i, expected := range values {
		require.Equal(t, expected, *results[i])
	}
}

func TestNewPrebufferedIterator_MultipleItemsEqualToBufferSize(t *testing.T) {
	t.Parallel()

	values := []int{1, 2, 3}
	buffered := New(helperNewIterator(t, nil, values...), len(values))

	var results []*int
	for item, err := range buffered {
		require.NoError(t, err)
		results = append(results, item)
	}

	require.Len(t, results, len(values))
	for i, expected := range values {
		require.Equal(t, expected, *results[i])
	}
}

func TestNewPrebufferedIterator_MultipleItemsGreaterThanBufferSize(t *testing.T) {
	t.Parallel()

	values := []int{1, 2, 3, 4, 5, 6, 7}
	buffered := New(helperNewIterator(t, nil, values...), 3)

	var results []*int
	for item, err := range buffered {
		require.NoError(t, err)
		results = append(results, item)
	}

	require.Len(t, results, len(values))
	for i, expected := range values {
		require.Equal(t, expected, *results[i])
	}
}

func TestNewPrebufferedIterator_ErrorInSourceIterator(t *testing.T) {
	t.Parallel()

	testErr := errors.New("test error")
	buffered := New(helperNewIterator(t, testErr, 1, 2), 3)

	var results []*int
	var gotErr error
	for item, err := range buffered {
		if err != nil {
			gotErr = err
			break
		}
		results = append(results, item)
	}

	require.ErrorIs(t, gotErr, testErr)
	require.Len(t, results, 0)
}

func TestNewPrebufferedIterator_EarlyTermination(t *testing.T) {
	t.Parallel()

	values := []int{1, 2, 3, 4, 5}
	buffered := New(helperNewIterator(t, nil, values...), 3)

	var results []*int
	for item, err := range buffered {
		require.NoError(t, err)
		results = append(results, item)
		if len(results) == 2 {
			break
		}
	}

	require.Len(t, results, 2)
}

func TestNewPrebufferedIterator_BufferSizeOne(t *testing.T) {
	t.Parallel()

	values := []int{10, 20, 30}
	buffered := New(helperNewIterator(t, nil, values...), 1)

	var results []*int
	for item, err := range buffered {
		require.NoError(t, err)
		results = append(results, item)
	}

	require.Len(t, results, len(values))
	for i, expected := range values {
		require.Equal(t, expected, *results[i])
	}
}

func TestNewPrebufferedIterator_LargeBufferSize(t *testing.T) {
	t.Parallel()

	values := []int{1, 2, 3}
	buffered := New(helperNewIterator(t, nil, values...), 100)

	var results []*int
	for item, err := range buffered {
		require.NoError(t, err)
		results = append(results, item)
	}

	require.Len(t, results, len(values))
	for i, expected := range values {
		require.Equal(t, expected, *results[i])
	}
}

func TestNewPrebufferedIterator_WithMap(t *testing.T) {
	t.Parallel()

	values := []int{1, 2, 3}
	buffered := New(helperNewIterator(t, nil, values...), 100, WithMap(func(i *int) *int {
		if i == nil {
			return nil
		}
		mappedValue := *i * 10

		return &mappedValue
	}))

	var results []*int
	for item, err := range buffered {
		require.NoError(t, err)
		results = append(results, item)
	}

	require.Len(t, results, len(values))
	for i, expected := range values {
		require.Equal(t, expected*10, *results[i])
	}
}
