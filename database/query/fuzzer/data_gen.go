// SPDX-License-Identifier: ice License 1.0

//go:build test

package main

import (
	"context"
	"log"
	"math/rand/v2"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/schollz/progressbar/v3"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

func panicOnErr(err error) {
	if err == nil {
		return
	}

	if errors.IsAny(err, query.ErrRaceCondition) {
		return
	}

	panic(err)
}

func generateRandomString(n int) string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

	if n < 0 {
		panic("invalid length")
	}

	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.IntN(len(letters))]
	}

	return string(b)
}

func createUsers(ctx context.Context, n int) (privateKeys []string) {
	bar := progressbar.Default(int64(n), "creating users")
	defer bar.Finish()
	for i := range n {
		bar.Add(1)

		key := model.GeneratePrivateKey()
		privateKeys = append(privateKeys, key)

		now := model.Timestamp(time.Now().UnixNano())
		metadata := model.ProfileMetadataContent{
			RegisteredAt: now,
			Name:         "User " + strconv.Itoa(i+1),
			About:        "This is user " + strconv.Itoa(i+1) + ".",
		}

		var ev model.Event
		ev.Kind = nostr.KindProfileMetadata
		ev.CreatedAt = now + 1
		ev.Content = metadata.String()

		panicOnErr(ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		panicOnErr(query.AcceptEvents(ctx, &ev))
	}

	return privateKeys
}

func createPosts(ctx context.Context, keys []string, n int) []*model.Event {
	posts := make([]*model.Event, 0, n)
	bar := progressbar.Default(int64(n), "creating posts")
	for range n {
		bar.Add(1)

		hasImage := rand.Int64N(100) < 50
		hasVideo := rand.Int64N(100) < 50
		hasExpiration := rand.Int64N(100) < 50
		key := keys[rand.Int64N(int64(len(keys)))]

		var ev model.Event
		ev.Kind = nostr.KindTextNote
		ev.CreatedAt = model.Timestamp(time.Now().UnixNano())
		ev.Content = "This is a test post with some random content. " + generateRandomString(rand.IntN(100))

		if hasExpiration {
			ev.Tags = append(ev.Tags, model.Tag{"expiration", ev.CreatedAt.Add(time.Hour).String()})
		}

		if hasImage || hasVideo {
			imeta := model.Tag{"imeta"}
			if hasImage {
				imeta = append(imeta, "m image/jpeg")
			}
			if hasVideo {
				imeta = append(imeta, "m video/mp4")
			}
			ev.Tags = append(ev.Tags, imeta)
		}

		panicOnErr(ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		posts = append(posts, &ev)
	}
	bar.Finish()

	bar = progressbar.Default(int64(len(posts)), "inserting posts")
	batches := model.SplitBatch(posts, 100)
	for _, batch := range batches {
		bar.Add(len(batch))
		panicOnErr(query.AcceptEvents(ctx, batch...))
	}
	bar.Finish()

	return posts
}

func createFollowLists(ctx context.Context, keys []string) {
	bar := progressbar.Default(int64(len(keys)), "creating follows lists")
	defer bar.Finish()

	for _, key := range keys {
		bar.Add(1)
		numFollowers := rand.IntN(20) + 5
		var ev model.Event
		ev.Kind = nostr.KindFollowList
		ev.CreatedAt = model.Timestamp(time.Now().UnixNano())
		ev.Content = ""
		selectedKeys := make(map[int]bool)
		for range numFollowers {
			idx := rand.IntN(len(keys))
			if selectedKeys[idx] {
				continue
			}
			selectedKeys[idx] = true

			followerKey := keys[idx]
			pubkey, err := model.GetPublicKey(followerKey)
			if err != nil {
				continue
			}
			ev.Tags = append(ev.Tags, model.Tag{"p", pubkey})
		}
		panicOnErr(ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		panicOnErr(query.AcceptEvents(ctx, &ev))
	}
}

func createPostsReactions(ctx context.Context, keys []string, posts []*model.Event, n int) {
	const (
		reactionLike = iota
		reactionQuote
		reactionRepost
		reactionReply
		reactionMax
	)

	bar := progressbar.Default(int64(n), "creating reactions")
	reactions := make([]*model.Event, 0, n)
	for range n {
		bar.Add(1)

		key := keys[rand.Int64N(int64(len(keys)))]
		post := posts[rand.Int64N(int64(len(posts)))]
		reaction := rand.Int64N(reactionMax)

		var ev model.Event
		ev.CreatedAt = nostr.Now()
		switch reaction {
		case reactionLike:
			ev.Kind = nostr.KindReaction
			ev.Content = "+" + generateRandomString(rand.IntN(10))
			ev.Tags = model.Tags{
				{"p", post.GetMasterPublicKey()},
				{"k", strconv.Itoa(int(post.Kind))},
				{"e", post.ID},
			}
		case reactionQuote:
			ev.Kind = nostr.KindTextNote
			ev.Content = "This is a quote reaction to a post. " + generateRandomString(rand.IntN(100))
			ev.Tags = model.Tags{
				{"p", post.GetMasterPublicKey()},
				{"q", post.ID},
			}
		case reactionRepost:
			ev.Kind = nostr.KindRepost
			ev.Content = post.String()
			ev.Tags = model.Tags{
				{"p", post.GetMasterPublicKey()},
				{"k", strconv.Itoa(int(post.Kind))},
				{"e", post.ID},
			}
		case reactionReply:
			ev.Kind = nostr.KindTextNote
			ev.Content = "This is a reply to a post. " + generateRandomString(rand.IntN(100))
			ev.Tags = model.Tags{
				{"p", post.GetMasterPublicKey()},
				{"e", post.ID, "", "root"},
				{"e", post.ID, "", "reply"},
			}
		default:
			log.Panicf("unknown reaction type: %d", reaction)
		}

		panicOnErr(ev.SignWithAlg(key, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		reactions = append(reactions, &ev)
	}
	bar.Finish()

	bar = progressbar.Default(int64(len(reactions)), "inserting reactions")
	batches := model.SplitBatch(reactions, 100)
	for _, batch := range batches {
		bar.Add(len(batch))
		panicOnErr(query.AcceptEvents(ctx, batch...))
	}
	bar.Finish()
}
