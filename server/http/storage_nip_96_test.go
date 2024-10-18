// SPDX-License-Identifier: ice License 1.0

package http

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	gomime "github.com/cubewise-code/go-mime"
	"github.com/jamiealquiza/tachymeter"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip94"
	"github.com/nbd-wtf/go-nostr/nip96"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/storage"
	storagefixture "github.com/ice-blockchain/subzero/storage/fixture"
)

//go:embed .testdata
var testdata embed.FS

func TestNIP96(t *testing.T) {
	t.Parallel()
	now := time.Now().Unix()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	defer func() {
		require.NoError(t, storage.Client().Close())
		require.NoError(t, os.RemoveAll("./../../.test-uploads"))
		require.NoError(t, os.RemoveAll("./../../.test-uploads2"))
	}()
	master := "c24f7ab5b42254d6558e565ec1c170b266a7cd2be1edf9f42bfb375640f7f559"
	masterPubKey, _ := nostr.GetPublicKey(master)
	user1 := "3c00c01e6556c4b603b4c49d12059e02c42161d055b658e5635fa6206f594306"
	user2 := "cea41ff6c6e9eb0cde6740a1fbe8c134bda650ce819e43b68bf61add2c68f8d9"
	var tagsToBroadcast nostr.Tags
	var contentToBroadcast string
	t.Run("create on-behalf attestations", func(t *testing.T) {
		user1PubKey, _ := nostr.GetPublicKey(user1)
		user2PubKey, _ := nostr.GetPublicKey(user2)
		var ev model.Event
		ev.Kind = model.CustomIONKindAttestation
		ev.CreatedAt = 1
		ev.Tags = model.Tags{
			{model.TagAttestationName, user1PubKey, "", model.CustomIONAttestationKindActive + ":" + strconv.Itoa(int(now-10))},
			{model.TagAttestationName, user2PubKey, "", model.CustomIONAttestationKindActive + ":" + strconv.Itoa(int(now-5))},
		}
		require.NoError(t, ev.Sign(master))
		require.NoError(t, query.AcceptEvent(ctx, &ev))
	})
	t.Run("files are uploaded, response is ok", func(t *testing.T) {
		var responses chan *nip96.UploadResponse
		responses = make(chan *nip96.UploadResponse, 2)
		upload(t, ctx, user1, masterPubKey, ".testdata/image2.png", "profile.png", "ice profile pic", func(resp *nip96.UploadResponse) {
			responses <- resp
		})
		upload(t, ctx, user1, masterPubKey, ".testdata/image.jpg", "ice.jpg", "ice logo", func(resp *nip96.UploadResponse) {
			responses <- resp
		})
		upload(t, ctx, user2, masterPubKey, ".testdata/image2.png", "profile.png", "ice profile pic", func(resp *nip96.UploadResponse) {})
		upload(t, ctx, user2, masterPubKey, ".testdata/image.jpg", "ice.jpg", "ice logo", func(resp *nip96.UploadResponse) {})
		upload(t, ctx, user2, masterPubKey, ".testdata/text.txt", "text.txt", "text file", func(resp *nip96.UploadResponse) {})
		upload(t, ctx, master, "", ".testdata/text-master.txt", "master.txt", "master's file", func(resp *nip96.UploadResponse) {})
		close(responses)
		for resp := range responses {
			verifyFile(t, resp.Nip94Event.Content, resp.Nip94Event.Tags)
			tagsToBroadcast = resp.Nip94Event.Tags
			contentToBroadcast = resp.Nip94Event.Content
		}
	})
	const newStorageRoot = "./../../.test-uploads2"
	var nip94EventToSign *model.Event
	t.Run("nip-94 event is broadcasted, it causes download to other node", func(t *testing.T) {
		tagsToBroadcast = tagsToBroadcast.AppendUnique(model.Tag{"b", masterPubKey})
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
		if xTag := nip94EventToSign.Tags.GetFirst([]string{"x"}); xTag != nil && len(*xTag) > 1 {
			fileHash = xTag.Value()
		} else {
			t.Fatalf("malformed x tag in nip94 event %v", nip94EventToSign.ID)
		}
		status := deleteFile(t, ctx, user1, fileHash, masterPubKey)
		require.Equal(t, http.StatusOK, status)
		fileName := nip94.ParseFileMetadata(nostr.Event{Tags: expectedResponse(nip94EventToSign.Content).Nip94Event.Tags}).Summary
		require.NoFileExists(t, filepath.Join(storageRoot, masterPubKey, fileName))
		deletionEventToSign := &model.Event{nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindDeletion,
			Tags: nostr.Tags{
				nostr.Tag{"e", nip94EventToSign.ID},
				nostr.Tag{"k", strconv.FormatInt(int64(nostr.KindFileMetadata), 10)},
				nostr.Tag{"b", masterPubKey},
			},
		}}
		require.NoError(t, deletionEventToSign.Sign(user1))
		require.NoError(t, storage.AcceptEvent(ctx, deletionEventToSign))
		require.NoFileExists(t, filepath.Join(newStorageRoot, masterPubKey, fileName))
	})
	t.Run("delete file owned by user 2 on behave of usr 1 (attestation)", func(t *testing.T) {
		status := deleteFile(t, ctx, user1, "982d9e3eb996f559e633f4d194def3761d909f5a3b647d1a851fead67c32c9d1", masterPubKey)
		require.Equal(t, http.StatusOK, status)
		fileName := "text.txt"
		require.NoFileExists(t, filepath.Join(storageRoot, masterPubKey, fileName))
		deletionEventToSign := &model.Event{nostr.Event{
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      nostr.KindDeletion,
			Tags: nostr.Tags{
				nostr.Tag{"e", nip94EventToSign.ID},
				nostr.Tag{"k", strconv.FormatInt(int64(nostr.KindFileMetadata), 10)},
				nostr.Tag{"b", masterPubKey},
			},
		}}
		require.NoError(t, deletionEventToSign.Sign(user1))
		require.NoError(t, storage.AcceptEvent(ctx, deletionEventToSign))
		require.NoFileExists(t, filepath.Join(newStorageRoot, masterPubKey, fileName))
	})
	t.Run("delete file owned by master by usr1 (Forbidden)", func(t *testing.T) {
		status := deleteFile(t, ctx, user1, "fc613b4dfd6736a7bd268c8a0e74ed0d1c04a959f59dd74ef2874983fd443fc9", masterPubKey)
		require.Equal(t, http.StatusForbidden, status)
		fileName := "master.txt"
		require.FileExists(t, filepath.Join(storageRoot, masterPubKey, fileName))
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
	httpResp := authorizedReq(t, ctx, sk, "POST", "https://localhost:9997/files", hex.EncodeToString(fileHash.Sum(nil)), writer.FormDataContentType(), &requestBody, master)
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
	resp := authorizedReq(t, ctx, sk, "GET", fmt.Sprintf("https://localhost:9997/files/%v", fileHash), "", "", nil, masterPubkey...)
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
	resp := authorizedReq(t, ctx, sk, "GET", fmt.Sprintf("https://localhost:9997/files?page=%v&count=%v", page, limit), "", "", nil, masterPubkey...)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var files listedFiles
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(body, &files))
	return &files
}

