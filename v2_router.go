package semanticrouter

import (
	"math"
	"strings"
	"time"

	"semantic-router/vllm_pool_client"
)

// ============================================================================
// V2 Type Definitions
// ============================================================================

// V2OutputMode 输出形式
type V2OutputMode string

const (
	V2OutputModeText  V2OutputMode = "text"
	V2OutputModeImage V2OutputMode = "image"
	V2OutputModeVideo V2OutputMode = "video"
)

// V2Domain 领域
type V2Domain string

const (
	V2DomainTechnical V2Domain = "technical"
	V2DomainGeneral   V2Domain = "general"
	V2DomainUnknown   V2Domain = "unknown"
)

// V2PhysicalGroup V2 物理组
type V2PhysicalGroup string

const (
	V2GroupTextGeneral         V2PhysicalGroup = "text_general_group"
	V2GroupTextTechnical       V2PhysicalGroup = "text_technical_group"
	V2GroupMultimodalGeneral   V2PhysicalGroup = "multimodal_general_group"
	V2GroupMultimodalTechnical V2PhysicalGroup = "multimodal_technical_group"
	V2GroupImageGeneration     V2PhysicalGroup = "image_generation_group"
	V2GroupVideoGeneration     V2PhysicalGroup = "video_generation_group"
)

// V2ToolProfile 工具画像
type V2ToolProfile string

const (
	V2ToolNone              V2ToolProfile = "none"
	V2ToolCodeTools         V2ToolProfile = "code_tools"
	V2ToolDataTools         V2ToolProfile = "data_tools"
	V2ToolDocumentTools     V2ToolProfile = "document_tools"
	V2ToolWebSearch         V2ToolProfile = "web_search"
	V2ToolOCRTools          V2ToolProfile = "ocr_tools"
	V2ToolChartAnalysisTools V2ToolProfile = "chart_analysis_tools" // 图表/图文融合分析
)

// V2MultimodalType 多模态任务类型细分
type V2MultimodalType string

const (
	V2MultimodalNone              V2MultimodalType = "none"
	V2MultimodalVisionQA          V2MultimodalType = "vision_qa"           // 视觉识别与问答
	V2MultimodalChartAnalysis     V2MultimodalType = "chart_analysis"      // 图文融合: 图表分析
	V2MultimodalMemeAnalysis      V2MultimodalType = "meme_analysis"       // 图文融合: 梗图/讽刺图理解
	V2MultimodalDocumentParse     V2MultimodalType = "document_parse"      // 文档智能解析
	V2MultimodalImageGeneration   V2MultimodalType = "image_generation"    // 文生图
	V2MultimodalImageEditing      V2MultimodalType = "image_editing"       // 图生文/图片编辑
	V2MultimodalCrossModalRetrieval V2MultimodalType = "cross_modal_retrieval" // 跨模态检索
)

// V2TaskUnderstandingLite Task Understanding Lite 输出
type V2TaskUnderstandingLite struct {
	OutputMode         V2OutputMode      `json:"output_mode"`
	RequiresMultimodal bool              `json:"requires_multimodal"`
	MultimodalType     V2MultimodalType  `json:"multimodal_type,omitempty"` // 多模态类型细分
	Domain             V2Domain          `json:"domain"`
	ToolProfile        V2ToolProfile     `json:"tool_profile"`
	FineIntent         string            `json:"fine_intent,omitempty"`
}

// V2CapabilityWindow Capability Window / Capability Gate 输出
type V2CapabilityWindow struct {
	HasImage             bool              `json:"has_image"`
	HasDocument          bool              `json:"has_document"`
	HasCSV               bool              `json:"has_csv"`
	RequiresMultimodal   bool              `json:"requires_multimodal"`
	MultimodalType       V2MultimodalType  `json:"multimodal_type,omitempty"` // 与 tu 同步
	DetectedModalities   []string          `json:"detected_modalities"`
	ImageGeneration      bool              `json:"image_generation"`
	VideoGeneration      bool              `json:"video_generation"`
	MultimodalByMetadata bool              `json:"multimodal_by_metadata"`
	// 图文融合细化
	ChartAnalysis        bool              `json:"chart_analysis,omitempty"`         // 图表分析
	MemeAnalysis         bool              `json:"meme_analysis,omitempty"`          // 梗图分析
	TextImageFusion      bool              `json:"text_image_fusion,omitempty"`      // 图文融合综合
}

// V2LocalResult Local V2 路由结果
type V2LocalResult struct {
	Domain     V2Domain        `json:"domain"`
	Group      V2PhysicalGroup `json:"group"`
	Confidence float64         `json:"confidence"`
	Reason     string          `json:"reason"`
	RuleType   string          `json:"rule_type"` // "keyword", "embedding", "rule", "fallback"
}

