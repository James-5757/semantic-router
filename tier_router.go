package semanticrouter

import (
	"math"
	"sort"
	"strings"
)

// ConfidenceThreshold 置信度阈值
var (
	HighConfidenceThreshold     = 0.75
	LowConfidenceThreshold      = 0.45
	FallbackConfidenceThreshold = 0.25
)

// TierRule Tier 路由规则
type TierRule struct {
	Name            string
	Priority        int
	Enabled         bool
	ModelMatches    []string
	TaskType        TaskType
	TaskTypeMatches []TaskType // 多个任务类型匹配
	PreferredTier   PreferredTier
	Reason          string
	Confidence      float64
	PromptContains  []string // prompt 包含关键词
}

// DecisionSource 决策来源
type DecisionSource string

const (
	DecisionSourceRule     DecisionSource = "rule"
	DecisionSourceSemantic DecisionSource = "semantic"
	DecisionSourceFallback DecisionSource = "fallback"
)

// MultiLayerDecision 多层路由决策结果
type MultiLayerDecision struct {
	PreferredPool        PreferredPool
	TaskType             TaskType
	Modality             Modality
	RequiredCapabilities RequiredCapabilities
	Confidence           float64
	DecisionSource       DecisionSource
	FallbackReason       string
	MatchedRules         []string
	SemanticScores       map[string]float64
	RuleScore            float64
	SemanticScore        float64
	SecondBestPool       PreferredPool // 第二选择的 pool
	SecondBestScore      float64       // 第二选择的得分
	ScoreMargin          float64       // 得分差距
}

// MultiLayerRouter 三层路由组合器
type MultiLayerRouter struct {
	ruleRouter       SemanticRouter
	similarityRouter SemanticSimilarityRouter
}

// NewMultiLayerRouter 创建多层路由组合器
func NewMultiLayerRouter() *MultiLayerRouter {
	return &MultiLayerRouter{
		ruleRouter:       NewRuleBasedSemanticRouter(),
		similarityRouter: NewTokenOverlapSimilarityRouter(),
	}
}

// NewMultiLayerRouterWithRouter 使用自定义路由器创建
func NewMultiLayerRouterWithRouter(ruleRouter SemanticRouter, similarityRouter SemanticSimilarityRouter) *MultiLayerRouter {
	return &MultiLayerRouter{
		ruleRouter:       ruleRouter,
		similarityRouter: similarityRouter,
	}
}

