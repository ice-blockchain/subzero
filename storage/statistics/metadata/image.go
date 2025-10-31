// SPDX-License-Identifier: ice License 1.0

package metadata

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/davidbyttow/govips/v2/vips"

	"github.com/ice-blockchain/subzero/log"
)

type imageMetaExtractor struct{}

type ImageMetadata struct {
	Width  int
	Height int
}

func newImageExtractor() Extractor {
	vips.LoggingSettings(func(messageDomain string, messageLevel vips.LogLevel, message string) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		switch messageLevel {
		case vips.LogLevelError:
			log.Error(ctx, errors.Errorf("%v:%v", messageDomain, message))
		case vips.LogLevelCritical, vips.LogLevelWarning:
			log.Warn(ctx, fmt.Sprintf("%v:%v", messageDomain, message))
		case vips.LogLevelInfo:
			log.Info(ctx, fmt.Sprintf("%v:%v", messageDomain, message))
		case vips.LogLevelDebug:
			log.Debug(ctx, fmt.Sprintf("%v:%v", messageDomain, message))
		case vips.LogLevelMessage:
			log.Trace(ctx, fmt.Sprintf("%v:%v", messageDomain, message))
		}
	}, vips.LogLevelInfo)
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
