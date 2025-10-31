// SPDX-License-Identifier: ice License 1.0

//go:build test

package opentelemetry

import (
	"context"
	"crypto/tls"
	"fmt"
	"math/rand/v2"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/appcontext"
)

var (
	otServer   OpenTelemetry
	clientTLS  *tls.Config
	otServices = []string{"openobserve-1", "openobserve-2", "openobserve-3"}
)

const (
	multiThreadRoutines = 100
	targetIngestingRate = 36 * 1024 * 1024 * 1024
)

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	serverCtx, serverCancel := appcontext.NewAppContext(ctx)
	defer cancel()
	defer serverCancel()
	var err error
	clientTLS = ClientTLS()
	otServer, err = NewOTServer(serverCtx, "cm9vdEBleGFtcGxlLmNvbTpwYXNz")
	if err != nil {
		panic(errors.Wrap(err, "failed to init open telemetry server"))
	}
	defer func() {
		err = otServer.Stop(serverCtx, true)
		if err != nil {
			fmt.Println(errors.Wrap(err, "failed to stop open telemetry server"))
		}
		os.RemoveAll("./subzero-tmp-bkp-logfile")
		os.RemoveAll("./subzero-tmp-bkp-tracefile")
	}()
	exitCode := m.Run()
	os.Exit(exitCode)
}

func TestLoggingBeforeInitAndAfterShutDown(t *testing.T) {
	start := time.Now()
	msgs := []string{}
	msgs = append(msgs,
		"before init",
		"normal message after init",
		"after otel server went down",
		"after otel server went up",
		"after logger shutdown",
		"after logger restart",
	)
	globalLogger.Info(t.Context(), msgs[0])
	MustInit(appcontext.TestContext(t), WithTLS(clientTLS))
	time.Sleep(30 * time.Second)
	globalLogger.Debug(t.Context(), msgs[1])
	require.NoError(t, globalTelemetry.redundantLogExporter.ForceFlush(t.Context()))
	require.NoError(t, otServer.Stop(t.Context(), false))
	globalLogger.Warn(context.Background(), msgs[2])
	require.NoError(t, otServer.Start(t.Context()))
	globalLogger.Trace(t.Context(), msgs[3])
	end := time.Now()
	MustShutdown(t.Context())
	globalLogger.Error(t.Context(), errors.New(msgs[4]))
	MustInit(appcontext.TestContext(t), WithTLS(clientTLS)) // restart and log to drain buffer after shutdown
	globalLogger.Info(t.Context(), msgs[5])
	require.NoError(t, globalTelemetry.redundantLogExporter.ForceFlush(t.Context()))
	logs, err := otServer.GetLogs(t.Context(), Query{
		Sql:       "SELECT * FROM subzero",
		StartTime: start.Add(-1 * time.Second).UnixMicro(),
		EndTime:   end.Add(1 * time.Second).UnixMicro(),
		From:      0,
		Size:      100,
	})
	require.NoError(t, err)
	require.Greater(t, len(logs), len(msgs))
	for msgIdx, msg := range msgs {
		require.Contains(t, logs, msg, msgIdx)
	}
}

func TestOpenTelemetryStressful(t *testing.T) {
	start := time.Now()
	timeoutCtx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()
	go helperBringOTServerDownAndUp(t, timeoutCtx)
	var wg sync.WaitGroup
	MustInit(appcontext.TestContext(t), WithTLS(clientTLS))
	producedLogs := make([]chan string, 0, multiThreadRoutines)
	for r := 0; r < multiThreadRoutines; r++ {
		iterations := targetIngestingRate / (multiThreadRoutines * len(uuid.NewString()))
		producedLogs = append(producedLogs, make(chan string, 1000))
		wg.Go(func() {
			for i := 0; i < iterations && timeoutCtx.Err() == nil; i++ {
				logMsg := uuid.NewString()
				func() {
					globalLogger.Info(t.Context(), logMsg)
					producedLogs[r] <- logMsg
					time.Sleep(time.Duration(rand.N[int](100)) * time.Millisecond)
				}()
			}
		})
	}
	go func() {
		wg.Wait()
		for i := range producedLogs {
			close(producedLogs[i])
		}
	}()
	produced := xsync.NewMap[string, struct{}]()
	for i := range producedLogs {
		go func() {
			c := 0
			for msg := range producedLogs[i] {
				produced.Store(msg, struct{}{})
				c += 1
			}
			fmt.Println("idx", i, c)
		}()
	}
	<-timeoutCtx.Done()
	require.NoError(t, otServer.Start(t.Context(), otServices...))
	hasMore := true
	limit := 500
	logsCollectedCount := uint64(0)
	MustShutdown(t.Context())
	for offset := 0; hasMore && produced.Size() > 0; offset += (limit + 1) {
		logs, err := otServer.GetLogs(t.Context(), Query{
			Sql:       "SELECT * FROM subzero",
			StartTime: start.UnixMicro(),
			EndTime:   time.Now().UnixMicro(),
			From:      offset,
			Size:      (limit + 1),
		})
		require.NoError(t, err)
		hasMore = len(logs) >= limit+1
		logsCollectedCount += uint64(len(logs))
		for _, log := range logs {
			produced.Delete(log)
		}
	}
	require.Greater(t, logsCollectedCount, uint64(0))
	require.Zero(t, produced.Size(), logsCollectedCount)
}

func helperBringOTServerDownAndUp(t *testing.T, ctx context.Context) {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	iteration := 0
	for ctx.Err() == nil {
		select {
		case <-ticker.C:
			rand.Shuffle(len(otServices), func(i, j int) { otServices[i], otServices[j] = otServices[j], otServices[i] })
			n := rand.N(len(otServices))
			if iteration == 2 {
				n = 3 // Make sure we bring all nodes down at some point
			}
			for _, s := range otServices[:n] {
				go func() {
					startStopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					if r, _ := otServer.IsRunning(startStopCtx, s); r {
						require.NoError(t, otServer.Stop(startStopCtx, false, s))
					} else {
						require.NoError(t, otServer.Start(startStopCtx, s))
					}
					cancel()
				}()
			}
			iteration++

		case <-ctx.Done():
			return
		}
	}
}
