// SPDX-License-Identifier: ice License 1.0

package query

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreatePgURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		username string
		password string
		target   string
		want     string
		wantErr  bool
	}{
		{
			name:     "target without postgres schema",
			username: "user",
			password: "pass",
			target:   "localhost:5432/db",
			want:     "postgres://user:pass@localhost:5432/db",
			wantErr:  false,
		},
		{
			name:     "target with postgres schema",
			username: "user",
			password: "pass",
			target:   "postgres://localhost:5432/db",
			want:     "postgres://user:pass@localhost:5432/db",
			wantErr:  false,
		},
		{
			name:     "target with existing user credentials",
			username: "newuser",
			password: "newpass",
			target:   "postgres://existinguser:existingpass@localhost:5432/db",
			want:     "postgres://existinguser:existingpass@localhost:5432/db",
			wantErr:  false,
		},
		{
			name:     "URL with special characters in password",
			username: "user",
			password: "p@ss:w0rd",
			target:   "localhost:5432/db",
			want:     "postgres://user:p%40ss%3Aw0rd@localhost:5432/db",
			wantErr:  false,
		},
		{
			name:     "URL with port only",
			username: "user",
			password: "pass",
			target:   "localhost:5432",
			want:     "postgres://user:pass@localhost:5432",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := createPgURL(tt.username, tt.password, tt.target)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.Equal(t, tt.want, got)
		})
	}
}
