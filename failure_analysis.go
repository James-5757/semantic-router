package semanticrouter

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// FailureCase 记录单个路由失败案例的详细信息，用于后续分析和调试。
type FailureCase struct {
	Prompt              string
	ExpectedPool        string
	PredictedPool       string
	ExpectedTier        string
	PredictedTier       string
	MatchedRules        []string
	RuleScore           float64
	SemanticScore       float64
	Confidence          float64
	SecondBestPool      string
	ScoreMargin         float64
	FinalDecisionSource string
	FailureReason       string
}

// FailureCluster 将具有相同失败模式的案例归为一组。
type FailureCluster struct {
	ClusterName string
	Count       int
	Examples    []FailureCase
}

// AnalyzeFailures 对每个评估样本执行路由并与预期结果对比，生成详细的失败分析报告。
// 返回失败案例列表以及以下关键错误计数值：
//   - codeToDefault:    预期为 code 却被路由到 default 的样本数
//   - dataToDefault:    预期为 data 却被路由到 default 的样本数
//   - imageGenToVision: 预期为 image_generation 却被路由到 vision 的样本数
func AnalyzeFailures(cases []EvalCase, router *MultiLayerRouter, tierRouter *RuleBasedTierRouter, model string) ([]FailureCase, int, int, int) {
	failures := make([]FailureCase, 0)

	var codeToDefault, dataToDefault, imageGenToVision int

	// Label normalization map（与 eval_test.go 中的定义保持一致）
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
		"code_pool":          "code",
		"data_pool":          "data",
		"vision_pool":        "vision",
		"document_pool":      "document",
		"image_generation_pool": "image_generation",
	}

	for _, c := range cases {
		expectedPool := normalizeLabel(c.ExpectedPool, labelMap)
		expectedTier := normalizeLabel(c.ExpectedTier, labelMap)

		// 构建路由请求
		req := &RouteRequest{
			Prompt:      c.Prompt,
			Model:       model,
			HasImage:    len(c.Images) > 0,
			HasDocument: len(c.Documents) > 0,
		}

		// 执行语义路由
		semanticDecision := router.Route(req)

		// 执行 Tier 路由
		tierDecision, _ := tierRouter.RouteWithPrompt(nil, model, semanticDecision.TaskType, c.Prompt)

		predictedPool := string(semanticDecision.PreferredPool)
		predictedTier := string(tierDecision.PreferredTier)

		poolMatched := predictedPool == expectedPool
		tierMatched := predictedTier == expectedTier

		// 如果 pool 和 tier 都匹配则跳过
		if poolMatched && tierMatched {
			continue
		}

		// 构造失败原因分析
		failureReason := buildFailureReason(expectedPool, predictedPool, expectedTier, predictedTier, semanticDecision, tierDecision)

		fc := FailureCase{
			Prompt:              c.Prompt,
			ExpectedPool:        expectedPool,
			PredictedPool:       predictedPool,
			ExpectedTier:        expectedTier,
			PredictedTier:       predictedTier,
			MatchedRules:        semanticDecision.MatchedRules,
			RuleScore:           semanticDecision.RuleScore,
			SemanticScore:       semanticDecision.SemanticScore,
			Confidence:          semanticDecision.Confidence,
			SecondBestPool:      string(semanticDecision.SecondBestPool),
			ScoreMargin:         semanticDecision.ScoreMargin,
			FinalDecisionSource: string(semanticDecision.DecisionSource),
			FailureReason:       failureReason,
		}

		failures = append(failures, fc)

		// 统计特定的错误模式
		if expectedPool == "code" && predictedPool == "default" {
			codeToDefault++
		}
		if expectedPool == "data" && predictedPool == "default" {
			dataToDefault++
		}
		if expectedPool == "image_generation" && predictedPool == "vision" {
			imageGenToVision++
		}
	}

	return failures, codeToDefault, dataToDefault, imageGenToVision
}

// buildFailureReason 分析预期与实际结果之间的差异，返回人类可读的失败原因描述。
func buildFailureReason(expectedPool, predictedPool, expectedTier, predictedTier string, decision *MultiLayerDecision, _ *TierRouteDecision) string {
	var reasons []string

	if expectedPool != predictedPool {
		reason := analyzePoolMismatch(expectedPool, predictedPool, decision)
		reasons = append(reasons, reason)
	}

	if expectedTier != predictedTier {
		reasons = append(reasons, fmt.Sprintf("tier_mismatch: expected=%s, got=%s", expectedTier, predictedTier))
	}

	if len(reasons) == 0 {
		return "unknown"
	}
	return strings.Join(reasons, "; ")
}

