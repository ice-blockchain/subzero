package model

import (
	"encoding/json"
	"fmt"

	"github.com/mailru/easyjson"
	"github.com/nbd-wtf/go-nostr"
	"github.com/tidwall/gjson"
)

type (
	EnvelopeType string

	Envelope interface {
		nostr.Envelope
	}

	ReqEnvelope struct {
		SubscriptionID string
		Filters
	}

	CountEnvelope struct {
		SubscriptionID string
		Filters
		Count *int64
	}
)

const (
	EnvelopeTypeEvent  EnvelopeType = "EVENT"
	EnvelopeTypeReq    EnvelopeType = "REQ"
	EnvelopeTypeCount  EnvelopeType = "COUNT"
	EnvelopeTypeNotice EnvelopeType = "NOTICE"
	EnvelopeTypeEOSE   EnvelopeType = "EOSE"
	EnvelopeTypeOK     EnvelopeType = "OK"
	EnvelopeTypeAuth   EnvelopeType = "AUTH"
	EnvelopeTypeClosed EnvelopeType = "CLOSED"
	EnvelopeTypeClose  EnvelopeType = "CLOSE"
)

func (*ReqEnvelope) Label() string {
	return string(EnvelopeTypeReq)
}

func (v *ReqEnvelope) UnmarshalJSON(data []byte) error {
	r := gjson.ParseBytes(data)
	arr := r.Array()
	if len(arr) < 3 {
		return fmt.Errorf("failed to decode REQ envelope: missing filters")
	}
	v.SubscriptionID = arr[1].Str
	v.Filters = make(Filters, len(arr)-2)
	f := 0
	for i := 2; i < len(arr); i++ {
		if err := easyjson.Unmarshal([]byte(arr[i].Raw), &v.Filters[f]); err != nil {
			return fmt.Errorf("%w -- on filter %d", err, f)
		}
		f++
	}

	return nil
}

func (v *ReqEnvelope) MarshalJSON() ([]byte, error) {
	data := []any{EnvelopeTypeReq, v.SubscriptionID}

	if len(v.Filters) > 0 {
		filterData, err := marshalFilters(v.Filters)
		if err != nil {
			return nil, err
		}
		data = append(data, filterData...)
	}

	return json.Marshal(data)
}

func (v *ReqEnvelope) String() string {
	data, _ := json.Marshal(v)
	return string(data)
}

func (*CountEnvelope) Label() string {
	return string(EnvelopeTypeCount)
}

func (v *CountEnvelope) UnmarshalJSON(data []byte) error {
	r := gjson.ParseBytes(data)
	arr := r.Array()
	if len(arr) < 3 {
		return fmt.Errorf("failed to decode COUNT envelope: missing filters")
	}
	v.SubscriptionID = arr[1].Str

	if len(arr) < 3 {
		return fmt.Errorf("COUNT array must have at least 3 items")
	}

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
			return fmt.Errorf("%w -- on filter %d", err, f)
		}

		f++
	}

	return nil
}

func (v *CountEnvelope) MarshalJSON() ([]byte, error) {
	data := []any{EnvelopeTypeCount, v.SubscriptionID}

	if v.Count != nil {
		var count = struct {
			Count int64 `json:"count"`
		}{
			Count: *v.Count,
		}
		data = append(data, &count)
	} else if len(v.Filters) > 0 {
		filterData, err := marshalFilters(v.Filters)
		if err != nil {
			return nil, err
		}
		data = append(data, filterData...)
	}

	return json.Marshal(data)
}

func (v *CountEnvelope) String() string {
	data, _ := json.Marshal(v)
	return string(data)
}

func marshalFilters(filters Filters) ([]any, error) {
	var messages = make([]any, 0, len(filters))
	for _, filter := range filters {
		filterData, err := json.Marshal(filter)
		if err != nil {
			return nil, err
		}
		messages = append(messages, json.RawMessage(filterData))
	}
	return messages, nil
}
