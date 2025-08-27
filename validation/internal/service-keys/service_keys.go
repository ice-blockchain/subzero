// SPDX-License-Identifier: ice License 1.0

package servicekeys

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
	ServiceKeys interface {
		ServiceKeys() []string
	}
	ionIdentityServiceKeysFetcher struct {
		client      *req.Client
		serviceKeys atomic.Pointer[serviceKeys]
	}
	serviceKeys struct {
		Keys    []string
		Version string
	}
)

func MustNewIONIdentityServiceKeys(ctx context.Context, baseUrl string) ServiceKeys {
	if baseUrl == "" {
		log.Panic(errors.New("service keys base url not set"))
	}
	f := ionIdentityServiceKeysFetcher{
		client: req.C().SetBaseURL(baseUrl),
	}
	f.client.GetClient().Transport = &http2.Transport{}
	f.client.GetClient().Timeout = 30 * time.Second
	f.client.SetJsonMarshal(json.Marshal)
	f.client.SetJsonUnmarshal(json.Unmarshal)
	err := f.syncServiceKeys()
	if err != nil {
		log.Panic(errors.Wrapf(err, "failed to sync service keys during startup"))
	}
	go f.startSync(ctx)
	return &f
}

func (f *ionIdentityServiceKeysFetcher) ServiceKeys() []string {
	if keys := f.serviceKeys.Load(); keys != nil {
		return keys.Keys
	}
	return nil
}
func (f *ionIdentityServiceKeysFetcher) startSync(ctx context.Context) {
	for ctx.Err() == nil {
		timeSleep := 1 * time.Hour
		if err := f.syncServiceKeys(); err != nil {
			log.Printf("failed to sync service keys: %v", err)
			timeSleep = 30 * time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(timeSleep):
		}
	}
}

func (f *ionIdentityServiceKeysFetcher) syncServiceKeys() error {
	reqCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	prev := f.serviceKeys.Load()
	version := "0"
	if prev != nil {
		version = prev.Version
	}
	keys, err := f.fetchServiceKey(reqCtx, version)
	if err != nil {
		return errors.Wrapf(err, "failed to fetch service keys")
	}
	f.serviceKeys.Store(keys)

	return nil
}

func (f *ionIdentityServiceKeysFetcher) fetchServiceKey(ctx context.Context, version string) (*serviceKeys, error) {
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
				log.Printf("failed to fetch service keys, retrying...: %v", err)
			} else {
				log.Printf("failed to fetch service keys with status code:%v, retrying...", resp.GetStatusCode())
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
		return nil, errors.Wrap(err, "failed to fetch service keys")
	}
	if resp.GetStatusCode() != http.StatusOK {
		if resp.GetStatusCode() == http.StatusNoContent {
			return f.serviceKeys.Load(), nil
		}
		return nil, fmt.Errorf("service keys service responded with status: %d", resp.GetStatusCode())
	} else if body, bErr := resp.ToBytes(); bErr != nil {
		return nil, errors.Wrap(bErr, "failed to read service keys response")
	} else {
		var keys []string
		err = json.Unmarshal(body, &keys)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to unmarshal service keys response: %v", string(body))
		}
		return &serviceKeys{
			Keys:    keys,
			Version: resp.GetHeader("X-Version"),
		}, nil
	}
}
