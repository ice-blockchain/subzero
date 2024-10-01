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

	gomime "github.com/cubewise-code/go-mime"
	"github.com/gookit/goutil/errorx"
	"github.com/xssnick/tonutils-go/adnl"
	"github.com/xssnick/tonutils-go/adnl/dht"
	"github.com/xssnick/tonutils-go/adnl/overlay"
	"github.com/xssnick/tonutils-go/tl"
	"github.com/xssnick/tonutils-storage/storage"
)

func (c *client) StartUpload(ctx context.Context, userPubkey, relativePathToFileForUrl, hash string, newFile *FileMeta) (bagID, url string, err error) {
	existingBagForUser, err := c.bagByUser(userPubkey)
	if err != nil {
		return "", "", errorx.Withf(err, "failed to find existing bag for user %s", userPubkey)
	}
	var existingHD headerData
	if existingBagForUser != nil {
		if len(existingBagForUser.Header.Data) > 0 {
			if err = json.Unmarshal(existingBagForUser.Header.Data, &existingHD); err != nil {
				return "", "", errorx.Withf(err, "corrupted header metadata for bag %v", hex.EncodeToString(existingBagForUser.BagID))
			}
		}
	}

	var bs []*Bootstrap
	var bag *storage.Torrent
	bag, bs, err = c.upload(ctx, userPubkey, relativePathToFileForUrl, hash, newFile, &existingHD)
	if err != nil {
		return "", "", errorx.Withf(err, "failed to start upload of %v", relativePathToFileForUrl)
	}
	bagID = hex.EncodeToString(bag.BagID)
	if newFile != nil {
		uplFile, err := bag.GetFileOffsets(relativePathToFileForUrl)
		if err != nil {
			return "", "", errorx.Withf(err, "failed to get just created file from new bag")
		}
		fullFilePath := filepath.Join(c.rootStoragePath, userPubkey, relativePathToFileForUrl)
		go c.stats.ProcessFile(fullFilePath, gomime.TypeByExtension(filepath.Ext(fullFilePath)), uplFile.Size)
	}

	url, err = c.buildUrl(bagID, relativePathToFileForUrl, bs)
	if err != nil {
		return "", "", errorx.Withf(err, "failed to build url for %v (bag %v)", relativePathToFileForUrl, bagID)
	}
	return bagID, url, err
}

func (c *client) upload(ctx context.Context, user, relativePath, hash string, fileMeta *FileMeta, headerMetadata *headerData) (torrent *storage.Torrent, bootstrap []*Bootstrap, err error) {
	if fileMeta != nil {
		c.newFilesMx.Lock()
		if userNewFiles, hasNewFiles := c.newFiles[user]; !hasNewFiles || userNewFiles == nil {
			c.newFiles[user] = make(map[string]*FileMeta)
		}
		c.newFiles[user][relativePath] = fileMeta
		c.newFilesMx.Unlock()
	}
	rootUserPath, _ := c.BuildUserPath(user, "")
	refs, err := c.progressStorage.GetAllFilesRefsInDir(rootUserPath)
	if err != nil {
		return nil, nil, errorx.With(err, "failed to detect shareable files")
	}
	headerMD := &headerData{
		User:         user,
		FileMetadata: headerMetadata.FileMetadata,
		FileHash:     headerMetadata.FileHash,
	}
	if headerMD.FileHash == nil {
		headerMD.FileHash = make(map[string]string)
	}
	if headerMD.FileMetadata == nil {
		headerMD.FileMetadata = make(map[string]*FileMeta)
	}
	if fileMeta != nil {
		headerMD.FileMetadata[relativePath] = fileMeta
		headerMD.FileHash[hex.EncodeToString(fileMeta.Hash)] = relativePath
	} else {
		delete(headerMD.FileMetadata, relativePath)
		delete(headerMD.FileHash, hash)
	}
	c.newFilesMx.RLock()
	for key, value := range c.newFiles[user] {
		headerMD.FileMetadata[key] = value
		headerMD.FileHash[hex.EncodeToString(value.Hash)] = key
	}
	c.newFilesMx.RUnlock()
	var headerMDSerialized []byte
	headerMDSerialized, err = json.Marshal(headerMD)
	if err != nil {
		return nil, nil, errorx.With(err, "failed to put file hashes")
	}
	log.Println("UPLOAD META FOR ", relativePath, string(headerMDSerialized))
	header := &storage.TorrentHeader{
		DirNameSize:   uint32(len(user)),
		DirName:       []byte(user),
		Data:          headerMDSerialized,
		TotalDataSize: uint64(len(headerMDSerialized)),
	}
	var wg sync.WaitGroup
	wg.Add(1)
	tr, err := storage.CreateTorrentWithInitialHeader(ctx, c.rootStoragePath, user, header, c.progressStorage, c.conn, refs, func(done uint64, max uint64) {
		if done == max {
			wg.Done()
		}
	})
	if err != nil {
		return nil, nil, errorx.With(err, "failed to initialize bag")
	}
	err = tr.Start(true, true, false)
	if err != nil {
		return nil, nil, errorx.With(err, "failed to start bag upload")
	}
	wg.Wait()
	err = c.saveUploadTorrent(tr, user)
	if err != nil {
		return nil, nil, errorx.With(err, "failed to save updated bag")
	}
	bootstrapNode, err := c.buildBootstrapNodeInfo(tr)
	if err != nil {
		return nil, nil, errorx.With(err, "failed to build bootstrap node info")
	}
	return tr, []*Bootstrap{bootstrapNode}, nil
}

func (c *client) buildBootstrapNodeInfo(tr *storage.Torrent) (*Bootstrap, error) {
	key := c.server.GetADNLPrivateKey()
	overlayNode, err := overlay.NewNode(tr.BagID, key)
	if err != nil {
		return nil, errorx.With(err, "failed to build overlay node")
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
		return nil, errorx.Withf(err, "failed to sign dht bootstrap, serialize failure")
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
		return "", errorx.Withf(err, "failed to marshal %#v", bs)
	}
	bootstrap := base64.StdEncoding.EncodeToString(b)
	url := fmt.Sprintf("http://%v.bag/%v?bootstrap=%v", bagID, relativePath, bootstrap)

	return url, nil
}

func (c *client) saveUploadTorrent(tr *storage.Torrent, userPubKey string) error {
	if err := c.saveTorrent(tr, userPubKey); err != nil {
		return errorx.With(err, "failed to save upload torrent into storage")
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
