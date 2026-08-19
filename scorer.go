package semanticrouter

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Scorer 接口 - 可插拔的相似度计算器
type Scorer interface {
	// Score 计算 prompt 与 pool 的匹配得分
	Score(prompt string, pool PreferredPool) (float64, []string, error)

	// ScoreAll 计算 prompt 与所有 pool 的匹配得分
	ScoreAll(prompt string) (map[string]float64, error)

	// ScoreAllWithBest 计算所有 pool 的得分并返回最佳 pool 和相关信息
	ScoreAllWithBest(prompt string) (*ScorerResult, error)

	// Name 返回 scorer 名称
	Name() string

	// IsAvailable 检查 scorer 是否可用
	IsAvailable() bool
}

// ScorerResult 完整 scorer 结果
type ScorerResult struct {
	Scores          map[string]float64 `json:"scores"`
	BestPool        string             `json:"best_pool"`
	BestScore       float64            `json:"best_score"`
	SecondBestPool  string             `json:"second_best_pool"`
	SecondBestScore float64            `json:"second_best_score"`
	ScoreMargin     float64            `json:"score_margin"`
	MatchedKeywords []string           `json:"matched_keywords,omitempty"`
	ScorerType      string             `json:"scorer_type"`
}

// ScorerType scorer 类型
type ScorerType string

const (
	ScorerTypeTokenOverlap ScorerType = "token_overlap"
	ScorerTypeBERT         ScorerType = "bert"
)

// ErrBERTNotAvailable BERT 不可用
var ErrBERTNotAvailable = errors.New("BERT embedding service is not available")

// PoolDescriptions 在 similarity_router.go 中定义
// 这里复用 similarity_router.go 中的定义

// NewScorer 创建 scorer 实例
func NewScorer(scorerType ScorerType) (Scorer, error) {
	switch scorerType {
	case ScorerTypeTokenOverlap:
		return NewTokenOverlapScorer(), nil
	case ScorerTypeBERT:
		// 预留 BERT 接口，如果不可用则返回错误
		return nil, ErrBERTNotAvailable
	default:
		return nil, errors.New("unknown scorer type")
	}
}

// TokenOverlapScorer 基于 token overlap 的 scorer
type TokenOverlapScorer struct {
	router *TokenOverlapSimilarityRouter
}

// NewTokenOverlapScorer 创建 token overlap scorer
func NewTokenOverlapScorer() *TokenOverlapScorer {
	return &TokenOverlapScorer{
		router: NewTokenOverlapSimilarityRouter(),
	}
}

// Score 计算 prompt 与 pool 的匹配得分
func (s *TokenOverlapScorer) Score(prompt string, pool PreferredPool) (float64, []string, error) {
	score, keywords := s.router.CalculateKeywordScore(prompt, pool)
	descScore := s.router.CalculateDescriptionSimilarity(prompt, pool)
	totalScore := score*0.7 + descScore*0.3
	return totalScore, keywords, nil
}

// ScoreAll 计算 prompt 与所有 pool 的匹配得分
func (s *TokenOverlapScorer) ScoreAll(prompt string) (map[string]float64, error) {
	scores := make(map[string]float64)
	pools := []PreferredPool{PoolDefault, PoolCheap, PoolCode, PoolVision, PoolDocument, PoolData, PoolPrivate}

	for _, pool := range pools {
		score, _, _ := s.Score(prompt, pool)
		scores[string(pool)] = Round(score)
	}

	return scores, nil
}

// ScoreAllWithBest 实现 Scorer 接口
func (s *TokenOverlapScorer) ScoreAllWithBest(prompt string) (*ScorerResult, error) {
	scores, err := s.ScoreAll(prompt)
	if err != nil {
		return nil, err
	}
	return getScorerResult(scores, s.Name()), nil
}

// Name 返回 scorer 名称
func (s *TokenOverlapScorer) Name() string {
	return "TokenOverlapScorer"
}

// IsAvailable 检查 scorer 是否可用
func (s *TokenOverlapScorer) IsAvailable() bool {
	return true
}

