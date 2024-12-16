// SPDX-License-Identifier: ice License 1.0

package model

import (
	"context"
)

type (
	userData struct {
		PublicKey       string
		MasterPublicKey string
		Authenticated   bool
	}
	userKey string
)

const (
	userKeyCtx userKey = "subzero_metadata_user"
)

func GetUserDataFromContext(ctx context.Context) (master, pub string, authenticated bool) {
	if data, ok := ctx.Value(userKeyCtx).(*userData); ok {
		return data.MasterPublicKey, data.PublicKey, data.Authenticated
	}
	return "", "", false
}

func SetUserDataInContext(ctx context.Context, master, pk string, authenticated bool) context.Context {
	return context.WithValue(ctx, userKeyCtx, &userData{
		PublicKey:       pk,
		MasterPublicKey: master,
		Authenticated:   authenticated,
	})
}
