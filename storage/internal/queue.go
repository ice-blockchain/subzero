// SPDX-License-Identifier: ice License 1.0

package internal

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/rs/zerolog/log"
)

type (
	jobParams struct {
		ContentType string `json:"contentType"`
		FileName    string `json:"fileName"`
		FilePath    string `json:"filePath"`
	}
)

func formatQueueName(name string) string {
	return strings.ReplaceAll(
		strings.ReplaceAll(strings.ReplaceAll(name, ":", "_"), "/", ""),
		".", "_")
}

func (c *client) FileUploadAsync(ctx context.Context, filePath, contentType, fileName string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if c.river.Load() == nil {
		return nil
	}
	res, err := c.river.Load().Insert(ctx, &jobParams{
		ContentType: contentType,
		FileName:    fileName,
		FilePath:    filePath,
	}, &river.InsertOpts{
		UniqueOpts: river.UniqueOpts{ByArgs: true, ByQueue: false},
		Queue:      formatQueueName(c.relayUrl),
	})
	if err != nil {
		if isDBDead(err) {
			err = errors.Join(err, c.db.switchMaster(ctx, err))
			if err = c.river.Load().Stop(ctx); err != nil {
				return errors.Wrap(err, "failed to stop river for old master")
			}
			c.river.Store(nil)
			if err = c.initQueueProcessing(ctx, c.config); err != nil {
				return errors.Wrap(err, "failed to reinit queue processing dur to master switch")
			}
			return c.FileUploadAsync(ctx, filePath, contentType, fileName)
		}
		return errors.Wrapf(err, "failed to insert job for file %v upload", fileName)
	}
	log.Debug().Str("context", "STORAGE").
		Str("file", fileName).
		Str("path", filePath).
		Int64("jobID", res.Job.ID).
		Msg("enqueue upload to cdn")
	return nil
}

func (j *jobParams) Kind() string {
	return "cdnUpload"
}
func (c *client) initQueueProcessing(ctx context.Context, cfg *CdnConfig) error {
	riverClient, err := river.NewClient[pgx.Tx](riverpgxv5.New(c.db.primary()), &river.Config{
		Queues: map[string]river.QueueConfig{
			formatQueueName(c.relayUrl): {MaxWorkers: cfg.MaxQueueWorkers},
		},
		Workers:    c.workers,
		JobTimeout: c.config.JobMaxTimeout,
		ID:         c.relayUrl,
	})
	if err != nil {
		return errors.Wrap(err, "failed to create river client")
	}
	if err = riverClient.Start(ctx); err != nil {
		return errors.Wrap(err, "failed to start river")
	}
	c.river.CompareAndSwap(nil, riverClient)
	return nil
}

func (c *client) Work(ctx context.Context, job *river.Job[*jobParams]) (err error) {
	defer func() {
		log.Debug().
			Str("context", "STORAGE").
			Err(err).
			Str("file", job.Args.FileName).
			Int64("jobID", job.ID).
			Int("attempt", job.Attempt).
			Msg("uploaded to cdn")
	}()
	log.Debug().
		Str("context", "STORAGE").
		Str("file", job.Args.FileName).
		Int64("jobID", job.ID).
		Msg("starting file upload to cdn")
	f, err := os.Open(filepath.Join(c.rootPath, job.Args.FilePath))
	if err != nil {
		if os.IsNotExist(err) && job.Queue == formatQueueName(c.relayUrl) {
			err = nil
		}
		return errors.Wrapf(err, "failed to open %v", job.Args.FilePath)
	}
	defer f.Close()
	return errors.Wrapf(c.FileUpload(ctx, f, job.Args.ContentType, job.Args.FileName), "failed to upload file %v", job.Args.FileName)
}

func (c *client) Stop(ctx context.Context) error {
	if c.river.Load() == nil {
		return nil
	}
	if err := c.river.Load().Stop(ctx); err != nil {
		return errors.Wrap(err, "error stopping river")
	}
	return errors.Wrap(c.db.Close(), "error closing db")
}