// analyzePoolMismatch 分析 pool 不匹配的具体根因。
func analyzePoolMismatch(expected, predicted string, decision *MultiLayerDecision) string {
	// 检查是否 fallback 到了 default
	if decision.DecisionSource == DecisionSourceFallback {
		return fmt.Sprintf("default_fallback: expected=%s, reason=%s", expected, decision.FallbackReason)
	}

	// 检查语义得分是否过低导致无法识别
	scores := decision.SemanticScores
	expectedScore := scores[expected]
	predictedScore := scores[predicted]

	if expectedScore == 0 && predictedScore == 0 {
		return fmt.Sprintf("no_semantic_signal: expected=%s, predicted=%s, all_scores_near_zero", expected, predicted)
	}

	// 检查是否是某个得分刚好略高导致"抢走"
	if predictedScore > expectedScore {
		decisionSource := string(decision.DecisionSource)
		return fmt.Sprintf("higher_competitor_score: expected=%s(%.3f), predicted=%s(%.3f), margin=%.3f, source=%s",
			expected, expectedScore, predicted, predictedScore, predictedScore-expectedScore, decisionSource)
	}

	// 检查规则是否覆盖了语义得分
	if decision.RuleScore > 0 && decision.RuleScore >= decision.Confidence && decision.DecisionSource == DecisionSourceRule {
		return fmt.Sprintf("rule_overrides_semantic: expected=%s(%.3f), predicted=%s, rule_score=%.3f, rules=%v",
			expected, expectedScore, predicted, decision.RuleScore, decision.MatchedRules)
	}

	// 检查 score_margin 是否很小（歧义大）
	if decision.ScoreMargin < 0.05 {
		return fmt.Sprintf("ambiguous_scores: expected=%s(%.3f), predicted=%s(%.3f), margin=%.3f",
			expected, expectedScore, predicted, predictedScore, decision.ScoreMargin)
	}

	return fmt.Sprintf("pool_mismatch: expected=%s, predicted=%s, expected_score=%.3f, predicted_score=%.3f, source=%s",
		expected, predicted, expectedScore, predictedScore, string(decision.DecisionSource))
}

// ClusterFailures 将失败案例按模式归类，返回分组结果。
// 预定义的归类包括："code->default", "code->data", "data->default", "data->code",
// "vision->default", "image_generation->vision", "document->default", "tier_mismatch"。
func ClusterFailures(failures []FailureCase) []FailureCluster {
	type clusterKey struct {
		name     string
		matcher  func(FailureCase) bool
	}

	// 定义归类规则匹配函数（按优先级匹配，每个失败案例只归入第一个匹配的簇）
	clusterDefs := []clusterKey{
		{name: "code->default", matcher: func(f FailureCase) bool {
			return f.ExpectedPool == "code" && f.PredictedPool == "default"
		}},
		{name: "code->data", matcher: func(f FailureCase) bool {
			return f.ExpectedPool == "code" && f.PredictedPool == "data"
		}},
		{name: "data->default", matcher: func(f FailureCase) bool {
			return f.ExpectedPool == "data" && f.PredictedPool == "default"
		}},
		{name: "data->code", matcher: func(f FailureCase) bool {
			return f.ExpectedPool == "data" && f.PredictedPool == "code"
		}},
		{name: "vision->default", matcher: func(f FailureCase) bool {
			return f.ExpectedPool == "vision" && f.PredictedPool == "default"
		}},
		{name: "image_generation->vision", matcher: func(f FailureCase) bool {
			return f.ExpectedPool == "image_generation" && f.PredictedPool == "vision"
		}},
		{name: "document->default", matcher: func(f FailureCase) bool {
			return f.ExpectedPool == "document" && f.PredictedPool == "default"
		}},
		{name: "tier_mismatch", matcher: func(f FailureCase) bool {
			return f.ExpectedPool == f.PredictedPool && f.ExpectedTier != f.PredictedTier
		}},
	}

	// 先按优先级归类
	clusters := make([]FailureCluster, 0, len(clusterDefs))
	remaining := make([]FailureCase, 0)

	// 用于标记已被归类的案例
	used := make([]bool, len(failures))

	for _, def := range clusterDefs {
		var matched []FailureCase
		for i, f := range failures {
			if used[i] {
				continue
			}
			if def.matcher(f) {
				matched = append(matched, f)
				used[i] = true
			}
		}
		if len(matched) > 0 {
			examples := matched
			if len(examples) > 3 {
				examples = matched[:3]
			}
			clusters = append(clusters, FailureCluster{
				ClusterName: def.name,
				Count:       len(matched),
				Examples:    examples,
			})
		}
	}

	// 找出未被任何簇匹配的案例
	for i, f := range failures {
		if !used[i] {
			remaining = append(remaining, f)
		}
	}

	if len(remaining) > 0 {
		examples := remaining
		if len(examples) > 3 {
			examples = remaining[:3]
		}
		clusterNames := make([]string, 0)
		seen := make(map[string]bool)
		for _, f := range remaining {
			key := fmt.Sprintf("%s->%s", f.ExpectedPool, f.PredictedPool)
			if !seen[key] {
				seen[key] = true
				clusterNames = append(clusterNames, key)
			}
		}
		name := fmt.Sprintf("other_pool_mismatch (%s)", strings.Join(clusterNames, ", "))
		clusters = append(clusters, FailureCluster{
			ClusterName: name,
			Count:       len(remaining),
			Examples:    examples,
		})
	}

	return clusters
}