// V2OfficialResult Official V2 路由结果
type V2OfficialResult struct {
	Domain       V2Domain           `json:"domain"`
	DomainScore  float64            `json:"domain_score"`
	SecondDomain V2Domain           `json:"second_domain"`
	DomainMargin float64            `json:"domain_margin"`
	FineIntent   string             `json:"fine_intent"`
	ToolProfile  V2ToolProfile      `json:"tool_profile"`
	Group        V2PhysicalGroup    `json:"group"`
	Available    bool               `json:"available"`
	E5Scores     map[string]float64 `json:"e5_scores,omitempty"` // 真实 E5 embedding 分数
	// OfficialScores are the scores returned by the official vLLM /eval shadow.
	// They are displayed for comparison only and never affect V1 routing.
	OfficialScores   map[string]float64 `json:"official_scores,omitempty"`
	TechnicalScore   float64            `json:"technical_score,omitempty"`
	GeneralScore     float64            `json:"general_score,omitempty"`
	OfficialDecision string             `json:"official_decision,omitempty"`
	ScoreSource      string             `json:"score_source,omitempty"`
}

// V2HybridResult Hybrid V2 结果
type V2HybridResult struct {
	Triggered      bool            `json:"triggered"`
	Disagreement   bool            `json:"disagreement"`
	CandidateGroup V2PhysicalGroup `json:"candidate_group"`
	UsedForFinal   bool            `json:"used_for_final"` // always false
}

// V2ToolProfileResult 最终工具画像
type V2ToolProfileResult struct {
	Primary   V2ToolProfile   `json:"primary"`
	Secondary []V2ToolProfile `json:"secondary,omitempty"`
	Reason    string          `json:"reason"`
}

// V2TierResult Tier 路由结果（V2）
type V2TierResult struct {
	ComplexityCandidateTier PreferredTier `json:"complexity_candidate_tier"`
	RouteLLMCandidateTier   PreferredTier `json:"routellm_candidate_tier"`
	PolicyMinimumTier       PreferredTier `json:"policy_minimum_tier"`
	SelectedTier            PreferredTier `json:"selected_tier"`
	SelectedTierSource      string        `json:"selected_tier_source"`
	TierAgreement           bool          `json:"tier_agreement"`
}

// V2Decision V2 完整决策
type V2Decision struct {
	TaskUnderstanding V2TaskUnderstandingLite `json:"task_understanding"`
	CapabilityWindow  V2CapabilityWindow      `json:"capability_window"`
	Local             V2LocalResult           `json:"local"`
	Official          V2OfficialResult        `json:"official"`
	Hybrid            V2HybridResult          `json:"hybrid"`
	ToolProfile       V2ToolProfileResult     `json:"tool_profile"`
	Tier              V2TierResult            `json:"tier"`
}

// ============================================================================
// V2 Physical Group Helpers
// ============================================================================

// V2GroupFromDomainAndModality 从 domain 和 modality 确定 V2 Physical Group
func V2GroupFromDomainAndModality(domain V2Domain, requiresMultimodal bool, outputMode V2OutputMode) V2PhysicalGroup {
	if outputMode == V2OutputModeImage {
		return V2GroupImageGeneration
	}
	if outputMode == V2OutputModeVideo {
		return V2GroupVideoGeneration
	}

	if domain == V2DomainUnknown {
		domain = V2DomainGeneral
	}

	if requiresMultimodal {
		if domain == V2DomainTechnical {
			return V2GroupMultimodalTechnical
		}
		return V2GroupMultimodalGeneral
	}

	if domain == V2DomainTechnical {
		return V2GroupTextTechnical
	}
	return V2GroupTextGeneral
}

// LegacyPoolToV2Intent 将旧 pool 映射到 V2 fine_intent
func LegacyPoolToV2Intent(pool string) string {
	switch pool {
	case "code_pool":
		return "code_generation"
	case "data_pool":
		return "data_analysis"
	case "document_pool":
		return "document_processing"
	case "vision_pool":
		return "image_understanding"
	case "image_generation_pool":
		return "image_generation"
	case "cheap_chat_pool":
		return "simple_chat"
	default:
		return "general_chat"
	}
}

// LegacyPoolToToolProfile 将旧 pool 映射到 V2 tool_profile
func LegacyPoolToToolProfile(pool string) V2ToolProfile {
	switch pool {
	case "code_pool":
		return V2ToolCodeTools
	case "data_pool":
		return V2ToolDataTools
	case "document_pool":
		return V2ToolDocumentTools
	case "vision_pool":
		return V2ToolOCRTools
	default:
		return V2ToolNone
	}
}

