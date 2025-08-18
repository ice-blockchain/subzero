// SPDX-License-Identifier: ice License 1.0

package storage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFileNameFromContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ctx      context.Context
		expected string
	}{
		{
			name:     "context with filename",
			ctx:      WithFileNameInContext(t.Context(), "example.txt"),
			expected: "example.txt",
		},
		{
			name:     "context without filename",
			ctx:      t.Context(),
			expected: "",
		},
		{
			name:     "context with empty filename",
			ctx:      WithFileNameInContext(t.Context(), ""),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FileNameFromContext(tt.ctx)
			require.Equal(t, tt.expected, result)
		})
	}
}
