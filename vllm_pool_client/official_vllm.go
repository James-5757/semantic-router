package vllm_pool_client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// =============================================================================
// OfficialVLLMSemanticRouter 官方 vLLM Semantic Router 客户端
// 调用 http://127.0.0.1:8080/api/v1/eval 接口获取 Signal-Decision 结果
// Shadow Only
// =============================================================================

// OfficialVLLMSemanticRouter 官方 vLLM Semantic Router 客户端
type OfficialVLLMSemanticRouter struct {
	config     OfficialVLLMConfig
	httpClient *http.Client
	mu         sync.RWMutex
}

// OfficialVLLMConfig 官方 vLLM 配置
type OfficialVLLMConfig struct {
	Enabled    bool          `json:"enabled"`
	BaseURL    string        `json:"base_url"`    // 默认 http://127.0.0.1:8080
	Timeout    time.Duration `json:"timeout"`     // 请求超时
	ShadowOnly bool          `json:"shadow_only"` // 永远只做 Shadow
}

// OfficialVLLMResult 官方 vLLM 结果
type OfficialVLLMResult struct {
	Provider string `json:"provider"`
	// API 可访问性
	APIAvailable    bool `json:"api_available"`     // HTTP 服务可访问
	ReadyEndpointOK bool `json:"ready_endpoint_ok"` // /ready 返回 200
	ReadyFlag       bool `json:"ready_flag"`        // /ready.ready 字段
	ReadyModels     int  `json:"ready_models"`      // /ready.ready_models (诊断字段)
	TotalModels     int  `json:"total_models"`      // /ready.total_models (诊断字段)

	// 分类器加载状态（仅诊断，不决定是否可用）
	ClassifierReady bool `json:"classifier_ready"` // Classifier 模型已加载

	// 评估状态
	EvaluationStatus     string                 `json:"evaluation_status"` // "not_evaluated" | "evaluated" | "error"
	MatchedSignals       map[string]float64     `json:"matched_signals"`
	MatchedDecision      string                 `json:"matched_decision"`
	RoutingDecision      string                 `json:"routing_decision"`
	RawTrace             map[string]interface{} `json:"raw_trace"`
	SemanticCategory     string                 `json:"semantic_category"`
	Confidence           float64                `json:"confidence"`
	TopK                 []CategoryScore        `json:"top_k"`
	LegacyMappedPool     string                 `json:"legacy_mapped_pool"`
	ExecutionFamily      string                 `json:"execution_family"`
	RequiredCapabilities []string               `json:"required_capabilities"`

	// Shadow 可用性判断
	OperationalForShadow bool   `json:"operational_for_shadow"`
	LastDecision         string `json:"last_decision"`

	// 详细信号值（用于 Playground 展示全部 7 个信号）
	SignalValues        map[string]float64 `json:"signal_values"`
	TopSignal           string             `json:"top_signal"`
	TopScore            float64            `json:"top_score"`
	EmbeddingConfidence float64            `json:"embedding_confidence"`
	EmbeddingTimeMs     float64            `json:"embedding_time_ms"`

	LatencyMs    float64 `json:"latency_ms"`
	Error        string  `json:"error"`
	UsedForFinal bool    `json:"used_for_final"`
}

