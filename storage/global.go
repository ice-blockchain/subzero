package storage

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/ice-blockchain/subzero/storage/statistics"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/gookit/goutil/errorx"
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
)

var globalClient *client

const DefaultConfigUrl = "https://ton.org/global.config.json"

func Client() StorageClient {
	return globalClient
}

func AcceptEvent(ctx context.Context, event *model.Event) error {
	if !(event.Kind == nostr.KindFileMetadata || event.Kind == nostr.KindDeletion) {
		return nil
	}
	if event.Kind == nostr.KindDeletion {
		matchKTag := false
		if kTag := event.Tags.GetFirst([]string{"k"}); kTag != nil && len(*kTag) > 1 {
			if kTag.Value() == strconv.FormatInt(int64(nostr.KindFileMetadata), 10) {
				matchKTag = true
			}
		}
		if matchKTag {
			return acceptDeletion(ctx, event)
		}
		return nil
	}
	return acceptNewBag(ctx, event)
}
func acceptDeletion(ctx context.Context, event *model.Event) error {
	refs, err := model.ParseEventReference(event.Tags)
	if err != nil {
		return errorx.Withf(err, "failed to detect events for delete")
	}
	filters := model.Filters{}
	for _, r := range refs {
		filters = append(filters, r.Filter())
	}
	events := query.GetStoredEvents(ctx, &model.Subscription{Filters: filters})
	var originalEvent *model.Event
	for fileEvent, err := range events {
		if err != nil {
			return errorx.Withf(err, "failed to query referenced deletion file event")
		}
		if fileEvent.Kind != nostr.KindFileMetadata {
			return errorx.Errorf("event mismatch: event %v is %v not file metadata (%v)", fileEvent.ID, fileEvent.Kind, nostr.KindFileMetadata)
		}
		if fileEvent.PubKey != event.PubKey {
			return errorx.Errorf("user mismatch: event %v is signed by %v not %v", fileEvent.ID, fileEvent.PubKey, event.PubKey)
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
		return errorx.Errorf("malformed x tag in event %v", originalEvent.ID)
	}
	bag, err := globalClient.bagByUser(event.PubKey)
	if err != nil {
		return errorx.Withf(err, "failed to get bagID for the user %v", event.PubKey)
	}
	file, err := globalClient.detectFile(bag, fileHash)
	if err != nil {
		return errorx.Withf(err, "failed to detect file %v in bag %v", fileHash, hex.EncodeToString(bag.BagID))
	}
	userRoot, _ := globalClient.BuildUserPath(event.PubKey, "")
	if err := os.Remove(filepath.Join(userRoot, file)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errorx.Withf(err, "failed to delete file %v", file)
	}
	if _, _, err := globalClient.StartUpload(ctx, event.PubKey, file, fileHash, nil); err != nil {
		return errorx.Withf(err, "failed to rebuild bag with deleted file")
	}
	return nil
}

func MustInit(ctx context.Context, nodeKey ed25519.PrivateKey, tonConfigUrl, rootStorage string, externalAddress net.IP, port int, debug bool) {
	storage.Logger = log.Println
	var lsCfg *liteclient.GlobalConfig
	u, err := url.Parse(tonConfigUrl)
	if err != nil {
		log.Panic(errorx.Wrapf(err, "invalid ton config url: %v", tonConfigUrl))
	}
	if u.Scheme == "file" {
		lsCfg, err = liteclient.GetConfigFromFile(u.Path)
		if err != nil {
			log.Panic(errorx.Wrapf(err, "failed to load ton network config from file: %v", u.Path))
		}
	} else {
		downloadConfigCtx, cancelDownloadConfig := context.WithTimeout(ctx, 30*time.Second)
		defer cancelDownloadConfig()
		lsCfg, err = liteclient.GetConfigFromUrl(downloadConfigCtx, tonConfigUrl)
		if err != nil {
			log.Panic(errorx.Wrapf(err, "failed to load ton network config from url: %v", u.String()))
		}
	}

	gate := adnl.NewGateway(nodeKey)
	gate.SetExternalIP(externalAddress)
	if err = gate.StartServer(fmt.Sprintf(":%v", port)); err != nil {
		log.Panic(errorx.Wrapf(err, "failed to start adnl gateway"))
	}
	dhtGate := adnl.NewGateway(nodeKey)
	if err = dhtGate.StartClient(); err != nil {
		log.Panic(errorx.Wrapf(err, "failed to start dht"))
	}

	dhtClient, err := dht.NewClientFromConfig(dhtGate, lsCfg)
	if err != nil {
		log.Panic(errorx.Wrapf(err, "failed to create dht client"))
	}
	srv := storage.NewServer(dhtClient, gate, nodeKey, true)
	conn := storage.NewConnector(srv)
	fStorage, err := ldbstorage.OpenFile(filepath.Join(rootStorage, "db"), false)
	if err != nil {
		log.Panic(errorx.Wrapf(err, "failed to open leveldb storage %v", filepath.Join(rootStorage, "db")))
	}
	progressDb, err := leveldb.Open(fStorage, nil)
	if err != nil {
		log.Panic(errorx.Wrapf(err, "failed to open leveldb"))
	}
	globalClient = &client{
		conn:            conn,
		db:              progressDb,
		server:          srv,
		gateway:         gate,
		dht:             dhtClient,
		events:          make(chan db.Event),
		rootStoragePath: rootStorage,
		newFiles:        make(map[string]map[string]*FileMeta),
		newFilesMx:      &sync.RWMutex{},
		stats:           statistics.NewStatistics(rootStorage, debug),
	}
	progressStorage, err := db.NewStorage(progressDb, conn, true, globalClient.events)
	if err != nil {
		log.Panic(errorx.Wrapf(err, "failed to open storage"))
	}
	globalClient.progressStorage = progressStorage
	globalClient.server.SetStorage(progressStorage)
}
