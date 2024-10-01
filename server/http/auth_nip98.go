// SPDX-License-Identifier: ice License 1.0

package http

import (
	"encoding/base64"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gookit/goutil/errorx"
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

func NewAuth() AuthClient {
	return &authNostr{}
}

func (a *authNostr) VerifyToken(gCtx *gin.Context, token string) (Token, error) {
	bToken, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return nil, errorx.With(err, "failed to unmarshal auth token: malformed base64")
	}
	var event nostr.Event
	if err = event.UnmarshalJSON(bToken); err != nil {
		return nil, errorx.With(err, "failed to unmarshal auth token: malformed event json")
	}
	var ok bool
	if ok, err = event.CheckSignature(); err != nil {
		return nil, errorx.Withf(err, "invalid token signature")
	} else if !ok {
		return nil, errorx.New("invalid token signature")
	}
	if event.Kind != nostrHttpAuthKind {
		return nil, errorx.Newf("invalid token event kind %v", event.Kind)
	}
	now := time.Now()
	if event.CreatedAt.Time().After(now) || (event.CreatedAt.Time().Before(now) && now.Sub(event.CreatedAt.Time()) > tokenExpirationWindow) {
		return nil, errorx.New("expired token")
	}
	if urlTag := event.Tags.GetFirst([]string{"u"}); urlTag != nil && len(*urlTag) > 1 {
		url := (*urlTag)[1]
		fullReqUrl := fmt.Sprintf("https://%v%v", gCtx.Request.Host, gCtx.Request.RequestURI)
		if url != fullReqUrl {
			return nil, errorx.Newf("invalid token: url mismatch token>%v url>%v", url, fullReqUrl)
		}
	} else {
		return nil, errorx.Newf("invalid token: malformed u tag %v", urlTag)
	}
	if methodTag := event.Tags.GetFirst([]string{"method"}); methodTag != nil && len(*methodTag) > 1 {
		method := (*methodTag)[1]
		if method != gCtx.Request.Method {
			return nil, errorx.Newf("invalid token: method mismatch token>%v url>%v", method, gCtx.Request.Method)
		}
	} else {
		return nil, errorx.Newf("invalid token: malformed method tag %v", methodTag)
	}
	expectedHash := ""
	if payloadTag := event.Tags.GetFirst([]string{"payload"}); payloadTag != nil && len(*payloadTag) > 1 {
		expectedHash = (*payloadTag)[1]
	}
	return &nostrToken{ev: event, expectedHash: expectedHash}, nil
}
func (t *nostrToken) PubKey() string {
	return t.ev.PubKey
}
func (t *nostrToken) ExpectedHash() string {
	return t.expectedHash
}