// getScorerResult 从 scores map 构建 ScorerResult
func getScorerResult(scores map[string]float64, scorerName string) *ScorerResult {
	result := &ScorerResult{
		Scores:     scores,
		ScorerType: scorerName,
	}

	bestScore := -1.0
	secondBestScore := -1.0
	bestPool := ""
	secondBestPool := ""

	for pool, score := range scores {
		if score > bestScore {
			secondBestScore = bestScore
			secondBestPool = bestPool
			bestScore = score
			bestPool = pool
		} else if score > secondBestScore {
			secondBestScore = score
			secondBestPool = pool
		}
	}

	result.BestPool = bestPool
	result.BestScore = bestScore
	result.SecondBestPool = secondBestPool
	result.SecondBestScore = secondBestScore

	if secondBestScore >= 0 && bestScore >= 0 {
		result.ScoreMargin = bestScore - secondBestScore
	}

	return result
}

// BERTEmbeddingRequest BERT embedding 请求
type BERTEmbeddingRequest struct {
	Texts []string `json:"texts"`
}

// BERTEmbeddingResponse BERT embedding 响应
type BERTEmbeddingResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
	Error      string      `json:"error,omitempty"`
}

// BERTEmbeddingScorer 基于 BERT embedding 的 scorer（预留接口）
type BERTEmbeddingScorer struct {
	endpoint       string
	enabled        bool
	httpClient     *http.Client
	poolEmbeddings map[PreferredPool][]float64 // 缓存 pool 描述的 embedding
	mu             sync.RWMutex
	timeout        time.Duration
}

// NewBERTEmbeddingScorer 创建 BERT scorer
func NewBERTEmbeddingScorer(endpoint string) *BERTEmbeddingScorer {
	return &BERTEmbeddingScorer{
		endpoint: endpoint,
		enabled:  false, // 默认禁用，需要配置后才能启用
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		poolEmbeddings: make(map[PreferredPool][]float64),
		timeout:        5 * time.Second,
	}
}

// SetEndpoint 设置 BERT 服务端点
func (s *BERTEmbeddingScorer) SetEndpoint(endpoint string) {
	s.endpoint = endpoint
}

// SetTimeout 设置请求超时
func (s *BERTEmbeddingScorer) SetTimeout(timeout time.Duration) {
	s.timeout = timeout
	s.httpClient.Timeout = timeout
}

// Enable 启用 BERT scorer
func (s *BERTEmbeddingScorer) Enable() {
	s.enabled = true
}

// Disable 禁用 BERT scorer
func (s *BERTEmbeddingScorer) Disable() {
	s.enabled = false
}

// EnableWithEndpoint 启用 BERT scorer 并设置端点
func (s *BERTEmbeddingScorer) EnableWithEndpoint(endpoint string) {
	s.endpoint = endpoint
	s.enabled = true
	// 预计算 pool 描述的 embedding
	s.precomputePoolEmbeddings()
}

// precomputePoolEmbeddings 预计算 pool 描述的 embedding
func (s *BERTEmbeddingScorer) precomputePoolEmbeddings() {
	if s.endpoint == "" {
		return
	}

	// 构建 pool 描述列表
	texts := make([]string, 0, len(PoolDescriptions))
	pools := make([]PreferredPool, 0, len(PoolDescriptions))
	for pool, desc := range PoolDescriptions {
		texts = append(texts, desc)
		pools = append(pools, pool)
	}

	// 调用 BERT 服务获取 embedding
	embeddings, err := s.getEmbeddings(texts)
	if err != nil {
		fmt.Printf("BERT scorer: failed to precompute pool embeddings: %v\n", err)
		return
	}

	// 缓存 embedding
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, pool := range pools {
		if i < len(embeddings) {
			s.poolEmbeddings[pool] = embeddings[i]
		}
	}
}

// getEmbeddings 调用 BERT 服务获取 embedding
func (s *BERTEmbeddingScorer) getEmbeddings(texts []string) ([][]float64, error) {
	reqBody, err := json.Marshal(BERTEmbeddingRequest{Texts: texts})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", s.endpoint+"/embed", strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("BERT service returned status %d", resp.StatusCode)
	}

	var result BERTEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if result.Error != "" {
		return nil, errors.New(result.Error)
	}

	return result.Embeddings, nil
}

// cosineSimilarity 计算余弦相似度
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (normA * normB)
}

