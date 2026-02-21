// SPDX-License-Identifier: ice License 1.0

package model

import (
	"strconv"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestFiltersMatchWithMasterKey(t *testing.T) {
	t.Parallel()

	privkey, pubkey := GenerateKeyPair()
	masterPriv, masterPubkey := GenerateKeyPair()

	createEvent := func(kind int, tags Tags, pk string) *Event {
		ev := &Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      kind,
			Tags:      tags,
			Content:   "test content",
		}}
		require.NoError(t, ev.SignWithAlg(pk, SignAlgEDDSA, KeyAlgCurve25519))
		return ev
	}

	t.Run("direct match - event kind", func(t *testing.T) {
		ev := createEvent(nostr.KindTextNote, nil, privkey)
		filters := Filters{{Kinds: []int{nostr.KindTextNote}}}

		result := FiltersMatch(filters, ev, masterPubkey, pubkey)
		require.True(t, result)
	})

	t.Run("direct match - event tag", func(t *testing.T) {
		ev := createEvent(nostr.KindTextNote, Tags{{"t", "test"}}, privkey)
		filters := Filters{{Tags: TagMap{}.Set("t", new("test"))}}

		result := FiltersMatch(filters, ev, masterPubkey, pubkey)
		require.True(t, result)
	})

	t.Run("master key match", func(t *testing.T) {
		ev := createEvent(nostr.KindTextNote, Tags{{CustomIONTagOnBehalfOf, masterPubkey}}, masterPriv)

		filters := Filters{{
			Authors: []string{pubkey},
			Kinds:   []int{nostr.KindTextNote},
		}}

		result := FiltersMatch(filters, ev, masterPubkey, pubkey)
		require.True(t, result)
	})

	t.Run("no match", func(t *testing.T) {
		ev := createEvent(nostr.KindTextNote, nil, privkey)
		filters := Filters{{Kinds: []int{nostr.KindArticle}}}

		result := FiltersMatch(filters, ev, masterPubkey, pubkey)
		require.False(t, result)
	})

	t.Run("device key not in authors", func(t *testing.T) {
		ev := createEvent(nostr.KindTextNote, Tags{{CustomIONTagOnBehalfOf, masterPubkey}}, privkey)
		filters := Filters{{
			Authors: []string{"different_key"},
			Kinds:   []int{nostr.KindTextNote},
		}}

		result := FiltersMatch(filters, ev, masterPubkey, pubkey)
		require.False(t, result)
	})

	t.Run("master key substitution match", func(t *testing.T) {
		// Create an event with the master key tag.
		ev := createEvent(nostr.KindTextNote, Tags{{CustomIONTagOnBehalfOf, masterPubkey}}, masterPriv)

		// Create a filter with device key that should match after substitution.
		filters := Filters{{
			Authors: []string{pubkey},
		}}

		result := FiltersMatch(filters, ev, masterPubkey, pubkey)
		require.True(t, result)
	})

	t.Run("complex filters with both matches", func(t *testing.T) {
		ev := createEvent(nostr.KindTextNote, Tags{{"t", "test"}, {CustomIONTagOnBehalfOf, masterPubkey}}, masterPriv)

		filters := Filters{
			{Kinds: []int{nostr.KindArticle}},      // No match.
			{Tags: TagMap{}.Set("t", new("test"))}, // Direct match.
			{Authors: []string{pubkey}},            // Master key match.
		}

		result := FiltersMatch(filters, ev, masterPubkey, pubkey)
		require.True(t, result)
	})
}
func TestFilterMatchKind(t *testing.T) {
	t.Parallel()

	createEvent := func(kind int, tags Tags) *Event {
		return &Event{Event: nostr.Event{
			Kind: kind,
			Tags: tags,
		}}
	}

	t.Run("empty kinds filter", func(t *testing.T) {
		filter := &Filter{Kinds: []int{}}
		ev := createEvent(nostr.KindTextNote, nil)

		result := filterMatchKind(filter, ev)
		require.True(t, result)
	})

	t.Run("direct kind match", func(t *testing.T) {
		filter := &Filter{Kinds: []int{nostr.KindTextNote}}
		ev := createEvent(nostr.KindTextNote, nil)

		result := filterMatchKind(filter, ev)
		require.True(t, result)
	})

	t.Run("no kind match", func(t *testing.T) {
		filter := &Filter{Kinds: []int{nostr.KindArticle}}
		ev := createEvent(nostr.KindTextNote, nil)

		result := filterMatchKind(filter, ev)
		require.False(t, result)
	})

	t.Run("negative kind match", func(t *testing.T) {
		filter := &Filter{Kinds: []int{-nostr.KindTextNote}}
		ev := createEvent(nostr.KindTextNote, nil)

		result := filterMatchKind(filter, ev)
		require.False(t, result)
	})

	t.Run("repost of article", func(t *testing.T) {
		filter := &Filter{Kinds: []int{CustomIONKindRepostOfArticle}}
		ev := createEvent(nostr.KindGenericRepost, Tags{{"k", strconv.Itoa(nostr.KindArticle)}})

		result := filterMatchKind(filter, ev)
		require.True(t, result)
	})

	t.Run("repost of editable text note", func(t *testing.T) {
		filter := &Filter{Kinds: []int{CustomIONKindRepostOfEditableTextNote}}
		ev := createEvent(nostr.KindGenericRepost, Tags{{"k", strconv.Itoa(CustomIONKindEditableTextNote)}})

		result := filterMatchKind(filter, ev)
		require.True(t, result)
	})

	t.Run("repost with wrong k tag value", func(t *testing.T) {
		filter := &Filter{Kinds: []int{CustomIONKindRepostOfArticle}}
		ev := createEvent(nostr.KindGenericRepost, Tags{{"k", strconv.Itoa(nostr.KindTextNote)}})

		result := filterMatchKind(filter, ev)
		require.False(t, result)
	})

	t.Run("repost with missing k tag", func(t *testing.T) {
		filter := &Filter{Kinds: []int{CustomIONKindRepostOfArticle}}
		ev := createEvent(nostr.KindGenericRepost, nil)

		result := filterMatchKind(filter, ev)
		require.False(t, result)
	})

	t.Run("repost with invalid k tag value", func(t *testing.T) {
		filter := &Filter{Kinds: []int{CustomIONKindRepostOfArticle}}
		ev := createEvent(nostr.KindGenericRepost, Tags{{"k", "not-a-number"}})

		result := filterMatchKind(filter, ev)
		require.False(t, result)
	})
}

