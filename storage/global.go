// SPDX-License-Identifier: ice License 1.0

package storage

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hashicorp/go-multierror"
	"github.com/nbd-wtf/go-nostr"
	"github.com/syndtr/goleveldb/leveldb"
	ldbstorage "github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/xssnick/tonutils-go/adnl"
	"github.com/xssnick/tonutils-go/adnl/dht"
	"github.com/xssnick/tonutils-go/liteclient"
	"github.com/xssnick/tonutils-storage/db"
	"github.com/xssnick/tonutils-storage/storage"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/storage/statistics"
)

var globalClient *client

const DefaultConfigUrl = "https://ton.org/global.config.json"

var ConcurrentBagsDownloading = runtime.NumCPU() * 10

const threadsPerBagForDownloading = 7

func Client() StorageClient {
	return globalClient
}

func AcceptEvents(ctx context.Context, events ...*model.Event) error {
	var acceptErrors multierror.Error

	for _, event := range events {
		switch event.Kind {
		case nostr.KindFileMetadata:
			multierror.Append(&acceptErrors, errors.Wrapf(acceptNewBag(ctx, event), "failed to accept new bag %v", event))

		case nostr.KindDeletion:
			if kTag := event.Tags.GetFirst([]string{"k"}); kTag != nil && len(*kTag) > 1 {
				if kTag.Value() == strconv.FormatInt(int64(nostr.KindFileMetadata), 10) {
					multierror.Append(&acceptErrors, errors.Wrapf(acceptDeletion(ctx, event), "failed to accept deletion %v", event))
				}
			}

		}
	}

	return acceptErrors.ErrorOrNil()
}

func acceptDeletion(ctx context.Context, event *model.Event) error {
	refs, err := model.ParseEventReference(event.Tags)
	if err != nil {
		return errors.Wrapf(err, "failed to detect events for delete")
	}
	filters := model.Filters{}
	for _, r := range refs {
		filters = append(filters, r.Filter())
	}
	events := query.GetStoredEvents(ctx, &model.Subscription{Filters: filters})
	var originalEvent *model.Event
	for fileEvent, err := range events {
		if err != nil {
			return errors.Wrapf(err, "failed to query referenced deletion file event")
		}
		if fileEvent.Kind != nostr.KindFileMetadata {
			return errors.Errorf("event mismatch: event %v is %v not file metadata (%v)", fileEvent.ID, fileEvent.Kind, nostr.KindFileMetadata)
		}
		if fileEvent.PubKey != event.PubKey {
			return errors.Errorf("user mismatch: event %v is signed by %v not %v", fileEvent.ID, fileEvent.PubKey, event.PubKey)
		}
		originalEvent = fileEvent
		break
	}
	if originalEvent == nil {
		return nil
	}
	fileHash := ""
	if xTag := originalEvent.Tags.GetFirst([]string{"x"}); xTag != nil && len(*xTag) > 1 {
		fileHash = xTag.Value()
	} else {
		return errors.Errorf("malformed x tag in event %v", originalEvent.ID)
	}
	bag, err := globalClient.bagByUser(event.GetMasterPublicKey())
	if err != nil {
		return errors.Wrapf(err, "failed to get bagID for the user %v", event.GetMasterPublicKey())
	}
	if bag == nil {
		return errors.Errorf("bagID for user %v not found", event.GetMasterPublicKey())
	}
	file, err := globalClient.detectFile(bag, fileHash)
	if err != nil {
		return errors.Wrapf(err, "failed to detect file %v in bag %v", fileHash, hex.EncodeToString(bag.BagID))
	}
	userRoot, _ := globalClient.BuildUserPath(event.GetMasterPublicKey(), "")
	if err := os.Remove(filepath.Join(userRoot, file)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Wrapf(err, "failed to delete file %v", file)
	}
	if _, _, _, err := globalClient.StartUpload(ctx, event.PubKey, event.GetMasterPublicKey(), file, fileHash, nil); err != nil {
		return errors.Wrapf(err, "failed to rebuild bag with deleted file")
	}
	return nil
}

func MustInit(ctx context.Context, nodeKey ed25519.PrivateKey, tonConfigUrl, rootStorage string, externalAddress net.IP, port int, debug bool) {
	globalClient = mustInit(ctx, nodeKey, tonConfigUrl, rootStorage, externalAddress, port, debug)
}

