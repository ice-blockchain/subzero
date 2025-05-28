// SPDX-License-Identifier: ice License 1.0

package metadata

import (
	"io"
	"strings"

	"github.com/cockroachdb/errors"
)

type Extractor interface {
	io.Closer
	Extract(filePath, contentType string, size uint64) (*Metadata, error)
}
type Metadata struct {
	TypeMeta any
	Ext      string
	Size     uint64
}
type extractor struct {
	extractorsByFileType map[string]Extractor
	generic              *genericMetaExtractor
}

func NewExtractor() Extractor {
	return &extractor{
		extractorsByFileType: map[string]Extractor{
			"video": newVideoExtractor(),
			"image": newImageExtractor(),
		},
	}
}
func (e *extractor) Extract(filePath, contentType string, size uint64) (*Metadata, error) {
	fileType := strings.Split(contentType, "/")[0]
	if ext, hasExtractor := e.extractorsByFileType[fileType]; hasExtractor {
		return ext.Extract(filePath, contentType, size)
	}
	return e.generic.Extract(filePath, contentType, size)
}
func (e *extractor) Close() (err error) {
	for k, ex := range e.extractorsByFileType {
		err = errors.Join(err, errors.Wrapf(ex.Close(), "failed to close %v meta extractor", k))
	}
	err = errors.Join(err, errors.Wrapf(e.generic.Close(), "failed to close generic meta extractor"))

	return err
}
