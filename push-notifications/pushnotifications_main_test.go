// SPDX-License-Identifier: ice License 1.0

//go:build test

package pushnotifications

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/panjf2000/ants/v2"
	"go.uber.org/goleak"

	"github.com/ice-blockchain/subzero/appcontext"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/validation"
)

var (
	testGlobalAntsPool          *ants.Pool
	testGlobalDatabaseContainer *query.Container
	testGlobalDatabaseConfig    *query.Config
)

func TestMain(m *testing.M) {
	pool, err := ants.NewPool(10 * runtime.NumCPU())
	if err != nil {
		panic(fmt.Sprintf("failed to create test ants pool: %v", err))
	}
	testGlobalAntsPool = pool
	defer func() {
		testGlobalAntsPool.Release()
		if err := goleak.Find(); err != nil {
			panic(fmt.Sprintf("goleak found issues: %v\n", err))
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	ctx, _ = appcontext.NewAppContext(ctx)

	testGlobalDatabaseContainer = query.NewTestContainer(ctx)
	addr, release := testGlobalDatabaseContainer.MustTempDB(ctx)

	testGlobalDatabaseConfig = &query.Config{
		WriteURLs:       []string{addr},
		RunDDL:          true,
		DisableSelfTest: true,
	}

	query.MustInit(ctx, query.WithConfig(testGlobalDatabaseConfig))
	validation.MustInit(ctx,
		validation.WithQueryFunc(query.GetStoredEvents),
		validation.WithIONIdentityPublicKeys(func() []string { return nil }))

	code := m.Run()
	cancel()
	release()
	os.Exit(code)
}
