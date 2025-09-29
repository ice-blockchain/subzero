// SPDX-License-Identifier: ice License 1.0

package ionidentitypubkeys

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/goccy/go-json"
	"github.com/imroc/req/v3"
	"github.com/rs/zerolog/log"
)

type (
	IONIdentityKeysProvider interface {
		PublicKeys() []string
	}
	ionIdentityPublicKeysFetcher struct {
		Client *req.Client
		Stored atomic.Pointer[publicKeys]
	}
	publicKeys struct {
		Version string
		Keys    []string
	}
)

func MustNewIONIdentityPublicKeys(ctx context.Context, baseUrl string) IONIdentityKeysProvider {
	if baseUrl == "" {
		log.Panic().
			Str("context", "VALIDATION").
			Err(errors.New("ion identity public keys base url not set")).
			Msg("ion identity public keys base url not set")
	}

	f := &ionIdentityPublicKeysFetcher{
		Client: req.C().
			SetBaseURL(baseUrl).
			SetTimeout(30 * time.Second).
			SetJsonMarshal(json.Marshal).
			SetJsonUnmarshal(json.Unmarshal),
	}

	err := f.syncPubKeys(ctx)
	if err != nil {
		log.Panic().Str("context", "VALIDATION").Err(err).Msg("failed to sync ion identity public keys during startup")
	}

	go f.backgroundSync(ctx)

	return f
}

func (f *ionIdentityPublicKeysFetcher) PublicKeys() []string {
	data := f.Stored.Load()

	if data != nil && len(data.Keys) > 0 {
		return data.Keys
	}
	return []string{}
}

func (f *ionIdentityPublicKeysFetcher) backgroundSync(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := f.syncPubKeys(ctx); err != nil {
				log.Error().Str("context", "VALIDATION").Err(err).Msg("failed to sync ion identity public keys")
			}
		}
	}
}

func (f *ionIdentityPublicKeysFetcher) GetCurrentVersion() string {
	data := f.Stored.Load()
	if data != nil {
		return data.Version
	}
	return "0"
}

func (f *ionIdentityPublicKeysFetcher) syncPubKeys(ctx context.Context) error {
	keys, err := f.fetchPubKeys(ctx, f.GetCurrentVersion())
	if err != nil {
		return errors.Wrapf(err, "failed to fetch public keys from ion identity")
	} else if keys == nil {
		// Notihing has changed.
		return nil
	}

	f.Stored.Store(keys)
	log.Info().Str("context", "VALIDATION").
		Int("keys_count", len(keys.Keys)).
		Str("version", keys.Version).
		Msg("fetched ion identity public keys")

	return nil
}

// fetchPubKeys fetches the public keys from the ion identity service.
// It uses the version parameter to avoid fetching the same keys again.
// If the version is the same as the current version, it returns nil.
func (f *ionIdentityPublicKeysFetcher) fetchPubKeys(ctx context.Context, version string) (*publicKeys, error) {
	resp, err := f.Client.R().
		SetContext(ctx).
		SetRetryCount(5).
		SetRetryInterval(func(resp *req.Response, attempt int) time.Duration {
			switch {
			case attempt <= 1:
				return 100 * time.Millisecond
			case attempt == 2:
				return 1 * time.Second
			default:
				return 10 * time.Second
			}
		}).
		SetRetryHook(func(resp *req.Response, err error) {
			if err != nil {
				log.Warn().
					Str("context", "VALIDATION").
					Err(err).
					Msg("failed to fetch ion identity public keys, retrying")
			} else {
				warn := log.Warn().Int("status_code", resp.GetStatusCode()).Str("context", "VALIDATION")
				if v, respErr := resp.ToString(); respErr == nil {
					warn.Str("response", v)
				} else {
					warn.Err(respErr)
				}
				warn.Msg("failed to fetch ion identity public keys, retrying")
			}
		}).
		SetRetryCondition(func(resp *req.Response, err error) bool {
			return err != nil || (resp.GetStatusCode() != http.StatusOK && resp.GetStatusCode() != http.StatusNoContent)
		}).
		AddQueryParam("caller", "subzero").
		SetHeader("Accept", "application/json").
		SetHeader("Cache-Control", "no-cache, no-store, must-revalidate").
		SetHeader("Pragma", "no-cache").
		SetHeader("Expires", "0").
		AddQueryParam("version", version).
		Get("/v1/config/service_pubkeys")

	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch public keys from ion identity")
	}
	if resp.GetStatusCode() != http.StatusOK {
		if resp.GetStatusCode() == http.StatusNoContent {
			return nil, nil
		}
		return nil, errors.Errorf("ion identity public keys service responded with status: %d", resp.GetStatusCode())
	}

	var keys []string
	err = resp.UnmarshalJson(&keys)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal ion identity public keys response")
	}

	return &publicKeys{
		Keys:    keys,
		Version: resp.GetHeader("X-Version"),
	}, nil
}