// Route 执行三层路由判断
func (r *MultiLayerRouter) Route(req *RouteRequest) *MultiLayerDecision {
	// 第一层：规则路由（高置信度）
	ruleResult, _ := r.ruleRouter.Route(nil, req)

	// 计算所有 pool 的语义得分
	allSemanticScores := r.calculateAllSemanticScores(req.Prompt)

	decision := &MultiLayerDecision{
		TaskType:             ruleResult.TaskType,
		Modality:             ruleResult.Modality,
		RequiredCapabilities: ruleResult.RequiredCapabilities,
		Confidence:           ruleResult.Confidence,
		MatchedRules:         []string{ruleResult.MatchedRule},
		RuleScore:            ruleResult.Confidence,
		SemanticScores:       allSemanticScores,
	}

	// 获取语义决策
	semanticResult := r.similarityRouter.Route(
		req.Prompt,
		ruleResult.TaskType,
		req.HasImage,
		req.HasDocument,
	)
	decision.SemanticScore = semanticResult.Confidence

	// 确定最终 pool
	var finalPool PreferredPool
	var finalConfidence float64
	var source DecisionSource
	var fallbackReason string

	rulePool := ruleResult.PreferredPool
	codeScore := allSemanticScores[string(PoolCode)]
	dataScore := allSemanticScores[string(PoolData)]
	visionScore := allSemanticScores[string(PoolVision)]
	docScore := allSemanticScores[string(PoolDocument)]
	imageGenerationScore := allSemanticScores[string(PoolImageGeneration)]
	cheapScore := allSemanticScores[string(PoolCheap)]
	imagePromptAuthoring := isImagePromptAuthoringRequest(req.Prompt)

	// 优先级检查
	// 1. image_by_images_field - 强制规则最高
	switch {
	case req.HasImage:
		finalPool = PoolVision
		finalConfidence = 0.9
		source = DecisionSourceRule
		decision.MatchedRules = []string{"image_by_images_field"}
	case req.HasDocument:
		finalPool = PoolDocument
		finalConfidence = 0.9
		source = DecisionSourceRule
		decision.MatchedRules = []string{"document_by_documents_field"}
	case imagePromptAuthoring:
		// The request discusses image generation, but the requested deliverable is
		// text (keywords, tags, or a prompt), not an image asset. Keep it on a
		// text-capable pool and do not require an image-generation account.
		finalPool = PoolCheap
		finalConfidence = 0.8
		source = DecisionSourceRule
		decision.MatchedRules = []string{"image_prompt_authoring_text_output"}
		decision.RequiredCapabilities = RequiredCapabilities{ImageCapability: ImageCapabilityNone}
	case (codeScore > 0.1 && codeScore >= dataScore+0.10 && codeScore >= docScore+0.10 && codeScore >= imageGenerationScore+0.10) || (codeScore > 0.05 && dataScore <= 0.05 && visionScore <= 0.05 && docScore <= 0.05 && imageGenerationScore <= 0.05):
		// Code requires meaningful evidence before it can outrank another
		// professional pool. Tiny description-token overlap must not turn a
		// contract review or image request into code.
		finalPool = PoolCode
		finalConfidence = math.Min(codeScore+0.3, 0.85)
		source = DecisionSourceSemantic
		decision.MatchedRules = []string{"code_intent"}
	case dataScore > 0.15:
		// data 高置信度优先于 image_generation
		finalPool = PoolData
		finalConfidence = math.Min(dataScore+0.3, 0.85)
		source = DecisionSourceSemantic
		decision.MatchedRules = []string{"data_intent"}
	case ruleResult.TaskType == TaskTypeImageGenerate || (imageGenerationScore >= 0.25 && codeScore <= 0.15 && dataScore <= 0.15):
		// image_generation 优先级高于 vision（防止 "生成图片" 落入 vision）
		// 提高阈值到 0.25，且 code/data 得分不能同时较高，避免编程请求误入
		finalPool = PoolImageGeneration
		finalConfidence = math.Min(math.Max(ruleResult.Confidence, imageGenerationScore+0.3), 0.9)
		source = DecisionSourceSemantic
		if ruleResult.TaskType == TaskTypeImageGenerate && ruleResult.MatchedRule != "" && ruleResult.MatchedRule != "default_text" {
			source = DecisionSourceRule
			decision.MatchedRules = []string{ruleResult.MatchedRule}
		} else {
			decision.MatchedRules = []string{"image_generation_intent"}
		}
	case dataScore > 0.08:
		// data 较低置信度
		finalPool = PoolData
		finalConfidence = math.Min(dataScore+0.3, 0.85)
		source = DecisionSourceSemantic
		decision.MatchedRules = []string{"data_intent"}
	case visionScore > 0.05:
		finalPool = PoolVision
		finalConfidence = math.Min(visionScore+0.3, 0.8)
		source = DecisionSourceSemantic
		decision.MatchedRules = []string{"vision_intent"}
	case docScore > 0.05:
		finalPool = PoolDocument
		finalConfidence = math.Min(docScore+0.5, 0.9)
		source = DecisionSourceSemantic
		decision.MatchedRules = []string{"document_intent"}
	case ruleResult.Confidence >= HighConfidenceThreshold && r.isForcedRule(ruleResult.MatchedRule):
		finalPool = rulePool
		finalConfidence = ruleResult.Confidence
		source = DecisionSourceRule
		decision.MatchedRules = []string{ruleResult.MatchedRule}
	case ruleResult.Confidence >= 0.9 && ruleResult.MatchedRule != "default_text":
		finalPool = rulePool
		finalConfidence = ruleResult.Confidence
		source = DecisionSourceRule
		decision.MatchedRules = []string{ruleResult.MatchedRule}
	case semanticResult.Confidence >= HighConfidenceThreshold && semanticResult.Pool != PoolDefault:
		finalPool = semanticResult.Pool
		finalConfidence = semanticResult.Confidence
		source = DecisionSourceSemantic
		decision.MatchedRules = []string{semanticResult.Reason}
	case cheapScore > 0:
		finalPool = PoolCheap
		finalConfidence = math.Min(cheapScore+0.3, 0.7)
		source = DecisionSourceSemantic
		decision.MatchedRules = []string{"simple_chat_intent"}
	case ruleResult.Confidence >= LowConfidenceThreshold && ruleResult.MatchedRule != "default_text":
		finalPool = rulePool
		finalConfidence = ruleResult.Confidence
		source = DecisionSourceRule
		decision.MatchedRules = []string{ruleResult.MatchedRule}
	default:
		finalPool = PoolDefault
		finalConfidence = 0.5
		source = DecisionSourceFallback
		fallbackReason = "no_rule_or_semantic_intent"
	}

	decision.PreferredPool = finalPool
	decision.Confidence = finalConfidence

	decision.SecondBestPool, decision.SecondBestScore = findSecondBestPoolScore(allSemanticScores, finalPool)
	decision.ScoreMargin = math.Abs(allSemanticScores[string(finalPool)] - decision.SecondBestScore)

	// 设置决策来源和 fallback 原因
	if source == DecisionSourceFallback {
		decision.DecisionSource = DecisionSourceFallback
		decision.FallbackReason = fallbackReason
	} else {
		decision.DecisionSource = source
		decision.FallbackReason = ""
	}

	if imagePromptAuthoring {
		decision.TaskType = TaskTypeText
	} else {
		decision.TaskType = taskTypeForPool(finalPool, decision.TaskType)
	}

	return decision
}

