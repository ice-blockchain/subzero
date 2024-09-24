package http

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	gomime "github.com/cubewise-code/go-mime"
	"github.com/gookit/goutil/errorx"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip94"
	"github.com/nbd-wtf/go-nostr/nip96"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/storage"
	storagefixture "github.com/ice-blockchain/subzero/storage/fixture"
)

//go:embed .testdata
var testdata embed.FS

func TestNIP96(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	defer func() {
		require.NoError(t, os.RemoveAll("./../../.test-uploads"))
		require.NoError(t, os.RemoveAll("./../../.test-uploads2"))
	}()
	user1 := nostr.GeneratePrivateKey()
	var tagsToBroadcast nostr.Tags
	var contentToBroadcast string
	t.Run("files are uploaded, response is ok", func(t *testing.T) {
		var responses chan *nip96.UploadResponse
		responses = make(chan *nip96.UploadResponse, 2)
		upload(t, ctx, user1, ".testdata/image2.png", "profile.png", "ice profile pic", func(resp *nip96.UploadResponse) {
			responses <- resp
		})
		upload(t, ctx, user1, ".testdata/image.jpg", "ice.jpg", "ice logo", func(resp *nip96.UploadResponse) {
			responses <- resp
		})
		close(responses)
		var latest uint64
		for resp := range responses {
			var createdAt uint64
			if createdAtTag := resp.Nip94Event.Tags.GetFirst([]string{"createdAt"}); createdAtTag != nil && len(*createdAtTag) > 1 {
				var err error
				createdAt, err = strconv.ParseUint(createdAtTag.Value(), 10, 64)
				require.NoError(t, err)
				if createdAt > latest {
					latest = createdAt
					tagsToBroadcast = resp.Nip94Event.Tags
					contentToBroadcast = resp.Nip94Event.Content
				}
			}
			verifyFile(t, resp.Nip94Event.Content, resp.Nip94Event.Tags)
		}
	})
	const newStorageRoot = "./../../.test-uploads2"
	var nip94EventToSign *model.Event
	t.Run("nip-94 event is broadcasted, it causes download to other node", func(t *testing.T) {
		nip94EventToSign = &model.Event{nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindFileMetadata,
			Tags:      tagsToBroadcast,
			Content:   contentToBroadcast,
		}}
		require.NoError(t, nip94EventToSign.Sign(user1))
		// Simulate another storage node where we broadcast event/bag, and it needs to download it.
		initStorage(ctx, newStorageRoot)
		require.NoError(t, query.AcceptEvent(ctx, nip94EventToSign))
		require.NoError(t, storage.AcceptEvent(ctx, nip94EventToSign))
		pk, err := nostr.GetPublicKey(user1)
		require.NoError(t, err)
		downloadedProfileHash, err := storagefixture.WaitForFile(ctx, newStorageRoot, filepath.Join(newStorageRoot, pk, "image/profile.png"), "b2b8cf9202b45dad7e137516bcf44b915ce30b39c3b294629a9b6b8fa1585292", int64(182744))
		require.NoError(t, err)
		require.Equal(t, "b2b8cf9202b45dad7e137516bcf44b915ce30b39c3b294629a9b6b8fa1585292", downloadedProfileHash)
		downloadedLogoHash, err := storagefixture.WaitForFile(ctx, newStorageRoot, filepath.Join(newStorageRoot, pk, "image/ice.jpg"), "777d453395088530ce8de776fe54c3e5ace548381007b743e067844858962218", int64(415939))
		require.NoError(t, err)
		require.Equal(t, "777d453395088530ce8de776fe54c3e5ace548381007b743e067844858962218", downloadedLogoHash)
	})

	t.Run("download endpoint redirects to same download url over ton storage", func(t *testing.T) {
		expected := nip94.ParseFileMetadata(nostr.Event{Tags: expectedResponse("ice logo").Nip94Event.Tags})
		status, location := download(t, ctx, user1, "777d453395088530ce8de776fe54c3e5ace548381007b743e067844858962218")
		require.Equal(t, http.StatusFound, status)
		require.Regexp(t, fmt.Sprintf("^http://[0-9a-fA-F]{64}.bag/%v", expected.Summary), location)

		expected = nip94.ParseFileMetadata(nostr.Event{Tags: expectedResponse("ice profile pic").Nip94Event.Tags})
		status, location = download(t, ctx, user1, "b2b8cf9202b45dad7e137516bcf44b915ce30b39c3b294629a9b6b8fa1585292")
		require.Equal(t, http.StatusFound, status)
		require.Regexp(t, fmt.Sprintf("^http://[0-9a-fA-F]{64}.bag/%v", expected.Summary), location)
		status, _ = download(t, ctx, user1, "non_valid_hash")
		require.Equal(t, http.StatusNotFound, status)
	})
	t.Run("list files responds with up to all files for the user when total is less than page", func(t *testing.T) {
		files := list(t, ctx, user1, 0, 0)
		assert.Equal(t, uint32(2), files.Total)
		assert.Len(t, files.Files, 2)
		for _, f := range files.Files {
			verifyFile(t, f.Content, f.Tags)
		}
	})
	t.Run("list files with pagination", func(t *testing.T) {
		files := list(t, ctx, user1, 0, 1)
		assert.Equal(t, uint32(2), files.Total)
		assert.Len(t, files.Files, 1)
		uniqFiles := map[string]struct{}{}
		for _, f := range files.Files {
			verifyFile(t, f.Content, f.Tags)
			uniqFiles[f.Content] = struct{}{}
		}
		files = list(t, ctx, user1, 1, 1)
		assert.Equal(t, uint32(2), files.Total)
		assert.Len(t, files.Files, 1)
		for _, f := range files.Files {
			verifyFile(t, f.Content, f.Tags)
			_, presentedBefore := uniqFiles[f.Content]
			require.False(t, presentedBefore)
		}
	})
	t.Run("delete file", func(t *testing.T) {
		fileHash := ""
		if xTag := nip94EventToSign.Tags.GetFirst([]string{"x"}); xTag != nil && len(*xTag) > 1 {
			fileHash = xTag.Value()
		} else {
			t.Fatalf("malformed x tag in nip94 event %v", nip94EventToSign.ID)
		}
		status := deleteFile(t, ctx, user1, fileHash)
		fileName := nip94.ParseFileMetadata(nostr.Event{Tags: expectedResponse(nip94EventToSign.Content).Nip94Event.Tags}).Summary
		require.Equal(t, http.StatusOK, status)
		pk, err := nostr.GetPublicKey(user1)
		require.NoError(t, err)
		require.NoFileExists(t, filepath.Join(storageRoot, pk, fileName))
		deletionEventToSign := &model.Event{nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindDeletion,
			Tags: nostr.Tags{
				nostr.Tag{"e", nip94EventToSign.ID},
				nostr.Tag{"k", strconv.FormatInt(int64(nostr.KindFileMetadata), 10)},
			},
		}}
		require.NoError(t, deletionEventToSign.Sign(user1))
		require.NoError(t, storage.AcceptEvent(ctx, deletionEventToSign))
		require.NoFileExists(t, filepath.Join(newStorageRoot, pk, fileName))
	})

}

