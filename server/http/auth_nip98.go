// SPDX-License-Identifier: ice License 1.0

package http

import (
	"encoding/base64"
	"net/url"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/nbd-wtf/go-nostr"
)

type (
	Token interface {
		PubKey() string
		ExpectedHash() string
	}
	AuthClient interface {
		VerifyToken(gCtx *gin.Context, token string) (Token, error)
	}

	nostrToken struct {
		ev           nostr.Event
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

func (a *authNostr) VerifyToken(gCtx *gin.Context, token string) (Token, error) {
	bToken, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal auth token: malformed base64")
	}
	var event nostr.Event
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
	now := time.Now()
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
func (t *nostrToken) ExpectedHash() string {
	return t.expectedHash
}
