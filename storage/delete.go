// SPDX-License-Identifier: ice License 1.0

package storage

import (
	"encoding/hex"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"github.com/syndtr/goleveldb/leveldb"
)

func (c *client) Delete(userPubKey, masterKey, fileHash string) error {
	bag, err := c.bagByUser(masterKey)
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
		return errors.Wrapf(err, "failed to detect file %v in bag %v", fileHash, hex.EncodeToString(bag.BagID))
	}
	if userPubKey != masterKey {
		if metadata.FileMetadata[file].Owner == masterKey {
			return ErrForbidden
		}
	}
	userPath, _ := c.BuildUserPath(masterKey, "")
	err = os.Remove(filepath.Join(userPath, file))
	if err != nil {
		return errors.Wrapf(err, "failed to remove file %v (%v)", fileHash, filepath.Join(userPath, file))
	}
	return nil
}

func (c *client) DeleteUser(masterKey string) error {
	bag, err := c.bagByUser(masterKey)
	if err != nil {
		return errors.Wrapf(err, "failed to get serving bag for user %v")
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
	userPath, _ := c.BuildUserPath(masterKey, "")
	err = os.RemoveAll(userPath)
	return errors.Wrapf(err, "failed to clean up user storage %v", masterKey)
}
