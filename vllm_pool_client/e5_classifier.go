package vllm_pool_client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// =============================================================================
// E5PrototypeSemanticClassifier 基于 E5 Embedding 的原型语义分类器
// 使用 multilingual-e5-small 计算 prompt 与各类别 prototype 的语义相似度
// =============================================================================

// E5PrototypeSemanticClassifier E5 原型语义分类器
type E5PrototypeSemanticClassifier struct {
	config          E5ClassifierConfig
	prototypeData   *PrototypeData
	prototypeCache  map[string][][]float32 // category -> embeddings
	httpClient      *http.Client
	mu              sync.RWMutex
}

// E5ClassifierConfig E5 分类器配置
type E5ClassifierConfig struct {
	Enabled           bool          `json:"enabled"`
	EmbeddingURL      string        `json:"embedding_url"`       // embedding 服务 URL
	Timeout           time.Duration `json:"timeout"`             // 请求超时
	PrototypePath     string        `json:"prototype_path"`      // prototype 数据路径
	TopKPerCategory   int           `json:"top_k_per_category"`  // 每个类别使用的 prototype 数量
	SimilarityThreshold float64     `json:"similarity_threshold"`// 相似度阈值
	AmbiguousMargin   float64       `json:"ambiguous_margin"`    // ambiguous 边界阈值
}

// E5ClassifyResult E5 分类结果
type E5ClassifyResult struct {
	Category         string              `json:"category"`
	Confidence       float64             `json:"confidence"`
	Scores           map[string]float64  `json:"scores"`
	TopK             []CategoryScore     `json:"top_k"`
	Ambiguous        bool                `json:"ambiguous"`
	MatchedSignals   []string            `json:"matched_signals"`
	E5ServiceUsed    bool                `json:"e5_service_used"`
}

// NewE5PrototypeSemanticClassifier 创建 E5 分类器
func NewE5PrototypeSemanticClassifier(config E5ClassifierConfig) (*E5PrototypeSemanticClassifier, error) {
	c := &E5PrototypeSemanticClassifier{
		config:         config,
		prototypeCache: make(map[string][][]float32),
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}

	// 如果启用，加载 prototype 数据
	if config.Enabled {
		// 尝试加载原型数据
		if config.PrototypePath != "" {
			if err := c.loadPrototypeData(config.PrototypePath); err != nil {
				return nil, err
			}
		} else {
			// 使用默认路径
			exePath, _ := os.Executable()
			dir := filepath.Dir(exePath)
			defaultPath := filepath.Join(dir, "vllm_pool_client", "prototypes.json")
			if err := c.loadPrototypeData(defaultPath); err != nil {
				// 尝试当前目录
				if err := c.loadPrototypeData("prototypes.json"); err != nil {
					return nil, err
				}
			}
		}
	}

	return c, nil
}

// loadPrototypeData 加载原型数据
func (c *E5PrototypeSemanticClassifier) loadPrototypeData(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var prototypes PrototypeData
	if err := json.Unmarshal(data, &prototypes); err != nil {
		return err
	}

	c.prototypeData = &prototypes
	return nil
}

// Classify 执行 E5 原型语义分类
func (c *E5PrototypeSemanticClassifier) Classify(prompt string) *E5ClassifyResult {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.prototypeData == nil {
		return &E5ClassifyResult{
			Category:       "general",
			Confidence:     0.5,
			E5ServiceUsed:  false,
			MatchedSignals: []string{"error:no_prototype_data"},
		}
	}

	promptLower := strings.ToLower(prompt)
	scores := make(map[string]float64)

	// 计算每个类别的分数
	for name, proto := range c.prototypeData.Categories {
		// 获取该类别的 prototype
		prototypes := c.getCategoryPrototypes(proto)
		
		if len(prototypes) == 0 {
			scores[name] = 0.0
			continue
		}

		// 计算与每个 prototype 的相似度
		score := c.calculateCategoryScore(prompt, prototypes)
		scores[name] = score
	}

	// 排序获取 Top-K
	topK := c.getTopK(scores, 5)

	// 判断 ambiguous
	ambiguous := c.isAmbiguous(scores)

	// 置信度和类别
	var category string
	var confidence float64
	if len(topK) > 0 && topK[0].Score > 0 {
		category = topK[0].Category
		confidence = topK[0].Score
	} else {
		category = "general"
		confidence = 0.5
	}

	// 收集匹配的信号
	matchedSignals := c.collectMatchedSignals(prompt, promptLower)

	return &E5ClassifyResult{
		Category:       category,
		Confidence:     confidence,
		Scores:         scores,
		TopK:           topK,
		Ambiguous:      ambiguous,
		MatchedSignals: matchedSignals,
		E5ServiceUsed:  true,
	}
}

// getCategoryPrototypes 获取类别的 prototype 文本列表
func (c *E5PrototypeSemanticClassifier) getCategoryPrototypes(proto CategoryPrototype) []string {
	var prototypes []string
	
	// 添加中文正例
	prototypes = append(prototypes, proto.ChinesePositive...)
	// 添加英文正例
	prototypes = append(prototypes, proto.EnglishPositive...)
	// 添加反例（作为负向 prototype）
	prototypes = append(prototypes, proto.NegativeSamples...)
	
	// 限制数量
	if c.config.TopKPerCategory > 0 && len(prototypes) > c.config.TopKPerCategory {
		prototypes = prototypes[:c.config.TopKPerCategory]
	}
	
	return prototypes
}

