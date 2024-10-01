// SPDX-License-Identifier: ice License 1.0

package metadata

import (
	"io"
	"strings"

	"github.com/gookit/goutil/errorx"
	"github.com/hashicorp/go-multierror"
)

type Extractor interface {
	io.Closer
	Extract(filePath, contentType string, size uint64) (*Metadata, error)
}
type Metadata struct {
	Ext      string
	Size     uint64
	TypeMeta any
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
func (e *extractor) Close() error {
	var mErr *multierror.Error
	for k, ex := range e.extractorsByFileType {
		if clErr := ex.Close(); clErr != nil {
			mErr = multierror.Append(mErr, errorx.Withf(clErr, "failed to close %v meta extractor", k))
		}

	}
	if err := e.generic.Close(); err != nil {
		mErr = multierror.Append(mErr, errorx.Withf(err, "failed to close generic meta extractor"))
	}
	return mErr.ErrorOrNil()
}