// ============================================================================
// V2 Router Implementation
// ============================================================================

// V2Router implements the V2 routing pipeline
type V2Router struct {
	multilayer          *MultiLayerRouter
	multilayerV2        *MultiLayerRouter // 独立的 V2 多层路由器实例（不共享 V1 状态）
	e5Classifier        *vllm_pool_client.E5PrototypeSemanticClassifier
	prototypeClassifier *vllm_pool_client.PrototypeClassifierV2 // 本地关键词分类器
}

// NewV2Router creates a new V2 router
func NewV2Router(multilayer *MultiLayerRouter) *V2Router {
	r := &V2Router{
		multilayer:   multilayer,
		multilayerV2: NewMultiLayerRouter(),
	}
	// Initialize E5 classifier for authentic embedding scores
	config := vllm_pool_client.E5ClassifierConfig{
		Enabled:             true,
		EmbeddingURL:        "http://localhost:8899",
		Timeout:             10 * time.Second,
		TopKPerCategory:     3,
		SimilarityThreshold: 0.3,
		AmbiguousMargin:     0.05,
		// 留空让分类器自己从默认路径查找
	}
	e5c, err := vllm_pool_client.NewE5PrototypeSemanticClassifier(config)
	if err == nil {
		r.e5Classifier = e5c
	}

	// Initialize local prototype classifier (keyword-based, always available)
	pc, err := vllm_pool_client.NewPrototypeClassifierV2("vllm_pool_client/prototypes.json")
	if err == nil {
		r.prototypeClassifier = pc
	}
	return r
}

// SetE5Classifier sets the E5 classifier (used to share from adapter)
func (r *V2Router) SetE5Classifier(c *vllm_pool_client.E5PrototypeSemanticClassifier) {
	r.e5Classifier = c
}

// SetPrototypeClassifier sets the prototype classifier (used to share from adapter)
func (r *V2Router) SetPrototypeClassifier(c *vllm_pool_client.PrototypeClassifierV2) {
	r.prototypeClassifier = c
}

// Route executes the full V2 pipeline
func (r *V2Router) Route(req *RouteRequest) *V2Decision {
	return r.routeWithScores(req, nil)
}

// RouteWithScores executes the full V2 pipeline with optional pre-computed E5 scores
// scores: map[string]float64 with keys "code", "data_analysis", "document", "vision_understanding", "image_generation", "simple_chat", "general"
func (r *V2Router) RouteWithScores(req *RouteRequest, e5Scores map[string]float64) *V2Decision {
	return r.routeWithScores(req, e5Scores)
}

func (r *V2Router) routeWithScores(req *RouteRequest, e5Scores map[string]float64) *V2Decision {
	decision := &V2Decision{}

	// If no external E5 scores provided, try to compute from local E5 classifier
	scores := e5Scores
	if len(scores) == 0 && r.e5Classifier != nil {
		classifyResult := r.e5Classifier.Classify(req.Prompt)
		if classifyResult != nil && len(classifyResult.Scores) > 0 {
			// Normalize E5 classifier category keys to pool-like keys for compatibility
			raw := classifyResult.Scores
			scores = make(map[string]float64, len(raw))
			for k, v := range raw {
				// Map category names to compatible keys
				switch k {
				case "code":
					scores["code_pool"] = v
				case "data_analysis":
					scores["data_pool"] = v
				case "document":
					scores["document_pool"] = v
				case "vision_understanding":
					scores["vision_pool"] = v
				case "image_generation":
					scores["image_generation_pool"] = v
				case "simple_chat":
					scores["cheap_chat_pool"] = v
				case "general":
					scores["general_pool"] = v
				default:
					scores[k] = v
				}
			}
		}
	}
	// Fallback to prototype classifier (keyword-based, always available locally)
	if len(scores) == 0 && r.prototypeClassifier != nil {
		classifyResult := r.prototypeClassifier.Classify(req.Prompt)
		if classifyResult != nil && len(classifyResult.Scores) > 0 {
			raw := classifyResult.Scores
			scores = make(map[string]float64, len(raw))
			for k, v := range raw {
				switch k {
				case "code":
					scores["code_pool"] = v
				case "data_analysis":
					scores["data_pool"] = v
				case "document":
					scores["document_pool"] = v
				case "vision_understanding":
					scores["vision_pool"] = v
				case "image_generation":
					scores["image_generation_pool"] = v
				case "simple_chat":
					scores["cheap_chat_pool"] = v
				case "general":
					scores["general_pool"] = v
				default:
					scores[k] = v
				}
			}
		}
	}
	// If still no scores, fall back to keyword-based multilayer
	if len(scores) == 0 {
		mlDecision := r.multilayerV2.Route(req)
		scores = mlDecision.SemanticScores
	}

	// 1. Task Understanding Lite
	tu := r.analyzeTaskUnderstandingLite(req)
	decision.TaskUnderstanding = tu

	// 2. Capability Window
	cw := r.analyzeCapabilityWindow(req, tu)
	decision.CapabilityWindow = cw

	// 3. Local V2
	local := r.localV2Route(req, tu, cw, scores)
	decision.Local = local

	// 4. Official V2 (shadow)
	official := r.officialV2Route(req, tu, cw, scores)
	decision.Official = official

	// 5. Hybrid V2
	hybrid := r.hybridV2(local, official)
	decision.Hybrid = hybrid

	// 6. Tool Profile
	tp := r.selectToolProfile(req, tu, local, official, scores)
	decision.ToolProfile = tp

	// 7. Tier
	tier := r.determineTier(req, scores)
	decision.Tier = tier

	return decision
}

