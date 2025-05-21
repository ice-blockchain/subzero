// SPDX-License-Identifier: ice License 1.0

package nip96

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	gomime "github.com/cubewise-code/go-mime"
	"github.com/gin-gonic/gin"
	"github.com/jamiealquiza/tachymeter"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip94"
	"github.com/nbd-wtf/go-nostr/nip96"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/http/nip11"
	"github.com/ice-blockchain/subzero/server/http/nip98"
	wsserver "github.com/ice-blockchain/subzero/server/ws"
	"github.com/ice-blockchain/subzero/server/ws/fixture"
	"github.com/ice-blockchain/subzero/storage"
	storagefixture "github.com/ice-blockchain/subzero/storage/fixture"
)

const (
	minLeadingZeroBits = 5
	storageRoot        = "../../.test-uploads"
)

var (
	pubsubServer    *fixture.MockService
	privKey, pubKey = model.GenerateKeyPair()
)

func TestMain(m *testing.M) {
	serverCtx, serverCancel := context.WithTimeout(context.Background(), 10*time.Minute)

	addr, release := query.NewTestDatabase(serverCtx)
	query.MustInit(serverCtx, query.WithConfig(&query.Config{
		URL: addr,
	}))

	initServer(serverCtx, 9996)
	http.DefaultClient.Transport = &http2.Transport{TLSClientConfig: fixture.ClientTLS()}
	code := m.Run()
	serverCancel()
	release()
	os.Exit(code)
}

func initServer(serverCtx context.Context, port uint16) {
	initStorage(serverCtx)
	type globalCfg struct {
		TLSCert string `yaml:"tls-cert"`
		TLSKey  string `yaml:"tls-key"`
	}
	globalConfig := cfg.MustGet[globalCfg]()
	uploader := NewUploadHandler(serverCtx, false)
	pubsubServer = fixture.NewTestServer(serverCtx, &wsserver.Config{
		TLSConfig: wsserver.LoadTLSConfig(globalConfig.TLSCert, globalConfig.TLSKey),
		Port:      port,
	}, nil, nip11.NewNIP11Handler(serverCtx, &nip11.Config{MinLeadingZeroBits: minLeadingZeroBits, PrivateKey: privKey}, uploader.RootPath(), os.TempDir()), map[string]gin.HandlerFunc{
		"POST /files":         uploader.Upload(),
		"GET /files":          uploader.ListFiles(),
		"GET /files/:file":    uploader.Download(),
		"DELETE /files/:file": uploader.Delete(),
	})
	time.Sleep(100 * time.Millisecond)
}

//go:embed .testdata
var testdata embed.FS

