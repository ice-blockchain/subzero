// SPDX-License-Identifier: ice License 1.0

package nip98

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetAuthHeader(t *testing.T) {
	tests := []struct {
		name           string
		authHeader     string
		expectedResult string
	}{
		{
			name:           "Bearer token",
			authHeader:     "Bearer some-token",
			expectedResult: "some-token",
		},
		{
			name:           "Nostr token",
			authHeader:     "Nostr some-token",
			expectedResult: "some-token",
		},
		{
			name:           "IONConnect token",
			authHeader:     "IONConnect some-token",
			expectedResult: "some-token",
		},
		{
			name:           "Unknown token type",
			authHeader:     "Unknown some-token",
			expectedResult: "",
		},
		{
			name:           "Empty token",
			authHeader:     "",
			expectedResult: "",
		},
	}

	gin.SetMode(gin.TestMode)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(nil)
			c.Request = &http.Request{
				Header: make(http.Header),
			}
			c.Request.Header.Set("Authorization", tt.authHeader)

			result := GetAuthHeader(c)
			require.Equal(t, tt.expectedResult, result)
		})
	}
}
