package vllm_pool_client

import (
	"sort"
	"strings"
)

// =============================================================================
// vLLM Semantic Router v0.3.0 API Types
// 基于官方源码: src/semantic-router/pkg/services/classification_signal_types.go
// =============================================================================

// =============================================================================
// Request Types
// =============================================================================

// VLLMIntentRequest intent classification 请求
// 对应官方 services.IntentRequest
type VLLMIntentRequest struct {
	Text     string        `json:"text"`
	Messages []VLLMMessage `json:"messages,omitempty"`
	Options  *VLLMOptions  `json:"options,omitempty"`
}

// VLLMMessage 消息结构
type VLLMMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// VLLMOptions intent classification 选项
type VLLMOptions struct {
	ReturnProbabilities bool    `json:"return_probabilities,omitempty"`
	ConfidenceThreshold float64 `json:"confidence_threshold,omitempty"`
	IncludeExplanation  bool    `json:"include_explanation,omitempty"`
	EvaluateAllSignals  bool    `json:"evaluate_all_signals,omitempty"`
	Trace               bool    `json:"trace,omitempty"`
}

// =============================================================================
// Response Types
// =============================================================================

// VLLMIntentResponse intent classification 响应
// 对应官方 services.IntentResponse
type VLLMIntentResponse struct {
	Classification   VLLMClassification      `json:"classification"`
	Probabilities    map[string]float64     `json:"probabilities,omitempty"`
	RecommendedModel string                 `json:"recommended_model,omitempty"`
	RoutingDecision  string                 `json:"routing_decision,omitempty"`
	MatchedSignals   *VLLmmatchedSignals    `json:"matched_signals,omitempty"`
	DecisionResult   *VLLMDecisionResult    `json:"decision_result,omitempty"`
}

// VLLMClassification 分类结果
// 对应官方 services.Classification
type VLLMClassification struct {
	Category         string  `json:"category"`
	Confidence       float64 `json:"confidence"`
	ProcessingTimeMs int64   `json:"processing_time_ms"`
}

// VLLmmatchedSignals 匹配的信号
type VLLmmatchedSignals struct {
	Keywords     []string `json:"keywords,omitempty"`
	Embeddings   []string `json:"embeddings,omitempty"`
	Domains      []string `json:"domains,omitempty"`
	FactCheck    []string `json:"fact_check,omitempty"`
	UserFeedback []string `json:"user_feedback,omitempty"`
	Reask        []string `json:"reask,omitempty"`
	Preferences  []string `json:"preferences,omitempty"`
	Language     []string `json:"language,omitempty"`
	Context      []string `json:"context,omitempty"`
	Structure    []string `json:"structure,omitempty"`
	Complexity   []string `json:"complexity,omitempty"`
	Modality     []string `json:"modality,omitempty"`
	Authz        []string `json:"authz,omitempty"`
	Jailbreak    []string `json:"jailbreak,omitempty"`
}

