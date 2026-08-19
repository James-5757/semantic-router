package semanticrouter

import (
	"math"
	"sort"
	"sync"
)

const shadowLatencySampleLimit = 4096

type ShadowAccountInspector interface {
	GetAccountByID(id int64) (*RealSchedulerAccount, bool)
}

type ShadowStatsSnapshot struct {
	ShadowTotal                  uint64            `json:"shadow_total"`
	ShadowSuccess                uint64            `json:"shadow_success"`
	ShadowError                  uint64            `json:"shadow_error"`
	PoolSuggestionCount          map[string]uint64 `json:"pool_suggestion_count"`
	OldVsNewAgreementRate        float64           `json:"old_vs_new_agreement_rate"`
	AccountZeroCount             uint64            `json:"account_zero_count"`
	DisabledAccountSelectedCount uint64            `json:"disabled_account_selected_count"`
	ShadowLatencyMs              float64           `json:"shadow_latency_ms"`
	AverageShadowLatencyMs       float64           `json:"average_shadow_latency_ms"`
	P95ShadowLatencyMs           float64           `json:"p95_shadow_latency_ms"`
	ShadowErrorRate              float64           `json:"shadow_error_rate"`

	// Takeover metrics
	TakeoverTotal                  uint64  `json:"takeover_total"`
	TakeoverSuccess                uint64  `json:"takeover_success"`
	TakeoverError                  uint64  `json:"takeover_error"`
	TakeoverRateActual             float64 `json:"takeover_rate_actual"`
	TakeoverFallbackCount          uint64  `json:"takeover_fallback_count"`
	TakeoverAccountZeroBlocked     uint64  `json:"takeover_account_zero_blocked"`
	TakeoverDisabledAccountBlocked uint64  `json:"takeover_disabled_account_blocked"`
	OldSchedulerFallbackCount      uint64  `json:"old_scheduler_fallback_count"`
}

type ShadowMetrics struct {
	mu                  sync.RWMutex
	total               uint64
	success             uint64
	errors              uint64
	poolSuggestions     map[string]uint64
	agreementComparable uint64
	agreements          uint64
	accountZero         uint64
	disabledAccount     uint64
	latestLatencyMs     float64
	latencySumMs        float64
	latencySamples      []float64
	latencySampleIndex  int

	// Takeover tracking
	takeoverTotal              uint64
	takeoverSuccess            uint64
	takeoverError              uint64
	takeoverFallbackCount      uint64
	takeoverAccountZeroBlocked uint64
	takeoverDisabledBlocked    uint64
	oldSchedulerFallbackCount  uint64
}

func NewShadowMetrics() *ShadowMetrics {
	return &ShadowMetrics{poolSuggestions: make(map[string]uint64)}
}

func (m *ShadowMetrics) Record(oldResult, suggestion *SchedulerSelectResult, shadowErr error, scheduler SchedulerFacade, latencyMs float64, takeoverResult *SchedulerSelectResult, didAttemptTakeover, takeoverUsedSuggestion bool) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	m.total++
	m.latestLatencyMs = latencyMs
	m.latencySumMs += latencyMs
	if len(m.latencySamples) < shadowLatencySampleLimit {
		m.latencySamples = append(m.latencySamples, latencyMs)
	} else {
		m.latencySamples[m.latencySampleIndex] = latencyMs
		m.latencySampleIndex = (m.latencySampleIndex + 1) % shadowLatencySampleLimit
	}
	if shadowErr == nil && suggestion != nil && suggestion.Error == nil && suggestion.SelectedAccountID != 0 {
		m.success++
	} else {
		m.errors++
	}
	if suggestion == nil {
		m.recordTakeoverMetrics(scheduler, suggestion, takeoverResult, didAttemptTakeover, takeoverUsedSuggestion)
		return
	}
	if suggestion.PoolUsed != "" {
		m.poolSuggestions[suggestion.PoolUsed]++
	}
	if suggestion.SelectedAccountID == 0 {
		m.accountZero++
	}
	if isDisabledShadowSelection(scheduler, suggestion) {
		m.disabledAccount++
	}
	if shadowErr == nil && oldResult != nil && oldResult.Error == nil && oldResult.SelectedAccountID != 0 && suggestion.Error == nil && suggestion.SelectedAccountID != 0 {
		m.agreementComparable++
		if oldResult.SelectedAccountID == suggestion.SelectedAccountID {
			m.agreements++
		}
	}
	m.recordTakeoverMetrics(scheduler, suggestion, takeoverResult, didAttemptTakeover, takeoverUsedSuggestion)
}

