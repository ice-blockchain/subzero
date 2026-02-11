// SPDX-License-Identifier: ice License 1.0

package nip11

import (
	"context"
	"crypto/tls"
	"net/http"
	"os"
	"slices"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nbd-wtf/go-nostr/nip11"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/appcontext"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/cert"
	wsserver "github.com/ice-blockchain/subzero/server/ws"
	"github.com/ice-blockchain/subzero/server/ws/fixture"
	"github.com/ice-blockchain/subzero/storage"
)

const (
	minLeadingZeroBits = 5
	testRelayURL       = "wss://localhost:9996"
)

var (
	pubsubServer    *fixture.MockService
	privKey, pubKey = model.GenerateKeyPair()
)

func TestMain(m *testing.M) {
	serverCtx, serverCancel := context.WithTimeout(context.Background(), 10*time.Minute)
	serverCtx, _ = appcontext.NewAppContext(serverCtx)
	addr, release := query.NewTestDatabase(serverCtx)
	query.MustInit(serverCtx, query.WithConfig(&query.Config{
		WriteURLs:       []string{addr},
		RunDDL:          true,
		DisableSelfTest: true,
	}))

	initServer(serverCtx, 9996)
	code := m.Run()
	serverCancel()
	release()
	os.Exit(code)
}

func initServer(serverCtx context.Context, port uint16) {
	pubsubServer = fixture.NewTestServer(serverCtx, &wsserver.Config{
		TLSConfig:    cert.MustGenerateTLSConfigSelfSigned("localhost"),
		BindingPorts: []uint16{port},
	}, nil, NewNIP11Handler(serverCtx, &Config{
		MinLeadingZeroBits: minLeadingZeroBits,
		PrivateKey:         privKey,
	}, os.TempDir(), os.TempDir()), map[string]gin.HandlerFunc{})
	time.Sleep(100 * time.Millisecond)
}

func helperNewHandler(t testing.TB) *nip11handler {
	t.Helper()

	handler := &nip11handler{
		cfg:                  &Config{MinLeadingZeroBits: minLeadingZeroBits},
		systemMetrics:        new(atomic.Pointer[SystemMetrics]),
		systemStatus:         new(atomic.Pointer[SystemStatus]),
		databaseReportGetter: query.GetStatusReport,
		storageClient:        storage.Client(),
	}

	handler.systemMetrics.Store(&SystemMetrics{})
	handler.systemStatus.Store(&SystemStatus{
		EventsRead:  SystemStatusStateOK,
		EventsWrite: SystemStatusStateOK,
		DVM:         SystemStatusStateOK,
		FilesRead:   SystemStatusStateOK,
		FilesWrite:  SystemStatusStateOK,
		PushesSend:  SystemStatusStateOK,
	})

	return handler
}

func TestNIP11(t *testing.T) {
	t.Parallel()

	handler := helperNewHandler(t)
	expected := handler.info(t.Context())
	require.NotNil(t, expected)

	t.Run("Fetch via standard nip11 fetcher", func(t *testing.T) {
		defClient := http.DefaultClient
		defer func() {
			http.DefaultClient = defClient
		}()

		http.DefaultClient.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
		info, err := nip11.Fetch(t.Context(), "wss://localhost:9996")
		require.NoError(t, err)
		require.NotNil(t, info)
		require.Equal(t, "subzero", info.Name)
		require.Equal(t, pubKey, info.PubKey)
		require.Equal(t, testRelayURL, info.URL)

		require.Zero(t, slices.CompareFunc(info.SupportedNIPs, expected.RelayInformationDocument.SupportedNIPs, func(a, b any) int {
			require.EqualValues(t, a, b)
			return 0
		}))
	})
	t.Run("Fetch via custom fetcher", func(t *testing.T) {
		ctx := appcontext.TestContext(t)
		fetcher := NewFetcher(ctx, WithInsecureFetch())
		require.NotNil(t, fetcher)

		data, err := fetcher.Fetch(ctx, "wss://localhost:9996")
		require.NoError(t, err)
		require.NotNil(t, data)

		require.Equal(t, "subzero", data.Name)
		require.Equal(t, pubKey, data.PubKey)
		require.Equal(t, testRelayURL, data.URL)
		require.NotEmpty(t, data.SystemStatus)
		require.Equal(t, SystemStatusStateOK, data.SystemStatus.EventsRead)
		require.Equal(t, SystemStatusStateOK, data.SystemStatus.EventsWrite)

		require.Len(t, data.FCMAndroidConfigs, len(expected.FCMAndroidConfigs))
		require.Len(t, data.FCMIOSConfigs, len(expected.FCMIOSConfigs))
		require.Len(t, data.FCMWebConfigs, len(expected.FCMWebConfigs))
		require.NotNil(t, data.SystemMetrics)
	})
}