func TestFilterMatchAddresses(t *testing.T) {
	t.Parallel()

	var ev Event
	ev.Kind = CustomIONKindEditableTextNote
	ev.CreatedAt = nostr.Now()
	ev.Content = "test content"
	require.NoError(t, ev.SignWithAlg(GeneratePrivateKey(), SignAlgEDDSA, KeyAlgCurve25519))

	var regularEv Event
	regularEv.Kind = nostr.KindTextNote
	regularEv.CreatedAt = nostr.Now()
	regularEv.Content = "regular content"
	require.NoError(t, regularEv.SignWithAlg(GeneratePrivateKey(), SignAlgEDDSA, KeyAlgCurve25519))

	t.Logf("regular event address %v / id %v", regularEv.Address(), regularEv.ID)

	t.Logf("address %v / id %v", ev.Address(), ev.ID)
	t.Run("match by address", func(t *testing.T) {
		filter := &Filter{Addresses: []string{ev.Address()}}
		result := filterMatchAddresses(filter, &ev)
		require.True(t, result)
	})
	t.Run("match by ID in address", func(t *testing.T) {
		filter := &Filter{Addresses: []string{ev.ID}}
		result := filterMatchAddresses(filter, &ev)
		require.True(t, result)
	})

	t.Run("empty address filter", func(t *testing.T) {
		filter := &Filter{Addresses: []string{}}
		result := filterMatchAddresses(filter, &ev)
		require.False(t, result)
	})

	t.Run("no match - wrong address", func(t *testing.T) {
		filter := &Filter{Addresses: []string{"wrong:address:format"}}
		result := filterMatchAddresses(filter, &ev)
		require.False(t, result)
	})

	t.Run("regular event - match by ID", func(t *testing.T) {
		filter := &Filter{Addresses: []string{regularEv.ID}}
		result := filterMatchAddresses(filter, &regularEv)
		require.True(t, result)
	})

	t.Run("regular event - match by address (which is ID)", func(t *testing.T) {
		// For regular events, Address() returns the ID itself.
		filter := &Filter{Addresses: []string{regularEv.Address()}}
		result := filterMatchAddresses(filter, &regularEv)
		require.True(t, result)
	})

	t.Run("multiple addresses - one matches", func(t *testing.T) {
		filter := &Filter{Addresses: []string{"wrong:address:1", ev.ID, "wrong:address:2"}}
		result := filterMatchAddresses(filter, &ev)
		require.True(t, result)
	})

	t.Run("multiple addresses - none match", func(t *testing.T) {
		filter := &Filter{Addresses: []string{"wrong:address:1", "wrong:address:2", "wrong:address:3"}}
		result := filterMatchAddresses(filter, &ev)
		require.False(t, result)
	})
}
