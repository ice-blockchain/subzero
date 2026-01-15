// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"context"
	"crypto/sha3"
	"encoding/base32"
	"encoding/binary"
	"math"
	"math/rand/v2"
	"net/url"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip42"

	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/auth"
)

func authEventVerifier(nostrEvent *nostr.Event) (bool, error) {
	return (&model.Event{Event: *nostrEvent}).CheckSignature()
}

func authEventRelayMatcher(expected, found *url.URL) bool {
	return strings.EqualFold(expected.Scheme, found.Scheme) &&
		strings.EqualFold(expected.Path, found.Path) &&
		strings.EqualFold(expected.Hostname(), found.Hostname()) // Ignore port differences.
}

func authConnGetChallenge(respWriter Writer) string {
	challengeValue, ok := respWriter.Metadata().Get(connMetadataChallengeKey)
	if !ok {
		return ""
	}
	return challengeValue.(string)
}

func authConnGetState(respWriter Writer) model.UserDataContext {
	stateValue, ok := respWriter.Metadata().Get(connMetadataAuthKey)
	if !ok {
		return model.UserDataContext{}
	}

	state, ok := stateValue.(model.UserDataContext)
	if !ok {
		return model.UserDataContext{}
	}
	return state
}

func generateChallenge(hints ...string) string {
	h := sha3.New512()

	binary.Write(h, binary.LittleEndian, uint64(time.Now().UnixNano()))
	binary.Write(h, binary.LittleEndian, rand.Uint64N(math.MaxUint64))
	for i := range hints {
		h.Write([]byte(hints[i]))
	}

	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(h.Sum(nil))
}

func authConnGenerateChallenge(respWriter Writer) string {
	actual, _ := respWriter.
		Metadata().
		GetOrSet(
			connMetadataChallengeKey,
			generateChallenge(respWriter.RemoteAddr().String()),
		)

	return actual.(string)
}

func authConnSet(respWriter Writer, userdata model.UserDataContext) {
	respWriter.Metadata().Set(connMetadataAuthKey, userdata)
}

func (h *handler) handleAuth(ctx context.Context, respWriter Writer, e *model.Event) *nostr.OKEnvelope {
	var resp = nostr.OKEnvelope{EventID: e.Event.ID}

	challenge := authConnGetChallenge(respWriter)
	if challenge == "" {
		resp.Reason = "received unexpected auth message: no challenge was sent"
		return &resp
	}

	stateValue, ok := respWriter.Metadata().Get(connMetadataAuthKey)
	if ok {
		state := stateValue.(model.UserDataContext)
		if state.Authenticated && state.PublicKey != e.PubKey {
			resp.Reason = "received unexpected auth message: already authenticated with a different public key"
			return &resp
		}
	}

	_, err := nip42.ValidateAuthEvent(
		&e.Event,
		challenge,
		h.RelayURL,
		nip42.WithCustomVerificator(authEventVerifier),
		nip42.WithCustomRelayMatcher(authEventRelayMatcher),
	)
	if err != nil {
		resp.Reason = "failed to validate auth event: " + err.Error()

		return &resp
	}

	var masterPubKey = e.GetMasterPublicKey()
	var userdata model.UserDataContext
	if e.PubKey != masterPubKey {
		var err error
		if userdata.Kinds, userdata.Authoritative, err = auth.ValidateUserAccess(ctx, h.RelayURL, e); err != nil {
			if errors.IsAny(err, auth.ErrAttestationRecordNotFound, auth.ErrRelayNotAuthoritative) {
				resp.Reason = auth.ErrRelayNotAuthoritative.Error()
			} else {
				resp.Reason = "failed to validate on-behalf access: " + err.Error()
			}
			return &resp
		}
	}

	userdata.MasterPublicKey = masterPubKey
	userdata.PublicKey = e.PubKey
	userdata.UserAgent = e.GetTag("user-agent").Value()
	userdata.Authenticated = true

	authConnSet(respWriter, userdata)

	resp.OK = true

	return &resp
}
