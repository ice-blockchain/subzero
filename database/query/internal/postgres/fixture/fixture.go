// // SPDX-License-Identifier: ice License 1.0

package fixture

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	pgImage    = "postgres:17-alpine"
	pgUser     = "subzero-user"
	pgPass     = "subzero-password"
	pgDatabase = "subzerodb" // `-` is not allowed in database names.
)

type Container struct {
	address   string
	container *postgres.PostgresContainer
	seed      uint64
	mu        sync.Mutex
}

func New(ctx context.Context) *Container {
	container, err := postgres.Run(ctx, pgImage,
		postgres.WithDatabase(pgDatabase),
		postgres.WithUsername(pgUser),
		postgres.WithPassword(pgPass),
		testcontainers.WithWaitStrategyAndDeadline(time.Minute, wait.ForExposedPort()),
	)
	if err != nil {
		panic("failed to start postgres container: " + err.Error())
	}

	return &Container{
		address:   container.MustConnectionString(ctx, "sslmode=disable"),
		container: container,
		seed:      uint64(time.Now().UnixMilli()),
	}
}

func (c *Container) ConnectionString() string {
	return c.address
}

func (c *Container) Close(ctx context.Context) error {
	return c.container.Terminate(ctx)
}

func (c *Container) MustTempDB(ctx context.Context) (string, func()) {
	c.mu.Lock()
	defer c.mu.Unlock()

	conn, err := pgx.Connect(ctx, c.address)
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

	return strings.ReplaceAll(c.address, pgDatabase, dbName), func() {}
}