// calculateCategoryScore 计算类别分数
// 尝试调用 E5 embedding 服务，如果失败则使用本地简化计算
func (c *E5PrototypeSemanticClassifier) calculateCategoryScore(prompt string, prototypes []string) float64 {
	// 尝试调用 E5 embedding 服务
	promptEmbedding, err := c.getEmbedding(prompt)
	if err != nil || promptEmbedding == nil {
		// 如果失败，使用简化的关键词匹配作为后备
		return c.calculateFallbackScore(prompt, prototypes)
	}

	// 获取 prototypes 的 embeddings
	var prototypeEmbeddings [][]float32
	for _, proto := range prototypes {
		emb, err := c.getEmbedding(proto)
		if err != nil || emb == nil {
			continue
		}
		prototypeEmbeddings = append(prototypeEmbeddings, emb)
	}

	if len(prototypeEmbeddings) == 0 {
		return c.calculateFallbackScore(prompt, prototypes)
	}

	// 计算与每个 prototype 的余弦相似度
	var similarities []float64
	for _, emb := range prototypeEmbeddings {
		sim := cosineSimilarity(promptEmbedding, emb)
		similarities = append(similarities, float64(sim))
	}

	// 使用 Top-K 均值
	topK := len(similarities)
	if topK > 3 {
		topK = 3
	}
	
	// 排序取 Top-K
	sorted := make([]float64, len(similarities))
	copy(sorted, similarities)
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j] > sorted[i] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	var avgScore float64
	for i := 0; i < topK; i++ {
		avgScore += sorted[i]
	}
	avgScore /= float64(topK)

	// 归一化到 0-1 (E5 相似度通常在 0.5-0.9 范围)
	normalized := (avgScore - 0.5) / 0.4
	if normalized < 0 {
		normalized = 0
	}
	if normalized > 1 {
		normalized = 1
	}

	return normalized
}

// getEmbedding 获取文本的 embedding
func (c *E5PrototypeSemanticClassifier) getEmbedding(text string) ([]float32, error) {
	if c.config.EmbeddingURL == "" {
		return nil, fmt.Errorf("embedding URL not configured")
	}

	// 调用 embedding 服务
	reqBody := map[string]interface{}{
		"texts": []string{text},
		"model": "multilingual-e5-small",
	}
	
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Post(
		c.config.EmbeddingURL+"/v1/embeddings",
		"application/json",
		bytes.NewBuffer(jsonBody),
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("embedding service returned status %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// 解析 embedding
	data, ok := result["data"].([]interface{})
	if !ok || len(data) == 0 {
		return nil, fmt.Errorf("invalid embedding response")
	}

	embedding, ok := data[0].(map[string]interface{})["embedding"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid embedding format")
	}

	resultEmbed := make([]float32, len(embedding))
	for i, v := range embedding {
		resultEmbed[i] = float32(v.(float64))
	}

	return resultEmbed, nil
}

// calculateFallbackScore 计算后备分数（当 E5 服务不可用时）
func (c *E5PrototypeSemanticClassifier) calculateFallbackScore(prompt string, prototypes []string) float64 {
	promptLower := strings.ToLower(prompt)
	score := 0.0
	matchCount := 0

	for _, proto := range prototypes {
		protoLower := strings.ToLower(proto)
		if strings.Contains(promptLower, protoLower) {
			score += 0.3
			matchCount++
		}
		// 部分匹配
		words := strings.Fields(protoLower)
		matched := 0
		for _, w := range words {
			if len(w) > 2 && strings.Contains(promptLower, w) {
				matched++
			}
		}
		if matched >= len(words)/2 {
			score += 0.15
		}
	}

	if matchCount > 0 {
		score = 0.3*float64(matchCount) + score*0.5
	}

	return math.Min(score, 1.0)
}

// cosineSimilarity 计算余弦相似度
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct float32
	var normA float32
	var normB float32

	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB))))
}

// getTopK 获取 Top-K 结果
func (c *E5PrototypeSemanticClassifier) getTopK(scores map[string]float64, k int) []CategoryScore {
	type scoredCategory struct {
		category string
		score    float64
	}

	var sorted []scoredCategory
	for cat, score := range scores {
		sorted = append(sorted, scoredCategory{category: cat, score: score})
	}

	// 排序
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].score > sorted[j].score
	})

	// 取 Top-K
	result := make([]CategoryScore, 0, k)
	for i := 0; i < len(sorted) && i < k; i++ {
		if sorted[i].score > 0 {
			result = append(result, CategoryScore{
				Category: sorted[i].category,
				Score:    sorted[i].score,
			})
		}
	}

	return result
}

// isAmbiguous 判断是否 ambiguous
func (c *E5PrototypeSemanticClassifier) isAmbiguous(scores map[string]float64) bool {
	if len(scores) < 2 {
		return false
	}

	// 找到最高分和次高分
	var sorted []float64
	for _, s := range scores {
		sorted = append(sorted, s)
	}
	sort.Float64s(sorted)

	if len(sorted) < 2 || sorted[len(sorted)-1] == 0 {
		return false
	}

	margin := sorted[len(sorted)-1] - sorted[len(sorted)-2]
	return margin < c.config.AmbiguousMargin
}

// collectMatchedSignals 收集匹配的信号
func (c *E5PrototypeSemanticClassifier) collectMatchedSignals(prompt string, promptLower string) []string {
	signals := []string{}

	// 检测语言
	if containsChinese(prompt) {
		signals = append(signals, "language:chinese")
	} else {
		signals = append(signals, "language:english")
	}

	// 检测长度
	if len(prompt) < 50 {
		signals = append(signals, "length:short")
	} else if len(prompt) > 200 {
		signals = append(signals, "length:long")
	} else {
		signals = append(signals, "length:medium")
	}

	// 添加 E5 方法标记
	signals = append(signals, "method:e5_prototype_semantic")

	return signals
}

// IsEnabled 检查是否启用
func (c *E5PrototypeSemanticClassifier) IsEnabled() bool {
	return c.config.Enabled
}