// ============================================================================
// Task Understanding Lite
// ============================================================================

func (r *V2Router) analyzeTaskUnderstandingLite(req *RouteRequest) V2TaskUnderstandingLite {
	result := V2TaskUnderstandingLite{
		OutputMode:         V2OutputModeText,
		RequiresMultimodal: false,
		MultimodalType:     V2MultimodalNone,
		Domain:             V2DomainGeneral,
		ToolProfile:        V2ToolNone,
	}

	prompt := strings.ToLower(req.Prompt)

	// Output mode: image
	if hasImageGenerationKeywords(prompt) {
		result.OutputMode = V2OutputModeImage
	}
	// Output mode: video (simple heuristic)
	if hasVideoGenerationKeywords(prompt) {
		result.OutputMode = V2OutputModeVideo
	}

	// Requires multimodal: must understand attached images/scans
	if req.HasImage {
		result.RequiresMultimodal = true
		result.MultimodalType = V2MultimodalVisionQA // default
	}
	// CSV, JSON, code files are NOT multimodal
	if req.HasCSV {
		result.RequiresMultimodal = false
		result.MultimodalType = V2MultimodalNone
	}
	// Has document with images/scans -> multimodal
	if req.HasDocument && (containsAny(prompt, []string{"扫描件", "扫描", "图片", "image", "scan", "screenshot", "截图"}) ||
		containsAny(prompt, []string{"识别", "提取", "ocr", "describe", "分析图片", "理解图片"})) {
		result.RequiresMultimodal = true
		result.MultimodalType = V2MultimodalDocumentParse
	}
	// Pure text document
	if req.HasDocument && !result.RequiresMultimodal {
		result.RequiresMultimodal = false
		result.MultimodalType = V2MultimodalNone
	}

	// 图文融合检测: 图表分析
	if result.RequiresMultimodal && hasChartAnalysisKeywords(prompt) {
		result.MultimodalType = V2MultimodalChartAnalysis
	}
	// 图文融合检测: 梗图分析
	if result.RequiresMultimodal && hasMemeAnalysisKeywords(prompt) {
		result.MultimodalType = V2MultimodalMemeAnalysis
	}
	// 图文融合检测: 通用图文融合（若有图片但未匹配上述类型）
	if result.RequiresMultimodal && result.MultimodalType == V2MultimodalVisionQA && hasTextImageFusionKeywords(prompt) {
		result.MultimodalType = V2MultimodalChartAnalysis // 统一归为图文融合
	}

	// 图片生成/编辑
	if result.OutputMode == V2OutputModeImage && req.HasImage {
		result.MultimodalType = V2MultimodalImageEditing
	} else if result.OutputMode == V2OutputModeImage {
		result.MultimodalType = V2MultimodalImageGeneration
	}

	// Domain: technical
	if hasTechnicalKeywords(prompt) {
		result.Domain = V2DomainTechnical
	}
	// Image/video generation is always "general" domain by default
	if result.OutputMode == V2OutputModeImage || result.OutputMode == V2OutputModeVideo {
		if hasTechnicalKeywords(prompt) {
			result.Domain = V2DomainTechnical
		}
	}

	// Tool Profile
	if hasCodeKeywords(prompt) {
		result.ToolProfile = V2ToolCodeTools
	} else if hasDataKeywords(prompt) {
		result.ToolProfile = V2ToolDataTools
	} else if hasDocumentKeywords(prompt) {
		result.ToolProfile = V2ToolDocumentTools
	} else if hasOCROrImageKeywords(prompt) {
		result.ToolProfile = V2ToolOCRTools
	}

	return result
}

// ============================================================================
// Capability Window
// ============================================================================

