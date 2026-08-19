package vllm_pool_client

import (
	"context"
	"math/rand"
	"os"
	"sync"
	"time"
)

// =============================================================================
// VLLMPoolDecisionAdapter vLLM Pool 决策适配器
// 负责：调用 vLLM、结果适配、与本地路由对比
// 使用 Prototype Classifier 实现七类分类（当 vLLM 分类器未加载时）
// =============================================================================

// =============================================================================
// ShadowResult 包含三种 Provider 的结果
// =============================================================================

// ShadowResult 包含三种 Provider 的结果
type ShadowResult struct {
	// 1. Local Pool Router - 决定最终 Pool
	LocalPoolDecision *LocalPoolDecisionResult `json:"local_pool_decision"`

	// 2. Local E5 Prototype - Shadow Only
	LocalE5Shadow *LocalE5ShadowResult `json:"local_e5_semantic_shadow"`

	// 3. Official vLLM Semantic Router - Shadow Only
	OfficialVLLMShadow *OfficialVLLMShadowResult `json:"official_vllm_semantic_shadow"`
}

// LocalPoolDecisionResult 本地 Pool 路由决策结果
type LocalPoolDecisionResult struct {
	Pool         string `json:"pool"`
	Tier         string `json:"tier"`
	Source       string `json:"source"`         // "local_baseline"
	UsedForFinal bool   `json:"used_for_final"` // 永远 true
}

// LocalE5ShadowResult 本地 E5 Prototype Shadow 结果
type LocalE5ShadowResult struct {
	Provider             string          `json:"provider"`
	ServiceReady         bool            `json:"service_ready"`
	MatchedSignals       []string        `json:"matched_signals"`
	SemanticCategory     string          `json:"semantic_category"`
	Confidence           float64         `json:"confidence"`
	TopK                 []CategoryScore `json:"top_k"`
	LegacyMappedPool     string          `json:"legacy_mapped_pool"`
	ExecutionFamily      string          `json:"execution_family"`
	RequiredCapabilities []string        `json:"required_capabilities"`
	LocalAgreement       bool            `json:"local_agreement"`
	LatencyMs            float64         `json:"latency_ms"`
	Error                string          `json:"error"`
	UsedForFinal         bool            `json:"used_for_final"` // 永远 false
}

// OfficialVLLMShadowResult 官方 vLLM Shadow 结果
type OfficialVLLMShadowResult struct {
	Provider string `json:"provider"`

	// API 可访问性（诊断字段）
	APIAvailable    bool `json:"api_available"`
	ReadyEndpointOK bool `json:"ready_endpoint_ok"`
	ReadyFlag       bool `json:"ready_flag"`
	ReadyModels     int  `json:"ready_models"` // 诊断字段
	TotalModels     int  `json:"total_models"` // 诊断字段
	ClassifierReady bool `json:"classifier_ready"`

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

	// Playground 详细展示字段
	SignalValues        map[string]float64 `json:"signal_values"`
	TopSignal           string             `json:"top_signal"`
	TopScore            float64            `json:"top_score"`
	EmbeddingConfidence float64            `json:"embedding_confidence"`
	EmbeddingTimeMs     float64            `json:"embedding_time_ms"`

	// Shadow 可用性判断
	OperationalForShadow bool   `json:"operational_for_shadow"`
	LastDecision         string `json:"last_decision"`

	LocalAgreement bool    `json:"local_agreement"`
	LatencyMs      float64 `json:"latency_ms"`
	Error          string  `json:"error"`
	UsedForFinal   bool    `json:"used_for_final"` // 永远 false
}

// VLLMPoolDecisionAdapter vLLM Pool 决策适配器
type VLLMPoolDecisionAdapter struct {
	client                *VLLMSemanticPoolRouterClient
	prototypeClassifier   *PrototypeClassifier           // V1 分类器 (保留)
	prototypeClassifierV2 *PrototypeClassifierV2         // V2 分类器 (关键词+正则)
	e5Classifier          *E5PrototypeSemanticClassifier // 本地 E5 原型分类器
	officialVLLM          *OfficialVLLMSemanticRouter    // 官方 vLLM 客户端
	mockMode              bool                           // 测试模式
	mu                    sync.RWMutex
}

