// SPDX-License-Identifier: ice License 1.0

package storage

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/url"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/xssnick/tonutils-storage/storage"

	"github.com/ice-blockchain/subzero/model"
)

func (c *client) DownloadUrl(userPubkey string, fileHash string) (string, error) {
	bag, err := c.bagByUser(userPubkey)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get bagID for the user %v", userPubkey)
	}
	if bag == nil {
		return "", ErrNotFound
	}
	bs, err := c.buildBootstrapNodeInfo(bag)
	if err != nil {
		return "", errors.Wrapf(err, "failed to build bootstap for bag %v", hex.EncodeToString(bag.BagID))
	}
	file, err := c.detectFile(bag, fileHash)
	if err != nil {
		return "", errors.Wrapf(err, "failed to detect file %v in bag %v", fileHash, hex.EncodeToString(bag.BagID))
	}
	return c.buildUrl(hex.EncodeToString(bag.BagID), file, []*Bootstrap{bs})
}

func acceptNewBag(ctx context.Context, event *model.Event) error {
	infohash := ""
	var fileUrl *url.URL
	var err error
	if iTag := event.Tags.GetFirst([]string{"i"}); iTag != nil && len(*iTag) > 1 {
		infohash = iTag.Value()
	} else {
		return errors.Newf("malformed i tag %v", iTag)
	}
	if urlTag := event.Tags.GetFirst([]string{"url"}); urlTag != nil && len(*urlTag) > 1 {
		fileUrl, err = url.Parse(urlTag.Value())
		if err != nil {
			return errors.Wrapf(err, "malformed url in url tag %v", *urlTag)
		}
	} else {
		return errors.Newf("malformed url tag %v", urlTag)
	}
	bootstrap := fileUrl.Query().Get("bootstrap")
	if err = globalClient.newBagIDPromoted(ctx, event.GetMasterPublicKey(), infohash, &bootstrap); err != nil {
		return errors.Wrapf(err, "failed to promote new bag ID %v for user %v", infohash, event.PubKey)
	}
	return nil
}

func (c *client) newBagIDPromoted(ctx context.Context, user, bagID string, bootstap *string) error {
	existingBagForUser, err := c.bagByUser(user)
	if err != nil {
		return errors.Wrapf(err, "failed to find existing bag for user %s", user)
	}
	if existingBagForUser != nil {
		existingBagForUser.Stop()
		if err = c.progressStorage.RemoveTorrent(existingBagForUser, false); err != nil {
			return errors.Wrapf(err, "failed to replace bag for user %s", user)
		}
	}
	if err = c.download(ctx, bagID, user, bootstap); err != nil {
		return errors.Wrapf(err, "failed to download new bag ID %v for user %v", bagID, user)
	}
	return nil
}

func (c *client) download(ctx context.Context, bagID, user string, bootstrap *string) (err error) {
	bag, err := hex.DecodeString(bagID)
	if err != nil {
		return errors.Wrapf(err, "invalid bagID %v", bagID)
	}
	if len(bag) != 32 {
		return errors.Wrapf(err, "invalid bagID %v, should be len 32", bagID)
	}

	tor := c.progressStorage.GetTorrent(bag)
	if tor == nil {
		tor = storage.NewTorrent(c.rootStoragePath, c.progressStorage, c.conn)
		tor.BagID = bag
		c.downloadQueue <- queueItem{
			tor:       tor,
			bootstrap: bootstrap,
			user:      &user,
		}
		if err = c.saveTorrent(tor, &user, bootstrap); err != nil {
			return errors.Wrapf(err, "failed to store new torrent %v", bagID)
		}
	} else {
		if err = tor.Start(true, true, false); err != nil {
			return errors.Wrapf(err, "failed to start existing torrent %v", bagID)
		}
	}
	return nil
}

