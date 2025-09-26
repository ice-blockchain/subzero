// SPDX-License-Identifier: ice License 1.0

package model

import (
	"crypto/ed25519"
	"encoding/hex"

	"github.com/cockroachdb/errors"
	"github.com/rs/zerolog/log"
)

func GeneratePrivateKey() string {
	priv, _ := GenerateKeyPair()

	return priv
}

func GenerateKeyPair() (private string, public string) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		log.Panic().Str("context", "MODEL").Err(err).Msg("failed to generate private key")
	}

	return hex.EncodeToString(priv), hex.EncodeToString(pub)
}

func GetPublicKey(pk string) (string, error) {
	privateKey, err := hex.DecodeString(pk)
	if err != nil {
		return "", err
	}

	if len(privateKey) != ed25519.PrivateKeySize {
		return "", errors.Errorf("invalid private key size: %d, expected: %d", len(privateKey), ed25519.PrivateKeySize)
	}

	publicKey := make([]byte, ed25519.PublicKeySize)
	copy(publicKey, privateKey[32:])

	return hex.EncodeToString(publicKey), nil
}