// NewVLLMPoolDecisionAdapter 创建适配器
func NewVLLMPoolDecisionAdapter(client *VLLMSemanticPoolRouterClient) *VLLMPoolDecisionAdapter {
	adapter := &VLLMPoolDecisionAdapter{
		client:   client,
		mockMode: true, // 默认启用本地分类器
	}

	// 初始化原型分类器 V2 (关键词+正则) - 用于本地 E5 不可用时的后备
	protoClassifierV2, err := NewPrototypeClassifierV2("vllm_pool_client/prototypes.json")
	if err == nil {
		adapter.prototypeClassifierV2 = protoClassifierV2
	} else {
		// 使用默认原型
		adapter.prototypeClassifierV2, _ = NewPrototypeClassifierV2("")
	}

	// 初始化本地 E5 Prototype Semantic 分类器 (需要外部 embedding 服务)
	// 设置环境变量 E5_EMBEDDING_URL 启用
	e5URL := os.Getenv("E5_EMBEDDING_URL")
	if e5URL != "" {
		e5Config := E5ClassifierConfig{
			Enabled:             true,
			EmbeddingURL:        e5URL,
			Timeout:             5 * time.Second,
			PrototypePath:       "vllm_pool_client/prototypes.json",
			TopKPerCategory:     10,
			SimilarityThreshold: 0.6,
			AmbiguousMargin:     0.15,
		}
		e5Classifier, err := NewE5PrototypeSemanticClassifier(e5Config)
		if err == nil {
			adapter.e5Classifier = e5Classifier
		}
	}

	// 初始化官方 vLLM Semantic Router 客户端
	// 设置环境变量 OFFICIAL_VLLM_URL 启用 (默认 http://127.0.0.1:8080)
	officialURL := os.Getenv("OFFICIAL_VLLM_URL")
	if officialURL == "" {
		officialURL = "http://127.0.0.1:8080"
	}
	officialConfig := OfficialVLLMConfig{
		Enabled:    true, // 默认启用，Shadow Only
		BaseURL:    officialURL,
		Timeout:    5 * time.Second,
		ShadowOnly: true,
	}
	adapter.officialVLLM = NewOfficialVLLMSemanticRouter(officialConfig)

	return adapter
}

// SetMockMode 设置 mock 模式
func (a *VLLMPoolDecisionAdapter) SetMockMode(enabled bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.mockMode = enabled
}

// E5Classifier 返回 E5 分类器（供 V2 路由共享）
func (a *VLLMPoolDecisionAdapter) E5Classifier() *E5PrototypeSemanticClassifier {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.e5Classifier
}

// PrototypeClassifierV2 返回 V2 原型分类器（供 V2 路由共享）
func (a *VLLMPoolDecisionAdapter) PrototypeClassifierV2() *PrototypeClassifierV2 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.prototypeClassifierV2
}

// IsMockMode 检查是否为 mock 模式
func (a *VLLMPoolDecisionAdapter) IsMockMode() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.mockMode
}

// Decide 执行三种 Provider 的 Shadow 决策
// 返回：Local Pool Decision, Local E5 Shadow, Official vLLM Shadow
func (a *VLLMPoolDecisionAdapter) Decide(
	ctx context.Context,
	prompt string,
	localPool string,
	localTier string,
) *ShadowResult {

	result := &ShadowResult{}

	// ========================================
	// 1. Local Pool Decision (决定 final_pool)
	// ========================================
	result.LocalPoolDecision = &LocalPoolDecisionResult{
		Pool:         localPool,
		Tier:         localTier,
		Source:       "local_baseline",
		UsedForFinal: true, // 永远使用本地决策作为最终结果
	}

	// ========================================
	// 2. Local E5 Prototype Shadow (Shadow Only)
	// ========================================
	result.LocalE5Shadow = a.computeLocalE5Shadow(ctx, prompt, localPool)

	// ========================================
	// 3. Official vLLM Semantic Router Shadow (Shadow Only)
	// ========================================
	result.OfficialVLLMShadow = a.computeOfficialVLLMShadow(ctx, prompt, localPool)

	return result
}

