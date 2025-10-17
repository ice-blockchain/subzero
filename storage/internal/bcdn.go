// SPDX-License-Identifier: ice License 1.0

package internal

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"
	"github.com/imroc/req/v3"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/rs/zerolog/log"
)

type (
	CDNClient interface {
		FileUpload(ctx context.Context, data io.Reader, contentType, fileName string) error
		FileDelete(ctx context.Context, name string) error
		Stop(ctx context.Context) error
		FileUploadAsync(ctx context.Context, filePath, contentType, fileName string) error
		HealthCheck(ctx context.Context) error
	}
	CdnConfig struct {
		AccessKey       string        `yaml:"access-key"`
		URLUpload       string        `yaml:"url-upload"`
		URLDownload     string        `yaml:"url-download"`
		MaxQueueWorkers int           `yaml:"max-queue-workers"`
		JobMaxTimeout   time.Duration `yaml:"max-job-timeout"`
		DB              struct {
			WriteUrls []string `yaml:"write-urls"`
			Username  string   `yaml:"username,omitempty"`
			Password  string   `yaml:"password,omitempty"`
		} `yaml:"db"`
	}
	client struct {
		river.WorkerDefaults[*jobParams]
		workers             *river.Workers
		config              *CdnConfig
		relayUrl            string
		rootPath            string
		river               atomic.Pointer[river.Client[pgx.Tx]]
		db                  *DB
		healthCheckPassedAt atomic.Int64
		healthCheckMtx      sync.Mutex
	}
)

const (
	defaultJobTimeout = 10 * time.Minute
	defaultWorkers    = 95
)

func NewCDNClient(ctx context.Context, config *CdnConfig, relayUrl, rootPath string) CDNClient {
	if config.MaxQueueWorkers == 0 {
		config.MaxQueueWorkers = defaultWorkers
	}
	if config.JobMaxTimeout == 0 {
		config.JobMaxTimeout = defaultJobTimeout
	}
	c := &client{
		config:   config,
		relayUrl: relayUrl,
		rootPath: rootPath,
	}
	var err error
	c.workers = river.NewWorkers()
	if err = river.AddWorkerSafely[*jobParams](c.workers, c); err != nil {
		log.Panic().
			Str("context", "STORAGE").
			Err(err).
			Msg("failed to register cdnUpload worker")
	}
	c.db, err = NewDBConn(ctx,
		WithWriteURLs(config.DB.Username, config.DB.Password, config.DB.WriteUrls...),
		WithMigration("river", func(ctx context.Context, pool *pgxpool.Pool) error {
			migrator, err := rivermigrate.New[pgx.Tx](riverpgxv5.New(pool), &rivermigrate.Config{})
			if err != nil {
				return errors.Wrap(err, "cannot create river migrator")
			}
			_, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
			return errors.Wrap(err, "failed to migrate river")
		}),
	)
	if err != nil {
		log.Panic().
			Str("context", "STORAGE").
			Err(err).
			Msg("failed setup db connection")
	}
	if err = c.HealthCheck(ctx); err != nil {
		log.Panic().
			Str("context", "STORAGE").
			Err(err).
			Msg("failed to bootstrap cdn")
	}
	if err = c.initQueueProcessing(ctx, config); err != nil {
		log.Panic().
			Str("context", "STORAGE").
			Err(err).
			Msg("failed to init queue processing")
	}
	return c
}

func (c *client) cdnUploadURL(filename string) string {
	if strings.HasPrefix(filename, c.config.URLUpload) {
		return filename
	}
	u, _ := url.JoinPath(c.config.URLUpload, filename)

	return u
}

func (c *client) FileUpload(ctx context.Context, data io.Reader, contentType, fileName string) (err error) {
	fileData, err := io.ReadAll(data)
	if err != nil {
		return errors.Wrapf(err, "error reading file %v", fileName)
	}

	return errors.Wrapf(c.doCdnUpload(ctx, contentType, fileName, fileData), "error uploading file %v", fileName)
}