func deleteFile(t *testing.T, ctx context.Context, sk string, fileHash string, masterKey ...string) int {
	t.Helper()
	resp := authorizedReq(t, ctx, sk, "DELETE", fmt.Sprintf("https://localhost:9997/files/%v", fileHash), "", "", nil, masterKey...)
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
	uploadReq.Header.Set("Content-Type", contentType)
	require.NoError(t, err)
	auth, err := generateAuthHeader(t, sk, method, fileHash, uploadReq.URL, masterKey...)
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
					nostr.Tag{"summary", "profile.png"},
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
					nostr.Tag{"summary", "ice.jpg"},
					nostr.Tag{"ox", "777d453395088530ce8de776fe54c3e5ace548381007b743e067844858962218"},
					nostr.Tag{"x", "777d453395088530ce8de776fe54c3e5ace548381007b743e067844858962218"},
					nostr.Tag{"m", "image/png"},
					nostr.Tag{"size", "415939"},
				},
				Content: "ice profile pic",
			},
		},
		"text file": &nip96.UploadResponse{
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
		"master's file": &nip96.UploadResponse{
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

func initStorage(ctx context.Context, path string) {
	transportOverride := http.DefaultClient.Transport
	http.DefaultClient.Transport = http.DefaultTransport
	_, nodeKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		log.Panic(errors.Wrapf(err, "failed to generate node key"))
	}
	storagePort, err := rand.Int(rand.Reader, big.NewInt(63500))
	if err != nil {
		log.Panic(errors.Wrapf(err, "failed to generate port number"))
	}
	wd, _ := os.Getwd()
	rootStorage := filepath.Join(wd, path)
	port := int(storagePort.Int64()) + 1024
	storage.MustInit(ctx, nodeKey, storage.DefaultConfigUrl, rootStorage, net.ParseIP("127.0.0.1"), port, true)
	http.DefaultClient.Transport = transportOverride
}

func generateAuthHeader(t *testing.T, sk, method, fileHash string, urlValue *url.URL, masterPubkey ...string) (string, error) {
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
			nostr.Tag{"u", urlValue.String()},
			nostr.Tag{"method", method},
			nostr.Tag{"payload", fileHash},
		},
	}
	if len(masterPubkey) > 0 && masterPubkey[0] != "" {
		event.Tags = append(event.Tags, nostr.Tag{"b", masterPubkey[0]})
	}
	require.NoError(t, event.Sign(sk))

	b, err := json.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("json.Marshal: %w", err)
	}

	payload := base64.StdEncoding.EncodeToString(b)

	return fmt.Sprintf("Nostr %s", payload), nil
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
			sk := nostr.GeneratePrivateKey()
			img, _ := testdata.Open(".testdata/image2.png")
			defer img.Close()
			start := time.Now()
			resp, err := nip96.Upload(ctx, nip96.UploadRequest{
				Host:        "https://localhost:9997/files",
				File:        img,
				Filename:    "profile.png",
				Caption:     "ice profile pic",
				ContentType: "image/png",
				SK:          sk,
				SignPayload: true,
			})
			require.NoError(b, err)
			meter.AddTime(time.Since(start))
			nip94Event := &model.Event{nostr.Event{
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      nostr.KindFileMetadata,
				Tags:      resp.Nip94Event.Tags,
			}}
			require.NoError(b, nip94Event.Sign(sk))

			relay := nostr.NewRelay(ctx, "wss://localhost:9998/")
			err = relay.ConnectWithTLS(ctx, &tls.Config{})
			if err = nip94Event.Sign(sk); err != nil {
				log.Panic(err)
			}
			if err = relay.Publish(ctx, nip94Event.Event); err != nil {
				log.Panic(err)
			}
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
