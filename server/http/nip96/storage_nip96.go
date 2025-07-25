// SPDX-License-Identifier: ice License 1.0

package nip96

import (
	"context"
	_ "embed"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/nbd-wtf/go-nostr"
	tusd "github.com/tus/tusd/v2/pkg/handler"

	"github.com/ice-blockchain/subzero/server/http/nip11"
	"github.com/ice-blockchain/subzero/server/http/nip98"
	"github.com/ice-blockchain/subzero/storage"
)

type (
	Uploader interface {
		Upload() gin.HandlerFunc
		NIP96Info() gin.HandlerFunc
		Download() gin.HandlerFunc
		Delete() gin.HandlerFunc
		ListFiles() gin.HandlerFunc
		RootPath() string
		CrossRelayDownload() gin.HandlerFunc
		LargeFiles() http.Handler
	}
)

//go:embed nip96.json
var nip96Info string

type storageHandler struct {
	storageClient storage.StorageClient
	tus           *tusd.Handler
	tusStorage    interface {
		tusd.DataStore
		tusd.TerminaterDataStore
	}
	auth               nip98.AuthClient
	nip11Fetcher       nip11.Fetcher
	ionLibertyDisabled bool
}

const mediaEndpointTimeout = 60 * time.Second
const maxUploadSize = 1 * 1024 * 1024

type (
	fileUploadResponse struct {
		Status        string `json:"status"`
		Message       string `json:"message"`
		ProcessingURL string `json:"processing_url"`
		Nip94Event    struct {
			Tags    nostr.Tags `json:"tags"`
			Content string     `json:"content"`
		} `json:"nip94_event"`
	}
	listedFiles struct {
		Total uint32 `json:"total"`
		Page  uint32 `json:"page"`
		Files []struct {
			Tags      nostr.Tags `json:"tags"`
			Content   string     `json:"content"`
			CreatedAt uint64     `json:"created_at"`
		}
	}
)

func (storageHandler) NIP96Info() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Data(http.StatusOK, "application/json", []byte(nip96Info))
	}
}

func (s *storageHandler) Upload() gin.HandlerFunc {
	return func(gCtx *gin.Context) {
		now := time.Now()
		ctx, cancel := context.WithTimeout(gCtx, mediaEndpointTimeout)
		defer cancel()
		authHeader := nip98.GetAuthHeader(gCtx.GetHeader("Authorization"))
		token, authErr := s.auth.VerifyToken(&url.URL{
			Scheme:   "https",
			Host:     gCtx.Request.Host,
			Path:     gCtx.Request.URL.Path,
			RawQuery: gCtx.Request.URL.RawQuery,
			Fragment: gCtx.Request.URL.Fragment,
		}, gCtx.Request.Method, authHeader, now)
		if authErr != nil {
			log.Printf("ERROR: endpoint authentification failed: %v", errors.Wrap(authErr, "endpoint authentification failed"))
			gCtx.JSON(http.StatusUnauthorized, uploadErr("Unauthorized"))
			return
		}
		attestationValid := token.ValidateAttestation(ctx, nostr.KindFileMetadata, now)
		if attestationValid != nil {
			log.Printf("ERROR: on-behalf attestation failed: %v", errors.Wrap(attestationValid, "endpoint authentification failed"))
			gCtx.JSON(http.StatusForbidden, uploadErr("Forbidden: on-behalf attestation failed"))
			return
		}
		log.Printf("[STORAGE DURATION %v %v] validation1 %v, whole %v", token.MasterPubKey(), token.ExpectedHash(), time.Since(now), time.Since(now))
		hStart := time.Now()
		uploadingFilePath, input, hash, err := s.storageClient.SaveFile(ctx, now, token.MasterPubKey(), gCtx.Request, maxUploadSize)
		if err != nil {
			log.Printf("ERROR: failed to save temp file while processing upload %v", err)
			switch {
			case errors.Is(err, storage.ErrValidationFailed):
				gCtx.JSON(http.StatusBadRequest, uploadErr("failed validate upload request"))
				return
			case errors.Is(err, storage.ErrFileTooBig):
				gCtx.JSON(http.StatusRequestEntityTooLarge, uploadErr(fmt.Sprintf("file too large: %v", input.FileSize)))
				return
			}
			gCtx.JSON(http.StatusBadRequest, uploadErr("failed to store temporary file"))
			return
		}
		log.Printf("[STORAGE DURATION %v %v] SAVING HASHING TOOK %v, whole %v", token.MasterPubKey(), token.ExpectedHash(), time.Since(hStart), time.Since(now))
		hashHex := hex.EncodeToString(hash)
		if hashHex != token.ExpectedHash() {
			log.Printf("ERROR: endpoint authentification failed: %v", errors.Errorf("payload hash mismatch actual>%v token>%v", hashHex, token.ExpectedHash()))
			gCtx.JSON(http.StatusForbidden, uploadErr("Unauthorized"))
			os.Remove(uploadingFilePath)
			return
		}
		bagID, url, existed, err := s.storageClient.StartUpload(ctx, now, token.PubKey(), token.MasterPubKey(), input.Filename, hex.EncodeToString(hash), input)

		if err != nil {
			log.Printf("ERROR: failed to upload file: %v", errors.Wrap(err, "failed to upload file to ion storage"))
			gCtx.JSON(http.StatusInternalServerError, uploadErr("oops, error occurred!"))
			os.Remove(uploadingFilePath)
			return
		}
		resStatus := http.StatusCreated
		if existed {
			resStatus = http.StatusOK
		}
		gCtx.JSON(resStatus, fileUploadResponse{
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
		})
		return
	}
}