func TestNIP96(t *testing.T) {
	t.Parallel()
	now := time.Now().Unix()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	defer func() {
		require.NoError(t, storage.Client().Close())
		require.NoError(t, os.RemoveAll("./../../.test-uploads"))
		require.NoError(t, os.RemoveAll("./../../.test-uploads2"))
		require.NoError(t, os.RemoveAll("db.sqlite"))
	}()
	master, masterPubKey := model.GenerateKeyPair()
	user1, user1PubKey := model.GenerateKeyPair()
	user2, user2PubKey := model.GenerateKeyPair()
	var tagsToBroadcast nostr.Tags
	var contentToBroadcast string
	var outdatedTags nostr.Tags
	var outdatedContent string
	t.Run("create on-behalf attestations", func(t *testing.T) {
		var ev model.Event
		ev.Kind = model.CustomIONKindAttestation
		ev.CreatedAt = 1
		ev.Tags = model.Tags{
			{model.TagAttestationName, user1PubKey, "", model.CustomIONAttestationKindActive + ":" + strconv.Itoa(int(now-10))},
			{model.TagAttestationName, user2PubKey, "", model.CustomIONAttestationKindActive + ":" + strconv.Itoa(int(now-5))},
		}
		require.NoError(t, ev.SignWithAlg(master, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, query.AcceptEvents(ctx, &ev))
	})

	t.Run("files are uploaded, response is ok", func(t *testing.T) {
		var responses = make([]*nip96.UploadResponse, 0)
		const filesCount = 4
		responsesCh := make(chan *nip96.UploadResponse, filesCount)
		var wg sync.WaitGroup
		wg.Add(filesCount)
		go upload(t, ctx, user1, masterPubKey, ".testdata/image2.png", "profile.png", "ice profile pic", func(resp *nip96.UploadResponse) {
			defer wg.Done()
			responsesCh <- resp
		})
		go upload(t, ctx, master, "", ".testdata/text-master.txt", "master.txt", "master's file", func(resp *nip96.UploadResponse) {
			defer wg.Done()
			responsesCh <- resp
		})
		go upload(t, ctx, user1, masterPubKey, ".testdata/image.jpg", "ice.jpg", "ice logo", func(resp *nip96.UploadResponse) {
			defer wg.Done()
			responsesCh <- resp
		})
		go upload(t, ctx, user1, masterPubKey, ".testdata/text.txt", "text.txt", "text file", func(resp *nip96.UploadResponse) {
			defer wg.Done()
			responsesCh <- resp
		})
		wg.Wait()
		close(responsesCh)
		for r := range responsesCh {
			if r.Nip94Event.Content == "ice profile pic" {
				outdatedTags = r.Nip94Event.Tags
				outdatedContent = r.Nip94Event.Content
			}
			responses = append(responses, r)
		}
		tagsToBroadcast = responses[len(responses)-1].Nip94Event.Tags
		contentToBroadcast = responses[len(responses)-1].Nip94Event.Content
		for _, resp := range responses {
			verifyFile(t, resp.Nip94Event.Content, resp.Nip94Event.Tags)
			tagsToBroadcast = resp.Nip94Event.Tags.AppendUnique(model.Tag{"b", masterPubKey}).
				AppendUnique(model.Tag{"expiration", strconv.FormatInt(time.Now().Unix()-10, 10)})
			contentToBroadcast = resp.Nip94Event.Content
			nip94EventToSign := &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindFileMetadata,
				Tags:      tagsToBroadcast,
				Content:   contentToBroadcast,
			}}
			require.NoError(t, nip94EventToSign.SignWithAlg(user1, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, query.AcceptEvents(ctx, nip94EventToSign))
			require.NoError(t, storage.AcceptEvents(ctx, nip94EventToSign))
		}
	})

	var outdatedNip94EventToSign *model.Event
	t.Run("nip-94 event is accepted on the same relay it was uploaded to = no-op", func(t *testing.T) {
		outdatedTags = outdatedTags.AppendUnique(model.Tag{"b", masterPubKey})
		outdatedNip94EventToSign = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindFileMetadata,
			Tags:      outdatedTags,
			Content:   outdatedContent,
		}}
		require.NoError(t, outdatedNip94EventToSign.SignWithAlg(user1, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, query.AcceptEvents(ctx, outdatedNip94EventToSign))
		require.NoError(t, storage.AcceptEvents(ctx, outdatedNip94EventToSign))
		time.Sleep(3 * time.Second)
	})
	const newStorageRoot = "./../../.test-uploads2"
	var nip94EventToSign *model.Event
	t.Run("nip-94 event is broadcasted, it causes download to other node", func(t *testing.T) {
		tagsToBroadcast = tagsToBroadcast.AppendUnique(model.Tag{"b", masterPubKey})
		nip94EventToSign = &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindFileMetadata,
			Tags:      tagsToBroadcast,
			Content:   contentToBroadcast,
		}}
		require.NoError(t, nip94EventToSign.SignWithAlg(user1, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		// Simulate another storage node where we broadcast event/bag, and it needs to download it.
		cfg.MustInit("./../../../server/http/nip96/.testdata/storage-2nd-instance.yaml")
		initStorage(ctx)
		require.NoError(t, query.AcceptEvents(ctx, nip94EventToSign))
		require.NoError(t, storage.AcceptEvents(ctx, nip94EventToSign))
		downloadedProfileHash, err := storagefixture.WaitForFile(ctx, newStorageRoot, filepath.Join(newStorageRoot, masterPubKey, "profile.png"), "b2b8cf9202b45dad7e137516bcf44b915ce30b39c3b294629a9b6b8fa1585292", int64(182744))
		require.NoError(t, err)
		require.Equal(t, "b2b8cf9202b45dad7e137516bcf44b915ce30b39c3b294629a9b6b8fa1585292", downloadedProfileHash)
		downloadedLogoHash, err := storagefixture.WaitForFile(ctx, newStorageRoot, filepath.Join(newStorageRoot, masterPubKey, "ice.jpg"), "777d453395088530ce8de776fe54c3e5ace548381007b743e067844858962218", int64(415939))
		require.NoError(t, err)
		require.Equal(t, "777d453395088530ce8de776fe54c3e5ace548381007b743e067844858962218", downloadedLogoHash)
	})

	t.Run("download endpoint redirects to same download url over ton storage", func(t *testing.T) {
		expected := nip94.ParseFileMetadata(nostr.Event{Tags: expectedResponse("ice logo").Nip94Event.Tags})
		status, location := download(t, ctx, user1, "777d453395088530ce8de776fe54c3e5ace548381007b743e067844858962218", masterPubKey)
		require.Equal(t, http.StatusFound, status)
		require.Regexp(t, fmt.Sprintf("^http://[0-9a-fA-F]{64}.bag/%v", expected.Summary), location)

		expected = nip94.ParseFileMetadata(nostr.Event{Tags: expectedResponse("ice profile pic").Nip94Event.Tags})
		status, location = download(t, ctx, user1, "b2b8cf9202b45dad7e137516bcf44b915ce30b39c3b294629a9b6b8fa1585292", masterPubKey)
		require.Equal(t, http.StatusFound, status)
		require.Regexp(t, fmt.Sprintf("^http://[0-9a-fA-F]{64}.bag/%v", expected.Summary), location)
		status, _ = download(t, ctx, user1, "non_valid_hash")
		require.Equal(t, http.StatusNotFound, status)
	})
	t.Run("list files responds with up to all files for the user when total is less than page", func(t *testing.T) {
		files := list(t, ctx, user1, 0, 0, masterPubKey)
		assert.Equal(t, uint32(4), files.Total)
		assert.Len(t, files.Files, 4)
		for _, f := range files.Files {
			verifyFile(t, f.Content, f.Tags)
		}
	})
	t.Run("list files with pagination", func(t *testing.T) {
		files := list(t, ctx, user1, 0, 1, masterPubKey)
		assert.Equal(t, uint32(4), files.Total)
		assert.Len(t, files.Files, 1)
		uniqFiles := map[string]struct{}{}
		for _, f := range files.Files {
			verifyFile(t, f.Content, f.Tags)
			uniqFiles[f.Content] = struct{}{}
		}
		files = list(t, ctx, user1, 1, 1, masterPubKey)
		assert.Equal(t, uint32(4), files.Total)
		assert.Len(t, files.Files, 1)
		for _, f := range files.Files {
			verifyFile(t, f.Content, f.Tags)
			_, presentedBefore := uniqFiles[f.Content]
			require.False(t, presentedBefore)
		}
	})
	t.Run("delete file owned by user 1 on behave of usr 1 (normally)", func(t *testing.T) {
		fileHash := ""
		if xTag := outdatedNip94EventToSign.Tags.GetFirst([]string{"x"}); xTag != nil && len(*xTag) > 1 {
			fileHash = xTag.Value()
		} else {
			t.Fatalf("malformed x tag in nip94 event %v", nip94EventToSign.ID)
		}
		status := deleteFile(t, ctx, user1, fileHash, masterPubKey)
		require.Equal(t, http.StatusOK, status)
		fileName := nip94.ParseFileMetadata(nostr.Event{Tags: expectedResponse(outdatedNip94EventToSign.Content).Nip94Event.Tags}).Summary
		require.NoFileExists(t, filepath.Join(storageRoot, masterPubKey, fileName))
		deletionEventToSign := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindDeletion,
			Tags: nostr.Tags{
				nostr.Tag{"e", outdatedNip94EventToSign.ID},
				nostr.Tag{"k", strconv.FormatInt(int64(nostr.KindFileMetadata), 10)},
				nostr.Tag{"b", masterPubKey},
			},
		}}
		require.NoError(t, deletionEventToSign.SignWithAlg(user1, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, storage.AcceptEvents(ctx, deletionEventToSign))
		require.NoFileExists(t, filepath.Join(newStorageRoot, masterPubKey, fileName))
	})
	t.Run("delete file owned by user 1 on behave of usr 2 (attestation) via deletion of imeta tagged post", func(t *testing.T) {
		status := deleteFile(t, ctx, user2, "982d9e3eb996f559e633f4d194def3761d909f5a3b647d1a851fead67c32c9d1", masterPubKey)
		require.Equal(t, http.StatusOK, status)
		fileName := "text.txt"
		require.NoFileExists(t, filepath.Join(storageRoot, masterPubKey, fileName))
		imetaEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindTextNote,
			Tags: nostr.Tags{
				nostr.Tag{
					"imeta",
					"x 982d9e3eb996f559e633f4d194def3761d909f5a3b647d1a851fead67c32c9d1",
					"ox 982d9e3eb996f559e633f4d194def3761d909f5a3b647d1a851fead67c32c9d1",
					fmt.Sprintf("url %v", nip94EventToSign.Tags.GetFirst([]string{"url"}).Value()),
				},
				nostr.Tag{"k", strconv.FormatInt(int64(nostr.KindTextNote), 10)},
				nostr.Tag{"b", masterPubKey},
			},
		}}
		require.NoError(t, imetaEvent.SignWithAlg(user2, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, query.AcceptEvents(ctx, imetaEvent))
		deletionEventToSign := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindDeletion,
			Tags: nostr.Tags{
				nostr.Tag{"e", imetaEvent.ID},
				nostr.Tag{"k", strconv.FormatInt(int64(nostr.KindTextNote), 10)},
				nostr.Tag{"b", masterPubKey},
			},
		}}
		require.NoError(t, deletionEventToSign.SignWithAlg(user2, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, storage.AcceptEvents(ctx, deletionEventToSign))
		require.NoFileExists(t, filepath.Join(newStorageRoot, masterPubKey, fileName))
	})
	t.Run("delete file owned by master by usr1 (Forbidden)", func(t *testing.T) {
		status := deleteFile(t, ctx, user1, "fc613b4dfd6736a7bd268c8a0e74ed0d1c04a959f59dd74ef2874983fd443fc9", masterPubKey)
		require.Equal(t, http.StatusForbidden, status)
		fileName := "master.txt"
		require.FileExists(t, filepath.Join(storageRoot, masterPubKey, fileName))
	})
	ch := make(chan struct{}, 100)
	query.RegisterExpiredEventsProcessor(func(ctx context.Context, events ...*model.Event) error {
		err := storage.DeleteExpiredFiles(ctx, events...)
		ch <- struct{}{}
		return err
	})
	require.NoError(t, query.TriggerExpiredEventsCleanup(ctx))
	select {
	case <-ch:
	case <-time.After(30 * time.Second):
		t.Fatal("Expired events processor was not triggered")
	}
	require.NoFileExists(t, filepath.Join(newStorageRoot, masterPubKey, "master.txt"), "expiration")
	t.Run("profile removal - causes whole storage removal for that user", func(t *testing.T) {
		deletionEventToSign := &model.Event{
			Event: nostr.Event{
				ID:      "deletion event4",
				PubKey:  masterPubKey,
				Kind:    nostr.KindDeletion,
				Content: "profile deletion",
			}}
		require.NoError(t, deletionEventToSign.SignWithAlg(master, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, storage.AcceptEvents(ctx, deletionEventToSign))
		require.NoDirExists(t, filepath.Join(newStorageRoot, masterPubKey))
	})
}

