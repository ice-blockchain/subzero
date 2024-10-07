package model

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func makePtr[T any](b T) *T {
	return &b
}

func TestEnvelopeEncodeDecodeWithNostr(t *testing.T) {
	t.Run(string(EnvelopeTypeCount), func(t *testing.T) {
		t.Run("MarshalRequest", func(t *testing.T) {
			e := &CountEnvelope{
				SubscriptionID: "sub",
				Filters: Filters{
					{
						Expiration: makePtr(true),
						Filter: nostr.Filter{
							IDs:   []string{"1"},
							Kinds: []int{2},
							Tags: nostr.TagMap{
								"tag": []string{"foo"},
							},
						},
					},
					{
						Quotes: makePtr(false),
						Filter: nostr.Filter{IDs: []string{"2"}, Kinds: []int{3}},
					},
				},
			}
			data, err := e.MarshalJSON()
			require.NoError(t, err)
			t.Logf("data: %s", string(data))

			e2 := &nostr.CountEnvelope{}
			err = e2.UnmarshalJSON(data)
			require.NoError(t, err)
			require.Equal(t, e.Count, e2.Count)
			require.Equal(t, e.SubscriptionID, e2.SubscriptionID)
			require.Equal(t, e.Filters[0].Filter, e2.Filters[0])

			e3 := &CountEnvelope{}
			err = e3.UnmarshalJSON(data)
			require.NoError(t, err)
			require.Equal(t, e, e3)
		})
		t.Run("MarshalResponse", func(t *testing.T) {
			e := &CountEnvelope{Count: makePtr(int64(1)), SubscriptionID: "sub"}
			data, err := e.MarshalJSON()
			require.NoError(t, err)
			t.Logf("data: %s", string(data))
			e2 := &nostr.CountEnvelope{}
			err = e2.UnmarshalJSON(data)
			require.NoError(t, err)
			require.Equal(t, e.Count, e2.Count)
			require.Equal(t, e.SubscriptionID, e2.SubscriptionID)
		})
	})
	t.Run(string(EnvelopeTypeReq), func(t *testing.T) {
		t.Run("MarshalRequest", func(t *testing.T) {
			e := &ReqEnvelope{
				SubscriptionID: "sub",
				Filters: Filters{
					{
						Expiration: makePtr(true),
						Filter: nostr.Filter{
							IDs:   []string{"1"},
							Kinds: []int{2},
							Tags: nostr.TagMap{
								"tag": []string{"foo"},
							},
						},
					},
					{
						Quotes: makePtr(false),
						Filter: nostr.Filter{IDs: []string{"2"}, Kinds: []int{3}},
					},
				},
			}
			data, err := e.MarshalJSON()
			require.NoError(t, err)
			t.Logf("data: %s", string(data))

			e2 := &nostr.ReqEnvelope{}
			err = e2.UnmarshalJSON(data)
			require.NoError(t, err)
			require.Equal(t, e.SubscriptionID, e2.SubscriptionID)
			require.Equal(t, e.Filters[0].Filter, e2.Filters[0])

			e3 := &ReqEnvelope{}
			err = e3.UnmarshalJSON(data)
			require.NoError(t, err)
			require.Equal(t, e, e3)
		})
		t.Run("MarshalResponse", func(t *testing.T) {
			e := &ReqEnvelope{SubscriptionID: "sub", Filters: Filters{{Filter: nostr.Filter{Authors: []string{"foo"}, Tags: TagMap{"e": []string{"f"}}}}}}
			data, err := e.MarshalJSON()
			require.NoError(t, err)
			t.Logf("data: %s", string(data))
			e2 := &nostr.ReqEnvelope{}
			err = e2.UnmarshalJSON(data)
			require.NoError(t, err)
			require.Equal(t, e.SubscriptionID, e2.SubscriptionID)
			require.Equal(t, e.Filters[0].Filter, e2.Filters[0])
		})
	})
}