func (r *V2Router) analyzeCapabilityWindow(req *RouteRequest, tu V2TaskUnderstandingLite) V2CapabilityWindow {
	cw := V2CapabilityWindow{
		HasImage:    req.HasImage,
		HasDocument: req.HasDocument,
		HasCSV:      req.HasCSV,
	}

	cw.RequiresMultimodal = tu.RequiresMultimodal
	cw.MultimodalType = tu.MultimodalType
	cw.ImageGeneration = (tu.OutputMode == V2OutputModeImage)
	cw.VideoGeneration = (tu.OutputMode == V2OutputModeVideo)

	// 图文融合细化
	cw.ChartAnalysis = (tu.MultimodalType == V2MultimodalChartAnalysis)
	cw.MemeAnalysis = (tu.MultimodalType == V2MultimodalMemeAnalysis)
	cw.TextImageFusion = cw.ChartAnalysis || cw.MemeAnalysis

	// Modalities
	cw.DetectedModalities = []string{"text"}
	if req.HasImage && tu.RequiresMultimodal {
		cw.DetectedModalities = append(cw.DetectedModalities, "image")
	}
	if req.HasDocument && tu.RequiresMultimodal {
		cw.DetectedModalities = append(cw.DetectedModalities, "document")
	}

	// Multimodal by metadata
	cw.MultimodalByMetadata = (req.HasImage || (req.HasDocument && tu.RequiresMultimodal))

	return cw
}

// ============================================================================
// Local V2 Route
// ============================================================================

func (r *V2Router) localV2Route(req *RouteRequest, tu V2TaskUnderstandingLite, cw V2CapabilityWindow, e5Scores map[string]float64) V2LocalResult {
	result := V2LocalResult{
		Domain:     tu.Domain,
		Confidence: 0.7,
		RuleType:   "keyword",
		Reason:     "Task Understanding Lite",
	}

	// Get E5 scores if available, otherwise fall back to keyword-based scores
	var scores map[string]float64
	if len(e5Scores) > 0 {
		scores = e5Scores
	} else {
		mlDecision := r.multilayerV2.Route(req)
		scores = mlDecision.SemanticScores
	}

	// Domain decision
	if cw.ImageGeneration || cw.VideoGeneration {
		// Image/video generation: domain from task understanding
		if tu.Domain == V2DomainTechnical {
			result.Domain = V2DomainTechnical
			result.Reason = "image/video generation with technical context"
		} else {
			result.Domain = V2DomainGeneral
			result.Reason = "image/video generation"
		}
	} else if req.HasImage && cw.RequiresMultimodal {
		// Multimodal
		codeScore := scores[string(PoolCode)]
		dataScore := scores[string(PoolData)]
		docScore := scores[string(PoolDocument)]

		// Check if multimodal task is technical
		techScore := math.Max(codeScore, math.Max(dataScore, docScore))
		if techScore > 0.15 || tu.Domain == V2DomainTechnical {
			result.Domain = V2DomainTechnical
			result.Reason = "multimodal technical task"
		} else {
			result.Domain = V2DomainGeneral
			result.Reason = "multimodal general task"
		}
		result.RuleType = "rule"
	} else if req.HasCSV {
		// CSV data is always text + technical
		result.Domain = V2DomainTechnical
		result.Reason = "CSV data analysis"
		result.RuleType = "rule"
	} else {
		// Text-only: use semantic scores
		codeScore := scores[string(PoolCode)]
		dataScore := scores[string(PoolData)]
		docScore := scores[string(PoolDocument)]
		generalScore := scores[string(PoolDefault)]

		techScore := math.Max(codeScore, math.Max(dataScore, docScore))
		if techScore > generalScore+0.05 && techScore > 0.1 {
			result.Domain = V2DomainTechnical
			result.Reason = "code/data/document intent detected"
			result.RuleType = "semantic"
		} else if tu.Domain == V2DomainTechnical {
			result.Domain = V2DomainTechnical
			result.Reason = "technical domain from task understanding"
		} else {
			result.Domain = V2DomainGeneral
			result.Reason = "general text task"
			result.RuleType = "fallback"
		}
	}

	// Use semantic scores to derive confidence
	maxScore := 0.0
	for _, v := range scores {
		if v > maxScore {
			maxScore = v
		}
	}
	result.Confidence = maxScore

	// Group
	result.Group = V2GroupFromDomainAndModality(result.Domain, cw.RequiresMultimodal, tu.OutputMode)

	return result
}

// ============================================================================
// Official V2 Route (Shadow)
// ============================================================================