// PrintFailureAnalysis 将失败的详细分析结果格式化输出到 stdout。
func PrintFailureAnalysis(failures []FailureCase, clusters []FailureCluster) {
	fmt.Println("\n=== Failure Analysis Report ===")
	fmt.Println(strings.Repeat("=", 40))

	// --- Summary ---
	fmt.Printf("\n--- Summary ---\n")
	fmt.Printf("Total failures: %d\n", len(failures))

	// 统计失败类型
	poolMismatches := 0
	tierMismatches := 0
	for _, f := range failures {
		if f.ExpectedPool != f.PredictedPool {
			poolMismatches++
		}
		if f.ExpectedTier != f.PredictedTier {
			tierMismatches++
		}
	}
	fmt.Printf("Pool mismatches:  %d\n", poolMismatches)
	fmt.Printf("Tier mismatches:  %d\n", tierMismatches)

	// --- Pool Confusion Matrix ---
	fmt.Println("\n--- Pool Confusion Matrix ---")
	poolMatrix := buildPoolConfusionMatrix(failures)
	printFailConfusionMatrix(poolMatrix)

	// --- Tier Confusion Matrix ---
	fmt.Println("\n--- Tier Confusion Matrix ---")
	tierMatrix := buildTierConfusionMatrix(failures)
	printFailConfusionMatrix(tierMatrix)

	// --- Top 20 Pool Failures (by score_margin ascending = most ambiguous first) ---
	fmt.Println("\n--- Top 20 Pool Failures (most ambiguous first) ---")
	poolFailures := filterPoolFailures(failures)
	sort.Slice(poolFailures, func(i, j int) bool {
		return poolFailures[i].ScoreMargin < poolFailures[j].ScoreMargin
	})
	topN := 20
	if len(poolFailures) < topN {
		topN = len(poolFailures)
	}
	for idx := 0; idx < topN; idx++ {
		f := poolFailures[idx]
		fmt.Println()
		fmt.Printf("[%d] Prompt: %s\n", idx, truncatePrompt(f.Prompt, 80))
		fmt.Printf("    Pool: expected=%s, got=%s\n", f.ExpectedPool, f.PredictedPool)
		fmt.Printf("    MatchedRules: %v\n", f.MatchedRules)
		fmt.Printf("    Confidence: %.3f, RuleScore: %.3f, SemanticScore: %.3f\n", f.Confidence, f.RuleScore, f.SemanticScore)
		fmt.Printf("    SecondBest: %s, ScoreMargin: %.4f\n", f.SecondBestPool, f.ScoreMargin)
		fmt.Printf("    DecisionSource: %s\n", f.FinalDecisionSource)
		fmt.Printf("    FailureReason: %s\n", f.FailureReason)
	}

	// --- Top 20 Tier Failures ---
	fmt.Println("\n--- Top 20 Tier Failures ---")
	tierFailures := filterTierFailures(failures)
	topNTier := 20
	if len(tierFailures) < topNTier {
		topNTier = len(tierFailures)
	}
	for idx := 0; idx < topNTier; idx++ {
		f := tierFailures[idx]
		fmt.Println()
		fmt.Printf("[%d] Prompt: %s\n", idx, truncatePrompt(f.Prompt, 80))
		fmt.Printf("    Tier: expected=%s, got=%s\n", f.ExpectedTier, f.PredictedTier)
		fmt.Printf("    Pool: expected=%s, got=%s\n", f.ExpectedPool, f.PredictedPool)
		fmt.Printf("    Confidence: %.3f\n", f.Confidence)
		fmt.Printf("    FailureReason: %s\n", f.FailureReason)
	}

	// --- Failure Clusters ---
	fmt.Println("\n--- Failure Clusters ---")
	for _, cluster := range clusters {
		fmt.Printf("\nCluster: %s (count=%d)\n", cluster.ClusterName, cluster.Count)
		for i, example := range cluster.Examples {
			fmt.Printf("  Example %d:\n", i+1)
			fmt.Printf("    Prompt:      %s\n", truncatePrompt(example.Prompt, 80))
			fmt.Printf("    Expected:    pool=%s, tier=%s\n", example.ExpectedPool, example.ExpectedTier)
			fmt.Printf("    Predicted:   pool=%s, tier=%s\n", example.PredictedPool, example.PredictedTier)
			fmt.Printf("    Reason:      %s\n", example.FailureReason)
		}
	}

	fmt.Println()
}

