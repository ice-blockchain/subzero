// SPDX-License-Identifier: ice License 1.0
package validation

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
)

func validateInternalTopicTC(_ context.Context, _ *eventValidator, event *model.Event, _ *ruleSet) error {
	for _, tag := range event.Tags {
		if tag.Key() == "t" && tag.Value() == "community_token" {
			return nil
		}
	}
	return errors.Errorf("missing required tag %q with value %q", "t", "community_token")
}

func validateTokenizedCommunityFirstBuy(ctx context.Context, v *eventValidator, firstBuy *model.Event, _ *ruleSet) error {
	creator := firstBuy.GetTag("p").Value()
	if creator == "" {
		// Looks like a generic defination event, nothing to validate.
		return nil
	}

	tcEventAddress := firstBuy.GetTag("a").Value()
	if tcEventAddress == "" {
		tcEventAddress = firstBuy.GetTag("e").Value()
	}
	if tcEventAddress == "" {
		// Looks like xcom event, nothing to validate.
		return nil
	}

	var tokenizedEvent, creatorProfileEvent *model.Event
	for ev, err := range v.QueryFunc(ctx, model.Filter{
		Addresses: []string{tcEventAddress},
		Search:    "include:dependencies:kind" + strconv.Itoa(model.KindAny) + ">kind0",
		Limit:     1,
	}) {
		if err != nil {
			return err
		}
		if ev.Kind == nostr.KindProfileMetadata {
			creatorProfileEvent = ev
		} else {
			tokenizedEvent = ev
		}
	}

	if tokenizedEvent == nil && creatorProfileEvent != nil && creatorProfileEvent.Address() == tcEventAddress {
		// Tokenized profile event.
		tokenizedEvent = creatorProfileEvent
	}

	if tokenizedEvent == nil {
		return errors.Wrap(ErrNotFound, "tokenized event not found")
	}

	if tokenizedEvent.GetMasterPublicKey() == firstBuy.GetMasterPublicKey() {
		// Nothing to validate.
		return nil
	}

	if creatorProfileEvent == nil {
		return errors.Wrap(ErrNotFound, "profile not found")
	}

	var meta model.ProfileMetadataContent
	if err := json.Unmarshal([]byte(creatorProfileEvent.Content), &meta); err != nil {
		return errors.Wrap(err, "unmarshal profile metadata content failed")
	}

	var bscWallet string
	for network, walletAddr := range meta.Wallets {
		if strings.EqualFold(network, "bsc") || strings.EqualFold(network, "bsctestnet") {
			bscWallet = walletAddr
			break
		}
	}

	if bscWallet == "" {
		return ErrWalletRequired
	}

	return nil
}