func TestFCMConfigParsing(t *testing.T) {
	t.Parallel()

	androidConfig := `{"apiKey":"android-key","appId":"android-app-id","messagingSenderId":"android-messaging-sender","projectId":"android-project"}`
	iosConfig := `{"apiKey":"ios-key","appId":"ios-app-id","messagingSenderId":"ios-messaging-sender","projectId":"ios-project"}`
	webConfig := `{"apiKey":"web-key","appId":"web-app-id","messagingSenderId":"web-messaging-sender","projectId":"web-project"}`

	handler := helperNewHandler(t)
	handler.cfg = &Config{
		MinLeadingZeroBits: minLeadingZeroBits,
		FCMAndroidConfigs:  []string{androidConfig},
		FCMIOSConfigs:      []string{iosConfig},
		FCMWebConfigs:      []string{webConfig},
	}
	info := handler.info(t.Context())

	require.Len(t, info.FCMAndroidConfigs, 1)
	androidCfg := info.FCMAndroidConfigs[0]
	require.Equal(t, "android-key", androidCfg.ApiKey)
	require.Equal(t, "android-app-id", androidCfg.AppID)
	require.Equal(t, "android-messaging-sender", androidCfg.MessagingSenderID)
	require.Equal(t, "android-project", androidCfg.ProjectID)

	require.Len(t, info.FCMIOSConfigs, 1)
	iosCfg := info.FCMIOSConfigs[0]
	require.Equal(t, "ios-key", iosCfg.ApiKey)
	require.Equal(t, "ios-app-id", iosCfg.AppID)
	require.Equal(t, "ios-messaging-sender", iosCfg.MessagingSenderID)
	require.Equal(t, "ios-project", iosCfg.ProjectID)

	require.Len(t, info.FCMWebConfigs, 1)
	webCfg := info.FCMWebConfigs[0]
	require.Equal(t, "web-key", webCfg.ApiKey)
	require.Equal(t, "web-app-id", webCfg.AppID)
	require.Equal(t, "web-messaging-sender", webCfg.MessagingSenderID)
	require.Equal(t, "web-project", webCfg.ProjectID)

	handlerWithInvalidJSON := helperNewHandler(t)
	handlerWithInvalidJSON.cfg = &Config{
		MinLeadingZeroBits: minLeadingZeroBits,
		FCMAndroidConfigs:  []string{`invalid json`},
	}
	infoWithInvalidJSON := handlerWithInvalidJSON.info(t.Context())
	require.Empty(t, infoWithInvalidJSON.FCMAndroidConfigs)
}

func TestStorageStatusCheck(t *testing.T) {
	t.Parallel()

	ctx := appcontext.TestContext(t)
	storageClient := storage.NewClient(ctx, nil, storage.WithConfig(&storage.Config{
		PrivateKey:              model.GeneratePrivateKey(),
		AbsoluteRootStoragePath: t.TempDir(),
		RelayURL:                testRelayURL,
		IONLibertyDisabled:      true,
		ExternalADNLPort:        12345,
		IONStorageConfigURL:     "https://ton.org/testnet-global.config.json",
	}))
	require.NotNil(t, storageClient)

	h := helperNewHandler(t)
	h.storageClient = storageClient

	var report SystemStatus
	h.RunStorageStatusCheck(ctx, &report)
	require.Equal(t, SystemStatusStateOK, report.FilesRead)
	require.Equal(t, SystemStatusStateOK, report.FilesWrite)

	internalReport := storageClient.Health()
	require.False(t, internalReport.InReadErrorState)
	require.False(t, internalReport.InWriteErrorState)

	storageClient.Close()
}