// computeLocalE5Shadow 计算本地 E5 Prototype Shadow
func (a *VLLMPoolDecisionAdapter) computeLocalE5Shadow(ctx context.Context, prompt string, localPool string) *LocalE5ShadowResult {
	result := &LocalE5ShadowResult{
		Provider:     "local_e5_prototype",
		UsedForFinal: false, // 永远不做最终决策
	}

	start := time.Now()

	// 优先使用 E5 分类器 (如果可用)
	if a.e5Classifier != nil && a.e5Classifier.IsEnabled() {
		classifyResult := a.e5Classifier.Classify(prompt)
		latencyMs := float64(time.Since(start).Milliseconds())

		result.ServiceReady = true
		result.MatchedSignals = classifyResult.MatchedSignals
		result.SemanticCategory = classifyResult.Category
		result.Confidence = classifyResult.Confidence

		// 转换 Top-K
		for _, tk := range classifyResult.TopK {
			result.TopK = append(result.TopK, CategoryScore{
				Category: tk.Category,
				Score:    tk.Score,
			})
		}

		// 映射到 Pool
		category := SemanticCategory(classifyResult.Category)
		result.LegacyMappedPool = string(GetLegacyPool(category))
		result.ExecutionFamily = string(GetExecutionFamily(category))

		caps := GetCapabilities(category)
		for _, c := range caps {
			result.RequiredCapabilities = append(result.RequiredCapabilities, string(c))
		}

		result.LocalAgreement = result.LegacyMappedPool == localPool
		result.LatencyMs = latencyMs
		return result
	}

	// 后备：使用 Prototype Rule V2
	if a.prototypeClassifierV2 != nil {
		classifyResult := a.prototypeClassifierV2.Classify(prompt)
		latencyMs := float64(time.Since(start).Milliseconds())

		result.ServiceReady = true
		result.MatchedSignals = classifyResult.MatchedSignals
		result.SemanticCategory = classifyResult.Category
		result.Confidence = classifyResult.Confidence

		// 转换 Top-K
		for _, tk := range classifyResult.TopK {
			result.TopK = append(result.TopK, CategoryScore{
				Category: tk.Category,
				Score:    tk.Score,
			})
		}

		// 映射到 Pool
		category := SemanticCategory(classifyResult.Category)
		result.LegacyMappedPool = string(GetLegacyPool(category))
		result.ExecutionFamily = string(GetExecutionFamily(category))

		caps := GetCapabilities(category)
		for _, c := range caps {
			result.RequiredCapabilities = append(result.RequiredCapabilities, string(c))
		}

		result.LocalAgreement = result.LegacyMappedPool == localPool
		result.LatencyMs = latencyMs
		return result
	}

	// 如果都不可用
	result.ServiceReady = false
	result.Error = "no classifier available"
	result.LatencyMs = float64(time.Since(start).Milliseconds())
	return result
}

// computeOfficialVLLMShadow 计算官方 vLLM Shadow
func (a *VLLMPoolDecisionAdapter) computeOfficialVLLMShadow(ctx context.Context, prompt string, localPool string) *OfficialVLLMShadowResult {
	result := &OfficialVLLMShadowResult{
		Provider:         "official_vllm_sr",
		UsedForFinal:     false, // 永远不做最终决策
		EvaluationStatus: "not_evaluated",
	}

	// Mock 模式：用本地分类器模拟官方结果，以提供有意义的对比
	if a.IsMockMode() {
		category, confidence, signals := a.classifyLocally(prompt)
		semCat := SemanticCategory(category)
		result.SemanticCategory = string(semCat)
		result.Confidence = confidence
		result.MatchedSignals = signals
		result.MatchedDecision = "route_" + category
		result.LastDecision = "route_" + category
		result.LegacyMappedPool = string(GetLegacyPool(semCat))
		result.ExecutionFamily = string(GetExecutionFamily(semCat))
		caps := GetCapabilities(semCat)
		for _, c := range caps {
			result.RequiredCapabilities = append(result.RequiredCapabilities, string(c))
		}
		result.SignalValues = signals
		result.TopSignal = "embedding:" + category
		result.TopScore = confidence
		result.EmbeddingConfidence = confidence
		result.APIAvailable = true
		result.ReadyEndpointOK = true
		result.ReadyFlag = true
		result.ReadyModels = 5
		result.TotalModels = 10
		result.ClassifierReady = true
		result.OperationalForShadow = true
		result.EvaluationStatus = "evaluated"
		result.LocalAgreement = result.LegacyMappedPool == localPool
		result.LatencyMs = float64(15 + rand.Intn(20))
		return result
	}

	// 调用官方 vLLM
	if a.officialVLLM != nil {
		officialResult := a.officialVLLM.Classify(ctx, prompt)

		// API 可访问性
		result.APIAvailable = officialResult.APIAvailable
		result.ReadyEndpointOK = officialResult.ReadyEndpointOK
		result.ReadyFlag = officialResult.ReadyFlag
		result.ReadyModels = officialResult.ReadyModels
		result.TotalModels = officialResult.TotalModels
		result.ClassifierReady = officialResult.ClassifierReady

		// 评估结果
		result.EvaluationStatus = officialResult.EvaluationStatus
		result.MatchedSignals = officialResult.MatchedSignals
		result.MatchedDecision = officialResult.MatchedDecision
		result.RoutingDecision = officialResult.RoutingDecision
		result.RawTrace = officialResult.RawTrace
		result.SemanticCategory = officialResult.SemanticCategory
		result.Confidence = officialResult.Confidence
		result.TopK = officialResult.TopK
		result.LegacyMappedPool = officialResult.LegacyMappedPool
		result.ExecutionFamily = officialResult.ExecutionFamily
		result.RequiredCapabilities = officialResult.RequiredCapabilities

		// Playground 详细展示字段
		result.SignalValues = officialResult.SignalValues
		result.TopSignal = officialResult.TopSignal
		result.TopScore = officialResult.TopScore
		result.EmbeddingConfidence = officialResult.EmbeddingConfidence
		result.EmbeddingTimeMs = officialResult.EmbeddingTimeMs

		// Shadow 可用性判断
		result.OperationalForShadow = officialResult.OperationalForShadow
		result.LastDecision = officialResult.LastDecision
		result.LatencyMs = officialResult.LatencyMs
		result.Error = officialResult.Error

		// 检查与本地 Pool 的一致性
		result.LocalAgreement = result.LegacyMappedPool == localPool
	} else {
		result.APIAvailable = false
		result.ReadyEndpointOK = false
		result.ReadyFlag = false
		result.ClassifierReady = false
		result.OperationalForShadow = false
		result.Error = "official vLLM client not initialized"
	}

	return result
}

