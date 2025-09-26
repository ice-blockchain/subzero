// SPDX-License-Identifier: ice License 1.0

package nip96

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	gomime "github.com/cubewise-code/go-mime"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jamiealquiza/tachymeter"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip94"
	"github.com/nbd-wtf/go-nostr/nip96"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/cert"
	"github.com/ice-blockchain/subzero/server/http/nip11"
	"github.com/ice-blockchain/subzero/server/http/nip98"
	wsserver "github.com/ice-blockchain/subzero/server/ws"
	"github.com/ice-blockchain/subzero/server/ws/fixture"
	"github.com/ice-blockchain/subzero/storage"
)

const (
	minLeadingZeroBits = 5
	testPrivateKey     = `f2e7f1829027cf347d3ddb90e40890fcffb1ca99a3fc2564a120286570b690e548ac550ea62ab27c8c85d1f862b779b8c7dd09d733403dd9a3104be42f8ddb7b`
)

var (
	//go:embed .testdata
	testdata embed.FS

	testMainStorageRoot string
)

func TestMain(m *testing.M) {
	serverCtx, serverCancel := context.WithTimeout(context.Background(), 10*time.Minute)

	addr, release := query.NewTestDatabase(serverCtx)
	query.MustInit(serverCtx, query.WithConfig(&query.Config{
		WriteURLs:  []string{addr},
		PrivateKey: testPrivateKey,
		RelayURL:   "wss://localhost:9996",
	}))

	var err error
	testMainStorageRoot, err = os.MkdirTemp("", "test-nip96-storage-root")
	if err != nil {
		log.Panic().Err(err).Msg("failed to create temp storage root")
	}

	initServer(serverCtx, 9996, storage.WithConfig(&storage.Config{
		PrivateKey:              testPrivateKey,
		RelayURL:                "wss://localhost:9996",
		ExternalADNLPort:        12345,
		Debug:                   true,
		AbsoluteRootStoragePath: testMainStorageRoot,
		IONStorageConfigURL:     "https://ton.org/testnet-global.config.json",
	}))

	code := m.Run()
	serverCancel()
	release()
	os.RemoveAll(testMainStorageRoot)
	os.Exit(code)
}

func initServer(serverCtx context.Context, port uint16, opts ...storage.Option) *fixture.MockService {
	initStorage(serverCtx, opts...)
	uploader := NewUploadHandler(serverCtx, false, nip11.NewFetcher(serverCtx))
	return fixture.NewTestServer(
		serverCtx,
		&wsserver.Config{
			TLSConfig: cert.MustGenerateTLSConfigSelfSigned("localhost"),
			Port:      port,
		},
		nil,
		nip11.NewNIP11Handler(
			serverCtx,
			&nip11.Config{MinLeadingZeroBits: minLeadingZeroBits, PrivateKey: testPrivateKey},
			uploader.RootPath(),
			os.TempDir(),
		),
		map[string]gin.HandlerFunc{
			"POST /files":         uploader.Upload(),
			"GET /files":          uploader.ListFiles(),
			"GET /files/:file":    uploader.Download(),
			"HEAD /files/:file":   uploader.CrossRelayDownload(),
			"DELETE /files/:file": uploader.Delete(),

			"POST /xfiles/*tus-uploader":    gin.WrapH(http.StripPrefix("/xfiles/", uploader.LargeFiles())),
			"PATCH /xfiles/*tus-uploader":   gin.WrapH(http.StripPrefix("/xfiles/", uploader.LargeFiles())),
			"HEAD /xfiles/*tus-uploader":    gin.WrapH(http.StripPrefix("/xfiles/", uploader.LargeFiles())),
			"GET /xfiles/*tus-uploader":     gin.WrapH(http.StripPrefix("/xfiles/", uploader.LargeFiles())),
			"OPTIONS /xfiles/*tus-uploader": gin.WrapH(http.StripPrefix("/xfiles/", uploader.LargeFiles())),
		})
}

