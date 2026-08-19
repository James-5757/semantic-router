package semanticrouter

import (
	"sort"
	"strings"
)

// CandidatePoolGenerator 候选池生成器
type CandidatePoolGenerator struct {
	taskEngine *TaskUnderstandingEngine
	validator  *PoolValidator
	scorer     *TokenOverlapSimilarityRouter
}

// NewCandidatePoolGenerator 创建候选池生成器
func NewCandidatePoolGenerator() *CandidatePoolGenerator {
	return &CandidatePoolGenerator{
		taskEngine: NewTaskUnderstandingEngine(),
		validator:  NewPoolValidator(),
		scorer:     NewTokenOverlapSimilarityRouter(),
	}
}

// GenerateCandidates 生成候选池
// 流程：Task Understanding -> Baseline/Embedding 候选生成 -> Evidence Validation -> 置信度/冲突解决
func (g *CandidatePoolGenerator) GenerateCandidates(prompt string, hasImage, hasDocument, hasCSV bool, baselineScores map[string]float64) *CandidatePoolResult {
	result := &CandidatePoolResult{
		OriginalPrompt: prompt,
	}

	// 1. 先执行 Task Understanding（必须先于 Pool 决策）
	taskUnderstanding := g.taskEngine.Understand(prompt, hasImage, hasDocument, hasCSV)
	result.TaskUnderstanding = taskUnderstanding

	// 2. 从 baseline scores 生成候选池
	candidates := g.generateFromBaseline(taskUnderstanding, baselineScores)

	// 3. 证据验证
	validatedCandidates := g.validator.ValidateCandidates(candidates, taskUnderstanding)
	result.Candidates = validatedCandidates

	// 4. 冲突解决和最终池选择
	result.FinalPool = g.resolveFinalPool(validatedCandidates, taskUnderstanding)

	// 5. 检查是否需要 fallback
	result.FallbackPool, result.FallbackReason = g.checkFallback(validatedCandidates, taskUnderstanding)

	// 6. 如果需要 fallback，使用 fallback pool
	if result.FallbackPool != "" {
		result.FinalPool = result.FallbackPool
	}

	return result
}

// generateFromBaseline 从 baseline scores 生成候选池
func (g *CandidatePoolGenerator) generateFromBaseline(taskUnderstanding *TaskSchema, baselineScores map[string]float64) []*CandidatePool {
	candidates := make([]*CandidatePool, 0)

	// 所有可能的池
	allPools := []PreferredPool{
		PoolCode, PoolData, PoolVision, PoolDocument, PoolImageGeneration, PoolCheap, PoolDefault,
	}

	for _, pool := range allPools {
		score := baselineScores[string(pool)]
		if score <= 0 {
			continue // 跳过得分为 0 的池
		}

		candidate := &CandidatePool{
			Pool:           pool,
			CandidateScore: score,
		}

		// 从 Task Understanding 提取证据
		evidence := g.extractEvidence(taskUnderstanding)
		candidate.SupportingEvidence = evidence

		candidates = append(candidates, candidate)
	}

	// 按分数排序
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].CandidateScore > candidates[j].CandidateScore
	})

	return candidates
}

// extractEvidence 从 Task Understanding 提取证据
func (g *CandidatePoolGenerator) extractEvidence(taskUnderstanding *TaskSchema) []EvidenceType {
	evidence := []EvidenceType{}

	// 动作证据
	for _, action := range taskUnderstanding.Actions {
		switch action {
		case "write", "debug", "refactor":
			evidence = append(evidence, EvidenceCodingAction)
		case "analyze":
			// 需要结合领域判断
			if containsString(taskUnderstanding.Domains, "data_science") {
				evidence = append(evidence, EvidenceDataAction)
			} else {
				evidence = append(evidence, EvidenceVisionAction)
			}
		case "generate":
			// 需要结合输出判断
			if containsString(taskUnderstanding.OutputArtifacts, "image") {
				evidence = append(evidence, EvidenceCreativeAction)
			} else if containsString(taskUnderstanding.OutputArtifacts, "chart") {
				evidence = append(evidence, EvidenceDataAction)
			}
		case "explain", "plan":
			// 这些可能是简单查询，标记为待定
		}
	}

	// 对象证据
	for _, obj := range taskUnderstanding.Objects {
		if strings.HasPrefix(obj, "programming_language:") {
			evidence = append(evidence, EvidenceProgrammingContext)
		} else if containsString([]string{"function", "class", "method", "api", "code", "algorithm"}, obj) {
			evidence = append(evidence, EvidenceTechnicalObject)
		}
	}

	// 输入模态证据
	for _, modality := range taskUnderstanding.InputModalities {
		switch modality {
		case "image":
			evidence = append(evidence, EvidenceImageInput)
		case "document":
			evidence = append(evidence, EvidenceDocumentInput)
		case "csv":
			evidence = append(evidence, EvidenceDataObject)
		case "text":
			if len(taskUnderstanding.InputModalities) == 1 {
				evidence = append(evidence, EvidenceTextOnly)
			}
		}
	}

	// 输出产物证据
	for _, artifact := range taskUnderstanding.OutputArtifacts {
		switch artifact {
		case "executable_code":
			evidence = append(evidence, EvidenceCodeOutput)
		case "chart":
			evidence = append(evidence, EvidenceChartOutput)
		case "image", "creative_image":
			evidence = append(evidence, EvidenceArtisticOutput)
		case "data":
			evidence = append(evidence, EvidenceDataObject)
		}
	}

	// 领域证据
	for _, domain := range taskUnderstanding.Domains {
		switch domain {
		case "programming":
			// 已在动作/对象中处理
		case "data_science":
			evidence = append(evidence, EvidenceDataDomain)
		case "vision":
			evidence = append(evidence, EvidenceVisionAction)
		}
	}

	// 意图证据
	switch taskUnderstanding.PrimaryIntent {
	case "code_generation":
		// 已在动作/对象中处理
	case "data_analysis":
		evidence = append(evidence, EvidenceDataAction)
	case "image_understanding":
		evidence = append(evidence, EvidenceVisionAction, EvidenceImageInput)
	case "image_generation":
		evidence = append(evidence, EvidenceImageGenIntent, EvidenceCreativeAction)
	case "document_processing":
		evidence = append(evidence, EvidenceDocumentAction, EvidenceDocumentInput)
	case "general_chat":
		if len(taskUnderstanding.InputModalities) == 1 && taskUnderstanding.InputModalities[0] == "text" {
			evidence = append(evidence, EvidenceSimpleQuery)
		}
	}

	// 歧义证据
	if taskUnderstanding.Ambiguous {
		evidence = append(evidence, EvidenceAmbiguous)
	}

	return uniqueEvidence(evidence)
}

