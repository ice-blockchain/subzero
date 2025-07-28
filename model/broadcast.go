// SPDX-License-Identifier: ice License 1.0

package model

import (
	"github.com/cockroachdb/errors"
	"github.com/mailru/easyjson"
	"github.com/mailru/easyjson/jwriter"
	"github.com/nbd-wtf/go-nostr"
	"github.com/tidwall/gjson"
)

var (
	_ nostr.Envelope = (*BroadcastEnvelope)(nil)
)

type BroadcastEnvelope struct {
	Relay string
	Event Event
}

func (BroadcastEnvelope) Label() string {
	return "BROADCAST"
}

func (b BroadcastEnvelope) String() string {
	j, _ := b.MarshalJSON()
	return string(j)
}

func (b *BroadcastEnvelope) UnmarshalJSON(data []byte) error {
	r := gjson.ParseBytes(data)
	arr := r.Array()

	if len(arr) != 3 {
		return errors.Errorf("failed to decode BROADCAST envelope: expected 3 elements, got %d", len(arr))
	}

	if arr[1].Type != gjson.String {
		return errors.Errorf("failed to decode BROADCAST envelope: expected relay to be a string, got %s", arr[1].Type.String())
	}
	b.Relay = arr[1].Str

	err := easyjson.Unmarshal([]byte(arr[2].Raw), &b.Event)

	return errors.Wrap(err, "failed to decode BROADCAST envelope: bad event")
}

func (b BroadcastEnvelope) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{NoEscapeHTML: true}
	w.RawString(`["BROADCAST",`)
	w.String(b.Relay)
	w.RawByte(',')
	b.Event.MarshalEasyJSON(&w)
	w.RawByte(']')

	return w.BuildBytes()
}
