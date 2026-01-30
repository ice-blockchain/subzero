// SPDX-License-Identifier: ice License 1.0

package nip11

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/appcontext"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/storage"
)

type (
	SystemStatusState string

	SystemStatus struct {
		EventsWrite SystemStatusState `json:"publishing_events"`
		EventsRead  SystemStatusState `json:"subscribing_for_events"`
		DVM         SystemStatusState `json:"dvm"`
		FilesWrite  SystemStatusState `json:"uploading_files"`
		FilesRead   SystemStatusState `json:"reading_files"`
		PushesSend  SystemStatusState `json:"sending_push_notifications"`
	}
)

const (
	SystemStatusStateOK          SystemStatusState = "UP"
	SystemStatusStateError       SystemStatusState = "DOWN"
	SystemStatusStateMaintenance SystemStatusState = "MAINTENANCE"

	forceDatabaseCheckInterval = 5 * time.Minute
	forceStorageCheckInterval  = 10 * time.Minute
)

func (n *nip11handler) collectStorageStatus(_ context.Context, systemReport *SystemStatus) (scheduleCheck bool) {
	if n.storageClient == nil {
		log.Error().Msg("storage client is nil, cannot collect storage status")
		return false
	}

	report := n.storageClient.Health()

	systemReport.FilesRead = SystemStatusStateOK
	systemReport.FilesWrite = SystemStatusStateOK

	if report.InReadErrorState {
		systemReport.FilesRead = SystemStatusStateError
	}

	if report.InWriteErrorState {
		systemReport.FilesWrite = SystemStatusStateError
	}

	scheduleCheck = time.Since(report.LastWrite) > forceStorageCheckInterval ||
		time.Since(report.LastRead) > forceStorageCheckInterval

	return scheduleCheck
}

func (n *nip11handler) collectDatabaseStatus(ctx context.Context, systemReport *SystemStatus) (scheduleCheck bool) {
	report, err := n.databaseReportGetter(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to collect database status")
		return false
	}

	systemReport.EventsRead = SystemStatusStateOK
	systemReport.EventsWrite = SystemStatusStateOK
	systemReport.DVM = SystemStatusStateOK

	if report.InReadErrorState {
		systemReport.EventsRead = SystemStatusStateError
	}

	if report.InWriteErrorState {
		systemReport.EventsWrite = SystemStatusStateError
	}

	if report.InReadErrorState || report.InWriteErrorState {
		systemReport.DVM = SystemStatusStateError
	}

	// Schedule a manual check if the last read or write was long ago.
	scheduleCheck = time.Since(report.LastWrite) > forceDatabaseCheckInterval ||
		time.Since(report.LastRead) > forceDatabaseCheckInterval

	return scheduleCheck
}