func (r *V2Router) officialV2Route(req *RouteRequest, tu V2TaskUnderstandingLite, cw V2CapabilityWindow, e5Scores map[string]float64) V2OfficialResult {
	result := V2OfficialResult{
		Available: false,
	}

	// Get E5 scores if available, otherwise fall back to keyword-based scores
	var scores map[string]float64
	if len(e5Scores) > 0 {
		scores = e5Scores
		result.Available = true
	} else {
		mlDecision := r.multilayerV2.Route(req)
		scores = mlDecision.SemanticScores
		result.Available = true
	}

	// Copy all scores for display
	result.E5Scores = scores

	codeScore := scores[string(PoolCode)]
	dataScore := scores[string(PoolData)]
	docScore := scores[string(PoolDocument)]
	generalScore := scores[string(PoolDefault)]
	visionScore := scores[string(PoolVision)]
	imageGenScore := scores[string(PoolImageGeneration)]

	// If all scores are zero, try to get from the official scores (enriched later)
	// This handles the case where E5 classifier's embedding service is unavailable
	// but official VLLM shadow returned scores through enrichOfficialV2Scores.
	if codeScore <= 0 && dataScore <= 0 && docScore <= 0 && generalScore <= 0 &&
		visionScore <= 0 && imageGenScore <= 0 {
		// Use multilayer scores as fallback (they have _pool suffix keys)
		mlDecision := r.multilayerV2.Route(req)
		scores = mlDecision.SemanticScores
		result.E5Scores = scores
		codeScore = scores[string(PoolCode)]
		dataScore = scores[string(PoolData)]
		docScore = scores[string(PoolDocument)]
		generalScore = scores[string(PoolDefault)]
		visionScore = scores[string(PoolVision)]
		imageGenScore = scores[string(PoolImageGeneration)]
	}

	techScore := math.Max(codeScore, math.Max(dataScore, docScore))

	// Domain decision
	if cw.ImageGeneration {
		result.Domain = V2DomainGeneral
		result.DomainScore = imageGenScore
		result.SecondDomain = V2DomainTechnical
		result.DomainMargin = 0.2
		result.FineIntent = "image_generation"
		result.ToolProfile = V2ToolNone
	} else if cw.VideoGeneration {
		result.Domain = V2DomainGeneral
		result.DomainScore = 0.7
		result.FineIntent = "video_generation"
	} else if visionScore > 0.1 && req.HasImage {
		// Vision with image -> multimodal
		if techScore > generalScore {
			result.Domain = V2DomainTechnical
		} else {
			result.Domain = V2DomainGeneral
		}
		result.DomainScore = visionScore
		result.SecondDomain = V2DomainGeneral
		result.DomainMargin = math.Abs(visionScore - generalScore)
		result.FineIntent = "image_understanding"
		result.ToolProfile = V2ToolOCRTools
	} else if techScore > generalScore+0.05 && techScore > 0.1 {
		result.Domain = V2DomainTechnical
		result.DomainScore = techScore
		result.SecondDomain = V2DomainGeneral
		result.DomainMargin = techScore - generalScore
		// Fine intent from max score
		if codeScore >= dataScore && codeScore >= docScore {
			result.FineIntent = "code_generation"
			result.ToolProfile = V2ToolCodeTools
		} else if dataScore >= docScore {
			result.FineIntent = "data_analysis"
			result.ToolProfile = V2ToolDataTools
		} else {
			result.FineIntent = "document_processing"
			result.ToolProfile = V2ToolDocumentTools
		}
	} else {
		result.Domain = V2DomainGeneral
		result.DomainScore = generalScore
		if docScore > 0.05 {
			result.SecondDomain = V2DomainTechnical
			result.DomainMargin = math.Abs(generalScore - docScore)
			result.FineIntent = "document_processing"
		} else {
			result.SecondDomain = V2DomainUnknown
			result.DomainMargin = 0.1
			result.FineIntent = "general_chat"
		}
		result.ToolProfile = V2ToolNone
	}

	// Group
	result.Group = V2GroupFromDomainAndModality(result.Domain, cw.RequiresMultimodal, tu.OutputMode)

	// Official vLLM Scores for display
	{
		generalChatScore := generalScore
		simpleChatScore := scores[string(PoolCheap)]
		if generalChatScore < simpleChatScore {
			generalChatScore = simpleChatScore
		}
		result.TechnicalScore = techScore
		result.GeneralScore = generalChatScore

		bestScore := 0.0
		bestPool := "general"
		for _, candidate := range []struct {
			pool  string
			score float64
		}{
			{string(PoolCode), codeScore},
			{string(PoolData), dataScore},
			{string(PoolDocument), docScore},
			{string(PoolVision), visionScore},
			{string(PoolImageGeneration), imageGenScore},
			{string(PoolCheap), simpleChatScore},
			{string(PoolDefault), generalScore},
		} {
			if candidate.score > bestScore {
				bestScore = candidate.score
				bestPool = candidate.pool
			}
		}
		result.OfficialDecision = "route_" + bestPool
		result.ScoreSource = "official_vllm_eval"
	}

	return result
}

