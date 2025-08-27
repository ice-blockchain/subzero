// SPDX-License-Identifier: ice License 1.0

package ionidentitypubkeys

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/imroc/req/v3"
	"golang.org/x/net/http2"
)

type (
	IONIdentityKeys interface {
		PublicKeys() []string
	}
	ionIdentityPublicKeysFetcher struct {
		client     *req.Client
		publicKeys atomic.Pointer[publicKeys]
	}
	publicKeys struct {
		Keys    []string
		Version string
	}
)

func MustNewIONIdentityPublicKeys(ctx context.Context, baseUrl string) IONIdentityKeys {
	if baseUrl == "" {
		log.Panic(errors.New("ion identity public keys base url not set"))
	}
	f := ionIdentityPublicKeysFetcher{
		client: req.C().SetBaseURL(baseUrl),
	}
	f.client.GetClient().Transport = &http2.Transport{}
	f.client.GetClient().Timeout = 30 * time.Second
	f.client.SetJsonMarshal(json.Marshal)
	f.client.SetJsonUnmarshal(json.Unmarshal)
	err := f.syncPubKeys()
	if err != nil {
		log.Panic(errors.Wrapf(err, "failed to sync ion identity public keys during startup"))
	}
	go f.startSync(ctx)
	return &f
}

func (f *ionIdentityPublicKeysFetcher) PublicKeys() []string {
	if keys := f.publicKeys.Load(); keys != nil {
		return keys.Keys
	}
	err := f.syncPubKeys()
	if err != nil {
		log.Panic(errors.Wrapf(err, "failed to sync ion identity public keys during startup"))
	}
	keys := f.publicKeys.Load()
	return keys.Keys
}
func (f *ionIdentityPublicKeysFetcher) startSync(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := f.syncPubKeys(); err != nil {
				log.Printf("failed to sync ion identity public keys: %v", err)
			}
		}
	}
}

func (f *ionIdentityPublicKeysFetcher) syncPubKeys() error {
	reqCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	prev := f.publicKeys.Load()
	version := "0"
	if prev != nil {
		version = prev.Version
	}
	keys, err := f.fetchPubKeys(reqCtx, version)
	if err != nil {
		return errors.Wrapf(err, "failed to fetch public keys from ion identity")
	}
	f.publicKeys.Store(keys)

	return nil
}

func (f *ionIdentityPublicKeysFetcher) fetchPubKeys(ctx context.Context, version string) (*publicKeys, error) {
	resp, err := f.client.R().
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
			return f.publicKeys.Load(), nil
		}
		return nil, fmt.Errorf("ion identity public keys service responded with status: %d", resp.GetStatusCode())
	} else if body, bErr := resp.ToBytes(); bErr != nil {
		return nil, errors.Wrap(bErr, "failed to read ion identity public keys response")
	} else {
		var keys []string
		err = json.Unmarshal(body, &keys)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to unmarshal ion identity public keys response: %v", string(body))
		}
		return &publicKeys{
			Keys:    keys,
			Version: resp.GetHeader("X-Version"),
		}, nil
	}
}
