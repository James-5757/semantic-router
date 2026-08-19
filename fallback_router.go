package semanticrouter

import (
	"math"
)

// FallbackConfig Fallback 配置
type FallbackConfig struct {
	// 置信度低阈值
	LowConfidenceThreshold float64
	// Top1/Top2 差距小阈值
	SmallMarginThreshold float64
	// 冲突检测阈值
	ConflictThreshold float64
	// 是否启用安全模式（无法理解时默认 cheap）
	SafeMode bool
}

// DefaultFallbackConfig 默认配置
var DefaultFallbackConfig = FallbackConfig{
	LowConfidenceThreshold: 0.4,
	SmallMarginThreshold:   0.05,
	ConflictThreshold:      0.1,
	SafeMode:               true,
}

// FallbackDecision Fallback 决策结果
type FallbackDecision struct {
	// 是否需要 fallback
	ShouldFallback bool `json:"should_fallback"`
	// Fallback 目标池
	FallbackPool PreferredPool `json:"fallback_pool"`
	// Fallback 原因
	FallbackReason string `json:"fallback_reason"`
	// 冲突详情
	ConflictDetails []string `json:"conflict_details"`
	// 歧义标志
	Ambiguous bool `json:"ambiguous"`
	// 原始决策（如果被覆盖）
	OriginalPool PreferredPool `json:"original_pool,omitempty"`
	// 是否使用了 embedding
	EmbeddingUsed bool `json:"embedding_used"`
}

// FallbackRouter Fallback 路由器
type FallbackRouter struct {
	config *FallbackConfig
}

// NewFallbackRouter 创建 Fallback 路由器
func NewFallbackRouter() *FallbackRouter {
	return &FallbackRouter{
		config: &DefaultFallbackConfig,
	}
}

// NewFallbackRouterWithConfig 使用自定义配置创建
func NewFallbackRouterWithConfig(config FallbackConfig) *FallbackRouter {
	return &FallbackRouter{
		config: &config,
	}
}

// EvaluateFallback 评估是否需要 fallback
// 当：
// - Task Understanding 置信度低
// - 候选 Pool 都缺少必要证据
// - Top1/Top2 差距过小
// - Baseline、Embedding 与 Task Understanding 严重冲突
func (r *FallbackRouter) EvaluateFallback(
	taskUnderstanding *TaskSchema,
	baselineScores map[string]float64,
	embeddingScores map[string]float64,
	evidenceValidationResult *CandidatePoolResult,
) *FallbackDecision {

	decision := &FallbackDecision{
		ShouldFallback: false,
	}

	conflictDetails := []string{}

	// 条件1：Task Understanding 置信度低
	if taskUnderstanding.Confidence < r.config.LowConfidenceThreshold {
		conflictDetails = append(conflictDetails, "low_task_understanding_confidence")
		decision.ShouldFallback = true
		decision.Ambiguous = true
	}

	// 条件2：候选 Pool 都缺少必要证据
	if evidenceValidationResult != nil {
		allFailed := true
		for _, c := range evidenceValidationResult.Candidates {
			if c.Validated {
				allFailed = false
				break
			}
		}
		if allFailed && len(evidenceValidationResult.Candidates) > 0 {
			conflictDetails = append(conflictDetails, "all_candidates_validation_failed")
			decision.ShouldFallback = true
			decision.Ambiguous = true
		}
	}

	// 条件3：Top1/Top2 差距过小
	margin := r.calculateTopMargin(baselineScores)
	if margin < r.config.SmallMarginThreshold && taskUnderstanding.Ambiguous {
		conflictDetails = append(conflictDetails, "top_candidates_margin_too_small")
		decision.ShouldFallback = true
		decision.Ambiguous = true
	}

	// 条件4：Baseline、Embedding 与 Task Understanding 严重冲突
	if len(baselineScores) > 0 && len(embeddingScores) > 0 {
		conflict := r.detectConflict(taskUnderstanding, baselineScores, embeddingScores)
		if conflict.detected {
			conflictDetails = append(conflictDetails, conflict.details...)
			decision.ShouldFallback = true
			decision.Ambiguous = true
		}
	}

	decision.ConflictDetails = conflictDetails

	// 如果不需要 fallback，直接返回
	if !decision.ShouldFallback {
		return decision
	}

	// 确定 fallback 目标池
	decision.FallbackPool = r.determineFallbackPool(taskUnderstanding, evidenceValidationResult)
	decision.FallbackReason = r.determineFallbackReason(conflictDetails)

	return decision
}

