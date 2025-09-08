// SPDX-License-Identifier: ice License 1.0

package ionidentitypubkeys

import (
	"context"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/goccy/go-json"
	"github.com/imroc/req/v3"
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
		Keys    []string
		Version string
	}
)

func MustNewIONIdentityPublicKeys(ctx context.Context, baseUrl string) IONIdentityKeysProvider {
	if baseUrl == "" {
		log.Panic(errors.New("ion identity public keys base url not set"))
	}

	f := &ionIdentityPublicKeysFetcher{
		Client: req.C().
			SetBaseURL(baseUrl).
			SetTimeout(30 * time.Second).
			SetJsonMarshal(json.Marshal).
			SetJsonUnmarshal(json.Unmarshal).
			EnableH2C(),
	}

	err := f.syncPubKeys(ctx)
	if err != nil {
		log.Panicf("failed to sync ion identity public keys during startup: %v", err)
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
				log.Printf("failed to sync ion identity public keys: %v", err)
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
	log.Printf("fetched %d ion identity public keys, version: %s", len(keys.Keys), keys.Version)

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
				log.Printf("failed to fetch ion identity public keys, retrying...: %v", err)
			} else {
				log.Printf("failed to fetch ion identity public keys with status code:%v, retrying...", resp.GetStatusCode())
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
