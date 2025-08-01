// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestValidateKindGiftWrapEvent(t *testing.T) {
	t.Parallel()

	var cases = []struct {
		Event *model.Event
		Err   bool
	}{
		{
			Event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindGiftWrap,
					Tags: model.Tags{
						{"p", "test"},
						{"k", "1"},
					},
				},
			},
			Err: true, // Missing expiration tag.
		},
		{
			Event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindGiftWrap,
					Tags: model.Tags{
						{"p", "test"},
						{"k", "1"},
						{"expiration", "foo"}, // Invalid expiration value.
					},
				},
			},
			Err: true,
		},
		{
			Event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindGiftWrap,
					Tags: model.Tags{
						{"p", "test"},
						{"k", "1"},
						{"expiration", "9223372036"}, // Too far in the future.
					},
				},
			},
			Err: true,
		},
		{
			Event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindGiftWrap,
					Tags: model.Tags{
						{"p", "test"},
						{"k", strconv.Itoa(model.CustomIONKindUserBlock)},
						{"expiration", strconv.Itoa(int(time.Now().Add(24 * time.Hour).Unix()))},
					},
				},
			},
		},
		{
			Event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindGiftWrap,
					Tags: model.Tags{
						{"p", "test"},
						{"k", "1"},
						{"expiration", strconv.Itoa(int(time.Now().Add(24 * time.Hour).Unix()))},
					},
				},
			},
		},
		{
			Event: &model.Event{
				Event: nostr.Event{
					Kind: nostr.KindGiftWrap,
					Tags: model.Tags{
						{"p", "test"},
						{"k", strconv.Itoa(model.CustomIONKindUserBlock)},
					},
				},
			},
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("case %d", i), func(t *testing.T) {
			require.NoError(t, c.Event.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
			err := Validate(t.Context(), model.Events{c.Event})
			if c.Err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
