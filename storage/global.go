// SPDX-License-Identifier: ice License 1.0

package storage

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"math"
	"math/rand/v2"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/syndtr/goleveldb/leveldb"
	ldbstorage "github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/xssnick/tonutils-go/adnl"
	adnlAddress "github.com/xssnick/tonutils-go/adnl/address"
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
	globalClient struct {
		Client *client
		Once   sync.Once
	}
)

type (
	Config struct {
		PrivateKey              string `yaml:"private-key"`
		IONStorageConfigURL     string `yaml:"ion-storage-config-url"`
		AbsoluteRootStoragePath string `yaml:"absolute-root-storage-path"`
		ExternalADNLAddress     string `yaml:"external-adnl-address"`
		ExternalADNLPort        int    `yaml:"external-adnl-port"`
		Debug                   bool   `yaml:"debug"`
		IONLibertyDisabled      bool   `yaml:"ion-liberty-disabled"`
		RelayURL                string `yaml:"relay-url"`
	}
	Option     func(*client)
	acceptorFn func(ctx context.Context, fh, master, infohash string) error
)

var ConcurrentBagsDownloading = runtime.NumCPU() * 10

const threadsPerBagForDownloading = 7

const allowedTimeLagForFileReplication = 1 * time.Minute

func init() {
	db.CachedFDLimit = math.MaxInt64
}

func Client() StorageClient {
	return globalClient.Client
}

func AcceptEvents(ctx context.Context, events ...*model.Event) (err error) {
	return acceptEvents(ctx, globalClient.Client.StartDownloadNewBag, events...)
}

func acceptEvents(ctx context.Context, acceptor acceptorFn, events ...*model.Event) (err error) {
	for _, event := range events {
		switch event.Kind {
		case nostr.KindFileMetadata:
			err = errors.Join(err, errors.Wrapf(acceptNewBag(ctx, event, acceptor), "failed to accept new bag %v", event))

		case nostr.KindDeletion:
			if (len(event.Tags) == 0 || (len(event.Tags) == 1 && event.GetTag("b").Value() != "")) && event.GetMasterPublicKey() != "" {
				err = errors.Join(err, errors.Wrapf(globalClient.Client.DeleteUser(event.GetMasterPublicKey()), "failed to accept profile deletion %v", event))
			} else if len(event.Tags) > 1 {
				if kTag := event.Tags.GetFirst([]string{"k"}); kTag != nil && len(*kTag) > 1 {
					err = errors.Join(err, errors.Wrapf(acceptDeletion(ctx, event), "failed to accept deletion %v", event))
				}
			}
		case nostr.KindArticle, nostr.KindDraftArticle, model.CustomIONKindEditableTextNote:
			var val model.Timestamp
			if len(event.Content) < 1 && event.GetTag(model.CustomIONTagRichText) == nil {
				val, err = nostr.ParseTimestamp(event.GetTag("published_at").Value())
				softDelete := err == nil && event.CreatedAt.After(val)
				if softDelete {
					err = errors.Join(err, errors.Wrapf(acceptDeletion(ctx, event), "failed to accept deletion %v", event))
				}
			}
		}

	}

	return err
}

func ReplicateFileOnPeers(ctx context.Context, events ...*model.Event) (err error) {
	for _, event := range events {
		if event.Kind == nostr.KindFileMetadata {
			err = errors.Join(err, errors.Wrapf(acceptNewBag(ctx, event, globalClient.Client.triggerDownloadOnAllPeers(events...)), "failed to accept new bag %v", event))
		}
	}
	return err
}

