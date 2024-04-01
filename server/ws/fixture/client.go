package fixture

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/gookit/goutil/errorx"
	"github.com/hashicorp/go-multierror"
	h2ec "github.com/ice-blockchain/go/src/net/http"
	"github.com/ice-blockchain/subzero/server/ws/internal/adapters"
	connectwsupgrader "github.com/ice-blockchain/subzero/server/ws/internal/connect-ws-upgrader"
	"github.com/nbd-wtf/go-nostr"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/quic-go/quicvarint"
	"github.com/quic-go/webtransport-go"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

func NewWebTransportClientHttp3(ctx context.Context, url string) (Client, error) {
	d := webtransport.Dialer{
		RoundTripper: http3RoundTripper(),
	}
	d.TLSClientConfig = localhostTLS()
	_, conn, err := d.Dial(ctx, url, nil)
	if err != nil {
		return nil, errorx.Withf(err, "failed to establish webtransport conn to %v", url)
	}
	stream, err := conn.OpenStream()
	if err != nil {
		return nil, errorx.Withf(err, "failed to open webtransport stream to %v", url)
	}
	wt, closectx := adapters.NewWebTransportAdapter(ctx, conn, stream, 0, 0)
	go wt.Write(closectx)
	c := &wtransportClient{
		wt:            wt.(*adapters.WebtransportAdapter),
		inputMessages: make(chan []byte),
	}
	go c.read(closectx)
	return c, nil
}

func NewWebsocketClientHttp3(ctx context.Context, urlStr string) (Client, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	h := http.Header{}
	h.Set("Sec-Websocket-Version", "13")
	req := &http.Request{
		Method: http.MethodConnect,
		Header: h,
		Proto:  "websocket",
		Host:   u.Host,
		URL:    u,
	}
	req = req.WithContext(ctx)
	rt := http3RoundTripper()
	rsp, err := rt.RoundTripOpt(req, http3.RoundTripOpt{DontCloseRequestStream: true})
	if err != nil {
		return nil, err
	}
	if rsp.StatusCode < 200 || rsp.StatusCode >= 300 {
		return nil, errorx.Errorf("received status %d", rsp.StatusCode)
	}
	stream := rsp.Body.(http3.HTTPStreamer).HTTPStream()
	streamCreator := rsp.Body.(http3.Hijacker).StreamCreator()
	if err != nil {
		return nil, err
	}
	conn := connectwsupgrader.NewHttp3Proxy(stream, streamCreator)
	c, _ := clientWebSocketAdapter(ctx, conn, 0, 0)
	go func() {
		defer c.Close()
		c.read(ctx)
	}()

	return c, nil
}

func NewWebsocketClientHttp2(ctx context.Context, urlStr string) (Client, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	h := http.Header{}
	h.Set("Sec-Websocket-Version", "13")
	bodyr, bodyw := io.Pipe()
	req := &h2ec.Request{
		Method: http.MethodConnect,
		Header: h2ec.Header(h),
		Proto:  "websocket",
		Host:   u.Host,
		URL:    u,
		Body:   bodyr,
	}
	req = req.WithContext(ctx)
	rt := &h2ec.Http2Transport{AllowHTTP: false, TLSClientConfig: localhostTLS()}
	client := h2ec.Client{Transport: rt}
	rsp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if rsp.StatusCode < 200 || rsp.StatusCode >= 300 {
		return nil, errorx.Errorf("received status %d", rsp.StatusCode)
	}
	conn := newHTTP2ClientStream(bodyw, rsp)
	c, _ := clientWebSocketAdapter(ctx, conn, 0, 0)
	go func() {
		defer c.Close()
		c.read(ctx)
	}()

	return c, nil
}
func NewWebtransportClientHttp2(ctx context.Context, urlStr string) (Client, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	bodyr, bodyw := io.Pipe()
	req := &h2ec.Request{
		Method: http.MethodConnect,
		Header: h2ec.Header{},
		Proto:  "webtransport",
		Host:   u.Host,
		URL:    u,
		Body:   bodyr,
	}
	req = req.WithContext(ctx)
	rt := &h2ec.Http2Transport{AllowHTTP: false, TLSClientConfig: localhostTLS()}
	client := h2ec.Client{Transport: rt}
	rsp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if rsp.StatusCode < 200 || rsp.StatusCode >= 300 {
		return nil, errorx.Errorf("received status %d", rsp.StatusCode)
	}
	conn := newHTTP2ClientStream(bodyw, rsp)
	stream := &http2WebtransportWrapper{conn: conn}
	wt, closectx := adapters.NewWebTransportAdapter(ctx, nil, stream, 0, 0)
	go wt.Write(closectx)
	c := &wtransportClient{
		wt:            wt.(*adapters.WebtransportAdapter),
		inputMessages: make(chan []byte),
	}
	go c.read(closectx)

	return c, nil
}

