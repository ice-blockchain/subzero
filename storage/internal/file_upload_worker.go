// SPDX-License-Identifier: ice License 1.0

package internal

import (
	"cmp"
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/rq"
)

type (
	cdnUploadWorkerArgs struct {
		ContentType string
		FileName    string
		FilePath    string
		RelayURL    string
	}
	cdnUploadWorker struct {
		rq.WorkerDefaults[cdnUploadWorkerArgs]
		Client     CDNClient
		RootPath   string
		RelayURL   string
		JobTimeout time.Duration
	}
)

func (cdnUploadWorkerArgs) Kind() string {
	return "storage_cdn_upload_worker_args"
}

func (w *cdnUploadWorker) Timeout(job *rq.Job[cdnUploadWorkerArgs]) time.Duration {
	return cmp.Or(w.JobTimeout, defaultJobTimeout)
}

func (w *cdnUploadWorker) Work(ctx context.Context, job *rq.Job[cdnUploadWorkerArgs]) (err error) {
	defer func() {
		log.Debug().
			Str("context", "STORAGE").
			Err(err).
			Str("file", job.Args.FileName).
			Int("attempt", job.Attempt).
			Msg("uploaded to cdn")
	}()

	log.Debug().
		Str("context", "STORAGE").
		Str("file", job.Args.FileName).
		Int("attempt", job.Attempt).
		Msg("starting file upload to cdn")

	f, err := os.Open(filepath.Join(w.RootPath, job.Args.FilePath))
	if err != nil {
		if os.IsNotExist(err) && strings.EqualFold(job.Args.RelayURL, w.RelayURL) {
			err = nil
		}
		return errors.Wrapf(err, "failed to open %v", job.Args.FilePath)
	}
	defer f.Close()

	return errors.Wrapf(w.Client.FileUpload(ctx, f, job.Args.ContentType, job.Args.FileName), "failed to upload file %v", job.Args.FileName)
}
