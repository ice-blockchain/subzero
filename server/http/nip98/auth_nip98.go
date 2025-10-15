// SPDX-License-Identifier: ice License 1.0

package nip98

import (
	"context"
	"encoding/base64"
	"net/url"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/auth"
)

type (
	Token interface {
		PubKey() string
		MasterPubKey() string
		ExpectedHash() string
		ValidateAttestation(ctx context.Context, kind int, now time.Time) error
	}
	AuthClient interface {
		VerifyToken(actualReqUrl *url.URL, reqMethod, token string, now time.Time) (Token, error)
	}

	nostrToken struct {
		ev           model.Event
		expectedHash string
	}
	authNostr struct {
	}
)

const (
	tokenExpirationWindow = 15 * time.Minute
	NostrHttpAuthKind     = 27235
)

var (
	ErrTokenExpired = errors.New("expired token")
	ErrTokenInvalid = errors.New("invalid token")
)

func NewAuth() AuthClient {
	return &authNostr{}
}

func DetectAuthHeader(val string) string {
	knownTypes := []string{"Bearer", "Nostr", "IONConnect"}

	for _, t := range knownTypes {
		if strings.HasPrefix(val, t) {
			return strings.TrimSpace(strings.TrimPrefix(val, t))
		}
	}
	return ""
}

func (a *authNostr) VerifyToken(fullReqUrl *url.URL, reqMethod, token string, now time.Time) (Token, error) {
	if token == "" {
		return nil, errors.Wrapf(ErrTokenInvalid, "empty token")
	}
	bToken, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal auth token: malformed base64: %q", token)
	}

	var event model.Event
	if err := event.UnmarshalJSON(bToken); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal auth token: malformed event json")
	}

	if ok, err := event.CheckSignature(); err != nil {
		return nil, errors.Wrapf(err, "cannot check token signature")
	} else if !ok {
		return nil, errors.Wrapf(ErrTokenInvalid, "invalid token signature")
	}

	if event.Kind != NostrHttpAuthKind {
		return nil, errors.Wrapf(ErrTokenInvalid, "invalid token event kind: %d, expected: %d", event.Kind, NostrHttpAuthKind)
	}
	if event.CreatedAt.Time().After(now.Add(tokenExpirationWindow)) || event.CreatedAt.Time().Before(now.Add(-tokenExpirationWindow)) {
		return nil, ErrTokenExpired
	}
	if urlTag := event.Tags.GetFirst([]string{"u"}); urlTag != nil && len(*urlTag) > 1 {
		var urlValue *url.URL
		urlValue, err = url.Parse(urlTag.Value())
		if err != nil {
			return nil, errors.Wrapf(ErrTokenInvalid, "failed to parse url tag with %q: %v", urlTag.Value(), err)
		}
		if urlValue.String() != fullReqUrl.String() {
			return nil, errors.Wrapf(ErrTokenInvalid, "url mismatch token>%v url>%v", urlValue, fullReqUrl)
		}
	} else {
		return nil, errors.Wrapf(ErrTokenInvalid, "malformed u tag %v", urlTag)
	}

	if methodTag := event.Tags.GetFirst([]string{"method"}); methodTag != nil && len(*methodTag) > 1 {
		method := methodTag.Value()
		if method != reqMethod {
			return nil, errors.Wrapf(ErrTokenInvalid, "method mismatch token>%v url>%v", method, reqMethod)
		}
	} else {
		return nil, errors.Wrapf(ErrTokenInvalid, "malformed method tag %v", methodTag)
	}
	expectedHash := ""
	if payloadTag := event.Tags.GetFirst([]string{"payload"}); payloadTag != nil && len(*payloadTag) > 1 {
		expectedHash = payloadTag.Value()
	}
	return &nostrToken{ev: event, expectedHash: expectedHash}, nil
}
func (t *nostrToken) PubKey() string {
	return t.ev.PubKey
}
func (t *nostrToken) MasterPubKey() string {
	return t.ev.GetMasterPublicKey()
}
func (t *nostrToken) ExpectedHash() string {
	return t.expectedHash
}

func (t *nostrToken) ValidateAttestation(ctx context.Context, kind int, now time.Time) error {
	if t.ev.PubKey == t.MasterPubKey() {
		return nil
	}
	relayUrl := ""
	if urlTag := t.ev.Tags.GetFirst([]string{"u"}); urlTag != nil && len(*urlTag) > 1 {
		urlValue, err := url.Parse(urlTag.Value())
		if err != nil {
			return errors.Wrapf(ErrTokenInvalid, "failed to parse url tag with %q: %v", urlTag.Value(), err)
		}
		switch urlValue.Scheme {
		case "https":
			urlValue.Scheme = "wss"
		case "http":
			urlValue.Scheme = "ws"
		default:
			urlValue.Scheme = "wss"
		}
		relayUrl = urlValue.Scheme + "://" + urlValue.Host
	}
	kinds, err := auth.ValidateUserAccess(ctx, relayUrl, &t.ev)
	if err != nil {
		return errors.Wrapf(err, "failed to validate on-behalf access")
	}
	if _, ok := kinds[kind]; len(kinds) != 0 && !ok {
		return errors.Wrapf(model.ErrOnBehalfAccessDenied, "kind %d", kind)
	}
	return nil
}

func GenerateAuthHeader(sk, method, fileHash string, urlValue *url.URL, masterPubkey ...string) (string, error) {
	pk, err := model.GetPublicKey(sk)
	if err != nil {
		return "", errors.Wrapf(err, "malformed private-key for generating auth")
	}
	event := model.Event{
		Event: nostr.Event{
			Kind:      NostrHttpAuthKind,
			PubKey:    pk,
			CreatedAt: nostr.Now(),
			Tags: model.Tags{
				model.Tag{"u", (&url.URL{
					Scheme:   "https",
					Host:     urlValue.Host,
					Path:     urlValue.Path,
					RawQuery: urlValue.RawQuery,
					Fragment: urlValue.Fragment,
				}).String()},
				model.Tag{"method", method},
				model.Tag{"payload", fileHash},
			},
		},
	}
	if len(masterPubkey) > 0 && masterPubkey[0] != "" {
		event.Tags = append(event.Tags, model.Tag{"b", masterPubkey[0]})
	}
	if err = event.SignWithAlg(sk, model.SignAlgEDDSA, model.KeyAlgCurve25519); err != nil {
		return "", errors.Wrap(err, "failed to sign auth event")
	}

	return `Nostr ` + base64.StdEncoding.EncodeToString([]byte(event.String())), nil
}
