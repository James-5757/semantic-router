package semanticrouter

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// ModelPlacement describes where a model can be scheduled. A model may have
// more than one placement when the same upstream model is exposed by multiple
// logical pools or groups.
type ModelPlacement struct {
	Group string
	Pools []string
	Tier  PreferredTier
}

// ModelCapabilityProfile is the declarative model registry entry. It is
// independent from credentials, account health, and runtime load.
type ModelCapabilityProfile struct {
	ModelID             string
	Provider            string
	Placements          []ModelPlacement
	Capabilities        map[string]bool
	Enabled             bool
	ContextWindowTokens int
	MaxOutputTokens     int
	// Benchmark and operational signals are advisory ranking inputs only.
	CodingAgentScore  float64
	DataAnalysisScore float64
	ReasoningScore    float64
	ChineseScore      float64
	LongContextScore  float64
	VisionScore       float64
	DocumentScore     float64
	GeneralScore      float64
	CostPerTask       float64
	LatencyMS         float64
	ProfileSource     string
	EvidenceSource    string
	ProfileSnapshot   string
	ScoreConfidence   float64
	BenchmarkVersion  string
	EvaluatedAt       string
}

type ModelRegistry interface {
	GetModel(ctx context.Context, modelID string) (*ModelCapabilityProfile, bool)
}

// PhysicalGroupForPool is the shared coarse routing boundary used by model
// selection. Pool and capability checks still remain mandatory afterwards.
func PhysicalGroupForPool(pool PreferredPool) string {
	switch pool {
	case PoolCode, PoolData, PoolDocument:
		return "technical_models"
	case PoolVision:
		return "vision_models"
	case PoolImageGeneration:
		return "image_models"
	case PoolCheap, PoolDefault:
		return "general_chat_models"
	default:
		return ""
	}
}

type StaticModelRegistry struct {
	models map[string]*ModelCapabilityProfile
}

func NewStaticModelRegistry(profiles []*ModelCapabilityProfile) *StaticModelRegistry {
	registry := &StaticModelRegistry{models: make(map[string]*ModelCapabilityProfile, len(profiles))}
	for _, profile := range profiles {
		if profile == nil || strings.TrimSpace(profile.ModelID) == "" {
			continue
		}
		copyProfile := *profile
		copyProfile.Placements = append([]ModelPlacement(nil), profile.Placements...)
		copyProfile.Capabilities = cloneBoolMap(profile.Capabilities)
		registry.models[profile.ModelID] = &copyProfile
	}
	return registry
}

func (r *StaticModelRegistry) GetModel(_ context.Context, modelID string) (*ModelCapabilityProfile, bool) {
	if r == nil {
		return nil, false
	}
	profile, ok := r.models[modelID]
	if !ok {
		return nil, false
	}
	copyProfile := *profile
	copyProfile.Placements = append([]ModelPlacement(nil), profile.Placements...)
	copyProfile.Capabilities = cloneBoolMap(profile.Capabilities)
	return &copyProfile, true
}

