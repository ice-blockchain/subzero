// SPDX-License-Identifier: ice License 1.0

package storage

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	gomime "github.com/cubewise-code/go-mime"
	"github.com/hashicorp/go-multierror"
	"github.com/nbd-wtf/go-nostr/nip94"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/xssnick/tonutils-go/adnl"
	"github.com/xssnick/tonutils-go/adnl/dht"
	"github.com/xssnick/tonutils-go/adnl/overlay"
	"github.com/xssnick/tonutils-storage/db"
	"github.com/xssnick/tonutils-storage/storage"

	"github.com/ice-blockchain/subzero/storage/statistics"
)

type (
	StorageClient interface {
		io.Closer
		StartUpload(ctx context.Context, userPubkey, relativePathToFileForUrl, fileHash string, newFile *FileMetaInput) (bagID, url string, existed bool, err error)
		BuildUserPath(userPubKey, contentType string) (string, string)
		DownloadUrl(userPubkey, fileSha256 string) (string, error)
		ListFiles(userPubkey string, page, count uint32) (totalFiles uint32, files []*FileMetadata, err error)
		Delete(userPubkey string, fileSha256 string) error
	}
	Bootstrap struct {
		Overlay *overlay.Node
		DHT     *dht.Node
	}
	headerData struct {
		User         string                    `json:"u"`
		FileMetadata map[string]*FileMetaInput `json:"f"`
		FileHash     map[string]string         `json:"fh"`
	}
	FileMetaInput struct {
		Hash      []byte `json:"h"`
		Caption   string `json:"c"`
		Alt       string `json:"a"`
		CreatedAt uint64 `json:"cAt"`
	}
	FileMetadata struct {
		*nip94.FileMetadata
		CreatedAt uint64 `json:"created_at"`
	}
	client struct {
		conn              *storage.Connector
		db                *leveldb.DB
		server            *storage.Server
		progressStorage   *db.Storage
		gateway           *adnl.Gateway
		dht               *dht.Client
		rootStoragePath   string
		newFiles          map[string]map[string]*FileMetaInput
		newFilesMx        *sync.RWMutex
		stats             statistics.Statistics
		downloadQueue     chan queueItem
		activeDownloads   map[string]bool
		activeDownloadsMx *sync.RWMutex
	}
	queueItem struct {
		tor       *storage.Torrent
		bootstrap *string
		user      *string
	}
)

var ErrNotFound = storage.ErrFileNotExist

func (c *client) fileMeta(bag *storage.Torrent) (*headerData, error) {
	var desc headerData
	hData := bag.Header.Data
	if len(hData) == 0 {
		hData = []byte("{}")
	}
	if err := json.Unmarshal(hData, &desc); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal bag header data")
	}
	return &desc, nil
}

func (c *client) detectFile(bag *storage.Torrent, fileHash string) (string, error) {
	metadata, err := c.fileMeta(bag)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse bag header data %v", hex.EncodeToString(bag.BagID))
	}
	name := metadata.FileHash[fileHash]
	f, err := bag.GetFileOffsets(name)
	if err != nil {
		return "", errors.Wrapf(err, "failed to locate file %v in bag %v", name, hex.EncodeToString(bag.BagID))
	}
	return f.Name, nil
}

func (c *client) bagByUser(userPubKey string) (*storage.Torrent, error) {
	k := make([]byte, 3+64)
	copy(k, "ub:")
	copy(k[3:], userPubKey)
	bagID, err := c.db.Get(k, nil)
	if err != nil && !errors.Is(err, leveldb.ErrNotFound) {
		return nil, errors.Wrap(err, "failed to read userID:bag mapping")
	}
	tr := c.progressStorage.GetTorrent(bagID)

	return tr, nil
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

func (c *client) BuildUserPath(userPubKey string, contentType string) (userStorage string, uploadPath string) {
	spl := strings.Split(contentType, "/")
	return filepath.Join(c.rootStoragePath, userPubKey), spl[0]
}

func (c *client) ListFiles(userPubKey string, page, limit uint32) (total uint32, res []*FileMetadata, err error) {
	bag, err := c.bagByUser(userPubKey)
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
		url, _ := c.buildUrl(hex.EncodeToString(bag.BagID), f, []*Bootstrap{bs})
		res = append(res, &FileMetadata{
			FileMetadata: &nip94.FileMetadata{
				Size:            strconv.FormatUint(uint64(fileInfo.Size), 10),
				Summary:         md.Alt,
				URL:             url,
				M:               gomime.TypeByExtension(filepath.Ext(files[idx])),
				X:               hex.EncodeToString(md.Hash),
				OX:              hex.EncodeToString(md.Hash),
				TorrentInfoHash: hex.EncodeToString(bag.BagID),
				Content:         md.Caption,
			},
			CreatedAt: uint64(time.Unix(0, int64(md.CreatedAt)).Unix()),
		})
	}
	return bag.Header.FilesCount, res, nil
}

func (c *client) Delete(userPubKey, fileHash string) error {
	bag, err := c.bagByUser(userPubKey)
	if err != nil {
		return errors.Wrapf(err, "failed to get bagID for the user %v", userPubKey)
	}
	file, err := c.detectFile(bag, fileHash)
	if err != nil {
		return errors.Wrapf(err, "failed to detect file %v in bag %v", fileHash, hex.EncodeToString(bag.BagID))
	}
	userPath, _ := c.BuildUserPath(userPubKey, "")
	err = os.Remove(filepath.Join(userPath, file))
	if err != nil {
		return errors.Wrapf(err, "failed to remove file %v (%v)", fileHash, filepath.Join(userPath, file))
	}
	return nil
}

func (c *client) Close() error {
	var err *multierror.Error
	c.server.Stop()
	c.dht.Close()
	if gClose := c.gateway.Close(); gClose != nil {
		err = multierror.Append(err, errors.Wrapf(gClose, "failed to stop gateway"))
	}
	if sClose := c.stats.Close(); sClose != nil {
		err = multierror.Append(err, errors.Wrapf(sClose, "failed to close stats file"))
	}
	if dErr := c.db.Close(); dErr != nil {
		err = multierror.Append(err, errors.Wrapf(dErr, "failed to close db"))
	}
	close(c.downloadQueue)
	return err.ErrorOrNil()
}
