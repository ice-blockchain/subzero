// // SPDX-License-Identifier: ice License 1.0

package query

import (
	"context"
	_ "embed"

	postgres "github.com/ice-blockchain/subzero/database/query/internal/postgres"
)

var (
	//go:embed DDL_postgres.sql
	ddlPostgres string
)

type (
	Config     = postgres.Config
	StorageCfg = postgres.StorageCfg
)

func openPostgresDatabase(cfg *Config, _ bool) *dbClient {
	return &dbClient{
		dbPostgres: postgres.MustConnect(context.Background(), cfg, ddlPostgres),
	}
}

func (c *dbClient) Close() error {
	return c.dbPostgres.Close()
}