func NewWebsocketClient(ctx context.Context, url string) (Client, error) {
	dialer := ws.Dialer{TLSConfig: localhostTLS()}
	dialer.TLSConfig = localhostTLS()
	conn, _, _, err := dialer.Dial(ctx, url)
	if err != nil {
		return nil, errorx.Withf(err, "failed to establish websocket conn to %v", url)
	}
	c, closectx := clientWebSocketAdapter(ctx, conn, 0, 0)
	go c.read(closectx)

	return c, nil
}

func NewRelayClient(ctx context.Context, url string) (*nostr.Relay, error) {
	relay := nostr.NewRelay(ctx, url)
	err := relay.ConnectWithTLS(ctx, localhostTLS())
	return relay, err
}

func (c *wtransportClient) read(ctx context.Context) {
	for ctx.Err() == nil {
		_, msg, err := c.wt.ReadMessage()
		if err != nil {
			break
		}
		if len(msg) > 0 {
			select {
			case <-ctx.Done():
				return
			default:
				func() {
					c.closedMx.Lock()
					defer c.closedMx.Unlock()
					if !c.closed {
						c.inputMessages <- msg
					}
				}()
			}

		}
	}
}
func (c *wtransportClient) Received() <-chan []byte {
	return c.inputMessages
}

func (c *wtransportClient) WriteMessage(messageType int, data []byte) error {
	if wErr := c.wt.WriteMessageToStream(data); wErr != nil {
		return errorx.Withf(wErr, "client: webtransport writing message failed")
	}
	return nil
}

func (c *wtransportClient) Close() error {
	err := c.wt.Close()
	c.closedMx.Lock()
	if c.closed {
		c.closedMx.Unlock()
		return nil
	}
	close(c.inputMessages)
	c.closed = true
	c.closedMx.Unlock()

	return err
}

func (c *wsocketClient) read(ctx context.Context) {
	for ctx.Err() == nil {
		_, msg, err := c.ReadMessage()
		if err != nil {
			break
		}
		if len(msg) > 0 {
			select {
			case <-c.closeChannel:
				return
			default:
				func() {
					c.closeMx.Lock()
					defer c.closeMx.Unlock()
					if !c.closed {
						c.inputMessages <- msg
					}
				}()
			}

		}
	}
}
func (c *wsocketClient) Received() <-chan []byte {
	return c.inputMessages
}

func clientWebSocketAdapter(ctx context.Context, conn net.Conn, readTimeout, writeTimeout time.Duration) (*wsocketClient, context.Context) {
	wt := &wsocketClient{
		conn:          conn,
		closeChannel:  make(chan struct{}, 1),
		readTimeout:   readTimeout,
		writeTimeout:  writeTimeout,
		inputMessages: make(chan []byte),
	}

	return wt, adapters.NewCustomCancelContext(ctx, wt.closeChannel)
}

func (w *wsocketClient) writeMessageToWebsocket(messageType int, data []byte) error {
	select {
	case <-w.closeChannel:
		return nil
	default:
		var err error
		if w.writeTimeout > 0 {
			err = multierror.Append(nil, w.conn.SetWriteDeadline(time.Now().Add(w.writeTimeout)))
		}
		w.closeMx.Lock()
		if w.closed {
			w.closeMx.Unlock()
			return nil
		}
		w.closeMx.Unlock()
		wErr := wsutil.WriteClientMessage(w.conn, ws.OpCode(messageType), data)
		if isConnClosedErr(wErr) {
			wErr = nil
		}
		if err = multierror.Append(err, wErr).ErrorOrNil(); err != nil {
			return errorx.Withf(err, "client: failed to write data to websocket")
		}

		if flusher, ok := w.conn.(http.Flusher); err == nil && ok {
			flusher.Flush()
		}
		return nil
	}
}

func (w *wsocketClient) WriteMessage(messageType int, data []byte) error {
	select {
	case <-w.closeChannel:
		return nil
	default:
		if wErr := w.writeMessageToWebsocket(messageType, data); wErr != nil {
			return errorx.Withf(wErr, "client: failed to send message to websocket")
		}
		return nil
	}
}

func (w *wsocketClient) ReadMessage() (messageType int, p []byte, err error) {
	if w.readTimeout > 0 {
		_ = w.conn.SetReadDeadline(time.Now().Add(w.readTimeout)) //nolint:errcheck // It is not crucial if we ignore it here.
	}
	msgBytes, typ, err := wsutil.ReadServerData(w.conn)
	if err != nil {
		return int(typ), msgBytes, err
	}
	if typ == ws.OpPing {
		err = wsutil.WriteClientMessage(w.conn, ws.OpPong, nil)
		if err == nil {
			return w.ReadMessage()
		}

		return int(typ), msgBytes, err
	}

	return int(typ), msgBytes, err
}