func acceptDeletion(ctx context.Context, event *model.Event) error {
	var originalEvent *model.Event

	switch event.Kind {
	case nostr.KindDeletion:
		refs, err := model.ParseEventReference(event.Tags)
		if err != nil {
			return errors.Wrapf(err, "failed to detect events for delete")
		}
		filters := model.Filters{}
		for _, r := range refs {
			filters = append(filters, r.Filter())
		}
		events := query.GetStoredEvents(ctx, filters...)
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
	case nostr.KindArticle, nostr.KindDraftArticle, model.CustomIONKindEditableTextNote:
		originalEvent = event.Previous
	}
	if originalEvent == nil {
		return nil
	}
	log.Printf("[STORAGE] INFO: ACCEPT FILE DELETION OF NIP-94 for user %v: %v, original event %v", event.GetMasterPublicKey(), event.String(), originalEvent.String())
	fileHashes := map[string]string{}
	if xTag := originalEvent.GetTag("ox"); originalEvent.Kind == nostr.KindFileMetadata && xTag.Value() != "" {
		fileHashes[xTag.Value()] = originalEvent.GetTag("url").Value()
	} else {
		imetas := originalEvent.Tags.GetAll([]string{"imeta"})
		for _, imeta := range imetas {
			imetaValues, err := model.ParseIMeta(imeta)
			if err != nil {
				return errors.Wrapf(err, "malformed imeta")
			}
			hash := imetaValues["ox"]
			if hash == "" {
				return errors.Errorf("malformed imeta: empty x, ox tags")
			}
			fileHashes[hash] = imetaValues["url"]
		}
	}
	var err error
	var u *url.URL
	for fh := range fileHashes {
		u, err = url.Parse(fileHashes[fh])
		if err != nil {
			return errors.Wrapf(err, "failed to parse malformed url %v", fileHashes[fh])
		}
		ext := filepath.Ext(u.Path)
		deleteErr := processEventDeletion(ctx, fh, originalEvent.GetMasterPublicKey(), originalEvent.PubKey, ext)
		if errors.IsAny(deleteErr, os.ErrNotExist, ErrNotFound) {
			deleteErr = nil // File already deleted.
		}
		err = errors.Join(err, deleteErr)
	}
	return err
}

func processEventDeletion(ctx context.Context, fileHash, masterPubkey, pubkey, ext string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	it := query.GetStoredEvents(ctx,
		model.Filter{
			Kinds:     []int{nostr.KindFileMetadata},
			Authors:   []string{masterPubkey},
			Addresses: nil,
			Tags:      model.TagMap{}.Append("ox", &fileHash),
			Limit:     2,
		})
	count := int64(0)
	for _, err := range it {
		if err != nil {
			return errors.Wrapf(err, "failed to find deletable file hash %v", fileHash)
		}
		count += 1
	}
	if count >= 2 {
		return nil // Used by other posts
	}
	bag, _, err := globalClient.Client.bagByUser(masterPubkey)
	if err != nil {
		return errors.Wrapf(err, "failed to get bagID for the user %v", masterPubkey)
	}
	if bag == nil {
		return errors.Errorf("bagID for user %v not found", masterPubkey)
	}
	userRoot, _ := globalClient.Client.BuildUserPath(masterPubkey, "")
	file, err := globalClient.Client.detectFile(bag, fileHash)
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, errLatestBagNotDownloadedYet) {
			return os.Remove(filepath.Join(userRoot, fileHash+ext))
		}
		return errors.Wrapf(err, "failed to detect file %v in bag %v", fileHash, hex.EncodeToString(bag.BagID))
	}
	if err := os.Remove(filepath.Join(userRoot, file)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Wrapf(err, "failed to delete file %v", file)
	}
	bagID, _, _, err := globalClient.Client.StartUpload(ctx, time.Now(), pubkey, masterPubkey, file, fileHash, nil)
	if err != nil {
		return errors.Wrapf(err, "failed to rebuild bag with deleted file")
	}
	log.Printf("[STORAGE] INFO: bag %x replaced by %v due to file deletion %+v for user %v", bag.BagID, bagID, fileHash, masterPubkey)
	return nil
}

func WithConfig(cfg *Config) Option {
	return func(c *client) {
		if cfg == nil {
			log.Panicf("nil config passed to WithConfig")
		}
		c.config = cfg
	}
}

func MustInit(ctx context.Context, opts ...Option) {
	globalClient.Once.Do(func() {
		globalClient.Client = mustInit(ctx, opts...)
	})
	go func() {
		<-ctx.Done()
		if globalClient.Client == nil {
			return
		}
		globalClient.Client.Close()
		globalClient.Client = nil
		globalClient.Once = sync.Once{}
	}()
}

