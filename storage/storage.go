// SPDX-License-Identifier: ice License 1.0

package storage

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
	gomime "github.com/cubewise-code/go-mime"
	"github.com/nbd-wtf/go-nostr/nip94"
	"github.com/rs/zerolog/log"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/xssnick/tonutils-go/adnl"
	"github.com/xssnick/tonutils-go/adnl/dht"
	"github.com/xssnick/tonutils-go/adnl/overlay"
	"github.com/xssnick/tonutils-storage/db"
	"github.com/xssnick/tonutils-storage/storage"

	"github.com/ice-blockchain/subzero/cmd/subzero-ion-connect/appcontext"
	"github.com/ice-blockchain/subzero/storage/internal"
	"github.com/ice-blockchain/subzero/storage/statistics"
)

type (
	StorageClient interface {
		io.Closer
		SaveFile(ctx context.Context, now time.Time, masterPubKey string, r *http.Request, maxSize uint64) (filePath string, metaInput *FileMetaInput, hash []byte, err error)
		StartUpload(ctx context.Context, now time.Time, userPubKey, masterKey, relativePathToFileForUrl, fileHash string, newFile *FileMetaInput) (bagID, url string, existed bool, err error)
		BuildUserPath(masterKey, contentType string) (string, string)
		RootPath() string
		DownloadUrl(masterKey, fileSha256 string) (string, error)
		FilePath(masterKey, fileSha256, ext string) (string, error)
		ListFiles(masterKey string, page, count uint32) (totalFiles uint32, files []*FileMetadata, err error)
		Delete(ctx context.Context, userPubkey, masterKey string, fileSha256 string) error
		DeleteUser(masterKey string) error
		StartDownloadNewBag(ctx context.Context, fileHash, masterKey, infohash string) error
	}
	Bootstrap struct {
		Overlay *overlay.Node
		DHT     *dht.Node
	}
	headerData struct {
		FileMetadata map[string]*FileMetaInput `json:"f"`
		FileHash     map[string]string         `json:"fh"`
		Master       string                    `json:"m"`
	}
	FileMetaInput struct {
		Caption     string `json:"c"`
		Alt         string `json:"a"`
		Owner       string `json:"o"`
		ContentType string `json:"-"`
		Filename    string `json:"-"`
		Hash        []byte `json:"h"`
		CreatedAt   uint64 `json:"cAt"`
		FileSize    uint64 `json:"-"`
	}
	FileMetadata struct {
		*nip94.FileMetadata
		CreatedAt uint64 `json:"created_at"`
	}
	client struct {
		stats             statistics.Statistics
		progressStorage   *db.Storage
		server            *storage.Server
		conn              *storage.Connector
		gateway           *adnl.Gateway
		dht               *dht.Client
		newFiles          map[string]map[string]*FileMetaInput
		newFilesMx        *sync.RWMutex
		db                *leveldb.DB
		downloadQueue     chan queueItem
		activeDownloads   map[string]bool
		activeDownloadsMx *sync.RWMutex
		config            *Config
		rootStoragePath   string
		closed            atomic.Bool
		cdn               internal.CDNClient
	}
	queueItem struct {
		tor       *storage.Torrent
		bootstrap *string
		user      *string
		version   int64
	}
)

var (
	ErrNotFound                  = storage.ErrFileNotExist
	ErrForbidden                 = errors.New("forbidden")
	ErrNoRelays                  = errors.New("no relays")
	ErrFileTooBig                = errors.New("too big")
	ErrValidationFailed          = errors.New("validation failed")
	errLatestBagNotDownloadedYet = errors.New("no header fetched yet")
)

const MediaTypeAvatar = "avatar"
const MediaTypeBanner = "banner"

func (c *client) fileMeta(bag *storage.Torrent) (*headerData, error) {
	var desc headerData
	var hData []byte
	if bag.Header == nil {
		var err error
		hData, err = c.latestHeaderForBag(bag.BagID)
		if err != nil {
			return nil, errors.Wrapf(err, "No header fetched yet for %v and failed to get stored", hex.EncodeToString(bag.BagID))
		}
		if hData == nil {
			return nil, errors.Wrapf(errLatestBagNotDownloadedYet, "No header fetched yet for %v", hex.EncodeToString(bag.BagID))
		}
	} else {
		hData = bag.Header.Data
	}

	if len(hData) == 0 {
		hData = []byte("{}")
	}
	if err := json.Unmarshal(hData, &desc); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal bag header data: %v", string(hData))
	}
	return &desc, nil
}

