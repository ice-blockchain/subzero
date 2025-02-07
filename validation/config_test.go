// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestGlobalMaxPostSizeOf(t *testing.T) {
	t.Parallel()

	require.Equal(t, 0, globalConfig.MaxPostSizeOf(0))
	require.Equal(t, 0xffff, globalConfig.MaxPostSizeOf(nostr.KindArticle))
}
