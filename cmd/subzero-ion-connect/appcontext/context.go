// SPDX-License-Identifier: ice License 1.0

package appcontext

import (
	"context"
	"runtime/debug"
	"sync"

	"github.com/rs/zerolog/log"
)

type (
	appContext struct {
		context.Context //nolint:containedctx // Custom implementation.
		cancel          context.CancelFunc
		wg              *sync.WaitGroup
	}
	AppContext interface {
		context.Context
		OnShutdown(f func() error)
		Recover()
	}
	WaitForShutdown interface {
		context.Context
		WaitForShutdown()
	}
	appContextKey string
)

const (
	appContextCtxKey appContextKey = "appContext"
)

func GetAppContext(ctx context.Context) AppContext {
	if appCtx, ok := ctx.(AppContext); ok {
		return appCtx
	}
	if cVal := ctx.Value(appContextCtxKey); cVal != nil {
		return cVal.(AppContext)
	}
	log.Panic().Msg("appContext was not initialized")

	return nil
}

func NewAppContext(ctx context.Context) (WaitForShutdown, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	c := &appContext{
		wg:     new(sync.WaitGroup),
		cancel: cancel,
	}
	ctx = context.WithValue(ctx, appContextCtxKey, c)
	c.Context = ctx
	return c, cancel
}
func (c *appContext) WaitForShutdown() {
	c.wg.Wait()
}
func (c *appContext) OnShutdown(f func() error) {
	c.wg.Go(func() {
		<-c.Done()
		if err := f(); err != nil {
			log.Error().Err(err).Msg("failed to shutdown")
		}
	})
}

func (c *appContext) Recover() {
	if pErr := recover(); pErr != nil {
		if err, isErr := pErr.(error); isErr {
			log.Error().Str("stack", string(debug.Stack())).Err(err).Msg("panic")
			c.cancel()
		} else {
			log.Error().Str("stack", string(debug.Stack())).Interface("err", pErr).Msg("panic")
			c.cancel()
		}
	}
}
