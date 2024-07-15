package internal

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/quic-go/quic-go/http3"

	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	"github.com/ice-blockchain/subzero/server/ws/internal/http2"
)

func WithWS(wsHandler WSHandler, httpHandler http.Handler) gin.HandlerFunc {
	return func(ginCtx *gin.Context) {
		w := ginCtx.Writer.(http2.Unwrapper).Unwrap()
		_, isHttp3 := w.(http3.Hijacker)
		if isHttp3 {
			server(ginCtx.Request.Context()).H3Server.HandleWS(wsHandler, httpHandler, w, ginCtx.Request)
		} else {
			server(ginCtx.Request.Context()).H2Server.HandleWS(wsHandler, httpHandler, ginCtx.Writer, ginCtx.Request)
		}
	}
}

func server(ctx context.Context) *Srv {
	return ctx.Value(adapters.CtxKeyServer).(*Srv)
}