// NewDefaultModelRegistry contains conservative dry-run examples. Production
// deployments should load these entries from OAM or a real repository.
func NewDefaultModelRegistry() ModelRegistry {
	registry := NewStaticModelRegistry([]*ModelCapabilityProfile{
		{
			ModelID: "gpt-4.1-mini",
			Placements: []ModelPlacement{
				{Group: "technical_models", Pools: []string{"code_pool", "data_pool", "document_pool"}, Tier: TierMedium},
				{Group: "general_chat_models", Pools: []string{"default_pool"}, Tier: TierMedium},
			},
			Capabilities: map[string]bool{"code": true, "data": true, "document": true, "json": true, "streaming": true},
			Enabled:      true,
		},
		{
			ModelID:      "gpt-4.1-nano",
			Placements:   []ModelPlacement{{Group: "general_chat_models", Pools: []string{"cheap_chat_pool", "default_pool"}, Tier: TierWeak}},
			Capabilities: map[string]bool{"text": true, "json": true, "streaming": true},
			Enabled:      true,
		},
		{
			ModelID: "gpt-4o",
			Placements: []ModelPlacement{
				{Group: "vision_models", Pools: []string{"vision_pool"}, Tier: TierStrong},
				{Group: "technical_models", Pools: []string{"document_pool"}, Tier: TierStrong},
			},
			Capabilities: map[string]bool{"code": true, "data": true, "vision": true, "document": true, "tool_call": true, "json": true, "streaming": true},
			Enabled:      true,
		},
		{
			ModelID:      "gpt-image-1",
			Placements:   []ModelPlacement{{Group: "image_models", Pools: []string{"image_generation_pool"}, Tier: TierMedium}},
			Capabilities: map[string]bool{"image_generation": true},
			Enabled:      true,
		},
		// Provider entries below are conservative starter metadata. Verify them
		// against the deployed provider endpoint before enabling takeover.
		{
			ModelID: "deepseek-chat",
			Placements: []ModelPlacement{
				{Group: "technical_models", Pools: []string{"code_pool", "data_pool"}, Tier: TierMedium},
				{Group: "general_chat_models", Pools: []string{"default_pool"}, Tier: TierMedium},
			},
			Capabilities: map[string]bool{"code": true, "data": true, "json": true, "streaming": true}, Enabled: true,
		},
		{
			ModelID:      "deepseek-reasoner",
			Placements:   []ModelPlacement{{Group: "technical_models", Pools: []string{"code_pool", "data_pool"}, Tier: TierStrong}},
			Capabilities: map[string]bool{"code": true, "data": true, "json": true}, Enabled: true,
		},
		{
			ModelID:      "deepseek-coder",
			Placements:   []ModelPlacement{{Group: "technical_models", Pools: []string{"code_pool"}, Tier: TierStrong}},
			Capabilities: map[string]bool{"code": true, "tool_call": true, "json": true, "streaming": true}, Enabled: true,
		},
		{
			ModelID:      "qwen-plus",
			Placements:   []ModelPlacement{{Group: "technical_models", Pools: []string{"code_pool", "data_pool", "document_pool"}, Tier: TierMedium}},
			Capabilities: map[string]bool{"code": true, "data": true, "document": true, "tool_call": true, "json": true, "streaming": true}, Enabled: true,
		},
		{
			ModelID:      "qwen-max",
			Placements:   []ModelPlacement{{Group: "technical_models", Pools: []string{"code_pool", "data_pool", "document_pool"}, Tier: TierStrong}},
			Capabilities: map[string]bool{"code": true, "data": true, "document": true, "tool_call": true, "json": true, "streaming": true}, Enabled: true,
		},
		{
			ModelID:      "qwen-vl-max",
			Placements:   []ModelPlacement{{Group: "vision_models", Pools: []string{"vision_pool", "document_pool"}, Tier: TierStrong}},
			Capabilities: map[string]bool{"vision": true, "document": true, "data": true, "json": true}, Enabled: true,
		},
		{
			ModelID:      "qwen-turbo",
			Placements:   []ModelPlacement{{Group: "general_chat_models", Pools: []string{"cheap_chat_pool", "default_pool"}, Tier: TierWeak}},
			Capabilities: map[string]bool{"text": true, "json": true, "streaming": true}, Enabled: true,
		},
		{
			ModelID:      "MiniMax-Text-01",
			Placements:   []ModelPlacement{{Group: "general_chat_models", Pools: []string{"default_pool", "cheap_chat_pool"}, Tier: TierMedium}},
			Capabilities: map[string]bool{"text": true, "code": true, "data": true, "json": true, "streaming": true}, Enabled: true,
		},
		{
			ModelID:      "claude-3-5-sonnet",
			Placements:   []ModelPlacement{{Group: "technical_models", Pools: []string{"code_pool", "data_pool", "document_pool"}, Tier: TierStrong}},
			Capabilities: map[string]bool{"code": true, "data": true, "document": true, "tool_call": true, "json": true, "streaming": true}, Enabled: true,
		},
		{
			ModelID:      "claude-3-7-sonnet",
			Placements:   []ModelPlacement{{Group: "technical_models", Pools: []string{"code_pool", "data_pool", "document_pool"}, Tier: TierStrong}},
			Capabilities: map[string]bool{"code": true, "data": true, "document": true, "tool_call": true, "json": true, "streaming": true}, Enabled: true,
		},
	})
	// Starter scores are intentionally conservative priors. Replace them with
	// a dated provider benchmark snapshot before using takeover in production.
	for _, profile := range registry.models {
		profile.Provider = profileProvider(profile.ModelID)
		profile.ProfileSource = "starter_registry"
		profile.EvidenceSource = "starter_prior_not_benchmark"
		profile.ProfileSnapshot = "2026-08-07"
		profile.ScoreConfidence = 0.25
		profile.BenchmarkVersion = "prior-only"
		profile.EvaluatedAt = "2026-08-07"
		profile.CodingAgentScore = starterScore(profile.ModelID, "code")
		profile.DataAnalysisScore = starterScore(profile.ModelID, "data")
		profile.ReasoningScore = starterScore(profile.ModelID, "reasoning")
		profile.ChineseScore = starterScore(profile.ModelID, "chinese")
		profile.LongContextScore = starterScore(profile.ModelID, "long_context")
		profile.VisionScore = starterScore(profile.ModelID, "vision")
		profile.DocumentScore = starterScore(profile.ModelID, "document")
		profile.GeneralScore = starterScore(profile.ModelID, "general")
	}
	return registry
}

