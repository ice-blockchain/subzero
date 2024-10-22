// SPDX-License-Identifier: ice License 1.0

package model

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/mailru/easyjson"
	"github.com/mailru/easyjson/jwriter"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip13"
	"github.com/tidwall/gjson"
)

type (
	Event struct {
		nostr.Event
	}
	EventEnvelope struct {
		SubscriptionID *string
		Events         []*Event
	}

	EventSignAlg string
	EventKeyAlg  string
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
	if err := nip13.Check(e.GetID(), minLeadingZeroBits); err != nil {
		log.Printf("difficulty: %v < %v, id:%v", nip13.Difficulty(e.GetID()), minLeadingZeroBits, e.GetID())

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

func (e *Event) GetMasterPublicKey() (pubkey string) {
	pubkey = e.PubKey
	if bTag := e.GetTag(CustomIONTagOnBehalfOf); bTag != nil {
		pubkey = bTag.Value()
	}
	return pubkey
}

func (EventEnvelope) Label() string { return "EVENT" }

func (v *EventEnvelope) UnmarshalJSON(data []byte) error {
	r := gjson.ParseBytes(data)
	arr := r.Array()
	switch len(arr) {
	case 0, 1:
		return errors.Wrapf(ErrUnknownMessage, "failed to decode EVENT envelope: unknown array len: %v", len(arr))

	// No subscription ID: ["EVENT", event].
	case 2:
		var ev Event

		err := easyjson.Unmarshal([]byte(arr[1].Raw), &ev)
		if err == nil {
			v.Events = []*Event{&ev}

			return nil
		}

		return errors.Wrap(err, "failed to decode event")

	// With multiple events: ["EVENT", [optional subscriptionID], <event1>, [event2], ...].
	default:
		jsonEvents := arr[1:] // Skip the first element, which is the label.
		if jsonEvents[0].Type == gjson.String {
			v.SubscriptionID = &jsonEvents[0].Str
			jsonEvents = jsonEvents[1:]
		} else if jsonEvents[0].Type == gjson.Null {
			v.SubscriptionID = nil
			jsonEvents = jsonEvents[1:]
		}
		v.Events = make([]*Event, 0, len(jsonEvents))
		for i := range jsonEvents {
			var ev Event
			if err := easyjson.Unmarshal([]byte(jsonEvents[i].Raw), &ev); err != nil {
				return errors.Wrapf(err, "failed to decode event %d", i)
			}
			v.Events = append(v.Events, &ev)
		}
	}

	return nil
}

func (v EventEnvelope) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	w.RawString(`["EVENT",`)
	if v.SubscriptionID != nil {
		w.RawString(`"` + *v.SubscriptionID + `"`)
		if len(v.Events) > 0 {
			w.RawByte(',')
		}
	}

	for i := range v.Events {
		v.Events[i].MarshalEasyJSON(&w)
		if i < len(v.Events)-1 {
			w.RawByte(',')
		}
	}
	w.RawString(`]`)

	return w.BuildBytes()
}

func (v EventEnvelope) String() string {
	j, _ := v.MarshalJSON()

	return string(j)
}
