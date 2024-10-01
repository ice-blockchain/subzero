// SPDX-License-Identifier: ice License 1.0

package storage

import (
	"io"
	"os"

	"github.com/xssnick/tonutils-storage/storage"
)

func init() {
	storage.Fs = newFS()
}

type fs struct{}
type fd struct {
	f *os.File
}

func (f *fd) Get() io.ReaderAt {
	return f.f
}

func newFS() storage.FSController {
	return &fs{}
}

func (f *fs) Acquire(path string) (storage.FDesc, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &fd{file}, nil
}

func (fs *fs) Free(f storage.FDesc) {
	f.(*fd).f.Close()
}
