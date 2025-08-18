// SPDX-License-Identifier: ice License 1.0

package storage

import (
	"context"
)

type storageContextFileNameKey string

const (
	storageContextFileNameValue storageContextFileNameKey = "FileName"
)

func WithFileNameInContext(ctx context.Context, fileName string) context.Context {
	return context.WithValue(ctx, storageContextFileNameValue, fileName)
}

func FileNameFromContext(ctx context.Context) string {
	fileName, ok := ctx.Value(storageContextFileNameValue).(string)
	if !ok {
		return ""
	}
	return fileName
}
