// SPDX-License-Identifier: ice License 1.0

package model

import (
	"encoding/json"
	"strconv"

	"github.com/cockroachdb/errors"
	"github.com/mailru/easyjson"
	"github.com/mailru/easyjson/jwriter"
	"github.com/tidwall/gjson"
)

type ReqEnvelope struct {
	SubscriptionID string
	Filters
}

func (r ReqEnvelope) String() string {
	v, _ := json.Marshal(r)
	return string(v)
}

func (_ ReqEnvelope) Label() string { return "REQ" }

func (v *ReqEnvelope) UnmarshalJSON(data []byte) error {
	r := gjson.ParseBytes(data)
	arr := r.Array()
	if len(arr) < 3 {
		return errors.Errorf("failed to decode REQ envelope: missing filters")
	}
	v.SubscriptionID = arr[1].Str
	v.Filters = make(Filters, len(arr)-2)
	f := 0
	for i := 2; i < len(arr); i++ {
		if err := easyjson.Unmarshal([]byte(arr[i].Raw), &v.Filters[f]); err != nil {
			return errors.Wrapf(err, "on filter %d", f)
		}
		f++
	}

	return nil
}

func (v ReqEnvelope) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{NoEscapeHTML: true}
	w.RawString(`["REQ",`)
	w.RawString(`"` + v.SubscriptionID + `"`)
	for _, filter := range v.Filters {
		w.RawString(`,`)
		filter.MarshalEasyJSON(&w)
	}
	w.RawString(`]`)
	return w.BuildBytes()
}

type CountEnvelope struct {
	SubscriptionID string
	Filters
	Count *int64
}

func (_ CountEnvelope) Label() string { return "COUNT" }
func (c CountEnvelope) String() string {
	v, _ := json.Marshal(c)
	return string(v)
}

func (v *CountEnvelope) UnmarshalJSON(data []byte) error {
	r := gjson.ParseBytes(data)
	arr := r.Array()
	if len(arr) < 3 {
		return errors.Errorf("failed to decode COUNT envelope: missing filters")
	}
	v.SubscriptionID = arr[1].Str

	var countResult struct {
		Count *int64 `json:"count"`
	}
	if err := json.Unmarshal([]byte(arr[2].Raw), &countResult); err == nil && countResult.Count != nil {
		v.Count = countResult.Count
		return nil
	}

	v.Filters = make(Filters, len(arr)-2)
	f := 0
	for i := 2; i < len(arr); i++ {
		item := []byte(arr[i].Raw)

		if err := easyjson.Unmarshal(item, &v.Filters[f]); err != nil {
			return errors.Wrapf(err, "on filter %d", f)
		}

		f++
	}

	return nil
}

func (v CountEnvelope) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{NoEscapeHTML: true}
	w.RawString(`["COUNT",`)
	w.RawString(`"` + v.SubscriptionID + `"`)
	if v.Count != nil {
		w.RawString(`,{"count":`)
		w.RawString(strconv.FormatInt(*v.Count, 10))
		w.RawString(`}`)
	} else {
		for _, filter := range v.Filters {
			w.RawString(`,`)
			filter.MarshalEasyJSON(&w)
		}
	}
	w.RawString(`]`)
	return w.BuildBytes()
}

type EventEnvelope struct {
	SubscriptionID *string
	Events         []*Event
}

func (EventEnvelope) Label() string { return "EVENT" }

func (v *EventEnvelope) UnmarshalJSON(data []byte) error {
	r := gjson.ParseBytes(data)
	arr := r.Array()
	switch len(arr) {
	case 0, 1:
		return errors.Wrapf(ErrUnknownMessage, "failed to decode EVENT envelope: unknown array len: %v", len(arr))

	// No subscription ID: ["EVENT", event].
	case 2:
		var ev Event

		err := easyjson.Unmarshal([]byte(arr[1].Raw), &ev)
		if err == nil {
			v.Events = []*Event{&ev}

			return nil
		}

		return errors.Wrap(err, "failed to decode event")

	// With multiple events: ["EVENT", [optional subscriptionID], <event1>, [event2], ...].
	default:
		jsonEvents := arr[1:] // Skip the first element, which is the label.
		if jsonEvents[0].Type == gjson.String {
			v.SubscriptionID = &jsonEvents[0].Str
			jsonEvents = jsonEvents[1:]
		} else if jsonEvents[0].Type == gjson.Null {
			v.SubscriptionID = nil
			jsonEvents = jsonEvents[1:]
		}
		v.Events = make([]*Event, 0, len(jsonEvents))
		for i := range jsonEvents {
			var ev Event
			if err := easyjson.Unmarshal([]byte(jsonEvents[i].Raw), &ev); err != nil {
				return errors.Wrapf(err, "failed to decode event %d", i)
			}
			v.Events = append(v.Events, &ev)
		}
	}

	return nil
}

func (v EventEnvelope) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{NoEscapeHTML: true}
	w.RawString(`["EVENT",`)
	if v.SubscriptionID != nil {
		w.RawString(`"` + *v.SubscriptionID + `"`)
		if len(v.Events) > 0 {
			w.RawByte(',')
		}
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

func (v EventEnvelope) String() string {
	j, _ := v.MarshalJSON()

	return string(j)
}