func verifyFile(t *testing.T, content string, tags nostr.Tags) {
	t.Helper()
	md := nip94.ParseFileMetadata(nostr.Event{Tags: tags})
	expected := nip94.ParseFileMetadata(nostr.Event{Tags: expectedResponse(content).Nip94Event.Tags})
	url := md.URL
	bagID := md.TorrentInfoHash
	expectedFileName := expected.Summary
	expected.Summary = ""
	md.URL = ""
	md.TorrentInfoHash = ""
	require.Equal(t, expected, md)
	require.Contains(t, url, fmt.Sprintf("http://%v.bag/%v", bagID, expectedFileName))
	require.Regexp(t, fmt.Sprintf("^http://[0-9a-fA-F]{64}.bag/%v", expectedFileName), url)
	require.Regexp(t, "^[0-9a-fA-F]{64}$", bagID)
}

func upload(t *testing.T, ctx context.Context, sk, path, filename, caption string, result func(resp *nip96.UploadResponse)) {
	t.Helper()
	img, _ := testdata.Open(path)
	defer img.Close()
	resp, err := nip96.Upload(ctx, nip96.UploadRequest{
		Host:        "https://localhost:9997/media",
		File:        img,
		Filename:    filename,
		Caption:     caption,
		ContentType: gomime.TypeByExtension(filepath.Ext(path)),
		SK:          sk,
		SignPayload: true,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	jResp, err := json.Marshal(resp)
	require.NoError(t, err)
	log.Println(filename, string(jResp))
	result(resp)
}

func download(t *testing.T, ctx context.Context, sk, fileHash string) (status int, locationUrl string) {
	t.Helper()
	http.DefaultClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	defer func() {
		http.DefaultClient.CheckRedirect = func(req *http.Request, via []*http.Request) error { return nil }
	}()
	resp := authorizedReq(t, ctx, sk, "GET", fmt.Sprintf("https://localhost:9997/media/%v", fileHash))
	if resp.StatusCode == http.StatusFound {
		require.Equal(t, http.StatusFound, resp.StatusCode)
		locationUrl = resp.Header.Get("location")
		require.NotEmpty(t, locationUrl)
		return resp.StatusCode, locationUrl
	}
	return resp.StatusCode, ""
}

func list(t *testing.T, ctx context.Context, sk string, page, limit uint32) *listedFiles {
	t.Helper()
	resp := authorizedReq(t, ctx, sk, "GET", fmt.Sprintf("https://localhost:9997/media?page=%v&count=%v", page, limit))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var files listedFiles
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(body, &files))
	return &files
}

func deleteFile(t *testing.T, ctx context.Context, sk string, fileHash string) int {
	t.Helper()
	resp := authorizedReq(t, ctx, sk, "DELETE", fmt.Sprintf("https://localhost:9997/media/%v", fileHash))
	if resp.StatusCode == http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		var respBody struct {
			Message string `json:"message"`
			Status  string `json:"status"`
		}
		require.NoError(t, json.Unmarshal(body, &respBody))
		require.Equal(t, "success", respBody.Status)
		require.Equal(t, "deleted", respBody.Message)
		return resp.StatusCode
	}
	return resp.StatusCode
}