func createStorageUploadRequest() (req *http.Request, filename string, hash []byte, err error) {
	var body bytes.Buffer

	testFileName := "subzero_storage_healthcheck_" + strconv.FormatInt(time.Now().UnixNano(), 16) + ".txt"
	testFileContent := rand.Text()
	testFileHash := sha256.Sum256([]byte(testFileContent))

	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("file", testFileName)
	if err != nil {
		return nil, "", nil, errors.Wrap(err, "failed to create form file")
	}

	_, wErr := io.WriteString(part, testFileContent)
	cErr := writer.Close()
	if wErr != nil || cErr != nil {
		return nil, "", nil, errors.Wrap(errors.Join(wErr, cErr), "failed to write to form file or close writer")
	}

	req = &http.Request{
		Header: make(http.Header),
		Body:   io.NopCloser(&body),
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return req, testFileName, testFileHash[:], nil
}

func (n *nip11handler) RunStorageStatusCheck(ctx context.Context, systemReport *SystemStatus) {
	systemReport.FilesWrite = SystemStatusStateError
	systemReport.FilesRead = SystemStatusStateError

	if n.storageClient == nil {
		log.Error().Msg("storage client is nil, cannot run storage status check")
		return
	}

	req, filename, expectedHash, err := createStorageUploadRequest()
	if err != nil {
		log.Error().Err(err).Str("context", "storage health check").Msg("failed to create upload request")
		return
	}

	now := time.Now()
	_, masterPubKey := model.GenerateKeyPair()

	uploadPath, metaInput, hash, err := n.storageClient.SaveFile(ctx, now, masterPubKey, req, 1<<20)
	if err != nil {
		log.Error().Err(err).Str("context", "storage health check").Msg("failed to save file to storage")
		return
	} else if !bytes.Equal(hash, expectedHash) {
		log.Error().
			Hex("expected_hash", expectedHash).
			Hex("actual_hash", hash).
			Str("context", "storage health check").
			Msg("hash mismatch after saving file to storage")
		return
	}
	hashHex := hex.EncodeToString(hash)

	defer func() {
		deleteErr := n.storageClient.Delete(ctx, masterPubKey, masterPubKey, hashHex)
		if deleteErr != nil && !errors.Is(deleteErr, storage.ErrNotFound) {
			log.Error().Err(deleteErr).Str("context", "storage health check").Msg("failed to delete file from storage")
		}
	}()

	log.Trace().
		Str("file_path", uploadPath).
		Str("context", "storage health check").
		Hex("hash", hash).
		Msg("file saved to storage successfully")

	bagID, targetURL, _, err := n.storageClient.StartUpload(ctx, now, masterPubKey, masterPubKey, uploadPath, hashHex, metaInput)
	if err != nil {
		log.Error().Err(err).Str("context", "storage health check").Msg("failed to start upload to storage")
		return
	}

	log.Trace().
		Str("bag_id", bagID).
		Str("target_url", targetURL).
		Str("context", "storage health check").
		Msg("upload started successfully")

	systemReport.FilesWrite = SystemStatusStateOK

	fullPath, err := n.storageClient.FilePath(masterPubKey, hashHex, filepath.Ext(filename))
	if err != nil {
		log.Error().Err(err).Str("context", "storage health check").Msg("failed to get file path from storage client")
		return
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		log.Error().Err(err).Str("context", "storage health check").Msg("failed to read file from storage")
		return
	}

	actualHash := sha256.Sum256(data)
	if !bytes.Equal(actualHash[:], expectedHash) {
		log.Error().
			Hex("expected_hash", expectedHash).
			Hex("actual_hash", actualHash[:]).
			Str("context", "storage health check").
			Msg("hash mismatch after reading file from storage")
		return
	} else {
		log.Trace().
			Str("file_path", fullPath).
			Str("context", "storage health check").
			Hex("hash", actualHash[:]).
			Msg("file read from storage successfully with matching hash")
	}

	systemReport.FilesRead = SystemStatusStateOK
}

func (*nip11handler) RunDatabaseStatusCheck(ctx context.Context, systemReport *SystemStatus) {
	const numEvents = 3
	const testEventKind = 9998

	var events model.Events
	for i := range numEvents {
		var ev model.Event

		ev.Kind = testEventKind
		ev.CreatedAt = nostr.Now()
		ev.Content = "status check event " + strconv.Itoa(i)
		ev.Tags = model.Tags{
			{"expiration", ev.CreatedAt.Add(time.Minute * 2).String()},
		}
		ev.SignWithAlg(model.GeneratePrivateKey(), model.SignAlgEDDSA, model.KeyAlgCurve25519)

		events = append(events, &ev)
	}

	systemReport.EventsWrite = SystemStatusStateError
	systemReport.DVM = SystemStatusStateError
	systemReport.EventsRead = SystemStatusStateError

	err := query.AcceptEvents(ctx, events...)
	if err != nil {
		log.Error().Err(err).Msg("failed to write test events for database status check")
		return
	}

	systemReport.EventsWrite = SystemStatusStateOK

	var received model.Events
	for attempt := range numEvents {
		received = nil
		for ev, err := range query.GetStoredEvents(ctx, model.Filter{IDs: events.IDs()}) {
			if err != nil {
				log.Error().Err(err).Msg("failed to read test events for database status check")
				break
			}
			received = append(received, ev)
		}
		if len(received) == len(events) {
			break
		}

		// Trying again if we didn't get all events.
		log.Info().Msgf("database status check: attempt %d: expected %d events, got %d", attempt+1, len(events), len(received))
		if attempt < numEvents-1 { // No wait on last attempt.
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second * 10):
			}
		}
	}

	if len(received) == len(events) {
		systemReport.EventsRead = SystemStatusStateOK
		systemReport.DVM = SystemStatusStateOK
	}
}

func (n *nip11handler) startSystemStatusCollector(ctx context.Context, regularTicks chan struct{}) {
	var status = SystemStatus{
		EventsWrite: SystemStatusStateOK,
		EventsRead:  SystemStatusStateOK,
		DVM:         SystemStatusStateOK,
		FilesWrite:  SystemStatusStateOK,
		FilesRead:   SystemStatusStateOK,
		PushesSend:  SystemStatusStateOK,
	}

	defer appcontext.GetAppContext(ctx).Recover()

	if regularTicks == nil {
		// Use internal ticker if none provided.
		regularTicks = make(chan struct{}, 1)
		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()

			for ctx.Err() == nil {
				select {
				case <-ticker.C:
					select {
					case <-ctx.Done():
						return
					case regularTicks <- struct{}{}:
					default:
						log.Debug().Msg("skipping system status collection tick, previous tick still being processed")
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	databaseCheck := make(chan struct{}, 1)
	storageCheck := make(chan struct{}, 1)

	for ctx.Err() == nil {
		workingStatus := status // Working copy.

		select {
		case <-ctx.Done():
			return

		case <-regularTicks:
			scheduleDatabaseCheck := n.collectDatabaseStatus(ctx, &workingStatus)
			if scheduleDatabaseCheck {
				select {
				case databaseCheck <- struct{}{}:
				default:
					log.Debug().Msg("database status check already scheduled, skipping")
				}
			}

			scheduleStorageCheck := n.collectStorageStatus(ctx, &workingStatus)
			if scheduleStorageCheck {
				select {
				case storageCheck <- struct{}{}:
				default:
					log.Debug().Msg("storage status check already scheduled, skipping")
				}
			}

		case <-databaseCheck:
			log.Info().Msg("starting scheduled database status check")
			testCtx, testCancel := context.WithTimeout(ctx, time.Minute*3)
			n.RunDatabaseStatusCheck(testCtx, &workingStatus)
			testCancel()

		case <-storageCheck:
			log.Info().Msg("starting scheduled storage status check")
			testCtx, testCancel := context.WithTimeout(ctx, time.Minute*3)
			n.RunStorageStatusCheck(testCtx, &workingStatus)
			testCancel()
		}

		status = workingStatus
		n.systemStatus.Store(&status)
	}
}
