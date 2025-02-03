// SPDX-License-Identifier: ice License 1.0

package model

import (
	"context"
)

type (
	UserDataContext struct {
		PublicKey       string
		MasterPublicKey string
		Authenticated   bool
		Kinds           map[int]struct{}
	}
	userKey string
)

const (
	userKeyCtx userKey = "subzero_metadata_user"
)

func GetUserDataFromContext(ctx context.Context) (master, pub string, authenticated bool, kinds map[int]struct{}) {
	if data, ok := ctx.Value(userKeyCtx).(*UserDataContext); ok {
		return data.MasterPublicKey, data.PublicKey, data.Authenticated, data.Kinds
	}
	return "", "", false, nil
}

func SetUserDataInContext(ctx context.Context, master, pk string, authenticated bool, kinds map[int]struct{}) context.Context {
	return context.WithValue(ctx, userKeyCtx, &UserDataContext{
		PublicKey:       pk,
		MasterPublicKey: master,
		Authenticated:   authenticated,
		Kinds:           kinds,
	})
}

func (u UserDataContext) IsKindAllowed(kind int) bool {
	if len(u.Kinds) == 0 {
		return true
	}

	_, ok := u.Kinds[kind]
	return ok
}

func (u UserDataContext) IsFilterAllowed(filter ...Filter) bool {
	if len(u.Kinds) == 0 {
		return true
	}

	for _, f := range filter {
		for _, kind := range f.Kinds {
			if _, ok := u.Kinds[kind]; !ok {
				return false
			}
		}
	}
	return true
}

func (u UserDataContext) IsEventAllowed(event ...*Event) bool {
	if len(u.Kinds) == 0 {
		return true
	}

	for _, e := range event {
		if _, ok := u.Kinds[e.Kind]; !ok {
			return false
		}
	}
	return true
}
