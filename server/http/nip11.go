package http

import (
	"encoding/json"
	"github.com/gookit/goutil/errorx"
	"github.com/nbd-wtf/go-nostr/nip11"
	"log"
	"net/http"
)

type (
	nip11handler struct{}
)

func NewNIP11Handler() http.Handler {
	return &nip11handler{}
}

func (n *nip11handler) ServeHTTP(writer http.ResponseWriter, req *http.Request) {
	if req.Header.Get("Accept") != "application/nostr+json" {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	writer.Header().Add("Content-Type", "application/json")
	info := nip11.RelayInformationDocument{
		Name:          "subzero",
		Description:   "subzero",
		PubKey:        "~",
		Contact:       "~",
		SupportedNIPs: []int{1, 9, 11},
		Software:      "subzero",
	}
	bytes, err := json.Marshal(info)
	if err != nil {
		err = errorx.Withf(err, "failed to serialize NIP11 json %+v", info)
		log.Printf("ERROR:%v", err)
	}
	writer.WriteHeader(http.StatusOK)
	writer.Write(bytes)
}