func (m *ShadowMetrics) recordTakeoverMetrics(scheduler SchedulerFacade, suggestion, takeoverResult *SchedulerSelectResult, didAttemptTakeover, takeoverUsedSuggestion bool) {
	if !didAttemptTakeover || takeoverResult == nil {
		return
	}
	m.takeoverTotal++
	if !takeoverUsedSuggestion || takeoverResult.Error != nil || takeoverResult.SelectedAccountID == 0 {
		m.takeoverError++
		m.takeoverFallbackCount++
		m.oldSchedulerFallbackCount++
		if suggestion != nil && suggestion.SelectedAccountID == 0 {
			m.takeoverAccountZeroBlocked++
		}
		if isDisabledShadowSelection(scheduler, suggestion) {
			m.takeoverDisabledBlocked++
		}
	} else {
		m.takeoverSuccess++
	}
}

func (m *ShadowMetrics) Snapshot() ShadowStatsSnapshot {
	if m == nil {
		return ShadowStatsSnapshot{PoolSuggestionCount: map[string]uint64{}}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	pools := make(map[string]uint64, len(m.poolSuggestions))
	for pool, count := range m.poolSuggestions {
		pools[pool] = count
	}
	rate := 0.0
	if m.agreementComparable > 0 {
		rate = float64(m.agreements) / float64(m.agreementComparable)
	}
	errorRate := 0.0
	averageLatencyMs := 0.0
	if m.total > 0 {
		errorRate = float64(m.errors) / float64(m.total)
		averageLatencyMs = m.latencySumMs / float64(m.total)
	}
	latencySamples := append([]float64(nil), m.latencySamples...)
	sort.Float64s(latencySamples)
	p95LatencyMs := 0.0
	if len(latencySamples) > 0 {
		index := int(math.Ceil(float64(len(latencySamples))*0.95)) - 1
		p95LatencyMs = latencySamples[index]
	}
	takeoverRateActual := 0.0
	if m.total > 0 {
		takeoverRateActual = float64(m.takeoverTotal) / float64(m.total)
	}
	return ShadowStatsSnapshot{
		ShadowTotal:                  m.total,
		ShadowSuccess:                m.success,
		ShadowError:                  m.errors,
		PoolSuggestionCount:          pools,
		OldVsNewAgreementRate:        rate,
		AccountZeroCount:             m.accountZero,
		DisabledAccountSelectedCount: m.disabledAccount,
		ShadowLatencyMs:              m.latestLatencyMs,
		AverageShadowLatencyMs:       averageLatencyMs,
		P95ShadowLatencyMs:           p95LatencyMs,
		ShadowErrorRate:              errorRate,

		TakeoverTotal:                  m.takeoverTotal,
		TakeoverSuccess:                m.takeoverSuccess,
		TakeoverError:                  m.takeoverError,
		TakeoverRateActual:             takeoverRateActual,
		TakeoverFallbackCount:          m.takeoverFallbackCount,
		TakeoverAccountZeroBlocked:     m.takeoverAccountZeroBlocked,
		TakeoverDisabledAccountBlocked: m.takeoverDisabledBlocked,
		OldSchedulerFallbackCount:      m.oldSchedulerFallbackCount,
	}
}

func isDisabledShadowSelection(scheduler SchedulerFacade, suggestion *SchedulerSelectResult) bool {
	if suggestion == nil || suggestion.SelectedAccountID == 0 {
		return false
	}
	inspector, ok := scheduler.(ShadowAccountInspector)
	if !ok {
		return false
	}
	account, found := inspector.GetAccountByID(suggestion.SelectedAccountID)
	return found && (account.Status != "active" || !account.Schedulable)
}