func profileProvider(modelID string) string {
	switch {
	case strings.HasPrefix(modelID, "gpt"):
		return "openai"
	case strings.HasPrefix(modelID, "deepseek"):
		return "deepseek"
	case strings.HasPrefix(modelID, "qwen"):
		return "qwen"
	case strings.HasPrefix(modelID, "claude"):
		return "anthropic"
	case strings.HasPrefix(modelID, "MiniMax"):
		return "minimax"
	default:
		return "unknown"
	}
}

func starterScore(modelID, task string) float64 {
	// These are intentionally conservative priors for local staging. They are
	// differentiated by task, but are not presented as live benchmark truth.
	priors := map[string]map[string]float64{
		"gpt-4.1-mini":      {"code": 0.76, "data": 0.77, "reasoning": 0.72, "chinese": 0.70, "long_context": 0.70, "vision": 0.55, "document": 0.76, "general": 0.78},
		"gpt-4.1-nano":      {"code": 0.65, "data": 0.64, "vision": 0.45, "document": 0.60, "general": 0.72},
		"gpt-4o":            {"code": 0.81, "data": 0.80, "reasoning": 0.80, "chinese": 0.78, "long_context": 0.82, "vision": 0.88, "document": 0.84, "general": 0.82},
		"gpt-image-1":       {"code": 0.35, "data": 0.30, "vision": 0.82, "document": 0.35, "general": 0.40},
		"deepseek-chat":     {"code": 0.75, "data": 0.73, "reasoning": 0.78, "chinese": 0.80, "long_context": 0.68, "vision": 0.35, "document": 0.48, "general": 0.70},
		"deepseek-coder":    {"code": 0.86, "data": 0.70, "vision": 0.30, "document": 0.42, "general": 0.58},
		"deepseek-reasoner": {"code": 0.84, "data": 0.82, "vision": 0.30, "document": 0.50, "general": 0.68},
		"qwen-plus":         {"code": 0.78, "data": 0.79, "vision": 0.38, "document": 0.70, "general": 0.72},
		"qwen-max":          {"code": 0.83, "data": 0.85, "vision": 0.42, "document": 0.78, "general": 0.78},
		"qwen-vl-max":       {"code": 0.60, "data": 0.70, "vision": 0.86, "document": 0.84, "general": 0.70},
		"qwen-turbo":        {"code": 0.58, "data": 0.58, "vision": 0.30, "document": 0.42, "general": 0.70},
		"MiniMax-Text-01":   {"code": 0.68, "data": 0.65, "vision": 0.25, "document": 0.45, "general": 0.74},
		"claude-3-5-sonnet": {"code": 0.86, "data": 0.82, "vision": 0.48, "document": 0.86, "general": 0.84},
		"claude-3-7-sonnet": {"code": 0.90, "data": 0.85, "vision": 0.52, "document": 0.88, "general": 0.86},
	}
	if byTask, ok := priors[modelID]; ok {
		if score, ok := byTask[task]; ok {
			return score
		}
	}
	return 0.50
}