// VLLMDecisionResult 决策结果
type VLLMDecisionResult struct {
	Decision  string                 `json:"decision"`
	Category  string                 `json:"category"`
	Score     float64                `json:"score"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// =============================================================================
// Semantic Category 定义 (7个)
// =============================================================================

// SemanticCategory 语义分类标签
type SemanticCategory string

const (
	CategoryCode              SemanticCategory = "code"
	CategoryDataAnalysis      SemanticCategory = "data_analysis"
	CategoryDocument          SemanticCategory = "document"
	CategoryVisionUnderstanding SemanticCategory = "vision_understanding"
	CategoryImageGeneration   SemanticCategory = "image_generation"
	CategorySimpleChat        SemanticCategory = "simple_chat"
	CategoryGeneral           SemanticCategory = "general"
)

// IsValid 检查是否为有效分类
func (c SemanticCategory) IsValid() bool {
	switch c {
	case CategoryCode, CategoryDataAnalysis, CategoryDocument,
		CategoryVisionUnderstanding, CategoryImageGeneration,
		CategorySimpleChat, CategoryGeneral:
		return true
	}
	return false
}

// =============================================================================
// Capability 定义
// =============================================================================

// Capability 模型能力需求
type Capability string

const (
	// Text capabilities
	CapGeneralText           Capability = "general_text"
	CapRecommendation        Capability = "recommendation"
	CapGeneralReasoning      Capability = "general_reasoning"
	CapDocumentUnderstanding Capability = "document_understanding"
	CapLongContext           Capability = "long_context"

	// Code/Data capabilities
	CapCodeGeneration    Capability = "code_generation"
	CapCodeUnderstanding Capability = "code_understanding"
	CapDataAnalysis      Capability = "data_analysis"
	CapPython            Capability = "python"
	CapStructuredData    Capability = "structured_data"
	CapChartGeneration   Capability = "chart_generation"

	// Vision capabilities
	CapVisionInput             Capability = "vision_input"
	CapMultimodalUnderstanding Capability = "multimodal_understanding"

	// Image generation capabilities
	CapImageOutput      Capability = "image_output"
	CapCreativeGeneration Capability = "creative_generation"
)

// =============================================================================
// Execution Family 定义 (4个 - 合并后)
// =============================================================================

// ExecutionFamily 执行族
type ExecutionFamily string

const (
	FamilyTextPool            ExecutionFamily = "text"
	FamilyCodeData            ExecutionFamily = "code_data"
	FamilyVision              ExecutionFamily = "vision"
	FamilyImageGeneration     ExecutionFamily = "image_generation"
)

// String 返回 string 类型的执行族名称
func (f ExecutionFamily) String() string {
	return string(f)
}

// =============================================================================
// Category → Capabilities 映射
// =============================================================================

// CategoryToCapabilities 每个分类对应的能力需求
var CategoryToCapabilities = map[SemanticCategory][]Capability{
	CategoryCode: {
		CapCodeGeneration, CapCodeUnderstanding,
	},
	CategoryDataAnalysis: {
		CapDataAnalysis, CapPython, CapStructuredData, CapChartGeneration,
	},
	CategoryDocument: {
		CapDocumentUnderstanding, CapLongContext,
	},
	CategoryVisionUnderstanding: {
		CapVisionInput, CapMultimodalUnderstanding,
	},
	CategoryImageGeneration: {
		CapImageOutput, CapCreativeGeneration,
	},
	CategorySimpleChat: {
		CapGeneralText, CapRecommendation,
	},
	CategoryGeneral: {
		CapGeneralReasoning, CapGeneralText,
	},
}

// GetCapabilities 获取分类对应的能力需求
func GetCapabilities(category SemanticCategory) []Capability {
	if caps, ok := CategoryToCapabilities[category]; ok {
		return caps
	}
	return []Capability{CapGeneralText} // 默认返回通用文本能力
}

// =============================================================================
// Legacy Pool 映射 (7个原始 Pool)
// =============================================================================

// LegacyPool 传统池名称
type LegacyPool string

const (
	LegacyPoolCodePool           LegacyPool = "code_pool"
	LegacyPoolDataPool           LegacyPool = "data_pool"
	LegacyPoolVisionPool         LegacyPool = "vision_pool"
	LegacyPoolDocumentPool       LegacyPool = "document_pool"
	LegacyPoolImageGenerationPool LegacyPool = "image_generation_pool"
	LegacyPoolCheapChatPool      LegacyPool = "cheap_chat_pool"
	LegacyPoolGeneralPool        LegacyPool = "general_pool"
)

// String 返回 string 类型的池名称
func (p LegacyPool) String() string {
	return string(p)
}

// CategoryToLegacyPool 分类到传统池的映射
var CategoryToLegacyPool = map[SemanticCategory]LegacyPool{
	CategoryCode:              LegacyPoolCodePool,
	CategoryDataAnalysis:      LegacyPoolDataPool,
	CategoryDocument:          LegacyPoolDocumentPool,
	CategoryVisionUnderstanding: LegacyPoolVisionPool,
	CategoryImageGeneration:   LegacyPoolImageGenerationPool,
	CategorySimpleChat:        LegacyPoolCheapChatPool,
	CategoryGeneral:           LegacyPoolGeneralPool,
}

// GetLegacyPool 获取传统池
func GetLegacyPool(category SemanticCategory) LegacyPool {
	if pool, ok := CategoryToLegacyPool[category]; ok {
		return pool
	}
	return LegacyPoolGeneralPool // 默认
}

// =============================================================================
// Execution Family 映射 (4个)
// =============================================================================

// CategoryToExecutionFamily 分类到执行族的映射
var CategoryToExecutionFamily = map[SemanticCategory]ExecutionFamily{
	CategoryCode:              FamilyCodeData,
	CategoryDataAnalysis:      FamilyCodeData,
	CategoryDocument:          FamilyTextPool,
	CategoryVisionUnderstanding: FamilyVision,
	CategoryImageGeneration:   FamilyImageGeneration,
	CategorySimpleChat:        FamilyTextPool,
	CategoryGeneral:           FamilyTextPool,
}

// GetExecutionFamily 获取执行族
func GetExecutionFamily(category SemanticCategory) ExecutionFamily {
	if family, ok := CategoryToExecutionFamily[category]; ok {
		return family
	}
	return FamilyTextPool // 默认
}

// =============================================================================
// VLLMSemanticShadowResult Shadow 结果结构
// =============================================================================

// VLLMSemanticShadowResult vLLM Semantic Shadow 结果 (四层结构)
type VLLMSemanticShadowResult struct {
	// Classification Method
	ClassificationMethod string `json:"classification_method"` // "prototype_rule_v2" | "e5_prototype_semantic" | "vllm_api" | "mock"

	// Layer 1: Raw Semantic Category (来自 vLLM)
	RawCategory    string  `json:"raw_category"`
	RawConfidence  float64 `json:"raw_confidence"`
	RawProbabilities map[string]float64 `json:"raw_probabilities,omitempty"`

	// Legacy Mapped Pool (传统池)
	LegacyPool LegacyPool `json:"legacy_pool"`

	// Layer 2: Required Capabilities
	RequiredCapabilities []Capability `json:"required_capabilities"`

	// Layer 3: Proposed Execution Family
	ExecutionFamily ExecutionFamily `json:"execution_family"`

	// Local comparison (用于评估)
	LocalPool          LegacyPool     `json:"local_pool"`
	LegacyPoolAgreement bool          `json:"legacy_pool_agreement"`
	FamilyAgreement    bool           `json:"execution_family_agreement"`

	// Classification metadata
	Ambiguous        bool     `json:"ambiguous"`
	MatchedSignals   []string `json:"matched_signals"`

	// Final decision (永远使用本地)
	UsedForFinal   bool   `json:"used_for_final"` // 永远 false
	FinalPool      string `json:"final_pool"`     // 永远等于 local_pool
	DecisionSource string `json:"decision_source"`

	// Service status
	ServiceAvailable bool          `json:"service_available"`
	ServiceReady     bool          `json:"service_ready"`
	Invoked          bool          `json:"invoked"`
	TopK             []CategoryScore `json:"top_k,omitempty"`
	Error            string  `json:"error,omitempty"`
	LatencyMs        float64 `json:"latency_ms"`
}

// AdaptResponse 解析 vLLM 响应并适配为 Shadow Result
func (r *VLLMIntentResponse) AdaptResponse(localPool string) *VLLMSemanticShadowResult {
	if r.Classification.Category == "" {
		return &VLLMSemanticShadowResult{
			UsedForFinal:   false,
			Invoked:        false,
			Error:          "empty response from vLLM",
			ServiceAvailable: false,
		}
	}

	category := strings.ToLower(r.Classification.Category)
	semanticCategory := SemanticCategory(category)
	if !semanticCategory.IsValid() {
		semanticCategory = fuzzyMatchCategory(category)
	}

	// 获取三层映射
	legacyPool := GetLegacyPool(semanticCategory)
	capabilities := GetCapabilities(semanticCategory)
	executionFamily := GetExecutionFamily(semanticCategory)

	// 比较
	legacyPoolAgree := string(legacyPool) == localPool

	// 执行族比较
	localFamily := poolToFamily(localPool)
	familyAgree := executionFamily == localFamily

	// 构建 Top-K
	topK := convertProbabilitiesToTopK(r.Probabilities)

	return &VLLMSemanticShadowResult{
		RawCategory:         string(semanticCategory),
		RawConfidence:       r.Classification.Confidence,
		RawProbabilities:    r.Probabilities,
		LegacyPool:          legacyPool,
		RequiredCapabilities: capabilities,
		ExecutionFamily:     executionFamily,
		LocalPool:           LegacyPool(localPool),
		LegacyPoolAgreement: legacyPoolAgree,
		FamilyAgreement:     familyAgree,
		UsedForFinal:        false,
		FinalPool:           localPool,
		DecisionSource:      "local_baseline",
		ServiceAvailable:    true,
		Invoked:             true,
		TopK:                topK,
	}
}

// poolToFamily 将本地池转换为执行族
func poolToFamily(pool string) ExecutionFamily {
	switch pool {
	case "code_pool", "data_pool":
		return FamilyCodeData
	case "vision_pool":
		return FamilyVision
	case "image_generation_pool":
		return FamilyImageGeneration
	default:
		return FamilyTextPool
	}
}

// fuzzyMatchCategory 模糊匹配分类
func fuzzyMatchCategory(category string) SemanticCategory {
	lower := strings.ToLower(category)

	if strings.Contains(lower, "code") || strings.Contains(lower, "program") {
		return CategoryCode
	}
	if strings.Contains(lower, "data") || strings.Contains(lower, "analysis") || strings.Contains(lower, "chart") {
		return CategoryDataAnalysis
	}
	if strings.Contains(lower, "doc") || strings.Contains(lower, "pdf") || strings.Contains(lower, "summarize") {
		return CategoryDocument
	}
	if strings.Contains(lower, "vision") || strings.Contains(lower, "image") && strings.Contains(lower, "understand") {
		return CategoryVisionUnderstanding
	}
	if strings.Contains(lower, "image") || strings.Contains(lower, "generat") || strings.Contains(lower, "draw") {
		return CategoryImageGeneration
	}
	if strings.Contains(lower, "simple") || strings.Contains(lower, "chat") || strings.Contains(lower, "basic") {
		return CategorySimpleChat
	}
	return CategoryGeneral
}

// convertProbabilitiesToTopK 将 probabilities 转换为 Top-K
func convertProbabilitiesToTopK(probs map[string]float64) []CategoryScore {
	if len(probs) == 0 {
		return nil
	}

	var topK []CategoryScore
	for category, score := range probs {
		topK = append(topK, CategoryScore{
			Category: category,
			Score:    score,
		})
	}
	// 排序
	sort.Slice(topK, func(i, j int) bool {
		return topK[i].Score > topK[j].Score
	})
	// 限制 Top-7
	if len(topK) > 7 {
		topK = topK[:7]
	}
	return topK
}

// CategoryScore 分类分数
type CategoryScore struct {
	Category string  `json:"category"`
	Score    float64 `json:"score"`
}

// TopK 字段 (用于 JSON 输出)
func (r *VLLMSemanticShadowResult) GetTopK() []CategoryScore {
	if r.RawProbabilities != nil {
		return convertProbabilitiesToTopK(r.RawProbabilities)
	}
	return nil
}