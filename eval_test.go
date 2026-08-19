package semanticrouter

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

// PoolConfusionMatrix Pool 混淆矩阵
type PoolConfusionMatrix struct {
	Matrix map[string]map[string]int // actual -> predicted -> count
}

// TierConfusionMatrix Tier 混淆矩阵
type TierConfusionMatrix struct {
	Matrix map[string]map[string]int // actual -> predicted -> count
}

// EvalResult 评估结果
type EvalResult struct {
	TotalCases        int
	PoolCorrect       int
	TierCorrect       int
	FallbackCorrect   int
	FailedCases       []FailedCase
	PoolConfusion     PoolConfusionMatrix
	TierConfusion     TierConfusionMatrix
}

// FailedCase 失败案例 - 包含详细信息
type FailedCase struct {
	Index                int                      `json:"index"`
	Prompt               string                   `json:"prompt"`
	ExpectedPool         string                   `json:"expected_pool"`
	GotPool              string                   `json:"got_pool"`
	ExpectedTier         string                   `json:"expected_tier"`
	GotTier              string                   `json:"got_tier"`
	MatchedRules         []string                 `json:"matched_rules,omitempty"`
	SemanticScores       map[string]float64       `json:"semantic_scores,omitempty"`
	Confidence           float64                  `json:"confidence"`
	FinalDecisionSource  string                   `json:"final_decision_source"`
	FallbackReason       string                   `json:"fallback_reason,omitempty"`
	SecondBestPool       string                   `json:"second_best_pool,omitempty"`
	SecondBestScore      float64                  `json:"second_best_score"`
	ScoreMargin          float64                  `json:"score_margin"`
	RuleScore            float64                  `json:"rule_score"`
}

// EvalSummary 评估汇总
type EvalSummary struct {
	TotalCases      int                    `json:"total_cases"`
	PoolAccuracy    float64                `json:"pool_accuracy"`
	TierAccuracy    float64                `json:"tier_accuracy"`
	FallbackRate    float64                `json:"fallback_rate"`
	FailedCases     []FailedCase           `json:"failed_cases"`
	PoolConfusion   map[string]map[string]int `json:"pool_confusion_matrix"`
	TierConfusion   map[string]map[string]int `json:"tier_confusion_matrix"`
}

// loadEvalCases 加载评估样本
func loadEvalCases(path string) ([]EvalCase, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var cases []EvalCase
	decoder := json.NewDecoder(file)
	for decoder.More() {
		var c EvalCase
		if err := decoder.Decode(&c); err != nil {
			return nil, err
		}
		cases = append(cases, c)
	}
	return cases, nil
}

