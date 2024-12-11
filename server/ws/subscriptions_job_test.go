// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	_ "embed"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/dvm"
	"github.com/ice-blockchain/subzero/model"
)

func helperNewFilter(t *testing.T, filters ...model.Filter) string {
	t.Helper()

	data, err := json.Marshal(filters)
	require.NoError(t, err)

	return string(data)
}

func helperWaitFor[T any](t *testing.T, ch <-chan T, deadline time.Duration) T {
	t.Helper()

	select {
	case v := <-ch:
		return v

	case <-time.After(deadline):
		t.Fatalf("timeout")
	}

	var zero T
	return zero
}

func TestJobOnline(t *testing.T) {
	jobResults := make(chan *model.Event, 1)

	RegisterWSSubscriptionListener(func(ctx context.Context, s *model.Subscription) query.EventIterator {
		return query.GetStoredEvents(ctx, s)
	})
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		for _, ev := range events {
			if ev.Kind == model.KindDVMCountResponse {
				jobResults <- ev
			}
		}
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))
		require.NoError(t, dvm.AcceptJob(ctx, events[0]))

		return nil
	})

	ctx := context.Background()
	privkey := model.GeneratePrivateKey()
	servicePubkey, err := dvm.PublicKey()
	require.NoError(t, err)
	relay := helperMustNewRelay(t, pubsubServers[0])

	article1 := &model.Event{
		Event: nostr.Event{
			CreatedAt: 1,
			Kind:      nostr.KindTextNote,
			Content:   "dummy content 1",
		},
	}
	article2 := &model.Event{
		Event: nostr.Event{
			CreatedAt: 2,
			Kind:      nostr.KindArticle,
			Tags:      model.Tags{model.Tag{"title", "dummy"}},
			Content:   "dummy content 2",
		},
	}
	t.Run("Send articles", func(t *testing.T) {
		helperSignWithMinLeadingZeroBits(t, article1, privkey)
		helperSignWithMinLeadingZeroBits(t, article2, privkey)
		require.NoError(t, relay.PublishMany(ctx, &article1.Event, &article2.Event))
	})

	reaction1 := &model.Event{
		Event: nostr.Event{
			CreatedAt: 3,
			Kind:      nostr.KindReaction,
			Tags: model.Tags{
				model.Tag{"e", article1.ID, "relay"},
				model.Tag{"p", article1.PubKey},
				model.Tag{"k", strconv.Itoa(article1.Kind)},
			},
			Content: "+",
		},
	}
	reaction2 := &model.Event{
		Event: nostr.Event{
			CreatedAt: 4,
			Kind:      nostr.KindReaction,
			Tags: model.Tags{
				model.Tag{"e", article1.ID, "relay"},
				model.Tag{"p", article1.PubKey},
				model.Tag{"k", strconv.Itoa(article1.Kind)},
			},
			Content: "-",
		},
	}
	t.Run("send reactions", func(t *testing.T) {
		helperSignWithMinLeadingZeroBits(t, reaction1, privkey)
		helperSignWithMinLeadingZeroBits(t, reaction2, privkey)
		require.NoError(t, relay.PublishMany(ctx, &reaction1.Event, &reaction2.Event))
	})
	t.Run("send dvm search nostr count job for author filter", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: 5,
				Kind:      model.KindJobNostrEventCount,
				Tags: model.Tags{
					model.Tag{"param", "relay", pubsubServers[0].Endpoint()},
					model.Tag{"p", servicePubkey},
					model.Tag{"relays", pubsubServers[0].Endpoint()},
				},
				Content: helperNewFilter(t, model.Filter{Search: "foo", Authors: []string{article1.PubKey}}),
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, model.GeneratePrivateKey())
		require.NoError(t, relay.Publish(ctx, ev.Event))
		resp := helperWaitFor(t, jobResults, time.Second)
		t.Logf("received DVM response: %+v", resp)
		require.Equal(t, ev.String(), resp.GetTag("request").Value())
		require.Equal(t, "4", resp.Content) // 2 reactions + 2 articles.
	})
	t.Run("send dvm search nostr count job for kinds and #e filter groupped by content", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: 6,
				Kind:      model.KindJobNostrEventCount,
				Tags: model.Tags{
					model.Tag{"param", "relay", pubsubServers[0].Endpoint()},
					model.Tag{"p", servicePubkey},
					model.Tag{"param", "group", "content"},
					model.Tag{"relays", pubsubServers[0].Endpoint()},
				},
				Content: helperNewFilter(t, model.Filter{
					Kinds: []int{nostr.KindReaction},
					Tags:  model.TagMap{}.SetLiterals("e", article1.ID),
				}),
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, model.GeneratePrivateKey())
		require.NoError(t, relay.Publish(ctx, ev.Event))
		resp := helperWaitFor(t, jobResults, time.Second)
		t.Logf("received DVM response: %+v", resp)
		require.Equal(t, ev.String(), resp.GetTag("request").Value())
		require.JSONEq(t, `{"total":2}`, resp.Content)
	})
	t.Run("send dvm search nostr count job with 0 result for group", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: 7,
				Kind:      model.KindJobNostrEventCount,
				Tags: model.Tags{
					model.Tag{"param", "relay", pubsubServers[0].Endpoint()},
					model.Tag{"p", servicePubkey},
					model.Tag{"param", "group", "pubkey"},
					model.Tag{"relays", pubsubServers[0].Endpoint()},
				},
				Content: helperNewFilter(t, model.Filter{
					Search: "foo",
					Kinds:  []int{nostr.KindArticle},
					Tags:   model.TagMap{}.SetLiterals("title", "dummy"),
				}),
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, model.GeneratePrivateKey())
		require.NoError(t, relay.Publish(ctx, ev.Event))
		resp := helperWaitFor(t, jobResults, time.Second)
		t.Logf("received DVM response: %+v", resp)
		require.Equal(t, ev.String(), resp.GetTag("request").Value())
		require.Equal(t, "1", resp.Content) // 1 article.
	})
	time.Sleep(time.Second)
	helperMustCloseRelay(t, relay)
}

