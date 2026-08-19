package semanticrouter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ShadowDebugResponse /v1/debug/shadow 接口响应
type ShadowDebugResponse struct {
	Config       SemanticRouterRuntimeConfig          `json:"config"`
	Metrics      ShadowStatsSnapshot                   `json:"metrics"`
	RecentDecisions []ShadowDecisionSummary            `json:"recent_decisions,omitempty"`
	MismatchSamples  []ShadowDecisionSummary            `json:"mismatch_samples,omitempty"`
	AccountZeroSamples []ShadowDecisionSummary          `json:"account_zero_samples,omitempty"`
	DisabledAccountSamples []ShadowDecisionSummary      `json:"disabled_account_samples,omitempty"`
	FallbackSamples   []ShadowDecisionSummary           `json:"fallback_samples,omitempty"`
}

// ShadowDecisionSummary 影子模式决策摘要
type ShadowDecisionSummary struct {
	RequestID            string         `json:"request_id,omitempty"`
	PreferredPool        PreferredPool  `json:"preferred_pool"`
	PreferredTier        PreferredTier  `json:"preferred_tier"`
	Confidence           float64        `json:"confidence"`
	FinalDecisionSource  string         `json:"final_decision_source"`
	FallbackReason       string         `json:"fallback_reason,omitempty"`
	OldAccountID         int64          `json:"old_account_id"`
	NewAccountID         int64          `json:"new_account_id"`
	IsAgree              bool           `json:"is_agree"`
	ShadowLatencyMs      float64        `json:"shadow_latency_ms"`
	CreatedAt            time.Time      `json:"created_at"`
}

