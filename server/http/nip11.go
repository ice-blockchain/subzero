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
	FCMConfig struct {
		ApiKey            string `json:"apiKey,omitempty"`
		AppID             string `json:"appId,omitempty"`
		SenderID          string `json:"senderId,omitempty"`
		MessagingSenderID string `json:"messagingSenderId,omitempty"`
		ProjectID         string `json:"projectId,omitempty"`
	}

	RelayInformationDocument struct {
		nip11.RelayInformationDocument `json:",inline"`
		FCMAndroidConfigs              []FCMConfig `json:"fcm_android_configs"`
		FCMIOSConfigs                  []FCMConfig `json:"fcm_ios_configs"`
		FCMWebConfigs                  []FCMConfig `json:"fcm_web_configs"`
	}
	Config struct {
		MinLeadingZeroBits int
		FCMAndroidConfigs  []string
		FCMIOSConfigs      []string
		FCMWebConfigs      []string
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

func (n *nip11handler) info() RelayInformationDocument {
	var androidConfigs []FCMConfig
	var iosConfigs []FCMConfig
	var webConfigs []FCMConfig

	for _, jsonStr := range n.cfg.FCMAndroidConfigs {
		var config FCMConfig
		if err := json.Unmarshal([]byte(jsonStr), &config); err == nil {
			androidConfigs = append(androidConfigs, config)
		} else {
			log.Printf("Failed to parse Android FCM config: %v", err)
		}
	}

	for _, jsonStr := range n.cfg.FCMIOSConfigs {
		var config FCMConfig
		if err := json.Unmarshal([]byte(jsonStr), &config); err == nil {
			iosConfigs = append(iosConfigs, config)
		} else {
			log.Printf("Failed to parse iOS FCM config: %v", err)
		}
	}
	for _, jsonStr := range n.cfg.FCMWebConfigs {
		var config FCMConfig
		if err := json.Unmarshal([]byte(jsonStr), &config); err == nil {
			webConfigs = append(webConfigs, config)
		} else {
			log.Printf("Failed to parse Web FCM config: %v", err)
		}
	}

	return RelayInformationDocument{
		RelayInformationDocument: nip11.RelayInformationDocument{
			Name:          "subzero",
			Description:   "subzero",
			PubKey:        "~",
			Contact:       "~",
			SupportedNIPs: []any{1, 2, 9, 10, 11, 13, 18, 23, 24, 25, 32, 40, 45, 50, 51, 56, 58, 65, 90, 92, 96, 98},
			Software:      "subzero",
			Limitation: &nip11.RelayLimitationDocument{
				MinPowDifficulty: n.cfg.MinLeadingZeroBits,
			},
		},
		FCMAndroidConfigs: androidConfigs,
		FCMIOSConfigs:     iosConfigs,
		FCMWebConfigs:     webConfigs,
	}
}