func TestNIP96(t *testing.T) {
	const filesCount = 7
	master, masterPubKey := model.GenerateKeyPair()
	user1, user1PubKey := model.GenerateKeyPair()
	user2, user2PubKey := model.GenerateKeyPair()

	helperCreateAttestations(t, t.Context(), master, masterPubKey, user1PubKey, user2PubKey)

	var events model.Events
	t.Run("Upload files", func(t *testing.T) {
		type uploadResponse struct {
			R   *nip96.UploadResponse
			Err error
		}
		responsesCh := make(chan uploadResponse, filesCount)
		var wg sync.WaitGroup
		wg.Go(func() {
			resp, err := upload(t.Context(), user1, masterPubKey, ".testdata/image2.png", "profile.png", "ice profile pic")
			responsesCh <- uploadResponse{R: resp, Err: err}
		})
		wg.Go(func() {
			resp, err := upload(t.Context(), master, "", ".testdata/text-master.txt", "master.txt", "master's file")
			responsesCh <- uploadResponse{R: resp, Err: err}
		})
		wg.Go(func() {
			resp, err := upload(t.Context(), user1, masterPubKey, ".testdata/image.jpg", "ice.jpg", "ice logo")
			responsesCh <- uploadResponse{R: resp, Err: err}
		})
		wg.Go(func() {
			resp, err := upload(t.Context(), user1, masterPubKey, ".testdata/text.txt", "text.txt", "text file")
			responsesCh <- uploadResponse{R: resp, Err: err}
		})
		wg.Go(func() {
			resp, err := upload(t.Context(), user1, masterPubKey, ".testdata/to-be-deleted-on-same-relay.txt", "to-be-deleted-on-same-relay.txt", "to be deleted")
			responsesCh <- uploadResponse{R: resp, Err: err}
		})
		wg.Go(func() {
			resp, err := upload(t.Context(), user1, masterPubKey, ".testdata/dupl1.txt", "dupl1.txt", "same content file1")
			responsesCh <- uploadResponse{R: resp, Err: err}
		})
		wg.Go(func() {
			resp, err := upload(t.Context(), user1, masterPubKey, ".testdata/dupl1.txt", "dupl2.txt", "same content file2")
			responsesCh <- uploadResponse{R: resp, Err: err}
		})
		wg.Wait()
		close(responsesCh)
		for resp := range responsesCh {
			require.NoError(t, resp.Err)
			require.NotNil(t, resp.R)

			verifyFile(t, resp.R.Nip94Event.Content, resp.R.Nip94Event.Tags)
			tagsToBroadcast := resp.R.Nip94Event.Tags.
				AppendUnique(model.Tag{"b", masterPubKey}).
				AppendUnique(model.Tag{"expiration", nostr.Now().Add(-time.Second * 10).String()})

			nip94EventToSign := &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindFileMetadata,
				Tags:      tagsToBroadcast,
				Content:   resp.R.Nip94Event.Content,
			}}
			require.NoError(t, nip94EventToSign.SignWithAlg(user1, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			events = append(events, nip94EventToSign)
		}
	})
	rand.Shuffle(len(events), func(i, j int) {
		events[i], events[j] = events[j], events[i]
	})
	t.Run("delete file on same relay", func(t *testing.T) {
		var nip94ToBeDeleted *model.Event
		for _, e := range events {
			if e.GetTag("ox").Value() == "aca3a138e07162a8565070575b3593ee2fef404d447162f5786d5cc645d82b7a" {
				nip94ToBeDeleted = e
				break
			}
		}
		fileHash := nip94ToBeDeleted.GetTag("ox").Value()
		require.NotEmptyf(t, fileHash, "ox tag should be present in the event %v", nip94ToBeDeleted.ID)
		fileName := nip94.ParseFileMetadata(nostr.Event{Tags: expectedResponse(nip94ToBeDeleted.Content).Nip94Event.Tags}).Summary
		var wg sync.WaitGroup
		imetaEvent := &model.Event{Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      model.CustomIONKindEditableTextNote,
			Tags: nostr.Tags{
				{
					"imeta",
					"ox aca3a138e07162a8565070575b3593ee2fef404d447162f5786d5cc645d82b7a",
					fmt.Sprintf("url %v", nip94ToBeDeleted.GetTag("url").Value()),
				},
				{"d", "editable post1"},
				{"b", masterPubKey},
			},
		}}
		require.NoError(t, imetaEvent.SignWithAlg(user2, model.SignAlgEDDSA, model.KeyAlgCurve25519))
		require.NoError(t, query.AcceptEvents(t.Context(), imetaEvent))
		wg.Go(func() {
			deletedPost := &model.Event{
				Event: nostr.Event{
					Kind:      imetaEvent.Kind,
					Content:   "",                                    // Empty content for soft deletion.
					CreatedAt: imetaEvent.CreatedAt.Add(time.Second), // Should be newer than the original post.
					Tags: model.Tags{
						{"b", masterPubKey},
						{"published_at", imetaEvent.CreatedAt.String()},
						{"d", "editable post1"},
					},
				},
			}
			require.NoError(t, deletedPost.SignWithAlg(user1, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, query.AcceptEvents(t.Context(), deletedPost))
			cpy := new(model.Event)
			*cpy = *deletedPost
			require.NoError(t, query.CommitEvents(t.Context(), cpy))
			require.NoError(t, storage.AcceptEvents(t.Context(), cpy))
		})
		wg.Go(func() {
			status := deleteFile(t, t.Context(), user1, fileHash, masterPubKey)
			require.Equal(t, http.StatusOK, status)
			require.NoFileExists(t, filepath.Join(testMainStorageRoot, masterPubKey, fileName))
		})
		wg.Wait()
		require.NoFileExists(t, filepath.Join(testMainStorageRoot, masterPubKey, fileName))
	})
	t.Run("nip-94 accepted on same relay where is was uploaded to = no-op", func(t *testing.T) {
		var wg sync.WaitGroup
		for _, e := range events {
			wg.Go(func() {
				require.NoError(t, query.AcceptEvents(t.Context(), e))
				require.NoError(t, storage.AcceptEvents(t.Context(), e))
				require.NoError(t, storage.ReplicateFileOnPeers(t.Context(), e))
			})
		}
		wg.Wait()
	})

	t.Run("Second node", func(t *testing.T) {
		// Simulate another storage node where we broadcast event/bag, and it needs to download it.
		newStorageRoot, err := os.MkdirTemp("", "test-nip96-storage")
		require.NoError(t, err)
		t.Cleanup(func() {
			if err := os.RemoveAll(newStorageRoot); err != nil {
				t.Logf("failed to remove temp storage root %v: %v", newStorageRoot, err)
			}
		})

		storage.Reset()
		initStorage(t.Context(), storage.WithConfig(&storage.Config{
			PrivateKey:              testPrivateKey,
			IONStorageConfigURL:     "https://ton.org/testnet-global.config.json",
			AbsoluteRootStoragePath: newStorageRoot,
			ExternalADNLPort:        12347,
			Debug:                   true,
			RelayURL:                "wss://localhost:9996",
		}))
		t.Logf("new storage root at %v initialized", newStorageRoot)

		t.Run("nip-94 event is broadcasted, it causes download to other node", func(t *testing.T) {
			var wg sync.WaitGroup
			wg.Add(len(events))
			for _, e := range events {
				go func() {
					defer wg.Done()
					require.NoError(t, query.AcceptEvents(t.Context(), e))
					require.NoError(t, storage.AcceptEvents(t.Context(), e))
					require.NoError(t, storage.ReplicateFileOnPeers(t.Context(), e))
				}()
			}
			wg.Wait()

			ctx, cancel := context.WithTimeout(t.Context(), time.Minute*5)
			defer cancel()

			downloadedProfileHash, err := storage.WaitForFile(t,
				ctx,
				newStorageRoot,
				filepath.Join(newStorageRoot, masterPubKey, "b2b8cf9202b45dad7e137516bcf44b915ce30b39c3b294629a9b6b8fa1585292.png"),
				"b2b8cf9202b45dad7e137516bcf44b915ce30b39c3b294629a9b6b8fa1585292",
				182744,
			)
			require.NoError(t, err)
			require.Equal(t, "b2b8cf9202b45dad7e137516bcf44b915ce30b39c3b294629a9b6b8fa1585292", downloadedProfileHash)
			downloadedLogoHash, err := storage.WaitForFile(t,
				ctx,
				newStorageRoot,
				filepath.Join(newStorageRoot, masterPubKey, "777d453395088530ce8de776fe54c3e5ace548381007b743e067844858962218.jpg"),
				"777d453395088530ce8de776fe54c3e5ace548381007b743e067844858962218",
				415939,
			)
			require.NoError(t, err)
			require.Equal(t, "777d453395088530ce8de776fe54c3e5ace548381007b743e067844858962218", downloadedLogoHash)
			require.NoFileExists(t, filepath.Join(testMainStorageRoot, masterPubKey, "aca3a138e07162a8565070575b3593ee2fef404d447162f5786d5cc645d82b7a.txt"))
		})
		t.Run("delete file by same hash used in multiple posts does not break link", func(t *testing.T) {
			deleteFileAndVerify := func(verify func(fileName string)) {
				var nip94ToBeDeleted *model.Event
				for _, e := range events {
					if e.GetTag("ox").Value() == "c7fce3cad585a3110c96b34516df16362c99f6f32359d64ddf1a58c1710247d1" {
						nip94ToBeDeleted = e
						break
					}
				}
				fileHash := nip94ToBeDeleted.GetTag("ox").Value()
				require.NotEmptyf(t, fileHash, "ox tag should be present in the event %v", nip94ToBeDeleted.ID)
				status := deleteFile(t, t.Context(), user1, fileHash, masterPubKey)
				require.Equal(t, http.StatusOK, status)
				fileName := nip94.ParseFileMetadata(nostr.Event{Tags: expectedResponse(nip94ToBeDeleted.Content).Nip94Event.Tags}).Summary

				t.Logf("File %v deleted, now verifying that it is removed from storage", fileName)

				deletionEventToSign := &model.Event{Event: nostr.Event{
					CreatedAt: nostr.Now(),
					Kind:      nostr.KindDeletion,
					Tags: nostr.Tags{
						{"e", nip94ToBeDeleted.ID},
						{"k", strconv.Itoa(nostr.KindFileMetadata)},
						{"b", masterPubKey},
					},
				}}
				require.NoError(t, deletionEventToSign.SignWithAlg(user1, model.SignAlgEDDSA, model.KeyAlgCurve25519))
				require.NoError(t, query.AcceptEvents(t.Context(), deletionEventToSign))
				require.NoError(t, storage.AcceptEvents(t.Context(), deletionEventToSign))

				verify(fileName)
			}
			deleteFileAndVerify(func(fileName string) {
				require.FileExists(t, filepath.Join(testMainStorageRoot, masterPubKey, fileName))
				status, location := download(t, t.Context(), user1, "c7fce3cad585a3110c96b34516df16362c99f6f32359d64ddf1a58c1710247d1", masterPubKey)
				require.Equal(t, http.StatusFound, status)
				require.Regexp(t, "^http://[0-9a-fA-F]{64}.bag/c7fce3cad585a3110c96b34516df16362c99f6f32359d64ddf1a58c1710247d1.txt.+", location)
			})
		})
		t.Run("download endpoint redirects to same download url over ton storage", func(t *testing.T) {
			expected := nip94.ParseFileMetadata(nostr.Event{Tags: expectedResponse("ice logo").Nip94Event.Tags})
			status, location := download(t, t.Context(), user1, "777d453395088530ce8de776fe54c3e5ace548381007b743e067844858962218", masterPubKey)
			require.Equal(t, http.StatusFound, status)
			require.Regexp(t, fmt.Sprintf("^http://[0-9a-fA-F]{64}.bag/%v", expected.Summary), location)

			expected = nip94.ParseFileMetadata(nostr.Event{Tags: expectedResponse("ice profile pic").Nip94Event.Tags})
			status, location = download(t, t.Context(), user1, "b2b8cf9202b45dad7e137516bcf44b915ce30b39c3b294629a9b6b8fa1585292", masterPubKey)
			require.Equal(t, http.StatusFound, status)
			require.Regexp(t, fmt.Sprintf("^http://[0-9a-fA-F]{64}.bag/%v", expected.Summary), location)
			status, _ = download(t, t.Context(), user1, "non_valid_hash")
			require.Equal(t, http.StatusNotFound, status)
		})
		t.Run("list files responds with up to all files for the user when total is less than page", func(t *testing.T) {
			files := list(t, t.Context(), user1, 0, 0, masterPubKey)
			require.Equal(t, uint32(filesCount-1), files.Total)
			require.Len(t, files.Files, filesCount-1)
			for _, f := range files.Files {
				verifyFile(t, f.Content, f.Tags)
			}
		})
		t.Run("list files with pagination", func(t *testing.T) {
			files := list(t, t.Context(), user1, 0, 1, masterPubKey)
			require.Equal(t, uint32(filesCount-1), files.Total)
			require.Len(t, files.Files, 1)
			uniqFiles := map[string]struct{}{}
			for _, f := range files.Files {
				verifyFile(t, f.Content, f.Tags)
				uniqFiles[f.Content] = struct{}{}
			}
			files = list(t, t.Context(), user1, 1, 1, masterPubKey)
			require.Equal(t, uint32(filesCount-1), files.Total)
			require.Len(t, files.Files, 1)
			for _, f := range files.Files {
				verifyFile(t, f.Content, f.Tags)
				_, presentedBefore := uniqFiles[f.Content]
				require.False(t, presentedBefore)
			}
		})
		t.Run("delete file owned by user 1 on behave of usr 1 (normally)", func(t *testing.T) {
			var nip94ToBeDeleted *model.Event
			for _, e := range events {
				if e.GetTag("ox").Value() == "b2b8cf9202b45dad7e137516bcf44b915ce30b39c3b294629a9b6b8fa1585292" {
					nip94ToBeDeleted = e
					break
				}
			}
			fileHash := nip94ToBeDeleted.GetTag("ox").Value()
			require.NotEmptyf(t, fileHash, "ox tag should be present in the event %v", nip94ToBeDeleted.ID)
			fileName := nip94.ParseFileMetadata(nostr.Event{Tags: expectedResponse(nip94ToBeDeleted.Content).Nip94Event.Tags}).Summary
			var wg sync.WaitGroup
			wg.Go(func() {
				status := deleteFile(t, t.Context(), user1, fileHash, masterPubKey)
				require.Equal(t, http.StatusOK, status)
				require.NoFileExists(t, filepath.Join(testMainStorageRoot, masterPubKey, fileName))
			})
			wg.Go(func() {
				deletionEventToSign := &model.Event{Event: nostr.Event{
					CreatedAt: nostr.Now(),
					Kind:      nostr.KindDeletion,
					Tags: nostr.Tags{
						{"e", nip94ToBeDeleted.ID},
						{"k", strconv.FormatInt(int64(nostr.KindFileMetadata), 10)},
						{"b", masterPubKey},
					},
				}}
				require.NoError(t, deletionEventToSign.SignWithAlg(user1, model.SignAlgEDDSA, model.KeyAlgCurve25519))
				require.NoError(t, storage.AcceptEvents(t.Context(), deletionEventToSign))
				require.NoFileExists(t, filepath.Join(newStorageRoot, masterPubKey, fileName))
			})
			wg.Wait()
		})
		t.Run("delete file owned by user 1 on behave of usr 2 (attestation) via deletion of imeta tagged post", func(t *testing.T) {
			var nip94ToBeDeleted *model.Event
			for _, e := range events {
				if e.GetTag("ox").Value() == "982d9e3eb996f559e633f4d194def3761d909f5a3b647d1a851fead67c32c9d1" {
					nip94ToBeDeleted = e
					break
				}
			}
			status := deleteFile(t, t.Context(), user2, "982d9e3eb996f559e633f4d194def3761d909f5a3b647d1a851fead67c32c9d1", masterPubKey)
			require.Equal(t, http.StatusOK, status)
			fileName := "982d9e3eb996f559e633f4d194def3761d909f5a3b647d1a851fead67c32c9d1.txt"
			require.NoFileExists(t, filepath.Join(testMainStorageRoot, masterPubKey, fileName))
			imetaEvent := &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindTextNote,
				Tags: nostr.Tags{
					{
						"imeta",
						"ox 982d9e3eb996f559e633f4d194def3761d909f5a3b647d1a851fead67c32c9d1",
						fmt.Sprintf("url %v", nip94ToBeDeleted.GetTag("url").Value()),
					},
					{"k", strconv.FormatInt(int64(nostr.KindTextNote), 10)},
					{"b", masterPubKey},
				},
			}}
			require.NoError(t, imetaEvent.SignWithAlg(user2, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, query.AcceptEvents(t.Context(), imetaEvent))
			deletionEventToSign := &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindDeletion,
				Tags: nostr.Tags{
					{"e", imetaEvent.ID},
					{"k", strconv.FormatInt(int64(nostr.KindTextNote), 10)},
					{"b", masterPubKey},
				},
			}}
			require.NoError(t, deletionEventToSign.SignWithAlg(user2, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, storage.AcceptEvents(t.Context(), deletionEventToSign))
			require.NoFileExists(t, filepath.Join(newStorageRoot, masterPubKey, fileName))
		})
		t.Run("delete file linked with editable note which is soft-deleted", func(t *testing.T) {
			var nip94ToBeDeleted *model.Event
			for _, e := range events {
				if e.GetTag("ox").Value() == "c7fce3cad585a3110c96b34516df16362c99f6f32359d64ddf1a58c1710247d1" {
					nip94ToBeDeleted = e
					break
				}
			}
			imetaEvent := &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      model.CustomIONKindEditableTextNote,
				Tags: nostr.Tags{
					{
						"imeta",
						"ox c7fce3cad585a3110c96b34516df16362c99f6f32359d64ddf1a58c1710247d1",
						fmt.Sprintf("url %v", nip94ToBeDeleted.GetTag("url").Value()),
					},
					{"d", "editable post1"},
					{"b", masterPubKey},
				},
			}}
			require.NoError(t, imetaEvent.SignWithAlg(user2, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, query.AcceptEvents(t.Context(), imetaEvent))

			deletedPost := &model.Event{
				Event: nostr.Event{
					Kind:      imetaEvent.Kind,
					Content:   "",                                    // Empty content for soft deletion.
					CreatedAt: imetaEvent.CreatedAt.Add(time.Second), // Should be newer than the original post.
					Tags: model.Tags{
						{"b", masterPubKey},
						{"published_at", imetaEvent.CreatedAt.String()},
						{"d", "editable post1"},
					},
				},
			}
			require.NoError(t, deletedPost.SignWithAlg(user1, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, query.AcceptEvents(t.Context(), deletedPost))
			cpy := new(model.Event)
			*cpy = *deletedPost
			require.NoError(t, query.CommitEvents(t.Context(), cpy))
			require.NoError(t, storage.AcceptEvents(t.Context(), cpy))
			fileName := "c7fce3cad585a3110c96b34516df16362c99f6f32359d64ddf1a58c1710247d1.jpg"
			require.NoFileExists(t, filepath.Join(newStorageRoot, masterPubKey, fileName))
		})
		t.Run("delete file owned by master by usr1 (Forbidden)", func(t *testing.T) {
			status := deleteFile(t, t.Context(), user1, "fc613b4dfd6736a7bd268c8a0e74ed0d1c04a959f59dd74ef2874983fd443fc9", masterPubKey)
			require.Equal(t, http.StatusForbidden, status)
			fileName := "fc613b4dfd6736a7bd268c8a0e74ed0d1c04a959f59dd74ef2874983fd443fc9.txt"
			require.FileExists(t, filepath.Join(testMainStorageRoot, masterPubKey, fileName))
		})
		t.Run("trigger expired events cleanup", func(t *testing.T) {
			ch := make(chan struct{}, 100)
			query.RegisterExpiredEventsProcessor(func(ctx context.Context, events ...*model.Event) error {
				err := storage.DeleteExpiredFiles(ctx, events...)
				ch <- struct{}{}
				return err
			})
			require.NoError(t, query.TriggerExpiredEventsCleanup(t.Context()))
			select {
			case <-ch:
			case <-time.After(30 * time.Second):
				t.Fatal("Expired events processor was not triggered")
			}
			require.NoFileExists(t, filepath.Join(newStorageRoot, masterPubKey, "fc613b4dfd6736a7bd268c8a0e74ed0d1c04a959f59dd74ef2874983fd443fc9.txt"), "expiration")
		})
		t.Run("file re-uploaded after deletion", func(t *testing.T) {
			resp, err := upload(t.Context(), user1, masterPubKey, ".testdata/image2.png", "profile.png", "ice profile pic")
			require.NoError(t, err)
			require.NotNil(t, resp)
			verifyFile(t, resp.Nip94Event.Content, resp.Nip94Event.Tags)

			expected := nip94.ParseFileMetadata(nostr.Event{Tags: expectedResponse("ice profile pic").Nip94Event.Tags})
			status, location := download(t, t.Context(), user1, "b2b8cf9202b45dad7e137516bcf44b915ce30b39c3b294629a9b6b8fa1585292", masterPubKey)
			require.Equal(t, http.StatusFound, status)
			require.Regexp(t, fmt.Sprintf("^http://[0-9a-fA-F]{64}.bag/%v", expected.Summary), location)
		})
		t.Run("profile removal - causes whole storage removal for that user", func(t *testing.T) {
			deletionEventToSign := &model.Event{
				Event: nostr.Event{
					ID:      "deletion event4",
					PubKey:  masterPubKey,
					Kind:    nostr.KindDeletion,
					Content: "profile deletion",
				}}
			require.NoError(t, deletionEventToSign.SignWithAlg(master, model.SignAlgEDDSA, model.KeyAlgCurve25519))
			require.NoError(t, storage.AcceptEvents(t.Context(), deletionEventToSign))
			require.NoDirExists(t, filepath.Join(newStorageRoot, masterPubKey))
		})
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

func upload(ctx context.Context, sk, master, path, filename, caption string) (*nip96.UploadResponse, error) {
	img, _ := testdata.Open(path)
	defer img.Close()

	var requestBody bytes.Buffer
	fileHash := sha256.New()
	writer := multipart.NewWriter(&requestBody)
	fileWriter, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create form file")
	}
	_, err = io.Copy(fileWriter, io.TeeReader(img, fileHash))
	if err != nil {
		return nil, errors.Wrap(err, "failed to copy file content")
	}
	writer.WriteField("caption", caption)
	writer.WriteField("content_type", gomime.TypeByExtension(filepath.Ext(path)))
	writer.WriteField("no_transform", "true")
	err = writer.Close()
	if err != nil {
		return nil, errors.Wrap(err, "failed to close multipart writer")
	}
	httpResp := mustAuthorizedReq(ctx, sk, "POST", "https://localhost:9996/files", hex.EncodeToString(fileHash.Sum(nil)), writer.FormDataContentType(), &requestBody, master)
	switch httpResp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted:
		var resp nip96.UploadResponse
		err = json.NewDecoder(httpResp.Body).Decode(&resp)
		return &resp, errors.Wrap(err, "failed to decode upload response")
	}
	return nil, errors.Errorf("unexpected http status code %v for upload %v by %v", httpResp.StatusCode, filename, sk)
}