// TestRoutingEvalCases 路由评估测试
func TestRoutingEvalCases(t *testing.T) {
	cases, err := loadEvalCases("routing_eval_cases.jsonl")
	if err != nil {
		t.Skipf("跳过评估测试：无法加载测试样本: %v", err)
	}

	multiLayerRouter := NewMultiLayerRouter()
	tierRouter := NewRuleBasedTierRouter()

	result := &EvalResult{
		TotalCases:  len(cases),
		FailedCases: []FailedCase{},
		PoolConfusion: PoolConfusionMatrix{
			Matrix: make(map[string]map[string]int),
		},
		TierConfusion: TierConfusionMatrix{
			Matrix: make(map[string]map[string]int),
		},
	}

	// Label normalization map
	// Short names in eval cases -> actual pool names in router
	labelMap := map[string]string{
		"code":               "code",
		"data":               "data",
		"vision":             "vision",
		"document":           "document",
		"image_generation":   "image_generation",
		"default":            "default",
		"cheap":              "cheap",
		"general":            "default",
		"private":            "private",
		"code_pool":          "code",   // 反向映射
		"data_pool":          "data",
		"vision_pool":        "vision",
		"document_pool":      "document",
		"image_generation_pool": "image_generation",
	}

	for i, c := range cases {
		// Normalize expected pool and tier labels
		expectedPool := normalizeLabel(c.ExpectedPool, labelMap)
		expectedTier := normalizeLabel(c.ExpectedTier, labelMap)

		// 构建路由请求
		req := &RouteRequest{
			Prompt:      c.Prompt,
			Model:       c.Model,
			HasImage:    len(c.Images) > 0,
			HasDocument: len(c.Documents) > 0,
		}

		// 执行语义路由
		semanticDecision := multiLayerRouter.Route(req)

		// 执行 Tier 路由
		tierDecision, _ := tierRouter.RouteWithPrompt(nil, c.Model, semanticDecision.TaskType, c.Prompt)

		// 更新混淆矩阵
		// Pool
		if _, ok := result.PoolConfusion.Matrix[expectedPool]; !ok {
			result.PoolConfusion.Matrix[expectedPool] = make(map[string]int)
		}
		result.PoolConfusion.Matrix[expectedPool][string(semanticDecision.PreferredPool)]++

		// Tier
		if _, ok := result.TierConfusion.Matrix[expectedTier]; !ok {
			result.TierConfusion.Matrix[expectedTier] = make(map[string]int)
		}
		result.TierConfusion.Matrix[expectedTier][string(tierDecision.PreferredTier)]++

		// 检查 Pool 匹配
		poolMatched := string(semanticDecision.PreferredPool) == expectedPool
		if poolMatched {
			result.PoolCorrect++
		}

		// 检查 Tier 匹配
		tierMatched := string(tierDecision.PreferredTier) == expectedTier
		if tierMatched {
			result.TierCorrect++
		}

		// 检查 Fallback 匹配
		fallbackMatched := true
		if c.ExpectedFallback {
			fallbackMatched = semanticDecision.DecisionSource == DecisionSourceFallback
		}
		if fallbackMatched {
			result.FallbackCorrect++
		}

		// 记录失败案例 - 包含详细信息
		if !poolMatched || !tierMatched {
			result.FailedCases = append(result.FailedCases, FailedCase{
				Index:               i,
				Prompt:              c.Prompt,
				ExpectedPool:        expectedPool,
				GotPool:             string(semanticDecision.PreferredPool),
				ExpectedTier:        expectedTier,
				GotTier:             string(tierDecision.PreferredTier),
				MatchedRules:        semanticDecision.MatchedRules,
				SemanticScores:      semanticDecision.SemanticScores,
				Confidence:          semanticDecision.Confidence,
				FinalDecisionSource: string(semanticDecision.DecisionSource),
				FallbackReason:      semanticDecision.FallbackReason,
				SecondBestPool:      string(semanticDecision.SecondBestPool),
				SecondBestScore:     semanticDecision.SecondBestScore,
				ScoreMargin:         semanticDecision.ScoreMargin,
				RuleScore:           semanticDecision.RuleScore,
			})
		}
	}

	// 输出汇总结果
	t.Logf("\n=== 评估汇总 ===")
	t.Logf("总样本数: %d", result.TotalCases)
	t.Logf("Pool 准确率: %.2f%% (%d/%d)",
		float64(result.PoolCorrect)/float64(result.TotalCases)*100,
		result.PoolCorrect, result.TotalCases)
	t.Logf("Tier 准确率: %.2f%% (%d/%d)",
		float64(result.TierCorrect)/float64(result.TotalCases)*100,
		result.TierCorrect, result.TotalCases)
	t.Logf("Fallback 准确率: %.2f%% (%d/%d)",
		float64(result.FallbackCorrect)/float64(result.TotalCases)*100,
		result.FallbackCorrect, result.TotalCases)

	// 输出 Pool Confusion Matrix
	t.Logf("\n=== Pool Confusion Matrix (Actual -> Predicted) ===")
	t.Logf("%-12s %s", "Actual", "Predicted")
	t.Logf(strings.Repeat("-", 60))
	for actual, predictedMap := range result.PoolConfusion.Matrix {
		for predicted, count := range predictedMap {
			if count > 0 {
				marker := ""
				if actual == predicted {
					marker = " ✓"
				}
				t.Logf("%-12s -> %-12s: %d%s", actual, predicted, count, marker)
			}
		}
	}

	// 输出 Tier Confusion Matrix
	t.Logf("\n=== Tier Confusion Matrix (Actual -> Predicted) ===")
	t.Logf("%-12s %s", "Actual", "Predicted")
	t.Logf(strings.Repeat("-", 60))
	for actual, predictedMap := range result.TierConfusion.Matrix {
		for predicted, count := range predictedMap {
			if count > 0 {
				marker := ""
				if actual == predicted {
					marker = " ✓"
				}
				t.Logf("%-12s -> %-12s: %d%s", actual, predicted, count, marker)
			}
		}
	}

	// 显示 Top 20 失败案例
	t.Logf("\n=== Top 20 失败案例 ===")
	// 按索引排序
	sort.Slice(result.FailedCases, func(i, j int) bool {
		return result.FailedCases[i].Index < result.FailedCases[j].Index
	})

	topN := 20
	if len(result.FailedCases) < topN {
		topN = len(result.FailedCases)
	}

	for idx := 0; idx < topN; idx++ {
		fc := result.FailedCases[idx]
		t.Logf("\n[%d] %s", fc.Index, fc.Prompt)
		t.Logf("    Pool: got=%s, want=%s", fc.GotPool, fc.ExpectedPool)
		t.Logf("    Tier: got=%s, want=%s", fc.GotTier, fc.ExpectedTier)
		t.Logf("    matched_rules: %v", fc.MatchedRules)
		t.Logf("    semantic_scores: %v", fc.SemanticScores)
		t.Logf("    confidence: %.2f, rule_score: %.2f", fc.Confidence, fc.RuleScore)
		t.Logf("    decision_source: %s, fallback_reason: %s", fc.FinalDecisionSource, fc.FallbackReason)
		t.Logf("    second_best_pool: %s (%.2f), score_margin: %.2f",
			fc.SecondBestPool, fc.SecondBestScore, fc.ScoreMargin)
	}

	// 打印 JSON 格式的汇总（便于程序解析）
	summary := EvalSummary{
		TotalCases:    result.TotalCases,
		PoolAccuracy:  float64(result.PoolCorrect) / float64(result.TotalCases) * 100,
		TierAccuracy:  float64(result.TierCorrect) / float64(result.TotalCases) * 100,
		FallbackRate:  float64(result.FallbackCorrect) / float64(result.TotalCases) * 100,
		FailedCases:   result.FailedCases,
		PoolConfusion: result.PoolConfusion.Matrix,
		TierConfusion: result.TierConfusion.Matrix,
	}
	summaryJSON, _ := json.MarshalIndent(summary, "", "  ")
	t.Logf("\n=== JSON 汇总 ===")
	t.Logf("%s", summaryJSON)

	// 设置测试通过阈值
	poolAccuracy := float64(result.PoolCorrect) / float64(result.TotalCases)
	if poolAccuracy < 0.5 {
		t.Logf("WARNING: Pool 准确率 %.2f%% 低于期望值 50%%", poolAccuracy*100)
	} else {
		t.Logf("Pool 准确率 %.2f%% 达到期望值", poolAccuracy*100)
	}
}

// boolToEmoji 将布尔值转换为 emoji
func boolToEmoji(b bool) string {
	if b {
		return "✅"
	}
	return "❌"
}

// formatFloat 格式化浮点数
func formatFloat(v float64) string {
	return fmt.Sprintf("%.2f", v)
}

