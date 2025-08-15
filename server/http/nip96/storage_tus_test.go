// SPDX-License-Identifier: ice License 1.0

package nip96

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"

	tusgo "github.com/bdragon300/tusgo"
	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip96"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/http/nip98"
	"github.com/ice-blockchain/subzero/storage"
)

type zr struct{}

func (zr) Read(dst []byte) (int, error) {
	for i := range dst {
		dst[i] = 0
	}
	return len(dst), nil
}

func TestLargeFileUploader(t *testing.T) {
	const numberOfChunks = 100
	const chunkSize = 1 << 20 // 1 MiB.

	master, masterPubKey := model.GenerateKeyPair()
	user1, user1PubKey := model.GenerateKeyPair()

	helperCreateAttestations(t, t.Context(), master, masterPubKey, user1PubKey)

	baseURL, err := url.Parse("https://localhost:9996/xfiles/")
	require.NoError(t, err)

	client := tusgo.NewClient(
		&http.Client{
			Transport: &http2.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		},
		baseURL,
	)
	require.NotNil(t, client)

	t.Run("Fails without auth", func(t *testing.T) {
		var upl tusgo.Upload
		res, err := client.CreateUpload(&upl, chunkSize, true, map[string]string{
			"fileName": "failed-video.mp4",
		})
		require.Error(t, err)
		require.NotNil(t, res)
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
		require.Empty(t, res.Header.Get("Location"))
	})

	const finalFileHash = "20492a4d0d84f8beb1767f6616229f85d44c2827b64bdbfb260ee12fa1109e0e" // 100M of zero bytes.
	streams := make(chan *tusgo.UploadStream, numberOfChunks)
	client.GetRequest = func(method, reqUrl string, body io.Reader, tusClient *tusgo.Client, httpClient *http.Client) (*http.Request, error) {
		req, err := http.NewRequest(method, reqUrl, body)
		require.NoError(t, err)

		parsedUrl, err := url.Parse(reqUrl)
		require.NoError(t, err)

		auth, err := nip98.GenerateAuthHeader(user1, method, finalFileHash, parsedUrl, masterPubKey)
		require.NoError(t, err)

		req.Header.Set("Authorization", auth)
		return req, nil
	}
	t.Run("Fails with incorrect payload", func(t *testing.T) {
		var upl tusgo.Upload
		res, err := client.CreateUpload(&upl, chunkSize, true, map[string]string{
			"fileName":  "failed-video.mp4",
			"mediaType": "incorrect payload",
		})
		require.Error(t, err)
		require.NotNil(t, res)
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
		require.Empty(t, res.Header.Get("Location"))
	})
	t.Run("Chunked upload", func(t *testing.T) {
		var wg sync.WaitGroup
		for range numberOfChunks {
			wg.Go(func() {
				var upl tusgo.Upload
				res, err := client.CreateUpload(&upl, chunkSize, true, map[string]string{
					"fileName": uuid.NewString() + ".chunk",
				})
				require.NoError(t, err)
				require.Equal(t, http.StatusCreated, res.StatusCode)
				require.NotEmpty(t, res.Header.Get("Location"))

				s := tusgo.NewUploadStream(client, &upl)
				s.ChunkSize = chunkSize
				res, err = s.Sync()
				require.NoError(t, err)
				require.NotNil(t, s)
				require.Equal(t, http.StatusOK, res.StatusCode)
				require.Equal(t, "0", res.Header.Get("Upload-Offset")) // Fresh chunk.

				streams <- s

				written, err := io.Copy(s, io.LimitReader(new(zr), chunkSize))
				require.NoError(t, err, "Written %d bytes, last response: %v", written, s.LastResponse)
				require.EqualValues(t, chunkSize, written)
			})
		}
		wg.Wait()
		close(streams)
	})
	var finalUpload tusgo.Upload
	var resp nip96.UploadResponse
	t.Run("Concatenate uploads into final file", func(t *testing.T) {
		var completedStreams []*tusgo.UploadStream
		for s := range streams {
			require.EqualValues(t, chunkSize, s.Tell())
			completedStreams = append(completedStreams, s)
		}
		response, err := client.ConcatenateStreams(&finalUpload, completedStreams, map[string]string{
			"fileName":     "video.file",
			"alt":          "video with cats",
			"caption":      "video with cute cats",
			"content_type": "application/octet-stream",
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, response.StatusCode)
		require.NotEmpty(t, response.Header.Get("Location"))

		_, err = client.GetUpload(&finalUpload, response.Header.Get("Location"))
		require.NoError(t, err)
		require.EqualValues(t, chunkSize*numberOfChunks, finalUpload.RemoteSize)
		require.Equal(t, "video.file", finalUpload.Metadata["fileName"])
		require.Equal(t, "video with cats", finalUpload.Metadata["alt"])
		require.Equal(t, "video with cute cats", finalUpload.Metadata["caption"])
		require.Equal(t, "application/octet-stream", finalUpload.Metadata["content_type"])

		buf := new(bytes.Buffer)
		bodyLen, err := io.Copy(buf, response.Body)
		defer response.Body.Close()
		require.NoError(t, err)
		require.NotZero(t, bodyLen)
		require.NoError(t, json.Unmarshal(buf.Bytes(), &resp))
	})
	t.Run("Verify file upload", func(t *testing.T) {
		verifyFile(t, "video with cute cats", resp.Nip94Event.Tags)
		expectedPath := filepath.Join(testMainStorageRoot, masterPubKey, finalFileHash+".file")
		require.FileExists(t, expectedPath)

		f, err := os.Open(expectedPath)
		require.NoError(t, err)
		defer f.Close()

		hashCalc := sha256.New()
		n, err := f.WriteTo(hashCalc)
		require.NoError(t, err)
		require.EqualValues(t, chunkSize*numberOfChunks, n)

		hash := hex.EncodeToString(hashCalc.Sum(nil))
		require.Equal(t, finalFileHash, hash)
	})

	t.Run("NIP-94 event is broadcasted, it causes download to other node", func(t *testing.T) {
		// Simulate another storage node where we broadcast event/bag, and it needs to download it.
		tempDir, err := os.MkdirTemp("", "test-nip96-tus")
		require.NoError(t, err)
		t.Cleanup(func() {
			if err := os.RemoveAll(tempDir); err != nil {
				t.Logf("failed to remove temp storage root %v: %v", tempDir, err)
			}
		})
		storage.Reset()
		initStorage(t.Context(), storage.WithConfig(&storage.Config{
			PrivateKey:              testPrivateKey,
			RelayURL:                "wss://localhost:9996",
			ExternalADNLPort:        12349,
			AbsoluteRootStoragePath: tempDir,
			IONStorageConfigURL:     "https://ton.org/testnet-global.config.json",
		}))
		nip94EventToSign := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindFileMetadata,
			Tags:      resp.Nip94Event.Tags.AppendUnique(model.Tag{"b", masterPubKey}),
			Content:   resp.Nip94Event.Content,
		}}
		require.NoError(t, nip94EventToSign.SignWithAlg(user1, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, query.AcceptEvents(t.Context(), nip94EventToSign))
		require.NoError(t, storage.AcceptEvents(t.Context(), nip94EventToSign))
		require.NoError(t, storage.ReplicateFileOnPeers(t.Context(), nip94EventToSign))

		downloadedVideoHash, err := storage.WaitForFile(
			t,
			t.Context(),
			tempDir,
			filepath.Join(tempDir, masterPubKey, finalFileHash+".file"),
			finalFileHash,
			chunkSize*numberOfChunks,
		)
		require.NoError(t, err)
		require.Equal(t, finalFileHash, downloadedVideoHash)
	})
}
