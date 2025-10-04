// SPDX-License-Identifier: ice License 1.0

package storage

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/imroc/req/v3"
	"github.com/nbd-wtf/go-nostr"
	"github.com/rs/zerolog/log"
	"github.com/xssnick/tonutils-go/adnl/keys"
	"github.com/xssnick/tonutils-storage/storage"
	"golang.org/x/sync/errgroup"

	"github.com/ice-blockchain/subzero/cmd/subzero-ion-connect/appcontext"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/server/http/nip98"
)

func (c *client) DownloadUrl(masterPubkey string, fileHash string) (string, error) {
	bag, _, err := c.bagByUser(masterPubkey)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get bagID for the user %v", masterPubkey)
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
	b, err := json.Marshal([]*Bootstrap{bs})
	if err != nil {
		return "", errors.Wrapf(err, "failed to marshal %#v", bs)
	}
	bootstrap := base64.StdEncoding.EncodeToString(b)
	return c.buildUrl(hex.EncodeToString(bag.BagID), file, masterPubkey, fileHash, bootstrap)
}

func acceptNewBag(ctx context.Context, event *model.Event, acceptor func(ctx context.Context, fh, master, infohash string) error) error {
	log.Info().Str("context", "STORAGE").
		Str("user", event.GetMasterPublicKey()).
		Str("event", event.String()).
		Msg("accepting NIP-94 with new files")
	infohash := event.GetTag("i").Value()
	if infohash == "" {
		return errors.Newf("malformed or missing i tag in event %v", event.ID)
	}

	fileHash := event.GetTag("ox").Value()
	if fileHash == "" {
		return errors.Newf("malformed or missing ox tag in event %v", event.ID)
	}

	return acceptor(ctx, fileHash, event.GetMasterPublicKey(), infohash)
}

func (c *client) StartDownloadNewBag(ctx context.Context, fileHash, userMasterKey, infohash string) error {
	log.Info().Str("context", "STORAGE").
		Str("user", userMasterKey).
		Str("infohash", infohash).
		Msg("accepting NIP-94 infohash with new files")
	spl := strings.Split(infohash, ":")
	if len(spl) != 3 {
		return errors.Newf("malformed i tag %v, cannot detect bootstrap and version", infohash)
	}
	infohash = spl[0]
	bootstrap := spl[1]
	version, cErr := strconv.ParseInt(spl[2], 10, 64)
	if cErr != nil {
		return errors.Wrapf(cErr, "malformed i tag %v, cannot version", infohash)
	}

	if err := c.newBagIDPromoted(ctx, userMasterKey, infohash, &bootstrap, version); err != nil {
		return errors.Wrapf(err, "failed to promote new bag ID %v for user %v", infohash, userMasterKey)
	}
	return nil
}

func (c *client) newBagIDPromoted(ctx context.Context, user, bagID string, bootstap *string, newVersion int64) error {
	existingBagForUser, ver, err := c.bagByUser(user)
	if err != nil {
		return errors.Wrapf(err, "failed to find existing bag for user %s", user)
	}
	replaceBagPerUser := existingBagForUser == nil
	if existingBagForUser != nil && hex.EncodeToString(existingBagForUser.BagID) != bagID {
		if (existingBagForUser.Header == nil && ver < newVersion) || (existingBagForUser.Header != nil && int64(existingBagForUser.Header.FilesCount) < newVersion) {
			log.Info().
				Str("context", "STORAGE").
				Str("user", user).
				Hex("existing_bag_id", existingBagForUser.BagID).
				Str("new_bag_id", bagID).
				Msg("got NIP-94 with new files, replacing")
			downloading := existingBagForUser.IsDownloadAll()
			existingBagForUser.Stop()
			c.activeDownloadsMx.Lock()
			delete(c.activeDownloads, hex.EncodeToString(existingBagForUser.BagID))
			c.activeDownloadsMx.Unlock()
			if downloading {
				if err = c.progressStorage.RemoveTorrent(existingBagForUser, false); err != nil {
					return errors.Wrapf(err, "failed to replace bag for user %s", user)
				}
			}
			replaceBagPerUser = true
		}
	}
	if replaceBagPerUser && user != "" {
		bagId, _ := hex.DecodeString(bagID)
		if err = c.saveBagPerUser(bagId, &newVersion, &user); err != nil {
			return errors.Wrapf(err, "failed to save bag per user")
		}
	}
	if err = c.download(ctx, bagID, user, bootstap, newVersion); err != nil {
		return errors.Wrapf(err, "failed to download new bag ID %v for user %v", bagID, user)
	}
	return nil
}