func (c *client) doCdnUpload(ctx context.Context, contentType, fileName string, fileData []byte) error {
	resp, err := c.cdnReq(ctx).
		SetHeader("Content-Type", contentType).
		SetBodyBytes(fileData).
		Put(c.cdnUploadURL(fileName))
	if err == nil && resp.IsSuccessState() {
		return nil
	}
	if err == nil && !resp.IsSuccessState() {
		body, rErr := resp.ToString()
		if rErr != nil {
			log.Error().Str("context", "STORAGE").Err(rErr).Int("status", resp.GetStatusCode()).Msg("failed to upload file")
		}

		err = errors.Errorf("upload new file failed with status: %v,body: %v", resp.GetStatusCode(), body)
	}

	return errors.Wrap(err, "upload file request failed")
}

func (c *client) FileDelete(ctx context.Context, name string) error {
	filename := name
	if filename == "" {
		return nil
	}
	resp, err := c.cdnReq(ctx).Delete(c.cdnUploadURL(filename))
	if err == nil && (resp.IsSuccessState() || resp.GetStatusCode() == 404) {
		return nil
	}
	if err == nil && !resp.IsSuccessState() && resp.GetStatusCode() != 404 {
		body, rErr := resp.ToString()
		if rErr != nil {
			log.Error().Str("context", "STORAGE").Err(rErr).Int("status", resp.GetStatusCode()).Msg("failed to delete file")
		}

		err = errors.Errorf("delete file failed with status: %v,body: %v", resp.GetStatusCode(), body)
	}

	return errors.Wrap(err, "delete file request failed")
}

func (c *client) cdnReq(ctx context.Context) *req.Request {
	return req.
		SetContext(ctx).
		SetRetryBackoffInterval(10*time.Millisecond, 1*time.Second). //nolint:mnd,gomnd // .
		SetRetryHook(func(resp *req.Response, err error) {
			switch { //nolint:revive // .
			case err != nil:
				log.Error().Str("context", "STORAGE").Err(err).Msg("failed to upload file, retrying... ")
			case resp.GetStatusCode() == http.StatusTooManyRequests:
				log.Error().Str("context", "STORAGE").Int("status", resp.GetStatusCode()).Msg("rate limit for upload file reached, retrying...")
			case resp.GetStatusCode() >= http.StatusInternalServerError:
				log.Error().Str("context", "STORAGE").Int("status", resp.GetStatusCode()).Msg("internal server error for upload file, retrying...")
			}
		}).
		SetRetryCount(25).
		SetRetryCondition(func(resp *req.Response, err error) bool {
			return err != nil || resp.GetStatusCode() == http.StatusTooManyRequests || resp.GetStatusCode() >= http.StatusInternalServerError
		}).
		SetHeader("AccessKey", c.config.AccessKey)
}

func (c *client) HealthCheck(ctx context.Context) error {
	locked := c.healthCheckMtx.TryLock()
	if hPassed := time.Unix(c.healthCheckPassedAt.Load(), 0); !locked || time.Now().Sub(hPassed) <= 30*time.Second {
		return nil
	}
	defer func() {
		if locked {
			c.healthCheckMtx.Unlock()
		}
	}()
	bootstrapCtx, cancelBootstrap := context.WithTimeout(ctx, 30*time.Second)
	defer cancelBootstrap()
	resp, err := c.cdnReq(bootstrapCtx).Delete(c.cdnUploadURL(uuid.NewString() + ".jpg"))
	if err != nil {
		return errors.Wrap(err, "cdn healthcheck failed")
	}
	if resp.GetStatusCode() != http.StatusNotFound {
		return errors.Errorf("cdn healthcheck failed with status: %v", resp.GetStatusCode())
	}
	if err = c.db.Ping(ctx); err != nil {
		return errors.Wrapf(err, "failed to ping database")
	}
	c.healthCheckPassedAt.Store(time.Now().Unix())
	return nil
}
