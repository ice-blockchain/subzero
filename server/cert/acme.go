// SPDX-License-Identifier: ice License 1.0

package cert

import (
	"context"
	"crypto/tls"

	"github.com/caddyserver/certmagic"
	"github.com/cockroachdb/errors"
	"github.com/libdns/cloudflare"
)

func loadTLSConfigFromACME(ctx context.Context, target string, magic *certmagic.Config) (*tls.Config, error) {
	err := magic.ManageSync(ctx, []string{target})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to manage TLS for %q", target)
	}
	return magic.TLSConfig(), nil
}

func LoadTLSConfigFromACMEWithDNS(ctx context.Context, domain, apiKey string) (*tls.Config, error) {
	magic := certmagic.NewDefault()
	magic.DefaultServerName = domain
	magic.Issuers = []certmagic.Issuer{
		certmagic.NewACMEIssuer(magic, certmagic.ACMEIssuer{
			CA:                      certmagic.LetsEncryptProductionCA,
			Email:                   "ssl@ice.io",
			Agreed:                  true,
			DisableHTTPChallenge:    true,
			DisableTLSALPNChallenge: true,
			DNS01Solver: &certmagic.DNS01Solver{
				DNSManager: certmagic.DNSManager{
					DNSProvider: &cloudflare.Provider{
						APIToken: apiKey,
					},
				},
			}}),
	}
	return loadTLSConfigFromACME(ctx, domain, magic)
}

func LoadTLSConfigFromACMEWithHTTP(ctx context.Context, domainOrIpAddress, apiKey string) (*tls.Config, error) {
	magic := certmagic.NewDefault()
	magic.DefaultServerName = domainOrIpAddress
	magic.Issuers = []certmagic.Issuer{&certmagic.ZeroSSLIssuer{
		APIKey:       apiKey,
		Logger:       magic.Logger,
		Storage:      magic.Storage,
		ValidityDays: 90,
	}}
	return loadTLSConfigFromACME(ctx, domainOrIpAddress, magic)
}
