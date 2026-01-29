// SPDX-License-Identifier: ice License 1.0

package nip11

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/goccy/go-json"
	"github.com/imroc/req/v3"
	"github.com/jellydator/ttlcache/v3"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/singleflight"

	"github.com/ice-blockchain/subzero/appcontext"
)

type (
	Fetcher interface {
		Fetch(ctx context.Context, relayUrl string) (*RelayInformationDocument, error)
	}
	FetcherOption func(*fetcher)

	fetcher struct {
		Cache    *ttlcache.Cache[string, *RelayInformationDocument]
		SfGroup  singleflight.Group
		Insecure bool
	}
)

const (
	cacheDuration = 1 * time.Minute
)

func WithInsecureFetch() FetcherOption {
	return func(f *fetcher) {
		f.Insecure = true
	}
}

func NewFetcher(ctx context.Context, opts ...FetcherOption) Fetcher {
	f := &fetcher{
		Cache: ttlcache.New(ttlcache.WithTTL[string, *RelayInformationDocument](cacheDuration)),
	}

	for _, opt := range opts {
		opt(f)
	}

	go f.Cache.Start()
	appcontext.GetAppContext(ctx).OnShutdown(func() error {
		log.Trace().Msg("NIP11 fetcher: shutting down")
		f.Cache.Stop()
		return nil
	})
	return f
}

func (f *fetcher) Fetch(ctx context.Context, relayUrl string) (*RelayInformationDocument, error) {
	item := f.Cache.Get(relayUrl)
	var nip11Data *RelayInformationDocument
	if item != nil && !item.IsExpired() {
		nip11Data = item.Value()
		return nip11Data, nil
	}
	nip11Data, err := f.fetch(ctx, relayUrl)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to fetch relay's nip11: %v", relayUrl)
	}
	f.Cache.Set(relayUrl, nip11Data, cacheDuration)

	return nip11Data, nil
}

func (f *fetcher) fetch(ctx context.Context, relayUrl string) (*RelayInformationDocument, error) {
	defer f.SfGroup.Forget(relayUrl)

	nip11, err, _ := f.SfGroup.Do(relayUrl, func() (any, error) {
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
		return nil, errors.Errorf("invalid scheme: %v", u.Scheme)
	}

	client := req.C()
	if f.Insecure {
		client = client.EnableInsecureSkipVerify()
	}

	resp, err := client.
		R().
		SetContext(ctx).
		SetRetryCount(3).
		SetRetryInterval(func(resp *req.Response, attempt int) time.Duration {
			return 1 * time.Second
		}).
		SetRetryHook(func(resp *req.Response, err error) {
			if err != nil {
				log.Error().Err(err).Str("relay_url", relayUrl).Msg("failed to call relay, retrying")
			} else {
				log.Error().Str("relay_url", relayUrl).Int("status_code", resp.StatusCode).Msg("failed to call relay with status code, retrying")
			}
		}).
		SetRetryCondition(func(resp *req.Response, err error) bool {
			return err != nil || resp.GetStatusCode() != http.StatusOK
		}).
		SetHeader("Accept", "application/nostr+json").
		Get(u.String())

	if err != nil {
		return nil, errors.Wrapf(err, "failed to call relay %v", relayUrl)
	} else if statusCode := resp.GetStatusCode(); statusCode != http.StatusOK {
		return nil, errors.Errorf("failed to check relay %v with status code:%v", relayUrl, statusCode)
	} else if data, err2 := resp.ToBytes(); err2 != nil {
		return nil, errors.Wrapf(err2, "failed to read body of relay %v response", relayUrl)
	} else {
		var nip11 RelayInformationDocument
		if err = json.Unmarshal(data, &nip11); err != nil {
			return nil, errors.Wrapf(err, "failed to unmarshal data: %v", string(data))
		}
		if nip11.URL == "" {
			nip11.URL = relayUrl
		}
		return &nip11, nil
	}
}
