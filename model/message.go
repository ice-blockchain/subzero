// SPDX-License-Identifier: ice License 1.0

package model

import (
	"bytes"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
)

var (
	ErrUnknownMessage = errors.New("unknown message")
	ErrParseMessage   = errors.New("parse message")
)

func ParseMessage(message []byte) (e nostr.Envelope, err error) {
	firstComma := bytes.IndexByte(message, ',')
	if firstComma == -1 {
		return nil, ErrUnknownMessage
	}

	label := message[0:firstComma]
	switch {
	case bytes.Contains(label, []byte("EVENT")):
		e = &EventEnvelope{}

	case bytes.Contains(label, []byte("REQ")):
		e = &ReqEnvelope{}

	case bytes.Contains(label, []byte("COUNT")):
		e = &CountEnvelope{}

	default:
		// Passthrough to the original implementation.
		e = nostr.ParseMessage(message)
		if e == nil {
			err = ErrParseMessage
		}
	}

	if err := e.UnmarshalJSON(message); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal %q envelope", e.Label())
	}

	return e, err
}