// buildPoolConfusionMatrix 根据失败案例构建 pool 混淆矩阵。
func buildPoolConfusionMatrix(failures []FailureCase) map[string]map[string]int {
	matrix := make(map[string]map[string]int)
	for _, f := range failures {
		if _, ok := matrix[f.ExpectedPool]; !ok {
			matrix[f.ExpectedPool] = make(map[string]int)
		}
		matrix[f.ExpectedPool][f.PredictedPool]++
	}
	return matrix
}

// buildTierConfusionMatrix 根据失败案例构建 tier 混淆矩阵。
func buildTierConfusionMatrix(failures []FailureCase) map[string]map[string]int {
	matrix := make(map[string]map[string]int)
	for _, f := range failures {
		if _, ok := matrix[f.ExpectedTier]; !ok {
			matrix[f.ExpectedTier] = make(map[string]int)
		}
		matrix[f.ExpectedTier][f.PredictedTier]++
	}
	return matrix
}

// printFailConfusionMatrix 将混淆矩阵格式化为表格输出。
func printFailConfusionMatrix(matrix map[string]map[string]int) {
	// 收集所有行列标签
	allKeys := make(map[string]bool)
	for actual, predictedMap := range matrix {
		allKeys[actual] = true
		for predicted := range predictedMap {
			allKeys[predicted] = true
		}
	}
	sortedKeys := sortedKeysStr(allKeys)

	// 打印表头
	fmt.Printf("%-18s", "Actual \\ Predicted")
	for _, k := range sortedKeys {
		fmt.Printf("%-12s", k)
	}
	fmt.Println()
	fmt.Println(strings.Repeat("-", 18+12*len(sortedKeys)))

	// 打印每一行
	for _, actual := range sortedKeys {
		fmt.Printf("%-18s", actual)
		for _, predicted := range sortedKeys {
			count := matrix[actual][predicted]
			if count > 0 {
				marker := ""
				if actual == predicted {
					marker = "*"
				}
				fmt.Printf("%-12s", fmt.Sprintf("%d%s", count, marker))
			} else {
				fmt.Printf("%-12s", "0")
			}
		}
		fmt.Println()
	}
}

// filterPoolFailures 从失败案例中筛选出 pool 不匹配的案例。
func filterPoolFailures(failures []FailureCase) []FailureCase {
	var result []FailureCase
	for _, f := range failures {
		if f.ExpectedPool != f.PredictedPool {
			result = append(result, f)
		}
	}
	return result
}

// filterTierFailures 从失败案例中筛选出 tier 不匹配的案例。
func filterTierFailures(failures []FailureCase) []FailureCase {
	var result []FailureCase
	for _, f := range failures {
		if f.ExpectedTier != f.PredictedTier {
			result = append(result, f)
		}
	}
	return result
}

// truncatePrompt 截断字符串到指定最大长度，并在末尾附加省略号。
func truncatePrompt(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// sortedKeysStr 返回字符串集合的排序后的切片。
func sortedKeysStr(keys map[string]bool) []string {
	result := make([]string, 0, len(keys))
	for k := range keys {
		result = append(result, k)
	}
	sort.Strings(result)
	return result
}

// round2 将浮点数四舍五入保留两位小数。
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// normalizeLabel 标准化标签
func normalizeLabel(label string, labelMap map[string]string) string {
	if mapped, ok := labelMap[label]; ok {
		return mapped
	}
	return label
}
