// SPDX-License-Identifier: ice License 1.0

package model

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseAttestationString(t *testing.T) {
	t.Parallel()

	cases := []struct {
		In     string
		Action string
		Ts     Timestamp
		Kinds  []int
		Err    bool
	}{
		{
			In:  "action",
			Err: true,
		},
		{
			In:     "action:123",
			Action: "action",
			Ts:     123,
		},
		{
			In:  "action:foo",
			Err: true,
		},
		{
			In:     "action:123:1,2,3",
			Action: "action",
			Ts:     123,
			Kinds:  []int{1, 2, 3},
		},
		{
			In:     "action:1749219680077637000:1,2,3",
			Action: "action",
			Ts:     1749219680077637000,
			Kinds:  []int{1, 2, 3},
		},
		{
			In:  "action:123:1,foo,3",
			Err: true,
		},
	}

	for i, c := range cases {
		t.Logf("case: %v = %v", i, c.In)
		action, ts, kinds, err := ParseAttestationString(c.In)
		if c.Err {
			require.Error(t, err)
			continue
		}

		require.NoError(t, err)
		require.Equal(t, c.Action, action)
		require.Equal(t, c.Ts, ts)
		require.Equal(t, c.Kinds, kinds)
	}
}

func TestIsAccessAllowed(t *testing.T) {
	t.Parallel()

	makeTS := func(v int64) *Timestamp {
		ts := Timestamp(v)
		return &ts
	}

	cases := []struct {
		Name      string
		Records   map[string]*OnBehalfAccessEntry
		DeviceKey string
		Kind      int
		Now       Timestamp
		Allowed   bool
		ErrIs     error
	}{
		{
			Name:    "deny attestation kind",
			Kind:    CustomIONKindAttestation,
			Now:     100,
			Allowed: false,
		},
		{
			Name:      "record not found",
			Records:   map[string]*OnBehalfAccessEntry{},
			DeviceKey: "missing",
			Kind:      1,
			Now:       100,
			Allowed:   false,
			ErrIs:     ErrAttestationRecordNotFound,
		},
		{
			Name: "revoked",
			Records: map[string]*OnBehalfAccessEntry{
				"device": {Revoked: makeTS(10)},
			},
			DeviceKey: "device",
			Kind:      1,
			Now:       11,
			Allowed:   false,
			ErrIs:     ErrAttestationRecordRevoked,
		},
		{
			Name: "expired",
			Records: map[string]*OnBehalfAccessEntry{
				"device": {End: makeTS(10)},
			},
			DeviceKey: "device",
			Kind:      1,
			Now:       11,
			Allowed:   false,
			ErrIs:     ErrAttestationRecordExpired,
		},
		{
			Name: "not active yet",
			Records: map[string]*OnBehalfAccessEntry{
				"device": {Start: makeTS(10)},
			},
			DeviceKey: "device",
			Kind:      1,
			Now:       9,
			Allowed:   false,
			ErrIs:     ErrAttestationRecordIsNotActive,
		},
		{
			Name: "kind not allowed",
			Records: map[string]*OnBehalfAccessEntry{
				"device": {Kinds: []int{1, 2}},
			},
			DeviceKey: "device",
			Kind:      3,
			Now:       11,
			Allowed:   false,
		},
		{
			Name: "kind allowed",
			Records: map[string]*OnBehalfAccessEntry{
				"device": {Kinds: []int{1, 2}},
			},
			DeviceKey: "device",
			Kind:      2,
			Now:       11,
			Allowed:   true,
		},
		{
			Name: "kinds empty allows all",
			Records: map[string]*OnBehalfAccessEntry{
				"device": {},
			},
			DeviceKey: "device",
			Kind:      5,
			Now:       11,
			Allowed:   true,
		},
		{
			Name: "negative kind bypasses kinds check",
			Records: map[string]*OnBehalfAccessEntry{
				"device": {Kinds: []int{1}},
			},
			DeviceKey: "device",
			Kind:      -1,
			Now:       11,
			Allowed:   true,
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			entries := &OnBehalfAccessEntries{Records: c.Records}
			allowed, err := entries.IsAccessAllowed(c.DeviceKey, c.Kind, c.Now)
			require.Equal(t, c.Allowed, allowed)
			if c.ErrIs != nil {
				require.ErrorIs(t, err, c.ErrIs)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestOnBehalfIsAccessAllowedTransitions(t *testing.T) {
	t.Parallel()

	attTag := func(pubkey, action string, ts Timestamp, kinds ...int) Tag {
		attestation := action + ":" + ts.String()
		if len(kinds) > 0 {
			kindStrs := make([]string, len(kinds))
			for i, kind := range kinds {
				kindStrs[i] = strconv.Itoa(kind)
			}
			attestation += ":" + strings.Join(kindStrs, ",")
		}
		return Tag{TagAttestationName, pubkey, "", attestation}
	}

	pubkey := "device"

	cases := []struct {
		Name    string
		Tags    Tags
		Kind    int
		Now     Timestamp
		Allowed bool
		ErrIs   error
	}{
		{
			Name: "active to revoked before revocation",
			Tags: Tags{
				attTag(pubkey, CustomIONAttestationKindActive, 10),
				attTag(pubkey, CustomIONAttestationKindRevoked, 20),
			},
			Kind:    1,
			Now:     15,
			Allowed: true,
		},
		{
			Name: "active to revoked after revocation",
			Tags: Tags{
				attTag(pubkey, CustomIONAttestationKindActive, 10),
				attTag(pubkey, CustomIONAttestationKindRevoked, 20),
			},
			Kind:    1,
			Now:     21,
			Allowed: false,
			ErrIs:   ErrAttestationRecordRevoked,
		},
		{
			Name: "active to inactive to active before reactivation",
			Tags: Tags{
				attTag(pubkey, CustomIONAttestationKindActive, 10),
				attTag(pubkey, CustomIONAttestationKindInactive, 20),
				attTag(pubkey, CustomIONAttestationKindActive, 30),
			},
			Kind:    1,
			Now:     25,
			Allowed: false,
			ErrIs:   ErrAttestationRecordIsNotActive,
		},
		{
			Name: "active to inactive to active after reactivation",
			Tags: Tags{
				attTag(pubkey, CustomIONAttestationKindActive, 10),
				attTag(pubkey, CustomIONAttestationKindInactive, 20),
				attTag(pubkey, CustomIONAttestationKindActive, 30),
			},
			Kind:    1,
			Now:     35,
			Allowed: true,
		},
		{
			Name: "inactive to active to revoked before revocation",
			Tags: Tags{
				attTag(pubkey, CustomIONAttestationKindInactive, 10),
				attTag(pubkey, CustomIONAttestationKindActive, 20),
				attTag(pubkey, CustomIONAttestationKindRevoked, 30),
			},
			Kind:    1,
			Now:     25,
			Allowed: true,
		},
		{
			Name: "inactive to active to revoked after revocation",
			Tags: Tags{
				attTag(pubkey, CustomIONAttestationKindInactive, 10),
				attTag(pubkey, CustomIONAttestationKindActive, 20),
				attTag(pubkey, CustomIONAttestationKindRevoked, 30),
			},
			Kind:    1,
			Now:     31,
			Allowed: false,
			ErrIs:   ErrAttestationRecordRevoked,
		},
		{
			Name: "active to inactive without reactivation",
			Tags: Tags{
				attTag(pubkey, CustomIONAttestationKindActive, 10),
				attTag(pubkey, CustomIONAttestationKindInactive, 20),
			},
			Kind:    1,
			Now:     25,
			Allowed: false,
			ErrIs:   ErrAttestationRecordExpired,
		},
		{
			Name: "active to inactive to active updates kinds",
			Tags: Tags{
				attTag(pubkey, CustomIONAttestationKindActive, 10, 1),
				attTag(pubkey, CustomIONAttestationKindInactive, 20),
				attTag(pubkey, CustomIONAttestationKindActive, 30, 2),
			},
			Kind:    1,
			Now:     35,
			Allowed: false,
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			allowed, err := OnBehalfIsAccessAllowed(c.Tags, pubkey, c.Kind, c.Now)
			require.Equal(t, c.Allowed, allowed)
			if c.ErrIs != nil {
				require.ErrorIs(t, err, c.ErrIs)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