func (c *client) torrentStateCallback(tor *storage.Torrent, user *string) func(event storage.Event) {
	return func(event storage.Event) {
		switch event.Name {
		case storage.EventDone:
			tor.Stop()
			if pErr := tor.Start(true, false, false); pErr != nil {
				log.Printf("ERROR: failed to stop torrent upload after downloading data: %v", pErr)
			}
			c.activeDownloadsMx.Lock()
			delete(c.activeDownloads, hex.EncodeToString(tor.BagID))
			c.activeDownloadsMx.Unlock()

		case storage.EventBagResolved:
			if _, isUplActive := tor.IsActive(); !isUplActive {
				if pErr := tor.StartWithCallback(true, true, false, c.torrentStateCallback(tor, user)); pErr != nil {
					log.Printf("ERROR: failed to start torrent upload after downloading header: %v", pErr)
				}
			}
		}
		if pErr := c.saveTorrent(tor, user, nil); pErr != nil {
			log.Printf("ERROR: failed save torrent with stopped download after downloading: %v", pErr)
		}

	}
}

func (c *client) connectToBootstrap(ctx context.Context, torrent *storage.Torrent, bootstrap string) error {
	b64, err := base64.StdEncoding.DecodeString(bootstrap)
	if err != nil {
		return errors.Wrapf(err, "failed to decode bootstrap %v", bootstrap)
	}
	var bootstraps []Bootstrap
	if err = json.Unmarshal(b64, &bootstraps); err != nil {
		return errors.Wrapf(err, "failed to decode bootstrap %v", string(b64))
	}
	for _, bs := range bootstraps {
		pk := bs.Overlay.ID.(map[string]any)["Key"].(string)
		var pubKey []byte
		pubKey, err = base64.StdEncoding.DecodeString(pk)
		if err != nil {
			return errors.Wrapf(err, "failed to decode bootstrap %v, invalid pubkey %v", string(b64), string(pk))
		}
		if err = c.server.ConnectToNode(ctx, torrent, bs.Overlay, bs.DHT.AddrList, pubKey); err != nil {
			return errors.Wrapf(err, "failed to connect to bootstrap node %#v", bs.DHT.AddrList.Addresses[0])
		}
	}
	return nil
}

func (c *client) saveTorrent(tr *storage.Torrent, userPubKey *string, bs *string) error {
	if err := c.progressStorage.SetTorrent(tr); err != nil {
		return errors.Wrap(err, "failed to save torrent into storage")
	}
	if userPubKey != nil {
		k := make([]byte, 3+64)
		copy(k, "ub:")
		copy(k[3:], *userPubKey)
		if err := c.db.Put(k, tr.BagID, nil); err != nil {
			return errors.Wrapf(err, "failed to save userID:bag mapping for bag %v", hex.EncodeToString(tr.BagID))
		}
	}
	if bs != nil {
		k := make([]byte, 3+32)
		copy(k, "bs:")
		copy(k[3:], tr.BagID)
		if err := c.db.Put(k, []byte(*bs), nil); err != nil {
			return errors.Wrapf(err, "failed to save bootstrap node for bag %v", hex.EncodeToString(tr.BagID))
		}
	}

	return nil
}

func (c *client) startDownloadsFromQueue() {
outerLoop:
	for {
		c.activeDownloadsMx.RLock()
		l := len(c.activeDownloads)
		c.activeDownloadsMx.RUnlock()
		for l < ConcurrentBagsDownloading {
			select {
			case q := <-c.downloadQueue:
				tor := q.tor
				if downloading, _ := tor.IsActive(); downloading {
					continue
				}
				if err := tor.StartWithCallback(false, true, false, c.torrentStateCallback(tor, q.user)); err != nil {
					log.Printf("ERROR: %v", errors.Wrapf(err, "failed to start new torrent %v", q.tor.BagID))
				}
				if q.bootstrap != nil && *q.bootstrap != "" {
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					if err := c.connectToBootstrap(ctx, tor, *q.bootstrap); err != nil {
						log.Printf("failed to connect to bootstrap node for bag %v, waiting for DHT: %v", hex.EncodeToString(q.tor.BagID), err)
					}
					cancel()
				}
				if err := c.saveTorrent(tor, q.user, q.bootstrap); err != nil {
					log.Printf("failed save updated upload / download torrrent state %v: %v", hex.EncodeToString(q.tor.BagID), err)
				}
				c.activeDownloadsMx.Lock()
				c.activeDownloads[hex.EncodeToString(tor.BagID)] = true
				c.activeDownloadsMx.Unlock()
				l += 1
			default:
				time.Sleep(100 * time.Millisecond)
				continue outerLoop
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

}