// NewOfficialVLLMSemanticRouter 创建官方 vLLM 客户端
func NewOfficialVLLMSemanticRouter(config OfficialVLLMConfig) *OfficialVLLMSemanticRouter {
	if config.BaseURL == "" {
		config.BaseURL = "http://127.0.0.1:8080"
	}
	if config.Timeout == 0 {
		config.Timeout = 5 * time.Second
	}

	return &OfficialVLLMSemanticRouter{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// IsEnabled 检查是否启用
func (o *OfficialVLLMSemanticRouter) IsEnabled() bool {
	return o.config.Enabled
}

// CheckServiceReady 检查服务是否就绪
func (o *OfficialVLLMSemanticRouter) CheckServiceReady() (bool, error) {
	resp, err := o.httpClient.Get(o.config.BaseURL + "/ready")
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return false, fmt.Errorf("service returned status %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}

	ready, ok := result["ready"].(bool)
	return ready && ok, nil
}

// CheckDetailedReady 检查详细的 Ready 状态（仅诊断，不用于判断是否可用）
func (o *OfficialVLLMSemanticRouter) CheckDetailedReady() (apiAvailable, readyEndpointOK, readyFlag bool, readyModels, totalModels int, classifierReady bool, err error) {
	resp, err := o.httpClient.Get(o.config.BaseURL + "/ready")
	if err != nil {
		return false, false, false, 0, 0, false, err
	}
	defer resp.Body.Close()

	apiAvailable = true

	if resp.StatusCode != 200 {
		return true, false, false, 0, 0, false, fmt.Errorf("service returned status %d", resp.StatusCode)
	}

	readyEndpointOK = true

	var readyResult map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&readyResult); err != nil {
		return true, true, false, 0, 0, false, err
	}

	readyFlag, _ = readyResult["ready"].(bool)

	// ready_models 和 total_models 作为诊断字段
	if rm, ok := readyResult["ready_models"].(float64); ok {
		readyModels = int(rm)
	}
	if tm, ok := readyResult["total_models"].(float64); ok {
		totalModels = int(tm)
	}

	// classifier_ready: 仅当 ready_models > 0 时认为 classifier 已加载
	classifierReady = readyModels > 0

	return apiAvailable, readyEndpointOK, readyFlag, readyModels, totalModels, classifierReady, nil
}

// decisionToSemanticCategory 将官方 decision_name 映射为语义类别
func decisionToSemanticCategory(decisionName string) (SemanticCategory, bool) {
	switch decisionName {
	case "route_code":
		return CategoryCode, true
	case "route_data_analysis", "route_data":
		return CategoryDataAnalysis, true
	case "route_document":
		return CategoryDocument, true
	case "route_vision_understanding", "route_vision":
		return CategoryVisionUnderstanding, true
	case "route_image_generation":
		return CategoryImageGeneration, true
	case "route_simple_chat":
		return CategorySimpleChat, true
	case "route_general":
		return CategoryGeneral, true
	case "default-route":
		return CategoryGeneral, true
	default:
		return CategoryGeneral, false
	}
}

// Classify 执行官方 vLLM 分类
// 即使 ready_models=0，也调用 /api/v1/eval 获取 Signal-Decision 结果
func (o *OfficialVLLMSemanticRouter) Classify(ctx context.Context, prompt string) *OfficialVLLMResult {
	result := &OfficialVLLMResult{
		Provider:         "official_vllm_sr",
		UsedForFinal:     false, // 永远不做最终决策
		EvaluationStatus: "not_evaluated",
		SemanticCategory: "",
		Confidence:       0,
	}

	// 检查详细的 Ready 状态（仅诊断，不影响 eval 调用）
	apiAvailable, readyEndpointOK, readyFlag, readyModels, totalModels, classifierReady, err := o.CheckDetailedReady()
	result.APIAvailable = apiAvailable
	result.ReadyEndpointOK = readyEndpointOK
	result.ReadyFlag = readyFlag
	result.ReadyModels = readyModels
	result.TotalModels = totalModels
	result.ClassifierReady = classifierReady

	if err != nil || !apiAvailable {
		result.Error = fmt.Sprintf("service error: %v", err)
		result.OperationalForShadow = false
		return result
	}

	// ========================================
	// 始终调用 /api/v1/eval，即使 ready_models=0
	// 因为实际测试证明 eval 接口仍然返回有效 decision
	// ========================================
	start := time.Now()

	reqBody := map[string]string{
		"text": prompt,
	}
	jsonBody, _ := json.Marshal(reqBody)

	evalResp, err := o.httpClient.Post(o.config.BaseURL+"/api/v1/eval", "application/json", bytes.NewBuffer(jsonBody))
	result.LatencyMs = float64(time.Since(start).Milliseconds())

	if err != nil {
		result.Error = fmt.Sprintf("eval request failed: %v", err)
		result.EvaluationStatus = "error"
		result.OperationalForShadow = false
		return result
	}
	defer evalResp.Body.Close()

	if evalResp.StatusCode != 200 {
		result.Error = fmt.Sprintf("eval returned status %d", evalResp.StatusCode)
		result.EvaluationStatus = "error"
		result.OperationalForShadow = false
		return result
	}

	var evalResponse map[string]interface{}
	if err := json.NewDecoder(evalResp.Body).Decode(&evalResponse); err != nil {
		result.Error = fmt.Sprintf("eval JSON decode failed: %v", err)
		result.EvaluationStatus = "error"
		result.OperationalForShadow = false
		return result
	}

	result.RawTrace = evalResponse
	result.EvaluationStatus = "evaluated"

	// 从 decision_result.decision_name 提取决策
	// 这是最可靠的字段，由官方 Decision Engine 直接返回
	var decisionName string
	if decision, ok := evalResponse["decision_result"].(map[string]interface{}); ok {
		if dn, ok := decision["decision_name"].(string); ok {
			decisionName = dn
			result.MatchedDecision = dn
			result.LastDecision = dn
		}
	}

	// 也检查 routing_decision
	routingDecision, _ := evalResponse["routing_decision"].(string)
	if routingDecision != "" && decisionName == "" {
		decisionName = routingDecision
		result.MatchedDecision = routingDecision
		result.LastDecision = routingDecision
	}

	// 解析 matched_signals（嵌套结构）
	if dr, ok := evalResponse["decision_result"].(map[string]interface{}); ok {
		if ms, ok := dr["matched_signals"].(map[string]interface{}); ok {
			matched := make(map[string]float64)
			for _, signals := range ms {
				if list, ok := signals.([]interface{}); ok {
					for _, s := range list {
						if name, ok := s.(string); ok {
							matched[name] = 1.0
						}
					}
				}
			}
			result.MatchedSignals = matched
			// Some vLLM Semantic Router versions return the matched rule in
			// matched_signals but leave decision_result.decision_name empty.
			// Preserve the official shadow result by deriving the decision from
			// the actual route_* signal instead of treating it as a service error.
			if decisionName == "" {
				for _, key := range []string{"keywords", "embedding", "domain"} {
					if signals, ok := ms[key].([]interface{}); ok {
						for _, signal := range signals {
							if name, ok := signal.(string); ok && strings.HasPrefix(name, "route_") {
								decisionName = name
								result.MatchedDecision = name
								result.LastDecision = name
								break
							}
						}
					}
					if decisionName != "" {
						break
					}
				}
			}
		}
	}

	// 提取 embedding confidence 和 execution time
	if metrics, ok := evalResponse["metrics"].(map[string]interface{}); ok {
		if emb, ok := metrics["embedding"].(map[string]interface{}); ok {
			if c, ok := emb["confidence"].(float64); ok {
				result.EmbeddingConfidence = c
			}
			if t, ok := emb["execution_time_ms"].(float64); ok {
				result.EmbeddingTimeMs = t
			}
		}
	}

	// 解析 signal_values（全部信号分数，含 :best、:support、:prototype_count）
	result.SignalValues = make(map[string]float64)
	if sv, ok := evalResponse["signal_values"].(map[string]interface{}); ok {
		for k, v := range sv {
			if fv, ok := v.(float64); ok {
				result.SignalValues[k] = fv
			}
		}
	}

	// 解析 signal_confidences（高置信度信号）
	if sc, ok := evalResponse["signal_confidences"].(map[string]interface{}); ok {
		confidences := make(map[string]float64)
		for signalName, conf := range sc {
			if c, ok := conf.(float64); ok {
				confidences[signalName] = c
			}
		}
		// 合并 confidences 到 matchedSignals
		if result.MatchedSignals == nil {
			result.MatchedSignals = make(map[string]float64)
		}
		for k, v := range confidences {
			result.MatchedSignals[k] = v
		}
	}

	// 从 MatchedSignals 或 SignalValues 找出 Top Signal
	result.TopScore = 0
	result.TopSignal = ""
	for k, v := range result.SignalValues {
		if strings.HasPrefix(k, "embedding:") && !strings.Contains(k, ":") {
			if v > result.TopScore {
				result.TopScore = v
				result.TopSignal = k
			}
		}
	}
	// 如果 signal_values 中没有主信号，回退到 signal_confidences
	if result.TopSignal == "" {
		for k, v := range result.MatchedSignals {
			if v > result.TopScore {
				result.TopScore = v
				result.TopSignal = k
			}
		}
	}

	// 解析 routing_decision
	routingDecision, _ = evalResponse["routing_decision"].(string)
	if routingDecision != "" {
		result.RoutingDecision = routingDecision
		result.LastDecision = routingDecision
	}

	// 确定语义类别
	decisionName = result.MatchedDecision
	if decisionName == "" {
		decisionName = routingDecision
	}
	if decisionName == "" {
		result.EvaluationStatus = "error"
		result.Error = "eval_no_decision: no decision_name in response"
		result.OperationalForShadow = false
		return result
	}

	// 从 decision_name 映射语义类别
	if cat, known := decisionToSemanticCategory(decisionName); known {
		result.SemanticCategory = string(cat)
		result.OperationalForShadow = true
	} else {
		result.SemanticCategory = "general"
		result.OperationalForShadow = true
		if result.Error == "" {
			result.Error = "unknown_decision: " + decisionName
		}
	}

	// 设置 confidence（优先使用 embedding confidence）
	if result.EmbeddingConfidence > 0 {
		result.Confidence = result.EmbeddingConfidence
	} else {
		for _, conf := range result.MatchedSignals {
			if conf > result.Confidence {
				result.Confidence = conf
			}
		}
	}

	// 映射到 Pool
	category := SemanticCategory(result.SemanticCategory)
	result.LegacyMappedPool = string(GetLegacyPool(category))
	result.ExecutionFamily = string(GetExecutionFamily(category))

	// Capabilities
	caps := GetCapabilities(category)
	for _, c := range caps {
		result.RequiredCapabilities = append(result.RequiredCapabilities, string(c))
	}

	// 生成 Top-K
	result.TopK = result.generateTopK()

	return result
}

// generateTopK 生成 Top-K
func (r *OfficialVLLMResult) generateTopK() []CategoryScore {
	if r.MatchedSignals == nil || len(r.MatchedSignals) == 0 {
		return []CategoryScore{
			{Category: r.SemanticCategory, Score: r.Confidence},
		}
	}

	var topK []CategoryScore
	for cat, conf := range r.MatchedSignals {
		topK = append(topK, CategoryScore{
			Category: cat,
			Score:    conf,
		})
	}
	return topK
}