func verifyFile(t *testing.T, content string, tags nostr.Tags) {
	t.Helper()
	md := nip94.ParseFileMetadata(nostr.Event{Tags: tags})
	expected := nip94.ParseFileMetadata(nostr.Event{Tags: expectedResponse(content).Nip94Event.Tags})
	url := md.URL
	bagID := md.TorrentInfoHash
	if strings.Contains(bagID, ":") {
		bagID = strings.Split(bagID, ":")[0]
	}
	expectedFileName := expected.Summary
	expected.Summary = ""
	md.URL = ""
	md.TorrentInfoHash = ""
	require.Equal(t, expected, md)
	require.Contains(t, url, fmt.Sprintf("http://%v.bag/%v", bagID, expectedFileName))
	require.Regexp(t, fmt.Sprintf("^http://[0-9a-fA-F]{64}.bag/%v", expectedFileName), url)
	require.Regexp(t, "^[0-9a-fA-F]{64}$", bagID)
}

func upload(t *testing.T, ctx context.Context, sk, master, path, filename, caption string, result func(resp *nip96.UploadResponse)) {
	t.Helper()
	img, _ := testdata.Open(path)
	defer img.Close()
	var requestBody bytes.Buffer
	fileHash := sha256.New()
	writer := multipart.NewWriter(&requestBody)
	fileWriter, err := writer.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = io.Copy(fileWriter, io.TeeReader(img, fileHash))
	require.NoError(t, err)
	require.NoError(t, writer.WriteField("caption", caption))
	require.NoError(t, writer.WriteField("content_type", gomime.TypeByExtension(filepath.Ext(path))))
	require.NoError(t, writer.WriteField("no_transform", "true"))
	err = writer.Close()
	require.NoError(t, err)
	httpResp := authorizedReq(t, ctx, sk, "POST", "https://localhost:9996/files", hex.EncodeToString(fileHash.Sum(nil)), writer.FormDataContentType(), &requestBody, master)
	require.NotNil(t, httpResp)
	switch httpResp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted:
		var resp nip96.UploadResponse
		err = json.NewDecoder(httpResp.Body).Decode(&resp)
		require.NoError(t, err)
		result(&resp)
	default:
		t.Fatalf("unexpected http status code %v for upload %v by %v", httpResp.StatusCode, filename, sk)
	}
}

