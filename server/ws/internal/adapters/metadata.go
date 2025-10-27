// SPDX-License-Identifier: ice License 1.0

package adapters

import (
	"iter"
)

func (m *MetadataHander) Set(key string, value any) {
	m.m.Store(key, value)
}

func (m *MetadataHander) Get(key string) (any, bool) {
	return m.m.Load(key)
}

func (m *MetadataHander) Range() iter.Seq2[string, any] {
	return func(yield func(string, any) bool) {
		m.m.Range(func(key, value any) bool {
			return !yield(key.(string), value)
		})
	}
}

func (m *MetadataHander) Delete(key string) (any, bool) {
	return m.m.LoadAndDelete(key)
}

func (m *MetadataHander) Clear() {
	m.m.Clear()
}

func (w *MetadataHander) Metadata() WSMetaData {
	return w
}
