// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"encoding/hex"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
)

var supportedIMetaKeys = map[string]tagState{
	"url":      tagStateRequired,
	"m":        tagStateRequired,
	"ox":       tagStateOptional,
	"size":     tagStateOptional,
	"dim":      tagStateOptional,
	"magnet":   tagStateOptional,
	"blurhash": tagStateOptional,
	"thumb":    tagStateOptional,
	"image":    tagStateOptional,
	"summary":  tagStateOptional,
	"alt":      tagStateRequired,
	"fallback": tagStateOptional,
	"duration": tagStateOptional,
}

func validateIMetaTag(tag nostr.Tag) error {
	if tag == nil {
		return nil
	}

	values, err := model.ParseIMeta(tag)
	if err != nil {
		return errors.Wrapf(ErrWrongEventParams, "invalid imeta: %v", err.Error())
	}
	// Check for all required values.
	for key, state := range supportedIMetaKeys {
		if state == tagStateRequired && values[key] == "" {
			return errors.Wrapf(ErrWrongEventParams, "missing required imeta value: %s", key)
		}
	}

	// Either x or ox should be present and they should be hex.
	if values["x"] == "" && values["ox"] == "" {
		return errors.Wrapf(ErrWrongEventParams, "missing required imeta value: x or ox")
	}

	// Check for values correctness.
	for key, value := range values {
		if _, ok := supportedIMetaKeys[key]; !ok {
			return errors.Wrapf(ErrWrongEventParams, "not supported imeta value: %s", key)
		}
		switch key {
		case "x", "ox":
			if _, err := hex.DecodeString(value); err != nil {
				return errors.Wrapf(ErrWrongEventParams, "wrong imeta value: %s, should be hex", key)
			}
		case "url":
			if !strings.HasPrefix(value, "http") {
				return errors.Wrapf(ErrWrongEventParams, "wrong imeta value: %s, should be url", key)
			}
		case "m":
			if strings.ToLower(value) != value {
				return errors.Wrapf(ErrWrongEventParams, "wrong imeta value: %s, should be lowercase", key)
			} else if strings.HasPrefix(value, "video") {
				for _, videoKey := range []string{"thumb", "image", "dim"} {
					if values[videoKey] == "" {
						return errors.Wrapf(ErrWrongEventParams, "missing required imeta value: %s for video content", videoKey)
					}
				}
			}
		case "dim":
			if len(strings.Split(value, "x")) != 2 {
				return errors.Wrapf(ErrWrongEventParams, "wrong imeta value: %s, should be in format: 123x123", key)
			}
		}
	}

	return nil
}
