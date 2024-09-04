package storage

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/url"

	"github.com/gookit/goutil/errorx"
	"github.com/xssnick/tonutils-storage/storage"

	"github.com/ice-blockchain/subzero/model"
)

func (c *client) DownloadUrl(userPubkey string, fileHash string) (string, error) {
	bag, err := c.bagByUser(userPubkey)
	if err != nil {
		return "", errorx.Withf(err, "failed to get bagID for the user %v", userPubkey)
	}
	bs, err := c.buildBootstrapNodeInfo(bag)
	if err != nil {
		return "", errorx.Withf(err, "failed to build bootstap for bag %v", hex.EncodeToString(bag.BagID))
	}
	file, err := c.detectFile(bag, fileHash)
	if err != nil {
		return "", errorx.Withf(err, "failed to detect file %v in bag %v", fileHash, hex.EncodeToString(bag.BagID))
	}
	return c.buildUrl(hex.EncodeToString(bag.BagID), file, []*Bootstrap{bs})
}

func acceptNewBag(ctx context.Context, event *model.Event) error {
	infohash := ""
	var fileUrl *url.URL
	var err error
	if iTag := event.Tags.GetFirst([]string{"i"}); iTag != nil && len(*iTag) > 1 {
		infohash = (*iTag)[1]
	} else {
		return errorx.Newf("malformed i tag %v", iTag)
	}
	if urlTag := event.Tags.GetFirst([]string{"url"}); urlTag != nil && len(*urlTag) > 1 {
		fileUrl, err = url.Parse((*urlTag)[1])
		if err != nil {
			return errorx.Withf(err, "malformed url in url tag %v", *urlTag)
		}
	} else {
		return errorx.Newf("malformed url tag %v", urlTag)
	}
	bootstrap := fileUrl.Query().Get("bootstrap")
	if err = globalClient.newBagIDPromoted(ctx, event.PubKey, infohash, &bootstrap); err != nil {
		return errorx.Withf(err, "failed to promote new bag ID %v for user %v", infohash, event.PubKey)
	}
	return nil
}

func (c *client) newBagIDPromoted(ctx context.Context, user, bagID string, bootstap *string) error {
	existingBagForUser, err := c.bagByUser(user)
	if err != nil {
		return errorx.Withf(err, "failed to find existing bag for user %s", user)
	}
	if existingBagForUser != nil {
		if err = c.progressStorage.RemoveTorrent(existingBagForUser, false); err != nil {
			return errorx.Withf(err, "failed to replace bag for user %s", user)
		}
	}
	if err = c.download(ctx, bagID, user, bootstap); err != nil {
		return errorx.Withf(err, "failed to download new bag ID %v for user %v", bagID, user)
	}
	return nil
}

func (c *client) download(ctx context.Context, bagID, user string, bootstrap *string) (err error) {
	bag, err := hex.DecodeString(bagID)
	if err != nil {
		return errorx.Withf(err, "invalid bagID %v", bagID)
	}
	if len(bag) != 32 {
		return errorx.Withf(err, "invalid bagID %v, should be len 32", bagID)
	}

	tor := c.progressStorage.GetTorrent(bag)
	if tor == nil {
		tor = storage.NewTorrent(c.rootStoragePath, c.progressStorage, c.conn)
		tor.BagID = bag
		if err = tor.Start(true, true, false); err != nil {
			return errorx.Withf(err, "failed to start new torrent %v", bagID)
		}
		if bootstrap != nil && *bootstrap != "" {
			if err = c.connectToBootstrap(ctx, tor, *bootstrap); err != nil {
				log.Printf("failed to connect to bootstrap node, waiting for DHT: %v", err)
			}
		}
		if err = c.saveTorrent(tor, user); err != nil {
			return errorx.Withf(err, "failed to store new torrent %v", bagID)
		}
	} else {
		if err = tor.Start(true, true, false); err != nil {
			return errorx.Withf(err, "failed to start existing torrent %v", bagID)
		}
	}
	return nil
}

func (c *client) connectToBootstrap(ctx context.Context, torrent *storage.Torrent, bootstrap string) error {
	b64, err := base64.StdEncoding.DecodeString(bootstrap)
	if err != nil {
		return errorx.Withf(err, "failed to decode bootstrap %v", bootstrap)
	}
	var bootstraps []Bootstrap
	if err = json.Unmarshal(b64, &bootstraps); err != nil {
		return errorx.Withf(err, "failed to decode bootstrap %v", string(b64))
	}
	for _, bs := range bootstraps {
		pk := bs.Overlay.ID.(map[string]any)["Key"].(string)
		var pubKey []byte
		pubKey, err = base64.StdEncoding.DecodeString(pk)
		if err != nil {
			return errorx.Withf(err, "failed to decode bootstrap %v, invalid pubkey %v", string(b64), string(pk))
		}
		if err = c.server.ConnectToNode(ctx, torrent, bs.Overlay, bs.DHT.AddrList, pubKey); err != nil {
			return errorx.Withf(err, "failed to connect to bootstrap node %#v", bs.DHT.AddrList.Addresses[0])
		}
	}
	return nil
}

func (c *client) saveTorrent(tr *storage.Torrent, userPubKey string) error {
	if err := c.progressStorage.SetTorrent(tr); err != nil {
		return errorx.With(err, "failed to save torrent into storage")
	}
	k := make([]byte, 5+64)
	copy(k, "desc:")
	copy(k[5:], userPubKey)
	if err := c.db.Put(k, tr.BagID, nil); err != nil {
		return errorx.With(err, "failed to save userID:bag mapping")
	}
	return nil
}