// Score 计算 prompt 与 pool 的匹配得分
func (s *BERTEmbeddingScorer) Score(prompt string, pool PreferredPool) (float64, []string, error) {
	if !s.enabled {
		return 0, nil, ErrBERTNotAvailable
	}

	if s.endpoint == "" {
		return 0, nil, errors.New("BERT endpoint not configured")
	}

	// 获取 prompt 的 embedding
	embeddings, err := s.getEmbeddings([]string{prompt})
	if err != nil {
		return 0, nil, err
	}
	if len(embeddings) == 0 {
		return 0, nil, errors.New("no embedding returned")
	}

	promptEmbedding := embeddings[0]

	// 获取 pool 描述的 embedding
	s.mu.RLock()
	poolEmbedding, ok := s.poolEmbeddings[pool]
	s.mu.RUnlock()

	if !ok {
		// 如果 pool embedding 不存在，计算并缓存
		desc := PoolDescriptions[pool]
		embeddings, err := s.getEmbeddings([]string{desc})
		if err != nil {
			return 0, nil, err
		}
		if len(embeddings) == 0 {
			return 0, nil, errors.New("no pool embedding returned")
		}
		poolEmbedding = embeddings[0]

		s.mu.Lock()
		s.poolEmbeddings[pool] = poolEmbedding
		s.mu.Unlock()
	}

	// 计算余弦相似度
	score := cosineSimilarity(promptEmbedding, poolEmbedding)
	// 将相似度映射到 0-1 范围
	score = (score + 1) / 2

	return score, nil, nil
}

// ScoreAll 计算 prompt 与所有 pool 的匹配得分
func (s *BERTEmbeddingScorer) ScoreAll(prompt string) (map[string]float64, error) {
	if !s.enabled {
		return nil, ErrBERTNotAvailable
	}

	scores := make(map[string]float64)
	pools := []PreferredPool{PoolDefault, PoolCheap, PoolCode, PoolVision, PoolDocument, PoolData, PoolPrivate}

	for _, pool := range pools {
		score, _, err := s.Score(prompt, pool)
		if err != nil {
			// 如果某个 pool 计算失败，使用 0
			scores[string(pool)] = 0
		} else {
			scores[string(pool)] = Round(score)
		}
	}

	return scores, nil
}

// ScoreAllWithBest 实现 Scorer 接口
func (s *BERTEmbeddingScorer) ScoreAllWithBest(prompt string) (*ScorerResult, error) {
	scores, err := s.ScoreAll(prompt)
	if err != nil {
		return nil, err
	}
	return getScorerResult(scores, s.Name()), nil
}

// Name 返回 scorer 名称
func (s *BERTEmbeddingScorer) Name() string {
	return "BERTEmbeddingScorer"
}

// IsAvailable 检查 scorer 是否可用
func (s *BERTEmbeddingScorer) IsAvailable() bool {
	return s.enabled && s.endpoint != ""
}

// FallbackToTokenOverlap 当 BERT 不可用时 fallback 到 TokenOverlap
// 如果 scorer 为 nil 或不可用，返回 TokenOverlapScorer
// 如果 scorer 是 BERT 且可用，直接返回 BERT
// 如果 scorer 是 BERT 且不可用（timeout/error），返回 TokenOverlapScorer
func FallbackToTokenOverlap(scorer Scorer) Scorer {
	if scorer == nil {
		return NewTokenOverlapScorer()
	}
	name := scorer.Name()
	if scorer.IsAvailable() {
		return scorer
	}
	// BERT 不可用时的日志（静默 fallback）
	_ = name
	return NewTokenOverlapScorer()
}

// ScorerFactory 根据配置创建合适的 scorer
// 如果 enableBERT=true 且 endpoint 不为空，尝试创建 BERT scorer
// 如果 BERT 不可用，fallback 到 TokenOverlapScorer
func ScorerFactory(enableBERT bool, bertEndpoint string, bertTimeout time.Duration) Scorer {
	if !enableBERT || bertEndpoint == "" {
		return NewTokenOverlapScorer()
	}

	bertScorer := NewBERTEmbeddingScorer(bertEndpoint)
	if bertTimeout > 0 {
		bertScorer.SetTimeout(bertTimeout)
	}
	bertScorer.Enable()

	// 尝试验证连接
	result, err := bertScorer.ScoreAll("hello test")
	if err != nil || !bertScorer.IsAvailable() {
		return NewTokenOverlapScorer()
	}
	_ = result
	return bertScorer
}

// MockBERTEmbeddingScorer 模拟 BERT embedding scorer，用于离线对比测试
// 基于 pool 关键词的加权计算，模拟 BERT 的行为
// 仅供 offline eval 和 shadow comparison 使用
type MockBERTEmbeddingScorer struct {
	router *TokenOverlapSimilarityRouter
}

// NewMockBERTEmbeddingScorer 创建 mock BERT scorer
func NewMockBERTEmbeddingScorer() *MockBERTEmbeddingScorer {
	return &MockBERTEmbeddingScorer{
		router: NewTokenOverlapSimilarityRouter(),
	}
}

