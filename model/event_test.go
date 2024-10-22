// SPDX-License-Identifier: ice License 1.0

package model

import (
	"crypto/ed25519"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func helperTestSignVerify(t *testing.T, ev *Event, pk string, signAlg EventSignAlg, keyAlg EventKeyAlg) {
	t.Helper()

	t.Run("Sign", func(t *testing.T) {
		err := ev.SignWithAlg(pk, signAlg, keyAlg)
		require.NoError(t, err)
		t.Logf("%v: signature: %s", signAlg, ev.Sig)
	})
	t.Run("Verify", func(t *testing.T) {
		ok, err := ev.CheckSignature()
		require.NoError(t, err)
		require.True(t, ok)
		t.Logf("%v: signature: %s: OK", signAlg, ev.Sig)
	})
}

func TestEventSignVerify(t *testing.T) {
	t.Parallel()

	t.Run(strings.Join([]string{string(SignAlgSchnorr), string(KeyAlgSecp256k1)}, "_"), func(t *testing.T) {
		var ev Event
		ev.Kind = nostr.KindTextNote
		ev.CreatedAt = 1
		ev.Content = string(SignAlgSchnorr)

		pk := nostr.GeneratePrivateKey()
		require.NotEmpty(t, pk)
		helperTestSignVerify(t, &ev, pk, SignAlgSchnorr, KeyAlgSecp256k1)
	})
	t.Run(strings.Join([]string{string(SignAlgEDDSA), string(KeyAlgCurve25519)}, "_"), func(t *testing.T) {
		var ev Event
		ev.Kind = nostr.KindTextNote
		ev.CreatedAt = 1
		ev.Content = string(SignAlgEDDSA)

		_, pk, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		helperTestSignVerify(t, &ev, hex.EncodeToString(pk), SignAlgEDDSA, KeyAlgCurve25519)
	})
	t.Run("ArgError", func(t *testing.T) {
		t.Run("Sign", func(t *testing.T) {
			var ev Event
			err := ev.SignWithAlg("", SignAlgEDDSA, "")
			require.ErrorIs(t, err, ErrUnsupportedAlg)
		})
		t.Run("Verify", func(t *testing.T) {
			testData := []string{
				"foo",
				"/foo",
				"/",
				"foo/",
				"",
				"/",
				"//",
			}
			for i := range testData {
				var ev Event
				ev.Sig = testData[i] + ":" + hex.EncodeToString([]byte("signature"))
				ok, err := ev.CheckSignature()
				require.ErrorIsf(t, err, ErrUnsupportedAlg, "testData[%d]: %s", i, testData[i])
				require.False(t, ok)

			}
		})
	})
	t.Run("Unknown", func(t *testing.T) {
		t.Run("Sign", func(t *testing.T) {
			var ev Event
			pk := hex.EncodeToString([]byte("private key"))
			require.ErrorIs(t, ev.SignWithAlg(pk, EventSignAlg("unknown"), EventKeyAlg("unknown")), ErrUnsupportedAlg)
			require.ErrorIs(t, ev.SignWithAlg(pk, SignAlgSchnorr, KeyAlgCurve25519), ErrUnsupportedAlg)
			require.ErrorIs(t, ev.SignWithAlg(pk, SignAlgEDDSA, KeyAlgSecp256k1), ErrUnsupportedAlg)
		})
		t.Run("Verify", func(t *testing.T) {
			var ev Event
			ev.Sig = "unknown/foo:" + hex.EncodeToString([]byte("signature"))
			ok, err := ev.CheckSignature()
			require.ErrorIs(t, err, ErrUnsupportedAlg)
			require.False(t, ok)
		})
	})
	t.Run("Default", func(t *testing.T) {
		var ev Event
		ev.Kind = nostr.KindTextNote
		ev.CreatedAt = 1
		ev.Content = "default"

		pk := nostr.GeneratePrivateKey()
		require.NotEmpty(t, pk)
		helperTestSignVerify(t, &ev, pk, "", "")
	})
}

func TestEventEnvelopeEncodeDecode(t *testing.T) {
	t.Parallel()

	t.Run("SingleEvent", func(t *testing.T) {
		var ev Event
		ev.Kind = nostr.KindTextNote
		ev.CreatedAt = 1
		ev.Content = "foo"
		ev.Tags = Tags{{"bar", "baz"}}

		envelopeSubzero := EventEnvelope{
			Events: []*Event{&ev},
		}
		envelopeNostr := nostr.EventEnvelope{
			Event: ev.Event,
		}

		t.Run("EncodeDecode", func(t *testing.T) {
			data, err := envelopeSubzero.MarshalJSON()
			require.NoError(t, err)
			t.Logf("data: %s", string(data))
			require.NotEmpty(t, data)

			dataNostr, err := envelopeNostr.MarshalJSON()
			require.NoError(t, err)
			t.Logf("dataNostr: %s", string(dataNostr))
			require.NotEmpty(t, dataNostr)

			require.Equal(t, dataNostr, data)

			// Cannot do deep equal because of internal fields.
			//                                 @@ -17,3 +17,4 @@
			// Sig: (string) "",
			// -    extra: (map[string]interface {}) <nil>
			// +    extra: (map[string]interface {}) {
			// +    }
			//	}

			e := nostr.ParseMessage(data)
			require.NotNil(t, e)
			require.Equal(t, envelopeSubzero.Events[0].Event.Content, e.(*nostr.EventEnvelope).Event.Content)
			require.Equal(t, envelopeSubzero.Events[0].Event.Tags, e.(*nostr.EventEnvelope).Event.Tags)
			require.Equal(t, envelopeSubzero.Events[0].Event.CreatedAt, e.(*nostr.EventEnvelope).Event.CreatedAt)
			require.Equal(t, envelopeSubzero.Events[0].Event.Kind, e.(*nostr.EventEnvelope).Event.Kind)

			e2, err := ParseMessage(dataNostr)
			require.NoError(t, err)
			require.NotNil(t, e2)
			require.Equal(t, envelopeSubzero.Events[0].Event.Content, e2.(*EventEnvelope).Events[0].Content)
			require.Equal(t, envelopeSubzero.Events[0].Event.Tags, e2.(*EventEnvelope).Events[0].Tags)
			require.Equal(t, envelopeSubzero.Events[0].Event.CreatedAt, e2.(*EventEnvelope).Events[0].CreatedAt)
			require.Equal(t, envelopeSubzero.Events[0].Event.Kind, e2.(*EventEnvelope).Events[0].Kind)
		})
	})
	t.Run("MultipleEventsNoSubscriptionID", func(t *testing.T) {
		envelope := EventEnvelope{
			Events: []*Event{
				{
					Event: nostr.Event{
						Content:   "foo",
						CreatedAt: 1,
						Kind:      nostr.KindTextNote,
					},
				},
				{
					Event: nostr.Event{
						Content:   "bar",
						CreatedAt: 2,
						Kind:      nostr.KindTorrent,
					},
				},
			},
		}

		t.Run("EncodeDecode", func(t *testing.T) {
			data, err := envelope.MarshalJSON()
			require.NoError(t, err)
			t.Logf("data: %s", string(data))
			require.NotEmpty(t, data)

			e, err := ParseMessage(data)
			require.NoError(t, err)
			require.NotNil(t, e)
			require.IsType(t, &EventEnvelope{}, e)
			require.Nil(t, e.(*EventEnvelope).SubscriptionID)
			require.Len(t, e.(*EventEnvelope).Events, 2)
		})
	})
	t.Run("MultipleEvents", func(t *testing.T) {
		subID := "subscription ID"
		envelope := EventEnvelope{
			SubscriptionID: &subID,
			Events: []*Event{
				{
					Event: nostr.Event{
						Content:   "foo",
						CreatedAt: 1,
						Kind:      nostr.KindTextNote,
					},
				},
				{
					Event: nostr.Event{
						Content:   "bar",
						CreatedAt: 2,
						Kind:      nostr.KindTorrent,
					},
				},
			},
		}

		t.Run("EncodeDecode", func(t *testing.T) {
			data, err := envelope.MarshalJSON()
			require.NoError(t, err)
			t.Logf("data: %s", string(data))
			require.NotEmpty(t, data)

			e, err := ParseMessage(data)
			require.NoError(t, err)
			require.NotNil(t, e)
			require.IsType(t, &EventEnvelope{}, e)
			require.Len(t, e.(*EventEnvelope).Events, 2)
			require.Equal(t, &subID, e.(*EventEnvelope).SubscriptionID)
		})
	})
}
