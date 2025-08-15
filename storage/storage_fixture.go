// SPDX-License-Identifier: ice License 1.0

//go:build test

package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/require"
)

func calcFileHash(t *testing.T, path string) (string, error) {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		return "", errors.Wrapf(err, "failed to open %v to check hash", path)
	}
	defer f.Close()
	hashCalc := sha256.New()

	if _, err = f.WriteTo(hashCalc); err != nil {
		return "", errors.Wrapf(err, "failed to calc hash of %v", path)
	}
	return hex.EncodeToString(hashCalc.Sum(nil)), nil
}

func WaitForFile(t *testing.T, ctx context.Context, watchPath, expectedPath, expectedHash string, expectedSize int64) (hash string, err error) {
	t.Helper()

	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	t.Logf("Waiting for file %v to be created with hash %v and size %d", expectedPath, expectedHash, expectedSize)

	skipWatch := false
	fileInfo, err := os.Stat(expectedPath)
	if err == nil && fileInfo.Size() == expectedSize {
		skipWatch = true
	}
	if !skipWatch {
		if err = watchFile(t, ctx, watchPath, expectedPath, expectedSize); err != nil {
			return "", errors.Wrapf(err, "failed to monitor file %v", expectedPath)
		}
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		select {
		case <-ticker.C:
			hash, err = calcFileHash(t, expectedPath)
			if err != nil {
				return "", errors.Wrapf(err, "failed to calculate hash of file %s", expectedPath)
			}
			if hash == expectedHash {
				return hash, nil
			}
			t.Logf("File %s hash %s does not match expected hash %s, waiting for it to be populated", expectedPath, hash, expectedHash)

		case <-ctx.Done():
			return "", errors.Wrapf(ctx.Err(), "context cancelled while waiting for file %s", expectedPath)
		}
	}
	return "", ctx.Err()
}

func watchFile(t *testing.T, ctx context.Context, monitorPath, expectedPath string, expectedSize int64) error {
	t.Helper()

	watcher, err := fsnotify.NewWatcher()
	require.NoError(t, err, "failed to create fsnotify watcher for %s", monitorPath)

	defer watcher.Close()
	err = watcher.Add(monitorPath)
	require.NoError(t, err, "failed to add watcher for %s", monitorPath)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	logTicker := time.NewTicker(10 * time.Second)
	defer logTicker.Stop()

loop:
	for ctx.Err() == nil {
		select {
		case event := <-watcher.Events:
			if event.Name != expectedPath {
				continue loop
			}

			t.Logf("Received event %v for file %s", event, expectedPath)
			if event.Op == fsnotify.Write {
				fileInfo, err := os.Stat(expectedPath)
				if err != nil {
					return errors.Wrapf(err, "failed to stat file %s", expectedPath)
				}
				if fileInfo.Size() == expectedSize {
					break loop
				}
				t.Logf("Received event %v for file %s, but size does not match expected just yet (expected size %d, actual size %d), continuing to wait",
					event, expectedPath, expectedSize, fileInfo.Size())
			}

		case <-logTicker.C:
			_, err := os.Stat(expectedPath)
			if errors.Is(err, os.ErrNotExist) {
				t.Logf("File %s does not exist yet, continuing to wait", expectedPath)
			}

		case <-ticker.C:
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
			t.Logf("File %s exists but size %d does not match expected size %d, continuing to wait", expectedPath, fileInfo.Size(), expectedSize)

		case err = <-watcher.Errors:
			return errors.Wrapf(err, "got error from fsnotify")

		case <-ctx.Done():
			return errors.Wrapf(ctx.Err(), "context cancelled while waiting for file %s", expectedPath)
		}
	}
	return ctx.Err()
}

func Reset() {
	globalClient.Once = sync.Once{}
}
