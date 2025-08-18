// SPDX-License-Identifier: ice License 1.0

package cert

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func helperTestTLSWithServer(t *testing.T, conf *tls.Config) {
	t.Helper()

	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	require.NotZero(t, port)

	t.Logf("using port %d for TLS testing", port)

	srv := http.Server{
		TLSConfig: conf,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := w.Write([]byte("OK"))
			require.NoError(t, err)
		}),
	}
	defer srv.Shutdown(t.Context())

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ServeTLS(listener, "", "")
	}()

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	resp, err := client.Get("https://localhost:" + strconv.Itoa(port))
	require.NoError(t, err)
	var body bytes.Buffer
	_, err = body.ReadFrom(resp.Body)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "OK", body.String())

	require.NotNil(t, resp.TLS)
	require.Len(t, resp.TLS.PeerCertificates, 1)
	t.Logf("TLS version: %s", tls.VersionName(resp.TLS.Version))
	for _, cert := range resp.TLS.PeerCertificates {
		t.Logf("Certificate Algorithm            %s", cert.PublicKeyAlgorithm.String())
		t.Logf("Certificate Subject Common Name: %s", cert.Subject.CommonName)
		t.Logf("Certificate Serial Number:       %s", cert.SerialNumber)
		t.Logf("Certificate DNS Names:           %v", cert.DNSNames)
		t.Logf("Certificate IP Addresses:        %v", cert.IPAddresses)
		t.Logf("Certificate Subject:    %s", cert.Subject)
		t.Logf("Certificate Issuer:     %s", cert.Issuer)
		t.Logf("Certificate Not Before: %s", cert.NotBefore)
		t.Logf("Certificate Not After:  %s", cert.NotAfter)
	}

	select {
	case err := <-errCh:
		require.NoError(t, err)
	default:
	}
}

func TestGenerateTLSConfigSelfSigned(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		domain string
	}{
		{
			name:   "valid domain name",
			domain: "example.com",
		},
		{
			name:   "localhost",
			domain: "localhost",
		},
		{
			name:   "IPv4 address",
			domain: "192.168.1.1",
		},
		{
			name:   "IPv6 address",
			domain: "2001:db8::1",
		},
		{
			name:   "subdomain",
			domain: "api.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := generateTLSConfigSelfSigned(tt.domain)
			require.NoError(t, err)
			require.NotNil(t, config)
			require.Len(t, config.Certificates, 1)

			cert := config.Certificates[0]
			x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
			require.NoError(t, err)

			if ip := net.ParseIP(tt.domain); ip != nil {
				require.Len(t, x509Cert.IPAddresses, 1)
				require.True(t, x509Cert.IPAddresses[0].Equal(ip))
			} else {
				require.Len(t, x509Cert.DNSNames, 1)
				require.Equal(t, tt.domain, x509Cert.DNSNames[0])
			}
			helperTestTLSWithServer(t, config)
		})
	}
}
