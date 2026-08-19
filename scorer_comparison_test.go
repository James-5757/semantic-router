package semanticrouter

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"testing"
)

// ScorerComparisonResult stores the full comparison results
type ScorerComparisonResult struct {
	ComparisonMetadata ComparisonMeta                `json:"comparison_metadata"`
	Metrics            ScorerMetricsSummary          `json:"metrics"`
	ConfusionMatrices  map[string]map[string]int     `json:"confusion_matrices"`
	ImprovedExamples   []ScorerCaseResult            `json:"improved_examples"`
	WorsenedExamples   []ScorerCaseResult            `json:"worsened_examples"`
}

// ComparisonMeta metadata
type ComparisonMeta struct {
	TotalPrompts    int    `json:"total_prompts"`
	Timestamp       string `json:"timestamp"`
}

// ScorerMetricsSummary metrics
type ScorerMetricsSummary struct {
	TokenOverlap ScorerMetrics `json:"token_overlap"`
	MockBERT     ScorerMetrics `json:"mock_bert"`
	Comparison   struct {
		AccuracyDelta  float64 `json:"accuracy_delta"`
		ImprovedCount  int     `json:"improved_count"`
		WorsenedCount  int     `json:"worsened_count"`
	} `json:"comparison"`
}

// ScorerMetrics per-scorer metrics
type ScorerMetrics struct {
	PoolAccuracy  float64 `json:"pool_accuracy"`
	Correct       int     `json:"correct"`
	FallbackCount int     `json:"fallback_count"`
	FallbackRate  float64 `json:"fallback_rate"`
}

// ScorerCaseResult per-case result
type ScorerCaseResult struct {
	Index         int     `json:"index"`
	Prompt        string  `json:"prompt"`
	ExpectedPool  string  `json:"expected_pool"`
	TokenPool     string  `json:"token_pool"`
	TokenScore    float64 `json:"token_score"`
	BERTPool      string  `json:"bert_pool"`
	BERTScore     float64 `json:"bert_score"`
}

