// SPDX-License-Identifier: ice License 1.0

package metadata

import "path/filepath"

type genericMetaExtractor struct{}

func newGenericExtractor() Extractor {
	return &genericMetaExtractor{}
}

func (*genericMetaExtractor) Extract(filePath, _ string, size uint64) (*Metadata, error) {
	ext := filepath.Ext(filePath)
	return &Metadata{
		Ext:  ext,
		Size: size,
	}, nil
}

func (*genericMetaExtractor) Close() error {
	return nil
}