func (c *client) download(ctx context.Context, bagID, user string, bootstrap *string, newVersion int64) (err error) {
	bag, err := hex.DecodeString(bagID)
	if err != nil {
		return errors.Wrapf(err, "invalid bagID %v", bagID)
	}
	if len(bag) != 32 {
		return errors.Wrapf(err, "invalid bagID %v, should be len 32", bagID)
	}
	c.activeDownloadsMx.RLock()
	if _, has := c.activeDownloads[bagID]; has {
		c.activeDownloadsMx.RUnlock()
		return
	}
	c.activeDownloadsMx.RUnlock()
	log.Info().Str("context", "STORAGE").
		Str("bag_id", bagID).
		Str("user", user).
		Int("queue_size", len(c.downloadQueue)).
		Msg("adding to downloads")
	tor := c.progressStorage.GetTorrent(bag)
	if tor == nil {
		tor = storage.NewTorrent(c.rootStoragePath, c.progressStorage, c.conn)
		tor.BagID = bag
		if err = c.saveTorrent(tor, &user, bootstrap, false, &newVersion); err != nil {
			return errors.Wrapf(err, "failed to store new torrent %v", bagID)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case c.downloadQueue <- queueItem{
			tor:       tor,
			bootstrap: bootstrap,
			user:      &user,
			version:   newVersion,
		}:
		}
	} else {
		if !tor.IsCompleted() {
			if err = tor.Start(true, true, false); err != nil {
				return errors.Wrapf(err, "failed to start existing torrent %v", bagID)
			}
		}
	}
	return nil
}

