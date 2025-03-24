// SPDX-License-Identifier: ice License 1.0

package dvm

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/ice-blockchain/subzero/database/query"
)

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	addr, release := query.NewTestDatabase(ctx)
	query.MustInit(ctx, query.WithConfig(&query.Config{
		URL: addr,
	}))
	MustInit(ctx)

	code := m.Run()
	cancel()
	release()
	if code == 0 {
		if err := goleak.Find(); err != nil {
			fmt.Printf("goleak found issues: %v\n", err)
			code = 1
		}
	}
	os.Exit(code)
}
