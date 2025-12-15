// SPDX-License-Identifier: ice License 1.0

package rq

import (
	"cmp"
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/rs/zerolog/log"
	slogzerolog "github.com/samber/slog-zerolog/v2"

	"github.com/ice-blockchain/subzero/cfg"
)

type (
	Register = river.Workers
	JobArgs  = river.JobArgs

	Job[T JobArgs]            = river.Job[T]
	Worker[T JobArgs]         = river.Worker[T]
	WorkerDefaults[T JobArgs] = river.WorkerDefaults[T]

	DBConfig struct {
		Username  string   `yaml:"username,omitempty"`
		Password  string   `yaml:"password,omitempty"`
		WriteUrls []string `yaml:"write-urls" validate:"required"`
	}
	Config struct {
		QueueName       string        `yaml:"queue-name"`                        // Queue name to use, could be empty.
		RelayURL        string        `yaml:"relay-url" validate:"required,url"` // Relay URL of this job queue client.
		ID              string        `yaml:"id,omitempty"`                      // ID of this client.
		DB              DBConfig      `yaml:"db"`
		MaxQueueWorkers int           `yaml:"max-queue-workers"`
		JobMaxTimeout   time.Duration `yaml:"max-job-timeout"`
	}
	Option func(*riverq)

	Client interface {
		Register() *Register
		Push(ctx context.Context, jobs ...JobArgs) error
		Stop(ctx context.Context) error
		Start(ctx context.Context) error
	}

	riverClient = river.Client[pgx.Tx]

	riverq struct {
		Logger         *slog.Logger
		WorkerRegister *river.Workers
		DB             *databaseClient
		Cfg            *Config
		River          atomic.Pointer[riverClient]
	}
)

const (
	defaultJobTimeout = 10 * time.Minute
	defaultWorkers    = 95
)

var (
	ErrNotConnected     = errors.New("not connected to job queue")
	ErrInvalidArguments = errors.New("invalid arguments")
)

func newClient(ctx context.Context, opts ...Option) (*riverq, error) {
	var client riverq

	client.Logger = slog.New(slogzerolog.Option{Level: slog.LevelDebug, Logger: &log.Logger}.NewZerologHandler()).
		With("context", "rq")

	if err := client.initOptions(opts...); err != nil {
		return nil, errors.Wrap(err, "failed to configure client")
	}

	client.WorkerRegister = river.NewWorkers()

	log.Debug().Str("context", "rq").Str("queue_name", client.Cfg.QueueName).Msg("initializing job queue client")

	db, err := newDatabaseClient(ctx, client.Cfg.DB.Username, client.Cfg.DB.Password, client.Cfg.DB.WriteUrls...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create database client")
	}

	if err := client.initDatabaseClient(ctx, db); err != nil {
		return nil, errors.Wrap(err, "failed to initialize database client")
	}

	if err := client.initRiverClient(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to initialize river client")
	}

	return &client, nil
}

func WithConfig(cfg *Config) Option {
	return func(q *riverq) {
		q.Cfg = cfg
	}
}

func MustNewClient[T JobArgs](ctx context.Context, opts ...Option) Client {
	client, err := newClient(ctx, opts...)
	if err != nil {
		log.Panic().Err(err).Msg("failed to create job queue client")
	}
	return client
}

func (q *riverq) Register() *Register {
	return q.WorkerRegister
}

func (q *riverq) initOptions(opts ...Option) error {
	for _, opt := range opts {
		opt(q)
	}

	if q.Cfg == nil {
		q.Cfg = cfg.MustGet[Config]()
	}

	if q.Cfg.QueueName == "" && q.Cfg.ID == "" && q.Cfg.RelayURL == "" {
		return errors.Wrapf(ErrInvalidArguments, "either queue name or client ID or relay URL must be provided")
	}

	if q.Cfg.MaxQueueWorkers <= 0 {
		q.Cfg.MaxQueueWorkers = defaultWorkers
	}
	if q.Cfg.JobMaxTimeout <= 0 {
		q.Cfg.JobMaxTimeout = defaultJobTimeout
	}

	if q.Cfg.QueueName == "" {
		q.Cfg.QueueName = formatQueueName(strings.Join([]string{q.Cfg.RelayURL, q.Cfg.ID}, "_"))
	}

	return nil
}