func (c *client) torrentStateCallback(tor *storage.Torrent, user *string) func(event storage.Event) {
	return func(event storage.Event) {
		usr := ""
		if user != nil {
			usr = *user
		}
		switch event.Name {
		case storage.EventDone:
			c.progressStorage.SetActiveFiles(tor.BagID, []uint32{})
			tor.Stop()

			files, _ := tor.ListFiles()

			log.Info().
				Str("context", "STORAGE").
				Hex("bag_id", tor.BagID).
				Str("user", usr).
				Uint32("files_count", tor.Header.FilesCount).
				Uint64("file_size", tor.Info.FileSize).
				Strs("files", files).
				Msg("bag downloaded, disabling download")
			if pErr := tor.Start(true, false, false); pErr != nil {
				log.Error().Err(pErr).Hex("bag_id", tor.BagID).Str("user", usr).Msg("failed to stop torrent download after downloading data")
			}
			c.activeDownloadsMx.Lock()
			delete(c.activeDownloads, hex.EncodeToString(tor.BagID))
			c.activeDownloadsMx.Unlock()
			ver := int64(tor.Header.FilesCount)
			if pErr := c.saveTorrent(tor, user, nil, false, &ver); pErr != nil {
				log.Error().Err(pErr).Hex("bag_id", tor.BagID).Msg("failed save torrent with stopped download after downloading")
			}

		case storage.EventBagResolved:
			if _, isUplActive := tor.IsActive(); !isUplActive {
				log.Info().
					Str("context", "STORAGE").
					Hex("bag_id", tor.BagID).
					Str("user", usr).
					Uint32("files_count", tor.Header.FilesCount).
					Uint64("file_size", uint64(tor.Info.FileSize)).
					Msg("bag header resolved, enabling upload to serve clients with chunks we own")
				if pErr := tor.StartWithCallback(true, true, false, c.torrentStateCallback(tor, user)); pErr != nil {
					log.Error().Err(pErr).Hex("bag_id", tor.BagID).Msg("failed to start torrent upload after downloading header")
				}
				if user != nil {
					m, err := c.fileMeta(tor)
					if err != nil {
						log.Error().Str("context", "STORAGE").Err(err).Hex("bag_id", tor.BagID).Msg("failed to get file meta for bag although it is resolved")
					}
					if m != nil {
						*user = m.Master
					}
				}
				ver := int64(tor.Header.FilesCount)
				if pErr := c.saveTorrent(tor, user, nil, false, &ver); pErr != nil {
					log.Error().Err(pErr).Hex("bag_id", tor.BagID).Msg("failed save torrent with stopped download after downloading")
				}
			}
		case storage.EventFileDownloaded:
			log.Debug().Str("context", "STORAGE").Hex("bag_id", tor.BagID).Str("user", usr).Any("file", event.Value).Msg("bag downloaded file")
		case storage.EventErr:
			log.Error().Str("context", "STORAGE").Hex("bag_id", tor.BagID).Str("user", usr).Any("error", event.Value).Msg("bag occurred error")
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
		bs.Overlay.ID = keys.PublicKeyED25519{Key: pubKey}
		if err = c.server.ConnectToNode(ctx, torrent, bs.Overlay, bs.DHT.AddrList); err != nil {
			return errors.Wrapf(err, "failed to connect to bootstrap node %#v", bs.DHT.AddrList.Addresses[0])
		}
	}
	return nil
}

func (c *client) saveTorrent(tr *storage.Torrent, userPubKey *string, bs *string, deletion bool, newVersion *int64) error {
	if err := c.progressStorage.SetTorrent(tr); err != nil {
		return errors.Wrap(err, "failed to save torrent into storage")
	}
	if userPubKey != nil && *userPubKey != "" {
		c.newFilesMx.RLock()
		f := len(c.newFiles[*userPubKey])
		c.newFilesMx.RUnlock()
		maxVal := uint32(f)
		existing, ver, err := c.bagByUser(*userPubKey)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.Header != nil {
				maxVal = max(uint32(f), existing.Header.FilesCount)
			} else {
				maxVal = max(uint32(f), uint32(ver))
			}
		}
		if deletion || (tr.Header == nil && newVersion != nil && *newVersion >= int64(maxVal)) || (tr.Header != nil && tr.Header.FilesCount >= maxVal) {
			if err := c.saveBagPerUser(tr.BagID, newVersion, userPubKey); err != nil {
				return errors.Wrapf(err, "failed to save bag per user")
			}
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
	if tr.Header != nil && len(tr.Header.Data) > 0 {
		k := make([]byte, 3+32)
		copy(k, "th:")
		copy(k[3:], tr.BagID)
		if err := c.db.Put(k, []byte(tr.Header.Data), nil); err != nil {
			return errors.Wrapf(err, "failed to save header for bag %v", hex.EncodeToString(tr.BagID))
		}
	}

	return nil
}

func (c *client) saveBagPerUser(bagID []byte, ver *int64, userPubKey *string) error {
	if userPubKey != nil && *userPubKey != "" {
		k := make([]byte, 3+64)
		copy(k, "ub:")
		copy(k[3:], *userPubKey)
		versionStr := ""
		if ver != nil {
			versionStr = strconv.FormatInt(*ver, 10)
		}
		if err := c.db.Put(k, append(bagID, []byte(versionStr)...), nil); err != nil {
			return errors.Wrapf(err, "failed to save userID:bag mapping for bag %X", bagID)
		}
	}
	return nil
}

func (c *client) startDownloadsFromQueue(ctx context.Context) {
	defer appcontext.GetAppContext(ctx).Recover()
outerLoop:
	for ctx.Err() == nil {
		c.activeDownloadsMx.RLock()
		l := len(c.activeDownloads)
		c.activeDownloadsMx.RUnlock()
		for l < ConcurrentBagsDownloading {
			select {
			case <-ctx.Done():
				log.Info().Str("context", "STORAGE").Msg("download loop stopped")
				return
			case q := <-c.downloadQueue:
				if q.tor == nil {
					continue
				}
				tor := c.progressStorage.GetTorrent(q.tor.BagID)
				if tor == nil {
					continue
				}
				if downloading, _ := tor.IsActive(); downloading || tor.IsCompleted() {
					continue
				}
				usr := ""
				if q.user != nil {
					usr = *q.user
				}
				log.Info().
					Str("context", "STORAGE").
					Hex("bag_id", tor.BagID).
					Str("user", usr).
					Int("queue_length", len(c.downloadQueue)).
					Msg("starting download")
				if err := tor.StartWithCallback(false, true, false, c.torrentStateCallback(tor, q.user)); err != nil {
					log.Error().Err(err).Bytes("bag_id", q.tor.BagID).Msg("failed to start new torrent")
				}
				if q.bootstrap != nil && *q.bootstrap != "" {
					ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
					if err := c.connectToBootstrap(ctx, tor, *q.bootstrap); err != nil {
						log.Warn().Err(err).Hex("bag_id", q.tor.BagID).Msg("failed to connect to bootstrap node, waiting for DHT")
					}
					cancel()
				}
				if err := c.saveTorrent(tor, q.user, q.bootstrap, false, &q.version); err != nil {
					log.Error().Err(err).Hex("bag_id", q.tor.BagID).Msg("failed save updated upload / download torrent state")
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

func (c *client) triggerDownloadOnAllPeers(events ...*model.Event) acceptorFn {
	return func(ctx context.Context, fileHash, userMasterKey, infohash string) error {
		var relays []string
		for _, e := range events {
			if e.Kind == nostr.KindRelayListMetadata {
				relays = model.CollectRelaysFromRelayEvent(e)
				break
			}
		}
		if len(relays) == 0 {
			var err error
			relays, err = fetchUserRelays(ctx, userMasterKey)
			if err != nil {
				log.Warn().Err(err).Str("user", userMasterKey).Msg("failed to fetch user's relays")
				return ErrNoRelays
			}
		}
		if len(relays) == 0 {
			return ErrNoRelays
		}

		var eg errgroup.Group
		log.Info().Strs("relays", relays).Str("user_master_key", userMasterKey).Str("file_hash", fileHash).Msg("triggering download")
		for _, relay := range relays {
			eg.Go(func() error {
				if err := globalClient.Client.triggerDownloadOnRelay(ctx, relay, fileHash, userMasterKey, infohash); err != nil {
					log.Warn().Err(err).Str("relay", relay).Str("user", userMasterKey).Msg("failed to trigger download on relay")
					return err
				}
				return nil
			})
		}
		return errors.Wrapf(eg.Wait(), "failed to trigger storage download on one or more relays")
	}
}

func (c *client) triggerDownloadOnRelay(ctx context.Context, relayUrl, fileHash, masterPubkey, infohash string) (err error) {
	fullStrUrl, err := url.JoinPath(relayUrl, "/files/", masterPubkey+":"+fileHash)
	if err != nil {
		return errors.Wrapf(err, "invalid relay url: %v", relayUrl)
	}
	u, err := url.Parse(fullStrUrl)
	if err != nil {
		return errors.Wrapf(err, "invalid relay url: %v", relayUrl)
	}
	switch u.Scheme {
	case "wss":
		u.Scheme = "https"
	case "ws":
		u.Scheme = "http"
	default:
		return errors.Errorf("unsupported scheme %v", u.Scheme)
	}
	fullStrUrl = u.String()
	values := u.Query()
	values.Set("i", infohash)
	u.RawQuery = values.Encode()
	auth, err := nip98.GenerateAuthHeader(c.config.PrivateKey, "HEAD", "", u)
	if err != nil {
		return errors.Wrapf(err, "failed to generate auth header from relay's key")
	}
	resp, err := req.DefaultClient().EnableInsecureSkipVerify().R().
		SetContext(ctx).
		SetRetryCount(5).
		SetRetryInterval(func(resp *req.Response, attempt int) time.Duration {
			switch {
			case attempt <= 1:
				return 100 * time.Millisecond
			case attempt == 2:
				return 1 * time.Second
			default:
				return 10 * time.Second
			}
		}).
		SetRetryHook(func(resp *req.Response, err error) {
			if err != nil {
				log.Error().Err(err).Str("master_pubkey", masterPubkey).Str("file_hash", fileHash).Str("relay_url", relayUrl).Msg("failed to start storage replication of file, retrying")
			} else {
				log.Error().Str("master_pubkey", masterPubkey).Str("file_hash", fileHash).Str("relay_url", relayUrl).Int("status_code", resp.GetStatusCode()).Msg("failed to start storage replication of file with status, retrying")
			}
		}).
		SetRetryCondition(func(resp *req.Response, err error) bool {
			return err != nil || resp.GetStatusCode() != http.StatusAccepted
		}).
		SetHeader("Authorization", auth).
		SetHeader("Referer", c.config.RelayURL).
		SetQueryString(u.RawQuery).
		Head(fullStrUrl)

	if err != nil {
		return errors.Wrap(err, "failed to start storage replication")
	}
	if resp.GetStatusCode() != http.StatusAccepted {
		return errors.Newf("storage replication service responded with status: %d", resp.GetStatusCode())
	}
	return nil
}

func fetchUserRelays(ctx context.Context, userMasterKey string) (relays []string, err error) {
	evIt := query.GetStoredEvents(ctx,
		model.Filter{
			Authors: []string{userMasterKey},
			Kinds:   []int{nostr.KindRelayListMetadata},
		},
	)
	for ev, iErr := range evIt {
		if iErr != nil {
			return nil, errors.Wrapf(iErr, "failed to fetch user's relays for user %v", userMasterKey)
		}
		relays = model.CollectRelaysFromRelayEvent(ev)
		break
	}
	return relays, nil
}
