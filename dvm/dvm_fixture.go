// SPDX-License-Identifier: ice License 1.0

//go:build test

package dvm

import "github.com/nbd-wtf/go-nostr"

func PublicKey() (string, error) {
	return nostr.GetPublicKey(globalDVM.privateKey)
}
