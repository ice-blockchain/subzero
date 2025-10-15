// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip42"

	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/auth"
)

func (h *handler) handleAuth(ctx context.Context, respWriter Writer, e *model.Event) *nostr.OKEnvelope {
	var resp = nostr.OKEnvelope{EventID: e.Event.ID}

	state, ok := h.ConnAuth.Load(respWriter)
	if !ok {
		resp.Reason = "received unexpected auth message: no challenge was sent"

		return &resp
	} else if state.Authenticated && state.PublicKey != e.PubKey {
		resp.Reason = "received unexpected auth message: already authenticated with a different public key"

		return &resp
	}

	_, err := nip42.ValidateAuthEvent(
		&e.Event,
		state.Challenge,
		h.RelayURL,
		nip42.WithCustomVerificator(func(nostrEvent *nostr.Event) (bool, error) {
			return (&model.Event{Event: *nostrEvent}).CheckSignature()
		}))
	if err != nil {
		resp.Reason = "failed to validate auth event: " + err.Error()

		return &resp
	}

	var userdata connAuthData
	if e.PubKey != e.GetMasterPublicKey() {
		var err error
		if userdata.Kinds, err = auth.ValidateUserAccess(ctx, h.RelayURL, e); err != nil {
			if errors.IsAny(err, auth.ErrAttestationRecordNotFound, auth.ErrRelayNotAuthoritative) {
				resp.Reason = auth.ErrRelayNotAuthoritative.Error()
			} else {
				resp.Reason = "failed to validate on-behalf access: " + err.Error()
			}

			return &resp
		}

		// TODO: use `authoritative` flag from validateUserAccess().
		_, hErr := auth.ValidateUserAccessAuthoritative(ctx, h.RelayURL, e)
		userdata.Authoritative = hErr == nil
	}

	userdata.Challenge = state.Challenge
	userdata.MasterPublicKey = e.GetMasterPublicKey()
	userdata.PublicKey = e.PubKey
	userdata.UserAgent = e.GetTag("user-agent").Value()
	userdata.Authenticated = true

	h.ConnAuth.Store(respWriter, userdata)

	resp.OK = true

	return &resp
}