// isImagePromptAuthoringRequest distinguishes asking a model to write a
// text-to-image prompt from asking it to create the image itself. The former is
// a text deliverable and must not consume an image-generation account.
func isImagePromptAuthoringRequest(prompt string) bool {
	contract := detectOutputContract(prompt)
	return contract.Domain == "image_generation" && contract.Kind == OutputContractText
}

// isForcedRule 检查是否是强制路由规则
// 只有当匹配到强制规则（如 images 字段、documents 字段）时才使用规则决策
func (r *MultiLayerRouter) isForcedRule(ruleName string) bool {
	forcedRules := map[string]bool{
		"image_by_images_field":    true,
		"image_by_image_url":       true,
		"document_by_docx":         true,
		"document_by_doc":          true,
		"document_by_pdf":          true,
		"document_by_content_type": true,
	}
	return forcedRules[ruleName]
}

// CalculateSemanticScores 计算所有 pool 的语义得分
func (r *MultiLayerRouter) CalculateSemanticScores(prompt string) map[string]float64 {
	return r.calculateAllSemanticScores(prompt)
}

// calculateAllSemanticScores 计算所有 pool 的语义得分
func (r *MultiLayerRouter) calculateAllSemanticScores(prompt string) map[string]float64 {
	scores := make(map[string]float64)
	allPools := []PreferredPool{PoolDefault, PoolCheap, PoolCode, PoolVision, PoolDocument, PoolData, PoolPrivate, PoolImageGeneration}

	// 获取 similarity router 的内部方法直接计算每个 pool 的得分
	simRouter := r.similarityRouter.(*TokenOverlapSimilarityRouter)

	for _, p := range allPools {
		// 直接计算每个 pool 的关键词得分
		keywordScore, _ := simRouter.CalculateKeywordScore(prompt, p)
		descScore := simRouter.CalculateDescriptionSimilarity(prompt, p)
		// 综合得分：关键词权重 0.7，描述相似度权重 0.3
		// 不使用 Round，保留原始分数以便比较
		totalScore := keywordScore*0.7 + descScore*0.3
		scores[string(p)] = totalScore
	}
	// Add only compositional professional signals. These require a task pattern
	// (not a lone broad keyword) and prevent specialised requests from falling
	// through to general chat when no attachment metadata is available yet.
	for pool, score := range professionalBoundaryScores(prompt) {
		if score > scores[string(pool)] {
			scores[string(pool)] = score
		}
	}

	return scores
}