// ============================================================================
// Hybrid V2
// ============================================================================

func (r *V2Router) hybridV2(local V2LocalResult, official V2OfficialResult) V2HybridResult {
	result := V2HybridResult{
		UsedForFinal: false,
	}

	if !official.Available {
		result.Triggered = false
		result.Disagreement = false
		result.CandidateGroup = local.Group
		return result
	}

	if local.Group == official.Group {
		result.Triggered = false
		result.Disagreement = false
		result.CandidateGroup = local.Group
		return result
	}

	// Disagreement: use pairwise arbiter
	result.Triggered = true
	result.Disagreement = true

	// Simple arbiter: prefer local (since official is shadow)
	if local.Confidence >= 0.7 {
		result.CandidateGroup = local.Group
	} else {
		// Use official if local confidence is low
		result.CandidateGroup = official.Group
	}

	return result
}

// ============================================================================
// Tool Profile
// ============================================================================

func (r *V2Router) selectToolProfile(req *RouteRequest, tu V2TaskUnderstandingLite, local V2LocalResult, official V2OfficialResult, e5Scores map[string]float64) V2ToolProfileResult {
	result := V2ToolProfileResult{
		Primary: tu.ToolProfile,
	}

	// 图文融合任务优先
	if tu.RequiresMultimodal && tu.MultimodalType == V2MultimodalChartAnalysis {
		result.Primary = V2ToolChartAnalysisTools
		result.Reason = "chart/graph analysis with vision + text fusion"
		return result
	}
	if tu.RequiresMultimodal && tu.MultimodalType == V2MultimodalMemeAnalysis {
		result.Primary = V2ToolChartAnalysisTools
		result.Reason = "meme/comic understanding with text-image fusion"
		return result
	}
	if tu.RequiresMultimodal && tu.MultimodalType == V2MultimodalDocumentParse {
		result.Primary = V2ToolOCRTools
		result.Reason = "document parsing with layout understanding"
		return result
	}

	if result.Primary != V2ToolNone {
		result.Reason = "task understanding tool profile"
		return result
	}

	// Fall back to local/official
	if local.Domain == V2DomainTechnical {
		var codeScore, dataScore, docScore float64
		if len(e5Scores) > 0 {
			codeScore = e5Scores["code"]
			dataScore = e5Scores["data_analysis"]
			docScore = e5Scores["document"]
		} else {
			mlDecision := r.multilayerV2.Route(req)
			s := mlDecision.SemanticScores
			codeScore = s[string(PoolCode)]
			dataScore = s[string(PoolData)]
			docScore = s[string(PoolDocument)]
		}

		if codeScore >= dataScore && codeScore >= docScore {
			result.Primary = V2ToolCodeTools
			result.Reason = "code intent detected"
		} else if dataScore >= docScore {
			result.Primary = V2ToolDataTools
			result.Reason = "data intent detected"
		} else {
			result.Primary = V2ToolDocumentTools
			result.Reason = "document intent detected"
		}
	} else {
		result.Primary = V2ToolNone
		result.Reason = "general task, no tools needed"
	}

	return result
}

// ============================================================================
// Tier
// ============================================================================

func (r *V2Router) determineTier(req *RouteRequest, e5Scores map[string]float64) V2TierResult {
	result := V2TierResult{}

	// Complexity-based tier
	codeScore := r.multilayer.CalculateSemanticScores(req.Prompt)
	score := codeScore[string(PoolCode)]

	complexityScore := 0.5
	if score > 0.3 {
		complexityScore = 0.7
	}
	if hasComplexKeywords(req.Prompt) {
		complexityScore = 0.8
	}

	if complexityScore >= 0.7 {
		result.ComplexityCandidateTier = TierMedium
	} else {
		result.ComplexityCandidateTier = TierWeak
	}

	// RouteLLM candidate (shadow)
	result.RouteLLMCandidateTier = TierWeak

	// Policy minimum
	result.PolicyMinimumTier = TierWeak

	// Selected tier
	result.SelectedTier = result.ComplexityCandidateTier
	result.SelectedTierSource = "complexity"

	// Agreement
	result.TierAgreement = (result.ComplexityCandidateTier == result.RouteLLMCandidateTier)

	return result
}

// ============================================================================
// Keyword Helper Functions
// ============================================================================