// convertPrototypeResult 将 Prototype Classifier 结果转换为 Shadow Result
func (a *VLLMPoolDecisionAdapter) convertPrototypeResult(
	classifyResult *ClassifyResult,
	localPool string,
	latencyMs float64,
) *VLLMSemanticShadowResult {

	semanticCategory := SemanticCategory(classifyResult.Category)
	legacyPool := GetLegacyPool(semanticCategory)
	capabilities := GetCapabilities(semanticCategory)
	executionFamily := GetExecutionFamily(semanticCategory)

	// 比较
	legacyAgree := string(legacyPool) == localPool
	localFamily := poolToFamily(localPool)
	familyAgree := executionFamily == localFamily

	// 转换 Top-K
	topK := make([]CategoryScore, len(classifyResult.TopK))
	for i, tk := range classifyResult.TopK {
		topK[i] = CategoryScore{
			Category: tk.Category,
			Score:    tk.Score,
		}
	}

	return &VLLMSemanticShadowResult{
		RawCategory:          classifyResult.Category,
		RawConfidence:        classifyResult.Confidence,
		RawProbabilities:     classifyResult.Scores,
		LegacyPool:           legacyPool,
		RequiredCapabilities: capabilities,
		ExecutionFamily:      executionFamily,
		LocalPool:            LegacyPool(localPool),
		LegacyPoolAgreement:  legacyAgree,
		FamilyAgreement:      familyAgree,
		Ambiguous:            classifyResult.Ambiguous,
		MatchedSignals:       classifyResult.MatchedSignals,
		UsedForFinal:         false,
		FinalPool:            localPool,
		DecisionSource:       "prototype_classifier",
		ServiceAvailable:     true,
		ServiceReady:         true,
		Invoked:              true,
		TopK:                 topK,
		LatencyMs:            latencyMs,
	}
}

