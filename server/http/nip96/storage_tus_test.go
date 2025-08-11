// SPDX-License-Identifier: ice License 1.0

package nip96

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tusgo "github.com/bdragon300/tusgo"
	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip96"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/http/nip98"
	"github.com/ice-blockchain/subzero/storage"
)

func TestLargeFileUploader(t *testing.T) {
	now := time.Now().Unix()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	master, masterPubKey := model.GenerateKeyPair()
	user1, user1PubKey := model.GenerateKeyPair()
	t.Run("create on-behalf attestations", func(t *testing.T) {
		helperCreateAttestations(t, ctx, now, master, masterPubKey, user1PubKey)
	})
	baseURL, err := url.Parse("https://localhost:9996/xfiles/")
	require.NoError(t, err)
	var client *tusgo.Client
	t.Run("it fails without auth", func(t *testing.T) {
		client = tusgo.NewClient(http.DefaultClient, baseURL)
		var upl tusgo.Upload
		res, err := client.CreateUpload(&upl, 1024, true, map[string]string{
			"fileName": "failed-video.mp4",
		})
		require.Error(t, err)
		require.NotNil(t, res)
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
		require.Empty(t, res.Header.Get("Location"))
	})
	finalFileHash := "20492a4d0d84f8beb1767f6616229f85d44c2827b64bdbfb260ee12fa1109e0e" // 100M of zero bytes
	streams := make(chan *tusgo.UploadStream, 100)
	client.GetRequest = func(method, reqUrl string, body io.Reader, tusClient *tusgo.Client, httpClient *http.Client) (*http.Request, error) {
		req, err := http.NewRequest(method, reqUrl, body)
		if err != nil {
			return nil, err
		}
		parsedUrl, err := url.Parse(reqUrl)
		require.NoError(t, err)
		auth, err := nip98.GenerateAuthHeader(user1, method, finalFileHash, parsedUrl, masterPubKey)
		require.NoError(t, err)
		req.Header.Set("Authorization", auth)
		return req, nil
	}
	t.Run("it fails with incorrect payload", func(t *testing.T) {
		var upl tusgo.Upload
		res, err := client.CreateUpload(&upl, 1024, true, map[string]string{
			"fileName":  "failed-video.mp4",
			"mediaType": "incorrect payload",
		})
		require.Error(t, err)
		require.NotNil(t, res)
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
		require.Empty(t, res.Header.Get("Location"))
	})
	t.Run("it runs fine with proper auth, chunks uploads", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(100)
		for i := 0; i < 100; i++ {
			go func() {
				defer wg.Done()
				var upl tusgo.Upload
				res, err := client.CreateUpload(&upl, 1024*1024, true, map[string]string{
					"fileName": uuid.NewString() + ".chunk",
				})
				require.NoError(t, err)
				require.Equal(t, http.StatusCreated, res.StatusCode)
				require.NotEmpty(t, res.Header.Get("Location"))

				s := tusgo.NewUploadStream(client, &upl)
				s.ChunkSize = 1024 * 1024
				res, err = s.Sync()
				require.NoError(t, err)
				require.NotNil(t, s)
				require.Equal(t, http.StatusOK, res.StatusCode)
				require.Equal(t, "0", res.Header.Get("Upload-Offset")) // Fresh chunk
				streams <- s
				zeroReader := strings.NewReader(strings.Repeat("\x00", 1024*1024))
				written, err := io.Copy(s, zeroReader)
				require.NoError(t, err, "Written %d bytes, last response: %v", written, s.LastResponse)
				require.Equal(t, int64(1024*1024), written)
			}()
		}
		wg.Wait()
		close(streams)
	})
	var finalUpload tusgo.Upload
	var resp nip96.UploadResponse
	t.Run("concatenate uploads into final file", func(t *testing.T) {
		completedStreams := make([]*tusgo.UploadStream, 0)
		for s := range streams {
			require.Equal(t, int64(1024*1024), s.Tell())
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
		require.Equal(t, int64(100*1024*1024), finalUpload.RemoteSize)
		require.Equal(t, "video.file", finalUpload.Metadata["fileName"])
		require.Equal(t, "video with cats", finalUpload.Metadata["alt"])
		require.Equal(t, "video with cute cats", finalUpload.Metadata["caption"])
		require.Equal(t, "application/octet-stream", finalUpload.Metadata["content_type"])
		buf := new(bytes.Buffer)
		bodyLen, err := io.Copy(buf, response.Body)
		defer response.Body.Close()
		require.NoError(t, err)
		require.Greater(t, bodyLen, int64(0))
		require.NoError(t, json.Unmarshal(buf.Bytes(), &resp))
	})
	verifyFile(t, "video with cute cats", resp.Nip94Event.Tags)
	expectedPath := filepath.Join(storageRoot, masterPubKey, "20492a4d0d84f8beb1767f6616229f85d44c2827b64bdbfb260ee12fa1109e0e.file")
	require.FileExists(t, expectedPath)
	f, err := os.Open(expectedPath)
	require.NoError(t, err)
	defer f.Close()
	hashCalc := sha256.New()
	var n int64
	n, err = io.Copy(hashCalc, f)
	require.NoError(t, err)
	require.Equal(t, int64(100*1024*1024), n)
	hash := hex.EncodeToString(hashCalc.Sum(nil))
	require.Equal(t, finalFileHash, hash)
	const newStorageRoot = "./../../.test-uploads3"
	t.Run("nip-94 event is broadcasted, it causes download to other node", func(t *testing.T) {
		// Simulate another storage node where we broadcast event/bag, and it needs to download it.
		cfg.Reset("./../../../server/http/nip96/.testdata/storage-3rd-instance.yaml")
		storage.Reset()
		initStorage(ctx)
		nip94EventToSign := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindFileMetadata,
			Tags:      resp.Nip94Event.Tags.AppendUnique(model.Tag{"b", masterPubKey}),
			Content:   resp.Nip94Event.Content,
		}}
		require.NoError(t, nip94EventToSign.SignWithAlg(user1, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, query.AcceptEvents(ctx, nip94EventToSign))
		require.NoError(t, storage.AcceptEvents(ctx, nip94EventToSign))
		require.NoError(t, storage.ReplicateFileOnPeers(ctx, nip94EventToSign))

		downloadedVideoHash, err := storage.WaitForFile(ctx, newStorageRoot, filepath.Join(newStorageRoot, masterPubKey, "20492a4d0d84f8beb1767f6616229f85d44c2827b64bdbfb260ee12fa1109e0e.file"), "20492a4d0d84f8beb1767f6616229f85d44c2827b64bdbfb260ee12fa1109e0e", int64(100*1024*1024))
		require.NoError(t, err)
		require.Equal(t, "20492a4d0d84f8beb1767f6616229f85d44c2827b64bdbfb260ee12fa1109e0e", downloadedVideoHash)
	})
}
