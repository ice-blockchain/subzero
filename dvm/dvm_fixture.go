// SPDX-License-Identifier: ice License 1.0

//go:build test

package dvm

import "github.com/ice-blockchain/subzero/model"

func PublicKey() (string, error) {
	return model.GetPublicKey(globalDVM.privateKey)
}