// Score 计算 prompt 与 pool 的匹配得分（模拟 BERT）
func (s *MockBERTEmbeddingScorer) Score(prompt string, pool PreferredPool) (float64, []string, error) {
	// 模拟 BERT 比 TokenOverlap 更精准
	// 综合使用关键词匹配 + 描述相似度 + 上下文分析
	keywordScore, keywords := s.router.CalculateKeywordScore(prompt, pool)
	descScore := s.router.CalculateDescriptionSimilarity(prompt, pool)

	// 模拟 BERT 的语义理解能力：对匹配关键词有更强的置信度
	// 如果有关键词匹配，增强得分
	if len(keywords) > 0 {
		keywordScore = keywordScore * 1.15 // BERT 更能理解语义匹配
		if keywordScore > 1.0 {
			keywordScore = 1.0
		}
	}

	// 描述相似度权重更高（模拟 BERT 的语义理解）
	totalScore := keywordScore*0.6 + descScore*0.4
	return totalScore, keywords, nil
}

// ScoreAll 计算所有 pool 的得分
func (s *MockBERTEmbeddingScorer) ScoreAll(prompt string) (map[string]float64, error) {
	scores := make(map[string]float64)
	pools := []PreferredPool{PoolDefault, PoolCheap, PoolCode, PoolVision, PoolDocument, PoolData, PoolPrivate, PoolImageGeneration}

	for _, pool := range pools {
		score, _, _ := s.Score(prompt, pool)
		scores[string(pool)] = Round(score)
	}

	return scores, nil
}

// ScoreAllWithBest 实现 Scorer 接口
func (s *MockBERTEmbeddingScorer) ScoreAllWithBest(prompt string) (*ScorerResult, error) {
	scores, err := s.ScoreAll(prompt)
	if err != nil {
		return nil, err
	}
	return getScorerResult(scores, s.Name()), nil
}

// Name 返回 scorer 名称
func (s *MockBERTEmbeddingScorer) Name() string {
	return "MockBERTEmbeddingScorer"
}

// IsAvailable 检查 scorer 是否可用
func (s *MockBERTEmbeddingScorer) IsAvailable() bool {
	return true // mock 始终可用
}

// RealEmbeddingRequest 请求格式（与 Python service 对应）
type RealEmbeddingRequest struct {
	Prompt          string   `json:"prompt"`
	CandidatePools  []string `json:"candidate_pools"`
}

// RealEmbeddingResponse 响应格式（与 Python service 对应）
type RealEmbeddingResponse struct {
	BestPool         string             `json:"best_pool"`
	BestScore        float64            `json:"best_score"`
	SecondBestPool   string             `json:"second_best_pool"`
	SecondBestScore  float64            `json:"second_best_score"`
	ScoreMargin      float64            `json:"score_margin"`
	Scores           map[string]float64 `json:"scores"`
	LatencyMs        float64            `json:"latency_ms"`
	ModelName        string             `json:"model_name"`
}

// RealEmbeddingScorer 基于真实 multilingual-e5-small 的 scorer
// 通过 HTTP 调用 Python embedding service
// 仅用于 offline comparison，不进入 takeover 主链路
type RealEmbeddingScorer struct {
	endpoint    string
	enabled     bool
	httpClient  *http.Client
	timeout     time.Duration
	poolMapping map[string]PreferredPool // Python pool name -> Go PreferredPool
}

// NewRealEmbeddingScorer 创建真实 embedding scorer
func NewRealEmbeddingScorer(endpoint string) *RealEmbeddingScorer {
	return &RealEmbeddingScorer{
		endpoint: endpoint,
		enabled:  false, // 默认禁用
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		timeout: 5 * time.Second,
		poolMapping: map[string]PreferredPool{
			"code_pool":             PoolCode,
			"data_pool":             PoolData,
			"document_pool":         PoolDocument,
			"vision_pool":           PoolVision,
			"image_generation_pool": PoolImageGeneration,
			"cheap_chat_pool":       PoolCheap,
			"general_pool":          PoolDefault,
		},
	}
}

// SetEndpoint 设置服务 endpoint
func (s *RealEmbeddingScorer) SetEndpoint(endpoint string) {
	s.endpoint = endpoint
}

// SetTimeout 设置请求超时
func (s *RealEmbeddingScorer) SetTimeout(timeout time.Duration) {
	s.timeout = timeout
	s.httpClient.Timeout = timeout
}

