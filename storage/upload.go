// SPDX-License-Identifier: ice License 1.0

package storage

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	gomime "github.com/cubewise-code/go-mime"
	"github.com/xssnick/tonutils-go/adnl"
	"github.com/xssnick/tonutils-go/adnl/dht"
	"github.com/xssnick/tonutils-go/adnl/overlay"
	"github.com/xssnick/tonutils-go/tl"
	"github.com/xssnick/tonutils-storage/storage"
)

func (c *client) StartUpload(ctx context.Context, userPubKey, masterPubKey, relativePathToFileForUrl, hash string, newFile *FileMetaInput) (bagID, url string, existed bool, err error) {
	existingBagForUser, err := c.bagByUser(masterPubKey)
	if err != nil {
		return "", "", false, errors.Wrapf(err, "failed to find existing bag for user %s", masterPubKey)
	}
	var existingHD headerData
	if existingBagForUser != nil {
		if len(existingBagForUser.Header.Data) > 0 {
			if err = json.Unmarshal(existingBagForUser.Header.Data, &existingHD); err != nil {
				return "", "", false, errors.Wrapf(err, "corrupted header metadata for bag %v", hex.EncodeToString(existingBagForUser.BagID))
			}
		}
	}
	_, existed = existingHD.FileHash[hash]
	if existed {
		if existingBagForUser != nil {
			url, err = c.DownloadUrl(masterPubKey, hash)
			if err != nil {
				if errors.Is(err, storage.ErrFileNotExist) {
					existed = false
				}
				return "", "", false,
					errors.Wrapf(err, "failed to build download url for already existing file %v/%v(%v)", masterPubKey, relativePathToFileForUrl, hash)
			}
			if existed {
				return hex.EncodeToString(existingBagForUser.BagID), url, existed, nil
			}

		}
	}
	var bs []*Bootstrap
	var bag *storage.Torrent
	bag, bs, err = c.upload(ctx, userPubKey, masterPubKey, relativePathToFileForUrl, hash, newFile, &existingHD)
	if err != nil {
		return "", "", false, errors.Wrapf(err, "failed to start upload of %v", relativePathToFileForUrl)
	}
	bagID = hex.EncodeToString(bag.BagID)
	log.Printf("[STORAGE] INFO: new upload %v for user %v hash %v resulted in bag %v, total %v", relativePathToFileForUrl, masterPubKey, hash, bagID, bag.Header.FilesCount)
	if newFile != nil {
		uplFile, err := bag.GetFileOffsets(relativePathToFileForUrl)
		if err != nil {
			return "", "", false, errors.Wrapf(err, "failed to get just created file from new bag")
		}
		fullFilePath := filepath.Join(c.rootStoragePath, masterPubKey, relativePathToFileForUrl)
		go c.stats.ProcessFile(fullFilePath, gomime.TypeByExtension(filepath.Ext(fullFilePath)), uplFile.Size)
	}
	url, err = c.buildUrl(bagID, relativePathToFileForUrl, bs)
	if err != nil {
		return "", "", false, errors.Wrapf(err, "failed to build url for %v (bag %v)", relativePathToFileForUrl, bagID)
	}
	return bagID, url, existed, err
}

