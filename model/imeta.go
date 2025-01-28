// SPDX-License-Identifier: ice License 1.0

package model

import (
	"strings"

	"github.com/cockroachdb/errors"
)

func ParseIMeta(tag Tag) (values map[string]string, err error) {
	values = make(map[string]string)
	// Parse tag values and check for all unsupported values.
	for _, val := range tag[1:] {
		parts := strings.Split(val, " ")
		if len(parts) < 2 {
			return nil, errors.Errorf("wrong imeta tag: %+v", tag)
		} else if _, ok := values[parts[0]]; ok {
			return nil, errors.Errorf("duplicate imeta value: %s", parts[0])
		}
		values[parts[0]] = parts[1]
	}
	return values, nil
}
