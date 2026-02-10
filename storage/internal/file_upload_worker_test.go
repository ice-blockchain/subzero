// SPDX-License-Identifier: ice License 1.0

package internal

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/rq"
)

var (
	testRelayURL   = "http://localhost:8080"
	testPrivateKey = model.GeneratePrivateKey()
)

type (
	mockedCDNClient struct {
		T        testing.TB
		RootPath string
		Data     chan []byte
	}
)

func (m *mockedCDNClient) FileDelete(ctx context.Context, name string) error {
	m.T.Logf("Mocked delete file: %s", name)
	return nil
}

func (m *mockedCDNClient) FileUploadAsync(ctx context.Context, filePath, contentType, fileName string) error {
	m.T.Logf("Mocked async upload file: %s", fileName)
	return nil
}

func (*mockedCDNClient) HealthCheck(context.Context) error {
	return nil
}

func (m *mockedCDNClient) FileUpload(ctx context.Context, r io.Reader, contentType, fileName string) error {
	m.T.Logf("Mocked upload file: %s", fileName)
	data, err := io.ReadAll(r)
	require.NoError(m.T, err)

	select {
	case m.Data <- data:

	case <-time.After(time.Second * 5):
		m.T.Error("failed to send data, timeout")

	case <-ctx.Done():
		m.T.Error("failed to send data, context done")
	}

	return nil
}

func helperNewCDNClient(t testing.TB, rqClient rq.Client) (m mockedCDNClient) {
	t.Helper()

	m.T = t
	m.RootPath = t.TempDir()
	m.Data = make(chan []byte, 1)
	rq.RegisterWorker(rqClient.Register(), &cdnUploadWorker{
		Client:   &m,
		RootPath: m.RootPath,
		RelayURL: testRelayURL,
	})

	return m
}

func TestUploadWorker(t *testing.T) {
	t.Parallel()

	addr, release := query.NewTestDatabase(t.Context())
	defer release()

	dbConf := query.Config{
		PrivateKey: testPrivateKey,
		RelayURL:   testRelayURL,
		WriteURLs:  []string{addr},
	}

	rqClient := rq.MustNewClient(t.Context(), rq.WithConfig(&rq.Config{
		Config: dbConf,
	}))
	cdnClient := helperNewCDNClient(t, rqClient)
	require.NotNil(t, cdnClient)

	var (
		testFile    = "test.png"
		testContent = []byte("test content")
	)

	require.NoError(t, rqClient.Start(t.Context()))
	require.NoError(t, os.WriteFile(filepath.Join(cdnClient.RootPath, testFile), testContent, 0o644))

	err := rqClient.Push(t.Context(), &cdnUploadWorkerArgs{
		ContentType: "image/png",
		FileName:    testFile,
		FilePath:    testFile,
		RelayURL:    testRelayURL,
	})
	require.NoError(t, err)

	select {
	case data := <-cdnClient.Data:
		t.Logf("Received uploaded data: %s", string(data))
		require.Equal(t, testContent, data)

	case <-time.After(time.Second * 5):
		t.Error("failed to receive uploaded data, timeout")

	case <-t.Context().Done():
		t.Error("failed to receive uploaded data, context done")
	}

	require.NoError(t, rqClient.Close(t.Context()))
}