func (q *riverq) initRiverClient(ctx context.Context) error {
	id := cmp.Or(q.Cfg.ID, formatQueueName(q.Cfg.RelayURL))
	log.Info().
		Str("context", "rq").
		Str("client_id", id).
		Str("queue_name", q.Cfg.QueueName).
		Msg("initializing river client")

	rClient, err := river.NewClient(
		riverpgxv5.New(q.DB.Get()),
		&river.Config{
			Queues: map[string]river.QueueConfig{
				q.Cfg.QueueName: {
					MaxWorkers: q.Cfg.MaxQueueWorkers,
				},
			},
			Workers:    q.WorkerRegister,
			JobTimeout: q.Cfg.JobMaxTimeout,
			ID:         id,
			Logger:     q.Logger,
		},
	)
	if err != nil {
		return errors.Wrap(err, "cannot create river client")
	}

	old := q.River.Swap(rClient)
	if old != nil {
		log.Info().Str("context", "rq").Msg("stopping old river client")
		old.Stop(ctx)
	}
	return err
}

func (q *riverq) initDatabaseClient(ctx context.Context, db *databaseClient) (err error) {
	migrator, err := rivermigrate.New(riverpgxv5.New(db.Get()), &rivermigrate.Config{
		Logger: q.Logger,
	})
	if err != nil {
		return errors.Wrap(err, "cannot create river migrator")
	}

	_, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	if err != nil {
		return errors.Wrap(err, "failed to migrate river")
	}

	q.DB = db

	return nil
}

func (q *riverq) Stop(ctx context.Context) error {
	o := q.River.Load()
	if o == nil {
		return nil
	}
	return o.Stop(ctx)
}

func (q *riverq) Start(ctx context.Context) error {
	o := q.River.Load()
	if o == nil {
		return ErrNotConnected
	}
	return o.Start(ctx)
}

func (q *riverq) Push(ctx context.Context, jobs ...JobArgs) error {
	err := q.push(ctx, jobs...)
	if !shouldSwitchMaster(err) {
		return err
	}

	err = q.DB.switchMaster(ctx, err)
	if err != nil {
		return errors.Wrap(err, "failed to switch master")
	}

	q.initDatabaseClient(ctx, q.DB)
	err = q.initRiverClient(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to reinitialize river client after master switch")
	}

	return q.push(ctx, jobs...)
}

func (q *riverq) push(ctx context.Context, jobs ...JobArgs) error {
	if len(jobs) == 0 {
		return nil
	}

	o := q.River.Load()
	if o == nil {
		return ErrNotConnected
	}

	opts := &river.InsertOpts{
		UniqueOpts: river.UniqueOpts{ByArgs: true},
		Queue:      q.Cfg.QueueName,
	}

	if len(jobs) == 1 {
		res, err := o.Insert(ctx, jobs[0], opts)
		if err == nil {
			log.Debug().Str("context", "rq").
				Int64("job-id", res.Job.ID).
				Str("kind", res.Job.Kind).
				Msg("pushed job to queue")
		}
		return err
	}

	var params []river.InsertManyParams
	for i := range jobs {
		params = append(params, river.InsertManyParams{
			Args:       jobs[i],
			InsertOpts: opts,
		})
	}

	res, err := o.InsertMany(ctx, params)
	if err != nil {
		return err
	}

	for _, r := range res {
		log.Debug().Str("context", "rq").
			Int64("job-id", r.Job.ID).
			Str("kind", r.Job.Kind).
			Msg("pushed job to queue")
	}

	return nil
}

func formatQueueName(name string) string {
	const prefix = "rq_"
	var sb strings.Builder
	var runes = map[rune]rune{
		':': '_',
		'/': '_',
		'.': '_',
		'_': '_', // To avoid consecutive underscores at the edges.
	}

	sb.Grow(len(name) + len(prefix))
	sb.WriteString(prefix)
	var lastReplaced bool
	for i, r := range name {
		replacement, ok := runes[r]
		if ok {
			if !lastReplaced && i != len(name)-1 && i != 0 {
				sb.WriteRune(replacement)
			}
			lastReplaced = true
		} else {
			sb.WriteRune(r)
			lastReplaced = false
		}
	}
	return sb.String()
}

func RegisterWorker[T JobArgs](register *Register, worker Worker[T]) {
	river.AddWorker(register, worker)
}