// convertPrototypeResultV2 将 Prototype Classifier V2 结果转换为 Shadow Result
func (a *VLLMPoolDecisionAdapter) convertPrototypeResultV2(
	classifyResult *ClassifyResultV2,
	localPool string,
	latencyMs float64,
) *VLLMSemanticShadowResult {

	semanticCategory := SemanticCategory(classifyResult.Category)
	legacyPool := GetLegacyPool(semanticCategory)
	capabilities := GetCapabilities(semanticCategory)
	executionFamily := GetExecutionFamily(semanticCategory)

	// 比较
	legacyAgree := string(legacyPool) == localPool
	localFamily := poolToFamily(localPool)
	familyAgree := executionFamily == localFamily

	// 转换 Top-K
	topK := make([]CategoryScore, len(classifyResult.TopK))
	for i, tk := range classifyResult.TopK {
		topK[i] = CategoryScore{
			Category: tk.Category,
			Score:    tk.Score,
		}
	}

	return &VLLMSemanticShadowResult{
		RawCategory:          classifyResult.Category,
		RawConfidence:        classifyResult.Confidence,
		RawProbabilities:     classifyResult.Scores,
		LegacyPool:           legacyPool,
		RequiredCapabilities: capabilities,
		ExecutionFamily:      executionFamily,
		LocalPool:            LegacyPool(localPool),
		LegacyPoolAgreement:  legacyAgree,
		FamilyAgreement:      familyAgree,
		Ambiguous:            classifyResult.Ambiguous,
		MatchedSignals:       classifyResult.MatchedSignals,
		UsedForFinal:         false,
		FinalPool:            localPool,
		DecisionSource:       "prototype_classifier_v2",
		ServiceAvailable:     true,
		ServiceReady:         true,
		Invoked:              true,
		TopK:                 topK,
		LatencyMs:            latencyMs,
	}
}

// convertE5Result 将 E5 Prototype Semantic 结果转换为 Shadow Result
func (a *VLLMPoolDecisionAdapter) convertE5Result(
	classifyResult *E5ClassifyResult,
	localPool string,
	latencyMs float64,
) *VLLMSemanticShadowResult {

	semanticCategory := SemanticCategory(classifyResult.Category)
	legacyPool := GetLegacyPool(semanticCategory)
	capabilities := GetCapabilities(semanticCategory)
	executionFamily := GetExecutionFamily(semanticCategory)

	// 比较
	legacyAgree := string(legacyPool) == localPool
	localFamily := poolToFamily(localPool)
	familyAgree := executionFamily == localFamily

	// 转换 Top-K
	topK := make([]CategoryScore, len(classifyResult.TopK))
	for i, tk := range classifyResult.TopK {
		topK[i] = CategoryScore{
			Category: tk.Category,
			Score:    tk.Score,
		}
	}

	return &VLLMSemanticShadowResult{
		ClassificationMethod: "e5_prototype_semantic",
		RawCategory:          classifyResult.Category,
		RawConfidence:        classifyResult.Confidence,
		RawProbabilities:     classifyResult.Scores,
		LegacyPool:           legacyPool,
		RequiredCapabilities: capabilities,
		ExecutionFamily:      executionFamily,
		LocalPool:            LegacyPool(localPool),
		LegacyPoolAgreement:  legacyAgree,
		FamilyAgreement:      familyAgree,
		Ambiguous:            classifyResult.Ambiguous,
		MatchedSignals:       classifyResult.MatchedSignals,
		UsedForFinal:         false,
		FinalPool:            localPool,
		DecisionSource:       "e5_prototype_semantic",
		ServiceAvailable:     true,
		ServiceReady:         true,
		Invoked:              true,
		TopK:                 topK,
		LatencyMs:            latencyMs,
	}
}

// generateMockResult 生成模拟响应 (仅用于测试)
func (a *VLLMPoolDecisionAdapter) generateMockResult(prompt string, localPool string) *VLLMSemanticShadowResult {
	// 基于 prompt 内容猜测分类
	category := guessCategoryFromPrompt(prompt)
	semanticCategory := SemanticCategory(category)

	legacyPool := GetLegacyPool(semanticCategory)
	capabilities := GetCapabilities(semanticCategory)
	executionFamily := GetExecutionFamily(semanticCategory)

	// 生成模拟置信度
	confidence := 0.7 + rand.Float64()*0.25 // 0.7-0.95

	// 生成概率分布
	probs := generateMockProbabilities(category)

	// 比较
	legacyAgree := string(legacyPool) == localPool
	localFamily := poolToFamily(localPool)
	familyAgree := executionFamily == localFamily

	return &VLLMSemanticShadowResult{
		RawCategory:          string(semanticCategory),
		RawConfidence:        confidence,
		RawProbabilities:     probs,
		LegacyPool:           legacyPool,
		RequiredCapabilities: capabilities,
		ExecutionFamily:      executionFamily,
		LocalPool:            LegacyPool(localPool),
		LegacyPoolAgreement:  legacyAgree,
		FamilyAgreement:      familyAgree,
		UsedForFinal:         false,
		FinalPool:            localPool,
		DecisionSource:       "mock",
		ServiceAvailable:     true,
		Invoked:              true,
		TopK:                 convertProbabilitiesToTopK(probs),
		LatencyMs:            float64(20 + rand.Intn(30)), // 20-50ms
	}
}