func professionalBoundaryScores(prompt string) map[PreferredPool]float64 {
	lower := strings.ToLower(prompt)
	hasAny := func(values ...string) bool {
		for _, value := range values {
			if strings.Contains(lower, strings.ToLower(value)) {
				return true
			}
		}
		return false
	}
	countAny := func(values ...string) int {
		count := 0
		for _, value := range values {
			if strings.Contains(lower, strings.ToLower(value)) {
				count++
			}
		}
		return count
	}
	scores := map[PreferredPool]float64{}

	// A competition name alone is not enough; planning at least two modelling
	// stages is a reliable data-science workflow signal.
	if hasAny("kaggle", "竞赛", "比赛") && countAny("baseline", "验证", "validation", "融合", "ensemble", "后处理", "调参", "hyperparameter") >= 2 {
		scores[PoolData] = 0.45
	}
	// A code-producing verb plus an implementation object is a professional
	// coding request. This remains deliberately compositional so a mention of
	// an "algorithm" in a general explanation does not force code routing.
	if hasAny("写一个", "写个", "编写", "实现", "写代码", "write a", "implement") && hasAny("算法", "程序", "代码", "函数", "脚本", "c语言", "python", "java", "golang", "algorithm", "program", "function", "script") {
		scores[PoolCode] = 0.45
	}
	// Creation verb plus a visual deliverable distinguishes image generation
	// from merely discussing pictures or charts.
	if hasAny("生成", "创作", "画一", "画一张", "generate", "create", "draw") && hasAny("海报", "插画", "图片", "图像", "poster", "illustration", "image") {
		scores[PoolImageGeneration] = 0.45
	}
	// Referencing a document object together with a document operation remains
	// document work even when the attachment is expected in a later turn.
	if hasAny("合同", "文档", "pdf", "word", "docx", "论文", "报告") && hasAny("总结", "审查", "分析", "提取", "翻译", "概括", "风险", "条款") {
		scores[PoolDocument] = 0.40
	}
	return scores
}

// findBestSemanticPool 找到语义得分最高的 pool（排除 default）
func findBestSemanticPool(scores map[string]float64) (PreferredPool, float64) {
	var bestPool PreferredPool = PoolDefault
	var bestScore float64 = 0

	for pool, score := range scores {
		// 跳过 default pool，优先选择具体的 pool
		if pool == string(PoolDefault) || pool == string(PoolCheap) {
			continue
		}
		if score > bestScore {
			bestScore = score
			bestPool = PreferredPool(pool)
		}
	}

	// 如果没有找到更好的 pool，才考虑 cheap
	if bestScore == 0 {
		if score, ok := scores[string(PoolCheap)]; ok && score > 0 {
			return PoolCheap, score
		}
	}

	return bestPool, bestScore
}

// findSecondBestPool 找到第二高的语义 pool
func findSecondBestPool(scores map[string]float64, bestPool PreferredPool) PreferredPool {
	pool, _ := findSecondBestPoolScore(scores, bestPool)
	return pool
}

func findSecondBestPoolScore(scores map[string]float64, finalPool PreferredPool) (PreferredPool, float64) {
	ranked := rankedPoolScores(scores)
	for _, candidate := range ranked {
		if candidate.pool != finalPool {
			return candidate.pool, candidate.score
		}
	}
	return PoolDefault, 0
}

type poolScore struct {
	pool  PreferredPool
	score float64
}

func rankedPoolScores(scores map[string]float64) []poolScore {
	ranked := make([]poolScore, 0, len(scores))
	for pool, score := range scores {
		ranked = append(ranked, poolScore{pool: PreferredPool(pool), score: score})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return poolRank(ranked[i].pool) < poolRank(ranked[j].pool)
		}
		return ranked[i].score > ranked[j].score
	})
	return ranked
}

func poolRank(pool PreferredPool) int {
	switch pool {
	case PoolVision:
		return 0
	case PoolDocument:
		return 1
	case PoolImageGeneration:
		return 2
	case PoolCode:
		return 3
	case PoolData:
		return 4
	case PoolCheap:
		return 5
	case PoolDefault:
		return 6
	case PoolPrivate:
		return 7
	default:
		return 99
	}
}