func TestJobDeletion(t *testing.T) {
	jobResults := make(chan *model.Event, 1)

	RegisterWSSubscriptionListener(func(ctx context.Context, s *model.Subscription) query.EventIterator {
		return query.GetStoredEvents(ctx, s)
	})
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		for _, ev := range events {
			if ev.Kind == 7000 {
				jobResults <- ev
			}
		}
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))
		require.NoError(t, dvm.AcceptJob(ctx, events[0]))

		return nil
	})

	ctx := context.Background()
	privkey := model.GeneratePrivateKey()
	servicePubkey, err := dvm.PublicKey()
	require.NoError(t, err)
	relay := helperMustNewRelay(t, pubsubServers[0])

	jobReq := &model.Event{
		Event: nostr.Event{
			Kind: model.KindJobNostrEventCount,
			Tags: model.Tags{
				model.Tag{"p", servicePubkey},
				model.Tag{"param", "group", "pubkey"},
				model.Tag{"relays", pubsubServers[0].Endpoint()},
			},
			Content: helperNewFilter(t, model.Filter{
				Search: "foo",
				Kinds:  []int{nostr.KindArticle},
				Tags:   model.TagMap{}.SetLiterals("title", "dummy"),
			}),
		},
	}
	helperSignWithMinLeadingZeroBits(t, jobReq, privkey)
	require.NoError(t, relay.Publish(ctx, jobReq.Event))

	time.Sleep(time.Microsecond)

	jobStop := &model.Event{
		Event: nostr.Event{
			Kind: nostr.KindDeletion,
			Tags: model.Tags{
				model.Tag{"e", jobReq.ID},
				model.Tag{"k", strconv.Itoa(jobReq.Kind)},
			},
		},
	}
	helperSignWithMinLeadingZeroBits(t, jobStop, privkey)
	require.NoError(t, relay.Publish(ctx, jobStop.Event))

	time.Sleep(time.Second)
	resp := helperWaitFor(t, jobResults, time.Second)
	t.Logf("received DVM response: %+v", resp)
	require.Equal(t, resp.GetTag("status").Value(), model.JobFeedbackStatusError)
	helperMustCloseRelay(t, relay)
}

