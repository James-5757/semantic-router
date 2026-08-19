package vllm_pool_client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// =============================================================================
// VLLMSemanticPoolRouterClient vLLM Semantic Router HTTP 客户端
// 对应官方 API: POST /api/v1/classify/intent
// =============================================================================

// VLLMSemanticPoolRouterClient vLLM Pool Router HTTP 客户端
type VLLMSemanticPoolRouterClient struct {
	config    VLLMPoolConfig
	httpClient *http.Client
}

// NewVLLMSemanticPoolRouterClient 创建新的客户端
func NewVLLMSemanticPoolRouterClient(config VLLMPoolConfig) *VLLMSemanticPoolRouterClient {
	if config.Timeout == 0 {
		config.Timeout = 5 * time.Second
	}

	return &VLLMSemanticPoolRouterClient{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// =============================================================================
// 公开方法
// =============================================================================

// ClassifyIntent 对 prompt 进行意图分类
// 对应官方 API: POST /api/v1/classify/intent
func (c *VLLMSemanticPoolRouterClient) ClassifyIntent(ctx context.Context, prompt string) (*VLLMIntentResponse, error) {
	if !c.config.Enabled {
		return nil, fmt.Errorf("vLLM client is disabled")
	}

	url := fmt.Sprintf("%s/api/v1/classify/intent", c.config.BaseURL)

	// 构建请求 (使用官方 API 结构)
	reqBody := VLLMIntentRequest{
		Text: prompt,
		Options: &VLLMOptions{
			ReturnProbabilities: true,
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var result VLLMIntentResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// ClassifyIntentWithResult 对 prompt 进行意图分类，返回适配后的结果
func (c *VLLMSemanticPoolRouterClient) ClassifyIntentWithResult(ctx context.Context, prompt string, localPool string) *VLLMSemanticShadowResult {
	start := time.Now()

	resp, err := c.ClassifyIntent(ctx, prompt)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return &VLLMSemanticShadowResult{
			UsedForFinal:     false,
			Invoked:          true,
			ServiceAvailable: false,
			Error:            err.Error(),
			LatencyMs:        float64(latency),
		}
	}

	result := resp.AdaptResponse(localPool)
	result.LatencyMs = float64(latency)
	return result
}

// HealthCheck 检查服务健康状态
// 对应官方 API: GET /health
func (c *VLLMSemanticPoolRouterClient) HealthCheck(ctx context.Context) error {
	url := fmt.Sprintf("%s/health", c.config.BaseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed with status %d", resp.StatusCode)
	}

	return nil
}

// ReadyCheck 检查服务是否就绪
// 对应官方 API: GET /ready (更准确的健康检查)
func (c *VLLMSemanticPoolRouterClient) ReadyCheck(ctx context.Context) (bool, error) {
	url := fmt.Sprintf("%s/ready", c.config.BaseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	return false, fmt.Errorf("service not ready, status: %d", resp.StatusCode)
}

// GetClassifierInfo 获取分类器信息
// 对应官方 API: GET /info/classifier
func (c *VLLMSemanticPoolRouterClient) GetClassifierInfo(ctx context.Context) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/info/classifier", c.config.BaseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}

	var info map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}

	return info, nil
}

// GetOpenAPISpec 获取 OpenAPI Schema
// 对应官方 API: GET /openapi.json
func (c *VLLMSemanticPoolRouterClient) GetOpenAPISpec(ctx context.Context) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/openapi.json", c.config.BaseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}

	var spec map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&spec); err != nil {
		return nil, err
	}

	return spec, nil
}

// IsEnabled 检查是否启用
func (c *VLLMSemanticPoolRouterClient) IsEnabled() bool {
	return c.config.Enabled
}

// GetConfig 获取配置
func (c *VLLMSemanticPoolRouterClient) GetConfig() VLLMPoolConfig {
	return c.config
}