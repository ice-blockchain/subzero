// SPDX-License-Identifier: ice License 1.0

package model

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"strings"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip13"
)

type (
	Event struct {
		nostr.Event
	}
	EventSignAlg string
)

const (
	// Schnorr with secp256k1.
	SignAlgSchnorr EventSignAlg = "schnorr"
	// EdDSA with ed25519.
	SignAlgEDDSA EventSignAlg = "eddsa"
	// ECDSA with secp256k1.
	SignAlgECDSA EventSignAlg = "ecdsa"
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

func (e *Event) SignWithAlg(privateKey string, alg EventSignAlg) error {
	if alg == "" {
		// Use default schnorr signature.
		return e.Event.Sign(privateKey)
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
	switch alg {
	case SignAlgEDDSA:
		pk := ed25519.PrivateKey(privKey)
		e.PubKey = hex.EncodeToString(pk.Public().(ed25519.PublicKey))
		headerSum = sha256.Sum256(e.Serialize())
		sign = ed25519.Sign(pk, headerSum[:])

	case SignAlgECDSA:
		priv, pub := btcec.PrivKeyFromBytes(privKey)
		e.PubKey = hex.EncodeToString(pub.SerializeCompressed())
		headerSum = sha256.Sum256(e.Serialize())
		sign = ecdsa.Sign(priv, headerSum[:]).Serialize()

	case SignAlgSchnorr:
		return e.Event.Sign(privateKey)

	default:
		return errors.Wrap(ErrUnsupportedSignatureAlg, string(alg))
	}

	e.ID = hex.EncodeToString(headerSum[:])
	e.Sig = string(alg) + ":" + hex.EncodeToString(sign)

	return nil
}

func (e *Event) CheckSignature() (bool, error) {
	sigTypeIdx := strings.IndexRune(e.Sig, ':')
	if sigTypeIdx == -1 {
		// Default schnorr signature.
		return e.Event.CheckSignature()
	}

	pk, err := hex.DecodeString(e.PubKey)
	if err != nil {
		return false, errors.Wrapf(err, "public key is invalid hex")
	}

	sign, err := hex.DecodeString(e.Sig[sigTypeIdx+1:])
	if err != nil {
		return false, errors.Wrap(err, "signature is invalid hex")
	}
	hash := sha256.Sum256(e.Serialize())

	sigName := EventSignAlg(e.Sig[:sigTypeIdx])
	switch sigName {
	case SignAlgEDDSA:
		return ed25519.Verify(pk, hash[:], sign), nil

	case SignAlgSchnorr:
		return e.Event.CheckSignature()

	case SignAlgECDSA:
		pubKey, err := btcec.ParsePubKey(pk)
		if err != nil {
			return false, err
		}
		signature, err := ecdsa.ParseSignature(sign)
		if err != nil {
			return false, err
		}
		return signature.Verify(hash[:], pubKey), nil
	}

	return false, errors.Wrap(ErrUnsupportedSignatureAlg, string(sigName))
}
