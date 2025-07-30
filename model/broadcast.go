// SPDX-License-Identifier: ice License 1.0

package model

import (
	"fmt"

	"github.com/mailru/easyjson"
	"github.com/mailru/easyjson/jwriter"
	"github.com/nbd-wtf/go-nostr"
	"github.com/tidwall/gjson"
)

var (
	_ nostr.Envelope = (*BroadcastEnvelope)(nil)
)

type BroadcastEnvelope struct {
	Relay  string
	Events Events
}

func (BroadcastEnvelope) Label() string {
	return "BROADCAST"
}

func (b BroadcastEnvelope) String() string {
	j, _ := b.MarshalJSON()
	return string(j)
}

func (v *BroadcastEnvelope) UnmarshalJSON(data []byte) error {
	r := gjson.ParseBytes(data)
	arr := r.Array()

	if len(arr) < 3 { // At least ["BROADCAST", "relay_url", event1, ....].
		return fmt.Errorf("failed to decode BROADCAST envelope: unknown array len: %v", len(arr))
	}

	if arr[1].Type == gjson.String {
		v.Relay = arr[1].Str
	}

	jsonEvents := arr[2:] // Skip label and relay URL.
	v.Events = make(Events, 0, len(jsonEvents))
	for i := range jsonEvents {
		var ev Event
		if err := easyjson.Unmarshal([]byte(jsonEvents[i].Raw), &ev); err != nil {
			return fmt.Errorf("%w -- on event %d", err, i)
		}
		v.Events = append(v.Events, &ev)
	}

	return nil
}

func (v BroadcastEnvelope) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{NoEscapeHTML: true}
	w.RawString(`["BROADCAST",`)

	w.RawString(`"` + v.Relay + `"`)
	if len(v.Events) > 0 {
		w.RawByte(',')
	}

	for i := range v.Events {
		v.Events[i].MarshalEasyJSON(&w)
		if i < len(v.Events)-1 {
			w.RawByte(',')
		}
	}
	w.RawString(`]`)

	return w.BuildBytes()
}