func TestErrorFeedback(t *testing.T) {
	t.Skip("TODO: figure out how to simulate error feedback")

	jobResults := make(chan *model.Event, 1)

	RegisterWSSubscriptionListener(func(ctx context.Context, s *model.Subscription) query.EventIterator {
		return helperNewIterator(t, []*model.Event{})
	})
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		for _, ev := range events {
			t.Logf("received event: %+v", ev)
			if ev.Kind == 7000 {
				jobResults <- ev
			}
		}
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))
		require.NoError(t, dvm.AcceptJob(ctx, events[0]))

		return nil
	})

	ctx := context.Background()
	privkey := model.GeneratePrivateKey()
	servicePubkey, err := dvm.PublicKey()
	require.NoError(t, err)
	relay := helperMustNewRelay(t, pubsubServers[0])

	jobReq := &model.Event{
		Event: nostr.Event{
			Kind: model.KindJobNostrEventCount,
			Tags: model.Tags{
				model.Tag{"p", servicePubkey},
				model.Tag{"param", "relay", "wss://somerandomrelay"},
				model.Tag{"param", "relay", pubsubServers[0].Endpoint()},
				model.Tag{"param", "relay", pubsubServers[0].Endpoint()},
				model.Tag{"param", "group", "pubkey"},
				model.Tag{"relays", pubsubServers[0].Endpoint()},
			},
			Content: helperNewFilter(t, model.Filter{
				Kinds: []int{nostr.KindArticle},
				Tags:  model.TagMap{}.SetLiterals("title", "dummy"),
			}),
		},
	}
	helperSignWithMinLeadingZeroBits(t, jobReq, privkey)
	require.NoError(t, relay.Publish(ctx, jobReq.Event))

	time.Sleep(time.Second)
	resp := helperWaitFor(t, jobResults, time.Second)
	t.Logf("received DVM response: %+v", resp)
	require.Equal(t, resp.GetTag("status").Value(), model.JobFeedbackStatusError)
	helperMustCloseRelay(t, relay)
}

