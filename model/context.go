// SPDX-License-Identifier: ice License 1.0

package model

import (
	"context"
)

type (
	userData struct {
		PublicKey     string
		Authenticated bool
	}
	userKey string
)

const (
	userKeyCtx userKey = "subzero_metadata_user"
)

func GetUserDataFromContext(ctx context.Context) (pk string, authenticated bool) {
	if data, ok := ctx.Value(userKeyCtx).(*userData); ok {
		return data.PublicKey, data.Authenticated
	}
	return "", false
}

func SetUserDataInContext(ctx context.Context, pk string, authenticated bool) context.Context {
	return context.WithValue(ctx, userKeyCtx, &userData{
		PublicKey:     pk,
		Authenticated: authenticated,
	})
}