func (s *storageHandler) redirectToDistributedStorageUrl() gin.HandlerFunc {
	return func(gCtx *gin.Context) {
		now := time.Now()
		authHeader := nip98.GetAuthHeader(gCtx.GetHeader("Authorization"))
		token, authErr := s.auth.VerifyToken(&url.URL{
			Scheme:   "https",
			Host:     gCtx.Request.Host,
			Path:     gCtx.Request.URL.Path,
			RawQuery: gCtx.Request.URL.RawQuery,
			Fragment: gCtx.Request.URL.Fragment,
		}, gCtx.Request.Method, authHeader, now)
		if authErr != nil {
			log.Printf("ERROR: endpoint authentification failed: %v", errors.Wrap(authErr, "endpoint authentification failed"))
			gCtx.JSON(http.StatusUnauthorized, uploadErr("Unauthorized"))
			return
		}
		file := gCtx.Param("file")
		if strings.TrimSpace(file) == "" {
			gCtx.JSON(http.StatusBadRequest, uploadErr("filename is required"))
			return
		}
		masterPubkey := token.MasterPubKey()
		spl := strings.SplitN(file, ":", 2)
		if len(spl) == 2 {
			masterPubkey = spl[0]
			file = spl[1]
		}
		url, err := s.storageClient.DownloadUrl(masterPubkey, file)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				gCtx.Status(http.StatusNotFound)
				return
			}
			log.Printf("ERROR: failed to build download url %v", err)
			gCtx.JSON(http.StatusInternalServerError, uploadErr("oops, error occurred!"))
			return
		}
		gCtx.Redirect(http.StatusFound, url)
	}
}
func (s *storageHandler) serveFileFromStorage() gin.HandlerFunc {
	return func(gCtx *gin.Context) {
		file := gCtx.Param("file")
		if strings.TrimSpace(file) == "" {
			gCtx.JSON(http.StatusBadRequest, uploadErr("filename is required"))
			return
		}
		var masterPubkey string
		spl := strings.SplitN(file, ":", 2)
		if len(spl) == 2 {
			masterPubkey = spl[0]
			file = spl[1]
		}
		fileHash := file
		if strings.Contains(file, ".") {
			fileHash = strings.TrimSuffix(file, filepath.Ext(file))
		}
		filePath, err := s.storageClient.FilePath(masterPubkey, fileHash, filepath.Ext(file))
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				gCtx.Status(http.StatusNotFound)
				return
			}
			log.Printf("ERROR: failed to build download url %v", err)
			gCtx.JSON(http.StatusInternalServerError, uploadErr("oops, error occurred!"))
			return
		}
		gCtx.File(filePath)
	}
}