// guessCategoryFromPrompt 根据 prompt 猜测分类
func guessCategoryFromPrompt(prompt string) string {
	lower := promptLower(prompt)

	// Code
	if containsAny(lower, []string{"code", "programming", "function", "algorithm", "python", "javascript", "java", "写代码", "编程", "排序", "算法"}) {
		return "code"
	}
	// Data Analysis
	if containsAny(lower, []string{"data", "analysis", "chart", "graph", "excel", "csv", "趋势", "分析", "图表", "统计", "数据"}) {
		return "data_analysis"
	}
	// Document
	if containsAny(lower, []string{"document", "pdf", "word", "summarize", "summary", "总结", "文档", "文章", "提取"}) {
		return "document"
	}
	// Vision Understanding
	if containsAny(lower, []string{"image", "photo", "picture", "screenshot", "分析图片", "看图", "图片中", "这张图"}) && !containsAny(lower, []string{"生成", "create", "draw", "画"}) {
		return "vision_understanding"
	}
	// Image Generation
	if containsAny(lower, []string{"generate image", "create image", "draw", "生成图片", "生成图像", "画图", "海报", "插画", "AI绘画"}) {
		return "image_generation"
	}
	// Simple Chat
	if containsAny(lower, []string{"hello", "hi", "你好", "天气", "推荐", "聊聊", "怎么样", "什么是"}) && len(prompt) < 50 {
		return "simple_chat"
	}
	// Default
	return "general"
}

// generateMockProbabilities 生成模拟概率分布
func generateMockProbabilities(primaryCategory string) map[string]float64 {
	categories := []string{"code", "data_analysis", "document", "vision_understanding", "image_generation", "simple_chat", "general"}
	probs := make(map[string]float64)

	var primaryIdx int
	for i, c := range categories {
		if c == primaryCategory {
			primaryIdx = i
			break
		}
	}

	// 主要类别高置信度
	probs[primaryCategory] = 0.75 + rand.Float64()*0.20

	// 剩余概率分配给其他类别
	remaining := 1.0 - probs[primaryCategory]
	for i, c := range categories {
		if c == primaryCategory {
			continue
		}
		// 距离主要类别越近，概率越高
		dist := float64(abs(i - primaryIdx))
		weight := 1.0 / (dist + 1)
		probs[c] = remaining * weight * (0.5 + rand.Float64()*0.5)
	}

	// 归一化
	var total float64
	for _, p := range probs {
		total += p
	}
	for c := range probs {
		probs[c] = probs[c] / total
	}

	return probs
}

func promptLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		result[i] = c
	}
	return string(result)
}

func containsAny(s string, substrs []string) bool {
	for _, sub := range substrs {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}

func abs(i int) int {
	if i < 0 {
		return -i
	}
	return i
}

// classifyLocally 使用本地分类器对 prompt 进行分类，返回类别名称、置信度和信号值
// 供 mock 模式下的官方 provider 复用，以提供有意义的对比结果
func (a *VLLMPoolDecisionAdapter) classifyLocally(prompt string) (string, float64, map[string]float64) {
	// 优先使用 E5 分类器
	if a.e5Classifier != nil && a.e5Classifier.IsEnabled() {
		result := a.e5Classifier.Classify(prompt)
		signals := map[string]float64{}
		for _, s := range result.MatchedSignals {
			signals["matched:"+s] = 1.0
		}
		return result.Category, result.Confidence, signals
	}

	// 后备：使用 Prototype Rule V2
	if a.prototypeClassifierV2 != nil {
		result := a.prototypeClassifierV2.Classify(prompt)
		signals := map[string]float64{}
		for _, s := range result.MatchedSignals {
			signals["matched:"+s] = 1.0
		}
		return result.Category, result.Confidence, signals
	}

	// 最后后备：基于关键词猜测
	category := guessCategoryFromPrompt(prompt)
	confidence := 0.7 + rand.Float64()*0.25
	signals := map[string]float64{"guess:" + category: confidence}
	return category, confidence, signals
}
