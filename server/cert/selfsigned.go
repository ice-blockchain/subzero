// SPDX-License-Identifier: ice License 1.0

package cert

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/rs/zerolog/log"
)

const (
	selfSignedOrganization       = "Ice Open Network"
	selfSignedCommonName         = "Ice Open Network"
	selfSignedOrganizationalUnit = "ION Connect"
	selfSignedValidity           = 10 * 365 * 24 * time.Hour // 10 years.
)

var (
	selfSignedCurve = elliptic.P384()
)

func MustGenerateTLSConfigSelfSigned(domain string) *tls.Config {
	tlsConfig, err := generateTLSConfigSelfSigned(domain)
	if err != nil {
		log.Panic().Err(err).Str("domain", domain).Msg("failed to generate self-signed TLS config")
	}
	return tlsConfig
}

func generateTLSConfigSelfSigned(domain string) (*tls.Config, error) {
	privateKey, err := ecdsa.GenerateKey(selfSignedCurve, rand.Reader)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate ECDSA private key")
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate serial number")
	}

	notBefore := time.Now().Add(-time.Minute) // Start from the current time, minus a minute to avoid clock skew issues.
	notAfter := notBefore.Add(selfSignedValidity)

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization:       []string{selfSignedOrganization},
			OrganizationalUnit: []string{selfSignedOrganizationalUnit},
			CommonName:         selfSignedCommonName,
		},
		NotBefore:   notBefore,
		NotAfter:    notAfter,
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    []string{domain},
	}

	if ip := net.ParseIP(domain); ip != nil {
		template.IPAddresses = []net.IP{ip}
	}

	derBytes, err := x509.CreateCertificate(
		rand.Reader,
		&template,
		&template,
		&privateKey.PublicKey,
		privateKey,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create self-signed certificate")
	}

	keyPEM := new(bytes.Buffer)
	ecBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal ECDSA private key")
	}
	err = pem.Encode(
		keyPEM,
		&pem.Block{
			Type:  "EC PRIVATE KEY",
			Bytes: ecBytes,
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to encode private key to PEM")
	}

	certPEM := new(bytes.Buffer)
	err = pem.Encode(
		certPEM,
		&pem.Block{Type: "CERTIFICATE", Bytes: derBytes},
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to encode certificate to PEM")
	}

	cert, err := tls.X509KeyPair(certPEM.Bytes(), keyPEM.Bytes())
	if err != nil {
		return nil, errors.Wrap(err, "failed to load X509 key pair")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}
