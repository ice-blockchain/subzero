// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	_ "embed"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
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
		t.Fatalf("timeout exceeded waiting for value: %v", deadline)
	}

	var zero T
	return zero
}

func helperQueryEvents(t *testing.T, ctx context.Context, relay *nostrRelay, filter model.Filter) []*model.Event {
	nResults, err := relay.QuerySync(ctx, filter)
	require.NoError(t, err)
	results := make([]*model.Event, 0, len(nResults))
	for _, r := range nResults {
		results = append(results, &model.Event{Event: *r})
	}
	return results
}

func TestJobOnline(t *testing.T) {
	jobResults := make(chan *model.Event, 1)

	RegisterWSSubscriptionListener(func(ctx context.Context, s *model.Subscription) EventIterator {
		return query.GetStoredEvents(ctx, s)
	}, dvm.GetStoredEvents)
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
			Tags:      model.Tags{{"title", "dummy"}, {"d", "foo"}},
			Content:   "dummy content 2",
		},
	}
	t.Run("Send articles", func(t *testing.T) {
		helperSignWithMinLeadingZeroBits(t, article1, privkey)
		helperSignWithMinLeadingZeroBits(t, article2, privkey)
		require.NoError(t, relay.PublishMany(t.Context(), &article1.Event, &article2.Event))
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
		require.NoError(t, relay.PublishMany(t.Context(), &reaction1.Event, &reaction2.Event))
	})
	responses := make([]*model.Event, 0)
	commonUserForFirst2Reqs := model.GeneratePrivateKey()
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
		helperSignWithMinLeadingZeroBits(t, ev, commonUserForFirst2Reqs)
		require.NoError(t, relay.Publish(t.Context(), ev.Event))
		resp := helperWaitFor(t, jobResults, time.Minute)
		responses = append(responses, resp)
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
		helperSignWithMinLeadingZeroBits(t, ev, commonUserForFirst2Reqs)
		require.NoError(t, relay.Publish(t.Context(), ev.Event))
		resp := helperWaitFor(t, jobResults, time.Minute)
		responses = append(responses, resp)
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
		require.NoError(t, relay.Publish(t.Context(), ev.Event))
		resp := helperWaitFor(t, jobResults, time.Minute)
		responses = append(responses, resp)
		t.Logf("received DVM response: %+v", resp)
		require.Equal(t, ev.String(), resp.GetTag("request").Value())
		require.Equal(t, "1", resp.Content) // 1 article.
	})
	t.Run("request dvm result via subscription with #e", func(t *testing.T) {
		eTag1 := responses[1].GetTag("e").Value()
		eTag2 := responses[2].GetTag("e").Value()
		dvmSearchResults := helperQueryEvents(t, t.Context(), relay,
			model.Filter{Kinds: []int{model.KindDVMCountResponse}, Tags: model.TagMap{}.
				Append("e", &eTag1).
				Append("e", &eTag2),
			})
		require.Len(t, dvmSearchResults, 2)
		require.Contains(t, dvmSearchResults, responses[1])
		require.Contains(t, dvmSearchResults, responses[2])
	})
	t.Run("request dvm result via subscription with #p, first 2 requests came from same user", func(t *testing.T) {
		pTag := responses[0].GetTag("p").Value()
		dvmSearchResults := helperQueryEvents(t, t.Context(), relay,
			model.Filter{Kinds: []int{model.KindDVMCountResponse}, Tags: model.TagMap{}.
				Append("p", &pTag),
			})
		require.Len(t, dvmSearchResults, 2)
		require.Contains(t, dvmSearchResults, responses[0])
		require.Contains(t, dvmSearchResults, responses[1])
	})
	t.Run("request dvm result via subscription with #p and #e", func(t *testing.T) {
		pTag := responses[1].GetTag("p").Value()
		eTag1 := responses[1].GetTag("e").Value()
		dvmSearchResults := helperQueryEvents(t, t.Context(), relay,
			model.Filter{Kinds: []int{model.KindDVMCountResponse}, Tags: model.TagMap{}.
				Append("p", &pTag).
				Append("e", &eTag1),
			})
		require.Len(t, dvmSearchResults, 1)
		require.Equal(t, []*model.Event{responses[1]}, dvmSearchResults)
	})
	time.Sleep(time.Second)
	helperMustCloseRelay(t, relay)
}

