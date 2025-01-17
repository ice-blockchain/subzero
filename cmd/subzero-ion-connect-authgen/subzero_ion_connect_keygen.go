// SPDX-License-Identifier: ice License 1.0

package main

import (
	"flag"
	"fmt"

	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
)

func main() {
	relayURL := flag.String("relay", "", "URL of the relay")
	challenge := flag.String("challenge", "", "Challenge string")
	key := flag.String("key", model.GeneratePrivateKey(), "Private key")
	master := flag.String("master-key", "", "Master public key")
	flag.Parse()

	if *relayURL == "" || *challenge == "" {
		fmt.Println("Usage: subzero_ion_connect_keygen -relay <relay URL> -challenge <challenge string>")
		return
	}

	public, err := model.GetPublicKey(*key)
	if err != nil {
		fmt.Println("Error getting public key:", err)
		return
	}

	fmt.Println("Private key:", *key)
	fmt.Println("Public key: ", public)

	tags := model.Tags{
		{"relay", *relayURL},
		{"challenge", *challenge},
	}
	if *master != "" {
		tags = append(tags, model.Tag{model.CustomIONTagOnBehalfOf, *master})
	}

	authEvent := model.Event{
		Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindClientAuthentication,
			Tags:      tags,
		},
	}
	err = authEvent.SignWithAlg(*key, model.SignAlgEDDSA, model.KeyAlgCurve25519)
	if err != nil {
		fmt.Println("Error signing event:", err)
		return
	}

	msg := nostr.AuthEnvelope{Event: authEvent.Event}
	fmt.Println(msg.String())
}