func taskTypeForPool(pool PreferredPool, fallback TaskType) TaskType {
	switch pool {
	case PoolCode:
		return TaskTypeCode
	case PoolVision:
		return TaskTypeVision
	case PoolDocument:
		return TaskTypeDocument
	case PoolImageGeneration:
		return TaskTypeImageGenerate
	default:
		return fallback
	}
}

// Round 四舍五入
func Round(x float64) float64 {
	return math.Round(x*100) / 100
}

// ===== 新架构集成 =====

// EnhancedMultiLayerRouter 增强的多层路由（整合 Task Understanding + Evidence Validation）
type EnhancedMultiLayerRouter struct {
	combinedRouter *CombinedRouter
	ruleRouter     SemanticRouter
}

// NewEnhancedMultiLayerRouter 创建增强的多层路由
func NewEnhancedMultiLayerRouter() *EnhancedMultiLayerRouter {
	return &EnhancedMultiLayerRouter{
		combinedRouter: NewCombinedRouter(),
		ruleRouter:     NewRuleBasedSemanticRouter(),
	}
}

// Route 执行增强路由
func (r *EnhancedMultiLayerRouter) Route(req *RouteRequest) *EnhancedRouteResult {
	// 使用新架构进行路由
	combinedResult := r.combinedRouter.Route(req.Prompt, req.HasImage, req.HasDocument, req.HasCSV)

	// 获取规则路由结果作为参考
	ruleResult, _ := r.ruleRouter.Route(nil, req)

	// 构建增强路由结果
	result := &EnhancedRouteResult{
		CombinedResult:    combinedResult,
		RuleResult:        ruleResult,
		FinalPool:         combinedResult.FinalPool,
		Confidence:        combinedResult.Confidence,
		TaskUnderstanding: combinedResult.TaskUnderstanding,
		DecisionSource:    combinedResult.DecisionSource,
	}

	// 如果规则路由有强制规则，覆盖结果
	if r.hasForcedRule(ruleResult) {
		result.FinalPool = ruleResult.PreferredPool
		result.DecisionSource = "rule_override"
	}

	return result
}

// hasForcedRule 检查是否有强制规则
func (r *EnhancedMultiLayerRouter) hasForcedRule(result *SemanticRouteDecision) bool {
	if result == nil {
		return false
	}
	forcedRules := map[string]bool{
		"image_by_images_field": true,
		"image_by_image_url":    true,
		"document_by_docx":      true,
		"document_by_pdf":       true,
	}
	return forcedRules[result.MatchedRule]
}

// EnhancedRouteResult 增强路由结果
type EnhancedRouteResult struct {
	CombinedResult    *CombinedRouteResult
	RuleResult        *SemanticRouteDecision
	FinalPool         PreferredPool
	Confidence        float64
	TaskUnderstanding *TaskSchema
	DecisionSource    string
}

