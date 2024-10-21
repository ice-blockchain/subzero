// SPDX-License-Identifier: ice License 1.0

package metadata

import (
	"path/filepath"

	"github.com/cockroachdb/errors"
	"github.com/davidbyttow/govips/v2/vips"
)

type imageMetaExtractor struct{}

type ImageMetadata struct {
	Width  int
	Height int
}

func newImageExtractor() Extractor {
	vips.Startup(nil)
	return &imageMetaExtractor{}
}

func (i *imageMetaExtractor) Extract(filePath, _ string, size uint64) (*Metadata, error) {
	ext := filepath.Ext(filePath)
	im, err := vips.LoadImageFromFile(filePath, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load image %v", filePath)
	}
	defer im.Close()
	return &Metadata{
		Ext:  ext,
		Size: size,
		TypeMeta: &ImageMetadata{
			Width:  im.Width(),
			Height: im.Height(),
		},
	}, nil
}

func (*imageMetaExtractor) Close() error {
	vips.Shutdown()
	return nil
}
