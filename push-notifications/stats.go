// SPDX-License-Identifier: ice License 1.0

package pushnotifications

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"

	pn "github.com/ice-blockchain/subzero/push-notifications/internal"
)

type (
	PushStats struct {
		mu            sync.RWMutex
		successByKind map[int]*uint64
		errorsByKind  map[int]map[string]*uint64
		totalSuccess  *uint64
		totalErrors   *uint64
		startTime     time.Time
	}
	StatsSnapshot struct {
		TotalSuccess  uint64                    `json:"total_success"`
		TotalErrors   uint64                    `json:"total_errors"`
		SuccessByKind map[int]uint64            `json:"success_by_kind"`
		ErrorsByKind  map[int]map[string]uint64 `json:"errors_by_kind"`
		Duration      time.Duration             `json:"duration"`
	}
)

func newPushStats() *PushStats {
	return &PushStats{
		successByKind: make(map[int]*uint64),
		errorsByKind:  make(map[int]map[string]*uint64),
		totalSuccess:  new(uint64),
		totalErrors:   new(uint64),
		startTime:     time.Now(),
	}
}

func (s *PushStats) RecordSuccess(kind int) {
	atomic.AddUint64(s.totalSuccess, 1)

	s.mu.Lock()
	if s.successByKind[kind] == nil {
		s.successByKind[kind] = new(uint64)
	}
	counter := s.successByKind[kind]
	s.mu.Unlock()

	atomic.AddUint64(counter, 1)
}

func (s *PushStats) RecordError(kind int, err error) {
	if err == nil {
		return
	}
	atomic.AddUint64(s.totalErrors, 1)
	errorReason := classifyError(err)
	s.mu.Lock()
	if s.errorsByKind[kind] == nil {
		s.errorsByKind[kind] = make(map[string]*uint64)
	}
	if s.errorsByKind[kind][errorReason] == nil {
		s.errorsByKind[kind][errorReason] = new(uint64)
	}
	counter := s.errorsByKind[kind][errorReason]
	s.mu.Unlock()

	atomic.AddUint64(counter, 1)
}

func (s *PushStats) GetStats() StatsSnapshot {
	snapshot := StatsSnapshot{
		TotalSuccess:  atomic.LoadUint64(s.totalSuccess),
		TotalErrors:   atomic.LoadUint64(s.totalErrors),
		SuccessByKind: make(map[int]uint64),
		ErrorsByKind:  make(map[int]map[string]uint64),
		Duration:      time.Since(s.startTime),
	}
	s.mu.RLock()
	successCounters := make(map[int]*uint64, len(s.successByKind))
	for kind, counter := range s.successByKind {
		successCounters[kind] = counter
	}
	errorCounters := make(map[int]map[string]*uint64, len(s.errorsByKind))
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
	if pn.IsMessageTooLargeError(err) {
		return "message_too_large"
	}
	if pn.IsInvalidDeviceTokenError(err) {
		return "invalid_token"
	}
	if pn.IsDecryptTokenError(err) {
		return "decrypt_token_error"
	}

	return "other_error"
}
