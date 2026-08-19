package semanticrouter

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// TokenCloudShadowResult keeps the old Scheduler result separate from the
// semantic-router suggestion. MainResult is always the old result in phase 2.
type TokenCloudShadowResult struct {
	MainResult            *SchedulerSelectResult
	NewSuggestion         *ModelSelectionResponse
	ShadowError           error
	ShadowLatencyMs       float64
	OldSelectedAccountID  int64
	NewSuggestedAccountID int64
	OldSelectedModel      string
	NewSuggestedModel     string
	OldSelectedPool       string
	NewSuggestedPool      string
	IsAgree               bool
}

func (r *TokenCloudShadowResult) Main() *SchedulerSelectResult {
	if r == nil {
		return nil
	}
	return r.MainResult
}

type TokenCloudShadowAdapter struct {
	client  *ModelSelectorTCPClient
	metrics *TokenCloudShadowMetrics
}

func NewTokenCloudShadowAdapter(client *ModelSelectorTCPClient) *TokenCloudShadowAdapter {
	return &TokenCloudShadowAdapter{client: client, metrics: NewTokenCloudShadowMetrics()}
}

func (a *TokenCloudShadowAdapter) Observe(ctx context.Context, old func() *SchedulerSelectResult, request *ModelSelectionRequest) *TokenCloudShadowResult {
	result := &TokenCloudShadowResult{}
	if old != nil {
		result.MainResult = old()
	}
	if result.MainResult == nil {
		result.MainResult = &SchedulerSelectResult{Error: fmt.Errorf("old scheduler returned nil result")}
	}
	started := time.Now()
	defer func() {
		result.ShadowLatencyMs = float64(time.Since(started)) / float64(time.Millisecond)
		if result.ShadowLatencyMs <= 0 {
			result.ShadowLatencyMs = 0.001
		}
		a.metrics.record(result)
	}()
	if a == nil || a.client == nil {
		result.ShadowError = fmt.Errorf("shadow client is not configured")
		return result
	}
	suggestion, err := a.client.Select(ctx, request)
	if err != nil {
		result.NewSuggestion, result.ShadowError = suggestion, err
		return result
	}
	result.NewSuggestion = suggestion
	result.NewSuggestedAccountID, result.NewSuggestedModel, result.NewSuggestedPool = suggestion.SelectedAccountID, suggestion.SelectedModel, suggestion.SelectedPool
	if result.MainResult.Error == nil {
		result.OldSelectedAccountID, result.OldSelectedModel, result.OldSelectedPool = result.MainResult.SelectedAccountID, result.MainResult.SelectedModel, result.MainResult.PoolUsed
		result.IsAgree = result.OldSelectedAccountID != 0 && result.OldSelectedAccountID == result.NewSuggestedAccountID
	}
	return result
}

type TokenCloudShadowMetricsSnapshot struct {
	ShadowTotal            uint64  `json:"shadow_total"`
	ShadowSuccess          uint64  `json:"shadow_success"`
	ShadowError            uint64  `json:"shadow_error"`
	ShadowErrorRate        float64 `json:"shadow_error_rate"`
	AverageShadowLatencyMs float64 `json:"average_shadow_latency_ms"`
	P95ShadowLatencyMs     float64 `json:"p95_shadow_latency_ms"`
	OldVsNewAgreementRate  float64 `json:"old_vs_new_agreement_rate"`
	AccountZeroCount       uint64  `json:"account_zero_count"`
	UpstreamCalledCount    uint64  `json:"upstream_called_count"`
}

type TokenCloudShadowMetrics struct {
	mu                                                                          sync.RWMutex
	total, success, errors, agreements, comparable, accountZero, upstreamCalled uint64
	latencySum                                                                  float64
	latencies                                                                   []float64
}

func NewTokenCloudShadowMetrics() *TokenCloudShadowMetrics {
	return &TokenCloudShadowMetrics{latencies: make([]float64, 0, 4096)}
}
func (m *TokenCloudShadowMetrics) record(result *TokenCloudShadowResult) {
	if m == nil || result == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.total++
	m.latencySum += result.ShadowLatencyMs
	if len(m.latencies) < 4096 {
		m.latencies = append(m.latencies, result.ShadowLatencyMs)
	}
	if result.ShadowError == nil && result.NewSuggestion != nil && result.NewSuggestion.Success {
		m.success++
	} else {
		m.errors++
	}
	if result.NewSuggestion != nil {
		if result.NewSuggestion.SelectedAccountID == 0 {
			m.accountZero++
		}
		if result.NewSuggestion.UpstreamCalled {
			m.upstreamCalled++
		}
		if result.MainResult != nil && result.MainResult.Error == nil && result.MainResult.SelectedAccountID != 0 {
			m.comparable++
			if result.IsAgree {
				m.agreements++
			}
		}
	}
}
func (m *TokenCloudShadowMetrics) Snapshot() TokenCloudShadowMetricsSnapshot {
	if m == nil {
		return TokenCloudShadowMetricsSnapshot{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	snapshot := TokenCloudShadowMetricsSnapshot{ShadowTotal: m.total, ShadowSuccess: m.success, ShadowError: m.errors, AccountZeroCount: m.accountZero, UpstreamCalledCount: m.upstreamCalled}
	if m.total > 0 {
		snapshot.AverageShadowLatencyMs = m.latencySum / float64(m.total)
		snapshot.ShadowErrorRate = float64(m.errors) / float64(m.total)
	}
	if m.comparable > 0 {
		snapshot.OldVsNewAgreementRate = float64(m.agreements) / float64(m.comparable)
	}
	sorted := append([]float64(nil), m.latencies...)
	sort.Float64s(sorted)
	if len(sorted) > 0 {
		snapshot.P95ShadowLatencyMs = sorted[int(float64(len(sorted))*0.95)-1]
	}
	return snapshot
}
