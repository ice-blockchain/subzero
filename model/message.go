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

	if bytes.Contains(message[:firstComma], []byte("EVENT")) {
		var eventEnvelope EventEnvelope

		if err = eventEnvelope.UnmarshalJSON(message); err != nil {
			return nil, errors.Wrap(err, "unmarshal event envelope")
		}

		e = &eventEnvelope
	} else {
		// Passthrough to the original implementation.
		e = nostr.ParseMessage(message)
	}

	if e == nil {
		err = ErrParseMessage
	}

	return e, err
}
