// SPDX-License-Identifier: ice License 1.0

package http

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	gomime "github.com/cubewise-code/go-mime"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/storage"
)

type (
	Uploader interface {
		Upload() gin.HandlerFunc
		NIP96Info() gin.HandlerFunc
		Download() gin.HandlerFunc
		Delete() gin.HandlerFunc
		ListFiles() gin.HandlerFunc
	}
)

//go:embed nip96.json
var nip96Info string

type storageHandler struct {
	storageClient storage.StorageClient
	auth          AuthClient
}

const mediaEndpointTimeout = 60 * time.Second
const maxUploadSize = 100 * 1024 * 1024
const mediaTypeAvatar = "avatar"
const mediaTypeBanner = "banner"

type (
	fileUpload struct {
		File *multipart.FileHeader `form:"file" formMultipart:"file" swaggerignore:"true"`
		// Optional.
		Caption string `form:"caption" formMultipart:"caption"`
		// Optional.
		Expiration  string `form:"expiration" formMultipart:"expiration" swaggerignore:"expiration"`
		Size        uint64 `form:"size" formMultipart:"size"`
		Alt         string `form:"alt" formMultipart:"alt"`
		MediaType   string `form:"media_type" formMultipart:"media_type"`
		ContentType string `form:"content_type" formMultipart:"content_type"`
		NoTransform string `form:"no_transform" formMultipart:"no_transform"`
	}
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
		authHeader := strings.TrimPrefix(gCtx.GetHeader("Authorization"), "Nostr ")
		token, authErr := s.auth.VerifyToken(gCtx, authHeader, now)
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

		var upload fileUpload
		if err := gCtx.ShouldBindWith(&upload, binding.FormMultipart); err != nil {
			log.Printf("ERROR: failed to bind multipart form: %v", errors.Wrap(err, "failed to bind multipart form"))
			gCtx.JSON(http.StatusBadRequest, uploadErr("invalid multipart data"))
			return
		}
		if upload.Size > maxUploadSize {
			gCtx.JSON(http.StatusRequestEntityTooLarge, uploadErr("file too large"))
			return
		}
		if upload.File == nil {
			gCtx.JSON(http.StatusBadRequest, uploadErr("file required"))
			return
		}
		if upload.MediaType != "" && upload.MediaType != mediaTypeAvatar && upload.MediaType != mediaTypeBanner {
			gCtx.JSON(http.StatusBadRequest, uploadErr(fmt.Sprintf("unsupported media type %v", upload.MediaType)))
			return
		}
		if upload.ContentType == "" {
			upload.ContentType = gomime.TypeByExtension(filepath.Ext(upload.File.Filename))
		}

		storagePath, _ := s.storageClient.BuildUserPath(token.MasterPubKey(), upload.ContentType)
		uploadingFilePath := filepath.Join(storagePath, upload.File.Filename)
		relativePath := upload.File.Filename
		if err := os.MkdirAll(filepath.Dir(uploadingFilePath), 0o755); err != nil {
			log.Printf("ERROR: %v", errors.Wrap(err, "failed to open temp file while processing upload"))
			gCtx.JSON(http.StatusInternalServerError, uploadErr("failed to open temporary file"))
			return
		}
		fileUploadTo, err := os.Create(uploadingFilePath)
		if err != nil {
			log.Printf("ERROR: %v", errors.Wrap(err, "failed to open temp file while processing upload"))
			gCtx.JSON(http.StatusInternalServerError, uploadErr("failed to open temporary file"))
			return
		}
		defer fileUploadTo.Close()
		mpFile, err := upload.File.Open()
		if err != nil {
			log.Printf("ERROR: %v", errors.Wrap(err, "failed to open upload file"))
			gCtx.JSON(http.StatusInternalServerError, uploadErr("failed to open upload file"))
			return
		}
		defer mpFile.Close()
		hashCalc := sha256.New()
		if _, err = io.Copy(fileUploadTo, io.TeeReader(mpFile, hashCalc)); err != nil {
			log.Printf("ERROR: %v", errors.Wrap(err, "failed to copy temp file while processing upload"))
			gCtx.JSON(http.StatusBadRequest, uploadErr("failed to store temporary file"))
			return
		}
		if err = fileUploadTo.Sync(); err != nil {
			log.Printf("ERROR: %v", errors.Wrap(err, "failed to copy temp file while processing upload"))
			gCtx.JSON(http.StatusBadRequest, uploadErr("failed to store temporary file"))
			return
		}
		hash := hashCalc.Sum(nil)
		hashHex := hex.EncodeToString(hash)
		if hashHex != token.ExpectedHash() {
			log.Printf("ERROR: endpoint authentification failed: %v", errors.Errorf("payload hash mismatch actual>%v token>%v", hashHex, token.ExpectedHash()))
			gCtx.JSON(http.StatusForbidden, uploadErr("Unauthorized"))
			os.Remove(uploadingFilePath)
			return
		}
		bagID, url, existed, err := s.storageClient.StartUpload(ctx, token.PubKey(), token.MasterPubKey(), relativePath, hex.EncodeToString(hash), &storage.FileMetaInput{
			Hash:      hash,
			Caption:   upload.Caption,
			Alt:       upload.Alt,
			CreatedAt: uint64(now.UnixNano()),
		})

		if err != nil {
			log.Printf("ERROR: failed to upload file: %v", errors.Wrap(err, "failed to upload file to ion storage"))
			gCtx.JSON(http.StatusInternalServerError, uploadErr("oops, error occured!"))
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
					nostr.Tag{"x", hashHex},
					nostr.Tag{"m", upload.ContentType},
					nostr.Tag{"i", bagID},
					nostr.Tag{"alt", upload.Alt},
					nostr.Tag{"size", strconv.FormatUint(uint64(upload.File.Size), 10)},
				},
				Content: upload.Caption,
			},
		})
		return
	}
}
func (s *storageHandler) Download() gin.HandlerFunc {
	return func(gCtx *gin.Context) {
		now := time.Now()
		authHeader := strings.TrimPrefix(gCtx.GetHeader("Authorization"), "Nostr ")
		token, authErr := s.auth.VerifyToken(gCtx, authHeader, now)
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
		url, err := s.storageClient.DownloadUrl(token.MasterPubKey(), file)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				gCtx.Status(http.StatusNotFound)
				return
			}
			log.Printf("ERROR: %v", errors.Wrap(err, "failed to build download url"))
			gCtx.JSON(http.StatusInternalServerError, uploadErr("oops, error occured!"))
			return
		}
		gCtx.Redirect(http.StatusFound, url)
	}
}
func (s *storageHandler) Delete() gin.HandlerFunc {
	return func(gCtx *gin.Context) {
		now := time.Now()
		ctx, cancel := context.WithTimeout(gCtx, mediaEndpointTimeout)
		defer cancel()
		authHeader := strings.TrimPrefix(gCtx.GetHeader("Authorization"), "Nostr ")
		token, authErr := s.auth.VerifyToken(gCtx, authHeader, now)
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
		if err := s.storageClient.Delete(token.PubKey(), token.MasterPubKey(), file); err != nil {
			log.Printf("ERROR: %v", errors.Wrap(err, "failed to delete file"))
			if errors.Is(err, storage.ErrNotFound) || errors.Is(err, storage.ErrForbidden) {
				gCtx.JSON(http.StatusForbidden, uploadErr("user do not own file"))
				return
			}
			gCtx.JSON(http.StatusInternalServerError, uploadErr("oops, error occured!"))
			return
		}
		gCtx.JSON(http.StatusOK, map[string]any{"status": "success", "message": "deleted"})
	}
}

func (s *storageHandler) ListFiles() gin.HandlerFunc {
	return func(gCtx *gin.Context) {
		now := time.Now()
		authHeader := strings.TrimPrefix(gCtx.GetHeader("Authorization"), "Nostr ")
		token, authErr := s.auth.VerifyToken(gCtx, authHeader, now)
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
			log.Printf("ERROR: %v", errors.Wrapf(err, "failed to list files for user %v", token.MasterPubKey()))
			gCtx.JSON(http.StatusInternalServerError, uploadErr("oops, error occured!"))
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

func uploadErr(message string) any {
	return map[string]any{"status": "error", "message": message}
}

func NewUploadHandler(ctx context.Context) Uploader {
	s := &storageHandler{storageClient: storage.Client(), auth: NewAuth()}
	return s
}
