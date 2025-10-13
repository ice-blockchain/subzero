// SPDX-License-Identifier: ice License 1.0

package storage

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	gomime "github.com/cubewise-code/go-mime"
	"github.com/rs/zerolog/log"
	"github.com/xssnick/tonutils-go/adnl/dht"
	"github.com/xssnick/tonutils-go/adnl/keys"
	"github.com/xssnick/tonutils-go/adnl/overlay"
	"github.com/xssnick/tonutils-go/tl"
	"github.com/xssnick/tonutils-storage/storage"
)

func (c *client) StartUpload(ctx context.Context, now time.Time, userPubKey, masterPubKey, relativePathToFileForUrl, hash string, newFile *FileMetaInput) (bagID, url string, existed bool, err error) {
	existingBagForUser, _, err := c.bagByUser(masterPubKey)
	if err != nil {
		return "", "", false, errors.Wrapf(err, "failed to find existing bag for user %s", masterPubKey)
	}
	var existingHDData []byte
	var existingHD headerData
	if existingBagForUser != nil {
		if existingBagForUser.Header != nil && len(existingBagForUser.Header.Data) > 0 {
			existingHDData = existingBagForUser.Header.Data
		} else {
			if existingHDData, err = c.latestHeaderForBag(existingBagForUser.BagID); err != nil {
				return "", "", false, errors.Wrapf(err, "failed to get header for bag %v", hex.EncodeToString(existingBagForUser.BagID))
			}
		}
		if len(existingHDData) > 0 {
			if err = json.Unmarshal(existingHDData, &existingHD); err != nil {
				return "", "", false, errors.Wrapf(err, "corrupted header metadata for bag %v", hex.EncodeToString(existingBagForUser.BagID))
			}
		}
	}
	_, existed = existingHD.FileHash[hash]
	if existed && newFile != nil {
		if existingBagForUser != nil {
			url, err = c.DownloadUrl(masterPubKey, hash)
			if err != nil {
				if errors.Is(err, storage.ErrFileNotExist) {
					existed = false
				} else {
					return "", "", false,
						errors.Wrapf(err, "failed to build download url for already existing file %v/%v(%v)", masterPubKey, relativePathToFileForUrl, hash)
				}
			}
			if existed {
				bagID = hex.EncodeToString(existingBagForUser.BagID)
				bootstrapNode, err := c.buildBootstrapNodeInfo(existingBagForUser)
				if err != nil {
					return "", "", false, errors.Wrap(err, "failed to build bootstrap node info")
				}
				bs := []*Bootstrap{bootstrapNode}
				b, err := json.Marshal(bs)
				if err != nil {
					return "", "", false, errors.Wrapf(err, "failed to marshal %#v", bs)
				}
				bootstrap := base64.StdEncoding.EncodeToString(b)
				version := int64(0)
				if existingBagForUser.Header != nil {
					version = int64(existingBagForUser.Header.FilesCount)
				}

				return bagID + ":" + bootstrap + ":" + strconv.FormatInt(version, 10), url, existed, nil
			}

		}
	}
	var bs []*Bootstrap
	var bag *storage.Torrent
	bag, bs, err = c.upload(ctx, now, userPubKey, masterPubKey, relativePathToFileForUrl, hash, newFile, &existingHD)
	if err != nil {
		return "", "", false, errors.Wrapf(err, "failed to start upload of %v", relativePathToFileForUrl)
	}
	bagID = hex.EncodeToString(bag.BagID)
	log.Info().
		Str("context", "STORAGE").
		Str("file_path", relativePathToFileForUrl).
		Str("user", masterPubKey).
		Str("hash", hash).
		Str("bag_id", bagID).
		Uint32("total_files", bag.Header.FilesCount).
		Msg("new upload resulted in bag")
	if newFile != nil && c.config.Debug {
		uplFile, err := bag.GetFileOffsets(relativePathToFileForUrl)
		if err != nil {
			return "", "", false, errors.Wrapf(err, "failed to get just created file from new bag")
		}
		fullFilePath := filepath.Join(c.rootStoragePath, masterPubKey, relativePathToFileForUrl)
		go c.stats.ProcessFile(ctx, fullFilePath, gomime.TypeByExtension(filepath.Ext(fullFilePath)), uplFile.Size)
	}
	b, err := json.Marshal(bs)
	if err != nil {
		return "", "", false, errors.Wrapf(err, "failed to marshal %#v", bs)
	}
	bootstrap := base64.StdEncoding.EncodeToString(b)
	var fileNameForCdn string
	url, fileNameForCdn, err = c.buildUrl(bagID, relativePathToFileForUrl, masterPubKey, hash, bootstrap)
	if err != nil {
		return "", "", false, errors.Wrapf(err, "failed to build url for %v (bag %v)", relativePathToFileForUrl, bagID)
	}
	if c.config.Cdn.URLUpload != "" && c.config.Cdn.AccessKey != "" && c.cdn != nil && newFile != nil {
		fullFilePath := filepath.Join(c.rootStoragePath, masterPubKey, relativePathToFileForUrl)
		if SyncCdnUpload(ctx) {
			f, ferr := os.Open(fullFilePath)
			if ferr != nil {
				return "", "", false, errors.Wrapf(ferr, "failed to open %v", fullFilePath)
			}
			defer f.Close()
			if err = c.cdn.FileUpload(ctx, f, newFile.ContentType, fileNameForCdn); err != nil {
				return "", "", false, errors.Wrapf(err, "failed to upload file %v to cdn", fileNameForCdn)
			}
		} else {
			if err = c.cdn.FileUploadAsync(ctx, fullFilePath, newFile.ContentType, fileNameForCdn); err != nil {
				return "", "", false, errors.Wrapf(err, "failed to enqueue file upload %v to cdn", fileNameForCdn)
			}
		}
	}
	return bagID + ":" + bootstrap + ":" + strconv.FormatInt(int64(bag.Header.FilesCount), 10), url, existed, err
}