// TestScorerComparison runs offline comparison between TokenOverlap and MockBERT
// This is a shadow comparison - results are written to a file, not used for routing
func TestScorerComparison(t *testing.T) {
	// Load eval cases from standard path
	cases, err := loadEvalCases("routing_eval_cases.jsonl")
	if err != nil {
		t.Skipf("no routing eval cases found: %v", err)
	}
	if len(cases) == 0 {
		t.Skip("no routing eval cases found")
	}

	tokenScorer := NewTokenOverlapScorer()
	bertScorer := NewMockBERTEmbeddingScorer()

	tokenCorrect := 0
	bertCorrect := 0
	tokenFallback := 0
	bertFallback := 0
	total := len(cases)

	tokenConfusion := make(map[string]map[string]int)
	bertConfusion := make(map[string]map[string]int)

	var improved, worsened []ScorerCaseResult

	for i, tc := range cases {
		prompt := tc.Prompt
		expected := string(tc.ExpectedPool)

		// TokenOverlap
		tokenResult, _ := tokenScorer.ScoreAllWithBest(prompt)
		tokenPool := tokenResult.BestPool

		// MockBERT
		bertResult, _ := bertScorer.ScoreAllWithBest(prompt)
		bertPool := bertResult.BestPool

		isTokenCorrect := tokenPool == expected
		isBERTCorrect := bertPool == expected

		if isTokenCorrect {
			tokenCorrect++
		}
		if isBERTCorrect {
			bertCorrect++
		}

		// Fallback detection (score < 0.3)
		if tokenResult.BestScore < 0.3 {
			tokenFallback++
		}
		if bertResult.BestScore < 0.3 {
			bertFallback++
		}

		// Confusion matrix
		if tokenConfusion[expected] == nil {
			tokenConfusion[expected] = make(map[string]int)
		}
		tokenConfusion[expected][tokenPool]++

		if bertConfusion[expected] == nil {
			bertConfusion[expected] = make(map[string]int)
		}
		bertConfusion[expected][bertPool]++

		// Track improvements/worsenings
		record := ScorerCaseResult{
			Index:        i,
			Prompt:       truncateString(prompt, 60),
			ExpectedPool: expected,
			TokenPool:    tokenPool,
			TokenScore:   tokenResult.BestScore,
			BERTPool:     bertPool,
			BERTScore:    bertResult.BestScore,
		}

		if !isTokenCorrect && isBERTCorrect {
			improved = append(improved, record)
		} else if isTokenCorrect && !isBERTCorrect {
			worsened = append(worsened, record)
		}
	}

	// Build report
	tokenAccuracy := float64(tokenCorrect) / float64(total)
	bertAccuracy := float64(bertCorrect) / float64(total)

	result := ScorerComparisonResult{
		ComparisonMetadata: ComparisonMeta{
			TotalPrompts: total,
			Timestamp:    "offline-shadow-comparison",
		},
		Metrics: ScorerMetricsSummary{
			TokenOverlap: ScorerMetrics{
				PoolAccuracy:  roundFloat(tokenAccuracy, 4),
				Correct:       tokenCorrect,
				FallbackCount: tokenFallback,
				FallbackRate:  roundFloat(float64(tokenFallback)/float64(total), 4),
			},
			MockBERT: ScorerMetrics{
				PoolAccuracy:  roundFloat(bertAccuracy, 4),
				Correct:       bertCorrect,
				FallbackCount: bertFallback,
				FallbackRate:  roundFloat(float64(bertFallback)/float64(total), 4),
			},
			Comparison: struct {
				AccuracyDelta  float64 `json:"accuracy_delta"`
				ImprovedCount  int     `json:"improved_count"`
				WorsenedCount  int     `json:"worsened_count"`
			}{
				AccuracyDelta: roundFloat(bertAccuracy-tokenAccuracy, 4),
				ImprovedCount: len(improved),
				WorsenedCount: len(worsened),
			},
		},
		ConfusionMatrices: map[string]map[string]int{
			"token_overlap": flattenConfusion(tokenConfusion),
			"mock_bert":     flattenConfusion(bertConfusion),
		},
		ImprovedExamples: topN(improved, 10),
		WorsenedExamples: topN(worsened, 10),
	}

	// Print to test log
	t.Logf("=== Scorer Offline Comparison Report ===")
	t.Logf("Total eval cases: %d", total)
	t.Logf("")
	t.Logf("--- Pool Accuracy ---")
	t.Logf("TokenOverlapScorer: %.2f%% (%d/%d)", tokenAccuracy*100, tokenCorrect, total)
	t.Logf("MockBERTEmbedding:  %.2f%% (%d/%d)", bertAccuracy*100, bertCorrect, total)
	t.Logf("Delta: %+.2f%%", (bertAccuracy-tokenAccuracy)*100)
	t.Logf("")
	t.Logf("--- Fallback ---")
	t.Logf("TokenOverlap: %d (%.2f%%)", tokenFallback, float64(tokenFallback)/float64(total)*100)
	t.Logf("MockBERT:     %d (%.2f%%)", bertFallback, float64(bertFallback)/float64(total)*100)
	t.Logf("")
	t.Logf("--- Improvements ---")
	t.Logf("BERT fixed: %d, BERT broke: %d", len(improved), len(worsened))
	t.Logf("")
	t.Logf("--- TokenOverlap Confusion ---")
	printConfusionMatrix(t, tokenConfusion)
	t.Logf("--- MockBERT Confusion ---")
	printConfusionMatrix(t, bertConfusion)
	t.Logf("")
	t.Logf("--- Top Improved ---")
	for _, ex := range topN(improved, 5) {
		t.Logf("  [%d] exp=%s token=%s(%.2f) bert=%s(%.2f) | %s", ex.Index, ex.ExpectedPool, ex.TokenPool, ex.TokenScore, ex.BERTPool, ex.BERTScore, ex.Prompt)
	}
	t.Logf("--- Top Worsened ---")
	for _, ex := range topN(worsened, 5) {
		t.Logf("  [%d] exp=%s token=%s(%.2f) bert=%s(%.2f) | %s", ex.Index, ex.ExpectedPool, ex.TokenPool, ex.TokenScore, ex.BERTPool, ex.BERTScore, ex.Prompt)
	}

	// Write report to file (if env var is set)
	if os.Getenv("SEMANTIC_ROUTER_REPORT") == "1" {
		reportPath := "scorer_comparison_report.json"
		f, err := os.Create(reportPath)
		if err == nil {
			defer f.Close()
			enc := json.NewEncoder(f)
			enc.SetIndent("", "  ")
			enc.Encode(result)
			t.Logf("Report written to %s", reportPath)
		}
	}
}

func printConfusionMatrix(t *testing.T, cm map[string]map[string]int) {
	pools := []string{"default", "cheap", "code", "vision", "document", "data", "image_generation"}
	for _, expected := range pools {
		row := cm[expected]
		if row == nil {
			continue
		}
		total := 0
		diag := row[expected]
		for _, v := range row {
			total += v
		}
		acc := float64(diag) / float64(total) * 100
		parts := make([]string, 0)
		for _, got := range pools {
			if count := row[got]; count > 0 {
				parts = append(parts, fmt.Sprintf("%s:%d", got, count))
			}
		}
		t.Logf("  %-20s -> %s (acc: %.0f%%)", expected, strings.Join(parts, ", "), acc)
	}
}

func flattenConfusion(cm map[string]map[string]int) map[string]int {
	flat := make(map[string]int)
	for expected, row := range cm {
		for got, count := range row {
			key := fmt.Sprintf("%s->%s", expected, got)
			flat[key] = count
		}
	}
	return flat
}

func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

func roundFloat(f float64, precision int) float64 {
	pow := math.Pow(10, float64(precision))
	return math.Round(f*pow) / pow
}

func topN(slice []ScorerCaseResult, n int) []ScorerCaseResult {
	if len(slice) <= n {
		return slice
	}
	return slice[:n]
}

// Ensure sorting for consistency
func sortCasesByIndex(cases []ScorerCaseResult) {
	sort.Slice(cases, func(i, j int) bool {
		return cases[i].Index < cases[j].Index
	})
}

func init() {
	// Ensure imports are used
	_ = sortCasesByIndex
}