func TestJobDeletion(t *testing.T) {
	jobResults := make(chan *model.Event, 1)
	wake := make(chan struct{}, 1)

	RegisterWSSubscriptionListener(func(ctx context.Context, s *model.Subscription) EventIterator {
		select {
		case <-wake:
		case <-time.After(time.Second * 10):
		case <-ctx.Done():
		}
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

	privkey, pubkey := model.GenerateKeyPair()
	servicePubkey, err := dvm.PublicKey()
	require.NoError(t, err)
	relay := helperMustNewRelay(t, pubsubServers[0])

	jobReq := &model.Event{
		Event: nostr.Event{
			Kind: model.KindJobNostrEventCount,
			Tags: model.Tags{
				model.Tag{"p", servicePubkey},
				model.Tag{"param", "group", "pubkey"},
				model.Tag{"param", "relay", pubsubServers[1].Endpoint()},
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
	require.NoError(t, relay.Publish(t.Context(), jobReq.Event))

	jobStop := &model.Event{
		Event: nostr.Event{
			Kind: nostr.KindDeletion,
			Tags: model.Tags{
				{"e", jobReq.ID},
				{"k", strconv.Itoa(jobReq.Kind)},
				{"p", jobReq.PubKey},
				{model.CustomIONTagOnBehalfOf, pubkey},
			},
		},
	}
	helperSignWithMinLeadingZeroBits(t, jobStop, privkey)
	require.NoError(t, relay.Publish(t.Context(), jobStop.Event))

	resp := helperWaitFor(t, jobResults, time.Minute)
	wake <- struct{}{}
	t.Logf("received DVM response: %+v", resp)
	require.Equal(t, resp.GetTag("status").Value(), model.JobFeedbackStatusError)
	helperMustCloseRelay(t, relay)
}

func TestErrorFeedback(t *testing.T) {
	t.Skip("TODO: figure out how to simulate error feedback")

	jobResults := make(chan *model.Event, 1)

	RegisterWSSubscriptionListener(func(ctx context.Context, s *model.Subscription) EventIterator {
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
	require.NoError(t, relay.Publish(t.Context(), jobReq.Event))

	time.Sleep(time.Second)
	resp := helperWaitFor(t, jobResults, time.Minute)
	t.Logf("received DVM response: %+v", resp)
	require.Equal(t, resp.GetTag("status").Value(), model.JobFeedbackStatusError)
	helperMustCloseRelay(t, relay)
}

func TestJobOffline(t *testing.T) {
	jobResults := make(chan *model.Event, 1)

	RegisterWSSubscriptionListener(func(ctx context.Context, s *model.Subscription) EventIterator {
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
			Tags: model.Tags{
				{"title", "dummy"},
				{"d", "foo"},
			},
			Content: "dummy content 2",
		},
	}
	t.Run("Send articles", func(t *testing.T) {
		helperSignWithMinLeadingZeroBits(t, article1, privkey)
		helperSignWithMinLeadingZeroBits(t, article2, privkey)
		require.NoError(t, relay.PublishMany(t.Context(), &article1.Event, &article2.Event))
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
		require.NoError(t, relay.PublishMany(t.Context(), &reaction1.Event, &reaction2.Event))
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
		require.NoError(t, relay.Publish(t.Context(), ev.Event))
		resp := helperWaitFor(t, jobResults, time.Minute)
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
		require.NoError(t, relay.Publish(t.Context(), ev.Event))
		resp := helperWaitFor(t, jobResults, time.Minute)
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
		require.NoError(t, relay.Publish(t.Context(), ev.Event))
		resp := helperWaitFor(t, jobResults, time.Minute)
		t.Logf("received DVM response: %+v", resp)
		require.Equal(t, ev.String(), resp.GetTag("request").Value())
		require.Equal(t, "1", resp.Content) // 1 article.
	})
	time.Sleep(time.Second)
	helperMustCloseRelay(t, relay)
}

func TestJobMembersCount_OpenCommunity(t *testing.T) {
	jobResults := make(chan *model.Event, 1)

	RegisterWSSubscriptionListener(query.GetStoredEvents, dvm.GetStoredEvents)
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

	relay := helperMustNewRelay(t, pubsubServers[0])
	hVal, err := uuid.NewV7()
	require.NoError(t, err)
	communityID := hVal.String()
	privkeyOwner, pubkeyCommunityOwner := model.GenerateKeyPair()
	privkeyUser1, pubkeyUser1 := model.GenerateKeyPair()

	t.Run("define open community definition", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"name", "some name"},
					{"description", "some description"},
					{"open"},
					{"d", "dtagvalue"},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(t.Context(), ev.Event))
	})
	t.Run("join owner to the community", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    pubkeyCommunityOwner,
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyCommunityOwner},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, &ev, privkeyOwner)
		require.NoError(t, relay.Publish(t.Context(), ev.Event))
	})
	t.Run("join user1 to the community", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    pubkeyUser1,
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser1},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, &ev, privkeyUser1)
		require.NoError(t, relay.Publish(t.Context(), ev.Event))
	})
	helperCountMembers(t, relay, jobResults, communityID, 2)

	time.Sleep(time.Second)
	helperMustCloseRelay(t, relay)
}