type ModelSelectionResult struct {
	Account          *RealSchedulerAccount
	CandidateCount   int
	CandidateModels  []string
	CandidateDetails []ModelCandidateScore
	DecisionSource   string
	FallbackReason   string
}

type ModelCandidateScore struct {
	AccountID        int64   `json:"account_id"`
	ModelID          string  `json:"model_id"`
	Provider         string  `json:"provider"`
	ProfileScore     float64 `json:"profile_score"`
	PoolScore        float64 `json:"pool_score"`
	TaskFitScore     float64 `json:"task_fit_score"`
	CapabilityScore  float64 `json:"capability_score"`
	TierFitScore     float64 `json:"tier_fit_score"`
	LoadScore        float64 `json:"load_score"`
	PriorityScore    float64 `json:"priority_score"`
	CostScore        float64 `json:"cost_score"`
	RuntimeScore     float64 `json:"runtime_score"`
	FinalScore       float64 `json:"final_score"`
	CostPerTask      float64 `json:"cost_per_task"`
	LatencyMS        float64 `json:"latency_ms"`
	ProfileSource    string  `json:"profile_source"`
	EvidenceSource   string  `json:"evidence_source"`
	ScoreConfidence  float64 `json:"score_confidence"`
	BenchmarkVersion string  `json:"benchmark_version"`
	EvaluatedAt      string  `json:"evaluated_at"`
	RankingVersion   string  `json:"ranking_version"`
	Reason           string  `json:"reason"`
}

type ModelCandidateSelector struct {
	registry ModelRegistry
}

func NewModelCandidateSelector(registry ModelRegistry) *ModelCandidateSelector {
	return &ModelCandidateSelector{registry: registry}
}

func (s *ModelCandidateSelector) Select(ctx context.Context, accounts []*RealSchedulerAccount, req *SchedulerSelectRequest) (*ModelSelectionResult, error) {
	if req == nil {
		return nil, fmt.Errorf("scheduler request is nil")
	}

	candidates := make([]*RealSchedulerAccount, 0, len(accounts))
	for _, account := range accounts {
		if !isRealAccountAvailable(account) || !isRealAccountPoolMatch(account, req.PreferredPool) || !isRealAccountCapabilityMatch(account, req) || !isRealAccountTierMatch(account, req.PreferredTier) {
			continue
		}
		if req.ModelGroup != "" && account.ModelGroup != req.ModelGroup {
			continue
		}
		if len(req.AllowedModelIDs) > 0 && !containsModelID(req.AllowedModelIDs, account.Model) {
			continue
		}
		if req.PreferredGroup != "" && account.Group != "" && account.Group != req.PreferredGroup {
			continue
		}
		if !s.registryAllows(ctx, account, req) {
			continue
		}
		candidates = append(candidates, account)
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no eligible model candidate for pool=%s group=%s tier=%s", req.PreferredPool, req.PreferredGroup, req.PreferredTier)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		if left.CurrentLoad != right.CurrentLoad {
			return left.CurrentLoad < right.CurrentLoad
		}
		if left.Priority != right.Priority {
			return left.Priority < right.Priority
		}
		if left.Price != right.Price {
			return left.Price < right.Price
		}
		return left.ID < right.ID
	})

	candidateModels := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidateModels = append(candidateModels, candidate.Model)
	}
	details := make([]ModelCandidateScore, 0, len(candidates))
	for _, candidate := range candidates {
		details = append(details, s.profileScore(ctx, candidate, req))
	}
	return &ModelSelectionResult{Account: candidates[0], CandidateCount: len(candidates), CandidateModels: candidateModels, CandidateDetails: details, DecisionSource: "model_registry_candidate_ranking"}, nil
}

func containsModelID(modelIDs []string, modelID string) bool {
	for _, candidate := range modelIDs {
		if strings.TrimSpace(candidate) == modelID {
			return true
		}
	}
	return false
}