func (w *wsocketClient) Close() error {
	w.closeMx.Lock()
	if w.closed {
		w.closeMx.Unlock()

		return nil
	}
	w.closed = true
	close(w.closeChannel)
	close(w.inputMessages)
	w.closeMx.Unlock()
	wErr := wsutil.WriteClientMessage(w.conn, ws.OpClose, ws.NewCloseFrameBody(ws.StatusNormalClosure, ""))
	err := w.conn.Close()

	return multierror.Append(wErr, err).ErrorOrNil()
}

func newHTTP2ClientStream(w *io.PipeWriter, resp *h2ec.Response) *http2ClientStream {
	return &http2ClientStream{
		w:    w,
		resp: resp,
	}
}

func (s *http2ClientStream) Read(p []byte) (n int, err error) {
	return s.resp.Body.Read(p)
}
func (s *http2ClientStream) Write(p []byte) (n int, err error) {
	return s.w.Write(p)
}
func (s *http2ClientStream) WriteByte(p byte) (err error) {
	n, err := s.w.Write([]byte{p})
	if err != nil {
		return err
	}
	if n != 1 {
		return errorx.Errorf("expected 1 written byte got %v", n)
	}
	return nil
}
func (s *http2ClientStream) Close() error {
	return multierror.Append(
		s.w.Close(),
		s.resp.Body.Close(),
	).ErrorOrNil()
}

func (s *http2ClientStream) LocalAddr() net.Addr {
	return nil
}

func (s *http2ClientStream) RemoteAddr() net.Addr {
	return nil
}

func (s *http2ClientStream) SetDeadline(t time.Time) error {
	return nil
}

func (s *http2ClientStream) SetReadDeadline(t time.Time) error {
	return nil
}

func (s *http2ClientStream) SetWriteDeadline(t time.Time) error {
	return nil
}

func (h *http2WebtransportWrapper) Write(p []byte) (n int, err error) {
	b := make([]byte, 0, 4+len(p))
	b = quicvarint.Append(b, uint64(h.streamID))
	b = append(b, p...)
	wErr := http3.WriteCapsule(h.conn, http3.CapsuleType(wtCapsuleStream), b)

	return len(b), wErr
}

func (h *http2WebtransportWrapper) Close() error {
	return h.conn.Close()
}

func (h *http2WebtransportWrapper) StreamID() quic.StreamID {
	return 0 // Not used on client.
}

func (h *http2WebtransportWrapper) CancelWrite(code webtransport.StreamErrorCode) {
	return
}

func (h *http2WebtransportWrapper) SetWriteDeadline(time time.Time) error {
	return nil
}

func (h *http2WebtransportWrapper) Read(p []byte) (n int, err error) {
	cType, data, err := http3.ParseCapsule(quicvarint.NewReader(h.conn))
	cData := bufio.NewReader(data)
	if cType == wtCapsuleStream || cType == wtCapsuleStreamFin {
		var sID uint64
		sID, err = quicvarint.Read(cData)
		if err != nil {
			err = errorx.Withf(err, "failed to parse WT_STREAM/StreamID")
			return 4, err
		}
		h.streamID = uint32(sID)
		return cData.Read(p)
	} else {
		if _, err = io.ReadAll(cData); err != nil { // We must read capsule until end.
			err = errorx.Withf(err, "failed to parse read till end capsule %v", cType)
			return 0, err
		}
	}
	return 0, nil
}

func (h *http2WebtransportWrapper) CancelRead(code webtransport.StreamErrorCode) {
	return
}

func (h *http2WebtransportWrapper) SetReadDeadline(time time.Time) error {
	return nil
}

func (h *http2WebtransportWrapper) SetDeadline(time time.Time) error {
	return nil
}

func http3RoundTripper() *http3.RoundTripper {
	return &http3.RoundTripper{
		TLSClientConfig: localhostTLS(),
		QuicConfig: &quic.Config{
			EnableDatagrams:      true,
			MaxIdleTimeout:       600 * time.Second,
			HandshakeIdleTimeout: 600 * time.Second,
		},
	}
}

func localhostTLS() *tls.Config {
	caCertPool := x509.NewCertPool()
	if ok := caCertPool.AppendCertsFromPEM([]byte(localhostCrt)); !ok {
		log.Panic(errorx.New("failed to append localhost tls to cert pool"))
	}

	return &tls.Config{
		MinVersion: tls.VersionTLS13,
		RootCAs:    caCertPool,
	}
}

func isConnClosedErr(err error) bool {
	return err != nil &&
		(errors.Is(err, syscall.EPIPE) ||
			errors.Is(err, syscall.ECONNRESET) ||
			errors.Is(err, h2ec.Http2errClientDisconnected) ||
			errors.Is(err, h2ec.Http2errStreamClosed) ||
			errors.Is(err, io.ErrClosedPipe) ||
			strings.Contains(err.Error(), "convert stream error 386759528") ||
			strings.Contains(err.Error(), "use of closed network connection"))
}