func TestJobMembersCount_ClosedCommunity(t *testing.T) {
	jobResults := make(chan *model.Event, 1)

	RegisterWSSubscriptionListener(query.GetStoredEvents, dvm.GetStoredEvents)
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

	relay := helperMustNewRelay(t, pubsubServers[0])
	hVal, err := uuid.NewV7()
	require.NoError(t, err)
	communityID := hVal.String()
	privkeyOwner, pubkeyCommunityOwner := model.GenerateKeyPair()
	privkeyUser1, pubkeyUser1 := model.GenerateKeyPair()

	t.Run("define closed community", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"name", "some name"},
					{"description", "some description"},
					{"closed"},
					{"d", "dtagvalue"},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(t.Context(), ev.Event))
	})
	t.Run("join owner to the community", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    pubkeyCommunityOwner,
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyCommunityOwner},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, &ev, privkeyOwner)
		require.NoError(t, relay.Publish(t.Context(), ev.Event))
	})
	t.Run("invite user1 by owner", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser1},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(t.Context(), ev.Event))
	})
	t.Run("accept invitation by user1", func(t *testing.T) {
		ownerAuthorizationEvent := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser1},
					{"expiration", strconv.FormatInt(time.Now().Add(1*time.Hour).Unix(), 10)},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ownerAuthorizationEvent, privkeyOwner)
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser1},
					{"authorization", ownerAuthorizationEvent.String()},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyUser1)
		require.NoError(t, relay.Publish(t.Context(), ev.Event))
	})
	helperCountMembers(t, relay, jobResults, communityID, 2)

	time.Sleep(time.Second)
	helperMustCloseRelay(t, relay)
}