func TestSystemStatusCollectorDatabase(t *testing.T) {
	t.Parallel()

	t.Run("System status collector sets correct statuses", func(t *testing.T) {
		var cases = []struct {
			Name string
			query.Status
		}{
			{
				Name: "Healthy database",
				Status: query.Status{
					LastWrite: time.Now(),
					LastRead:  time.Now(),
				},
			},
			{
				Name: "Broken reads",
				Status: query.Status{
					LastWrite:        time.Now(),
					LastRead:         time.Now(),
					InReadErrorState: true,
				},
			},
			{
				Name: "Broken writes",
				Status: query.Status{
					LastWrite:         time.Now(),
					LastRead:          time.Now(),
					InWriteErrorState: true,
				},
			},
		}
		synctest.Test(t, func(t *testing.T) {
			for _, tc := range cases {
				t.Logf("Running case: %s", tc.Name) // t.Run() is not available inside synctest.Test.
				handler := helperNewHandler(t)
				require.NotNil(t, handler)

				handler.storageClient = nil // Disable storage checks.
				handler.databaseReportGetter = func(context.Context) (*query.Status, error) {
					return &tc.Status, nil
				}

				ticker := make(chan struct{}, 1)

				workerCtx, workerCancel := context.WithCancel(appcontext.TestContext(t))
				go handler.startSystemStatusCollector(workerCtx, ticker)

				ticker <- struct{}{}
				synctest.Wait()

				data := handler.systemStatus.Load()
				require.NotNil(t, data)

				if tc.InReadErrorState {
					require.Equal(t, SystemStatusStateError, data.EventsRead)
				} else {
					require.Equal(t, SystemStatusStateOK, data.EventsRead)
				}

				if tc.InWriteErrorState {
					require.Equal(t, SystemStatusStateError, data.EventsWrite)
				} else {
					require.Equal(t, SystemStatusStateOK, data.EventsWrite)
				}

				if tc.InReadErrorState || tc.InWriteErrorState {
					require.Equal(t, SystemStatusStateError, data.DVM)
				} else {
					require.Equal(t, SystemStatusStateOK, data.DVM)
				}
				workerCancel()
			}
		})
	})
	t.Run("Forced check scheduling", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			handler := helperNewHandler(t)
			handler.storageClient = nil // Disable storage checks.
			require.NotNil(t, handler)

			handler.databaseReportGetter = func(context.Context) (*query.Status, error) {
				return &query.Status{
					LastWrite:         time.Now().Add(-2 * forceDatabaseCheckInterval),
					LastRead:          time.Now().Add(-2 * forceDatabaseCheckInterval),
					InReadErrorState:  true,
					InWriteErrorState: true,
				}, nil
			}

			ticker := make(chan struct{}, 1)

			workerCtx, workerCancel := context.WithCancel(appcontext.TestContext(t))
			go handler.startSystemStatusCollector(workerCtx, ticker)

			ticker <- struct{}{}
			time.Sleep(forceDatabaseCheckInterval + time.Second) // Ensure that next forced check would be due.
			synctest.Wait()

			events := helperSelectEvents(t, model.Filter{Kinds: []int{9998}})
			require.Len(t, events, 3) // Manual check should have created 3 test events.

			// Force check should fix the status.
			data := handler.systemStatus.Load()
			require.NotNil(t, data)
			require.Equal(t, SystemStatusStateOK, data.EventsRead)
			require.Equal(t, SystemStatusStateOK, data.EventsWrite)
			require.Equal(t, SystemStatusStateOK, data.DVM)

			workerCancel()
		})
	})
}

func helperSelectEvents(t *testing.T, filters ...model.Filter) (events []*model.Event) {
	t.Helper()

	t.Logf("selecting events: %s", model.Filters(filters).String())

	for ev, err := range query.GetStoredEvents(t.Context(), filters...) {
		require.NoError(t, err)
		require.NotNil(t, ev)
		events = append(events, ev)
	}

	return events
}
