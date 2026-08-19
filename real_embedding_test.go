package semanticrouter

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"
	"testing"
	"time"
)

// RealEmbeddingComparisonResult 对比结果
type RealEmbeddingComparisonResult struct {
	TotalCases            int                       `json:"total_cases"`
	TokenOverlapAccuracy  float64                   `json:"token_overlap_accuracy"`
	RealEmbeddingAccuracy float64                   `json:"real_embedding_accuracy"`
	Delta                 float64                   `json:"delta"`
	FixedCases            []string                  `json:"fixed_cases"`
	BrokenCases           []string                  `json:"broken_cases"`
	TokenOverlapMatrix    map[string]map[string]int `json:"token_overlap_confusion_matrix"`
	RealEmbeddingMatrix   map[string]map[string]int `json:"real_embedding_confusion_matrix"`
	LatencyAvg            float64                   `json:"latency_avg_ms"`
	LatencyP95            float64                   `json:"latency_p95_ms"`
	ChineseAccuracy       float64                   `json:"chinese_accuracy"`
	EnglishAccuracy       float64                   `json:"english_accuracy"`
}

// checkEmbeddingServiceAvailable 检查 Python embedding service 是否可用
func checkEmbeddingServiceAvailable(endpoint string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(endpoint + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// TestRealEmbeddingComparison 真实 Embedding 与 TokenOverlap 离线对比
// 需要先启动 Python embedding service:
//   python embedding_service.py --model-path ./models/multilingual-e5-small --port 8001
func TestRealEmbeddingComparison(t *testing.T) {
	const embeddingEndpoint = "http://localhost:8001"
	const evalFile = "routing_eval_cases.jsonl"

	// 检查服务是否可用
	if !checkEmbeddingServiceAvailable(embeddingEndpoint) {
		t.Skipf("Embedding service not available at %s. Start with: python embedding_service.py --port 8001", embeddingEndpoint)
	}

	// 创建 scorers
	tokenScorer := NewTokenOverlapScorer()
	realScorer := NewRealEmbeddingScorer(embeddingEndpoint)
	realScorer.Enable()

	// 读取 eval cases
	cases, err := loadEvalCasesFromFile(evalFile)
	if err != nil {
		t.Fatalf("Failed to load eval cases: %v", err)
	}

	// 运行对比
	result := runComparisonWithInterface(cases, tokenScorer, realScorer)

	// 打印结果
	fmt.Printf("\n============================================================\n")
	fmt.Printf("  Real Embedding vs TokenOverlap Comparison\n")
	fmt.Printf("============================================================\n")
	fmt.Printf("  Total cases: %d\n", result.TotalCases)
	fmt.Printf("\n--- Accuracy ---\n")
	fmt.Printf("  TokenOverlap:  %.2f%%\n", result.TokenOverlapAccuracy*100)
	fmt.Printf("  RealEmbedding: %.2f%%\n", result.RealEmbeddingAccuracy*100)
	fmt.Printf("  Delta:         %.2f%%\n", result.Delta*100)
	fmt.Printf("\n--- Language ---\n")
	fmt.Printf("  Chinese Accuracy:  %.2f%%\n", result.ChineseAccuracy*100)
	fmt.Printf("  English Accuracy:  %.2f%%\n", result.EnglishAccuracy*100)
	fmt.Printf("\n--- Latency ---\n")
	fmt.Printf("  Average: %.2f ms\n", result.LatencyAvg)
	fmt.Printf("  P95:     %.2f ms\n", result.LatencyP95)
	fmt.Printf("\n--- Case Changes ---\n")
	fmt.Printf("  Fixed (Real better): %d\n", len(result.FixedCases))
	fmt.Printf("  Broken (Real worse): %d\n", len(result.BrokenCases))
	fmt.Printf("\n============================================================\n")

	// 保存结果到文件
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	_ = os.WriteFile("real_embedding_comparison_result.json", resultJSON, 0644)

	// 验证至少不是更差
	if result.RealEmbeddingAccuracy < result.TokenOverlapAccuracy-0.05 {
		t.Errorf("RealEmbedding significantly worse than TokenOverlap: %.2f%% vs %.2f%%",
			result.RealEmbeddingAccuracy*100, result.TokenOverlapAccuracy*100)
	}
}

// loadEvalCasesFromFile 加载 eval cases (避免与 eval_test.go 冲突)
func loadEvalCasesFromFile(filename string) ([]EvalCase, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var cases []EvalCase
	decoder := json.NewDecoder(file)
	for {
		var ec EvalCase
		if err := decoder.Decode(&ec); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		cases = append(cases, ec)
	}
	return cases, nil
}

// runComparisonWithInterface 使用 Scorer 接口运行对比
func runComparisonWithInterface(cases []EvalCase, tokenScorer, realScorer Scorer) *RealEmbeddingComparisonResult {
	var wg sync.WaitGroup
	var mu sync.Mutex

	tokenPoolCorrect := 0
	realPoolCorrect := 0
	latencies := []float64{}
	fixedCases := []string{}
	brokenCases := []string{}

	tokenMatrix := make(map[string]map[string]int)
	realMatrix := make(map[string]map[string]int)

	isChinese := func(prompt string) bool {
		for _, r := range prompt {
			if r > 127 {
				return true
			}
		}
		return false
	}

	// 初始化 matrix
	pools := []string{"code", "data", "document", "vision", "image_generation", "cheap_chat", "default"}
	for _, p := range pools {
		tokenMatrix[p] = make(map[string]int)
		realMatrix[p] = make(map[string]int)
		for _, pp := range pools {
			tokenMatrix[p][pp] = 0
			realMatrix[p][pp] = 0
		}
	}

	normalizePool := func(pool string) string {
		switch pool {
		case "code_pool", "code":
			return "code"
		case "data_pool", "data":
			return "data"
		case "document_pool", "document":
			return "document"
		case "vision_pool", "vision":
			return "vision"
		case "image_generation_pool", "image_generation":
			return "image_generation"
		case "cheap_chat_pool", "cheap_chat":
			return "cheap_chat"
		default:
			return "default"
		}
	}

	for i, c := range cases {
		wg.Add(1)
		go func(idx int, ec EvalCase) {
			defer wg.Done()

			expectedPool := normalizePool(ec.ExpectedPool)

			// TokenOverlap scorer - 使用 ScoreAllWithBest
			tokenResult, err := tokenScorer.ScoreAllWithBest(ec.Prompt)
			var tokenPool string
			if err == nil && tokenResult != nil {
				tokenPool = normalizePool(tokenResult.BestPool)
			} else {
				tokenPool = "default"
			}

			// RealEmbedding scorer
			startTime := time.Now()
			realResult, err := realScorer.ScoreAllWithBest(ec.Prompt)
			latency := time.Since(startTime).Milliseconds()

			var realPool string
			if err == nil && realResult != nil {
				realPool = normalizePool(realResult.BestPool)
			} else {
				realPool = "default"
			}

			mu.Lock()
			// TokenOverlap 正确性
			if tokenPool == expectedPool {
				tokenPoolCorrect++
			}
			tokenMatrix[expectedPool][tokenPool]++

			// RealEmbedding 正确性
			if realPool == expectedPool {
				realPoolCorrect++
			}
			realMatrix[expectedPool][realPool]++

			// 记录延迟
			latencies = append(latencies, float64(latency))

			// 记录 fixed/broken cases
			if tokenPool != expectedPool && realPool == expectedPool {
				fixedCases = append(fixedCases, fmt.Sprintf("[%d] %s", idx, ec.Prompt[:minInt(50, len(ec.Prompt))]))
			}
			if tokenPool == expectedPool && realPool != expectedPool {
				brokenCases = append(brokenCases, fmt.Sprintf("[%d] %s", idx, ec.Prompt[:minInt(50, len(ec.Prompt))]))
			}
			mu.Unlock()
		}(i, c)
	}

	wg.Wait()

	// 计算语言准确率
	chineseCorrect, chineseTotal := 0, 0
	englishCorrect, englishTotal := 0, 0

	for _, c := range cases {
		expectedPool := normalizePool(c.ExpectedPool)

		realResult, _ := realScorer.ScoreAllWithBest(c.Prompt)
		realPool := "default"
		if realResult != nil {
			realPool = normalizePool(realResult.BestPool)
		}

		isZh := isChinese(c.Prompt)
		if isZh {
			chineseTotal++
			if realPool == expectedPool {
				chineseCorrect++
			}
		} else {
			englishTotal++
			if realPool == expectedPool {
				englishCorrect++
			}
		}
	}

	// 计算 P95 延迟
	sort.Float64s(latencies)
	var p95Latency float64
	if len(latencies) > 0 {
		p95Idx := int(float64(len(latencies)) * 0.95)
		if p95Idx >= len(latencies) {
			p95Idx = len(latencies) - 1
		}
		p95Latency = latencies[p95Idx]
	}

	// 计算平均延迟
	var totalLatency float64
	for _, l := range latencies {
		totalLatency += l
	}
	avgLatency := totalLatency / float64(len(latencies))

	return &RealEmbeddingComparisonResult{
		TotalCases:            len(cases),
		TokenOverlapAccuracy:  float64(tokenPoolCorrect) / float64(len(cases)),
		RealEmbeddingAccuracy: float64(realPoolCorrect) / float64(len(cases)),
		Delta:                 float64(realPoolCorrect-tokenPoolCorrect) / float64(len(cases)),
		FixedCases:            fixedCases,
		BrokenCases:           brokenCases,
		TokenOverlapMatrix:    tokenMatrix,
		RealEmbeddingMatrix:   realMatrix,
		LatencyAvg:            avgLatency,
		LatencyP95:            p95Latency,
		ChineseAccuracy:       float64(chineseCorrect) / float64(chineseTotal),
		EnglishAccuracy:       float64(englishCorrect) / float64(englishTotal),
	}
}

// minInt returns the minimum of two integers
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// checkForHoldoutFile 检查是否有 holdout eval file
func checkForHoldoutFile() string {
	holdoutFiles := []string{
		"holdout_eval_cases.jsonl",
		"routing_eval_cases_v1_locked.jsonl",
	}
	for _, f := range holdoutFiles {
		if _, err := os.Stat(f); err == nil {
			return f
		}
	}
	return ""
}

// TestRealEmbeddingOnHoldout 在 holdout 数据集上测试
func TestRealEmbeddingOnHoldout(t *testing.T) {
	const embeddingEndpoint = "http://localhost:8001"

	holdoutFile := checkForHoldoutFile()
	if holdoutFile == "" {
		t.Skip("No holdout eval file found")
	}

	if !checkEmbeddingServiceAvailable(embeddingEndpoint) {
		t.Skipf("Embedding service not available at %s", embeddingEndpoint)
	}

	tokenScorer := NewTokenOverlapScorer()
	realScorer := NewRealEmbeddingScorer(embeddingEndpoint)
	realScorer.Enable()

	cases, err := loadEvalCasesFromFile(holdoutFile)
	if err != nil {
		t.Fatalf("Failed to load holdout cases: %v", err)
	}

	result := runComparisonWithInterface(cases, tokenScorer, realScorer)

	fmt.Printf("\n============================================================\n")
	fmt.Printf("  Holdout Dataset: %s\n", holdoutFile)
	fmt.Printf("============================================================\n")
	fmt.Printf("  Total cases: %d\n", result.TotalCases)
	fmt.Printf("  TokenOverlap:  %.2f%%\n", result.TokenOverlapAccuracy*100)
	fmt.Printf("  RealEmbedding: %.2f%%\n", result.RealEmbeddingAccuracy*100)
	fmt.Printf("  Delta:         %.2f%%\n", result.Delta*100)
	fmt.Printf("============================================================\n")

	// 保存 holdout 结果
	holdoutResultFile := "holdout_comparison_result.json"
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	_ = os.WriteFile(holdoutResultFile, resultJSON, 0644)
	t.Logf("Holdout result saved to %s", holdoutResultFile)
}