func TestJobMembersCount_CommunityDefinitionChanged(t *testing.T) {
	jobResults := make(chan *model.Event, 1)

	RegisterWSSubscriptionListener(query.GetStoredEvents, dvm.GetStoredEvents)
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

	relay := helperMustNewRelay(t, pubsubServers[0])
	hVal, err := uuid.NewV7()
	require.NoError(t, err)
	communityID := hVal.String()
	privkeyOwner, pubkeyCommunityOwner := model.GenerateKeyPair()
	privkeyUser1, pubkeyUser1 := model.GenerateKeyPair()
	privkeyUser2, pubkeyUser2 := model.GenerateKeyPair()
	_, pubkeyUser3 := model.GenerateKeyPair()

	t.Run("define open community definition", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"name", "some name"},
					{"description", "some description"},
					{"open"},
					{"d", "dtagvalue"},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(t.Context(), ev.Event))
	})
	t.Run("join owner to the community", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    pubkeyCommunityOwner,
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyCommunityOwner},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, &ev, privkeyOwner)
		require.NoError(t, relay.Publish(t.Context(), ev.Event))
	})
	var joinUser1Event model.Event
	t.Run("join user1 to the community", func(t *testing.T) {
		joinUser1Event = model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    pubkeyUser1,
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser1},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, &joinUser1Event, privkeyUser1)
		require.NoError(t, relay.Publish(t.Context(), joinUser1Event.Event))
	})

	helperCountMembers(t, relay, jobResults, communityID, 2)

	t.Run("change community definition to closed", func(t *testing.T) {
		ev := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityChangeDefinition,
				Tags: model.Tags{
					{"h", communityID},
					{"name", "some name"},
					{"description", "some description"},
					{"closed"},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ev, privkeyOwner)
		require.NoError(t, relay.Publish(t.Context(), ev.Event))
	})
	t.Run("invite user2 to the closed community", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    pubkeyUser2,
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser2},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, &ev, privkeyOwner)
		require.NoError(t, relay.Publish(t.Context(), ev.Event))
	})

	helperCountMembers(t, relay, jobResults, communityID, 2)

	var joinUser2Event model.Event
	t.Run("accept invitation by user2 after changed openess community option", func(t *testing.T) {
		ownerAuthorizationEvent := &model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser2},
					{"expiration", strconv.FormatInt(time.Now().Add(1*time.Hour).Unix(), 10)},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, ownerAuthorizationEvent, privkeyOwner)
		joinUser2Event = model.Event{
			Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser2},
					{"authorization", ownerAuthorizationEvent.String()},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, &joinUser2Event, privkeyUser2)
		require.NoError(t, relay.Publish(t.Context(), joinUser2Event.Event))
	})

	helperCountMembers(t, relay, jobResults, communityID, 3)

	t.Run("delete user2 from the community", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    pubkeyUser2,
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindDeletion,
				Tags: model.Tags{
					{"e", joinUser2Event.GetID()},
					{"b", pubkeyUser2},
					{"k", strconv.FormatInt(model.CustomIONKindCommunityJoin, 10)},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, &ev, privkeyUser2)
		require.NoError(t, relay.Publish(t.Context(), ev.Event))
	})

	helperCountMembers(t, relay, jobResults, communityID, 2)

	t.Run("delete user1 from the community", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				PubKey:    pubkeyUser1,
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindDeletion,
				Tags: model.Tags{
					{"e", joinUser1Event.GetID()},
					{"b", pubkeyUser1},
					{"k", strconv.FormatInt(model.CustomIONKindCommunityJoin, 10)},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, &ev, privkeyUser1)
		require.NoError(t, relay.Publish(t.Context(), ev.Event))
	})

	helperCountMembers(t, relay, jobResults, communityID, 1)

	var inviteUser3Event model.Event
	t.Run("invite user3 to the closed community", func(t *testing.T) {
		inviteUser3Event = model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindCommunityJoin,
				Tags: model.Tags{
					{"h", communityID},
					{"p", pubkeyUser3},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, &inviteUser3Event, privkeyOwner)
		require.NoError(t, relay.Publish(t.Context(), inviteUser3Event.Event))
	})
	helperCountMembers(t, relay, jobResults, communityID, 1)

	t.Run("delete user3 invitation", func(t *testing.T) {
		ev := model.Event{
			Event: nostr.Event{
				ID:        uuid.NewString(),
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindDeletion,
				Tags: model.Tags{
					{"e", inviteUser3Event.GetID()},
					{"b", pubkeyCommunityOwner},
					{"k", strconv.FormatInt(model.CustomIONKindCommunityJoin, 10)},
				},
			},
		}
		helperSignWithMinLeadingZeroBits(t, &ev, privkeyOwner)
		require.NoError(t, relay.Publish(t.Context(), ev.Event))
	})
	helperCountMembers(t, relay, jobResults, communityID, 1)

	time.Sleep(time.Second)
	helperMustCloseRelay(t, relay)
}

func helperCountMembers(t *testing.T, relay *nostrRelay, jobResults chan *model.Event, communityID string, expectedCount int64) {
	t.Helper()
	responses := make([]*model.Event, 0)
	commonUserForFirst2Reqs := model.GeneratePrivateKey()
	servicePubkey, err := dvm.PublicKey()
	require.NoError(t, err)
	ev := &model.Event{
		Event: nostr.Event{
			CreatedAt: 5,
			Kind:      model.KindJobNostrEventCount,
			Tags: model.Tags{
				model.Tag{"param", "relay", pubsubServers[0].Endpoint()},
				model.Tag{"p", servicePubkey},
				model.Tag{"relays", pubsubServers[0].Endpoint()},
			},
			Content: helperNewFilter(t, model.Filter{Tags: nostr.TagMap{}.SetLiterals("h", communityID), Kinds: []int{model.CustomIONKindCommunityJoin}}),
		},
	}
	helperSignWithMinLeadingZeroBits(t, ev, commonUserForFirst2Reqs)
	require.NoError(t, relay.Publish(t.Context(), ev.Event))
	resp := helperWaitFor(t, jobResults, time.Minute)
	responses = append(responses, resp)
	t.Logf("received DVM response: %+v", resp)
	require.Equal(t, ev.String(), resp.GetTag("request").Value())
	require.Equal(t, strconv.FormatInt(expectedCount, 10), resp.Content)
}