func containsAny(s string, keywords []string) bool {
	sLower := strings.ToLower(s)
	for _, kw := range keywords {
		if strings.Contains(sLower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

func hasImageGenerationKeywords(prompt string) bool {
	keywords := []string{
		"生成图片", "生成图像", "生成一张图", "画一幅画", "画一张图",
		"创作图片", "创建图片", "帮我画", "帮我生成图片", "帮我生成一张",
		"generate image", "generate a picture", "create image", "draw",
		"make a picture", "generate an image", "创作一幅",
		"生成logo", "设计海报", "生成海报", "画一个",
		"生成一张", "文生图", "图片生成",
	}
	return containsAny(prompt, keywords)
}

func hasVideoGenerationKeywords(prompt string) bool {
	keywords := []string{
		"生成视频", "生成一段视频", "create a video", "generate a video",
		"make a video", "视频生成", "帮我生成视频",
	}
	return containsAny(prompt, keywords)
}

func hasTechnicalKeywords(prompt string) bool {
	keywords := []string{
		"代码", "函数", "算法", "编程", "debug", "调试", "sql", "query",
		"api", "接口", "数据库", "服务器", "部署", "docker", "k8s",
		"python", "javascript", "golang", "java", "rust", "typescript",
		"数据结构", "设计模式", "微服务", "架构",
		"分析数据", "数据分析", "统计", "可视化", "chart", "数据清洗",
		"机器学习", "深度学习", "神经网络", "model training",
		"数据报表", "excel", "csv",
	}
	return containsAny(prompt, keywords)
}

func hasCodeKeywords(prompt string) bool {
	keywords := []string{
		"写代码", "写一个函数", "实现", "编程", "代码", "开发",
		"write code", "write a function", "implement", "program",
		"debug", "重构", "refactor", "test", "测试",
		"api", "接口", "后端", "前端", "脚本",
	}
	return containsAny(prompt, keywords)
}

func hasDataKeywords(prompt string) bool {
	keywords := []string{
		"分析数据", "数据分析", "数据清洗", "数据可视化",
		"分析一下", "趋势", "统计", "图表", "报表",
		"analyze", "analysis", "statistics", "visualization",
		"excel", "csv", "数据", "表格",
	}
	return containsAny(prompt, keywords)
}

func hasDocumentKeywords(prompt string) bool {
	keywords := []string{
		"总结", "摘要", "提取", "文档", "报告", "论文",
		"summarize", "summary", "document", "contract", "合同",
		"润色", "改写", "翻译", "写作",
	}
	return containsAny(prompt, keywords)
}

func hasOCROrImageKeywords(prompt string) bool {
	keywords := []string{
		"识别", "提取文字", "ocr", "描述图片", "图片中",
		"这张图", "照片里", "分析图片", "理解图片",
		"describe", "identify", "what is in this image",
	}
	return containsAny(prompt, keywords)
}

func hasComplexKeywords(prompt string) bool {
	keywords := []string{
		"架构", "设计模式", "分布式", "微服务", "系统设计",
		"安全审查", "性能优化", "多步骤",
		"security review", "architecture", "performance optimization",
	}
	return containsAny(prompt, keywords)
}

// 图文融合: 图表分析检测
func hasChartAnalysisKeywords(prompt string) bool {
	keywords := []string{
		"图表", "表格", "折线图", "柱状图", "饼图", "趋势图",
		"数据分析图", "财务报表", "数据可视化", "走势图",
		"chart", "graph", "plot", "figure", "diagram", "pie chart",
		"bar chart", "line chart", "flow chart", "dashboard",
		"这个季度", "利润下降", "销售增长", "同比增长",
		"how does", "trend", "correlation", "compare",
	}
	return containsAny(prompt, keywords)
}

// 图文融合: 梗图/讽刺图检测
func hasMemeAnalysisKeywords(prompt string) bool {
	keywords := []string{
		"梗图", "表情包", "讽刺图", "漫画",
		"meme", "comic", "cartoon", "satire",
		"这张图什么意思", "这个梗", "笑点在哪",
		"explain the joke", "what does this meme mean",
		"双重讽刺", "文字和图片",
	}
	return containsAny(prompt, keywords)
}

// 图文融合: 通用图文融合检测（同时涉及文字和图像理解）
func hasTextImageFusionKeywords(prompt string) bool {
	keywords := []string{
		"分析这个", "解释这张图", "这张图说明了什么",
		"结合图片", "图文结合", "图中的文字",
		"图片里的", "从这张图", "根据图片",
		"照片上的", "菜单", "路牌", "告示牌",
		"sign", "menu", "label", "instruction",
		"translate this", "read this", "what does this say",
	}
	return containsAny(prompt, keywords)
}