// HandleDebugShadow 处理 /v1/debug/shadow 请求
// 提供影子模式调试信息：配置、指标、最近决策、不一致样本等
func HandleDebugShadow(
	w http.ResponseWriter,
	r *http.Request,
	shadowRouter *ShadowRouter,
	logStore *InMemoryRoutingDecisionLogStore,
) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error": "method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	response := ShadowDebugResponse{}

	// 配置信息
	if shadowRouter != nil {
		response.Config = shadowRouter.config
		response.Metrics = shadowRouter.Stats()
	}

	// 查询最近决策
	if logStore != nil {
		allEntries := logStore.Entries()
		totalEntries := len(allEntries)

		// 最近 100 条
		recentEnd := totalEntries
		recentStart := totalEntries - 100
		if recentStart < 0 {
			recentStart = 0
		}
		response.RecentDecisions = make([]ShadowDecisionSummary, 0, recentEnd-recentStart)
		for i := recentStart; i < recentEnd; i++ {
			entry := allEntries[i]
			response.RecentDecisions = append(response.RecentDecisions, toShadowDecisionSummary(entry))
		}

		// old/new 不一致样本（取最近 20 条）
		response.MismatchSamples = make([]ShadowDecisionSummary, 0, 20)
		for i := totalEntries - 1; i >= 0 && len(response.MismatchSamples) < 20; i-- {
			entry := allEntries[i]
			if entry.OldSelectedAccountID != 0 && entry.NewSuggestedAccountID != 0 && !entry.IsAgree {
				response.MismatchSamples = append(response.MismatchSamples, toShadowDecisionSummary(entry))
			}
		}

		// account_zero 样本（取最近 20 条）
		response.AccountZeroSamples = make([]ShadowDecisionSummary, 0, 20)
		for i := totalEntries - 1; i >= 0 && len(response.AccountZeroSamples) < 20; i-- {
			entry := allEntries[i]
			if entry.SelectedAccountID == 0 {
				response.AccountZeroSamples = append(response.AccountZeroSamples, toShadowDecisionSummary(entry))
			}
		}

		// disabled account 样本（取最近 20 条）
		response.DisabledAccountSamples = make([]ShadowDecisionSummary, 0, 20)
		for i := totalEntries - 1; i >= 0 && len(response.DisabledAccountSamples) < 20; i-- {
			entry := allEntries[i]
			if entry.NewSuggestedAccountID != 0 && entry.SelectedAccountID == 0 {
				response.DisabledAccountSamples = append(response.DisabledAccountSamples, toShadowDecisionSummary(entry))
			}
		}

		// fallback 样本（取最近 20 条）
		response.FallbackSamples = make([]ShadowDecisionSummary, 0, 20)
		for i := totalEntries - 1; i >= 0 && len(response.FallbackSamples) < 20; i-- {
			entry := allEntries[i]
			if entry.FinalDecisionSource == "fallback" || entry.FallbackReason != "" {
				response.FallbackSamples = append(response.FallbackSamples, toShadowDecisionSummary(entry))
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")

	// 如果 metrics 不达标，设置 warn 级别
	if shadowRouter != nil {
		snapshot := shadowRouter.Stats()
		if snapshot.ShadowErrorRate > 0.01 || snapshot.AccountZeroCount > 0 || snapshot.DisabledAccountSelectedCount > 0 || snapshot.P95ShadowLatencyMs > 100 {
			w.Header().Set("X-Shadow-Status", "warn")
		} else if snapshot.ShadowTotal > 0 {
			w.Header().Set("X-Shadow-Status", "healthy")
		}

		// 输出 staging 验收结果
		validation := map[string]interface{}{
			"shadow_enabled":                 shadowRouter.config.SemanticRouterShadowEnabled,
			"dry_run_enabled":                shadowRouter.config.SemanticRouterDryRunEnabled,
			"takeover_enabled":               shadowRouter.config.SemanticRouterTakeoverEnabled,
			"shadow_error_rate_ok":           snapshot.ShadowErrorRate < 0.01,
			"shadow_error_rate":              fmt.Sprintf("%.4f%%", snapshot.ShadowErrorRate*100),
			"account_zero_count_ok":          snapshot.AccountZeroCount == 0,
			"account_zero_count":             snapshot.AccountZeroCount,
			"disabled_account_count_ok":      snapshot.DisabledAccountSelectedCount == 0,
			"disabled_account_count":         snapshot.DisabledAccountSelectedCount,
			"p95_latency_ok":                 snapshot.P95ShadowLatencyMs < 100,
			"p95_latency_ms":                 fmt.Sprintf("%.2fms", snapshot.P95ShadowLatencyMs),
			"total_decisions_logged":         logStore.Size(),
			"old_new_agreement_rate":         fmt.Sprintf("%.2f%%", snapshot.OldVsNewAgreementRate*100),
		}
		w.Header().Set("X-Shadow-Validation", fmt.Sprintf("%v", validation))
		json.NewEncoder(w).Encode(map[string]interface{}{
			"config":              response.Config,
			"metrics":             response.Metrics,
			"validation":          validation,
			"recent_decisions":    response.RecentDecisions,
			"mismatch_samples":    response.MismatchSamples,
			"account_zero_samples": response.AccountZeroSamples,
			"disabled_account_samples": response.DisabledAccountSamples,
			"fallback_samples":    response.FallbackSamples,
		})
		return
	}

	json.NewEncoder(w).Encode(response)
}

func toShadowDecisionSummary(entry *RoutingDecisionLogEntry) ShadowDecisionSummary {
	if entry == nil {
		return ShadowDecisionSummary{}
	}
	return ShadowDecisionSummary{
		RequestID:           entry.RequestID,
		PreferredPool:       entry.PreferredPool,
		PreferredTier:       entry.PreferredTier,
		Confidence:          entry.Confidence,
		FinalDecisionSource: entry.FinalDecisionSource,
		FallbackReason:      entry.FallbackReason,
		OldAccountID:        entry.OldSelectedAccountID,
		NewAccountID:        entry.NewSuggestedAccountID,
		IsAgree:             entry.IsAgree,
		ShadowLatencyMs:     entry.ShadowLatencyMs,
		CreatedAt:           entry.CreatedAt,
	}
}

// HandleDebugShadowMetrics 处理 /v1/debug/shadow/metrics
// 仅输出 shadow metrics 快照
func HandleDebugShadowMetrics(
	w http.ResponseWriter,
	r *http.Request,
	shadowRouter *ShadowRouter,
) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error": "method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	if shadowRouter == nil {
		http.Error(w, `{"error": "shadow router not initialized"}`, http.StatusNotFound)
		return
	}

	snapshot := shadowRouter.Stats()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(snapshot)
}
