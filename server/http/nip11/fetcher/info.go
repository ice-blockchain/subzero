// SPDX-License-Identifier: ice License 1.0

package fetcher

import (
	"github.com/nbd-wtf/go-nostr/nip11"
)

type (
	FCMConfig struct {
		ApiKey            string `json:"apiKey"`
		AppID             string `json:"appId"`
		MessagingSenderID string `json:"messagingSenderId"`
		ProjectID         string `json:"projectId"`
	}
	SystemMetrics struct {
		UsedFileStorage     uint64 `json:"used_file_storage"`
		UsedDatabaseStorage uint64 `json:"used_database_storage"`
		UsedTotalStorage    uint64 `json:"used_total_storage"`
		UsedMemory          uint64 `json:"used_memory"`
		UsedCPU             uint16 `json:"used_cpu"`
		UsedBandwidth       uint64 `json:"used_bandwidth"`
	}
	SystemStatusState string
	SystemStatus      struct {
		EventsWrite SystemStatusState `json:"publishing_events"`
		EventsRead  SystemStatusState `json:"subscribing_for_events"`
		DVM         SystemStatusState `json:"dvm"`
		FilesWrite  SystemStatusState `json:"uploading_files"`
		FilesRead   SystemStatusState `json:"reading_files"`
		PushesSend  SystemStatusState `json:"sending_push_notifications"`
	}
	RelayInformationDocument struct {
		SystemStatus                   *SystemStatus  `json:"system_status,omitzero"`
		SystemMetrics                  *SystemMetrics `json:"system_metrics,omitempty"`
		nip11.RelayInformationDocument `json:",inline"`
		FCMAndroidConfigs              []FCMConfig `json:"fcm_android_configs"`
		FCMIOSConfigs                  []FCMConfig `json:"fcm_ios_configs"`
		FCMWebConfigs                  []FCMConfig `json:"fcm_web_configs"`
	}
)

const (
	SystemStatusStateOK          SystemStatusState = "UP"
	SystemStatusStateError       SystemStatusState = "DOWN"
	SystemStatusStateMaintenance SystemStatusState = "MAINTENANCE"
)