// resolveFinalPool 解决最终池选择
func (g *CandidatePoolGenerator) resolveFinalPool(candidates []*CandidatePool, taskUnderstanding *TaskSchema) PreferredPool {
	if len(candidates) == 0 {
		return PoolDefault
	}

	// 1. 优先选择验证通过的候选
	var validatedCandidates []*CandidatePool
	for _, c := range candidates {
		if c.Validated {
			validatedCandidates = append(validatedCandidates, c)
		}
	}

	if len(validatedCandidates) > 0 {
		// 选择分数最高的
		return validatedCandidates[0].Pool
	}

	// 2. 如果没有验证通过的，检查是否有高分候选可以覆盖
	// 但需要记录冲突信息
	for _, c := range candidates {
		if c.CandidateScore >= 0.3 {
			// 高分但验证失败，记录原因并继续
			continue
		}
	}

	// 3. 返回默认池
	return PoolDefault
}

// checkFallback 检查是否需要 fallback
func (g *CandidatePoolGenerator) checkFallback(candidates []*CandidatePool, taskUnderstanding *TaskSchema) (PreferredPool, string) {
	// 条件1：Task Understanding 置信度低
	if taskUnderstanding.Confidence < 0.4 {
		return PoolDefault, "low_task_understanding_confidence"
	}

	// 条件2：所有候选池都验证失败
	allFailed := true
	for _, c := range candidates {
		if c.Validated {
			allFailed = false
			break
		}
	}
	if allFailed && len(candidates) > 0 {
		// 检查候选分数，如果最高分足够高但验证失败，可能是误判
		if len(candidates) > 0 && candidates[0].CandidateScore > 0.3 {
			// 检查是否是歧义导致
			if taskUnderstanding.Ambiguous {
				return PoolDefault, "all_candidates_failed_with_ambiguity"
			}
		}
		return PoolCheap, "all_candidates_validation_failed"
	}

	// 条件3：Top1/Top2 差距过小
	if len(candidates) >= 2 {
		margin := candidates[0].CandidateScore - candidates[1].CandidateScore
		if margin < 0.05 && taskUnderstanding.Ambiguous {
			return PoolDefault, "top_candidates_margin_too_small"
		}
	}

	// 条件4：检查是否有严重冲突
	hasCodeCandidate := false
	hasDataCandidate := false
	for _, c := range candidates {
		if c.Pool == PoolCode && c.CandidateScore > 0.1 {
			hasCodeCandidate = true
		}
		if c.Pool == PoolData && c.CandidateScore > 0.1 {
			hasDataCandidate = true
		}
	}
	if hasCodeCandidate && hasDataCandidate {
		// 存在代码vs数据的冲突，需要更多证据
		if taskUnderstanding.Ambiguous {
			return PoolDefault, "code_data_conflict_with_ambiguity"
		}
	}

	// 不需要 fallback
	return "", ""
}

// CandidatePoolResult 候选池生成结果
type CandidatePoolResult struct {
	OriginalPrompt  string
	TaskUnderstanding *TaskSchema
	Candidates      []*CandidatePool
	FinalPool       PreferredPool
	FallbackPool    PreferredPool
	FallbackReason  string
	ConflictDetails []string
}

// uniqueEvidence 去重证据
func uniqueEvidence(evidence []EvidenceType) []EvidenceType {
	seen := make(map[EvidenceType]bool)
	result := []EvidenceType{}
	for _, e := range evidence {
		if !seen[e] {
			seen[e] = true
			result = append(result, e)
		}
	}
	return result
}

// GetRejectedCandidates 获取被拒绝的候选及原因
func (r *CandidatePoolResult) GetRejectedCandidates() []struct {
	Pool           PreferredPool
	RejectionReason string
} {
	rejected := make([]struct {
		Pool           PreferredPool
		RejectionReason string
	}, 0)

	for _, c := range r.Candidates {
		if !c.Validated && c.RejectionReason != "" {
			rejected = append(rejected, struct {
				Pool           PreferredPool
				RejectionReason string
			}{
				Pool:           c.Pool,
				RejectionReason: c.RejectionReason,
			})
		}
	}

	return rejected
}