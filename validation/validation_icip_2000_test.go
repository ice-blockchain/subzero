// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestValidateTagsBAndP(t *testing.T) {
	t.Parallel()

	require.Error(t, validateFollowListEvent(&model.Event{Event: nostr.Event{Tags: model.Tags{
		{"b", "foo"},
		{"p", "foo"},
	}}}))

	require.Error(t, validateEventTags(&model.Event{Event: nostr.Event{Tags: model.Tags{
		{"b", "foo"},
		{"b", "foo"},
	}}}, KindSupportedTags))

	require.Error(t, validateEventTags(&model.Event{Event: nostr.Event{Tags: model.Tags{
		{"p", "foo"},
		{"p", "foo"},
	}}}, KindSupportedTags))

	require.Error(t, validateFollowListEvent(&model.Event{Event: nostr.Event{PubKey: "foo", Tags: model.Tags{
		{"p", "foo"},
	}}}))

	require.NoError(t, validateEventTags(&model.Event{Event: nostr.Event{Tags: model.Tags{
		{"b", "foo"},
		{"p", "bar"},
	}}}, KindSupportedTags))
}
