// SPDX-License-Identifier: ice License 1.0

package internal

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"
	"github.com/imroc/req/v3"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/rs/zerolog/log"
)

type (
	CDNClient interface {
		FileUpload(ctx context.Context, data io.Reader, contentType, fileName string) error
		FileDelete(ctx context.Context, name string) error
		Stop(ctx context.Context) error
		FileUploadAsync(ctx context.Context, filePath, contentType, fileName string) error
		CdnDownloadURL(filename string) string
	}
	CdnConfig struct {
		AccessKey       string   `yaml:"access-key"`
		URLUpload       string   `yaml:"url-upload"`
		URLDownload     string   `yaml:"url-download"`
		MaxQueueWorkers int      `yaml:"max-queue-workers"`
		DBWriteUrls     []string `yaml:"db-write-urls"`
	}
	client struct {
		river.WorkerDefaults[*jobParams]
		config *CdnConfig
		river  *river.Client[pgx.Tx]
		db     *DB
	}
)

func NewCDNClient(ctx context.Context, config *CdnConfig) CDNClient {
	if config.MaxQueueWorkers == 0 {
		config.MaxQueueWorkers = 100
	}
	c := &client{
		config: config,
	}
	bootstrapCtx, cancelBootstrap := context.WithTimeout(ctx, 30*time.Second)
	defer cancelBootstrap()
	resp, err := c.cdnReq(bootstrapCtx).Delete(c.cdnUploadURL(uuid.NewString() + ".jpg"))
	if err != nil {
		log.Panic().
			Str("context", "STORAGE").
			Err(err).
			Msg("failed to bootstrap cdn")
	}
	if resp.GetStatusCode() != http.StatusNotFound {
		log.Panic().
			Str("context", "STORAGE").
			Int("status", resp.GetStatusCode()).
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

func (c *client) CdnDownloadURL(filename string) string {
	if strings.HasPrefix(filename, c.config.URLDownload) {
		return filename
	}
	u, _ := url.JoinPath(c.config.URLDownload, filename)

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
	downloadURL := c.CdnDownloadURL("*")
	filename = strings.Replace(filename, downloadURL[:len(downloadURL)-1], "", 1)
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