func (c *client) upload(ctx context.Context, now time.Time, user, master, relativePath, hash string, fileMeta *FileMetaInput, headerMetadata *headerData) (torrent *storage.Torrent, bootstrap []*Bootstrap, err error) {
	rootUserPath, _ := c.BuildUserPath(master, "")
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
	rTime := time.Now()
	var refs []storage.FileRef
	for relativeFilePath := range headerMD.FileMetadata {
		ref, frefErr := c.progressStorage.GetSingleFileRef(filepath.Join(rootUserPath, relativeFilePath))
		if frefErr != nil {
			if os.IsNotExist(frefErr) || errors.Is(frefErr, os.ErrNotExist) {
				delete(headerMD.FileMetadata, relativeFilePath)
				continue
			}
			return nil, nil, errors.Wrapf(frefErr, "failed to detect shareable files: %v", relativeFilePath)
		}
		refs = append(refs, ref)
	}
	log.Trace().Str("context", "STORAGE").
		Str("master", master).
		Str("hash", hash).
		Dur("duration_since_start", time.Since(rTime)).
		Dur("total_duration", time.Since(now)).
		Msg("building refs")

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
	iTime := time.Now()
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
	log.Trace().Str("context", "STORAGE").
		Str("master", master).
		Str("hash", hash).
		Dur("duration_since_start", time.Since(iTime)).
		Dur("total_duration", time.Since(now)).
		Msg("bag hashing and start")
	sTime := time.Now()
	err = c.saveUploadTorrent(tr, master, fileMeta == nil)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to save updated bag")
	}
	log.Trace().Str("context", "STORAGE").
		Str("master", master).
		Str("hash", hash).
		Dur("duration_since_start", time.Since(sTime)).
		Dur("total_duration", time.Since(now)).
		Msg("save")

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
		ID:        keys.PublicKeyED25519{Key: key.Public().(ed25519.PublicKey)},
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

func (c *client) buildUrl(bagID, relativePath, masterPubkey, fileHash string, bootstrap string) (fullUrl string, fileName string, err error) {
	fName := fmt.Sprintf("%v:%v%v", masterPubkey, fileHash, filepath.Ext(relativePath))
	if c.config.IONLibertyDisabled {
		relayUrl, err := url.Parse(c.config.RelayURL)
		if err != nil {
			return "", "", errors.Wrapf(err, "invalid relay-url configured %v", c.config.RelayURL)
		}

		return fmt.Sprintf("https://%v:%v/files/%v", relayUrl.Hostname(), relayUrl.Port(), fName), fName, nil
	}
	url := fmt.Sprintf("http://%v.bag/%v?bootstrap=%v", bagID, relativePath, bootstrap)

	return url, fName, nil
}