// Enable 启用 RealEmbedding scorer
func (s *RealEmbeddingScorer) Enable() {
	s.enabled = true
}

// Disable 禁用 RealEmbedding scorer
func (s *RealEmbeddingScorer) Disable() {
	s.enabled = false
}

// EnableWithEndpoint 启用并设置 endpoint
func (s *RealEmbeddingScorer) EnableWithEndpoint(endpoint string) {
	s.endpoint = endpoint
	s.enabled = true
}

// routePool 调用 Python service 进行 pool 路由
func (s *RealEmbeddingScorer) routePool(prompt string) (*RealEmbeddingResponse, error) {
	reqBody := RealEmbeddingRequest{
		Prompt: prompt,
		CandidatePools: []string{
			"code_pool",
			"data_pool",
			"document_pool",
			"vision_pool",
			"image_generation_pool",
			"cheap_chat_pool",
			"general_pool",
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", s.endpoint+"/route/pool", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding service returned status %d", resp.StatusCode)
	}

	var result RealEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// Score 计算 prompt 与 pool 的匹配得分
func (s *RealEmbeddingScorer) Score(prompt string, pool PreferredPool) (float64, []string, error) {
	if !s.enabled {
		return 0, nil, ErrBERTNotAvailable
	}

	result, err := s.routePool(prompt)
	if err != nil {
		return 0, nil, err
	}

	// 转换 pool name
	poolName := ""
	switch pool {
	case PoolCode:
		poolName = "code_pool"
	case PoolData:
		poolName = "data_pool"
	case PoolDocument:
		poolName = "document_pool"
	case PoolVision:
		poolName = "vision_pool"
	case PoolImageGeneration:
		poolName = "image_generation_pool"
	case PoolCheap:
		poolName = "cheap_chat_pool"
	case PoolDefault:
		poolName = "general_pool"
	default:
		poolName = "general_pool"
	}

	score, ok := result.Scores[poolName]
	if !ok {
		return 0, nil, fmt.Errorf("pool %s not in response", poolName)
	}

	return score, nil, nil
}

// ScoreAll 计算所有 pool 的得分
func (s *RealEmbeddingScorer) ScoreAll(prompt string) (map[string]float64, error) {
	if !s.enabled {
		return nil, ErrBERTNotAvailable
	}

	result, err := s.routePool(prompt)
	if err != nil {
		return nil, err
	}

	scores := make(map[string]float64)
	for pyName, score := range result.Scores {
		goPool, ok := s.poolMapping[pyName]
		if ok {
			scores[string(goPool)] = Round(score)
		}
	}

	return scores, nil
}

// ScoreAllWithBest 实现 Scorer 接口
func (s *RealEmbeddingScorer) ScoreAllWithBest(prompt string) (*ScorerResult, error) {
	if !s.enabled {
		return nil, ErrBERTNotAvailable
	}

	result, err := s.routePool(prompt)
	if err != nil {
		return nil, err
	}

	scores := make(map[string]float64)
	for pyName, score := range result.Scores {
		goPool, ok := s.poolMapping[pyName]
		if ok {
			scores[string(goPool)] = Round(score)
		}
	}

	scorerResult := &ScorerResult{
		Scores:     scores,
		ScorerType: s.Name(),
	}

	// 转换 pool names
	goBestPool, ok := s.poolMapping[result.BestPool]
	if ok {
		scorerResult.BestPool = string(goBestPool)
	}
	scorerResult.BestScore = result.BestScore

	if result.SecondBestPool != "" {
		goSecondPool, ok := s.poolMapping[result.SecondBestPool]
		if ok {
			scorerResult.SecondBestPool = string(goSecondPool)
		}
	}
	scorerResult.SecondBestScore = result.SecondBestScore
	scorerResult.ScoreMargin = result.ScoreMargin

	return scorerResult, nil
}

// Name 返回 scorer 名称
func (s *RealEmbeddingScorer) Name() string {
	return "RealEmbeddingScorer"
}

// IsAvailable 检查 scorer 是否可用
func (s *RealEmbeddingScorer) IsAvailable() bool {
	return s.enabled && s.endpoint != ""
}

// Ensure interfaces are satisfied
var _ Scorer = (*TokenOverlapScorer)(nil)
var _ Scorer = (*BERTEmbeddingScorer)(nil)
var _ Scorer = (*MockBERTEmbeddingScorer)(nil)
var _ Scorer = (*RealEmbeddingScorer)(nil)