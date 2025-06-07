// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"testing"

	"github.com/ice-blockchain/subzero/model"
	"github.com/stretchr/testify/require"
)

func TestValidateFundSend(t *testing.T) {
	t.Parallel()

	t.Run("With p", func(t *testing.T) {
		var ev model.Event
		ev.Kind = model.CustomIONKindFundSendNotify
		ev.Tags = model.Tags{
			{"b", "bar"},
			{"network", "ion"},
			{"asset_class", "native"},
			{"asset_address", "localhost"},
		}
		require.NoError(t, ev.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, Validate(t.Context(), &ev))

		ev.Tags = append(ev.Tags, model.Tag{"p", "foo"})
		require.NoError(t, ev.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, Validate(t.Context(), &ev))

		ev.Content = "foo"
		require.NoError(t, ev.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, Validate(t.Context(), &ev))
	})

	t.Run("With l+L", func(t *testing.T) {
		var ev model.Event
		ev.Kind = model.CustomIONKindFundSendNotify
		ev.Tags = model.Tags{
			{"b", "bar"},
			{"l", "1234", "wallet.address"},
			{"network", "ion"},
			{"asset_class", "native"},
			{"asset_address", "localhost"},
		}
		ev.Content = `{"to":"1234"}`
		require.NoError(t, ev.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, Validate(t.Context(), &ev))

		ev.Tags = append(ev.Tags, model.Tag{"L", "wallet.address"})
		require.NoError(t, ev.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, Validate(t.Context(), &ev))

		ev.Tags = append(ev.Tags, model.Tag{"p", "bar"})
		require.NoError(t, ev.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.Error(t, Validate(t.Context(), &ev))
	})
}
