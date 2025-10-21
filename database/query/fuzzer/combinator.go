// SPDX-License-Identifier: ice License 1.0

//go:build test

package main

import (
	"fmt"
	"strings"
)

// GenericCombinationsIterator provides an iterator for generating all possible combinations
// of a given length from a set of strings. It uses bit manipulation to efficiently
// track and generate subsets, where subsetBits represents the current combination
// state and maxSubsetBits defines the upper bound for iteration.
type GenericCombinationsIterator struct {
	set           []string
	length        uint
	subsetBits    int
	maxSubsetBits int
}

func NewGenericCombinationsIterator(set []string) *GenericCombinationsIterator {
	length := uint(len(set))
	return &GenericCombinationsIterator{
		set:           set,
		length:        length,
		subsetBits:    1,
		maxSubsetBits: 1 << length,
	}
}

func (it *GenericCombinationsIterator) Total() int {
	return it.maxSubsetBits - 1
}

func (it *GenericCombinationsIterator) Seek(position int) error {
	if position < 0 || position >= it.Total() {
		return fmt.Errorf("generic seek position %d is out of bounds (0-%d)", position, it.Total()-1)
	}
	it.subsetBits = position + 1
	return nil
}

func (it *GenericCombinationsIterator) Next() ([]string, bool) {
	if it.subsetBits >= it.maxSubsetBits {
		return nil, false
	}
	var subset []string
	for object := uint(0); object < it.length; object++ {
		if (it.subsetBits>>object)&1 == 1 {
			subset = append(subset, it.set[object])
		}
	}
	it.subsetBits++
	return subset, true
}

type FlagCombinationsIterator struct {
	flagMap   map[string][]string
	flagOrder []string
	indices   []int
	isDone    bool
}

// NewFlagCombinationsIterator creates a new iterator for generating all possible combinations
// of flags from the provided flagged slice. Each flag in the slice should be in the format
// "key:value". Flags with the same key are grouped together, and the iterator will generate
// combinations by selecting one flag from each key group.
//
// Example:
//
//	flags := []string{"color:red", "color:blue", "size:small", "size:large"}
//	iterator := NewFlagCombinationsIterator(flags)
//	// This will create combinations like: ["color:red", "size:small"], ["color:red", "size:large"], etc.
func NewFlagCombinationsIterator(flagged []string) *FlagCombinationsIterator {
	flagMap := make(map[string][]string)
	var flagOrder []string
	for _, flag := range flagged {
		parts := strings.SplitN(flag, ":", 2)
		if len(parts) == 2 {
			key := parts[0]
			if _, exists := flagMap[key]; !exists {
				flagOrder = append(flagOrder, key)
			}
			flagMap[key] = append(flagMap[key], flag)
		}
	}
	return &FlagCombinationsIterator{
		flagMap:   flagMap,
		flagOrder: flagOrder,
		indices:   make([]int, len(flagOrder)),
		isDone:    len(flagOrder) == 0 && len(flagged) > 0,
	}
}

func (it *FlagCombinationsIterator) Total() int {
	if len(it.flagMap) == 0 {
		return 1
	}
	total := 1
	for _, key := range it.flagOrder {
		total *= (len(it.flagMap[key]) + 1)
	}
	return total
}

func (it *FlagCombinationsIterator) Reset() {
	for i := range it.indices {
		it.indices[i] = 0
	}
	it.isDone = len(it.flagOrder) == 0 && len(it.flagMap) > 0
}

func (it *FlagCombinationsIterator) Seek(position int) error {
	if position < 0 || position >= it.Total() {
		return fmt.Errorf("flag seek position %d is out of bounds (0-%d)", position, it.Total()-1)
	}
	it.Reset()
	// Convert the linear position into the mixed-radix 'indices' state.
	remainingPosition := position
	for i := len(it.flagOrder) - 1; i >= 0; i-- {
		key := it.flagOrder[i]
		numChoices := len(it.flagMap[key]) + 1
		it.indices[i] = remainingPosition % numChoices
		remainingPosition /= numChoices
	}
	it.isDone = false
	return nil
}

func (it *FlagCombinationsIterator) Next() ([]string, bool) {
	if it.isDone {
		return nil, false
	}
	var currentCombination []string
	for i, key := range it.flagOrder {
		if it.indices[i] > 0 {
			variantIndex := it.indices[i] - 1
			currentCombination = append(currentCombination, it.flagMap[key][variantIndex])
		}
	}
	for i := len(it.indices) - 1; i >= 0; i-- {
		key := it.flagOrder[i]
		numChoices := len(it.flagMap[key]) + 1
		it.indices[i]++
		if it.indices[i] < numChoices {
			return currentCombination, true
		}
		it.indices[i] = 0
	}
	it.isDone = true
	return currentCombination, true
}

type SearchCombinationsIterator struct {
	genericIter          *GenericCombinationsIterator
	flaggedIter          *FlagCombinationsIterator
	currentGenericSubset []string
	currentPosition      int
	total                int
}

// NewSearchCombinationsIterator creates a new iterator that generates all possible combinations
// of generic and flagged string slices, starting from the specified position.
// The iterator combines every element from the generic combinations with every element from
// the flagged combinations, creating a cartesian product of all possibilities.
func NewSearchCombinationsIterator(generic []string, flagged []string, startPosition int) (*SearchCombinationsIterator, error) {
	gIter := NewGenericCombinationsIterator(generic)
	fIter := NewFlagCombinationsIterator(flagged)
	total := gIter.Total() * fIter.Total()

	if startPosition < 0 || startPosition > total {
		return nil, fmt.Errorf("startPosition %d is out of bounds (0-%d)", startPosition, total)
	}

	it := &SearchCombinationsIterator{
		genericIter:     gIter,
		flaggedIter:     fIter,
		currentPosition: startPosition,
		total:           total,
	}

	if startPosition > 0 && startPosition < total {
		numFlagCombos := fIter.Total()
		targetGenericIndex := startPosition / numFlagCombos
		targetFlagIndex := startPosition % numFlagCombos

		if err := gIter.Seek(targetGenericIndex); err != nil {
			return nil, err
		}
		if err := fIter.Seek(targetFlagIndex); err != nil {
			return nil, err
		}
	}

	// Prime the pump: load the first generic subset we need to work with.
	// This works for both starting at 0 and for a resumed session.
	it.currentGenericSubset, _ = gIter.Next()

	return it, nil
}

func (it *SearchCombinationsIterator) Total() int {
	return it.total
}

func (it *SearchCombinationsIterator) Position() int {
	return it.currentPosition
}

func (it *SearchCombinationsIterator) Next() ([]string, bool) {
	if it.currentPosition >= it.total || it.currentGenericSubset == nil {
		return nil, false
	}

	flagSubset, hasNextFlag := it.flaggedIter.Next()
	if !hasNextFlag {
		var hasNextGeneric bool
		it.currentGenericSubset, hasNextGeneric = it.genericIter.Next()
		if !hasNextGeneric {
			return nil, false
		}
		it.flaggedIter.Reset()
		flagSubset, _ = it.flaggedIter.Next()
	}

	combined := make([]string, 0, len(it.currentGenericSubset)+len(flagSubset))
	combined = append(combined, it.currentGenericSubset...)
	combined = append(combined, flagSubset...)

	it.currentPosition++

	return combined, true
}
