// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/model"
)

func TestValidatePriceChangeNotificationRequest(t *testing.T) {
	t.Parallel()

	validTags := model.Tags{
		{"i", "priceChange"},
		{"param", "timeWindow", "60"},
		{"param", "deltaPercentage", "10"},
		{"param", "token", "5176:0xabc123:"},
	}

	tests := []struct {
		name    string
		tags    model.Tags
		content string
		wantErr bool
	}{
		{
			name:    "valid request",
			tags:    validTags,
			wantErr: false,
		},
		{
			name: "valid request with output tag",
			tags: append(validTags, model.Tag{"output", "application/json"}),
		},
		{
			name: "valid request with negative deltaPercentage",
			tags: model.Tags{
				{"i", "priceChange"},
				{"param", "timeWindow", "60"},
				{"param", "deltaPercentage", "-50"},
				{"param", "token", "5176:0xabc123:"},
			},
		},
		{
			name: "missing i tag",
			tags: model.Tags{
				{"param", "timeWindow", "60"},
				{"param", "deltaPercentage", "10"},
				{"param", "token", "5176:0xabc123:"},
			},
			wantErr: true,
		},
		{
			name: "missing param tags",
			tags: model.Tags{
				{"i", "priceChange"},
			},
			wantErr: true,
		},
		{
			name: "missing timeWindow param",
			tags: model.Tags{
				{"i", "priceChange"},
				{"param", "deltaPercentage", "10"},
				{"param", "token", "5176:0xabc123:"},
			},
			wantErr: true,
		},
		{
			name: "missing deltaPercentage param",
			tags: model.Tags{
				{"i", "priceChange"},
				{"param", "timeWindow", "60"},
				{"param", "token", "5176:0xabc123:"},
			},
			wantErr: true,
		},
		{
			name: "token param is optional",
			tags: model.Tags{
				{"i", "priceChange"},
				{"param", "timeWindow", "60"},
				{"param", "deltaPercentage", "10"},
			},
		},
		{
			name: "timeWindow zero",
			tags: model.Tags{
				{"i", "priceChange"},
				{"param", "timeWindow", "0"},
				{"param", "deltaPercentage", "10"},
				{"param", "token", "5176:0xabc123:"},
			},
			wantErr: true,
		},
		{
			name: "timeWindow negative",
			tags: model.Tags{
				{"i", "priceChange"},
				{"param", "timeWindow", "-1"},
				{"param", "deltaPercentage", "10"},
				{"param", "token", "5176:0xabc123:"},
			},
			wantErr: true,
		},
		{
			name: "timeWindow exceeds maximum (more than 1 year in seconds)",
			tags: model.Tags{
				{"i", "priceChange"},
				{"param", "timeWindow", "31622401"},
				{"param", "deltaPercentage", "10"},
				{"param", "token", "5176:0xabc123:"},
			},
			wantErr: true,
		},
		{
			name: "timeWindow not a number",
			tags: model.Tags{
				{"i", "priceChange"},
				{"param", "timeWindow", "abc"},
				{"param", "deltaPercentage", "10"},
				{"param", "token", "5176:0xabc123:"},
			},
			wantErr: true,
		},
		{
			name: "deltaPercentage zero",
			tags: model.Tags{
				{"i", "priceChange"},
				{"param", "timeWindow", "60"},
				{"param", "deltaPercentage", "0"},
				{"param", "token", "5176:0xabc123:"},
			},
			wantErr: true,
		},
		{
			name: "deltaPercentage greater than 100",
			tags: model.Tags{
				{"i", "priceChange"},
				{"param", "timeWindow", "60"},
				{"param", "deltaPercentage", "101"},
				{"param", "token", "5176:0xabc123:"},
			},
			wantErr: true,
		},
		{
			name: "deltaPercentage less than -100",
			tags: model.Tags{
				{"i", "priceChange"},
				{"param", "timeWindow", "60"},
				{"param", "deltaPercentage", "-101"},
				{"param", "token", "5176:0xabc123:"},
			},
			wantErr: true,
		},
		{
			name: "deltaPercentage not a number",
			tags: model.Tags{
				{"i", "priceChange"},
				{"param", "timeWindow", "60"},
				{"param", "deltaPercentage", "xyz"},
				{"param", "token", "5176:0xabc123:"},
			},
			wantErr: true,
		},
		{
			name: "empty token value",
			tags: model.Tags{
				{"i", "priceChange"},
				{"param", "timeWindow", "60"},
				{"param", "deltaPercentage", "10"},
				{"param", "token", ""},
			},
			wantErr: true,
		},
		{
			name:    "unsupported output value",
			tags:    append(validTags, model.Tag{"output", "text/plain"}),
			wantErr: true,
		},
		{
			name: "unsupported i tag value",
			tags: model.Tags{
				{"i", "unsupportedJob"},
				{"param", "timeWindow", "60"},
				{"param", "deltaPercentage", "10"},
				{"param", "token", "5176:0xabc123:"},
			},
			wantErr: true,
		},
		{
			name: "param tag with less than 3 parts",
			tags: model.Tags{
				{"i", "priceChange"},
				{"param", "timeWindow"},
				{"param", "deltaPercentage", "10"},
				{"param", "token", "5176:0xabc123:"},
			},
			wantErr: true,
		},
		{
			name: "token value without colon format",
			tags: model.Tags{
				{"i", "priceChange"},
				{"param", "timeWindow", "60"},
				{"param", "deltaPercentage", "10"},
				{"param", "token", "ICE"},
			},
			wantErr: true,
		},
		{
			name:    "non-empty content",
			tags:    validTags,
			content: "some content",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ev model.Event

			ev.Kind = model.CustomIONKindDVMJobRequestPriceChange
			ev.Tags = tt.tags
			ev.Content = tt.content
			ev.CreatedAt = nostr.Now()
			require.NoError(t, ev.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519))

			err := Validate(t.Context(), model.Events{&ev})
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
