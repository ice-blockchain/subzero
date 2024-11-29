// SPDX-License-Identifier: ice License 1.0

package main

import (
	"fmt"

	"github.com/ice-blockchain/subzero/model"
)

func main() {
	private, public := model.GenerateKeyPair()
	fmt.Println("Private key:", private)
	fmt.Println("Public key: ", public)
}
