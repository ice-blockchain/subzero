// SPDX-License-Identifier: ice License 1.0

package adapters

import (
	"bytes"
	"compress/flate"
	"io"

	"github.com/cockroachdb/errors"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsflate"

	"github.com/ice-blockchain/subzero/server/ws/internal/pool"
)

type compressor struct {
	Writer *wsflate.Writer
	Buf    bytes.Buffer
}

const (
	WebSocketCompressorKeepInMemory = 200 // Number of compressors to keep in memory for reuse.
	WebSocketCompressLevel          = flate.BestCompression
)

var (
	compressorPool = pool.New(
		func() *compressor {
			var buf bytes.Buffer

			w := wsflate.NewWriter(&buf, func(w io.Writer) wsflate.Compressor {
				// If level is in the range [-2, 9] then the error returned will be nil.
				f, _ := flate.NewWriter(w, WebSocketCompressLevel)
				return f
			})

			return &compressor{
				Buf:    buf,
				Writer: w,
			}
		},
		pool.WithDesiredNumberOfItems[*compressor](WebSocketCompressorKeepInMemory),
		pool.WithPreFill[*compressor](true),
		pool.WithBeforeGet(func(c *compressor) *compressor {
			c.Buf.Reset()
			c.Writer.Reset(&c.Buf)
			return c
		}),
	)
)

func (c *compressor) Compress(p []byte) (data []byte, err error) {
	if _, err = c.Writer.Write(p); err != nil {
		return nil, err
	}
	if err := c.Writer.Flush(); err != nil {
		return nil, err
	}
	return c.Buf.Bytes(), nil
}

func compressFrame(f ws.Frame) (ws.Frame, error) {
	comp := compressorPool.Get()
	defer compressorPool.Put(comp)

	var err error
	f.Payload, err = comp.Compress(f.Payload)
	if err != nil {
		return f, errors.Wrap(err, "failed to compress ws frame payload")
	}
	f.Header.Length = int64(len(f.Payload))
	f.Header, err = wsflate.SetBit(f.Header)
	if err != nil {
		return f, errors.Wrap(err, "failed to set RSV1 bit on ws frame header")
	}
	return f, nil
}
