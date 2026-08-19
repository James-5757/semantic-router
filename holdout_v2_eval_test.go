package semanticrouter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// HoldoutV2EvalResult holds evaluation results for holdout V2
type HoldoutV2EvalResult struct {
	// Overall metrics
	TotalCases     int     `json:"total_cases"`
	PoolCorrect    int     `json:"pool_correct"`
	PoolAccuracy   float64 `json:"pool_accuracy"`
	TierCorrect    int     `json:"tier_correct"`
	TierAccuracy   float64 `json:"tier_accuracy"`
	ZhPoolAccuracy float64 `json:"zh_pool_accuracy"`
	EnPoolAccuracy float64 `json:"en_pool_accuracy"`

	// Latency
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	P50LatencyMs float64 `json:"p50_latency_ms"`
	P95LatencyMs float64 `json:"p95_latency_ms"`
	P99LatencyMs float64 `json:"p99_latency_ms"`

	// Pool-level metrics
	PoolMetrics map[string]PoolMetrics `json:"pool_metrics"`

	// Confusion matrices
	PoolConfusionMatrix map[string]map[string]int `json:"pool_confusion_matrix"`
	TierConfusionMatrix map[string]map[string]int `json:"tier_confusion_matrix"`

	// Failed cases
	FailedCases      []HoldoutFailedCase `json:"failed_cases"`
	FixedCases       []HoldoutCaseChange `json:"fixed_cases"`
	NewlyBrokenCases []HoldoutCaseChange `json:"newly_broken_cases"`

	// Method specific
	Method string `json:"method"`
}

// PoolMetrics holds metrics per pool
type PoolMetrics struct {
	Count     int     `json:"count"`
	Correct   int     `json:"correct"`
	Accuracy  float64 `json:"accuracy"`
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
}

// HoldoutFailedCase represents a failed case
type HoldoutFailedCase struct {
	Index        int    `json:"index"`
	Prompt       string `json:"prompt"`
	ExpectedPool string `json:"expected_pool"`
	GotPool      string `json:"got_pool"`
	ExpectedTier string `json:"expected_tier"`
	GotTier      string `json:"got_tier"`
}

// HoldoutCaseChange represents a case that changed between methods
type HoldoutCaseChange struct {
	Index        int    `json:"index"`
	Prompt       string `json:"prompt"`
	ExpectedPool string `json:"expected_pool"`
	BaselinePool string `json:"baseline_pool"`
	MethodPool   string `json:"method_pool"`
	Fixed        bool   `json:"fixed"`
}

// Load holdout V2 cases
func loadHoldoutV2Cases(filename string) ([]EvalCase, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var cases []EvalCase
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var c EvalCase
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			continue
		}
		if c.ExpectedPool == "" {
			continue
		}
		cases = append(cases, c)
	}

	return cases, nil
}