func download(t *testing.T, ctx context.Context, sk, fileHash string, masterPubkey ...string) (status int, locationUrl string) {
	t.Helper()
	http.DefaultClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	defer func() {
		http.DefaultClient.CheckRedirect = func(req *http.Request, via []*http.Request) error { return nil }
	}()
	resp := authorizedReq(t, ctx, sk, "GET", fmt.Sprintf("https://localhost:9996/files/%v", fileHash), "", "", nil, masterPubkey...)
	if resp.StatusCode == http.StatusFound {
		require.Equal(t, http.StatusFound, resp.StatusCode)
		locationUrl = resp.Header.Get("location")
		require.NotEmpty(t, locationUrl)
		return resp.StatusCode, locationUrl
	}
	return resp.StatusCode, ""
}

func list(t *testing.T, ctx context.Context, sk string, page, limit uint32, masterPubkey ...string) *listedFiles {
	t.Helper()
	resp := authorizedReq(t, ctx, sk, "GET", fmt.Sprintf("https://localhost:9996/files?page=%v&count=%v", page, limit), "", "", nil, masterPubkey...)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var files listedFiles
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(body, &files))
	return &files
}

func deleteFile(t *testing.T, ctx context.Context, sk string, fileHash string, masterKey ...string) int {
	t.Helper()
	resp := authorizedReq(t, ctx, sk, "DELETE", fmt.Sprintf("https://localhost:9996/files/%v", fileHash), "", "", nil, masterKey...)
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

func authorizedReq(t *testing.T, ctx context.Context, sk, method, url, fileHash, contentType string, body io.Reader, masterKey ...string) *http.Response {
	t.Helper()

	uploadReq, err := http.NewRequest(method, url, body)
	require.NoError(t, err)

	uploadReq.Header.Set("Content-Type", contentType)
	uploadReq.Header.Set("Authorization", generateAuthHeader(t, sk, method, fileHash, uploadReq.URL, masterKey...))

	resp, err := http.DefaultClient.Do(uploadReq.WithContext(ctx))
	require.NoError(t, err)
	require.NotNil(t, resp)

	return resp
}

func expectedResponse(caption string) *nip96.UploadResponse {
	expectedResponses := map[string]*nip96.UploadResponse{
		"ice profile pic": {
			Status:        "success",
			Message:       "Upload successful.",
			ProcessingURL: "",
			Nip94Event: struct {
				Tags    nostr.Tags `json:"tags"`
				Content string     `json:"content"`
			}{
				Tags: nostr.Tags{
					nostr.Tag{"summary", "profile.png"},
					nostr.Tag{"ox", "b2b8cf9202b45dad7e137516bcf44b915ce30b39c3b294629a9b6b8fa1585292"},
					nostr.Tag{"x", "b2b8cf9202b45dad7e137516bcf44b915ce30b39c3b294629a9b6b8fa1585292"},
					nostr.Tag{"m", "image/png"},
					nostr.Tag{"size", "182744"},
				},
				Content: "ice profile pic",
			},
		},
		"ice logo": {
			Status:        "success",
			Message:       "Upload successful.",
			ProcessingURL: "",
			Nip94Event: struct {
				Tags    nostr.Tags `json:"tags"`
				Content string     `json:"content"`
			}{
				Tags: nostr.Tags{
					nostr.Tag{"summary", "ice.jpg"},
					nostr.Tag{"ox", "777d453395088530ce8de776fe54c3e5ace548381007b743e067844858962218"},
					nostr.Tag{"x", "777d453395088530ce8de776fe54c3e5ace548381007b743e067844858962218"},
					nostr.Tag{"m", "image/png"},
					nostr.Tag{"size", "415939"},
				},
				Content: "ice profile pic",
			},
		},
		"text file": {
			Status:        "success",
			Message:       "Upload successful.",
			ProcessingURL: "",
			Nip94Event: struct {
				Tags    nostr.Tags `json:"tags"`
				Content string     `json:"content"`
			}{
				Tags: nostr.Tags{
					nostr.Tag{"summary", "text.txt"},
					nostr.Tag{"ox", "982d9e3eb996f559e633f4d194def3761d909f5a3b647d1a851fead67c32c9d1"},
					nostr.Tag{"x", "982d9e3eb996f559e633f4d194def3761d909f5a3b647d1a851fead67c32c9d1"},
					nostr.Tag{"m", "text/plain"},
					nostr.Tag{"size", "4"},
				},
				Content: "text file",
			},
		},
		"master's file": {
			Status:        "success",
			Message:       "Upload successful.",
			ProcessingURL: "",
			Nip94Event: struct {
				Tags    nostr.Tags `json:"tags"`
				Content string     `json:"content"`
			}{
				Tags: nostr.Tags{
					nostr.Tag{"summary", "master.txt"},
					nostr.Tag{"ox", "fc613b4dfd6736a7bd268c8a0e74ed0d1c04a959f59dd74ef2874983fd443fc9"},
					nostr.Tag{"x", "fc613b4dfd6736a7bd268c8a0e74ed0d1c04a959f59dd74ef2874983fd443fc9"},
					nostr.Tag{"m", "text/plain"},
					nostr.Tag{"size", "6"},
				},
				Content: "master's file",
			},
		},
	}
	return expectedResponses[caption]
}

func initStorage(ctx context.Context) {
	transportOverride := http.DefaultClient.Transport
	http.DefaultClient.Transport = http.DefaultTransport
	storage.MustInit(ctx)
	http.DefaultClient.Transport = transportOverride
}

func generateAuthHeader(t *testing.T, sk, method, fileHash string, urlValue *url.URL, masterPubkey ...string) string {
	t.Helper()

	pk, err := model.GetPublicKey(sk)
	require.NoError(t, err)

	event := model.Event{
		Event: nostr.Event{
			Kind:      nip98.NostrHttpAuthKind,
			PubKey:    pk,
			CreatedAt: nostr.Now(),
			Tags: model.Tags{
				model.Tag{"u", urlValue.String()},
				model.Tag{"method", method},
				model.Tag{"payload", fileHash},
			},
		},
	}
	if len(masterPubkey) > 0 && masterPubkey[0] != "" {
		event.Tags = append(event.Tags, model.Tag{"b", masterPubkey[0]})
	}
	require.NoError(t, event.SignWithAlg(sk, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	b, err := json.Marshal(event)
	require.NoError(t, err)

	return `Nostr ` + base64.StdEncoding.EncodeToString(b)
}

const benchParallelism = 100

func BenchmarkUploadFiles(b *testing.B) {
	if os.Getenv("CI") != "" {
		b.Skip()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	http.DefaultClient.Transport = &http2.Transport{TLSClientConfig: &tls.Config{}}
	meter := tachymeter.New(&tachymeter.Config{Size: b.N})
	b.ResetTimer()
	b.ReportAllocs()
	fmt.Println(b.N)
	b.SetParallelism(benchParallelism)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sk := model.GeneratePrivateKey()
			img, _ := testdata.Open(".testdata/image2.png")
			defer img.Close()
			start := time.Now()
			resp, err := nip96.Upload(ctx, nip96.UploadRequest{
				Host:        "https://localhost:9996/files",
				File:        img,
				Filename:    "profile.png",
				Caption:     "ice profile pic",
				ContentType: "image/png",
				SK:          sk,
				SignPayload: true,
			})
			require.NoError(b, err)
			meter.AddTime(time.Since(start))
			nip94Event := &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindFileMetadata,
				Tags:      resp.Nip94Event.Tags,
			}}
			require.NoError(b, nip94Event.SignWithAlg(sk, model.SignAlgEDDSA, model.KeyAlgCurve25519))

			relay := nostr.NewRelay(ctx, "wss://localhost:9998/", nostr.WithSignatureChecker(func(e *nostr.Event) bool {
				return true
			}))
			require.NoError(b, relay.ConnectWithTLS(ctx, &tls.Config{}))
			require.NoError(b, relay.Publish(ctx, nip94Event.Event))
			require.NoError(b, relay.Close())
			b.Log(nip94Event)
		}
	})
	helperBenchReportMetrics(b, meter)
}

func helperBenchReportMetrics(
	t interface {
		Helper()
		ReportMetric(float64, string)
	},
	meter *tachymeter.Tachymeter,
) {
	t.Helper()

	metric := meter.Calc()
	t.ReportMetric(float64(metric.Time.Avg.Milliseconds()), "avg-ms/op")
	t.ReportMetric(float64(metric.Time.StdDev.Milliseconds()), "stddev-ms/op")
	t.ReportMetric(float64(metric.Time.P50.Milliseconds()), "p50-ms/op")
	t.ReportMetric(float64(metric.Time.P95.Milliseconds()), "p95-ms/op")
}