func authorizedReq(t *testing.T, ctx context.Context, sk, method, url string) *http.Response {
	t.Helper()
	uploadReq, err := http.NewRequest(method, url, nil)
	require.NoError(t, err)
	auth, err := generateAuthHeader(t, sk, url, method)
	require.NoError(t, err)
	uploadReq.Header.Set("Authorization", auth)
	resp, err := http.DefaultClient.Do(uploadReq.WithContext(ctx))
	require.NoError(t, err)
	require.NotNil(t, resp)
	return resp
}

func expectedResponse(caption string) *nip96.UploadResponse {
	expectedResponses := map[string]*nip96.UploadResponse{
		"ice profile pic": &nip96.UploadResponse{
			Status:        "success",
			Message:       "Upload successful.",
			ProcessingURL: "",
			Nip94Event: struct {
				Tags    nostr.Tags `json:"tags"`
				Content string     `json:"content"`
			}{
				Tags: nostr.Tags{
					nostr.Tag{"summary", "image/profile.png"},
					nostr.Tag{"ox", "b2b8cf9202b45dad7e137516bcf44b915ce30b39c3b294629a9b6b8fa1585292"},
					nostr.Tag{"x", "b2b8cf9202b45dad7e137516bcf44b915ce30b39c3b294629a9b6b8fa1585292"},
					nostr.Tag{"m", "image/png"},
					nostr.Tag{"size", "182744"},
				},
				Content: "ice profile pic",
			},
		},
		"ice logo": &nip96.UploadResponse{
			Status:        "success",
			Message:       "Upload successful.",
			ProcessingURL: "",
			Nip94Event: struct {
				Tags    nostr.Tags `json:"tags"`
				Content string     `json:"content"`
			}{
				Tags: nostr.Tags{
					nostr.Tag{"summary", "image/ice.jpg"},
					nostr.Tag{"ox", "777d453395088530ce8de776fe54c3e5ace548381007b743e067844858962218"},
					nostr.Tag{"x", "777d453395088530ce8de776fe54c3e5ace548381007b743e067844858962218"},
					nostr.Tag{"m", "image/png"},
					nostr.Tag{"size", "415939"},
				},
				Content: "ice profile pic",
			},
		},
	}
	return expectedResponses[caption]
}

func initStorage(ctx context.Context, path string) {
	transportOverride := http.DefaultClient.Transport
	http.DefaultClient.Transport = http.DefaultTransport
	_, nodeKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		log.Panic(errorx.Wrapf(err, "failed to generate node key"))
	}
	storagePort, err := rand.Int(rand.Reader, big.NewInt(63500))
	if err != nil {
		log.Panic(errorx.Wrapf(err, "failed to generate port number"))
	}
	wd, _ := os.Getwd()
	rootStorage := filepath.Join(wd, path)
	port := int(storagePort.Int64()) + 1024
	storage.MustInit(ctx, nodeKey, storage.DefaultConfigUrl, rootStorage, net.ParseIP("127.0.0.1"), port)
	http.DefaultClient.Transport = transportOverride
}

func generateAuthHeader(t *testing.T, sk, url, method string) (string, error) {
	t.Helper()
	pk, err := nostr.GetPublicKey(sk)
	if err != nil {
		return "", fmt.Errorf("nostr.GetPublicKey: %w", err)
	}

	event := nostr.Event{
		Kind:      nostrHttpAuthKind,
		PubKey:    pk,
		CreatedAt: nostr.Now(),
		Tags: nostr.Tags{
			nostr.Tag{"u", url},
			nostr.Tag{"method", method},
		},
	}
	require.NoError(t, event.Sign(sk))

	b, err := json.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("json.Marshal: %w", err)
	}

	payload := base64.StdEncoding.EncodeToString(b)

	return fmt.Sprintf("Nostr %s", payload), nil
}