func (c *client) upload(ctx context.Context, user, master, relativePath, hash string, fileMeta *FileMetaInput, headerMetadata *headerData) (torrent *storage.Torrent, bootstrap []*Bootstrap, err error) {
	if fileMeta != nil {
		c.newFilesMx.Lock()
		if userNewFiles, hasNewFiles := c.newFiles[master]; !hasNewFiles || userNewFiles == nil {
			c.newFiles[master] = make(map[string]*FileMetaInput)
		}
		c.newFiles[master][relativePath] = fileMeta
		c.newFilesMx.Unlock()
	}
	rootUserPath, _ := c.BuildUserPath(master, "")
	refs, err := c.progressStorage.GetAllFilesRefsInDir(rootUserPath)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to detect shareable files")
	}
	headerMD := &headerData{
		Master:       master,
		FileMetadata: headerMetadata.FileMetadata,
		FileHash:     headerMetadata.FileHash,
	}
	if headerMD.FileHash == nil {
		headerMD.FileHash = make(map[string]string)
	}
	if headerMD.FileMetadata == nil {
		headerMD.FileMetadata = make(map[string]*FileMetaInput)
	}
	if fileMeta != nil {
		fileMeta.Owner = user
		headerMD.FileMetadata[relativePath] = fileMeta
		headerMD.FileHash[hex.EncodeToString(fileMeta.Hash)] = relativePath
	} else {
		delete(headerMD.FileMetadata, relativePath)
		delete(headerMD.FileHash, hash)
	}
	c.newFilesMx.RLock()
	for key, value := range c.newFiles[master] {
		headerMD.FileMetadata[key] = value
		headerMD.FileHash[hex.EncodeToString(value.Hash)] = key
	}
	c.newFilesMx.RUnlock()
	var headerMDSerialized []byte
	headerMDSerialized, err = json.Marshal(headerMD)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to put file hashes")
	}
	header := &storage.TorrentHeader{
		DirNameSize:   uint32(len(master)),
		DirName:       []byte(master),
		Data:          headerMDSerialized,
		TotalDataSize: uint64(len(headerMDSerialized)),
	}
	var wg sync.WaitGroup
	wg.Add(1)
	tr, err := storage.CreateTorrentWithInitialHeader(ctx, c.rootStoragePath, master, header, c.progressStorage, c.conn, refs, func(done uint64, max uint64) {
		if done == max {
			wg.Done()
		}
	}, false)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to initialize bag")
	}
	err = tr.Start(true, false, false)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to start bag upload")
	}
	wg.Wait()
	err = c.saveUploadTorrent(tr, master)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to save updated bag")
	}
	bootstrapNode, err := c.buildBootstrapNodeInfo(tr)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to build bootstrap node info")
	}
	return tr, []*Bootstrap{bootstrapNode}, nil
}

func (c *client) buildBootstrapNodeInfo(tr *storage.Torrent) (*Bootstrap, error) {
	key := c.server.GetADNLPrivateKey()
	overlayNode, err := overlay.NewNode(tr.BagID, key)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build overlay node")
	}
	addr := c.gateway.GetAddressList()

	dNode := dht.Node{
		ID:        adnl.PublicKeyED25519{Key: key.Public().(ed25519.PublicKey)},
		AddrList:  &addr,
		Version:   int32(time.Now().Unix()),
		Signature: nil,
	}

	toVerify, err := tl.Serialize(dNode, true)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to sign dht bootstrap, serialize failure")
	}
	dNode.Signature = ed25519.Sign(key, toVerify)

	return &Bootstrap{
		Overlay: overlayNode,
		DHT:     &dNode,
	}, nil
}

func (c *client) buildUrl(bagID, relativePath string, bs []*Bootstrap) (string, error) {
	b, err := json.Marshal(bs)
	if err != nil {
		return "", errors.Wrapf(err, "failed to marshal %#v", bs)
	}
	bootstrap := base64.StdEncoding.EncodeToString(b)
	url := fmt.Sprintf("http://%v.bag/%v?bootstrap=%v", bagID, relativePath, bootstrap)

	return url, nil
}

func (c *client) saveUploadTorrent(tr *storage.Torrent, userPubKey string) error {
	if err := c.saveTorrent(tr, &userPubKey, nil); err != nil {
		return errors.Wrap(err, "failed to save upload torrent into storage")
	}
	c.newFilesMx.Lock()
	for k := range c.newFiles[userPubKey] {
		if _, err := tr.GetFileOffsets(k); err == nil {
			delete(c.newFiles[userPubKey], k)
		}
	}
	c.newFilesMx.Unlock()
	return nil
}