func (s *storageHandler) Download() gin.HandlerFunc {
	if s.ionLibertyDisabled {
		return s.serveFileFromStorage()
	}
	return s.redirectToDistributedStorageUrl()
}
func (s *storageHandler) Delete() gin.HandlerFunc {
	return func(gCtx *gin.Context) {
		now := time.Now()
		ctx, cancel := context.WithTimeout(gCtx, mediaEndpointTimeout)
		defer cancel()
		authHeader := nip98.GetAuthHeader(gCtx.GetHeader("Authorization"))
		token, authErr := s.auth.VerifyToken(&url.URL{
			Scheme:   "https",
			Host:     gCtx.Request.Host,
			Path:     gCtx.Request.URL.Path,
			RawQuery: gCtx.Request.URL.RawQuery,
			Fragment: gCtx.Request.URL.Fragment,
		}, gCtx.Request.Method, authHeader, now)
		if authErr != nil {
			log.Printf("ERROR: endpoint authentification failed: %v", errors.Wrap(authErr, "endpoint authentification failed"))
			gCtx.JSON(http.StatusUnauthorized, uploadErr("Unauthorized"))
			return
		}
		attestationValid := token.ValidateAttestation(ctx, nostr.KindFileMetadata, now)
		if attestationValid != nil {
			log.Printf("ERROR: on-behalf attestation failed: %v", errors.Wrap(attestationValid, "endpoint authentification failed"))
			gCtx.JSON(http.StatusForbidden, uploadErr("Forbidden: on-behalf attestation failed"))
			return
		}
		file := gCtx.Param("file")
		if strings.TrimSpace(file) == "" {
			gCtx.JSON(http.StatusBadRequest, uploadErr("filehash is required"))
			return
		}
		if err := s.storageClient.Delete(ctx, token.PubKey(), token.MasterPubKey(), file); err != nil {
			log.Printf("ERROR: failed to delete file %v %v", file, err)
			if errors.Is(err, storage.ErrNotFound) || errors.Is(err, storage.ErrForbidden) {
				gCtx.JSON(http.StatusForbidden, uploadErr("user do not own file"))
				return
			}
			gCtx.JSON(http.StatusInternalServerError, uploadErr("oops, error occurred!"))
			return
		}
		gCtx.JSON(http.StatusOK, map[string]any{"status": "success", "message": "deleted"})
	}
}

func (s *storageHandler) ListFiles() gin.HandlerFunc {
	return func(gCtx *gin.Context) {
		now := time.Now()
		authHeader := nip98.GetAuthHeader(gCtx.GetHeader("Authorization"))
		token, authErr := s.auth.VerifyToken(&url.URL{
			Scheme:   "https",
			Host:     gCtx.Request.Host,
			Path:     gCtx.Request.URL.Path,
			RawQuery: gCtx.Request.URL.RawQuery,
			Fragment: gCtx.Request.URL.Fragment,
		}, gCtx.Request.Method, authHeader, now)
		if authErr != nil {
			log.Printf("ERROR: endpoint authentification failed: %v", errors.Wrap(authErr, "endpoint authentification failed"))
			gCtx.JSON(http.StatusUnauthorized, uploadErr("Unauthorized"))
			return
		}
		var params struct {
			Page  uint32 `form:"page"`
			Count uint32 `form:"count"`
		}
		if err := gCtx.ShouldBindWith(&params, binding.Query); err != nil {
			log.Printf("ERROR: failed to bind data : %v", errors.Wrap(err, "failed to bind data"))
			gCtx.JSON(http.StatusBadRequest, uploadErr("invalid data"))
			return
		}
		if params.Count == 0 {
			params.Count = 10
		}
		total, filesList, err := s.storageClient.ListFiles(token.MasterPubKey(), params.Page, params.Count)
		if err != nil {
			log.Printf("ERROR: failed to list files for user %v %v", token.MasterPubKey(), err)
			gCtx.JSON(http.StatusInternalServerError, uploadErr("oops, error occurred!"))
			return
		}
		res := &listedFiles{
			Total: total,
			Page:  params.Page,
			Files: []struct {
				Tags      nostr.Tags `json:"tags"`
				Content   string     `json:"content"`
				CreatedAt uint64     `json:"created_at"`
			}{},
		}
		for _, f := range filesList {
			res.Files = append(res.Files, struct {
				Tags      nostr.Tags `json:"tags"`
				Content   string     `json:"content"`
				CreatedAt uint64     `json:"created_at"`
			}{Tags: f.ToTags(), Content: f.Content, CreatedAt: f.CreatedAt})
		}
		gCtx.JSON(http.StatusOK, res)
	}
}

