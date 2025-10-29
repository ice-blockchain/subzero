// SPDX-License-Identifier: ice License 1.0

package nip11

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dundee/gdu/v5/pkg/analyze"
	"github.com/dundee/gdu/v5/pkg/fs"
	"github.com/nbd-wtf/go-nostr/nip11"
	"github.com/rs/zerolog/log"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"

	"github.com/ice-blockchain/subzero/appcontext"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
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
	RelayInformationDocument struct {
		SystemMetrics                  *SystemMetrics `json:"system_metrics,omitempty"`
		nip11.RelayInformationDocument `json:",inline"`
		FCMAndroidConfigs              []FCMConfig `json:"fcm_android_configs"`
		FCMIOSConfigs                  []FCMConfig `json:"fcm_ios_configs"`
		FCMWebConfigs                  []FCMConfig `json:"fcm_web_configs"`
	}
	Config struct {
		PrivateKey         string
		FCMAndroidConfigs  []string
		FCMIOSConfigs      []string
		FCMWebConfigs      []string
		MinLeadingZeroBits int
	}
	nip11handler struct {
		cfg                  *Config
		systemMetrics        *atomic.Pointer[SystemMetrics]
		storagePath          string
		commandPath          string
		lastBandwidthBytes   uint64
		lastBandwidthBytesAt int64
	}
)

const systemMetricsCollectionTime = 30 * time.Second

func NewNIP11Handler(ctx context.Context, cfg *Config, storagePath, commandPath string) http.Handler {
	h := &nip11handler{
		cfg:           cfg,
		storagePath:   storagePath,
		commandPath:   commandPath,
		systemMetrics: new(atomic.Pointer[SystemMetrics]),
	}
	go h.startSystemMetricsCollector(ctx)
	return h
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
		log.Error().Err(err).Interface("info", info).Msg("failed to serialize NIP11 json")
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
			if isValidFCMConfig(config) {
				androidConfigs = append(androidConfigs, config)
			} else {
				log.Panic().Interface("config", config).Msg("invalid Android FCM config: missing required fields")
			}
		} else {
			log.Error().Err(err).Msg("failed to parse Android FCM config")
		}
	}

	for _, jsonStr := range n.cfg.FCMIOSConfigs {
		var config FCMConfig
		if err := json.Unmarshal([]byte(jsonStr), &config); err == nil {
			if isValidFCMConfig(config) {
				iosConfigs = append(iosConfigs, config)
			} else {
				log.Panic().Interface("config", config).Msg("invalid iOS FCM config: missing required fields")
			}
		} else {
			log.Error().Err(err).Msg("failed to parse iOS FCM config")
		}
	}
	for _, jsonStr := range n.cfg.FCMWebConfigs {
		var config FCMConfig
		if err := json.Unmarshal([]byte(jsonStr), &config); err == nil {
			if isValidFCMConfig(config) {
				webConfigs = append(webConfigs, config)
			} else {
				log.Panic().Interface("config", config).Msg("invalid Web FCM config: missing required fields")
			}
		} else {
			log.Error().Err(err).Msg("failed to parse Web FCM config")
		}
	}

	pubKey := "~"
	if n.cfg.PrivateKey != "" {
		var err error
		pubKey, err = model.GetPublicKey(n.cfg.PrivateKey)
		if err != nil {
			log.Error().Err(err).Msg("failed to get public key from private key")
		}
	}

	return RelayInformationDocument{
		RelayInformationDocument: nip11.RelayInformationDocument{
			Name:          "subzero",
			Description:   "subzero",
			PubKey:        pubKey,
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
		SystemMetrics:     n.systemMetrics.Load(),
	}
}

func isValidFCMConfig(config FCMConfig) bool {
	return config.ApiKey != "" &&
		config.AppID != "" &&
		config.MessagingSenderID != "" &&
		config.ProjectID != ""
}

func (n *nip11handler) startSystemMetricsCollector(ctx context.Context) {
	ticks := make(chan struct{}, 1)
	ticks <- struct{}{}
	go func() {
		defer appcontext.GetAppContext(ctx).Recover()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		defer close(ticks)

		for {
			select {
			case <-ticker.C:
				select {
				case ticks <- struct{}{}:
				default:
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	for range ticks {
		metrics, err := n.collectMetrics(ctx)
		if err != nil {
			log.Error().Err(err).Msg("failed to collect system metrics")
		}
		n.systemMetrics.Store(metrics)
	}
}

func (n *nip11handler) collectMetrics(ctx context.Context) (*SystemMetrics, error) {
	reqCtx, reqCancel := context.WithTimeout(ctx, systemMetricsCollectionTime)
	defer reqCancel()
	cpuUsages, err := cpu.PercentWithContext(reqCtx, 0, false)
	if err != nil {
		return nil, errors.Wrap(err, "failed to collect cpu usage for nip-11 system metrics")
	}

	memUsage, err := mem.VirtualMemoryWithContext(reqCtx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to collect memory usage for nip-11 system metrics")
	}
	storageDiskCalculator := analyze.CreateAnalyzer()
	fileStorageDiskUsage := storageDiskCalculator.AnalyzeDir(n.storagePath, func(name, path string) bool { return false }, true)
	fileStorageDiskUsage.UpdateStats(make(fs.HardLinkedItems, 1))
	fileStorageDiskUsed := uint64(fileStorageDiskUsage.GetSize())
	commandStorageUsed := uint64(0)
	_, err = os.Stat(n.commandPath)
	if n.commandPath != "" && !os.IsNotExist(err) {
		commandDiskCalculator := analyze.CreateAnalyzer()
		commandDiskUsage := commandDiskCalculator.AnalyzeDir(n.commandPath, func(name, path string) bool { return false }, true)
		fileStorageDiskUsage.UpdateStats(make(fs.HardLinkedItems, 1))
		commandStorageUsed = uint64(commandDiskUsage.GetSize())
	}
	bandwidthUsage, err := net.IOCountersWithContext(reqCtx, false)
	if err != nil {
		return nil, errors.Wrap(err, "failed to collect bandwidth usage for nip-11 system metrics")
	}
	usedDatabaseStorage := query.UsedDatabaseStorage.Load()

	return &SystemMetrics{
		UsedCPU:             uint16(cpuUsages[0]),
		UsedMemory:          memUsage.Used,
		UsedFileStorage:     fileStorageDiskUsed,
		UsedDatabaseStorage: usedDatabaseStorage,
		UsedTotalStorage:    fileStorageDiskUsed + usedDatabaseStorage + commandStorageUsed,
		UsedBandwidth:       n.calcBandwidth(bandwidthUsage),
	}, nil
}

func (n *nip11handler) calcBandwidth(counters []net.IOCountersStat) uint64 {
	now := time.Now().UnixNano()
	rate := uint64(0)
	for _, c := range counters {
		if c.Name == "all" {
			totalBytes := c.BytesRecv + c.BytesSent
			prevBytes := atomic.SwapUint64(&n.lastBandwidthBytes, totalBytes)
			prevUpdatedAt := atomic.SwapInt64(&n.lastBandwidthBytesAt, now)
			rate = ((totalBytes - prevBytes) * uint64(time.Second) / uint64(now-prevUpdatedAt))

			break
		}
	}
	return rate
}
