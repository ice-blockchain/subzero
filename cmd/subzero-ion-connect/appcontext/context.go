// SPDX-License-Identifier: ice License 1.0

package appcontext

import (
	"context"
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
		WaitForShutdown()
	}
	WaitForShutdown interface {
		context.Context
		WaitForShutdown()
	}
)

func GetAppContext(ctx context.Context) AppContext {
	if appCtx, ok := ctx.(AppContext); ok {
		return appCtx
	}
	if cVal := ctx.Value("appContext"); cVal != nil {
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
	ctx = context.WithValue(ctx, "appContext", c)
	c.Context = ctx
	return c, cancel
}
func (c *appContext) WaitForShutdown() {
	c.wg.Wait()
}
func (c *appContext) OnShutdown(f func() error) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		<-c.Done()
		if err := f(); err != nil {
			log.Error().Err(err).Msg("failed to shutdown")
		}
	}()
}
