// SPDX-License-Identifier: ice License 1.0

package server

import (
	"context"
	"crypto/tls"
	"log"

	"github.com/caddyserver/certmagic"
	"github.com/cockroachdb/errors"
	"github.com/libdns/cloudflare"
)

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
			},
		}),
	}

	err := magic.ManageSync(ctx, []string{domain})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to manage TLS for %v", domain)
	}

	return magic.TLSConfig(), nil
}

func MustLoadTLSConfigFromACMEWithDNS(ctx context.Context, domain, apiKey string) *tls.Config {
	conf, err := LoadTLSConfigFromACMEWithDNS(ctx, domain, apiKey)
	if err != nil {
		log.Panic(err)
	}
	return conf
}
