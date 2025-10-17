// SPDX-License-Identifier: ice License 1.0

package storage

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/rs/zerolog/log"
	"github.com/syndtr/goleveldb/leveldb"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

func (c *client) Delete(ctx context.Context, userPubKey, masterKey, fileHash string) error {
	it := query.GetStoredEvents(ctx,
		model.Filter{
			Kinds:     []int{nostr.KindFileMetadata},
			Authors:   []string{masterKey},
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
	bag, _, err := c.bagByUser(masterKey)
	if err != nil {
		return errors.Wrapf(err, "failed to get bagID for the user %v", userPubKey)
	}
	if bag == nil {
		return ErrNotFound
	}
	var metadata *headerData
	metadata, err = c.fileMeta(bag)
	if err != nil {
		return errors.Wrapf(err, "failed to parse bag header data %v", hex.EncodeToString(bag.BagID))
	}
	file, err := c.detectFileFromMeta(bag, metadata, fileHash)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			err = nil
			file = fileHash
		}
		if err != nil {
			return errors.Wrapf(err, "failed to detect file %v in bag %v", fileHash, hex.EncodeToString(bag.BagID))
		}
	}
	if userPubKey != masterKey {
		if md, foundMD := metadata.FileMetadata[file]; foundMD && md.Owner == masterKey {
			return ErrForbidden
		}
	}
	userPath, _ := c.BuildUserPath(masterKey, "")
	if err = os.Remove(filepath.Join(userPath, file)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Wrapf(err, "failed to remove file %v (%v)", fileHash, filepath.Join(userPath, file))
	}
	fileName := buildFileName(masterKey, fileHash, file)
	if c.cdnEnabled() {
		if err = c.cdn.FileDelete(ctx, fileName); err != nil {
			return errors.Wrapf(err, "failed to delete file %v from cdn", fileName)
		}
	}
	return nil
}

func (c *client) DeleteUser(masterKey string) error {
	bag, _, err := c.bagByUser(masterKey)
	if err != nil {
		return errors.Wrapf(err, "failed to get serving bag for user %v")
	}
	userPath, _ := c.BuildUserPath(masterKey, "")
	if c.cdnEnabled() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		files, err := os.ReadDir(userPath)
		if err != nil {
			log.Error().Err(err).Str("user", masterKey).Msg("failed to list files in user storage")
		}
		for _, f := range files {
			fNameOnCDN := buildFileName(masterKey, f.Name(), f.Name())
			if err = c.cdn.FileDelete(ctx, fNameOnCDN); err != nil {
				log.Error().Err(err).Str("filename", fNameOnCDN).Msg("failed to delete file from cdn")
			}
		}
	}

	if bag != nil {
		bag.Stop()
		if err = c.progressStorage.RemoveTorrent(bag, false); err != nil {
			return errors.Wrapf(err, "failed to remove bag %x from progress storage (deletion of user %v)", bag.BagID, masterKey)
		}
		b := &leveldb.Batch{}
		b.Delete(append([]byte("ub:"), bag.BagID...))
		b.Delete(append([]byte("bs:"), bag.BagID...))
		b.Delete(append([]byte("th:"), bag.BagID...))

		if err = c.db.Write(b, nil); err != nil {
			return errors.Wrapf(err, "failed to remove extra fields for  bag %x (deletion of user %v)", bag.BagID, masterKey)
		}
	}

	err = os.RemoveAll(userPath)
	return errors.Wrapf(err, "failed to clean up user storage %v", masterKey)
}
