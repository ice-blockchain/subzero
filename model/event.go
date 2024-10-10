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
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip13"
)

type (
	Event struct {
		nostr.Event
	}
	EventSignAlg string
	EventKeyAlg  string
)

const (
	// Schnorr (default) signature.
	SignAlgSchnorr EventSignAlg = "schnorr"
	// ECDSA.
	SignAlgEDDSA EventSignAlg = "eddsa"

	// Secp256k1 (default) key.
	KeyAlgSecp256k1 EventKeyAlg = "secp256k1"
	// Curve25519.
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
		return err
	}

	var sign []byte
	var headerSum [32]byte
	switch {
	case (signAlg == "" && keyAlg == "") || (signAlg == SignAlgSchnorr && keyAlg == KeyAlgSecp256k1):
		return e.Event.Sign(privateKey)

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

func (e *Event) CheckSignature() (bool, error) {
	extensionEnd := strings.IndexRune(e.Sig, ':')
	if extensionEnd == -1 {
		// Default schnorr signature.
		return e.Event.CheckSignature()
	}
	keyStart := strings.IndexRune(e.Sig[:extensionEnd], '/')
	if keyStart == -1 {
		return false, errors.Wrap(ErrUnsupportedAlg, "key algorithm is not set")
	}

	signAlg := EventSignAlg(e.Sig[:keyStart])
	keyAlg := EventKeyAlg(e.Sig[keyStart+1 : extensionEnd])
	if signAlg == "" || keyAlg == "" {
		return false, errors.Wrap(ErrUnsupportedAlg, "signature and key algorithms must be set together")
	}

	pk, err := hex.DecodeString(e.PubKey)
	if err != nil {
		return false, errors.Wrapf(err, "public key is invalid hex")
	}

	sign, err := hex.DecodeString(e.Sig[extensionEnd+1:])
	if err != nil {
		return false, errors.Wrap(err, "signature is invalid hex")
	}
	hash := sha256.Sum256(e.Serialize())
	switch {
	case signAlg == SignAlgEDDSA && keyAlg == KeyAlgCurve25519:
		return ed25519.Verify(pk, hash[:], sign), nil

	case (signAlg == "" && keyAlg == "") || (signAlg == SignAlgSchnorr && keyAlg == KeyAlgSecp256k1):
		return e.Event.CheckSignature()
	}

	return false, errors.Wrapf(ErrUnsupportedAlg, "signature algorithm: %q, key algorithm: %q", signAlg, keyAlg)
}