func mustInit(ctx context.Context, nodeKey ed25519.PrivateKey, tonConfigUrl, rootStorage string, externalAddress net.IP, port int, debug bool) *client {
	storage.Logger = func(a ...any) {
		if debug {
			log.Println(a...)
		}
		if len(a) > 0 {
			if s, isStr := a[0].(string); isStr {
				if strings.Contains(strings.ToLower(s), "err") {
					log.Println(a)
				}
			}
		}
	}
	storage.DownloadThreads = threadsPerBagForDownloading
	adnl.Logger = func(v ...any) {}
	var lsCfg *liteclient.GlobalConfig
	u, err := url.Parse(tonConfigUrl)
	if err != nil {
		log.Panic(errors.Wrapf(err, "invalid ton config url: %v", tonConfigUrl))
	}
	if u.Scheme == "file" {
		lsCfg, err = liteclient.GetConfigFromFile(u.Path)
		if err != nil {
			log.Panic(errors.Wrapf(err, "failed to load ton network config from file: %v", u.Path))
		}
	} else {
		downloadConfigCtx, cancelDownloadConfig := context.WithTimeout(ctx, 30*time.Second)
		defer cancelDownloadConfig()
		lsCfg, err = liteclient.GetConfigFromUrl(downloadConfigCtx, tonConfigUrl)
		if err != nil {
			log.Panic(errors.Wrapf(err, "failed to load ton network config from url: %v", u.String()))
		}
	}

	gate := adnl.NewGateway(nodeKey)
	gate.SetExternalIP(externalAddress)
	if err = gate.StartServer(fmt.Sprintf(":%v", port)); err != nil {
		log.Panic(errors.Wrapf(err, "failed to start adnl gateway"))
	}
	dhtGate := adnl.NewGateway(nodeKey)
	if err = dhtGate.StartClient(); err != nil {
		log.Panic(errors.Wrapf(err, "failed to start dht"))
	}

	dhtClient, err := dht.NewClientFromConfig(dhtGate, lsCfg)
	if err != nil {
		log.Panic(errors.Wrapf(err, "failed to create dht client"))
	}
	srv := storage.NewServer(dhtClient, gate, nodeKey, true)
	conn := storage.NewConnector(srv)
	fStorage, err := ldbstorage.OpenFile(filepath.Join(rootStorage, "db"), false)
	if err != nil {
		log.Panic(errors.Wrapf(err, "failed to open leveldb storage %v", filepath.Join(rootStorage, "db")))
	}
	progressDb, err := leveldb.Open(fStorage, nil)
	if err != nil {
		log.Panic(errors.Wrapf(err, "failed to open leveldb"))
	}
	cl := &client{
		conn:              conn,
		db:                progressDb,
		server:            srv,
		gateway:           gate,
		dht:               dhtClient,
		rootStoragePath:   rootStorage,
		newFiles:          make(map[string]map[string]*FileMetaInput),
		newFilesMx:        &sync.RWMutex{},
		stats:             statistics.NewStatistics(rootStorage, debug),
		downloadQueue:     make(chan queueItem, 1000000),
		activeDownloads:   make(map[string]bool),
		activeDownloadsMx: &sync.RWMutex{},
		debug:             debug,
	}
	if debug {
		go cl.report(ctx)
	}
	loadMonitoringCh := make(chan *db.Event, 1000000)
	go func() {
		for ev := range loadMonitoringCh {
			if ev.Event == db.EventTorrentLoaded {
				if ev.Torrent != nil {
					if _, uploading := ev.Torrent.IsActive(); !uploading {
						if downloading := ev.Torrent.IsDownloadAll(); !downloading {
							bs, bsErr := cl.bootstrapForBag(ev.Torrent.BagID)
							if bsErr != nil {
								log.Printf("WARN: failed to find stored bootstrap for bag %v: %v", hex.EncodeToString(ev.Torrent.BagID), bsErr)
							}
							var usr string
							if ev.Torrent.Header != nil {
								var m *headerData
								m, err = cl.fileMeta(ev.Torrent)
								if err != nil {
									log.Printf("INFO:loading bag %v into queue but it is not resolved yet: %v", hex.EncodeToString(ev.Torrent.BagID), err)
								}
								if m != nil {
									usr = m.Master
								}
							}
							log.Printf("[STORAGE] INFO: bag %v not yet started before restart put it into queue", hex.EncodeToString(ev.Torrent.BagID))
							cl.downloadQueue <- queueItem{
								tor:       ev.Torrent,
								bootstrap: &bs,
								user:      &usr,
							}
						}
					}
				}
			}
		}
	}()
	progressStorage, err := db.NewStorage(progressDb, conn, true, loadMonitoringCh)
	if err != nil {
		log.Panic(errors.Wrapf(err, "failed to open storage"))
	}
	cl.progressStorage = progressStorage
	cl.server.SetStorage(progressStorage)
	cl.progressStorage.SetNotifier(nil)
	close(loadMonitoringCh)
	go cl.startDownloadsFromQueue()
	return cl
}
