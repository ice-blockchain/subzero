// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/model"
	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

type (
	PushStats struct {
		startTime     time.Time
		successByKind map[string]*uint64
		errorsByKind  map[string]map[string]*uint64
		totalSuccess  *uint64
		totalErrors   *uint64
		mu            sync.RWMutex
	}
	StatsSnapshot struct {
		SuccessByKind map[string]uint64            `json:"success_by_kind"`
		ErrorsByKind  map[string]map[string]uint64 `json:"errors_by_kind"`
		TotalSuccess  uint64                       `json:"total_success"`
		TotalErrors   uint64                       `json:"total_errors"`
		Duration      time.Duration                `json:"duration"`
	}
)

func newPushStats() *PushStats {
	return &PushStats{
		successByKind: make(map[string]*uint64),
		errorsByKind:  make(map[string]map[string]*uint64),
		totalSuccess:  new(uint64),
		totalErrors:   new(uint64),
		startTime:     time.Now(),
	}
}

func getExtendedKind(event *model.Event) string {
	switch event.Kind {
	case nostr.KindGiftWrap:
		if kTag := event.GetTag("k"); kTag != nil {
			return strconv.Itoa(event.Kind) + "+" + kTag.Value()
		}
	case nostr.KindGenericRepost:
		if event.Content != "" {
			var contentEvent model.Event
			if err := json.Unmarshal([]byte(event.Content), &contentEvent); err == nil {
				return strconv.Itoa(event.Kind) + "+" + strconv.Itoa(contentEvent.Kind)
			}
		}
	}

	return strconv.Itoa(event.Kind)
}

func (s *PushStats) RecordSuccess(event *model.Event) {
	atomic.AddUint64(s.totalSuccess, 1)

	extendedKind := getExtendedKind(event)
	s.mu.Lock()
	if s.successByKind[extendedKind] == nil {
		s.successByKind[extendedKind] = new(uint64)
	}
	counter := s.successByKind[extendedKind]
	s.mu.Unlock()

	atomic.AddUint64(counter, 1)
}

func (s *PushStats) RecordError(event *model.Event, err error) {
	if err == nil {
		return
	}
	atomic.AddUint64(s.totalErrors, 1)
	errorReason := classifyError(err)
	extendedKind := getExtendedKind(event)

	s.mu.Lock()
	if s.errorsByKind[extendedKind] == nil {
		s.errorsByKind[extendedKind] = make(map[string]*uint64)
	}
	if s.errorsByKind[extendedKind][errorReason] == nil {
		s.errorsByKind[extendedKind][errorReason] = new(uint64)
	}
	counter := s.errorsByKind[extendedKind][errorReason]
	s.mu.Unlock()

	atomic.AddUint64(counter, 1)
}

func (s *PushStats) GetStats() StatsSnapshot {
	snapshot := StatsSnapshot{
		TotalSuccess:  atomic.LoadUint64(s.totalSuccess),
		TotalErrors:   atomic.LoadUint64(s.totalErrors),
		SuccessByKind: make(map[string]uint64),
		ErrorsByKind:  make(map[string]map[string]uint64),
		Duration:      time.Since(s.startTime),
	}
	s.mu.RLock()
	successCounters := make(map[string]*uint64, len(s.successByKind))
	for kind, counter := range s.successByKind {
		successCounters[kind] = counter
	}
	errorCounters := make(map[string]map[string]*uint64, len(s.errorsByKind))
	for kind, errorMap := range s.errorsByKind {
		errorCounters[kind] = make(map[string]*uint64, len(errorMap))
		for errorReason, counter := range errorMap {
			errorCounters[kind][errorReason] = counter
		}
	}
	s.mu.RUnlock()

	for kind, counter := range successCounters {
		if counter != nil {
			snapshot.SuccessByKind[kind] = atomic.LoadUint64(counter)
		}
	}
	for kind, errorMap := range errorCounters {
		snapshot.ErrorsByKind[kind] = make(map[string]uint64)
		for errorReason, counter := range errorMap {
			if counter != nil {
				snapshot.ErrorsByKind[kind][errorReason] = atomic.LoadUint64(counter)
			}
		}
	}

	return snapshot
}

func (s *PushStats) StartPeriodicLogging(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Info().Msg("🛑 Stopping push notification statistics logging")
				return
			case <-ticker.C:
				snapshot := s.GetStats()
				logStats(snapshot)
			}
		}
	}()
	log.Info().Msg("📈 Started push notification statistics logging (every 1 minute)")
}

func logStats(s StatsSnapshot) {
	total := s.TotalSuccess + s.TotalErrors
	successRate := float64(0)
	if total > 0 {
		successRate = float64(s.TotalSuccess) / float64(total) * 100
	}

	ratePerMinute := float64(0)
	if s.Duration.Minutes() > 0 {
		ratePerMinute = float64(total) / s.Duration.Minutes()
	}

	log.Info().
		Uint64("total_success", s.TotalSuccess).
		Uint64("total_errors", s.TotalErrors).
		Uint64("total_requests", total).
		Float64("success_rate_percent", successRate).
		Float64("requests_per_minute", ratePerMinute).
		Dur("duration", s.Duration).
		Interface("success_by_kind", s.SuccessByKind).
		Interface("errors_by_kind", s.ErrorsByKind).
		Msg("📊 Push notification statistics")
}

func classifyError(err error) string {
	switch {
	case errors.Is(err, pn.ErrDecryptToken):
		return "decrypt_token_error"
	case errors.Is(err, pn.ErrMessageTooLarge):
		return "message_too_large"
	case errors.Is(err, pn.ErrInvalidDeviceToken):
		return "invalid_token"
	}

	return pn.FcmErrorToReason(err)
}
