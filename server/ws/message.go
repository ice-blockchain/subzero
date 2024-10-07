// SPDX-License-Identifier: ice License 1.0

package ws

import (
	"bytes"

	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
)

func parseMessage(message []byte) model.Envelope {
	firstComma := bytes.Index(message, []byte{','})
	if firstComma == -1 {
		return nil
	}
	label := message[0:firstComma]

	var v model.Envelope
	switch {
	// Subzero types.
	case bytes.Contains(label, []byte(model.EnvelopeTypeReq)):
		v = &model.ReqEnvelope{}
	case bytes.Contains(label, []byte(model.EnvelopeTypeCount)):
		v = &model.CountEnvelope{}
	// Nostr types.
	case bytes.Contains(label, []byte(model.EnvelopeTypeEvent)):
		v = &nostr.EventEnvelope{}
	case bytes.Contains(label, []byte(model.EnvelopeTypeNotice)):
		x := nostr.NoticeEnvelope("")
		v = &x
	case bytes.Contains(label, []byte(model.EnvelopeTypeEOSE)):
		x := nostr.EOSEEnvelope("")
		v = &x
	case bytes.Contains(label, []byte(model.EnvelopeTypeOK)):
		v = &nostr.OKEnvelope{}
	case bytes.Contains(label, []byte(model.EnvelopeTypeAuth)):
		v = &nostr.AuthEnvelope{}
	case bytes.Contains(label, []byte(model.EnvelopeTypeClosed)):
		v = &nostr.ClosedEnvelope{}
	case bytes.Contains(label, []byte(model.EnvelopeTypeClose)):
		x := nostr.CloseEnvelope("")
		v = &x
	default:
		return nil
	}

	if err := v.UnmarshalJSON(message); err != nil {
		return nil
	}
	return v
}
