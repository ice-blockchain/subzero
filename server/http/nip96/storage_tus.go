// SPDX-License-Identifier: ice License 1.0

package nip96

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	gomime "github.com/cubewise-code/go-mime"
	"github.com/nbd-wtf/go-nostr"
	"github.com/rs/zerolog/log"
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
		log.Error().Err(err).Msg("endpoint authentification failed")
		errResp := tusd.HTTPResponse{
			StatusCode: http.StatusUnauthorized,
			Body:       `{"status":"error", "message":"Unauthorized"}`,
		}
		hook.Upload.StopUpload(errResp)
		return errResp, tusd.FileInfoChanges{}, tusd.Error{
			ErrorCode:    errResp.Body,
			HTTPResponse: errResp,
		}
	}
	attestationValid := tok.ValidateAttestation(hook.Context, nostr.KindFileMetadata, now)
	if attestationValid != nil {
		log.Error().Err(attestationValid).Msg("on-behalf attestation failed")
		errResp := tusd.HTTPResponse{
			StatusCode: http.StatusUnauthorized,
			Body:       `{"status":"error", "message":"on-behalf attestation failed"}`,
		}
		hook.Upload.StopUpload(errResp)
		return errResp, tusd.FileInfoChanges{}, tusd.Error{
			ErrorCode:    errResp.Body,
			HTTPResponse: errResp,
		}
	}
	mediaType := hook.Upload.MetaData["mediaType"]
	if mediaType != "" && mediaType != storage.MediaTypeAvatar && mediaType != storage.MediaTypeBanner {
		errResp := tusd.HTTPResponse{
			StatusCode: http.StatusBadRequest,
			Body:       `{"status":"error", "message":"failed validate upload request"}`,
		}
		hook.Upload.StopUpload(errResp)
		return errResp, tusd.FileInfoChanges{}, tusd.Error{
			ErrorCode:    errResp.Body,
			HTTPResponse: errResp,
		}
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
			return errResp, tusd.FileInfoChanges{}, tusd.Error{
				ErrorCode:    errResp.Body,
				HTTPResponse: errResp,
			}
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
		log.Error().Err(err).Msg("endpoint authentification failed")
		errResp := tusd.HTTPResponse{
			StatusCode: http.StatusUnauthorized,
			Body:       `{"status":"error", "message":"Unauthorized"}`,
		}
		hook.Upload.StopUpload(errResp)
		return errResp, tusd.Error{
			ErrorCode:    errResp.Body,
			HTTPResponse: errResp,
		}
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
		log.Error().Err(errors.Errorf("payload hash mismatch actual>%v token>%v", hashHex, tok.ExpectedHash())).Msg("endpoint authentification failed")
		errResp := tusd.HTTPResponse{
			StatusCode: http.StatusForbidden,
			Body:       `{"status":"error", "message":"Unauthorized"}`,
		}
		hook.Upload.StopUpload(errResp)
		return errResp, tusd.Error{
			ErrorCode:    errResp.Body,
			HTTPResponse: errResp,
		}
	}
	input.Filename = hashHex + filepath.Ext(input.Filename)
	s.storageClient.SaveFile(storage.WithFileNameInContext(hook.Context, input.Filename), now, tok.MasterPubKey(), nil, 0)
	bagID, url, existed, err := s.storageClient.StartUpload(hook.Context, now, tok.PubKey(), tok.MasterPubKey(), input.Filename, hashHex, &input)
	if err != nil {
		err = errors.Wrap(err, "failed to upload file to ion storage")
		log.Error().Err(err).Msg("failed to upload file")
		return tusd.HTTPResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       `{"status":"error", "message":"oops, something went wrong"}`,
			Header:     nil,
		}, err
	}
	for _, partID := range hook.Upload.PartialUploads {
		part, err := s.tusStorage.GetUpload(hook.Context, partID)
		if err != nil {
			log.Error().Err(err).Str("part_id", partID).Msg("failed to get part")
			return tusd.HTTPResponse{
				StatusCode: http.StatusInternalServerError,
				Body:       `{"status":"error", "message":"oops, something went wrong"}`,
				Header:     nil,
			}, err
		}
		err = s.tusStorage.AsTerminatableUpload(part).Terminate(hook.Context)
		if err != nil {
			log.Error().Err(err).Str("part_id", partID).Msg("failed to cleanup part")
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
			Content string     `json:"content"`
			Tags    nostr.Tags `json:"tags"`
		}{
			Tags: nostr.Tags{
				{"url", url},
				{"ox", hashHex},
				{"m", input.ContentType},
				{"i", bagID},
				{"alt", input.Alt},
				{"size", strconv.FormatUint(uint64(input.FileSize), 10)},
			},
			Content: input.Caption,
		},
	}
	b, err := json.Marshal(result)
	if err != nil {
		err = errors.Wrap(err, "failed to encode json")
		log.Error().Err(err).Msg("failed to upload file")
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
func mustNewTusHandler(_ context.Context, hooks tusHooks) (*tusd.Handler, interface {
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
		log.Fatal().Err(err).Msg("unable to create tus handler")
	}
	return handler, store
}