// GetExplanatoryOutput 获取可解释性输出
func (r *EnhancedRouteResult) GetExplanatoryOutput() *ExplanatoryOutput {
	output := &ExplanatoryOutput{
		TaskUnderstanding: &TaskUnderstandingOutput{
			PrimaryIntent:    r.TaskUnderstanding.PrimaryIntent,
			SecondaryIntents: r.TaskUnderstanding.SecondaryIntents,
			Actions:          r.TaskUnderstanding.Actions,
			Objects:          r.TaskUnderstanding.Objects,
			InputModalities:  r.TaskUnderstanding.InputModalities,
			OutputArtifacts:  r.TaskUnderstanding.OutputArtifacts,
			Confidence:       r.TaskUnderstanding.Confidence,
			Ambiguous:        r.TaskUnderstanding.Ambiguous,
			MissingInputs:    r.TaskUnderstanding.MissingInputs,
		},
		CandidatePools: []CandidatePoolOutput{},
		RejectedPools:  []RejectedPoolOutput{},
		FinalPool:      string(r.FinalPool),
		Confidence:     r.Confidence,
		DecisionSource: r.DecisionSource,
		EmbeddingUsed:  false, // 原型评分不计入 embedding
	}

	// 填充候选池信息
	if r.CombinedResult.CandidateResult != nil {
		for _, c := range r.CombinedResult.CandidateResult.Candidates {
			candidate := CandidatePoolOutput{
				Pool:      string(c.Pool),
				Score:     c.CandidateScore,
				Evidence:  []string{},
				Validated: c.Validated,
			}
			for _, e := range c.SupportingEvidence {
				candidate.Evidence = append(candidate.Evidence, string(e))
			}
			output.CandidatePools = append(output.CandidatePools, candidate)

			// 收集被拒绝的池
			if !c.Validated && c.RejectionReason != "" {
				output.RejectedPools = append(output.RejectedPools, RejectedPoolOutput{
					Pool:            string(c.Pool),
					RejectionReason: c.RejectionReason,
				})
			}
		}
	}

	// Fallback 信息
	if r.CombinedResult.FallbackDecision != nil && r.CombinedResult.FallbackDecision.ShouldFallback {
		output.FallbackInfo = &FallbackInfoOutput{
			FallbackPool:    string(r.CombinedResult.FallbackDecision.FallbackPool),
			FallbackReason:  r.CombinedResult.FallbackDecision.FallbackReason,
			ConflictDetails: r.CombinedResult.FallbackDecision.ConflictDetails,
		}
	}

	return output
}

// ExplanatoryOutput 可解释性输出
type ExplanatoryOutput struct {
	TaskUnderstanding *TaskUnderstandingOutput `json:"task_understanding"`
	CandidatePools    []CandidatePoolOutput    `json:"candidate_pools"`
	RejectedPools     []RejectedPoolOutput     `json:"rejected_pools"`
	FinalPool         string                   `json:"final_pool"`
	Confidence        float64                  `json:"confidence"`
	DecisionSource    string                   `json:"decision_source"`
	EmbeddingUsed     bool                     `json:"embedding_used"`
	FallbackInfo      *FallbackInfoOutput      `json:"fallback_info,omitempty"`
}

// TaskUnderstandingOutput Task Understanding 输出
type TaskUnderstandingOutput struct {
	PrimaryIntent    string   `json:"primary_intent"`
	SecondaryIntents []string `json:"secondary_intents"`
	Actions          []string `json:"actions"`
	Objects          []string `json:"objects"`
	InputModalities  []string `json:"input_modalities"`
	OutputArtifacts  []string `json:"output_artifacts"`
	Confidence       float64  `json:"confidence"`
	Ambiguous        bool     `json:"ambiguous"`
	MissingInputs    []string `json:"missing_inputs"`
}

// CandidatePoolOutput 候选池输出
type CandidatePoolOutput struct {
	Pool      string   `json:"pool"`
	Score     float64  `json:"score"`
	Evidence  []string `json:"evidence"`
	Validated bool     `json:"validated"`
}

// RejectedPoolOutput 被拒绝的池输出
type RejectedPoolOutput struct {
	Pool            string `json:"pool"`
	RejectionReason string `json:"rejection_reason"`
}

// FallbackInfoOutput Fallback 信息输出
type FallbackInfoOutput struct {
	FallbackPool    string   `json:"fallback_pool"`
	FallbackReason  string   `json:"fallback_reason"`
	ConflictDetails []string `json:"conflict_details"`
}

// RuleBasedTierRouter 基于规则的 Tier 路由
type RuleBasedTierRouter struct {
	rules []TierRule
}

