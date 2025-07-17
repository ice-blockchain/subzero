// SPDX-License-Identifier: ice License 1.0

package nip11

import (
	"context"
	"github.com/cockroachdb/errors"
	"github.com/goccy/go-json"
	"github.com/imroc/req/v3"
	"github.com/jellydator/ttlcache/v3"
	"golang.org/x/sync/singleflight"
	"log"
	"net/http"
	"net/url"
	"time"
)

type (
	Fetcher interface {
		Fetch(ctx context.Context, relayUrl string) (*RelayInformationDocument, error)
	}
	fetcher struct {
		cache   *ttlcache.Cache[string, *RelayInformationDocument]
		sfGroup singleflight.Group
	}
)

const cacheDuration = 1 * time.Minute

func NewFetcher(ctx context.Context) Fetcher {
	f := &fetcher{
		cache: ttlcache.New[string, *RelayInformationDocument](ttlcache.WithTTL[string, *RelayInformationDocument](cacheDuration)),
	}
	go f.cache.Start()
	go func() {
		<-ctx.Done()
		log.Printf("NIP11 fetcher: shutting down")
		f.cache.Stop()
	}()
	return f
}

func (f *fetcher) Fetch(ctx context.Context, relayUrl string) (*RelayInformationDocument, error) {
	item := f.cache.Get(relayUrl)
	var nip11Data *RelayInformationDocument
	if item != nil && !item.IsExpired() {
		nip11Data = item.Value()
		return nip11Data, nil
	}
	nip11Data, err := f.fetch(ctx, relayUrl)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to fetch relay's nip11: %v", relayUrl)
	}
	f.cache.Set(relayUrl, nip11Data, cacheDuration)

	return nip11Data, nil
}

func (f *fetcher) fetch(ctx context.Context, relayUrl string) (*RelayInformationDocument, error) {
	defer f.sfGroup.Forget(relayUrl)
	nip11, err, _ := f.sfGroup.Do(relayUrl, func() (any, error) {
		return f.requestNIP11(ctx, relayUrl)
	})
	return nip11.(*RelayInformationDocument), err
}

func (f *fetcher) requestNIP11(ctx context.Context, relayUrl string) (*RelayInformationDocument, error) {
	u, err := url.Parse(relayUrl)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid url: %v", relayUrl)
	}
	switch u.Scheme {
	case "ws":
		u.Scheme = "http"
	case "wss":
		u.Scheme = "https"
	default:
		return nil, errors.Errorf("invalid scheme :%v", u.Scheme)
	}
	if resp, err := req.
		SetContext(ctx).
		SetRetryCount(3).
		SetRetryInterval(func(resp *req.Response, attempt int) time.Duration {
			return 1 * time.Second
		}).
		SetRetryHook(func(resp *req.Response, err error) {
			if err != nil {
				log.Printf("ERROR: %v", errors.Wrapf(err, "failed to call relay %v, retrying...", relayUrl))
			} else {
				log.Printf("ERROR: %v", errors.Errorf("failed to call relay %v with status code:%v, retrying...", relayUrl, resp.GetStatusCode()))
			}
		}).
		SetRetryCondition(func(resp *req.Response, err error) bool {
			return err != nil || resp.GetStatusCode() != http.StatusOK
		}).
		SetHeader("Accept", "application/nostr+json").
		Get(u.String()); err != nil {
		return nil, errors.Wrapf(err, "failed to call relay %v", relayUrl)

	} else if statusCode := resp.GetStatusCode(); statusCode != http.StatusOK {
		return nil, errors.Errorf("failed to check relay %v with status code:%v", relayUrl, statusCode)
	} else if data, err2 := resp.ToBytes(); err2 != nil {
		return nil, errors.Wrapf(err2, "failed to read body of relay %v response", relayUrl)
	} else {
		var nip11 RelayInformationDocument
		if err = json.UnmarshalContext(ctx, data, &nip11); err != nil {
			return nil, errors.Wrapf(err, "failed to unmarshal data: %v", string(data))
		}
		return &nip11, nil
	}
}