func (c *client) saveUploadTorrent(tr *storage.Torrent, userPubKey string, deletion bool) error {
	if err := c.saveTorrent(tr, &userPubKey, nil, deletion, nil); err != nil {
		return errors.Wrap(err, "failed to save upload torrent into storage")
	}
	c.newFilesMx.Lock()
	for k := range c.newFiles[userPubKey] {
		if _, err := tr.GetFileOffsets(k); err == nil {
			meta, _ := c.fileMeta(tr)
			if _, hasMeta := meta.FileMetadata[k]; hasMeta {
				delete(c.newFiles[userPubKey], k)
			}
		}
	}
	c.newFilesMx.Unlock()
	return nil
}
func (c *client) SaveFile(ctx context.Context, now time.Time, masterPubKey string, r *http.Request, maxSize uint64) (string, *FileMetaInput, []byte, error) {
	storagePath, _ := c.BuildUserPath(masterPubKey, "")
	input := &FileMetaInput{
		CreatedAt: uint64(now.UnixNano()),
	}
	var newName string
	var hash []byte
	if r != nil {
		reader, err := r.MultipartReader()
		if err != nil {
			return "", nil, nil, err
		}
		hashCalc := sha256.New()
		var fileName, contentType string
		var fileSize uint64
		for ctx.Err() == nil {
			part, err := reader.NextPart()
			if err != nil {
				if err == io.EOF {
					break
				}
				return "", nil, nil, errors.Wrap(err, "failed to read multipart")
			}
			switch part.FormName() {
			case "file":
				fStart := time.Now()
				if part.FileName() == "" || !filepath.IsLocal(part.FileName()) {
					return "", nil, nil, errors.Wrapf(ErrValidationFailed, "invalid filename %q, must be provided", part.FileName())
				}
				fileName = part.FileName()
				if contentType == "" {
					contentType = gomime.TypeByExtension(filepath.Ext(fileName))
				}
				uploadingFilePath := filepath.Join(storagePath, fileName)
				if err = os.MkdirAll(filepath.Dir(uploadingFilePath), 0o744); err != nil {
					log.Error().Str("context", "STORAGE").Err(err).Msg("failed to open temp file while processing upload")
					return "", nil, nil, errors.Wrapf(err, "failed to create tmp dir")
				}
				userDir, err := os.OpenRoot(storagePath)
				if err != nil {
					return "", nil, nil, errors.Wrap(err, "failed to open user folder while processing upload")
				}
				fileUploadTo, err := userDir.OpenFile(fileName, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
				if err != nil {
					return "", nil, nil, errors.Wrap(err, "failed to open temp file while processing upload")
				}
				defer func() {
					fileUploadTo.Sync()
					fileUploadTo.Close()
				}()
				written, err := io.Copy(io.MultiWriter(fileUploadTo, hashCalc), part)
				if err != nil {
					log.Error().
						Str("context", "STORAGE").
						Err(err).
						Str("filename", fileName).
						Msg("failed to copy file")
					return "", nil, nil, errors.Wrapf(err, "failed to copy file %v", fileName)
				}
				fileSize += uint64(written)
				if fileSize > maxSize {
					part.Close()
					defer os.Remove(uploadingFilePath)
					return "", &FileMetaInput{FileSize: fileSize}, nil, ErrFileTooBig
				}
				log.Trace().Str("context", "STORAGE").
					Str("master_pubkey", masterPubKey).
					Str("file_name", fileName).
					Dur("duration_since_start", time.Since(fStart)).
					Dur("total_duration", time.Since(now)).
					Int64("bytes", written).
					Msg("file processing")

			case "media_type":
				var mediaType string
				mediaType, err = readString(part, "media_type")
				if mediaType != "" && mediaType != MediaTypeAvatar && mediaType != MediaTypeBanner {
					return "", nil, nil, errors.Wrapf(ErrValidationFailed, "invalid media type %q, must be provided", mediaType)
				}
			case "content_type":
				contentType, err = readString(part, "content_type")
				if contentType == "" {
					if fileName != "" {
						contentType = gomime.TypeByExtension(filepath.Ext(fileName))
					}
				}
			case "caption":
				input.Caption, err = readString(part, "caption")
			case "alt":
				input.Alt, err = readString(part, "alt")
			}
			if err != nil {
				return "", nil, nil, errors.Wrap(err, "failed read multipart")
			}
			part.Close()
		}
		hStart := time.Now()
		hash = hashCalc.Sum(nil)
		input.Hash = hash
		input.ContentType = contentType
		input.FileSize = fileSize
		log.Trace().Str("context", "STORAGE").
			Str("master_pubkey", masterPubKey).
			Str("file_name", fileName).
			Dur("duration_since_start", time.Since(hStart)).
			Dur("total_duration", time.Since(now)).
			Msg("hash")
		hexHash := hex.EncodeToString(hash)
		newName = hexHash + filepath.Ext(fileName)
		if err = os.Rename(filepath.Join(storagePath, fileName), filepath.Join(storagePath, newName)); err != nil {
			log.Error().
				Str("context", "STORAGE").
				Err(err).
				Str("file_name", fileName).
				Str("new_name", newName).
				Msg("failed to rename file to hash")
			return "", nil, nil, errors.Wrapf(err, "failed to rename file %v %v", fileName, newName)
		}
	}
	if newName == "" {
		newName = FileNameFromContext(ctx)
	}
	c.newFilesMx.Lock()
	if userNewFiles, hasNewFiles := c.newFiles[masterPubKey]; !hasNewFiles || userNewFiles == nil {
		c.newFiles[masterPubKey] = make(map[string]*FileMetaInput)
	}
	c.newFiles[masterPubKey][newName] = input
	c.newFilesMx.Unlock()
	input.Filename = newName
	return filepath.Join(storagePath, newName), input, hash, nil
}

func readString(part *multipart.Part, name string) (string, error) {
	bufSize := 1024
	b := make([]byte, bufSize)
	read, err := part.Read(b)
	if err != nil {
		if err == io.EOF {
			return string(b[:read]), nil
		}
		return "", errors.Wrapf(err, "failed to read %v", name)
	}
	return string(b[:read]), nil
}
