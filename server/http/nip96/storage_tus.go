// SPDX-License-Identifier: ice License 1.0

package nip96

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	gomime "github.com/cubewise-code/go-mime"
	"github.com/nbd-wtf/go-nostr"
	"github.com/tus/tusd/v2/pkg/filelocker"
	"github.com/tus/tusd/v2/pkg/filestore"
	tusd "github.com/tus/tusd/v2/pkg/handler"

	"github.com/ice-blockchain/subzero/server/http/nip98"
	"github.com/ice-blockchain/subzero/storage"
)

type tusHooks interface {
	PreUploadCreateCallback(hook tusd.HookEvent) (tusd.HTTPResponse, tusd.FileInfoChanges, error)
	PreFinishResponseCallback(hook tusd.HookEvent) (tusd.HTTPResponse, error)
	RootPath() string
}

func (s *storageHandler) PreUploadCreateCallback(hook tusd.HookEvent) (tusd.HTTPResponse, tusd.FileInfoChanges, error) {
	now := time.Now()
	token := nip98.DetectAuthHeader(hook.HTTPRequest.Header.Get("Authorization"))
	uri, err := url.Parse(hook.HTTPRequest.URI)
	if err != nil {
		return tusd.HTTPResponse{}, tusd.FileInfoChanges{}, errors.Wrapf(err, "failed to parse url: %v", hook.HTTPRequest.URI)
	}
	if uri.Host == "" {
		uri.Host = hook.HTTPRequest.Header.Get("Host")
	}
	if uri.Scheme == "" {
		uri.Scheme = "https"
	}
	tok, err := s.auth.VerifyToken(uri, hook.HTTPRequest.Method, token, now)
	if err != nil {
		log.Printf("ERROR: endpoint authentification failed: %v", errors.Wrap(err, "endpoint authentification failed"))
		errResp := tusd.HTTPResponse{
			StatusCode: http.StatusUnauthorized,
			Body:       `{"status":"error", "message":"Unauthorized"}`,
		}
		hook.Upload.StopUpload(errResp)
		return errResp, tusd.FileInfoChanges{}, nil
	}
	attestationValid := tok.ValidateAttestation(hook.Context, nostr.KindFileMetadata, now)
	if attestationValid != nil {
		log.Printf("ERROR: on-behalf attestation failed: %v", errors.Wrap(attestationValid, "endpoint authentification failed"))
		errResp := tusd.HTTPResponse{
			StatusCode: http.StatusUnauthorized,
			Body:       `{"status":"error", "message":"on-behalf attestation failed"}`,
		}
		hook.Upload.StopUpload(errResp)
		return errResp, tusd.FileInfoChanges{}, nil
	}
	mediaType := hook.Upload.MetaData["mediaType"]
	if mediaType != "" && mediaType != storage.MediaTypeAvatar && mediaType != storage.MediaTypeBanner {
		errResp := tusd.HTTPResponse{
			StatusCode: http.StatusBadRequest,
			Body:       `{"status":"error", "message":"failed validate upload request"}`,
		}
		hook.Upload.StopUpload(errResp)
		return errResp, tusd.FileInfoChanges{}, nil
	}
	hook.Upload.MetaData["master"] = tok.MasterPubKey()
	hook.Upload.MetaData["user"] = tok.PubKey()
	hook.Upload.MetaData["expectedHash"] = tok.ExpectedHash()
	if hook.Upload.IsFinal {
		if hook.Upload.MetaData["fileName"] == "" {
			errResp := tusd.HTTPResponse{
				StatusCode: http.StatusBadRequest,
				Body:       fmt.Sprintf(`{"status":"error", "message":"failed validate upload request: invalid filename %q"}`, hook.Upload.MetaData["fileName"]),
			}
			hook.Upload.StopUpload(errResp)
			return errResp, tusd.FileInfoChanges{}, nil
		}
		if hook.Upload.MetaData["contentType"] == "" {
			hook.Upload.MetaData["contentType"] = gomime.TypeByExtension(filepath.Ext(hook.Upload.MetaData["fileName"]))
		}
		if hook.Upload.Storage == nil {
			hook.Upload.Storage = make(map[string]string)
		}
		hook.Upload.Storage["Path"] = filepath.Join(tok.MasterPubKey(), fmt.Sprintf("%v%v", tok.ExpectedHash(), filepath.Ext(hook.Upload.MetaData["fileName"])))
	}
	return tusd.HTTPResponse{}, tusd.FileInfoChanges{MetaData: hook.Upload.MetaData, Storage: hook.Upload.Storage}, nil
}
func (s *storageHandler) PreFinishResponseCallback(hook tusd.HookEvent) (tusd.HTTPResponse, error) {
	if !hook.Upload.IsFinal {
		return tusd.HTTPResponse{}, nil
	}
	now := time.Now()
	token := nip98.DetectAuthHeader(hook.HTTPRequest.Header.Get("Authorization"))
	uri, err := url.Parse(hook.HTTPRequest.URI)
	if err != nil {
		return tusd.HTTPResponse{}, errors.Wrapf(err, "failed to parse url: %v", hook.HTTPRequest.URI)
	}
	if uri.Host == "" {
		uri.Host = hook.HTTPRequest.Header.Get("Host")
	}
	if uri.Scheme == "" {
		uri.Scheme = "https"
	}
	tok, err := s.auth.VerifyToken(uri, hook.HTTPRequest.Method, token, now)
	if err != nil {
		log.Printf("ERROR: endpoint authentification failed: %v", errors.Wrap(err, "endpoint authentification failed"))
		errResp := tusd.HTTPResponse{
			StatusCode: http.StatusUnauthorized,
			Body:       `{"status":"error", "message":"Unauthorized"}`,
		}
		hook.Upload.StopUpload(errResp)
		return errResp, nil
	}
	input := storage.FileMetaInput{
		Caption:     hook.Upload.MetaData["caption"],
		Alt:         hook.Upload.MetaData["alt"],
		Owner:       tok.PubKey(),
		CreatedAt:   uint64(now.UnixNano()),
		ContentType: hook.Upload.MetaData["contentType"],
		FileSize:    uint64(hook.Upload.Size),
		Filename:    hook.Upload.MetaData["fileName"],
	}
	filePath := hook.Upload.Storage["Path"]
	fileUploadTo, err := os.Open(filePath)
	if err != nil {
		return tusd.HTTPResponse{}, errors.Wrap(err, "failed to open file while processing upload")
	}
	fileSize := uint64(0)
	hashCalc := sha256.New()
	written, err := io.Copy(hashCalc, fileUploadTo)
	fileSize += uint64(written)
	if fileSize != input.FileSize {
		fileUploadTo.Close()
		os.Remove(filePath)
		return tusd.HTTPResponse{
			StatusCode: http.StatusPartialContent,
		}, errors.Wrap(err, "actual file size mismatch, not all chucks reached?")
	}
	defer fileUploadTo.Close()
	hash := hashCalc.Sum(nil)
	input.Hash = hash
	hashHex := hex.EncodeToString(input.Hash)
	if hashHex != tok.ExpectedHash() || hashHex != hook.Upload.MetaData["expectedHash"] {
		log.Printf("ERROR: endpoint authentification failed: %v", errors.Errorf("payload hash mismatch actual>%v token>%v", hashHex, tok.ExpectedHash()))
		errResp := tusd.HTTPResponse{
			StatusCode: http.StatusForbidden,
			Body:       `{"status":"error", "message":"Unauthorized"}`,
		}
		hook.Upload.StopUpload(errResp)
		return errResp, nil
	}
	input.Filename = hashHex + filepath.Ext(input.Filename)
	s.storageClient.SaveFile(context.WithValue(hook.Context, "fileName", input.Filename), now, tok.MasterPubKey(), nil, 0)
	bagID, url, existed, err := s.storageClient.StartUpload(hook.Context, now, tok.PubKey(), tok.MasterPubKey(), input.Filename, hashHex, &input)
	if err != nil {
		err = errors.Wrap(err, "failed to upload file to ion storage")
		log.Printf("ERROR: failed to upload file: %v", err)
		return tusd.HTTPResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       `{"status":"error", "message":"oops, something went wrong"}`,
			Header:     nil,
		}, err
	}
	for _, partID := range hook.Upload.PartialUploads {
		part, err := s.tusStorage.GetUpload(hook.Context, partID)
		if err != nil {
			log.Printf("ERROR: failed to get part %v: %v", partID, err)
			return tusd.HTTPResponse{
				StatusCode: http.StatusInternalServerError,
				Body:       `{"status":"error", "message":"oops, something went wrong"}`,
				Header:     nil,
			}, err
		}
		err = s.tusStorage.AsTerminatableUpload(part).Terminate(hook.Context)
		if err != nil {
			log.Printf("ERROR: failed to cleanup part %v: %v", partID, err)
			return tusd.HTTPResponse{
				StatusCode: http.StatusInternalServerError,
				Body:       `{"status":"error", "message":"oops, something went wrong"}`,
				Header:     nil,
			}, err
		}
	}
	resStatus := http.StatusCreated
	if existed {
		resStatus = http.StatusOK
	}
	result := fileUploadResponse{
		Status:  "success",
		Message: "Upload successful.",
		Nip94Event: struct {
			Tags    nostr.Tags `json:"tags"`
			Content string     `json:"content"`
		}{
			Tags: nostr.Tags{
				nostr.Tag{"url", url},
				nostr.Tag{"ox", hashHex},
				nostr.Tag{"m", input.ContentType},
				nostr.Tag{"i", bagID},
				nostr.Tag{"alt", input.Alt},
				nostr.Tag{"size", strconv.FormatUint(uint64(input.FileSize), 10)},
			},
			Content: input.Caption,
		},
	}
	b, err := json.Marshal(result)
	if err != nil {
		err = errors.Wrap(err, "failed to encode json")
		log.Printf("ERROR: failed to upload file: %v", err)
		return tusd.HTTPResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       `{"status":"error", "message":"oops, something went wrong"}`,
			Header:     nil,
		}, err
	}
	return tusd.HTTPResponse{
		StatusCode: resStatus,
		Body:       string(b),
	}, nil
}
func mustNewTusHandler(ctx context.Context, hooks tusHooks) (*tusd.Handler, interface {
	tusd.DataStore
	tusd.TerminaterDataStore
}) {
	store := filestore.New(hooks.RootPath())
	locker := filelocker.New(hooks.RootPath())
	composer := tusd.NewStoreComposer()
	store.UseIn(composer)
	locker.UseIn(composer)

	handler, err := tusd.NewHandler(tusd.Config{
		BasePath:                  "/xfiles/",
		StoreComposer:             composer,
		DisableDownload:           true,
		DisableTermination:        true,
		PreUploadCreateCallback:   hooks.PreUploadCreateCallback,
		PreFinishResponseCallback: hooks.PreFinishResponseCallback,
	})
	if err != nil {
		log.Fatalf("unable to create tus handler: %v", err)
	}
	return handler, store
}
