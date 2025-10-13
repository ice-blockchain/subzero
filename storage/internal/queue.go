// SPDX-License-Identifier: ice License 1.0

package internal

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/database/query"
)

type (
	jobParams struct {
		ContentType string `json:"contentType"`
		FileName    string `json:"fileName"`
		FilePath    string `json:"filePath"`
	}
)

var driver struct {
	sync.Once
	riverdriver.Driver[pgx.Tx]
}

func (c *client) FileUploadAsync(ctx context.Context, filePath, contentType, fileName string) error {
	if c.river == nil {
		return nil
	}
	res, err := c.river.Insert(ctx, &jobParams{
		ContentType: contentType,
		FileName:    fileName,
		FilePath:    filePath,
	}, nil)
	if err != nil {
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
	workers := river.NewWorkers()
	if err := river.AddWorkerSafely[*jobParams](workers, c); err != nil {
		return errors.Wrap(err, "failed to register cdnUpload worker")
	}
	query.RequestDatabaseConn(ctx, query.WithDDLFunc("river", func(ctx context.Context, pool *pgxpool.Pool) error {
		driver.Do(func() {
			driver.Driver = riverpgxv5.New(pool)
		})
		migrator, err := rivermigrate.New(driver.Driver, &rivermigrate.Config{})
		if err != nil {
			return errors.Wrap(err, "cannot create river migrator")
		}
		_, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
		return errors.Wrap(err, "failed to migrate river")
	}), query.WithMasterSwitchCallback(func(ctx context.Context, newMaster *pgxpool.Pool) (err error) {
		if c.river == nil {
			return nil
		}
		if err = c.river.Stop(ctx); err != nil {
			return errors.Wrap(err, "failed to stop river for old master")
		}
		c.river = nil
		driver.Driver = nil
		driver.Once = sync.Once{}
		driver.Do(func() {
			driver.Driver = riverpgxv5.New(newMaster)
		})
		c.river, err = river.NewClient[pgx.Tx](driver.Driver, &river.Config{
			Queues: map[string]river.QueueConfig{
				river.QueueDefault: {MaxWorkers: cfg.MaxQueueWorkers},
			},
			Workers:    workers,
			JobTimeout: 10 * time.Minute,
		})
		if err != nil {
			return errors.Wrap(err, "failed to create river client with switched master")
		}
		if err = c.river.Start(ctx); err != nil {
			return errors.Wrap(err, "failed to start river after master switch")
		}
		return nil
	}))

	riverClient, err := river.NewClient[pgx.Tx](driver.Driver, &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: cfg.MaxQueueWorkers},
		},
		Workers:    workers,
		JobTimeout: 10 * time.Minute,
	})
	if err != nil {
		return errors.Wrap(err, "failed to create river client")
	}
	if err = riverClient.Start(ctx); err != nil {
		return errors.Wrap(err, "failed to start river")
	}
	c.river = riverClient
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
	f, err := os.Open(job.Args.FilePath)
	if err != nil {
		if os.IsNotExist(err) {
			err = nil
		}
		return errors.Wrapf(err, "failed to open %v", job.Args.FilePath)
	}
	defer f.Close()
	return errors.Wrapf(c.FileUpload(ctx, f, job.Args.ContentType, job.Args.FileName), "failed to upload file %v", job.Args.FileName)
}

func (c *client) Stop(ctx context.Context) error {
	if c.river == nil {
		return nil
	}
	return errors.Wrap(c.river.Stop(ctx), "error stopping river")
}
