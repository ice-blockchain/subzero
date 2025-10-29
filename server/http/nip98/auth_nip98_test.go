// SPDX-License-Identifier: ice License 1.0

package nip98

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/subzero/appcontext"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
	"github.com/ice-blockchain/subzero/validation"
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

			result := DetectAuthHeader(c.GetHeader("Authorization"))
			require.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestValidateAuthHeader(t *testing.T) {
	validation.MustInit(appcontext.TestContext(t), validation.WithIONIdentityPublicKeys(func() []string {
		return []string{}
	}))
	addr, release := query.NewTestDatabase(t.Context())
	defer release()
	query.MustInit(appcontext.TestContext(t), query.WithConfig(&query.Config{
		WriteURLs:       []string{addr},
		PrivateKey:      model.GeneratePrivateKey(),
		RelayURL:        "wss://localhost:443",
		RunDDL:          true,
		DisableSelfTest: true,
	}))

	privKey, pubKey := model.GenerateKeyPair()
	masterPrivKey, masterPubKey := model.GenerateKeyPair()
	sampleUrl, err := url.Parse("https://localhost:443/path/to/resource?query=value#fragment")
	require.NoError(t, err)
	tokenForMaster, err := GenerateAuthHeader(masterPrivKey, "POST", "2cd5c1f358b41b4762027a15fe80ad4d8c2be4bb15d01df5c54f33d01951b038", sampleUrl)
	require.NoError(t, err)
	tokenForUsr, err := GenerateAuthHeader(privKey, "POST", "2cd5c1f358b41b4762027a15fe80ad4d8c2be4bb15d01df5c54f33d01951b038", sampleUrl, masterPubKey)
	require.NoError(t, err)
	attestationEvent := &model.Event{Event: nostr.Event{
		Kind:      model.CustomIONKindAttestation,
		CreatedAt: 1,
		Tags: model.Tags{
			{model.TagAttestationName, pubKey, "", model.CustomIONAttestationKindActive + ":" + strconv.Itoa(int(time.Now().Unix()-10))},
		},
	}}
	require.NoError(t, attestationEvent.SignWithAlg(masterPrivKey, model.SignAlgEDDSA, model.KeyAlgCurve25519))
	embeddedAttestation := generateAuthWithEmbeddedAttestation(t, privKey, "POST", "2cd5c1f358b41b4762027a15fe80ad4d8c2be4bb15d01df5c54f33d01951b038", sampleUrl, attestationEvent)
	tests := []struct {
		name                   string
		authHeader             string
		expectedErr            bool
		expectedAttestationErr bool
	}{
		{
			name:        "Nostr token",
			authHeader:  "some-token",
			expectedErr: true,
		},
		{
			name:        "valid master token",
			authHeader:  tokenForMaster,
			expectedErr: false,
		},
		{
			name:                   "invalid usr token - no attestation",
			authHeader:             tokenForUsr,
			expectedErr:            false,
			expectedAttestationErr: true,
		},
		{
			name:        "valid usr token - embedded attestation",
			authHeader:  embeddedAttestation,
			expectedErr: false,
		},
	}
	gin.SetMode(gin.TestMode)
	for _, tt := range tests {
		auth := NewAuth()
		t.Run(tt.name, func(t *testing.T) {
			token, err := auth.VerifyToken(sampleUrl, "POST", DetectAuthHeader(tt.authHeader), time.Now())
			if tt.expectedErr {
				require.Error(t, err)
				require.Nil(t, token)
			} else {
				require.NoError(t, err)
				require.NotNil(t, token)
				require.Equal(t, token.MasterPubKey(), masterPubKey)
				attestationErr := token.ValidateAttestation(t.Context(), nostr.KindFileMetadata, time.Now())
				if tt.expectedAttestationErr {
					require.Error(t, attestationErr)
					return
				}
				require.NoError(t, attestationErr)
			}
		})
	}
}

func generateAuthWithEmbeddedAttestation(t *testing.T, sk, method, fileHash string, urlValue *url.URL, attestation *model.Event) string {
	event := model.Event{
		Event: nostr.Event{
			Kind:      NostrHttpAuthKind,
			CreatedAt: nostr.Now(),
			Tags: model.Tags{
				model.Tag{"u", (&url.URL{
					Scheme:   "https",
					Host:     urlValue.Host,
					Path:     urlValue.Path,
					RawQuery: urlValue.RawQuery,
					Fragment: urlValue.Fragment,
				}).String()},
				model.Tag{"method", method},
				model.Tag{"payload", fileHash},
				model.Tag{"attestation", attestation.String()},
				model.Tag{"b", attestation.PubKey},
			},
		},
	}
	require.NoError(t, event.SignWithAlg(sk, model.SignAlgEDDSA, model.KeyAlgCurve25519))

	return `Nostr ` + base64.StdEncoding.EncodeToString([]byte(event.String()))
}
