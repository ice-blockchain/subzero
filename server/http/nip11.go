// SPDX-License-Identifier: ice License 1.0

package http

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr/nip11"
)

type (
	Config struct {
		MinLeadingZeroBits int
	}
	nip11handler struct {
		cfg *Config
	}
)

func NewNIP11Handler(cfg *Config) http.Handler {
	return &nip11handler{
		cfg: cfg,
	}
}

func (n *nip11handler) ServeHTTP(writer http.ResponseWriter, req *http.Request) {
	if req.Header.Get("Accept") != "application/nostr+json" {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	writer.Header().Add("Content-Type", "application/json")
	info := n.info()
	bytes, err := json.Marshal(info)
	if err != nil {
		err = errors.Wrapf(err, "failed to serialize NIP11 json %+v", info)
		log.Printf("ERROR:%v", err)
	}
	writer.Write(bytes)
}

func (n *nip11handler) info() nip11.RelayInformationDocument {
	return nip11.RelayInformationDocument{
		Name:          "subzero",
		Description:   "subzero",
		PubKey:        "~",
		Contact:       "~",
		SupportedNIPs: []int{1, 2, 9, 10, 11, 13, 18, 23, 24, 25, 32, 40, 45, 50, 51, 56, 58, 65, 90, 92, 96, 98},
		Software:      "subzero",
		Limitation: &nip11.RelayLimitationDocument{
			MinPowDifficulty: n.cfg.MinLeadingZeroBits,
		},
	}
}
