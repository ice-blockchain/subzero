// SPDX-License-Identifier: ice License 1.0

package server

import (
	"context"
	"crypto/tls"
	"log"

	"github.com/caddyserver/certmagic"
	"github.com/cockroachdb/errors"
)

func LoadTLSConfigFromACME(ctx context.Context, domain string, listenPort int) (*tls.Config, error) {
	magic := certmagic.NewDefault()
	magic.DefaultServerName = domain
	magic.Issuers = []certmagic.Issuer{
		certmagic.NewACMEIssuer(magic, certmagic.ACMEIssuer{
			CA:                   certmagic.LetsEncryptProductionCA,
			Email:                "ssl@ice.io",
			Agreed:               true,
			DisableHTTPChallenge: true,
		}),
	}

	err := magic.ManageSync(ctx, []string{domain})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to manage TLS for %v", domain)
	}

	if listenPort != 443 {
		log.Println("listening on port 443 for ACME")
		ln, err := tls.Listen("tcp", ":443", magic.TLSConfig())
		if err != nil {
			return nil, errors.Wrapf(err, "failed to listen on port 443")
		}
		go func() {
			<-ctx.Done()
			ln.Close()
		}()
	}

	return magic.TLSConfig(), nil
}

func MustLoadTLSConfigFromACME(ctx context.Context, domain string, listenPort int) *tls.Config {
	conf, err := LoadTLSConfigFromACME(ctx, domain, listenPort)
	if err != nil {
		log.Panic(err)
	}
	return conf
}
