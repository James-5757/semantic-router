package semanticrouter

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// TakeoverDebugResponse /v1/debug/takeover/metrics 接口响应
type TakeoverDebugResponse struct {
	Config  SemanticRouterRuntimeConfig `json:"config"`
	Metrics TakeoverMetricsSnapshot     `json:"metrics"`
}

// TakeoverMetricsSnapshot takeover 相关指标快照
type TakeoverMetricsSnapshot struct {
	TakeoverTotal                  uint64  `json:"takeover_total"`
	TakeoverSuccess                uint64  `json:"takeover_success"`
	TakeoverError                  uint64  `json:"takeover_error"`
	TakeoverErrorRate              float64 `json:"takeover_error_rate"`
	TakeoverRateActual             float64 `json:"takeover_rate_actual"`
	TakeoverRateTarget             int     `json:"takeover_rate_target"`
	TakeoverFallbackCount          uint64  `json:"takeover_fallback_count"`
	TakeoverAccountZeroBlocked     uint64  `json:"takeover_account_zero_blocked"`
	TakeoverDisabledAccountBlocked uint64  `json:"takeover_disabled_account_blocked"`
	OldSchedulerFallbackCount      uint64  `json:"old_scheduler_fallback_count"`
	AverageLatencyMs               float64 `json:"average_latency_ms"`
	P95LatencyMs                   float64 `json:"p95_latency_ms"`
}

// TakeoverStatusResponse /v1/debug/takeover/status 接口响应
type TakeoverStatusResponse struct {
	TakeoverEnabled    bool   `json:"takeover_enabled"`
	TakeoverPercentage int    `json:"takeover_percentage"`
	IsTakeoverActive   bool   `json:"is_takeover_active"`
	Status             string `json:"status"` // "active", "shadow_only", "disabled"
}

// HandleDebugTakeoverMetrics 处理 /v1/debug/takeover/metrics
// 仅输出 takeover 相关指标
func HandleDebugTakeoverMetrics(
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
	config := shadowRouter.config

	metrics := TakeoverMetricsSnapshot{
		TakeoverTotal:                  snapshot.TakeoverTotal,
		TakeoverSuccess:                snapshot.TakeoverSuccess,
		TakeoverError:                  snapshot.TakeoverError,
		TakeoverRateActual:             snapshot.TakeoverRateActual,
		TakeoverRateTarget:             config.TakeoverPercentage,
		TakeoverFallbackCount:          snapshot.TakeoverFallbackCount,
		TakeoverAccountZeroBlocked:     snapshot.TakeoverAccountZeroBlocked,
		TakeoverDisabledAccountBlocked: snapshot.TakeoverDisabledAccountBlocked,
		OldSchedulerFallbackCount:      snapshot.OldSchedulerFallbackCount,
		AverageLatencyMs:               snapshot.AverageShadowLatencyMs,
		P95LatencyMs:                   snapshot.P95ShadowLatencyMs,
	}
	if metrics.TakeoverTotal > 0 {
		metrics.TakeoverErrorRate = float64(metrics.TakeoverError) / float64(metrics.TakeoverTotal)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

// HandleDebugTakeoverStatus 处理 /v1/debug/takeover/status
// 检查 takeover 是否在活跃状态
func HandleDebugTakeoverStatus(
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

	config := shadowRouter.config
	status := "disabled"
	if config.SemanticRouterTakeoverEnabled && config.TakeoverPercentage > 0 {
		status = "active"
	} else if config.SemanticRouterShadowEnabled {
		status = "shadow_only"
	}

	response := TakeoverStatusResponse{
		TakeoverEnabled:    config.SemanticRouterTakeoverEnabled,
		TakeoverPercentage: config.TakeoverPercentage,
		IsTakeoverActive:   config.SemanticRouterTakeoverEnabled && config.TakeoverPercentage > 0,
		Status:             status,
	}

	w.Header().Set("Content-Type", "application/json")

	// Header 快速检查
	if response.IsTakeoverActive {
		w.Header().Set("X-Takeover-Status", fmt.Sprintf("active_%d%%", config.TakeoverPercentage))
	} else {
		w.Header().Set("X-Takeover-Status", "inactive")
	}

	json.NewEncoder(w).Encode(response)
}
