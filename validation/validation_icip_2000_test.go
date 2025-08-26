// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"
	"fmt"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

func TestValidateTagsBAndP(t *testing.T) {
	t.Parallel()

	require.Error(t, validateFollowListEvent(&model.Event{Event: nostr.Event{Tags: model.Tags{
		{"b", "foo"},
		{"p", "foo"},
	}}}))

	require.Error(t, validateEventTags(&model.Event{Event: nostr.Event{Tags: model.Tags{
		{"b", "foo"},
		{"b", "foo"},
	}}}, KindSupportedTags))

	require.Error(t, validateEventTags(&model.Event{Event: nostr.Event{Tags: model.Tags{
		{"p", "foo"},
		{"p", "foo"},
	}}}, KindSupportedTags))

	require.Error(t, validateFollowListEvent(&model.Event{Event: nostr.Event{PubKey: "foo", Tags: model.Tags{
		{"p", "foo"},
	}}}))

	require.NoError(t, validateEventTags(&model.Event{Event: nostr.Event{Tags: model.Tags{
		{"b", "foo"},
		{"p", "bar"},
	}}}, KindSupportedTags))
}

func TestValidateAttestationEvent(t *testing.T) {
	t.Parallel()

	validator := &eventValidator{}

	t.Run("valid single attestation", func(t *testing.T) {
		var ev model.Event
		ev.PubKey = "event_pubkey"
		ev.Tags = model.Tags{
			{"p", "user1", "", "active:1692000000"},
		}

		err := validateAttestationEvent(validator, &ev)
		require.NoError(t, err)
	})

	t.Run("valid multiple attestations for different pubkeys", func(t *testing.T) {
		var ev model.Event
		ev.PubKey = "event_pubkey"
		ev.Tags = model.Tags{
			{"p", "user1", "", "active:1692000000"},
			{"p", "user2", "", "inactive:1692000100"},
			{"p", "user3", "", "revoked:1692000200"},
		}

		err := validateAttestationEvent(validator, &ev)
		require.NoError(t, err)
	})

	t.Run("valid state transition active to revoked", func(t *testing.T) {
		var ev model.Event
		ev.PubKey = "event_pubkey"
		ev.Tags = model.Tags{
			{"p", "user1", "", "active:1692000000"},
			{"p", "user1", "", "revoked:1692000100"},
		}

		err := validateAttestationEvent(validator, &ev)
		require.NoError(t, err)
	})

	t.Run("valid state transition active to inactive", func(t *testing.T) {
		var ev model.Event
		ev.PubKey = "event_pubkey"
		ev.Tags = model.Tags{
			{"p", "user1", "", "active:1692000000"},
			{"p", "user1", "", "inactive:1692000100"},
		}

		err := validateAttestationEvent(validator, &ev)
		require.NoError(t, err)
	})

	t.Run("valid equal timestamps", func(t *testing.T) {
		var ev model.Event
		ev.PubKey = "event_pubkey"
		ev.Tags = model.Tags{
			{"p", "user1", "", "active:1692000000"},
			{"p", "user1", "", "revoked:1692000000"},
		}

		err := validateAttestationEvent(validator, &ev)
		require.NoError(t, err)
	})

	t.Run("state transition from inactive to active", func(t *testing.T) {
		var ev model.Event
		ev.PubKey = "event_pubkey"
		ev.Tags = model.Tags{
			{"p", "user1", "", "inactive:1692000000"},
			{"p", "user1", "", "active:1692000100"},
		}

		err := validateAttestationEvent(validator, &ev)
		require.NoError(t, err)
	})

	t.Run("invalid state transition from revoked", func(t *testing.T) {
		var ev model.Event
		ev.PubKey = "event_pubkey"
		ev.Tags = model.Tags{
			{"p", "user1", "", "revoked:1692000000"},
			{"p", "user1", "", "active:1692000100"},
		}

		err := validateAttestationEvent(validator, &ev)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrAttestationInvalidTransition)
		require.Contains(t, err.Error(), "from \"revoked\" to \"active\"")
	})

	t.Run("invalid temporal order", func(t *testing.T) {
		var ev model.Event
		ev.PubKey = "event_pubkey"
		ev.Tags = model.Tags{
			{"p", "user1", "", "active:1692000100"},
			{"p", "user1", "", "revoked:1692000000"}, // Earlier timestamp.
		}

		err := validateAttestationEvent(validator, &ev)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrAttestationInvalidTemporalOrder)
		require.Contains(t, err.Error(), "previous timestamp")
		require.Contains(t, err.Error(), "is after current timestamp")
	})

	t.Run("unknown attestation action", func(t *testing.T) {
		var ev model.Event
		ev.PubKey = "event_pubkey"
		ev.Tags = model.Tags{
			{"p", "user1", "", "unknown_action:1692000000"},
		}

		err := validateAttestationEvent(validator, &ev)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrAttestationUnknownAction)
		require.Contains(t, err.Error(), "unknown_action")
	})

	t.Run("empty pubkey in tag", func(t *testing.T) {
		var ev model.Event
		ev.PubKey = "event_pubkey"
		ev.Tags = model.Tags{
			{"p", "", "", "active:1692000000"},
		}

		err := validateAttestationEvent(validator, &ev)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrAttestationInvalidPubkey)
		require.Contains(t, err.Error(), "empty pubkey")
	})

	t.Run("pubkey matches event pubkey", func(t *testing.T) {
		var ev model.Event
		ev.PubKey = "same_pubkey"
		ev.Tags = model.Tags{
			{"p", "same_pubkey", "", "active:1692000000"},
		}

		err := validateAttestationEvent(validator, &ev)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrAttestationInvalidPubkey)
		require.Contains(t, err.Error(), "matches event pubkey")
	})

	t.Run("invalid tag format - too few elements", func(t *testing.T) {
		var ev model.Event
		ev.PubKey = "event_pubkey"
		ev.Tags = model.Tags{
			{"p", "user1", "active:1692000000"},
		}

		err := validateAttestationEvent(validator, &ev)
		require.ErrorIs(t, err, ErrAttestationInvalidFormat)
	})

	t.Run("non-attestation tags are ignored", func(t *testing.T) {
		var ev model.Event
		ev.PubKey = "event_pubkey"
		ev.Tags = model.Tags{
			{"e", "event_id", "", "some_value"},
			{"t", "hashtag", "", ""},
			{"p", "user1", "", "active:1692000000"}, // Only this should be processed.
		}

		err := validateAttestationEvent(validator, &ev)
		require.NoError(t, err)
	})

	t.Run("malformed attestation string", func(t *testing.T) {
		var ev model.Event
		ev.PubKey = "event_pubkey"
		ev.Tags = model.Tags{
			{"p", "user1", "", "invalid_format"},
		}

		err := validateAttestationEvent(validator, &ev)
		require.Error(t, err)
		// Error should come from model.ParseAttestationString.
	})

	t.Run("complex valid scenario with multiple users and transitions", func(t *testing.T) {
		var ev model.Event
		ev.PubKey = "event_pubkey"
		ev.Tags = model.Tags{
			{"p", "user1", "", "active:1692000000"},
			{"p", "user2", "", "active:1692000050"},
			{"p", "user1", "", "revoked:1692000100"},
			{"p", "user3", "", "inactive:1692000150"},
			{"p", "user2", "", "inactive:1692000200"},
		}

		err := validateAttestationEvent(validator, &ev)
		require.NoError(t, err)
	})

	t.Run("empty event tags", func(t *testing.T) {
		var ev model.Event
		ev.PubKey = "event_pubkey"
		ev.Tags = model.Tags{}

		err := validateAttestationEvent(validator, &ev)
		require.NoError(t, err)
	})

	t.Run("attestation with kinds (should be ignored)", func(t *testing.T) {
		var ev model.Event
		ev.PubKey = "event_pubkey"
		ev.Tags = model.Tags{
			{"p", "user1", "", "active:1692000000:1,2,3"},
		}

		err := validateAttestationEvent(validator, &ev)
		require.NoError(t, err)
	})

	t.Run("multiple transitions for same user with valid temporal order", func(t *testing.T) {
		var ev model.Event
		ev.PubKey = "event_pubkey"
		ev.Tags = model.Tags{
			{"p", "user1", "", "active:1692000000"},
			{"p", "user1", "", "inactive:1692000100"},
			// Note: inactive is terminal, so no further transitions allowed.
		}

		err := validateAttestationEvent(validator, &ev)
		require.NoError(t, err)
	})

	t.Run("attempt transition from terminal state", func(t *testing.T) {
		var ev model.Event
		ev.PubKey = "event_pubkey"
		ev.Tags = model.Tags{
			{"p", "user1", "", "revoked:1692000000"},
			{"p", "user1", "", "active:1692000100"}, // Invalid: revoked is terminal.
		}

		err := validateAttestationEvent(validator, &ev)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrAttestationInvalidTransition)
	})

	t.Run("valid starting with any state", func(t *testing.T) {
		var ev model.Event
		ev.PubKey = "event_pubkey"
		ev.Tags = model.Tags{
			{"p", "user1", "", "revoked:1692000000"}, // Starting with terminal state is allowed.
		}

		err := validateAttestationEvent(validator, &ev)
		require.NoError(t, err)
	})

	t.Run("mixed tags", func(t *testing.T) {
		var ev model.Event
		ev.PubKey = "event_pubkey"
		ev.Tags = model.Tags{
			{"e", "event_id", "", "some_value"},     // Wrong key, should be ignored.
			{"p", "user2", "", "active:1692000000"}, // Valid, should be processed.
		}

		err := validateAttestationEvent(validator, &ev)
		require.NoError(t, err)
	})

	t.Run("nil tags", func(t *testing.T) {
		var ev model.Event
		ev.PubKey = "event_pubkey"
		ev.Tags = nil

		err := validateAttestationEvent(validator, &ev)
		require.NoError(t, err)
	})

	t.Run("nil tags", func(t *testing.T) {
		var ev model.Event
		ev.PubKey = "event_pubkey"
		ev.Tags = nil

		err := validateAttestationEvent(validator, &ev)
		require.NoError(t, err)
	})

	t.Run("run badges/devices validation", func(t *testing.T) {
		t.Run("disabled", func(t *testing.T) {
			customValidator := newEventValidator(&Config{}, WithQueryFunc(func(ctx context.Context, filter ...model.Filter) query.EventIterator {
				return func(yield func(*model.Event, error) bool) {
					return
				}
			}))
			var ev model.Event
			ev.PubKey = "master_key"
			ev.Tags = model.Tags{
				{"p", "device1", "", "active:1692000000"},
			}
			var rules ruleSet
			rules.SkipKindAttestationProofDevicesVerify = true
			require.NoError(t, customValidator.validateAttestationEvent(t.Context(), &rules, []*model.Event{&ev}, &ev))
		})
		t.Run("no old attestation - new device - require badges", func(t *testing.T) {
			customValidator := newEventValidator(&Config{}, WithQueryFunc(func(ctx context.Context, filter ...model.Filter) query.EventIterator {
				return func(yield func(*model.Event, error) bool) {
					return
				}
			}))
			var ev model.Event
			ev.PubKey = "master_key"
			ev.Tags = model.Tags{
				{"p", "device1", "", "active:1692000000"},
			}

			require.Error(t, customValidator.validateAttestationEvent(t.Context(), &ruleSet{}, []*model.Event{&ev}, &ev), ErrDeviceIdentificationProofFailed)
			bagdeDef, badgeAward := helperDeviceBadges(t, "device1")
			require.NoError(t, customValidator.validateAttestationEvent(t.Context(), &ruleSet{}, []*model.Event{&ev, bagdeDef, badgeAward}, &ev))
		})
		t.Run("old attestattion contains all devices - no proofs required", func(t *testing.T) {
			customValidator := newEventValidator(&Config{}, WithQueryFunc(func(ctx context.Context, filter ...model.Filter) query.EventIterator {
				return func(yield func(*model.Event, error) bool) {
					var ev model.Event
					ev.PubKey = "master_key"
					ev.Tags = model.Tags{
						{"p", "device1", "", "active:1692000000"},
						{"p", "device2", "", "active:1692000001"},
					}
					if !yield(&ev, nil) {
						return
					}
				}
			}))
			var ev model.Event
			ev.PubKey = "master_key"
			ev.Tags = model.Tags{
				{"p", "device1", "", "active:1692000000"},
				{"p", "device2", "", "active:1692000003"},
			}

			require.NoError(t, customValidator.validateAttestationEvent(t.Context(), &ruleSet{}, []*model.Event{&ev}, &ev))
			require.NoError(t, customValidator.validateAttestationEvent(t.Context(), &ruleSet{}, []*model.Event{&ev}, &ev))
		})

		t.Run("old attestation contains some devices - require proofs for missing", func(t *testing.T) {
			customValidator := newEventValidator(&Config{}, WithQueryFunc(func(ctx context.Context, filter ...model.Filter) query.EventIterator {
				return func(yield func(*model.Event, error) bool) {
					var ev model.Event
					ev.PubKey = "master_key"
					ev.Tags = model.Tags{
						{"p", "device1", "", "active:1692000000"},
						{"p", "device2", "", "active:1692000001"},
					}
					if !yield(&ev, nil) {
						return
					}
				}
			}))
			var ev model.Event
			ev.PubKey = "master_key"
			ev.Tags = model.Tags{
				{"p", "device1", "", "active:1692000000"},
				{"p", "device2", "", "active:1692000001"},
				{"p", "device3", "", "active:1692000002"},
			}
			require.Error(t, customValidator.validateAttestationEvent(t.Context(), &ruleSet{}, []*model.Event{&ev}, &ev), ErrDeviceIdentificationProofFailed)
			bagdeDef, badgeAward := helperDeviceBadges(t, "device3")
			require.NoError(t, customValidator.validateAttestationEvent(t.Context(), &ruleSet{}, []*model.Event{&ev, bagdeDef, badgeAward}, &ev))
		})
	})
}

func helperDeviceBadges(t *testing.T, devicePubkey string) (*model.Event, *model.Event) {
	t.Helper()
	_, publicKey := model.GenerateKeyPair()
	badgeDefinitionEvent := model.Event{
		Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeDefinition,
			Tags: model.Tags{
				{"d", deviceIdentificationProof + "~" + devicePubkey},
			},
		},
	}
	badgeAwardEvent := model.Event{
		Event: nostr.Event{
			CreatedAt: nostr.Now(),
			Kind:      nostr.KindBadgeAward,
			Tags: model.Tags{
				{"a", fmt.Sprintf("%d:%s:%s~%s", nostr.KindBadgeDefinition, publicKey, deviceIdentificationProof, devicePubkey)},
				{"p", devicePubkey},
			},
		},
	}
	return &badgeDefinitionEvent, &badgeAwardEvent
}
