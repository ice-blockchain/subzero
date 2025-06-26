// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestGlobalMaxPostSizeOf(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		MaxContentSizes: map[int]int{
			nostr.KindArticle: 0xffff,
		},
	}

	require.Equal(t, 0, cfg.MaxContentSizeOf(0))
	require.Equal(t, 0xffff, cfg.MaxContentSizeOf(nostr.KindArticle))
}
