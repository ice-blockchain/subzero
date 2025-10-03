// SPDX-License-Identifier: ice License 1.0

//go:build test

package appcontext

import (
	"context"
)

func TContext(t interface{ Context() context.Context }) context.Context {
	ctx, _ := NewAppContext(t.Context())
	return ctx
}
