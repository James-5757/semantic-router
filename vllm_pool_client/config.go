package vllm_pool_client

import "time"

// =============================================================================
// VLLMPoolConfig vLLM Pool Router 客户端配置
// =============================================================================

// VLLMPoolConfig vLLM Pool Router 客户端配置
type VLLMPoolConfig struct {
	// Enabled 是否启用 vLLM Shadow
	Enabled bool `json:"enabled" yaml:"enabled"`

	// BaseURL vLLM Semantic Router API 地址
	BaseURL string `json:"base_url" yaml:"base_url"`

	// Timeout 请求超时时间
	Timeout time.Duration `json:"timeout" yaml:"timeout"`

	// ShadowOnly 只做 Shadow，不影响最终决策
	ShadowOnly bool `json:"shadow_only" yaml:"shadow_only"`

	// MockMode 测试模式，使用模拟响应
	MockMode bool `json:"mock_mode" yaml:"mock_mode"`

	// ConfidenceThreshold 置信度阈值
	ConfidenceThreshold float64 `json:"confidence_threshold" yaml:"confidence_threshold"`
}

// DefaultVLLMPoolConfig 返回默认配置
func DefaultVLLMPoolConfig() VLLMPoolConfig {
	return VLLMPoolConfig{
		Enabled:              false,
		BaseURL:              "http://localhost:8080",
		Timeout:              5 * time.Second,
		ShadowOnly:           true,
		MockMode:             false,
		ConfidenceThreshold:  0.5,
	}
}