// NewRuleBasedTierRouter 创建基于规则的 Tier 路由
func NewRuleBasedTierRouter() *RuleBasedTierRouter {
	router := &RuleBasedTierRouter{
		rules: []TierRule{
			// 强模型规则 - 只对特定任务类型使用强模型，不是所有 gpt-4 请求都强
			{
				Name:            "strong_model_gpt4",
				Priority:        100,
				Enabled:         true,
				ModelMatches:    []string{"gpt-4", "gpt4", "gemini-2"},
				TaskTypeMatches: []TaskType{TaskTypeVision, TaskTypeDocument}, // 只有视觉/文档任务才用强模型
				PreferredTier:   TierStrong,
				Reason:          "GPT-4 系列强模型（视觉/文档任务）",
				Confidence:      0.9,
			},
			{
				Name:            "strong_model_claude3",
				Priority:        100,
				Enabled:         true,
				ModelMatches:    []string{"claude-3-opus", "claude-3-sonnet", "claude-sonnet", "claude-opus"},
				TaskTypeMatches: []TaskType{TaskTypeVision, TaskTypeDocument},
				PreferredTier:   TierStrong,
				Reason:          "Claude 3 系列强模型（视觉/文档任务）",
				Confidence:      0.9,
			},
			{
				Name:            "strong_model_gpt4o",
				Priority:        100,
				Enabled:         true,
				ModelMatches:    []string{"gpt-4o", "gpt-4-vision", "gpt4o"},
				TaskTypeMatches: []TaskType{TaskTypeVision},
				PreferredTier:   TierStrong,
				Reason:          "GPT-4o 强模型（视觉任务）",
				Confidence:      0.9,
			},
			// 弱模型规则 - simple QA / translation / rewrite
			{
				Name:           "weak_simple_chat",
				Priority:       95,
				Enabled:        true,
				PromptContains: []string{"hello", "hi", "你好", "今天天气", "几号", "是什么", "怎么样", "推荐", "解释"},
				PreferredTier:  TierWeak,
				Reason:         "简单问答用弱模型",
				Confidence:     0.8,
			},
			// 弱模型规则 - 只匹配真正弱的模型
			{
				Name:          "weak_model_davinci",
				Priority:      90,
				Enabled:       true,
				ModelMatches:  []string{"davinci", "babbage", "ada", "curie"},
				PreferredTier: TierWeak,
				Reason:        "Davinci 系列弱模型",
				Confidence:    0.9,
			},
			// 中等模型规则 - gpt-3.5-turbo 是中等强度
			{
				Name:          "medium_model_turbo",
				Priority:      92,
				Enabled:       true,
				ModelMatches:  []string{"gpt-3.5-turbo", "gpt35turbo", "gpt-3.5"},
				PreferredTier: TierMedium,
				Reason:        "GPT-3.5 Turbo 是中等模型",
				Confidence:    0.85,
			},
			// 单独处理 haiku（避免匹配到 claude-haiku）
			{
				Name:          "weak_model_haiku",
				Priority:      95,
				Enabled:       true,
				ModelMatches:  []string{"haiku"},
				PreferredTier: TierWeak,
				Reason:        "Haiku 弱模型",
				Confidence:    0.95,
			},
			// 任务类型规则 - 视觉任务需要强模型
			{
				Name:          "vision_task",
				Priority:      95,
				Enabled:       true,
				TaskType:      TaskTypeVision,
				PreferredTier: TierStrong,
				Reason:        "视觉任务需要强模型",
				Confidence:    0.9,
			},
			// 复杂任务需要强模型 - 扩展更多特征
			{
				Name:     "strong_complex_task",
				Priority: 88,
				Enabled:  true,
				PromptContains: []string{
					"架构", "架构设计", "微服务", "distributed", "architecture",
					"安全审查", "风险", "风险评估", "安全分析",
					"内存泄漏", "内存", "泄漏", "性能分析", "性能优化",
					"长文档", "复杂", "多步骤", "推理",
					"系统设计", "设计模式", "分布式系统",
					"security review", "risk assessment", "memory leak",
					"performance optimization", "system design",
					"architecture design", "multi-step", "reasoning",
					"漏洞挖掘", "渗透测试", "代码审计",
				},
				PreferredTier: TierStrong,
				Reason:        "复杂任务需要强模型",
				Confidence:    0.85,
			},
			// 代码任务默认中等强度
			{
				Name:          "code_task",
				Priority:      85,
				Enabled:       true,
				TaskType:      TaskTypeCode,
				PreferredTier: TierMedium,
				Reason:        "代码任务中等强度即可",
				Confidence:    0.8,
			},
			{
				Name:          "document_task",
				Priority:      85,
				Enabled:       true,
				TaskType:      TaskTypeDocument,
				PreferredTier: TierMedium,
				Reason:        "文档任务中等强度",
				Confidence:    0.8,
			},
		},
	}
	return router
}