func (c *client) detectFile(bag *storage.Torrent, fileHash string) (string, error) {
	metadata, err := c.fileMeta(bag)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse bag header data %v", hex.EncodeToString(bag.BagID))
	}
	return c.detectFileFromMeta(bag, metadata, fileHash)
}

func (c *client) detectFileFromMeta(bag *storage.Torrent, metadata *headerData, fileHash string) (string, error) {
	name, exists := metadata.FileHash[fileHash]
	if !exists {
		mdBytes, err := c.latestHeaderForBag(bag.BagID)
		if err != nil {
			return "", storage.ErrFileNotExist
		}
		if err := json.Unmarshal(mdBytes, metadata); err != nil {
			return "", errors.Wrap(err, "failed to unmarshal bag header data")
		}
		name, exists = metadata.FileHash[fileHash]
		if !exists {
			return "", storage.ErrFileNotExist
		}
	}
	f, err := bag.GetFileOffsets(name)
	if err != nil {
		return "", errors.Wrapf(err, "failed to locate file %v in bag %v", name, hex.EncodeToString(bag.BagID))
	}
	return f.Name, nil
}

func (c *client) bagByUser(userPubKey string) (*storage.Torrent, int64, error) {
	var version int64
	k := make([]byte, 3+64)
	copy(k, "ub:")
	copy(k[3:], userPubKey)
	var bagID []byte
	bagIDAndVersion, err := c.db.Get(k, nil)
	if len(bagIDAndVersion) > 0 {
		bagID = bagIDAndVersion[:32]
		versionStr := string(bagIDAndVersion[32:])
		if versionStr != "" {
			version, err = strconv.ParseInt(versionStr, 10, 64)
			if err != nil {
				return nil, 0, errors.Wrapf(err, "failed to read userID:bag mapping, version is %v, invalid", versionStr)
			}
		}
	}
	if err != nil && !errors.Is(err, leveldb.ErrNotFound) {
		return nil, 0, errors.Wrap(err, "failed to read userID:bag mapping")
	}
	tr := c.progressStorage.GetTorrent(bagID)
	if version == 0 && tr != nil && tr.Header != nil {
		version = int64(tr.Header.FilesCount)
	}
	return tr, version, nil
}
func (c *client) bootstrapForBag(bagID []byte) (string, error) {
	k := make([]byte, 3+32)
	copy(k, "bs:")
	copy(k[3:], bagID)
	bs, err := c.db.Get(k, nil)
	if err != nil && !errors.Is(err, leveldb.ErrNotFound) {
		return "", errors.Wrapf(err, "failed to read stored bootstrap node for %v =, will wait for DHT discovery", hex.EncodeToString(bagID))
	}
	return string(bs), nil
}
func (c *client) latestHeaderForBag(bagID []byte) ([]byte, error) {
	k := make([]byte, 3+32)
	copy(k, "th:")
	copy(k[3:], bagID)
	th, err := c.db.Get(k, nil)
	if err != nil && !errors.Is(err, leveldb.ErrNotFound) {
		return nil, errors.Wrapf(err, "failed to read stored header for %v, will wait downloading header", hex.EncodeToString(bagID))
	}
	return th, nil
}

func (c *client) BuildUserPath(userPubKey string, contentType string) (userStorage string, uploadPath string) {
	spl := strings.Split(contentType, "/")
	return filepath.Join(c.rootStoragePath, userPubKey), spl[0]
}

func (c *client) ListFiles(userPubKey string, page, limit uint32) (total uint32, res []*FileMetadata, err error) {
	bag, _, err := c.bagByUser(userPubKey)
	if err != nil {
		return 0, nil, errors.Wrapf(err, "failed to get bagID for the user %v", userPubKey)
	}
	metadata, err := c.fileMeta(bag)
	if err != nil {
		return 0, nil, errors.Wrapf(err, "failed to parse bag header data %v", hex.EncodeToString(bag.BagID))
	}
	startOffset := page * limit
	if startOffset >= bag.Header.FilesCount {
		return bag.Header.FilesCount, []*FileMetadata{}, nil
	}
	endOffset := page*limit + limit
	if endOffset >= bag.Header.FilesCount {
		endOffset = bag.Header.FilesCount
	}
	res = make([]*FileMetadata, 0, limit)
	bs, err := c.buildBootstrapNodeInfo(bag)
	if err != nil {
		return 0, nil, errors.Wrapf(err, "failed to build bootstap for bag %v", hex.EncodeToString(bag.BagID))
	}
	files, err := bag.ListFiles()
	if err != nil {
		return 0, nil, errors.Wrapf(err, "failed to parse bag info for files %v", hex.EncodeToString(bag.BagID))
	}
	for i, f := range files[startOffset:endOffset] {
		idx := page*limit + uint32(i)
		fileInfo, _ := bag.GetFileOffsets(f)
		md, hasMD := metadata.FileMetadata[fileInfo.Name]
		if !hasMD {
			continue
		}
		b, err := json.Marshal([]*Bootstrap{bs})
		if err != nil {
			return 0, nil, errors.Wrapf(err, "failed to marshal %#v", bs)
		}
		bootstrap := base64.StdEncoding.EncodeToString(b)
		url, _, _ := c.buildUrl(hex.EncodeToString(bag.BagID), f, metadata.Master, hex.EncodeToString(md.Hash), bootstrap)
		res = append(res, &FileMetadata{
			FileMetadata: &nip94.FileMetadata{
				Size:            strconv.FormatUint(uint64(fileInfo.Size), 10),
				Summary:         md.Alt,
				URL:             url,
				M:               gomime.TypeByExtension(filepath.Ext(files[idx])),
				OX:              hex.EncodeToString(md.Hash),
				TorrentInfoHash: hex.EncodeToString(bag.BagID),
				Content:         md.Caption,
			},
			CreatedAt: uint64(time.Unix(0, int64(md.CreatedAt)).Unix()),
		})
	}
	return bag.Header.FilesCount, res, nil
}

