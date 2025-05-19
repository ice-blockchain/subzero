// // SPDX-License-Identifier: ice License 1.0

package fixture

import (
	"context"
	"log"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	internal "github.com/ice-blockchain/subzero/database/query/internal/postgres"
)

const (
	pgImage    = "postgres:17-alpine"
	pgPass     = "postgres"
	pgDatabase = "postgres"
	dbPort     = "5432/tcp"
)

type (
	Container struct {
		container *postgres.PostgresContainer
		seed      uint64
		mu        sync.Mutex
	}
	Option = testcontainers.CustomizeRequestOption
)

func WithConfigFile(filePath string) Option {
	return postgres.WithConfigFile(filePath)
}

func WithConfigData(cfgBody string) Option {
	return func(req *testcontainers.GenericContainerRequest) error {
		cfgFile := testcontainers.ContainerFile{
			Reader:            strings.NewReader(cfgBody),
			ContainerFilePath: "/etc/postgresql.conf",
			FileMode:          0o755,
		}

		req.Files = append(req.Files, cfgFile)
		req.Cmd = append(req.Cmd, "-c", "config_file=/etc/postgresql.conf")

		return nil
	}
}

func New(ctx context.Context, opts ...Option) *Container {
	var customizers []testcontainers.ContainerCustomizer

	customizers = append(customizers,
		postgres.WithDatabase(pgDatabase),
		postgres.WithPassword(pgPass),
		testcontainers.WithWaitStrategyAndDeadline(
			time.Minute,
			wait.ForExposedPort(),
			wait.ForSQL(nat.Port(dbPort), "pgx", func(host string, port nat.Port) string {
				u := url.URL{
					Scheme: "postgres",
					User:   url.UserPassword("postgres", pgPass),
					Host:   net.JoinHostPort(host, port.Port()),
					Path:   pgDatabase,
				}
				return u.String()
			}),
		),
	)

	if internal.Config != "" {
		customizers = append(customizers, WithConfigData(internal.Config))
	}

	for i := range opts {
		customizers = append(customizers, opts[i])
	}

	container, err := postgres.Run(ctx, pgImage, customizers...)
	if err != nil {
		log.Panicf("failed to start postgres container: %v", err)
	}

	return &Container{
		container: container,
		seed:      uint64(time.Now().UnixMilli()),
	}
}

func (c *Container) ConnectionString(ctx context.Context, dbName string) string {
	containerPort, err := c.container.MappedPort(ctx, dbPort)
	if err != nil {
		log.Panicf("failed to get mapped port: %v", err)
	}

	if dbName == "" {
		dbName = pgDatabase
	}

	host, err := c.container.Host(ctx)
	if err != nil {
		log.Panicf("failed to get container host: %v", err)
	}

	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword("postgres", pgPass),
		Host:   net.JoinHostPort(host, containerPort.Port()),
		Path:   dbName,
	}

	return u.String()
}

func (c *Container) Close(ctx context.Context) error {
	return c.container.Terminate(ctx)
}

func (c *Container) MustTempDB(ctx context.Context) (string, func()) {
	c.mu.Lock()
	defer c.mu.Unlock()

	conn, err := pgx.Connect(ctx, c.ConnectionString(ctx, pgDatabase))
	if err != nil {
		panic("failed to connect to postgres container: " + err.Error())
	}

	dbName := "subzerodbtest" + strconv.FormatUint(atomic.AddUint64(&c.seed, 1), 10)
	stmt := `CREATE DATABASE ` + dbName + ` TEMPLATE ` + pgDatabase
	_, err = conn.Exec(ctx, stmt)
	if err != nil {
		panic("failed to create test database: " + err.Error())
	}
	conn.Close(ctx)

	return c.ConnectionString(ctx, dbName), func() {}
}