func (s *storageHandler) RootPath() string {
	return s.storageClient.RootPath()
}

func (s *storageHandler) CrossRelayDownload() gin.HandlerFunc {
	return func(gCtx *gin.Context) {
		now := time.Now()
		ctx, cancel := context.WithTimeout(gCtx, mediaEndpointTimeout)
		defer cancel()
		authHeader := nip98.GetAuthHeader(gCtx.GetHeader("Authorization"))
		token, authErr := s.auth.VerifyToken(&url.URL{
			Scheme:   "https",
			Host:     gCtx.Request.Host,
			Path:     gCtx.Request.URL.Path,
			RawQuery: gCtx.Request.URL.RawQuery,
			Fragment: gCtx.Request.URL.Fragment,
		}, gCtx.Request.Method, authHeader, now)
		if authErr != nil {
			log.Printf("ERROR: endpoint authentification failed: %v", errors.Wrap(authErr, "endpoint authentification failed"))
			gCtx.JSON(http.StatusUnauthorized, uploadErr("Unauthorized"))
			return
		}
		senderUrl := gCtx.GetHeader("Referer")
		if senderUrl == "" {
			gCtx.JSON(http.StatusBadRequest, uploadErr("unknown sender"))
			return
		}
		senderNIP11, err := s.nip11Fetcher.Fetch(ctx, senderUrl)
		if err != nil {
			log.Printf("ERROR: endpoint authentification failed: %v", errors.Wrap(err, "failed to fetch sender NIP11"))
			gCtx.JSON(http.StatusInternalServerError, uploadErr("oops, error occurred"))
			return
		}
		if token.PubKey() != senderNIP11.PubKey {
			log.Printf("ERROR: endpoint authentification failed: sender pubkey mismatch nip11>%v, token>%v", senderNIP11.PubKey, token.PubKey())
			gCtx.JSON(http.StatusUnauthorized, uploadErr("sender pubkey mismatch"))
			return
		}
		file := gCtx.Param("file")
		spl := strings.SplitN(file, ":", 2)
		var masterPubkey string
		if len(spl) == 2 {
			masterPubkey = spl[0]
			file = spl[1]
		}
		if err = storage.VerifyFileOwnershipAndAttestationForFileReplication(ctx, now, file, masterPubkey, senderUrl); err != nil {
			log.Printf("ERROR: not owning the file: %v %v user %v req from %v", err, file, masterPubkey, senderUrl)
			gCtx.JSON(http.StatusConflict, uploadErr("relay does not own the file"))
			return
		}
		var params struct {
			I string `form:"i"`
		}
		if err := gCtx.ShouldBindWith(&params, binding.Query); err != nil {
			log.Printf("ERROR: failed to bind data : %v", errors.Wrap(err, "failed to bind data"))
			gCtx.JSON(http.StatusBadRequest, uploadErr("invalid data"))
			return
		}
		if params.I == "" {
			gCtx.JSON(http.StatusBadRequest, uploadErr("invalid data: i tag not passed"))
			return
		}
		if err := s.storageClient.StartDownloadNewBag(ctx, file, masterPubkey, params.I); err != nil {
			log.Printf("ERROR: failed to accept new info hash %v for %v: %v", params.I, masterPubkey, err)
			gCtx.JSON(http.StatusInternalServerError, uploadErr("oops, error occurred!"))
			return
		}
		gCtx.Status(http.StatusAccepted)
	}
}

func (s *storageHandler) LargeFiles() http.Handler {
	return s.tus
}

func uploadErr(message string) any {
	return map[string]any{"status": "error", "message": message}
}

func NewUploadHandler(ctx context.Context, ionLibertyDisabled bool, fetcher nip11.Fetcher) Uploader {
	s := &storageHandler{storageClient: storage.Client(), auth: nip98.NewAuth(), ionLibertyDisabled: ionLibertyDisabled, nip11Fetcher: fetcher}
	tus, tusStorage := mustNewTusHandler(ctx, s)
	s.tus = tus
	s.tusStorage = tusStorage
	return s
}
