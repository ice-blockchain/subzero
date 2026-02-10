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
	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/rq"
)

type (
	CDNClient interface {
		FileDelete(ctx context.Context, name string) error
		FileUploadAsync(ctx context.Context, filePath, contentType, fileName string) error
		HealthCheck(ctx context.Context) error
		FileUpload(ctx context.Context, data io.Reader, contentType, fileName string) error
	}
	CDNConfig struct {
		AccessKey     string        `yaml:"access-key"`
		URLUpload     string        `yaml:"url-upload"`
		URLDownload   string        `yaml:"url-download"`
		JobMaxTimeout time.Duration `yaml:"max-job-timeout"`
	}

	client struct {
		RqClient            rq.Client
		Config              *CDNConfig
		RelayURL            string
		RootPath            string
		HealthCheckPassedAt atomic.Int64
		HealthCheckMux      sync.RWMutex
	}
)

const (
	defaultJobTimeout = 10 * time.Minute
)

func NewCDNClient(ctx context.Context, config *CDNConfig, rqClient rq.Client, relayUrl, rootPath string) CDNClient {
	var cdnClient = &client{
		Config:   config,
		RelayURL: relayUrl,
		RootPath: rootPath,
		RqClient: rqClient,
	}

	rq.RegisterWorker(rqClient.Register(), &cdnUploadWorker{
		Client:     cdnClient,
		JobTimeout: config.JobMaxTimeout,
		RootPath:   rootPath,
		RelayURL:   relayUrl,
	})
	if err := cdnClient.HealthCheck(ctx); err != nil {
		log.Panic().
			Str("context", "STORAGE").
			Err(err).
			Msg("failed to bootstrap cdn")
	}
	return cdnClient
}

func (c *client) cdnUploadURL(filename string) string {
	if strings.HasPrefix(filename, c.Config.URLUpload) {
		return filename
	}
	u, _ := url.JoinPath(c.Config.URLUpload, filename)
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
	if err != nil {
		return errors.Wrap(err, "upload file request failed")
	}

	if resp.IsSuccessState() {
		return nil
	}

	body, err := resp.ToString()
	log.Error().
		Str("context", "STORAGE").
		Err(err).
		Str("body", body).
		Str("file", fileName).
		Str("content_type", contentType).
		Int("status", resp.GetStatusCode()).
		Msg("failed to upload file")

	return errors.Errorf("upload file failed: code %v", resp.GetStatusCode())
}

func (c *client) FileDelete(ctx context.Context, name string) error {
	filename := name
	if filename == "" {
		return nil
	}

	resp, err := c.cdnReq(ctx).Delete(c.cdnUploadURL(filename))
	if err == nil && (resp.IsSuccessState() || resp.GetStatusCode() == http.StatusNotFound) {
		return nil
	}

	if err == nil && !resp.IsSuccessState() && resp.GetStatusCode() != http.StatusNotFound {
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
		SetRetryBackoffInterval(100*time.Millisecond, 1*time.Second).
		SetRetryHook(func(resp *req.Response, err error) {
			var body string
			if resp != nil {
				body, _ = resp.ToString()
			}
			switch {
			case err != nil:
				log.Error().Str("context", "STORAGE").Err(err).Str("body", body).Msg("failed to upload file, retrying... ")
			case resp.GetStatusCode() == http.StatusTooManyRequests:
				log.Error().Str("context", "STORAGE").Int("status", resp.GetStatusCode()).Str("body", body).Msg("rate limit for upload file reached, retrying...")
			case resp.GetStatusCode() >= http.StatusInternalServerError:
				log.Error().Str("context", "STORAGE").Int("status", resp.GetStatusCode()).Str("body", body).Msg("internal server error for upload file, retrying...")
			}
		}).
		SetRetryCount(25).
		SetRetryCondition(func(resp *req.Response, err error) bool {
			return err != nil || resp.GetStatusCode() == http.StatusTooManyRequests || resp.GetStatusCode() >= http.StatusInternalServerError
		}).
		SetHeader("AccessKey", c.Config.AccessKey)
}

func (c *client) HealthCheck(ctx context.Context) error {
	locked := c.HealthCheckMux.TryLock()
	if hPassed := time.Unix(c.HealthCheckPassedAt.Load(), 0); !locked || time.Since(hPassed) <= 30*time.Second {
		return nil
	}
	defer func() {
		if locked {
			c.HealthCheckMux.Unlock()
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
	if err = c.RqClient.HealthCheck(ctx); err != nil {
		return errors.Wrapf(err, "failed to perform rq client health check")
	}
	c.HealthCheckPassedAt.Store(time.Now().Unix())
	return nil
}

func (c *client) FileUploadAsync(ctx context.Context, filePath, contentType, fileName string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	err := c.RqClient.Push(ctx, &cdnUploadWorkerArgs{
		ContentType: contentType,
		FileName:    fileName,
		FilePath:    filePath,
		RelayURL:    c.RelayURL,
	})
	return errors.Wrapf(err, "failed to enqueue cdn upload job for file %v", fileName)
}
