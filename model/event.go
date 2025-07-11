// SPDX-License-Identifier: ice License 1.0

package model

import (
	"cmp"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip13"
)

type (
	Event struct {
		nostr.Event
		Previous *Event `db:"-" json:"-" swaggerignore:"true"` // Previous version of the event, if any.
	}
	Events                  []*Event
	EventSignAlg            string
	EventKeyAlg             string
	EphemeralEmbeddingEvent struct {
		ContentEvent *Event
		*Event
	}
)

const (
	SignAlgSchnorr EventSignAlg = "schnorr"
	SignAlgEDDSA   EventSignAlg = "eddsa"

	KeyAlgSecp256k1  EventKeyAlg = "secp256k1"
	KeyAlgCurve25519 EventKeyAlg = "curve25519"
)

func (e *Event) CheckNIP13Difficulty(minLeadingZeroBits int) error {
	if minLeadingZeroBits == 0 {
		return nil
	}
	if e.Kind >= 6000 && e.Kind <= nostr.KindJobFeedback {
		return nil
	}
	if err := nip13.Check(e.ID, minLeadingZeroBits); err != nil {
		log.Printf("difficulty: %v < %v, id:%v", nip13.Difficulty(e.ID), minLeadingZeroBits, e.ID)

		return err
	}

	return nil
}

func (e *Event) GenerateNIP13(ctx context.Context, minLeadingZeroBits int) error {
	if minLeadingZeroBits == 0 {
		return nil
	}
	tag, err := nip13.DoWork(ctx, e.Event, minLeadingZeroBits)
	if err != nil {
		log.Printf("can't do mining by the provided difficulty:%v", minLeadingZeroBits)

		return err
	}
	e.Tags = append(e.Tags, tag)

	return nil
}

func (e *Event) SignWithAlg(privateKey string, signAlg EventSignAlg, keyAlg EventKeyAlg) error {
	if (signAlg == "" && keyAlg != "") || (signAlg != "" && keyAlg == "") {
		// Both signAlg and keyAlg must be set OR both must be empty.
		return errors.Wrap(ErrUnsupportedAlg, "signature and key algorithms must be set together")
	}

	if e.Tags == nil {
		e.Tags = make(Tags, 0)
	}

	privKey, err := hex.DecodeString(privateKey)
	if err != nil {
		return errors.Wrap(err, "private key is invalid hex")
	}

	var sign []byte
	var headerSum [32]byte
	switch {
	case (signAlg == "" && keyAlg == "") || (signAlg == SignAlgSchnorr && keyAlg == KeyAlgSecp256k1):
		return errors.Wrap(e.Event.Sign(privateKey), "failed to sign event")

	case signAlg == SignAlgEDDSA && keyAlg == KeyAlgCurve25519:
		pk := ed25519.PrivateKey(privKey)
		e.PubKey = hex.EncodeToString(pk.Public().(ed25519.PublicKey))
		headerSum = sha256.Sum256(e.Serialize())
		sign = ed25519.Sign(pk, headerSum[:])

	default:
		return errors.Wrapf(ErrUnsupportedAlg, "signature algorithm: %q, key algorithm: %q", signAlg, keyAlg)
	}

	e.ID = hex.EncodeToString(headerSum[:])
	e.Sig = string(signAlg) + "/" + string(keyAlg) + ":" + hex.EncodeToString(sign)

	return nil
}

func (e *Event) ExtractSignature() (signAlg EventSignAlg, keyAlg EventKeyAlg, sign string, err error) {
	extensionEnd := strings.IndexRune(e.Sig, ':')
	if extensionEnd == -1 {
		sign = e.Sig

		return
	}

	keyStart := strings.IndexRune(e.Sig[:extensionEnd], '/')
	if keyStart == -1 {
		err = errors.Wrap(ErrUnsupportedAlg, "key algorithm is not set")

		return
	}

	signAlg = EventSignAlg(e.Sig[:keyStart])
	keyAlg = EventKeyAlg(e.Sig[keyStart+1 : extensionEnd])
	if signAlg == "" || keyAlg == "" {
		err = errors.Wrap(ErrUnsupportedAlg, "signature and key algorithms must be set together")

		return
	}

	return signAlg, keyAlg, e.Sig[extensionEnd+1:], nil
}

