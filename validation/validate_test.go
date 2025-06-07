// SPDX-License-Identifier: ice License 1.0
package validation

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)

	addr, release := query.NewTestDatabase(ctx)
	query.MustInit(ctx, query.WithConfig(&query.Config{
		URL: addr,
	}))
	MustInit()

	code := m.Run()
	cancel()
	release()
	if code == 0 {
		if err := goleak.Find(); err != nil {
			fmt.Printf("goleak found issues: %v\n", err)
			code = 1
		}
	}
	os.Exit(code)
}

func TestValidateGiftWrap(t *testing.T) {
	t.Parallel()

	key := model.GeneratePrivateKey()

	var ev model.Event
	ev.Kind = nostr.KindGiftWrap
	ev.CreatedAt = 1
	require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.Error(t, Validate(t.Context(), &ev))

	ev.Tags = append(ev.Tags, model.Tag{"p", "foop"}, model.Tag{"k", "123"})
	require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.Error(t, Validate(t.Context(), &ev))

	ev.Tags = append(ev.Tags, model.Tag{"expiration", "foo"})
	require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.Error(t, Validate(t.Context(), &ev))

	ev.Tags = append(ev.Tags[:len(ev.Tags)-2], model.Tag{"expiration", "123"})
	require.NoError(t, ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.Error(t, Validate(t.Context(), &ev))
}

func TestValidateDtag(t *testing.T) {
	t.Parallel()

	// Known event kind.
	var ev model.Event
	ev.Kind = nostr.KindArticle
	require.NoError(t, ev.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.Error(t, Validate(t.Context(), &ev))

	// Unknown event kind, but addressable.
	ev.Kind = nostr.KindLiveEvent
	require.Error(t, Validate(t.Context(), &ev))
	ev.Tags = append(ev.Tags, model.Tag{"d", "foo"})
	require.NoError(t, ev.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, Validate(t.Context(), &ev))
}

func TestValidateOneOfSingle(t *testing.T) {
	t.Parallel()

	validator := newKindValidatorBuilderEmpty().OneOfSingle("e", "a").Build()
	require.NotNil(t, validator)

	rules := map[model.Kind]kindValidator{
		nostr.KindTextNote: validator,
	}

	var cases = []struct {
		Err   bool
		Event *model.Event
	}{
		{true, &model.Event{Event: nostr.Event{Kind: nostr.KindTextNote}}},
		{true, &model.Event{Event: nostr.Event{Kind: nostr.KindTextNote, Tags: model.Tags{{"p", "test"}}}}},
		{false, &model.Event{Event: nostr.Event{Kind: nostr.KindTextNote, Tags: model.Tags{{"e", "test"}}}}},
		{false, &model.Event{Event: nostr.Event{Kind: nostr.KindTextNote, Tags: model.Tags{{"a", "1:aa:aa"}}}}},
		{true, &model.Event{Event: nostr.Event{Kind: nostr.KindTextNote, Tags: model.Tags{{"e", "test"}, {"a", "1:aa:aa"}}}}},
		{true, &model.Event{Event: nostr.Event{Kind: nostr.KindTextNote, Tags: model.Tags{{"e", "test"}, {"e", "test2"}}}}},
		{true, &model.Event{Event: nostr.Event{Kind: nostr.KindTextNote, Tags: model.Tags{{"a", "1:aa:aa2"}, {"a", "1:aa:aa2"}}}}},
		{true, &model.Event{Event: nostr.Event{Kind: nostr.KindTextNote, Tags: model.Tags{{"e", "test"}, {"e", "test2"}, {"a", "1:aa:aa"}}}}},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("case %d", i), func(t *testing.T) {
			err := validateEventTags(c.Event, rules)
			if c.Err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateArticleSoftDelete(t *testing.T) {
	t.Parallel()

	nowUnix := time.Now().Unix()

	tests := []struct {
		name    string
		event   *model.Event
		wantErr bool
	}{
		{
			name: "valid article soft delete",
			event: &model.Event{
				Event: nostr.Event{
					Kind:      nostr.KindArticle,
					CreatedAt: nostr.Timestamp(nowUnix),
					Content:   "",
					Tags: model.Tags{
						{"d", "test"},
						{"published_at", strconv.FormatInt(nowUnix+1, 10)},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid editable text note soft delete",
			event: &model.Event{
				Event: nostr.Event{
					Kind:      model.CustomIONKindEditableTextNote,
					CreatedAt: nostr.Timestamp(nowUnix),
					Content:   "",
					Tags: model.Tags{
						{"d", "test"},
						{"published_at", strconv.FormatInt(nowUnix+2, 10)},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid editable text note",
			event: &model.Event{
				Event: nostr.Event{
					Kind:      model.CustomIONKindEditableTextNote,
					CreatedAt: nostr.Timestamp(nowUnix),
					Content:   "",
					Tags: model.Tags{
						{"d", "test"},
						{"published_at", strconv.FormatInt(nowUnix, 10)},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, tt.event.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

			err := Validate(t.Context(), tt.event)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestMultipleTagsP(t *testing.T) {
	t.Parallel()

	require.Error(t, validateEventTags(&model.Event{Event: nostr.Event{
		Kind: nostr.KindTextNote,
		Tags: model.Tags{
			{"p", "foo"},
			{"p", "foo"},
		}}}, KindSupportedTags))

	require.NoError(t, validateEventTags(&model.Event{Event: nostr.Event{
		Kind: func() int {
			for k := range kindAllowMultipleTagsP {
				return k
			}
			panic("unreachable")
		}(),
		Tags: model.Tags{
			{"p", "foo"},
			{"p", "foo"},
		}}}, KindSupportedTags))
}
