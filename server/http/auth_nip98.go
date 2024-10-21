// SPDX-License-Identifier: ice License 1.0

package http

import (
	"context"
	"encoding/base64"
	"net/url"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

type (
	Token interface {
		PubKey() string
		MasterPubKey() string
		ExpectedHash() string
		ValidateAttestation(ctx context.Context, kind int, now time.Time) error
	}
	AuthClient interface {
		VerifyToken(gCtx *gin.Context, token string, now time.Time) (Token, error)
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
	nostrHttpAuthKind     = 27235
)

var (
	ErrTokenExpired = errors.New("expired token")
	ErrTokenInvalid = errors.New("invalid token")
)

func NewAuth() AuthClient {
	return &authNostr{}
}

func getAuthHeader(gCtx *gin.Context) string {
	return strings.TrimPrefix(gCtx.GetHeader("Authorization"), "Nostr ")
}

func (a *authNostr) VerifyToken(gCtx *gin.Context, token string, now time.Time) (Token, error) {
	bToken, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal auth token: malformed base64")
	}
	var event model.Event
	if err = event.UnmarshalJSON(bToken); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal auth token: malformed event json")
	}
	var ok bool
	if ok, err = event.CheckSignature(); err != nil {
		return nil, errors.Wrapf(err, "invalid token signature")
	} else if !ok {
		return nil, errors.Wrapf(ErrTokenInvalid, "invalid token signature")
	}
	if event.Kind != nostrHttpAuthKind {
		return nil, errors.Wrapf(ErrTokenInvalid, "invalid token event kind %v", event.Kind)
	}
	if event.CreatedAt.Time().After(now) || (event.CreatedAt.Time().Before(now) && now.Sub(event.CreatedAt.Time()) > tokenExpirationWindow) {
		return nil, ErrTokenExpired
	}
	if urlTag := event.Tags.GetFirst([]string{"u"}); urlTag != nil && len(*urlTag) > 1 {
		var urlValue *url.URL
		urlValue, err = url.Parse(urlTag.Value())
		if err != nil {
			return nil, errors.Wrapf(ErrTokenInvalid, "failed to parse url tag with %v", urlTag.Value())
		}
		fullReqUrl := (&url.URL{
			Scheme:   "https",
			Host:     gCtx.Request.Host,
			Path:     gCtx.Request.URL.Path,
			RawQuery: gCtx.Request.URL.RawQuery,
			Fragment: gCtx.Request.URL.Fragment,
		})
		if urlValue.String() != fullReqUrl.String() {
			return nil, errors.Wrapf(ErrTokenInvalid, "url mismatch token>%v url>%v", urlValue, fullReqUrl)
		}
	} else {
		return nil, errors.Wrapf(ErrTokenInvalid, "malformed u tag %v", urlTag)
	}
	if methodTag := event.Tags.GetFirst([]string{"method"}); methodTag != nil && len(*methodTag) > 1 {
		method := methodTag.Value()
		if method != gCtx.Request.Method {
			return nil, errors.Wrapf(ErrTokenInvalid, "method mismatch token>%v url>%v", method, gCtx.Request.Method)
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
	attestationEventIt := query.GetStoredEvents(ctx, &model.Subscription{model.Filters{model.Filter{
		Kinds: []int{model.CustomIONKindAttestation},
		Tags: model.TagMap{
			"p": []string{t.PubKey()},
		},
	},
	}})
	var allowed bool
	for attestation, err := range attestationEventIt {
		if err != nil {
			return errors.Wrapf(err, "failed to get attestation event")
		}
		allowed, err = model.OnBehalfIsAccessAllowed(attestation.Tags, t.ev.PubKey, kind, now.Unix())
		if err != nil {
			return errors.Wrapf(err, "failed to parse attestation event")
		}
		break
	}
	if !allowed {
		return model.ErrOnBehalfAccessDenied
	}
	return nil
}
