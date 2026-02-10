// SPDX-License-Identifier: ice License 1.0

package rq

import (
	"cmp"
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/rs/zerolog/log"
	slogzerolog "github.com/samber/slog-zerolog/v2"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/database/query"
)

type (
	Register = river.Workers
	JobArgs  = river.JobArgs

	InsertOpts = river.InsertOpts
	UniqueOpts = river.UniqueOpts

	Job[T JobArgs]            = river.Job[T]
	Worker[T JobArgs]         = river.Worker[T]
	WorkerDefaults[T JobArgs] = river.WorkerDefaults[T]

	Config struct {
		query.Config
		QueueName       string        `yaml:"queue-name"`   // Queue name to use, could be empty.
		ID              string        `yaml:"id,omitempty"` // ID of this client.
		MaxQueueWorkers int           `yaml:"max-queue-workers"`
		JobMaxTimeout   time.Duration `yaml:"max-job-timeout"`
	}
	Option func(*riverq)

	Client interface {
		Register() *Register
		Push(ctx context.Context, jobs ...JobArgs) error
		Stop(ctx context.Context) error
		Start(ctx context.Context) error
		HealthCheck(ctx context.Context) error
		Close(ctx context.Context) error
	}

	riverClient = river.Client[pgx.Tx]

	riverq struct {
		Logger         *slog.Logger
		WorkerRegister *river.Workers
		DB             *databaseClient
		Cfg            *Config
		River          atomic.Pointer[riverClient]
		SwitchMu       sync.Mutex
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

	db, err := newDatabaseClient(ctx, client.Cfg.Username, client.Cfg.Password, client.Cfg.WriteURLs...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create database client")
	}

	if err := client.initDatabaseClient(ctx, db); err != nil {
		return nil, errors.Wrap(err, "failed to initialize database client")
	}

	if err := client.initRiverClient(ctx, db.Get()); err != nil {
		return nil, errors.Wrap(err, "failed to initialize river client")
	}

	return &client, nil
}

func WithConfig(cfg *Config) Option {
	return func(q *riverq) {
		q.Cfg = cfg
	}
}

func MustNewClient(ctx context.Context, opts ...Option) Client {
	client, err := newClient(ctx, opts...)
	if err != nil {
		log.Panic().Err(err).Msg("failed to create job queue client")
	}
	return client
}

func (q *riverq) Close(ctx context.Context) error {
	return errors.Join(q.Stop(ctx), q.DB.Close())
}

func (q *riverq) Register() *Register {
	return q.WorkerRegister
}

func (q *riverq) initOptions(opts ...Option) error {
	for _, opt := range opts {
		opt(q)
	}

	if q.Cfg == nil {
		var err error
		q.Cfg, err = cfg.Load[Config]()
		if err != nil {
			return errors.Wrap(err, "failed to load configuration for rq client")
		}
	}

	if len(q.Cfg.WriteURLs) == 0 && len(q.Cfg.ReadURLs) == 0 {
		dbConf, err := cfg.Get[query.Config]()
		if err == nil && (len(dbConf.WriteURLs) > 0 || len(dbConf.ReadURLs) > 0 || dbConf.RelayURL != "") {
			log.Info().
				Str("context", "rq").
				Msg("using database configuration for rq client since no explicit configuration provided")
			q.Cfg.Config = *dbConf
		}
	}

	if err := cfg.Validate(q.Cfg); err != nil {
		return errors.Wrap(err, "configuration validation failed for rq client")
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

func (q *riverq) initRiverClient(ctx context.Context, pool *pgxpool.Pool) error {
	id := cmp.Or(q.Cfg.ID, formatQueueName(q.Cfg.RelayURL))
	log.Info().
		Str("context", "rq").
		Str("client_id", id).
		Str("queue_name", q.Cfg.QueueName).
		Msg("initializing river client")

	rClient, err := river.NewClient(
		riverpgxv5.New(pool),
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
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		log.Warn().Str("context", "rq").Msg("force stopping old river client")
		if err := old.StopAndCancel(ctx); err != nil {
			log.Error().Str("context", "rq").Err(err).Msg("force stopping old river client returned error")
		}
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
	return o.Stop(context.WithoutCancel(ctx))
}

func (q *riverq) Start(ctx context.Context) error {
	o := q.River.Load()
	if o == nil {
		return ErrNotConnected
	}
	return o.Start(ctx)
}

func (q *riverq) trySwitchMaster(ctx context.Context, reason error) error {
	q.SwitchMu.Lock()
	defer q.SwitchMu.Unlock()

	err := q.DB.switchMaster(ctx, reason)
	if err != nil {
		return errors.Wrap(err, "failed to switch master")
	}

	err = q.initDatabaseClient(ctx, q.DB)
	if err != nil {
		return errors.Wrap(err, "failed to reinitialize database client after master switch")
	}

	err = q.initRiverClient(ctx, q.DB.Get())
	if err != nil {
		return errors.Wrap(err, "failed to reinitialize river client after master switch")
	}

	if err := q.Start(ctx); err != nil {
		return errors.Wrap(err, "failed to restart river client after master switch")
	}

	return nil
}

func (q *riverq) Push(ctx context.Context, jobs ...JobArgs) error {
	for attempt := 1; ctx.Err() == nil; attempt++ {
		err := q.push(ctx, jobs...)
		if !shouldSwitchMaster(err) {
			return err
		}

		select {
		case <-time.After(time.Millisecond * 500):
			log.Debug().Str("context", "rq").Int("attempt", attempt).Msg("attempting to switch master and retry push")

		case <-ctx.Done():
			return ctx.Err()
		}

		if q.HealthCheck(ctx) == nil {
			log.Debug().Str("context", "rq").Int("attempt", attempt).Msg("health check passed, connection was already restored")
			continue
		}

		err = q.trySwitchMaster(ctx, err)
		if err != nil {
			log.Error().Err(err).Str("context", "rq").Int("attempt", attempt).Msg("master switching error")
			continue
		}
	}
	return ctx.Err()
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
				Int64("job_id", res.Job.ID).
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
			Int64("job_id", r.Job.ID).
			Str("kind", r.Job.Kind).
			Msg("pushed job to queue")
	}

	return nil
}

func (q *riverq) HealthCheck(ctx context.Context) error {
	o := q.River.Load()
	if o == nil {
		return ErrNotConnected
	}
	return q.DB.Ping(ctx)
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
	log.Info().
		Str("context", "rq").
		Type("worker", worker).
		Msg("registering worker")
	river.AddWorker(register, worker)
}
