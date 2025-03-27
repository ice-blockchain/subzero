// SPDX-License-Identifier: ice License 1.0

package storage

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"math"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
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

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/storage/statistics"
)

var (
	globalClient *client
	globalConfig *config
)

type (
	config struct {
		PrivateKey              string `yaml:"private-key"`
		IONStorageConfigURL     string `yaml:"ion-storage-config-url"`
		AbsoluteRootStoragePath string `yaml:"absolute-root-storage-path"`
		ExternalADNLAddress     string `yaml:"external-adnl-address"`
		ExternalADNLPort        int    `yaml:"external-adnl-port"`
		Debug                   bool   `yaml:"debug"`
		IONLibertyDisabled      bool   `yaml:"ion-liberty-disabled"`
		RelayURL                string `yaml:"relay-url"`
	}
)

var ConcurrentBagsDownloading = runtime.NumCPU() * 10

const threadsPerBagForDownloading = 7

func init() {
	db.CachedFDLimit = math.MaxInt64
}

func Client() StorageClient {
	return globalClient
}

func AcceptEvents(ctx context.Context, events ...*model.Event) error {
	var acceptErrors *multierror.Error

	for _, event := range events {
		switch event.Kind {
		case nostr.KindFileMetadata:
			acceptErrors = multierror.Append(acceptErrors, errors.Wrapf(acceptNewBag(ctx, event), "failed to accept new bag %v", event))

		case nostr.KindDeletion:
			if (len(event.Tags) == 0 || (len(event.Tags) == 1 && event.GetTag("b").Value() != "")) && event.GetMasterPublicKey() != "" {
				acceptErrors = multierror.Append(acceptErrors, errors.Wrapf(globalClient.DeleteUser(event.GetMasterPublicKey()), "failed to accept profile deletion %v", event))
			} else if len(event.Tags) > 1 {
				if kTag := event.Tags.GetFirst([]string{"k"}); kTag != nil && len(*kTag) > 1 {
					acceptErrors = multierror.Append(acceptErrors, errors.Wrapf(acceptDeletion(ctx, event), "failed to accept deletion %v", event))
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
			if fileEvent.GetTag("imeta") == nil {
				continue
			}
		}
		if fileEvent.GetMasterPublicKey() != event.GetMasterPublicKey() {
			return errors.Errorf("user mismatch: event %v is signed by %v not %v", fileEvent.ID, fileEvent.PubKey, event.PubKey)
		}
		originalEvent = fileEvent
		break
	}
	if originalEvent == nil {
		return nil
	}
	log.Printf("[STORAGE] INFO: ACCEPT FILE DELETION OF NIP-94 for user %v: %v, original event %v", event.GetMasterPublicKey(), event.String(), originalEvent.String())
	fileHashes := []string{}
	if xTag := originalEvent.Tags.GetFirst([]string{"x"}); originalEvent.Kind == nostr.KindFileMetadata && xTag != nil && len(*xTag) > 1 {
		fileHashes = append(fileHashes, xTag.Value())
	} else {
		imetas := originalEvent.Tags.GetAll([]string{"imeta"})
		for _, imeta := range imetas {
			imetaValues, err := model.ParseIMeta(imeta)
			if err != nil {
				return errors.Wrapf(err, "malformed imeta")
			}
			hash := imetaValues["ox"]
			if hash == "" {
				hash = imetaValues["x"]
			}
			if hash == "" {
				return errors.Errorf("malformed imeta: empty x, ox tags")
			}
			fileHashes = append(fileHashes, hash)
		}
	}
	var mErr *multierror.Error
	for _, fh := range fileHashes {
		mErr = multierror.Append(mErr, processEventDeletion(ctx, fh, originalEvent.GetMasterPublicKey(), originalEvent.PubKey))
	}
	return mErr.ErrorOrNil()
}

func processEventDeletion(ctx context.Context, fileHash, masterPubkey, pubkey string) error {
	bag, err := globalClient.bagByUser(masterPubkey)
	if err != nil {
		return errors.Wrapf(err, "failed to get bagID for the user %v", masterPubkey)
	}
	if bag == nil {
		return errors.Errorf("bagID for user %v not found", masterPubkey)
	}
	file, err := globalClient.detectFile(bag, fileHash)
	if err != nil {
		return errors.Wrapf(err, "failed to detect file %v in bag %v", fileHash, hex.EncodeToString(bag.BagID))
	}
	userRoot, _ := globalClient.BuildUserPath(masterPubkey, "")
	if err := os.Remove(filepath.Join(userRoot, file)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Wrapf(err, "failed to delete file %v", file)
	}
	bagID, _, _, err := globalClient.StartUpload(ctx, pubkey, masterPubkey, file, fileHash, nil)
	if err != nil {
		return errors.Wrapf(err, "failed to rebuild bag with deleted file")
	}
	log.Printf("[STORAGE] INFO: bag %x replaced by %v due to file deletion %+v", bag.BagID, bagID, fileHash)
	return nil
}

func MustInit(ctx context.Context) {
	globalConfig = cfg.MustGet[config]()
	globalClient = mustInit(ctx)
}

func mustInit(ctx context.Context) *client {
	storage.Logger = func(a ...any) {
		if globalConfig.Debug {
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
	u, err := url.Parse(globalConfig.IONStorageConfigURL)
	if err != nil {
		log.Panic(errors.Wrapf(err, "invalid ton config url: %v", globalConfig.IONStorageConfigURL))
	}
	if u.Scheme == "file" {
		lsCfg, err = liteclient.GetConfigFromFile(u.Path)
		if err != nil {
			log.Panic(errors.Wrapf(err, "failed to load ton network config from file: %v", u.Path))
		}
	} else {
		downloadConfigCtx, cancelDownloadConfig := context.WithTimeout(ctx, 30*time.Second)
		defer cancelDownloadConfig()
		lsCfg, err = liteclient.GetConfigFromUrl(downloadConfigCtx, globalConfig.IONStorageConfigURL)
		if err != nil {
			log.Panic(errors.Wrapf(err, "failed to load ton network config from url: %v", u.String()))
		}
	}
	privateKey, err := hex.DecodeString(globalConfig.PrivateKey)
	if err != nil {
		log.Panic(errors.Wrapf(err, "failed to decode private key as hex: %v", globalConfig.PrivateKey))
	}
	gate := adnl.NewGateway(privateKey)
	ip := net.ParseIP(globalConfig.ExternalADNLAddress)
	if ip == nil {
		log.Panic(errors.Errorf("invalid external-adnl-address: %v", globalConfig.ExternalADNLAddress))
	}
	gate.SetExternalIP(ip)
	if err = gate.StartServer(fmt.Sprintf(":%v", globalConfig.ExternalADNLPort)); err != nil {
		log.Panic(errors.Wrapf(err, "failed to start adnl gateway"))
	}
	dhtGate := adnl.NewGateway(privateKey)
	if err = dhtGate.StartClient(); err != nil {
		log.Panic(errors.Wrapf(err, "failed to start dht"))
	}

	dhtClient, err := dht.NewClientFromConfig(dhtGate, lsCfg)
	if err != nil {
		log.Panic(errors.Wrapf(err, "failed to create dht client"))
	}
	srv := storage.NewServer(dhtClient, gate, privateKey, true)
	conn := storage.NewConnector(srv)
	fStorage, err := ldbstorage.OpenFile(filepath.Join(globalConfig.AbsoluteRootStoragePath, "db"), false)
	if err != nil {
		log.Panic(errors.Wrapf(err, "failed to open leveldb storage %v", filepath.Join(globalConfig.AbsoluteRootStoragePath, "db")))
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
		rootStoragePath:   globalConfig.AbsoluteRootStoragePath,
		newFiles:          make(map[string]map[string]*FileMetaInput),
		newFilesMx:        &sync.RWMutex{},
		stats:             statistics.NewStatistics(globalConfig.AbsoluteRootStoragePath, globalConfig.Debug),
		downloadQueue:     make(chan queueItem, 1000000),
		activeDownloads:   make(map[string]bool),
		activeDownloadsMx: &sync.RWMutex{},
		debug:             globalConfig.Debug,
	}
	if globalConfig.Debug {
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
	progressStorage, err := db.NewStorage(progressDb, conn, true, true, loadMonitoringCh)
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

func DeleteExpiredFiles(ctx context.Context, events ...*model.Event) error {
	var err error
	for _, ev := range events {
		if ev.Kind != nostr.KindFileMetadata {
			continue
		}
		log.Printf("[STORAGE] DEBUG: FILE expired for user %v: %v", ev.GetMasterPublicKey(), ev.String())
		fileHash := ""
		if xTag := ev.Tags.GetFirst([]string{"x"}); ev.Kind == nostr.KindFileMetadata && xTag != nil && len(*xTag) > 1 {
			fileHash = xTag.Value()
		}
		if fileHash == "" {
			return errors.Errorf("malformed file event: no file hash, %v", ev.String())
		}
		err = processEventDeletion(ctx, fileHash, ev.GetMasterPublicKey(), ev.PubKey)
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, ErrNotFound) {
			err = nil
		}
		err = errors.Join(err, errors.Wrapf(err, "failed to delete files for expired event %+v", ev))
	}
	return errors.Wrapf(err, "failed to delete files for expired events")
}