func (c *client) FilePath(masterKey, fileHash, ext string) (string, error) {
	bag, _, err := c.bagByUser(masterKey)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get bagID for the user %v", masterKey)
	}
	if bag == nil {
		return "", ErrNotFound
	}
	userPath, _ := c.BuildUserPath(masterKey, "")
	var metadata *headerData
	metadata, err = c.fileMeta(bag)
	if err != nil {
		var serr *json.SyntaxError
		if errors.Is(err, errLatestBagNotDownloadedYet) || errors.As(err, &serr) {
			log.Warn().
				Str("context", "STORAGE").
				Err(err).
				Hex("bag_id", bag.BagID).
				Msg("failed to detect file meta")
			return filepath.Join(userPath, fmt.Sprintf("%v%v", fileHash, ext)), nil
		}
		return "", errors.Wrapf(err, "failed to parse bag header data %v", hex.EncodeToString(bag.BagID))
	}
	file, err := c.detectFileFromMeta(bag, metadata, fileHash)
	if err != nil {
		if errors.Is(err, storage.ErrFileNotExist) {
			return filepath.Join(userPath, fmt.Sprintf("%v%v", fileHash, ext)), nil
		}
		return "", errors.Wrapf(err, "failed to detect file %v in bag %v", fileHash, hex.EncodeToString(bag.BagID))
	}

	return filepath.Join(userPath, file), nil
}

func (c *client) Close() (err error) {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	c.server.Stop()
	c.dht.Close()
	if gClose := c.gateway.Close(); gClose != nil {
		err = errors.Join(err, errors.Wrapf(gClose, "failed to stop gateway"))
	}
	if sClose := c.stats.Close(); sClose != nil {
		err = errors.Join(err, errors.Wrapf(sClose, "failed to close stats file"))
	}
	if dErr := c.db.Close(); dErr != nil {
		err = errors.Join(err, errors.Wrapf(dErr, "failed to close db"))
	}
	close(c.downloadQueue)
	if c.cdn != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		err = errors.Join(err, c.cdn.Stop(shutdownCtx))
	}
	return err
}

func (c *client) report(ctx context.Context) {
	defer appcontext.GetAppContext(ctx).Recover()
	period := 1 * time.Hour
	if c.config.Debug {
		period = 1 * time.Minute
	}

	reporter := time.NewTicker(period)
	defer reporter.Stop()

	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		case <-reporter.C:
			activelyDownloading := 0
			activeUploading := 0
			notResolvedHeader := 0
			notResolvedInfo := 0
			notCompleted := 0
			all := c.progressStorage.GetAll()
			for _, t := range all {
				if t.IsDownloadAll() {
					activelyDownloading++
				}
				if _, upl := t.IsActive(); upl {
					activeUploading++
				}
				if !t.IsCompleted() {
					notCompleted++
				}
				if t.Info == nil {
					notResolvedInfo++
				}
				if t.Header == nil {
					notResolvedHeader++
				}
			}
			log.Info().Str("context", "STORAGE").
				Int("download_queue", len(c.downloadQueue)).
				Int("actively_downloading", activelyDownloading).
				Int("not_completed", notCompleted).
				Int("active_uploading", activeUploading).
				Int("not_resolved_info", notResolvedInfo).
				Int("not_resolved_header", notResolvedHeader).
				Int("total", len(all)).
				Msg("storage stats")
		}
	}
}