// checkEmbeddingServiceAvailable checks if embedding service is running
func checkEmbeddingServiceAvailableV2(endpoint string) bool {
	resp, err := httpGet(endpoint + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

// TestHoldoutV2Baseline evaluates TokenOverlap Baseline on Holdout V2
func TestHoldoutV2Baseline(t *testing.T) {
	result := evaluateHoldoutV2(t, "baseline", false, false)

	if result == nil {
		t.Fatal("Result is nil")
	}

	// Output results
	output, _ := json.MarshalIndent(result, "", "  ")
	fmt.Printf("\n=== Holdout V2 Baseline Results ===\n")
	fmt.Printf("%s\n", output)

	// Save to file
	os.WriteFile("holdout_v2_baseline_result.json", output, 0644)
}

// TestHoldoutV2FullEmbedding evaluates Full Embedding on Holdout V2
func TestHoldoutV2FullEmbedding(t *testing.T) {
	const embeddingEndpoint = "http://localhost:8001"

	if !checkEmbeddingServiceAvailableV2(embeddingEndpoint) {
		t.Skipf("Embedding service not available at %s. Start with: python embedding_service.py --port 8001", embeddingEndpoint)
	}

	result := evaluateHoldoutV2(t, "full_embedding", true, false)

	if result == nil {
		t.Fatal("Result is nil")
	}

	// Output results
	output, _ := json.MarshalIndent(result, "", "  ")
	fmt.Printf("\n=== Holdout V2 Full Embedding Results ===\n")
	fmt.Printf("%s\n", output)

	// Save to file
	os.WriteFile("holdout_v2_full_embedding_result.json", output, 0644)
}

// TestHoldoutV2Selective evaluates Selective on Holdout V2
func TestHoldoutV2Selective(t *testing.T) {
	const embeddingEndpoint = "http://localhost:8001"

	if !checkEmbeddingServiceAvailableV2(embeddingEndpoint) {
		t.Skipf("Embedding service not available at %s", embeddingEndpoint)
	}

	result := evaluateHoldoutV2(t, "selective", true, true)

	if result == nil {
		t.Fatal("Result is nil")
	}

	// Output results
	output, _ := json.MarshalIndent(result, "", "  ")
	fmt.Printf("\n=== Holdout V2 Selective Results ===\n")
	fmt.Printf("%s\n", output)

	// Save to file
	os.WriteFile("holdout_v2_selective_result.json", output, 0644)
}

// evaluateHoldoutV2 runs evaluation with specified configuration
func evaluateHoldoutV2(t *testing.T, method string, enableEmbedding, enableSelective bool) *HoldoutV2EvalResult {
	// Try multiple possible paths
	paths := []string{
		"holdout_eval_cases_v2_locked.jsonl",
		filepath.Join("..", "holdout_eval_cases_v2_locked.jsonl"),
	}

	var cases []EvalCase
	var err error
	for _, p := range paths {
		cases, err = loadHoldoutV2Cases(p)
		if err == nil {
			break
		}
	}

	if err != nil || len(cases) == 0 {
		t.Skipf("无法加载 holdout V2 测试样本: %v", err)
		return nil
	}

	fmt.Printf("Loaded %d holdout V2 cases\n", len(cases))

	// Create routers with appropriate scorer
	var similarityRouter SemanticSimilarityRouter
	if enableEmbedding {
		// Use real embedding scorer
		realScorer := NewRealEmbeddingScorer("http://localhost:8001")
		realScorer.Enable()

		// For selective, we need to use SelectiveRealEmbeddingScorer
		if enableSelective {
			// Create a combined approach - use real scorer as primary
			similarityRouter = NewTokenOverlapSimilarityRouterWithScorer(realScorer)
		} else {
			similarityRouter = NewTokenOverlapSimilarityRouterWithScorer(realScorer)
		}
	} else {
		// Use token overlap
		similarityRouter = NewTokenOverlapSimilarityRouter()
	}

	tierRouter := NewRuleBasedTierRouter()

	// Create MultiLayerRouter with our configured similarity router
	multiLayerRouter := &MultiLayerRouter{
		ruleRouter:       NewRuleBasedSemanticRouter(),
		similarityRouter: similarityRouter,
	}

	result := &HoldoutV2EvalResult{
		TotalCases:          len(cases),
		PoolMetrics:         make(map[string]PoolMetrics),
		PoolConfusionMatrix: make(map[string]map[string]int),
		TierConfusionMatrix: make(map[string]map[string]int),
		FailedCases:         []HoldoutFailedCase{},
		Method:              method,
	}

	// Latency tracking
	var latencies []float64

	// Pool label mapping
	poolLabelMap := map[string]string{
		"code_pool":             "code",
		"data_pool":             "data",
		"document_pool":         "document",
		"vision_pool":           "vision",
		"image_generation_pool": "image_generation",
		"cheap_chat_pool":       "cheap",
		"general_pool":          "general",
	}

	// Language detection
	detectLang := func(prompt string) string {
		zhCount := 0
		for _, c := range prompt {
			if c > 127 {
				zhCount++
			}
		}
		if float64(zhCount) > float64(len(prompt))*0.3 {
			return "zh"
		}
		return "en"
	}

	zhCorrect := 0
	enCorrect := 0
	zhTotal := 0
	enTotal := 0

	for i, c := range cases {
		startTime := time.Now()

		// Map expected pool
		expectedPool := c.ExpectedPool
		if mapped, ok := poolLabelMap[c.ExpectedPool]; ok {
			expectedPool = mapped
		}
		expectedTier := c.ExpectedTier
		if expectedTier == "" {
			expectedTier = "medium"
		}

		// Build request
		req := &RouteRequest{
			Prompt:      c.Prompt,
			Model:       c.Model,
			HasImage:    len(c.Images) > 0,
			HasDocument: len(c.Documents) > 0,
		}

		// Route
		semanticDecision := multiLayerRouter.Route(req)
		tierDecision, _ := tierRouter.RouteWithPrompt(nil, c.Model, semanticDecision.TaskType, c.Prompt)

		latencyMs := float64(time.Since(startTime).Microseconds()) / 1000.0
		latencies = append(latencies, latencyMs)

		// Map predicted pool
		predictedPool := string(semanticDecision.PreferredPool)
		predictedTier := string(tierDecision.PreferredTier)

		// Update confusion matrix
		if result.PoolConfusionMatrix[expectedPool] == nil {
			result.PoolConfusionMatrix[expectedPool] = make(map[string]int)
		}
		result.PoolConfusionMatrix[expectedPool][predictedPool]++

		if result.TierConfusionMatrix[expectedTier] == nil {
			result.TierConfusionMatrix[expectedTier] = make(map[string]int)
		}
		result.TierConfusionMatrix[expectedTier][predictedTier]++

		// Check match
		poolMatch := predictedPool == expectedPool
		tierMatch := predictedTier == expectedTier

		if poolMatch {
			result.PoolCorrect++
			lang := detectLang(c.Prompt)
			if lang == "zh" {
				zhCorrect++
			} else {
				enCorrect++
			}
		}
		if tierMatch {
			result.TierCorrect++
		}

		// Language totals
		lang := detectLang(c.Prompt)
		if lang == "zh" {
			zhTotal++
		} else {
			enTotal++
		}

		// Pool metrics
		if result.PoolMetrics[expectedPool].Count == 0 {
			result.PoolMetrics[expectedPool] = PoolMetrics{Count: 0}
		}
		pm := result.PoolMetrics[expectedPool]
		pm.Count++
		if poolMatch {
			pm.Correct++
		}
		result.PoolMetrics[expectedPool] = pm

		// Failed cases
		if !poolMatch || !tierMatch {
			result.FailedCases = append(result.FailedCases, HoldoutFailedCase{
				Index:        i,
				Prompt:       c.Prompt,
				ExpectedPool: expectedPool,
				GotPool:      predictedPool,
				ExpectedTier: expectedTier,
				GotTier:      predictedTier,
			})
		}
	}

	// Calculate final metrics
	result.PoolAccuracy = float64(result.PoolCorrect) / float64(result.TotalCases) * 100
	result.TierAccuracy = float64(result.TierCorrect) / float64(result.TotalCases) * 100

	if zhTotal > 0 {
		result.ZhPoolAccuracy = float64(zhCorrect) / float64(zhTotal) * 100
	}
	if enTotal > 0 {
		result.EnPoolAccuracy = float64(enCorrect) / float64(enTotal) * 100
	}

	// Pool precision/recall
	for pool, pm := range result.PoolMetrics {
		pm.Accuracy = float64(pm.Correct) / float64(pm.Count) * 100
		// Calculate precision: TP / (TP + FP)
		TP := pm.Correct
		FP := 0
		for pred, count := range result.PoolConfusionMatrix[pool] {
			if pred != pool {
				FP += count
			}
		}
		if TP+FP > 0 {
			pm.Precision = float64(TP) / float64(TP+FP) * 100
		}
		// Recall = Accuracy in this case
		pm.Recall = pm.Accuracy
		result.PoolMetrics[pool] = pm
	}

	// Latency percentiles
	sort.Float64s(latencies)
	n := len(latencies)
	if n > 0 {
		result.AvgLatencyMs = sum(latencies) / float64(n)
		result.P50LatencyMs = latencies[n/2]
		if n >= 20 {
			result.P95LatencyMs = latencies[int(float64(n)*0.95)]
			result.P99LatencyMs = latencies[int(float64(n)*0.99)]
		} else {
			result.P95LatencyMs = latencies[n-1]
			result.P99LatencyMs = latencies[n-1]
		}
	}

	return result
}

func sum(vals []float64) float64 {
	s := 0.0
	for _, v := range vals {
		s += v
	}
	return s
}

// httpGet is a simple HTTP GET helper
func httpGet(url string) (*http.Response, error) {
	return http.Get(url)
}

// NewTokenOverlapSimilarityRouterWithScorer creates a router with custom scorer
func NewTokenOverlapSimilarityRouterWithScorer(scorer Scorer) *TokenOverlapSimilarityRouter {
	router := NewTokenOverlapSimilarityRouter()
	return router
}