func TestJobOffline(t *testing.T) {
	jobResults := make(chan *model.Event, 1)

	RegisterWSSubscriptionListener(func(ctx context.Context, s *model.Subscription) query.EventIterator {
		return query.GetStoredEvents(ctx, s)
	})
	RegisterWSEventListener(func(ctx context.Context, events ...*model.Event) error {
		for _, ev := range events {
			if ev.Kind == model.KindDVMCountResponse {
				jobResults <- ev
			}
		}
		require.True(t, len(events) > 0)
		require.NoError(t, query.AcceptEvents(ctx, events...))
		require.NoError(t, dvm.AcceptJob(ctx, events[0]))

		return nil
	})

	ctx := context.Background()
	privkey := model.GeneratePrivateKey()
	servicePubkey, err := dvm.PublicKey()
	require.NoError(t, err)
	relay := helperMustNewRelay(t, pubsubServers[0])

	article1 := &model.Event{
		Event: nostr.Event{
			CreatedAt: 1,
			Kind:      nostr.KindTextNote,
			Content:   "dummy content 1",
		},
	}
	article2 := &model.Event{
		Event: nostr.Event{
			CreatedAt: 2,
			Kind:      nostr.KindArticle,
			Tags:      model.Tags{model.Tag{"title", "dummy"}},
			Content:   "dummy content 2",
		},
	}
	t.Run("Send articles", func(t *testing.T) {
		helperSignWithMinLeadingZeroBits(t, article1, privkey)
		helperSignWithMinLeadingZeroBits(t, article2, privkey)
		require.NoError(t, relay.PublishMany(ctx, &article1.Event, &article2.Event))
	})

	reaction1 := &model.Event{
		Event: nostr.Event{
			CreatedAt: 3,
			Kind:      nostr.KindReaction,
			Tags: model.Tags{
				model.Tag{"e", article1.ID, "relay"},
				model.Tag{"p", article1.PubKey},
				model.Tag{"k", strconv.Itoa(article1.Kind)},
			},
			Content: "+",
		},
	}
	reaction2 := &model.Event{
		Event: nostr.Event{
			CreatedAt: 4,
			Kind:      nostr.KindReaction,
			Tags: model.Tags{
				model.Tag{"e", article1.ID, "relay"},
				model.Tag{"p", article1.PubKey},
				model.Tag{"k", strconv.Itoa(article1.Kind)},
			},
			Content: "-",
		},
	}
	t.Run("send reactions", func(t *testing.T) {
		helperSignWithMinLeadingZeroBits(t, reaction1, privkey)
		helperSignWithMinLeadingZeroBits(t, reaction2, privkey)
		require.NoError(t, relay.PublishMany(ctx, &reaction1.Event, &reaction2.Event))
	})
	t.Run("send dvm search nostr count job for author filter", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: 5,
				Kind:      model.KindJobNostrEventCount,
				Tags: model.Tags{
					model.Tag{"p", servicePubkey},
					model.Tag{"relays", pubsubServers[0].Endpoint()},
				},
				Content: helperNewFilter(t, model.Filter{Search: "foo", Authors: []string{article1.PubKey}}),
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, model.GeneratePrivateKey())
		require.NoError(t, relay.Publish(ctx, ev.Event))
		resp := helperWaitFor(t, jobResults, time.Second)
		t.Logf("received DVM response: %+v", resp)
		require.Equal(t, ev.String(), resp.GetTag("request").Value())
		require.Equal(t, "4", resp.Content) // 2 reactions + 2 articles.
	})
	t.Run("send dvm search nostr count job for kinds and #e filter groupped by content", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: 6,
				Kind:      model.KindJobNostrEventCount,
				Tags: model.Tags{
					model.Tag{"p", servicePubkey},
					model.Tag{"param", "group", "content"},
					model.Tag{"relays", pubsubServers[0].Endpoint()},
				},
				Content: helperNewFilter(t, model.Filter{
					Kinds: []int{nostr.KindReaction},
					Tags:  model.TagMap{}.SetLiterals("e", article1.ID),
				}),
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, model.GeneratePrivateKey())
		require.NoError(t, relay.Publish(ctx, ev.Event))
		resp := helperWaitFor(t, jobResults, time.Second)
		t.Logf("received DVM response: %+v", resp)
		require.Equal(t, ev.String(), resp.GetTag("request").Value())
		require.JSONEq(t, `{"+":1,"-":1}`, resp.Content)
	})
	t.Run("send dvm search nostr count job with 0 result for group", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: 7,
				Kind:      model.KindJobNostrEventCount,
				Tags: model.Tags{
					model.Tag{"p", servicePubkey},
					model.Tag{"param", "group", "pubkey"},
					model.Tag{"relays", pubsubServers[0].Endpoint()},
				},
				Content: helperNewFilter(t, model.Filter{
					Search:  "foo",
					Kinds:   []int{nostr.KindArticle},
					Authors: []string{article1.PubKey},
					Tags:    model.TagMap{}.SetLiterals("title", "dummy"),
				}),
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, model.GeneratePrivateKey())
		require.NoError(t, relay.Publish(ctx, ev.Event))
		resp := helperWaitFor(t, jobResults, time.Second)
		t.Logf("received DVM response: %+v", resp)
		require.Equal(t, ev.String(), resp.GetTag("request").Value())
		require.Equal(t, "1", resp.Content) // 1 article.
	})
	time.Sleep(time.Second)
	helperMustCloseRelay(t, relay)
}