func (s *ModelCandidateSelector) profileScore(ctx context.Context, account *RealSchedulerAccount, req *SchedulerSelectRequest) ModelCandidateScore {
	detail := ModelCandidateScore{AccountID: account.ID, ModelID: account.Model, RankingVersion: "candidate_ranking_v2", Reason: "no_profile_score"}
	if s == nil || s.registry == nil {
		return detail
	}
	profile, ok := s.registry.GetModel(ctx, account.Model)
	if !ok || profile == nil {
		return detail
	}
	detail.Provider, detail.CostPerTask, detail.LatencyMS, detail.ProfileSource, detail.EvidenceSource = profile.Provider, profile.CostPerTask, profile.LatencyMS, profile.ProfileSource, profile.EvidenceSource
	detail.ScoreConfidence, detail.BenchmarkVersion, detail.EvaluatedAt = profile.ScoreConfidence, profile.BenchmarkVersion, profile.EvaluatedAt
	detail.PoolScore = profileScoreForPool(profile, req.PreferredPool)
	detail.TaskFitScore = taskFitScore(profile, req.TaskSignals)
	if len(req.TaskSignals) == 0 {
		// No task-specific evidence means the pool score is the neutral baseline.
		detail.TaskFitScore = detail.PoolScore
	}
	detail.ProfileScore = detail.PoolScore
	// Pool preserves task-domain relevance; task signals distinguish capable
	// models inside that eligible pool without changing group eligibility.
	detail.CapabilityScore = 0.75*detail.PoolScore + 0.25*detail.TaskFitScore
	detail.TierFitScore = tierFitScore(account.Tier, req.PreferredTier)
	detail.LoadScore = 1 / (1 + float64(maxInt(account.CurrentLoad, 0)))
	detail.PriorityScore = 1 / (1 + float64(maxInt(account.Priority, 0))/10)
	detail.CostScore = 1 / (1 + maxFloat(account.Price, 0))
	detail.RuntimeScore = 0.45*detail.LoadScore + 0.30*detail.PriorityScore + 0.25*detail.CostScore
	// Profile quality remains dominant; runtime signals break ties between
	// equally capable models without changing the old Scheduler main result.
	detail.FinalScore = 0.55*detail.CapabilityScore + 0.20*detail.TierFitScore + 0.25*detail.RuntimeScore
	detail.Reason = fmt.Sprintf("%s profile for %s; ranking_v2=pool/task-fit/tier/load/priority/cost", poolProfileLabel(req.PreferredPool), profile.Provider)
	return detail
}

func taskFitScore(profile *ModelCapabilityProfile, signals []string) float64 {
	if profile == nil || len(signals) == 0 {
		return profile.GeneralScore
	}
	var scores []float64
	for _, signal := range signals {
		switch strings.ToLower(strings.TrimSpace(signal)) {
		case "code", "code_generation", "api_design", "api_implementation", "backend_development", "tool_use", "tool_call":
			scores = append(scores, profile.CodingAgentScore)
		case "data", "data_analysis":
			scores = append(scores, profile.DataAnalysisScore)
		case "document", "document_processing", "contract_review":
			scores = append(scores, profile.DocumentScore)
		case "reasoning", "machine_learning", "validation_design", "ensemble_learning", "post_processing", "hyperparameter_optimization", "data_science":
			scores = append(scores, profile.ReasoningScore)
		case "chinese", "zh", "zh-cn":
			scores = append(scores, profile.ChineseScore)
		case "long_context", "long_document":
			scores = append(scores, profile.LongContextScore)
		}
	}
	if len(scores) == 0 {
		return profile.GeneralScore
	}
	var total float64
	for _, score := range scores {
		total += score
	}
	return total / float64(len(scores))
}

func tierFitScore(actual, requested PreferredTier) float64 {
	if requested == "" || actual == "" {
		return 0.5
	}
	if actual == requested {
		return 1
	}
	if requested == TierStrong && actual == TierMedium {
		return 0.7
	}
	if requested == TierMedium && actual == TierStrong {
		return 0.9
	}
	if requested == TierWeak && actual == TierMedium {
		return 0.85
	}
	return 0.4
}

