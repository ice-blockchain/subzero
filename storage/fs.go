// SPDX-License-Identifier: ice License 1.0

package storage

import (
	"github.com/xssnick/tonutils-storage/db"
	"io"
	"math"
	"os"
	"path/filepath"

	"github.com/xssnick/tonutils-storage/storage"
)

func init() {
	db.CachedFDLimit = math.MaxInt64
}

type fs struct{}
type fd struct {
	f *os.File
}

func (f *fd) Get() io.ReaderAt {
	return f.f
}
func (f *fd) Close() error {
	return f.f.Close()
}

func newFS() storage.FSController {
	return &fs{}
}

func (f *fs) AcquireRead(path string, p []byte, offset int64) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return -1, err
	}
	return file.ReadAt(p, offset)
}

func (fs *fs) Free(f *fd) {
	f.Close()
}
func (fs *fs) RemoveFile(p string) error {
	return os.Remove(filepath.Clean(p))
}
