package semanticrouter

import (
	"math"
	"sort"
	"sync"
	"time"
)

const modelSelectorLatencySampleLimit = 1000

type ModelSelectorRecommendedModelCount struct {
	ModelID string `json:"model_id"`
	Count   int64  `json:"count"`
}

type ModelSelectorOfficialVLLMStatus struct {
	Enabled       bool    `json:"enabled"`
	AttemptCount  int64   `json:"attempt_count"`
	SuccessCount  int64   `json:"success_count"`
	FallbackCount int64   `json:"fallback_count"`
	FallbackRate  float64 `json:"fallback_rate"`
}

// ModelSelectorStatusData is process-local operational telemetry. It contains
// no prompt, decoded request, credential, or raw API-key data.
type ModelSelectorStatusData struct {
	Status                    string                               `json:"status"`
	Version                   string                               `json:"version"`
	Timestamp                 int64                                `json:"timestamp"`
	UptimeSeconds             int64                                `json:"uptime_seconds"`
	Load                      float64                              `json:"load"`
	ShadowOnly                bool                                 `json:"shadow_only"`
	TakeoverEnabled           bool                                 `json:"takeover_enabled"`
	TotalSelections           int64                                `json:"total_selections"`
	SuccessfulSelections      int64                                `json:"successful_selections"`
	ErrorSelections           int64                                `json:"error_selections"`
	SelectionErrorRate        float64                              `json:"selection_error_rate"`
	AverageSelectionLatencyMS float64                              `json:"avg_selection_latency_ms"`
	P95SelectionLatencyMS     float64                              `json:"p95_selection_latency_ms"`
	LatencySampleCount        int                                  `json:"latency_sample_count"`
	CacheEnabled              bool                                 `json:"cache_enabled"`
	CacheHitRate              float64                              `json:"cache_hit_rate"`
	LoadedModelsCount         int                                  `json:"loaded_models_count"`
	SyncedGroupCount          int                                  `json:"synced_group_count"`
	APIKeyGroupBindingCount   int                                  `json:"api_key_group_binding_count"`
	LastHeartbeatAt           int64                                `json:"last_heartbeat_at"`
	OfficialVLLM              ModelSelectorOfficialVLLMStatus      `json:"official_vllm"`
	RecommendedModels         []ModelSelectorRecommendedModelCount `json:"recommended_models"`
}

type modelSelectorMetrics struct {
	mu                   sync.RWMutex
	startedAt            time.Time
	lastHeartbeatAt      time.Time
	active               int
	total                int64
	success              int64
	errors               int64
	totalLatencyMS       float64
	latenciesMS          []float64
	officialAttempts     int64
	officialSuccesses    int64
	officialFallbacks    int64
	recommendedModelHits map[string]int64
}

func newModelSelectorMetrics() *modelSelectorMetrics {
	return &modelSelectorMetrics{startedAt: time.Now().UTC(), recommendedModelHits: make(map[string]int64)}
}

func (m *modelSelectorMetrics) beginSelection() {
	m.mu.Lock()
	m.active++
	m.mu.Unlock()
}

func (m *modelSelectorMetrics) finishSelection(latency time.Duration, success, officialAttempted, officialSucceeded bool, recommendedModel string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active > 0 {
		m.active--
	}
	m.total++
	if success {
		m.success++
	} else {
		m.errors++
	}
	latencyMS := float64(latency) / float64(time.Millisecond)
	m.totalLatencyMS += latencyMS
	m.latenciesMS = append(m.latenciesMS, latencyMS)
	if len(m.latenciesMS) > modelSelectorLatencySampleLimit {
		m.latenciesMS = append([]float64(nil), m.latenciesMS[len(m.latenciesMS)-modelSelectorLatencySampleLimit:]...)
	}
	if officialAttempted {
		m.officialAttempts++
		if officialSucceeded {
			m.officialSuccesses++
		} else {
			m.officialFallbacks++
		}
	}
	if success && recommendedModel != "" {
		m.recommendedModelHits[recommendedModel]++
	}
}

func (m *modelSelectorMetrics) recordHeartbeat() {
	m.mu.Lock()
	m.lastHeartbeatAt = time.Now().UTC()
	m.mu.Unlock()
}

func (m *modelSelectorMetrics) snapshot(version string, officialEnabled bool, maxConcurrent, syncedGroupCount, loadedModelsCount, apiKeyGroupBindingCount int) ModelSelectorStatusData {
	if maxConcurrent <= 0 {
		maxConcurrent = 32
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	average := 0.0
	if m.total > 0 {
		average = m.totalLatencyMS / float64(m.total)
	}
	errorRate := 0.0
	if m.total > 0 {
		errorRate = float64(m.errors) / float64(m.total)
	}
	fallbackRate := 0.0
	if m.officialAttempts > 0 {
		fallbackRate = float64(m.officialFallbacks) / float64(m.officialAttempts)
	}
	recommended := make([]ModelSelectorRecommendedModelCount, 0, len(m.recommendedModelHits))
	for modelID, count := range m.recommendedModelHits {
		recommended = append(recommended, ModelSelectorRecommendedModelCount{ModelID: modelID, Count: count})
	}
	sort.Slice(recommended, func(i, j int) bool {
		if recommended[i].Count == recommended[j].Count {
			return recommended[i].ModelID < recommended[j].ModelID
		}
		return recommended[i].Count > recommended[j].Count
	})
	if len(recommended) > 20 {
		recommended = recommended[:20]
	}
	lastHeartbeat := int64(0)
	if !m.lastHeartbeatAt.IsZero() {
		lastHeartbeat = m.lastHeartbeatAt.Unix()
	}
	return ModelSelectorStatusData{
		Status: "running", Version: version, Timestamp: time.Now().Unix(), UptimeSeconds: int64(time.Since(m.startedAt).Seconds()),
		Load: roundSelectorScore(math.Min(1, float64(m.active)/float64(maxConcurrent))), ShadowOnly: true, TakeoverEnabled: false,
		TotalSelections: m.total, SuccessfulSelections: m.success, ErrorSelections: m.errors, SelectionErrorRate: roundSelectorScore(errorRate),
		AverageSelectionLatencyMS: roundSelectorScore(average), P95SelectionLatencyMS: roundSelectorScore(modelSelectorP95(m.latenciesMS)), LatencySampleCount: len(m.latenciesMS),
		CacheEnabled: false, CacheHitRate: 0, LoadedModelsCount: loadedModelsCount, SyncedGroupCount: syncedGroupCount, APIKeyGroupBindingCount: apiKeyGroupBindingCount,
		LastHeartbeatAt:   lastHeartbeat,
		OfficialVLLM:      ModelSelectorOfficialVLLMStatus{Enabled: officialEnabled, AttemptCount: m.officialAttempts, SuccessCount: m.officialSuccesses, FallbackCount: m.officialFallbacks, FallbackRate: roundSelectorScore(fallbackRate)},
		RecommendedModels: recommended,
	}
}

func modelSelectorP95(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	index := int(math.Ceil(0.95*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	return sorted[index]
}
