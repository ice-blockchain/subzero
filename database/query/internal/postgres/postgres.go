// SPDX-License-Identifier: ice License 1.0

package postgres

import (
	_ "embed"
)

//go:embed postgresql.conf
var Config string
