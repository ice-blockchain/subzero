// SPDX-License-Identifier: ice License 1.0

package storage

import (
	"context"
)

type storageContextFileNameKey string
type storageContextCdnUpload string

const (
	storageContextFileNameValue  storageContextFileNameKey = "FileName"
	storageContextCdnUploadValue storageContextCdnUpload   = "cdnUpload"
	syncUpload                                             = "sync"
)

func WithFileNameInContext(ctx context.Context, fileName string) context.Context {
	return context.WithValue(ctx, storageContextFileNameValue, fileName)
}
func WithSyncCdnUpload(ctx context.Context) context.Context {
	return context.WithValue(ctx, storageContextCdnUploadValue, syncUpload)
}

func FileNameFromContext(ctx context.Context) string {
	fileName, ok := ctx.Value(storageContextFileNameValue).(string)
	if !ok {
		return ""
	}
	return fileName
}
func SyncCdnUpload(ctx context.Context) bool {
	cdnUploadMode, ok := ctx.Value(storageContextCdnUploadValue).(string)
	if !ok {
		return false
	}
	return cdnUploadMode == syncUpload
}
