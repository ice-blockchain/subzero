// SPDX-License-Identifier: ice License 1.0

package fixture

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/cockroachdb/errors"
	"io"
	"os"
	"time"

	"github.com/fsnotify/fsnotify"
)

func WaitForFile(ctx context.Context, watchPath, expectedPath, expectedHash string, expectedSize int64) (hash string, err error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	skipWatch := false
	fileInfo, err := os.Stat(expectedPath)
	if err == nil && fileInfo.Size() == expectedSize {
		skipWatch = true
	}
	if !skipWatch {
		if err = watchFile(ctx, watchPath, expectedPath, expectedSize); err != nil {
			return "", errors.Wrapf(err, "failed to monitor file %v", expectedPath)
		}
	}
	f, err := os.Open(expectedPath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to open %v to check hash", expectedPath)
	}
	defer f.Close()
	hashCalc := sha256.New()
	var n int64
	if n, err = io.Copy(hashCalc, f); err != nil {
		return "", errors.Wrapf(err, "failed to calc hash of %v", expectedPath)
	}
	if n != expectedSize {
		return WaitForFile(ctx, watchPath, expectedPath, expectedHash, expectedSize)
	}
	hash = hex.EncodeToString(hashCalc.Sum(nil))
	if hash != expectedHash {
		return WaitForFile(ctx, watchPath, expectedPath, expectedHash, expectedSize)
	}
	return hash, nil
}

func watchFile(ctx context.Context, monitorPath, expectedPath string, expectedSize int64) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return errors.Wrapf(err, "failed to create fsnotify watcher %s", expectedPath)
	}
	defer watcher.Close()
	err = watcher.Add(monitorPath)
	if err != nil {
		return errors.Wrapf(err, "failed to monitor expectedPath %s", expectedPath)
	}
loop:
	for {
		select {
		case event := <-watcher.Events:
			if event.Op == fsnotify.Write && event.Name == expectedPath {
				fileInfo, err := os.Stat(expectedPath)
				if err != nil {
					return errors.Wrapf(err, "failed to stat file %s", expectedPath)
				}
				if fileInfo.Size() == expectedSize {
					break loop
				}
			}
		case <-time.After(1 * time.Second):
			fileInfo, err := os.Stat(expectedPath)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue loop
				}
				return errors.Wrapf(err, "failed to stat file %s", expectedPath)
			}
			if fileInfo.Size() == expectedSize {
				break loop
			}
		case err = <-watcher.Errors:
			return errors.Wrapf(err, "got error from fsnotify")
		case <-ctx.Done():
			return errors.New("timeout")
		}
	}
	return nil
}
