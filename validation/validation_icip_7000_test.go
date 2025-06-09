// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"strconv"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestPostWithRichTextOnly(t *testing.T) {
	t.Parallel()

	var ev model.Event
	ev.Kind = model.CustomIONKindEditableTextNote
	ev.CreatedAt = nostr.Now()
	ev.Tags = model.Tags{
		{model.CustomIONTagRichText, "foo"},
		{"d", "foo"},
		{"published_at", strconv.FormatInt(time.Now().Unix(), 10)},
	}

	ev.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519)
	require.NoError(t, Validate(t.Context(), &ev))
}
