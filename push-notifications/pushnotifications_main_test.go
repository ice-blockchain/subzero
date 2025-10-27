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

	"github.com/ice-blockchain/subzero/cmd/subzero-ion-connect/appcontext"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/validation"
)

var (
	globalTestAntsPool *ants.Pool
)

func TestMain(m *testing.M) {
	pool, err := ants.NewPool(10 * runtime.NumCPU())
	if err != nil {
		panic(fmt.Sprintf("failed to create test ants pool: %v", err))
	}
	globalTestAntsPool = pool
	defer func() {
		globalTestAntsPool.Release()
		if err := goleak.Find(); err != nil {
			panic(fmt.Sprintf("goleak found issues: %v\n", err))
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	ctx, _ = appcontext.NewAppContext(ctx)
	addr, release := query.NewTestDatabase(ctx)
	query.MustInit(ctx, query.WithConfig(&query.Config{
		WriteURLs:       []string{addr},
		RunDDL:          true,
		DisableSelfTest: true,
	}))
	validation.MustInit(ctx, validation.WithIONIdentityPublicKeys(func() []string { return nil }))

	code := m.Run()
	cancel()
	release()
	os.Exit(code)
}