func mustInit(ctx context.Context, opts ...Option) *client {
	var cl = &client{
		newFiles:          make(map[string]map[string]*FileMetaInput),
		newFilesMx:        &sync.RWMutex{},
		downloadQueue:     make(chan queueItem, 1000000),
		activeDownloads:   make(map[string]bool),
		activeDownloadsMx: &sync.RWMutex{},
	}

	for _, opt := range opts {
		opt(cl)
	}

	if cl.config == nil {
		cl.config = cfg.MustGet[Config]()
	} else {
		if err := cfg.Validate(cl.config); err != nil {
			log.Panicf("failed to validate config: %v", err)
		}
	}

	storage.Logger = func(a ...any) {
		if cl.config.Debug {
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
	storage.DownloadPrefetch = threadsPerBagForDownloading
	adnl.Logger = func(v ...any) {}

	u, err := url.Parse(cl.config.IONStorageConfigURL)
	if err != nil {
		log.Panicf("invalid ton config url: %v: %v", cl.config.IONStorageConfigURL, err)
	}

	var lsCfg *liteclient.GlobalConfig
	if u.Scheme == "file" {
		lsCfg, err = liteclient.GetConfigFromFile(u.Path)
		if err != nil {
			log.Panicf("failed to load ton network config from file: %v: %v", u.Path, err)
		}
	} else {
		downloadConfigCtx, cancelDownloadConfig := context.WithTimeout(ctx, 30*time.Second)
		defer cancelDownloadConfig()
		lsCfg, err = liteclient.GetConfigFromUrl(downloadConfigCtx, cl.config.IONStorageConfigURL)
		if err != nil {
			log.Panicf("failed to load ton network config from url: %v: %v", u.String(), err)
		}
	}
	privateKey, err := hex.DecodeString(cl.config.PrivateKey)
	if err != nil {
		log.Panicf("failed to decode private key as hex: %v: %v", cl.config.PrivateKey, err)
	}
	cl.gateway = adnl.NewGateway(privateKey)
	ip := net.IPv4(127, 0, byte(rand.IntN(200))+1, byte(rand.IntN(200))+1) // Default to localhost.
	if cl.config.ExternalADNLAddress != "" {
		ip = net.ParseIP(cl.config.ExternalADNLAddress)
		if ip == nil {
			log.Panicf("invalid external-adnl-address: %v: %v", cl.config.ExternalADNLAddress, err)
		}
	}
	cl.gateway.SetAddressList([]*adnlAddress.UDP{
		{
			IP:   ip,
			Port: int32(cl.config.ExternalADNLPort),
		},
	})
	if err = cl.gateway.StartServer(fmt.Sprintf(":%v", cl.config.ExternalADNLPort), ConcurrentBagsDownloading*threadsPerBagForDownloading); err != nil {
		log.Panicf("failed to start adnl gateway: %v", err)
	}

	dhtGate := adnl.NewGateway(privateKey)
	if err = dhtGate.StartClient(ConcurrentBagsDownloading); err != nil {
		log.Panicf("failed to start dht: %v", err)
	}

	cl.dht, err = dht.NewClientFromConfig(dhtGate, lsCfg)
	if err != nil {
		log.Panicf("failed to create dht client: %v", err)
	}
	cl.server = storage.NewServer(cl.dht, cl.gateway, privateKey, true, runtime.NumCPU())
	cl.conn = storage.NewConnector(cl.server)
	fStorage, err := ldbstorage.OpenFile(filepath.Join(cl.config.AbsoluteRootStoragePath, "db"), false)
	if err != nil {
		log.Panicf("failed to open leveldb storage %v: %v", filepath.Join(cl.config.AbsoluteRootStoragePath, "db"), err)
	}
	cl.db, err = leveldb.Open(fStorage, nil)
	if err != nil {
		log.Panicf("failed to open leveldb storage: %v", err)
	}

	cl.rootStoragePath = cl.config.AbsoluteRootStoragePath
	cl.stats = statistics.NewStatistics(cl.rootStoragePath, cl.config.Debug)
	if cl.config.Debug {
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
	progressStorage, err := db.NewStorage(cl.db, cl.conn, db.Config{
		Notifier:   loadMonitoringCh,
		SkipVerify: true,
		NoRemove:   true,
	})
	if err != nil {
		log.Panicf("failed to create progress storage: %v", err)
	}
	cl.progressStorage = progressStorage
	cl.server.SetStorage(progressStorage)
	cl.progressStorage.SetNotifier(nil)
	close(loadMonitoringCh)
	go cl.startDownloadsFromQueue(ctx)
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
		ext := ""
		if xTag := ev.GetTag("ox"); ev.Kind == nostr.KindFileMetadata && xTag.Value() != "" {
			fileHash = xTag.Value()
		}
		if tag := ev.GetTag("url"); ev.Kind == nostr.KindFileMetadata && tag.Value() != "" {
			u, err := url.Parse(tag.Value())
			if err != nil {
				return errors.Wrapf(err, "failed to parse malformed url %v", tag.Value())
			}
			ext = filepath.Ext(u.Path)
		}
		if fileHash == "" {
			return errors.Errorf("malformed file event: no file hash, %v", ev.String())
		}
		err = processEventDeletion(ctx, fileHash, ev.GetMasterPublicKey(), ev.PubKey, ext)
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, ErrNotFound) {
			err = nil
		}
		err = errors.Join(err, errors.Wrapf(err, "failed to delete files for expired event %+v", ev))
	}
	return errors.Wrapf(err, "failed to delete files for expired events")
}

func (c *client) RootPath() string {
	return c.rootStoragePath
}

func (c *client) verifyFileOwnershipAndAttestationForFileReplication(ctx context.Context, now time.Time, fileHash, masterPubkey, senderUrl string) error {
	fileIt := query.GetStoredEvents(ctx,
		model.Filter{
			Kinds:   []int{nostr.KindFileMetadata},
			Authors: []string{masterPubkey},
			Tags:    model.TagMap{}.Append("ox", &fileHash),
		})
	var attestation, relays, file *model.Event
	for e, err := range fileIt {
		if err != nil {
			return errors.Wrapf(err, "failed to find events for master %v and file %v", masterPubkey, fileHash)
		}
		if e.Kind == nostr.KindFileMetadata && file == nil {
			file = e
		}
		break
	}
	if file == nil {
		return errors.Errorf("failed to verify file ownership, no file %v for user %v", fileHash, masterPubkey)
	}
	if now.After(file.CreatedAt.Time().Add(allowedTimeLagForFileReplication)) || now.Before(file.CreatedAt.Time().Add(-allowedTimeLagForFileReplication)) {
		return errors.Errorf("file expired, received %v, now %v", file.CreatedAt.Time().UnixNano(), now.UnixNano())
	}
	eventsIt := query.GetStoredEvents(ctx,
		model.Filter{
			Kinds:   []int{nostr.KindRelayListMetadata},
			Authors: []string{masterPubkey},
		},
		model.Filter{
			Kinds:   []int{model.CustomIONKindAttestation},
			Authors: []string{masterPubkey},
			Tags:    model.TagMap{}.SetLiterals("p", file.PubKey),
		})
	for e, err := range eventsIt {
		if err != nil {
			return errors.Wrapf(err, "failed to find events for master %v and file %v", masterPubkey, fileHash)
		}
		switch {
		case e.Kind == model.CustomIONKindAttestation && attestation == nil:
			attestation = e
		case e.Kind == nostr.KindRelayListMetadata && relays == nil:
			relays = e
		case e.Kind == nostr.KindFileMetadata && file == nil:
			file = e
		}
		if attestation != nil && file != nil && relays != nil {
			break
		}
	}
	if attestation == nil {
		return errors.Errorf("failed to verify file ownership, no attestation for user %v", masterPubkey)
	}
	if relays == nil {
		return errors.Errorf("failed to verify file ownership, no relays for user %v", masterPubkey)
	}
	allowed, err := model.OnBehalfIsAccessAllowed(attestation.Tags, file.PubKey, nostr.KindFileMetadata, file.CreatedAt)
	if err != nil {
		return errors.Wrapf(err, "failed to parse attestation event")
	}
	if !allowed {
		return errors.Wrapf(model.ErrOnBehalfAccessDenied, "kind %d", nostr.KindFileMetadata)
	}
	relaysList := model.CollectRelaysFromRelayEvent(relays)
	relaysValid := slices.Contains(relaysList, senderUrl) && slices.Contains(relaysList, c.config.RelayURL)
	if !relaysValid {
		return errors.Errorf("failed to verify file ownership, invalid relays %v %v for user %v", senderUrl, c.config.RelayURL, masterPubkey)
	}

	return nil
}

func VerifyFileOwnershipAndAttestationForFileReplication(ctx context.Context, now time.Time, fileHash, masterPubkey, senderUrl string) error {
	return globalClient.Client.verifyFileOwnershipAndAttestationForFileReplication(ctx, now, fileHash, masterPubkey, senderUrl)
}