func (e *Event) CheckSignature() (bool, error) {
	signAlg, keyAlg, sign, err := e.ExtractSignature()
	if err != nil {
		return false, errors.Wrap(err, "failed to get signature and key algorithms")
	}

	pk, err := hex.DecodeString(e.PubKey)
	if err != nil {
		return false, errors.Wrap(err, "public key is invalid hex")
	}

	signBytes, err := hex.DecodeString(sign)
	if err != nil {
		return false, errors.Wrap(err, "signature is invalid hex")
	}
	hash := sha256.Sum256(e.Serialize())
	switch {
	case signAlg == SignAlgEDDSA && keyAlg == KeyAlgCurve25519:
		return ed25519.Verify(pk, hash[:], signBytes), nil

	case (signAlg == "" && keyAlg == "") || (signAlg == SignAlgSchnorr && keyAlg == KeyAlgSecp256k1):
		ok, err := e.Event.CheckSignature()

		return ok, errors.Wrap(err, "failed to check schnorr signature")
	}

	return false, errors.Wrapf(ErrUnsupportedAlg, "signature algorithm: %q, key algorithm: %q", signAlg, keyAlg)
}

func (e *Event) GetTag(tagName string) Tag {
	for _, tag := range e.Tags {
		if tag.Key() == tagName {
			return tag
		}
	}

	return nil
}

func (e *Event) GetTags(tagName string) (tags []Tag) {
	for _, tag := range e.Tags {
		if tag.Key() == tagName {
			tags = append(tags, tag)
		}
	}

	return tags
}

func (e *Event) GetMasterPublicKey() (pubkey string) {
	return cmp.Or(e.GetTag(CustomIONTagOnBehalfOf).Value(), e.PubKey)
}

func (e *Event) GetHTag() string {
	return cmp.Or(e.GetTag(CustomIONTagCommunity).Value(), e.ID)
}

func (evt *Event) IsReplaceable() bool {
	return nostr.IsReplaceableKind(evt.Kind)
}

func (evt *Event) IsAddressable() bool {
	return nostr.IsAddressableKind(evt.Kind)
}

func (evt *Event) IsRegular() bool {
	return nostr.IsRegularKind(evt.Kind)
}

func (evt *Event) IsEphemeral() bool {
	return nostr.IsEphemeralKind(evt.Kind)
}

func (e *Event) IsJobRequest() bool {
	return e.Kind >= 5000 && e.Kind < 6000
}

func (e *Event) IsJobResponse() bool {
	return e.Kind >= 6000 && e.Kind < nostr.KindJobFeedback
}

func (e *Event) Address() string {
	switch {
	case e.IsAddressable():
		return strconv.Itoa(e.Kind) + ":" + e.GetMasterPublicKey() + ":" + e.Tags.GetD()

	case e.IsReplaceable():
		return strconv.Itoa(e.Kind) + ":" + e.GetMasterPublicKey() + ":"
	}

	return e.ID
}

func (events Events) String() string {
	var sb strings.Builder
	for i, e := range events {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(e.String())
	}
	return sb.String()
}

func DeduplicateSlice[T any, H comparable](s []T, key func(elem T) H) []T {
	seen := make(map[H]struct{}, len(s))
	j := 0
	for _, v := range s {
		x := key(v)
		if _, ok := seen[x]; ok {
			continue
		}
		seen[x] = struct{}{}
		s[j] = v
		j++
	}

	return s[:j]
}

func SplitBatch[T any](slice []T, batchSize int) (batches [][]T) {
	for batchSize < len(slice) {
		slice, batches = slice[batchSize:], append(batches, slice[0:batchSize:batchSize])
	}
	return append(batches, slice)
}

func PointerOf[T any](v T) *T { return &v }

func ParseEphemeralEmbeddingEvents(events ...*Event) (map[string][]*EphemeralEmbeddingEvent, error) {
	ephemeralEmbeddingEventsByAddr := make(map[string][]*EphemeralEmbeddingEvent, len(events))
	for _, ev := range events {
		if ev.Kind != CustomIONKindEphemeralEmbeddding {
			continue
		}
		ref, content, aErr := ParseEphemeralEmbeddingEventRef(ev)
		if aErr != nil {
			return nil, errors.Wrapf(aErr, "malformed 21750: %v", ev.Content)
		}
		ephemeralEmbeddingEventsByAddr[ref] = append(ephemeralEmbeddingEventsByAddr[ref], &EphemeralEmbeddingEvent{ContentEvent: content, Event: ev})
	}
	return ephemeralEmbeddingEventsByAddr, nil
}

func ParseEphemeralEmbeddingEventRef(ev *Event) (key string, eventContent *Event, err error) {
	var content Event
	err = content.UnmarshalJSON([]byte(ev.Content))
	if err != nil {
		return "", nil, errors.Wrapf(err, "malformed %v event, incorrect content %v", CustomIONKindEphemeralEmbeddding, ev.Content)
	}
	eventContent = &content
	ref := cmp.Or(ev.GetTag("e").Value(), ev.GetTag("a").Value())
	if ref == "" {
		return "", nil, errors.Errorf("malformed ephemeral embedding, missing a / e tag: %+v", ev)
	}
	return ref, eventContent, nil
}