// calculateTopMargin 计算 Top1/Top2 差距
func (r *FallbackRouter) calculateTopMargin(scores map[string]float64) float64 {
	if len(scores) < 2 {
		return 1.0 // 只有一个候选，差距视为大
	}

	// 找到最大的两个分数
	var max1, max2 float64 = -1, -1
	for _, score := range scores {
		if score > max1 {
			max2 = max1
			max1 = score
		} else if score > max2 {
			max2 = score
		}
	}

	return max1 - max2
}

// detectConflict 检测严重冲突
func (r *FallbackRouter) detectConflict(
	taskUnderstanding *TaskSchema,
	baselineScores map[string]float64,
	embeddingScores map[string]float64,
) (struct {
	detected bool
	details  []string
}) {
	result := struct {
		detected bool
		details  []string
	}{}

	// 获取各池的最高分
	codeBaseline := baselineScores[string(PoolCode)]
	dataBaseline := baselineScores[string(PoolData)]

	// 检查 code vs data 冲突（常见误判场景）
	if codeBaseline > r.config.ConflictThreshold && dataBaseline > r.config.ConflictThreshold {
		result.detected = true
		result.details = append(result.details, "code_vs_data_conflict")
	}

	// 检查 baseline 和 embedding 方向不一致
	// 例如 baseline 最高是 code，embedding 最高是 data
	bestBaseline := r.getBestPool(baselineScores)
	bestEmbedding := r.getBestPool(embeddingScores)

	if bestBaseline != bestEmbedding {
		// 检查差距是否足够大以确定是冲突
		baselineTop2Margin := r.calculateTopMargin(baselineScores)
		embeddingTop2Margin := r.calculateTopMargin(embeddingScores)

		if baselineTop2Margin < r.config.SmallMarginThreshold &&
			embeddingTop2Margin < r.config.SmallMarginThreshold {
			result.detected = true
			result.details = append(result.details, "baseline_embedding_direction_conflict")
		}
	}

	// 检查与 Task Understanding 的冲突
	if taskUnderstanding.Ambiguous {
		intent := taskUnderstanding.PrimaryIntent
		// 如果意图是 general_chat 但 pool 选择的是专业池，可能是冲突
		if intent == "general_chat" {
			if codeBaseline > r.config.ConflictThreshold || dataBaseline > r.config.ConflictThreshold {
				result.detected = true
				result.details = append(result.details, "intent_pool_conflict")
			}
		}
	}

	return result
}

// getBestPool 获取最高分的池
func (r *FallbackRouter) getBestPool(scores map[string]float64) string {
	bestPool := ""
	bestScore := -1.0

	for pool, score := range scores {
		if score > bestScore {
			bestScore = score
			bestPool = pool
		}
	}

	return bestPool
}

// determineFallbackPool 确定 fallback 目标池
func (r *FallbackRouter) determineFallbackPool(
	taskUnderstanding *TaskSchema,
	evidenceValidationResult *CandidatePoolResult,
) PreferredPool {
	// 安全模式：根据任务复杂度选择
	if r.config.SafeMode {
		// 如果只是简单文本，没有明确专业需求，使用 cheap_chat_pool
		if len(taskUnderstanding.InputModalities) == 1 &&
			taskUnderstanding.InputModalities[0] == "text" &&
			taskUnderstanding.PrimaryIntent == "general_chat" {
			return PoolCheap
		}

		// 如果任务复杂或歧义，使用 general_pool
		if taskUnderstanding.Ambiguous ||
			len(taskUnderstanding.Constraints) > 1 ||
			taskUnderstanding.Confidence < 0.3 {
			return PoolDefault
		}

		// 默认使用 cheap
		return PoolCheap
	}

	// 非安全模式：更激进地使用通用池
	if taskUnderstanding.Ambiguous {
		return PoolDefault
	}

	return PoolCheap
}

// determineFallbackReason 确定 fallback 原因
func (r *FallbackRouter) determineFallbackReason(conflictDetails []string) string {
	if len(conflictDetails) == 0 {
		return "unknown"
	}

	// 优先返回最严重的原因
	priorityReasons := []string{
		"low_task_understanding_confidence",
		"all_candidates_validation_failed",
		"code_vs_data_conflict",
		"baseline_embedding_direction_conflict",
		"intent_pool_conflict",
		"top_candidates_margin_too_small",
	}

	for _, reason := range priorityReasons {
		for _, detail := range conflictDetails {
			if detail == reason {
				return reason
			}
		}
	}

	return conflictDetails[0]
}

// CombinedRouter 组合路由器（整合所有组件）
type CombinedRouter struct {
	taskEngine      *TaskUnderstandingEngine
	candidateGen    *CandidatePoolGenerator
	fallbackRouter  *FallbackRouter
	prototypeScorer *PrototypeScorer
	baselineScorer  *TokenOverlapSimilarityRouter
}

