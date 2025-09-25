// SPDX-License-Identifier: ice License 1.0

package model

import (
	"context"
)

type (
	UserDataContext struct {
		Kinds           map[int]struct{}
		PublicKey       string
		MasterPublicKey string
		UserAgent       string
		Authenticated   bool
		Authoritative   bool
	}
	userKey string
)

const (
	userKeyCtx userKey = "subzero_metadata_user"
)

func GetUserDataFromContext(ctx context.Context) (value UserDataContext) {
	if data, ok := ctx.Value(userKeyCtx).(*UserDataContext); ok {
		value = *data
	}
	return value
}

func SetUserDataInContext(ctx context.Context, value UserDataContext) context.Context {
	return context.WithValue(ctx, userKeyCtx, &value)
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