func maxInt(value, fallback int) int {
	if value > fallback {
		return value
	}
	return fallback
}

func maxFloat(value, fallback float64) float64 {
	if value > fallback {
		return value
	}
	return fallback
}

func profileScoreForPool(profile *ModelCapabilityProfile, pool PreferredPool) float64 {
	switch pool {
	case PoolCode:
		return profile.CodingAgentScore
	case PoolData:
		return profile.DataAnalysisScore
	case PoolVision, PoolImageGeneration:
		return profile.VisionScore
	case PoolDocument:
		return profile.DocumentScore
	default:
		return profile.GeneralScore
	}
}

func poolProfileLabel(pool PreferredPool) string {
	switch pool {
	case PoolCode:
		return "coding_agent"
	case PoolData:
		return "data_analysis"
	case PoolVision:
		return "vision"
	case PoolDocument:
		return "document"
	case PoolImageGeneration:
		return "image_generation"
	default:
		return "general"
	}
}

func (s *ModelCandidateSelector) registryAllows(ctx context.Context, account *RealSchedulerAccount, req *SchedulerSelectRequest) bool {
	if s == nil || s.registry == nil {
		return true
	}
	profile, ok := s.registry.GetModel(ctx, account.Model)
	if !ok || !profile.Enabled {
		return false
	}
	if profile.ContextWindowTokens > 0 && req.ContextTokens+req.MaxOutputTokens > profile.ContextWindowTokens {
		return false
	}
	if req.RequiresStreaming && !profile.Capabilities["streaming"] {
		return false
	}
	if req.RequiresToolCall && !profile.Capabilities["tool_call"] {
		return false
	}
	for _, capability := range requiredCapabilityNames(req) {
		if !profile.Capabilities[capability] {
			return false
		}
	}
	if req.PreferredGroup == "" {
		return true
	}
	for _, placement := range profile.Placements {
		if placement.Group == req.PreferredGroup && placementPoolMatch(placement, req.PreferredPool) && isTierAtLeast(placement.Tier, req.PreferredTier) {
			return true
		}
	}
	return false
}

func placementPoolMatch(placement ModelPlacement, pool PreferredPool) bool {
	wanted := map[PreferredPool]string{
		PoolCode: "code_pool", PoolData: "data_pool", PoolVision: "vision_pool",
		PoolDocument: "document_pool", PoolImageGeneration: "image_generation_pool",
		PoolCheap: "cheap_chat_pool", PoolDefault: "default_pool",
	}[pool]
	for _, candidatePool := range placement.Pools {
		if candidatePool == string(pool) || candidatePool == wanted || (pool == PoolDefault && candidatePool == "cheap_chat_pool") {
			return true
		}
	}
	return false
}

func requiredCapabilityNames(req *SchedulerSelectRequest) []string {
	capabilities := make([]string, 0, 4)
	switch req.PreferredPool {
	case PoolCode:
		capabilities = append(capabilities, "code")
	case PoolData:
		capabilities = append(capabilities, "data")
	case PoolVision:
		capabilities = append(capabilities, "vision")
	case PoolImageGeneration:
		capabilities = append(capabilities, "image_generation")
	case PoolDocument:
		capabilities = append(capabilities, "document")
	}
	if req.RequiredCapabilities.VisionCapable {
		capabilities = append(capabilities, "vision")
	}
	if req.RequiredCapabilities.DocumentCapable {
		capabilities = append(capabilities, "document")
	}
	if req.RequiredCapabilities.ImageCapability != "" && req.RequiredCapabilities.ImageCapability != ImageCapabilityNone {
		capabilities = append(capabilities, "vision")
	}
	return capabilities
}

func isTierAtLeast(actual, required PreferredTier) bool {
	rank := func(tier PreferredTier) int {
		switch tier {
		case TierStrong:
			return 3
		case TierMedium:
			return 2
		case TierWeak:
			return 1
		default:
			return 0
		}
	}
	return rank(actual) >= rank(required)
}

func cloneBoolMap(source map[string]bool) map[string]bool {
	if source == nil {
		return nil
	}
	target := make(map[string]bool, len(source))
	for key, value := range source {
		target[key] = value
	}
	return target
}