func download(t *testing.T, ctx context.Context, sk, fileHash string, masterPubkey ...string) (status int, locationUrl string) {
	t.Helper()

	http.DefaultClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	defer func() {
		http.DefaultClient.CheckRedirect = func(req *http.Request, via []*http.Request) error { return nil }
	}()
	resp := mustAuthorizedReq(ctx, sk, "GET", fmt.Sprintf("https://localhost:9996/files/%v", fileHash), "", "", nil, masterPubkey...)
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

	resp := mustAuthorizedReq(ctx, sk, "GET", fmt.Sprintf("https://localhost:9996/files?page=%v&count=%v", page, limit), "", "", nil, masterPubkey...)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var files listedFiles
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(body, &files))
	return &files
}

func deleteFile(t *testing.T, ctx context.Context, sk string, fileHash string, masterKey ...string) int {
	t.Helper()

	resp := mustAuthorizedReq(ctx, sk, "DELETE", fmt.Sprintf("https://localhost:9996/files/%v", fileHash), "", "", nil, masterKey...)
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

func mustAuthorizedReq(ctx context.Context, sk, method, url, fileHash, contentType string, body io.Reader, masterKey ...string) *http.Response {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		log.Panic().Err(err).Msg("failed to create request")
	}

	req.Header.Set("Content-Type", contentType)
	auth, err := nip98.GenerateAuthHeader(sk, method, fileHash, req.URL, masterKey...)
	if err != nil {
		log.Panic().Err(err).Msg("failed to generate auth header")
	}
	req.Header.Set("Authorization", auth)

	client := http.Client{
		Transport: &http2.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Panic().Err(err).Msg("failed to execute request")
	}
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
					{"summary", "b2b8cf9202b45dad7e137516bcf44b915ce30b39c3b294629a9b6b8fa1585292.png"},
					{"ox", "b2b8cf9202b45dad7e137516bcf44b915ce30b39c3b294629a9b6b8fa1585292"},
					{"m", "image/png"},
					{"size", "182744"},
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
					{"summary", "777d453395088530ce8de776fe54c3e5ace548381007b743e067844858962218.jpg"},
					{"ox", "777d453395088530ce8de776fe54c3e5ace548381007b743e067844858962218"},
					{"m", "image/png"},
					{"size", "415939"},
				},
				Content: "ice profile pic",
			},
		},
		"same content file1": {
			Status:        "success",
			Message:       "Upload successful.",
			ProcessingURL: "",
			Nip94Event: struct {
				Tags    nostr.Tags `json:"tags"`
				Content string     `json:"content"`
			}{
				Tags: nostr.Tags{
					{"summary", "c7fce3cad585a3110c96b34516df16362c99f6f32359d64ddf1a58c1710247d1.txt"},
					{"ox", "c7fce3cad585a3110c96b34516df16362c99f6f32359d64ddf1a58c1710247d1"},
					{"m", "text/plain"},
					{"size", "4"},
				},
				Content: "other text file same content",
			},
		},
		"same content file2": {
			Status:        "success",
			Message:       "Upload successful.",
			ProcessingURL: "",
			Nip94Event: struct {
				Tags    nostr.Tags `json:"tags"`
				Content string     `json:"content"`
			}{
				Tags: nostr.Tags{
					{"summary", "c7fce3cad585a3110c96b34516df16362c99f6f32359d64ddf1a58c1710247d1.txt"},
					{"ox", "c7fce3cad585a3110c96b34516df16362c99f6f32359d64ddf1a58c1710247d1"},
					{"m", "text/plain"},
					{"size", "4"},
				},
				Content: "other text file same content",
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
					{"summary", "982d9e3eb996f559e633f4d194def3761d909f5a3b647d1a851fead67c32c9d1.txt"},
					{"ox", "982d9e3eb996f559e633f4d194def3761d909f5a3b647d1a851fead67c32c9d1"},
					{"m", "text/plain"},
					{"size", "4"},
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
					{"summary", "fc613b4dfd6736a7bd268c8a0e74ed0d1c04a959f59dd74ef2874983fd443fc9.txt"},
					{"ox", "fc613b4dfd6736a7bd268c8a0e74ed0d1c04a959f59dd74ef2874983fd443fc9"},
					{"m", "text/plain"},
					{"size", "6"},
				},
				Content: "master's file",
			},
		},
		"to be deleted": {
			Status:        "success",
			Message:       "Upload successful.",
			ProcessingURL: "",
			Nip94Event: struct {
				Tags    nostr.Tags `json:"tags"`
				Content string     `json:"content"`
			}{
				Tags: nostr.Tags{
					{"summary", "aca3a138e07162a8565070575b3593ee2fef404d447162f5786d5cc645d82b7a.txt"},
					{"ox", "aca3a138e07162a8565070575b3593ee2fef404d447162f5786d5cc645d82b7a"},
					{"m", "text/plain"},
					{"size", "27"},
				},
				Content: "master's file",
			},
		},
		"video with cute cats": {
			Status:        "success",
			Message:       "Upload successful.",
			ProcessingURL: "",
			Nip94Event: struct {
				Tags    nostr.Tags `json:"tags"`
				Content string     `json:"content"`
			}{
				Tags: nostr.Tags{
					{"summary", "20492a4d0d84f8beb1767f6616229f85d44c2827b64bdbfb260ee12fa1109e0e.file"},
					{"ox", "20492a4d0d84f8beb1767f6616229f85d44c2827b64bdbfb260ee12fa1109e0e"},
					{"m", "application/octet-stream"},
					{"size", "104857600"},
				},
				Content: "master's file",
			},
		},
	}
	return expectedResponses[caption]
}