// Route 执行 Tier 路由
// 按规则优先级检查：model匹配 > taskType匹配 > prompt匹配
// 如果均未匹配，使用默认 medium
func (r *RuleBasedTierRouter) Route(ctx interface{}, model string, taskType TaskType) (*TierRouteDecision, error) {
	return r.RouteWithPrompt(ctx, model, taskType, "")
}

// RouteWithPrompt 执行 Tier 路由（含 prompt 关键词匹配）
func (r *RuleBasedTierRouter) RouteWithPrompt(ctx interface{}, model string, taskType TaskType, prompt string) (*TierRouteDecision, error) {
	// 按优先级排序
	rules := r.rules
	for i := 0; i < len(rules); i++ {
		for j := i + 1; j < len(rules); j++ {
			if rules[j].Priority > rules[i].Priority {
				rules[i], rules[j] = rules[j], rules[i]
			}
		}
	}

	modelLower := strings.ToLower(model)

	// 检查 prompt 长度作为辅助判断
	promptLen := len([]rune(prompt))

	// 按优先级遍历所有规则
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}

		// 1. 检查任务类型匹配（单个）
		if rule.TaskType != "" && rule.TaskType == taskType {
			return &TierRouteDecision{
				PreferredTier: rule.PreferredTier,
				Confidence:    rule.Confidence,
				MatchedRule:   rule.Name,
				Reason:        rule.Reason,
			}, nil
		}

		// 2. 检查任务类型匹配（多个）+ 模型匹配
		if len(rule.TaskTypeMatches) > 0 {
			matchedTask := false
			for _, tt := range rule.TaskTypeMatches {
				if tt == taskType {
					matchedTask = true
					break
				}
			}
			if matchedTask && len(rule.ModelMatches) > 0 {
				for _, m := range rule.ModelMatches {
					if strings.Contains(modelLower, strings.ToLower(m)) {
						return &TierRouteDecision{
							PreferredTier: rule.PreferredTier,
							Confidence:    rule.Confidence,
							MatchedRule:   rule.Name,
							Reason:        rule.Reason,
						}, nil
					}
				}
			}
		}

		// 3. 检查模型匹配
		if len(rule.ModelMatches) > 0 && len(rule.TaskTypeMatches) == 0 {
			for _, m := range rule.ModelMatches {
				if strings.Contains(modelLower, strings.ToLower(m)) {
					return &TierRouteDecision{
						PreferredTier: rule.PreferredTier,
						Confidence:    rule.Confidence,
						MatchedRule:   rule.Name,
						Reason:        rule.Reason,
					}, nil
				}
			}
		}

		// 4. 最后检查 PromptContains 匹配（低优先级，仅在 model/task 都不匹配时使用）
		if len(rule.PromptContains) > 0 && promptLen > 0 {
			// 防止误匹配：如果是 weak 规则但 prompt 较长（>30字），跳过
			if rule.PreferredTier == TierWeak && promptLen > 30 {
				continue
			}
			// 防止误匹配：如果是 strong 规则但 prompt 很短（<5字），跳过
			if rule.PreferredTier == TierStrong && promptLen < 5 {
				continue
			}
			promptLower := strings.ToLower(prompt)
			for _, kw := range rule.PromptContains {
				if strings.Contains(promptLower, strings.ToLower(kw)) {
					return &TierRouteDecision{
						PreferredTier: rule.PreferredTier,
						Confidence:    rule.Confidence,
						MatchedRule:   rule.Name,
						Reason:        rule.Reason,
					}, nil
				}
			}
		}
	}

	// 默认返回中等强度
	return &TierRouteDecision{
		PreferredTier: TierMedium,
		Confidence:    0.5,
		MatchedRule:   "default",
		Reason:        "默认中等模型",
	}, nil
}

// GetName 获取路由名称
func (r *RuleBasedTierRouter) GetName() string {
	return "RuleBasedTierRouter"
}