// NewCombinedRouter 创建组合路由器
func NewCombinedRouter() *CombinedRouter {
	return &CombinedRouter{
		taskEngine:      NewTaskUnderstandingEngine(),
		candidateGen:    NewCandidatePoolGenerator(),
		fallbackRouter:  NewFallbackRouter(),
		prototypeScorer: NewPrototypeScorer(),
		baselineScorer:  NewTokenOverlapSimilarityRouter(),
	}
}

// Route 执行完整路由
func (c *CombinedRouter) Route(prompt string, hasImage, hasDocument, hasCSV bool) *CombinedRouteResult {
	result := &CombinedRouteResult{
		OriginalPrompt: prompt,
	}

	// 1. Task Understanding（必须先执行）
	taskUnderstanding := c.taskEngine.Understand(prompt, hasImage, hasDocument, hasCSV)
	result.TaskUnderstanding = taskUnderstanding

	// 2. Baseline 评分
	baselineScores := c.getBaselineScores(prompt)
	result.BaselineScores = baselineScores

	// 3. Prototype 评分（模拟 embedding）
	prototypeScores := c.prototypeScorer.ScoreWithPrototypes(prompt)
	result.PrototypeScores = prototypeScores

	// 4. 候选池生成和验证
	candidateResult := c.candidateGen.GenerateCandidates(prompt, hasImage, hasDocument, hasCSV, baselineScores)
	result.CandidateResult = candidateResult

	// 5. Fallback 评估
	fallbackDecision := c.fallbackRouter.EvaluateFallback(
		taskUnderstanding,
		baselineScores,
		prototypeScores,
		candidateResult,
	)
	result.FallbackDecision = fallbackDecision

	// 6. 最终决策
	if fallbackDecision.ShouldFallback {
		result.FinalPool = fallbackDecision.FallbackPool
		result.DecisionSource = "fallback"
		result.FallbackReason = fallbackDecision.FallbackReason
	} else {
		result.FinalPool = candidateResult.FinalPool
		result.DecisionSource = "validated_candidate"
	}

	// 7. 计算置信度
	result.Confidence = c.calculateConfidence(result, fallbackDecision)

	return result
}

// getBaselineScores 获取 baseline 分数
func (c *CombinedRouter) getBaselineScores(prompt string) map[string]float64 {
	scores := make(map[string]float64)
	allPools := []PreferredPool{PoolCode, PoolData, PoolVision, PoolDocument, PoolImageGeneration, PoolCheap, PoolDefault}

	for _, pool := range allPools {
		keywordScore, _ := c.baselineScorer.CalculateKeywordScore(prompt, pool)
		descScore := c.baselineScorer.CalculateDescriptionSimilarity(prompt, pool)
		totalScore := keywordScore*0.7 + descScore*0.3
		scores[string(pool)] = Round(totalScore)
	}

	return scores
}

// calculateConfidence 计算最终置信度
func (c *CombinedRouter) calculateConfidence(result *CombinedRouteResult, fallback *FallbackDecision) float64 {
	confidence := result.TaskUnderstanding.Confidence

	// 如果触发了 fallback，降低置信度
	if fallback.ShouldFallback {
		confidence = math.Min(confidence, 0.5)
	}

	// 如果有验证通过的候选，提高置信度
	if result.CandidateResult != nil {
		for _, c := range result.CandidateResult.Candidates {
			if c.Validated && c.Pool == result.FinalPool {
				confidence = math.Min(confidence+0.2, 1.0)
				break
			}
		}
	}

	return Round(confidence)
}

// CombinedRouteResult 组合路由结果
type CombinedRouteResult struct {
	OriginalPrompt    string
	TaskUnderstanding *TaskSchema
	BaselineScores    map[string]float64
	PrototypeScores   map[string]float64
	CandidateResult   *CandidatePoolResult
	FallbackDecision  *FallbackDecision
	FinalPool         PreferredPool
	DecisionSource    string
	FallbackReason    string
	Confidence        float64
}

// GetRejectionDetails 获取拒绝详情（用于可解释性）
func (r *CombinedRouteResult) GetRejectionDetails() []PoolRejectionDetail {
	details := []PoolRejectionDetail{}

	if r.CandidateResult != nil {
		rejected := r.CandidateResult.GetRejectedCandidates()
		for _, rj := range rejected {
			details = append(details, PoolRejectionDetail{
				Pool:           rj.Pool,
				RejectionReason: rj.RejectionReason,
			})
		}
	}

	return details
}

// PoolRejectionDetail 池拒绝详情
type PoolRejectionDetail struct {
	Pool           PreferredPool `json:"pool"`
	RejectionReason string       `json:"rejection_reason"`
}