func initStorage(ctx context.Context, opts ...storage.Option) {
	transportOverride := http.DefaultClient.Transport
	http.DefaultClient.Transport = http.DefaultTransport
	storage.MustInit(ctx, opts...)
	http.DefaultClient.Transport = transportOverride
}

const benchParallelism = 100

func BenchmarkUploadFiles(b *testing.B) {
	if os.Getenv("CI") != "" {
		b.Skip()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	http.DefaultClient.Transport = &http2.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	meter := tachymeter.New(&tachymeter.Config{Size: b.N})
	b.ResetTimer()
	b.ReportAllocs()
	fmt.Println(b.N)
	b.SetParallelism(benchParallelism)
	keys := []string{}
	var keysMx sync.Mutex
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			var sk string
			keysMx.Lock()
			if rand.N(100) >= 70 && len(keys) > 0 {
				sk = keys[rand.N(len(keys))]
			}
			if sk == "" {
				sk = model.GeneratePrivateKey()
				keys = append(keys, sk)
			}
			keysMx.Unlock()
			dir, err := testdata.ReadDir(".testdata")
			require.NoError(b, err)
			fileName := dir[rand.IntN(len(dir))].Name()
			img, err := testdata.Open(filepath.Join(".testdata", fileName))
			require.NoError(b, err)
			defer img.Close()
			start := time.Now()
			resp, err := nip96.Upload(ctx, nip96.UploadRequest{
				Host:        "https://localhost:9910/files",
				File:        img,
				Filename:    uuid.NewString(),
				Caption:     "ice",
				SK:          sk,
				SignPayload: true,
			})
			require.NoError(b, err)
			meter.AddTime(time.Since(start))
			nip94Event := &model.Event{Event: nostr.Event{
				CreatedAt: nostr.Now(),
				Kind:      nostr.KindFileMetadata,
				Tags:      resp.Nip94Event.Tags,
			}}
			require.NoError(b, nip94Event.SignWithAlg(sk, model.SignAlgEDDSA, model.KeyAlgCurve25519))

			relay := nostr.NewRelay(ctx, "wss://localhost:9920/", nostr.WithSignatureChecker(func(e *nostr.Event) bool {
				return true
			}))
			require.NoError(b, relay.ConnectWithTLS(ctx, &tls.Config{InsecureSkipVerify: true}))
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

func helperCreateAttestations(t *testing.T, ctx context.Context, master, masterPubKey string, usrPubkey ...string) {
	t.Helper()

	var ev model.Event
	ev.Kind = model.CustomIONKindAttestation
	ev.CreatedAt = 1
	ev.Tags = model.Tags{}
	for _, usr := range usrPubkey {
		ev.Tags = append(ev.Tags, model.Tag{model.TagAttestationName, usr, "", model.CustomIONAttestationKindActive + ":1"})
	}
	require.NoError(t, ev.SignWithAlg(master, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, query.AcceptEvents(ctx, &ev))
	relaysList := &model.Event{Event: nostr.Event{
		CreatedAt: nostr.Now(),
		Kind:      nostr.KindRelayListMetadata,
		Tags: model.Tags{
			{model.CustomIONTagOnBehalfOf, masterPubKey},
			{"r", "wss://localhost:9996"},
		},
	}}
	require.NoError(t, relaysList.SignWithAlg(master, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	require.NoError(t, query.AcceptEvents(ctx, relaysList))
}
