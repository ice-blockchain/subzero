// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"
	"strconv"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
)

func (ev *eventValidator) validateKindRepostEvent(ctx context.Context, rules *ruleSet, batch model.Events, e *model.Event) error {
	var repostedEvent model.Event

	if err := repostedEvent.UnmarshalJSON([]byte(e.Content)); err != nil {
		return errors.Wrapf(ErrWrongEventParams, "nip-18: wrong json fields: %v", err)
	}
	repostRules := &ruleSet{
		SkipRootContentNFTCollectionsValidation: true,
	}
	if rules != nil {
		repostRules.SkipKindProfileProofEventsVerify = rules.SkipKindProfileProofEventsVerify
		repostRules.SkipKindAttestationProofDevicesVerify = rules.SkipKindAttestationProofDevicesVerify
		repostRules.BroadcastMode = rules.BroadcastMode
	}
	if err := ev.validate(ctx, repostRules, batch, &repostedEvent); err != nil && !errors.IsAny(err, ErrPollTTLExpired) {
		return errors.Wrapf(ErrWrongEventParams, "nip-18: invalid reposted event: %v", err)
	}

	if e.Kind == nostr.KindRepost {
		if repostedEvent.Kind != nostr.KindTextNote {
			return errors.Wrapf(ErrWrongEventParams, "nip-18: wrong kind of reposted event: found %d, expected %d", repostedEvent.Kind, nostr.KindTextNote)
		}
	} else {
		if kTag := e.GetTag("k"); kTag.Value() != strconv.Itoa(repostedEvent.Kind) {
			return errors.Wrapf(ErrWrongEventParams, "nip-18: wrong kind of generic reposted event: found %q, expected %d", kTag.Value(), repostedEvent.Kind)
		}
	}

	if repostedEvent.IsAddressable() || repostedEvent.IsReplaceable() {
		if eTag := e.GetTag("a"); eTag.Value() != repostedEvent.Address() {
			return errors.Wrapf(ErrWrongEventParams, "nip-18: repost must include a tag with address of the note: found %q, expected %q", eTag.Value(), repostedEvent.Address())
		}
	} else {
		if eTag := e.GetTag("e"); eTag.Value() != repostedEvent.ID {
			return errors.Wrapf(ErrWrongEventParams, "nip-18: repost must include e tag with id of the note: found %q, expected %q", eTag.Value(), repostedEvent.ID)
		}
	}

	if pTag := e.GetTag("p"); pTag.Value() != repostedEvent.GetMasterPublicKey() {
		return errors.Wrapf(ErrWrongEventParams,
			"nip-18: repost must include p tag with pubkey of the event being reposted: found %q, expected %q",
			pTag.Value(), repostedEvent.GetMasterPublicKey())
	}
	if err := ev.validatePostCommunityEvent(ctx, e); err != nil {
		return err
	}
	if err := ev.validateWhoCanReplySettings(ctx, rules, batch, e); err != nil {
		return err
	}

	return nil
}
