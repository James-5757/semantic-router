package semanticrouter

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Helper functions for configuration
func envBool(name string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return fallback
}

func parsePositiveInt(s string) int {
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 {
		return 0
	}
	return v
}

func parseFloat(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

// BoundaryHybridConfig contains configuration for boundary hybrid tier routing
type BoundaryHybridConfig struct {
	// Enable boundary detection and RouteLLM participation
	Enabled bool `json:"routellm_boundary_enabled" yaml:"routellm_boundary_enabled"`

	// Shadow only - never override final tier (safety requirement)
	ShadowOnly bool `json:"routellm_boundary_shadow_only" yaml:"routellm_boundary_shadow_only"`

	// Takeover is NOT allowed - this is a safety requirement
	// Configuration validation should reject if this is set to true
	TakeoverEnabled bool `json:"routellm_boundary_takeover_enabled" yaml:"routellm_boundary_takeover_enabled"`

	// RouteLLM service URL
	ServiceURL string `json:"routellm_boundary_service_url" yaml:"routellm_boundary_service_url"`

	// Timeout for RouteLLM calls
	TimeoutMS int `json:"routellm_boundary_timeout_ms" yaml:"routellm_boundary_timeout_ms"`

	// ============================================================================
	// 统一量纲说明 (Unified Scale):
	// 所有阈值统一使用 0~1 范围，与概率保持一致
	// ============================================================================

	// Rule confidence threshold: 规则置信度阈值，低于此值触发 RouteLLM
	// 量纲: 0~1 (概率)
	// 默认值: 0.65
	RuleConfidenceThreshold float64 `json:"routellm_boundary_rule_confidence_threshold" yaml:"routellm_boundary_rule_confidence_threshold"`

	// Threshold margin: 边界检测的容差范围
	// 量纲: 0~1 (相对值，与 boundary 相同)
	// 默认值: 0.10 (表示边界值 ±0.10 范围内视为边界)
	ThresholdMargin float64 `json:"routellm_boundary_threshold_margin" yaml:"routellm_boundary_threshold_margin"`

	// ============================================================================
	// Complexity Score 边界 (归一化到 0~1)
	// 注意: Complexity Score 原始范围是 0~10+，通过除以 10 归一化
	// ============================================================================

	// Medium-Strong boundary: Medium 与 Strong 的分界线
	// 原始值: 5.0, 归一化: 0.50 (5.0/10)
	// 量纲: 0~1 (归一化后的 complexity score)
	// 默认值: 0.50
	MediumStrongBoundary float64 `json:"routellm_boundary_medium_strong" yaml:"routellm_boundary_medium_strong"`

	// Weak-Medium boundary: Weak 与 Medium 的分界线
	// 原始值: 1.5, 归一化: 0.15 (1.5/10)
	// 量纲: 0~1 (归一化后的 complexity score)
	// 默认值: 0.15
	WeakMediumBoundary float64 `json:"routellm_boundary_weak_medium" yaml:"routellm_boundary_weak_medium"`

	// ============================================================================
	// Uncertainty Score 阈值 (新增)
	// ============================================================================

	// UncertaintyScoreThreshold: 不确定性分数阈值，超过此值触发 boundary
	// 量纲: 0~1 (概率)
	// 默认值: 0.30
	UncertaintyScoreThreshold float64 `json:"routellm_boundary_uncertainty_threshold" yaml:"routellm_boundary_uncertainty_threshold"`

	// Override eligibility is advisory in shadow mode. It records when the
	// learned tier is strong enough to cover a rule disagreement, but does not
	// change FinalTier while takeover is disabled.
	OverrideConfidenceThreshold float64 `json:"routellm_boundary_override_confidence_threshold" yaml:"routellm_boundary_override_confidence_threshold"`

	// ============================================================================
	// 信号配置 (Signals - 不参与归一化)
	// ============================================================================

	// Hard minimum tier signals - these tasks cannot be downgraded
	HardMinimumTierSignals []string `json:"-" yaml:"-"`

	// Professional task signals - these should trigger boundary detection
	ProfessionalTaskSignals []string `json:"-" yaml:"-"`
}

// DefaultBoundaryHybridConfig returns the default configuration
// 注意: 所有阈值已统一为 0~1 范围
func DefaultBoundaryHybridConfig() BoundaryHybridConfig {
	return BoundaryHybridConfig{
		Enabled:         false, // Disabled by default, enable via env
		ShadowOnly:      true,  // Always shadow by default
		TakeoverEnabled: false, // Never allow takeover
		ServiceURL:      "http://127.0.0.1:8002",
		TimeoutMS:       1000,
		// 统一量纲: 0~1
		RuleConfidenceThreshold: 0.65, // 规则置信度阈值
		ThresholdMargin:         0.10, // 边界容差 ±0.10
		// Complexity Score 边界 (已归一化: 原始值/10)
		MediumStrongBoundary: 0.50, // 原始 5.0 -> 归一化 0.50
		WeakMediumBoundary:   0.15, // 原始 1.5 -> 归一化 0.15
		// Uncertainty Score 阈值
		UncertaintyScoreThreshold:   0.30, // 超过 0.30 触发 boundary
		OverrideConfidenceThreshold: 0.80,
		HardMinimumTierSignals: []string{
			"安全", "security", "安全审计", "安全设计",
			"容灾", "disaster recovery", "跨区域", "multi-region",
			"高并发", "million", "百万并发", "high concurrency",
			"权限", "permission", "authorization", "认证",
			"架构", "architecture", "系统设计",
			"审计", "audit", "合规", "compliance",
			"金融", "financial", "支付", "payment",
		},
		ProfessionalTaskSignals: []string{
			"kaggle", "机器学习", "machine learning", "预测模型",
			"深度学习", "deep learning", "神经网络",
			"notebook", "jupyter", "数据科学",
			"baseline", "验证", "validation", "融合", "ensemble",
			"后处理", "post-processing", "调参", "hyperparameter",
		},
	}
}

// Validate checks configuration validity
func (c *BoundaryHybridConfig) Validate() error {
	if c.TakeoverEnabled {
		return fmt.Errorf("routellm_boundary_takeover_enabled cannot be true - takeover is not allowed")
	}
	if c.TimeoutMS <= 0 {
		c.TimeoutMS = 1000
	}
	if c.RuleConfidenceThreshold <= 0 || c.RuleConfidenceThreshold > 1 {
		c.RuleConfidenceThreshold = 0.65
	}
	if c.OverrideConfidenceThreshold <= 0 || c.OverrideConfidenceThreshold > 1 {
		c.OverrideConfidenceThreshold = 0.80
	}
	if c.ThresholdMargin < 0 {
		c.ThresholdMargin = 0.10
	}
	return nil
}

// LoadBoundaryHybridConfig loads configuration from environment variables
func LoadBoundaryHybridConfig() BoundaryHybridConfig {
	config := DefaultBoundaryHybridConfig()

	config.Enabled = envBool("ROUTELLM_BOUNDARY_ENABLED", config.Enabled)
	config.ShadowOnly = envBool("ROUTELLM_BOUNDARY_SHADOW_ONLY", config.ShadowOnly)
	config.TakeoverEnabled = envBool("ROUTELLM_BOUNDARY_TAKEOVER_ENABLED", config.TakeoverEnabled)

	if v := strings.TrimSpace(os.Getenv("ROUTELLM_BOUNDARY_SERVICE_URL")); v != "" {
		config.ServiceURL = v
	}
	if v := os.Getenv("ROUTELLM_BOUNDARY_TIMEOUT_MS"); v != "" {
		if parsed := parsePositiveInt(v); parsed > 0 {
			config.TimeoutMS = parsed
		}
	}
	if v := os.Getenv("ROUTELLM_BOUNDARY_RULE_CONFIDENCE_THRESHOLD"); v != "" {
		if parsed := parseFloat(v); parsed > 0 && parsed <= 1 {
			config.RuleConfidenceThreshold = parsed
		}
	}
	if v := os.Getenv("ROUTELLM_BOUNDARY_THRESHOLD_MARGIN"); v != "" {
		if parsed := parseFloat(v); parsed >= 0 {
			config.ThresholdMargin = parsed
		}
	}
	if v := os.Getenv("ROUTELLM_BOUNDARY_OVERRIDE_CONFIDENCE_THRESHOLD"); v != "" {
		if parsed := parseFloat(v); parsed > 0 && parsed <= 1 {
			config.OverrideConfidenceThreshold = parsed
		}
	}

	config.Validate() // Will reject takeover
	return config
}

// TierUncertaintyAssessment represents the complete tier uncertainty assessment
// 用于 Tier 判断的不确定性评估，替代原来的简单 BoundaryEligibility
type TierUncertaintyAssessment struct {
	UncertaintyScore   float64  `json:"uncertainty_score"`    // 0~1, 不确定性分数
	BoundaryEligible   bool     `json:"boundary_eligible"`    // 是否触发 RouteLLM
	Reasons            []string `json:"reasons"`              // 触发原因列表
	NearestBoundary    string   `json:"nearest_boundary"`     // "weak_medium" | "medium_strong" | "none"
	DistanceToBoundary float64  `json:"distance_to_boundary"` // 到边界的距离 (归一化 0~1)

	// 强制触发条件的结果
	TaskAmbiguous         bool `json:"task_ambiguous"`
	UnderstandingConflict bool `json:"understanding_conflict"`
	ShortButHard          bool `json:"short_but_hard"`
	LongButMechanical     bool `json:"long_but_mechanical"`

	// 加权条件的结果
	LowRuleConfidence bool `json:"low_rule_confidence"`
	MultiStep         bool `json:"multi_step"`
	MultiConstraint   bool `json:"multi_constraint"`
	MultiIntent       bool `json:"multi_intent"`
	MissingInputs     bool `json:"missing_inputs"`
	PolicyAdjusted    bool `json:"policy_adjusted"` // 是否因 policy 调整了 tier
}

// BoundaryEligibility represents the boundary detection result (legacy, for compatibility)
type BoundaryEligibility struct {
	Eligible bool     `json:"eligible"`
	Reasons  []string `json:"reasons"`
}

// BoundaryHybridDecision contains the complete hybrid tier decision
type BoundaryHybridDecision struct {
	// ============================================================================
	// Raw Rule Tier → Policy Floor → Final Rule Tier
	// ============================================================================

	// Raw rule decision (from RuleBasedTierRouter)
	RawTier        PreferredTier `json:"raw_rule_tier"`
	RuleConfidence float64       `json:"rule_confidence"`
	MatchedRule    string        `json:"matched_rule"`

	// Policy floor (minimum tier required by policy)
	MinimumTier       PreferredTier `json:"minimum_tier"`
	MinimumTierReason string        `json:"minimum_tier_reason"`
	PolicyAdjusted    bool          `json:"policy_adjusted"` // true if minimum_tier was applied

	// Final rule tier = max(raw_tier, minimum_tier)
	RuleTier PreferredTier `json:"rule_tier"`

	// Complexity assessment
	ComplexityScore float64 `json:"complexity_score"` // 原始复杂度分数 (归一化 0~1)

	// ============================================================================
	// Tier Uncertainty Assessment
	// ============================================================================

	// New: Comprehensive uncertainty assessment
	TierUncertainty TierUncertaintyAssessment `json:"tier_uncertainty"`

	// Legacy: for backward compatibility
	Boundary BoundaryEligibility `json:"boundary"`

	// ============================================================================
	// RouteLLM Shadow Result
	// ============================================================================

	// RouteLLM invocation context (区分调用原因)
	RouteLLMInvoked              bool `json:"routellm_invoked"`
	RouteLLMInvokedForComparison bool `json:"routellm_invoked_for_comparison"` // Compare All 模式
	RouteLLMInvokedForBoundary   bool `json:"routellm_invoked_for_boundary"`   // Boundary 模式
	RouteLLMUsedByHybrid         bool `json:"routellm_used_by_hybrid"`         // Hybrid 是否使用了 RouteLLM 结果

	RouteLLMAvailable         bool    `json:"routellm_available"`
	RouteLLMStrongProbability float64 `json:"routellm_strong_probability"`
	RouteLLMSuggestedTier     string  `json:"routellm_suggested_tier"`
	RouteLLMWeakThreshold     float64 `json:"routellm_weak_threshold"`
	RouteLLMStrongThreshold   float64 `json:"routellm_strong_threshold"`
	RouteLLMLatencyMs         float64 `json:"routellm_latency_ms"`
	RouteLLMError             string  `json:"routellm_error,omitempty"`

	// ============================================================================
	// Hybrid Shadow Decision
	// ============================================================================

	HybridShadowTier     PreferredTier `json:"hybrid_shadow_tier"`
	HybridDecisionReason string        `json:"hybrid_decision_reason"`
	HybridDisagreement   bool          `json:"hybrid_disagreement"`
	OverrideEligible     bool          `json:"override_eligible"`
	OverrideReason       string        `json:"override_reason"`
	OverrideThreshold    float64       `json:"override_confidence_threshold"`
	HybridUsedForFinal   bool          `json:"hybrid_used_for_final"`

	// Final decision (always equals rule_tier in shadow mode)
	FinalTier       PreferredTier `json:"final_tier"`
	FinalTierSource string        `json:"final_tier_source"`
}

// BoundaryHybridTierRouter wraps RuleBasedTierRouter with boundary detection
type BoundaryHybridTierRouter struct {
	mu         sync.RWMutex
	config     BoundaryHybridConfig
	ruleRouter *RuleBasedTierRouter
	routeLLM   LearnedTierScorer
	client     *RouteLLMTierClient
	httpClient *http.Client
}

// NewBoundaryHybridTierRouter creates a new boundary hybrid tier router
func NewBoundaryHybridTierRouter(ruleRouter *RuleBasedTierRouter, routeLLM LearnedTierScorer) *BoundaryHybridTierRouter {
	config := LoadBoundaryHybridConfig()
	client := NewRouteLLMTierClient(RouteLLMTierConfig{
		Enabled:    config.Enabled,
		ShadowOnly: config.ShadowOnly,
		ServiceURL: config.ServiceURL,
		TimeoutMS:  config.TimeoutMS,
	})

	return &BoundaryHybridTierRouter{
		config:     config,
		ruleRouter: ruleRouter,
		routeLLM:   routeLLM,
		client:     client,
		httpClient: &http.Client{Timeout: time.Duration(config.TimeoutMS) * time.Millisecond},
	}
}

// Route executes the boundary hybrid tier routing
func (r *BoundaryHybridTierRouter) Route(ctx context.Context, model string, taskType TaskType, prompt string) (*BoundaryHybridDecision, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	decision := &BoundaryHybridDecision{
		FinalTier:         TierMedium,
		FinalTierSource:   "rule_shadow",
		OverrideThreshold: r.config.OverrideConfidenceThreshold,
	}

	// Step 1: Get rule-based decision
	ruleDecision, err := r.ruleRouter.RouteWithPrompt(ctx, model, taskType, prompt)
	if err != nil {
		decision.FinalTierSource = "rule_error"
		return decision, err
	}

	// Populate raw rule decision
	decision.RawTier = ruleDecision.PreferredTier
	decision.RuleConfidence = ruleDecision.Confidence
	decision.MatchedRule = ruleDecision.MatchedRule
	decision.ComplexityScore = r.calculateComplexityScore(prompt, taskType)

	// Determine minimum tier based on hard signals (Policy Floor)
	decision.MinimumTier, decision.MinimumTierReason = r.determineMinimumTier(prompt)

	// Ensure final tier respects minimum tier: rule_tier = max(raw_tier, minimum_tier)
	decision.PolicyAdjusted = false
	if decision.RawTier < decision.MinimumTier {
		decision.RuleTier = decision.MinimumTier
		decision.PolicyAdjusted = true
	} else {
		decision.RuleTier = decision.RawTier
	}
	decision.FinalTier = decision.RuleTier

	// Step 2: Tier Uncertainty Assessment (替代原来的 checkBoundaryEligibility)
	decision.TierUncertainty = r.assessTierUncertainty(prompt, *ruleDecision, decision.ComplexityScore, decision.PolicyAdjusted)

	// Legacy compatibility: copy to Boundary
	decision.Boundary = BoundaryEligibility{
		Eligible: decision.TierUncertainty.BoundaryEligible,
		Reasons:  decision.TierUncertainty.Reasons,
	}

	// Step 3: If not boundary eligible, return with rule tier
	if !decision.Boundary.Eligible {
		decision.HybridShadowTier = decision.RuleTier
		decision.HybridDecisionReason = "rule_not_boundary"
		decision.HybridUsedForFinal = false
		decision.RouteLLMInvoked = false
		return decision, nil
	}

	// Step 4: Call RouteLLM if boundary eligible
	decision.RouteLLMInvoked = true
	decision.RouteLLMInvokedForBoundary = true // 当前是 boundary 模式

	// Check if RouteLLM service is available
	if r.routeLLM == nil {
		decision.RouteLLMAvailable = false
		decision.RouteLLMError = "routellm client not configured"
		decision.HybridShadowTier = decision.RuleTier
		decision.HybridDecisionReason = "routellm_unavailable_fallback"
		decision.HybridUsedForFinal = false
		return decision, nil
	}

	// Call RouteLLM service
	start := time.Now()
	score, err := r.routeLLM.Score(ctx, prompt)
	elapsed := float64(time.Since(start).Milliseconds())

	if err != nil {
		decision.RouteLLMAvailable = false
		decision.RouteLLMError = err.Error()
		decision.HybridShadowTier = decision.RuleTier
		decision.HybridDecisionReason = "routellm_error_fallback"
		decision.HybridUsedForFinal = false
		return decision, nil
	}

	// Populate RouteLLM result
	decision.RouteLLMAvailable = true
	decision.RouteLLMStrongProbability = score.StrongWinProbability
	decision.RouteLLMSuggestedTier = string(score.SuggestedTier)
	decision.RouteLLMWeakThreshold = score.WeakThreshold
	decision.RouteLLMStrongThreshold = score.StrongThreshold
	decision.RouteLLMLatencyMs = elapsed
	decision.RouteLLMUsedByHybrid = true // Hybrid 使用了 RouteLLM 结果

	// Step 5: Apply hybrid shadow policy
	decision.HybridShadowTier, decision.HybridDecisionReason = r.applyHybridPolicy(
		decision.RuleTier,
		decision.MinimumTier,
		score.StrongWinProbability,
		score.WeakThreshold,
		score.StrongThreshold,
		score.SuggestedTier,
		prompt,
		decision.ComplexityScore, // 已归一化的复杂度分数 (0~1)
	)
	decision.HybridDisagreement = decision.HybridShadowTier != decision.RuleTier
	decision.OverrideThreshold = r.config.OverrideConfidenceThreshold
	decision.OverrideEligible, decision.OverrideReason = r.assessOverrideEligibility(decision)

	// Final tier always equals rule tier (shadow mode)
	decision.HybridUsedForFinal = false
	decision.FinalTier = decision.RuleTier
	decision.FinalTierSource = "rule_shadow"

	return decision, nil
}

func (r *BoundaryHybridTierRouter) assessOverrideEligibility(decision *BoundaryHybridDecision) (bool, string) {
	if decision == nil || !decision.HybridDisagreement {
		return false, "no_tier_disagreement"
	}
	if !decision.RouteLLMAvailable {
		return false, "routellm_unavailable"
	}
	if decision.PolicyAdjusted && decision.HybridShadowTier < decision.MinimumTier {
		return false, "blocked_by_minimum_tier"
	}
	probability := decision.RouteLLMStrongProbability
	threshold := decision.OverrideThreshold
	if decision.HybridShadowTier == TierStrong && probability < threshold {
		return false, "strong_probability_below_override_threshold"
	}
	if decision.HybridShadowTier == TierWeak && probability > 1-threshold {
		return false, "weak_probability_above_override_threshold"
	}
	return true, "high_confidence_boundary_override_candidate"
}

// checkBoundaryEligibility determines if a request should trigger RouteLLM
// assessTierUncertainty 评估 Tier 判断的不确定性
// 返回完整的 TierUncertaintyAssessment 结构
func (r *BoundaryHybridTierRouter) assessTierUncertainty(prompt string, ruleDecision TierRouteDecision, complexityScore float64, policyAdjusted bool) TierUncertaintyAssessment {
	assessment := TierUncertaintyAssessment{
		UncertaintyScore:   0.0,
		BoundaryEligible:   false,
		Reasons:            []string{},
		NearestBoundary:    "none",
		DistanceToBoundary: 1.0, // 默认距离最远
		PolicyAdjusted:     policyAdjusted,
	}

	promptLower := strings.ToLower(prompt)
	promptLen := len(prompt)

	// ============================================================================
	// 计算到边界的距离
	// ============================================================================
	boundaryMargin := r.config.ThresholdMargin

	// 计算到 weak-medium 边界的距离
	distToWeakMedium := math.Abs(complexityScore - r.config.WeakMediumBoundary)
	// 计算到 medium-strong 边界的距离
	distToMediumStrong := math.Abs(complexityScore - r.config.MediumStrongBoundary)

	// 确定最近的边界
	if distToWeakMedium < distToMediumStrong {
		assessment.NearestBoundary = "weak_medium"
		assessment.DistanceToBoundary = distToWeakMedium
	} else if distToMediumStrong < distToWeakMedium {
		assessment.NearestBoundary = "medium_strong"
		assessment.DistanceToBoundary = distToMediumStrong
	} else {
		// 距离相等时，选择更高的不确定性
		assessment.NearestBoundary = "weak_medium"
		assessment.DistanceToBoundary = distToWeakMedium
	}

	// ============================================================================
	// 强制触发条件 (只要满足一个就触发 boundary)
	// ============================================================================

	// Condition 1: Task ambiguous (Task Understanding ambiguous = true)
	// TODO: 集成 Task Understanding 的 ambiguous 信号

	// Condition 2: Short but hard signal
	assessment.ShortButHard = promptLen < 100 && r.hasHighDifficultySignals(promptLower)
	if assessment.ShortButHard {
		assessment.BoundaryEligible = true
		assessment.UncertaintyScore += 0.3
		assessment.Reasons = append(assessment.Reasons, "short_but_hard")
	}

	// Condition 3: Long but mechanical signal
	assessment.LongButMechanical = promptLen > 500 && r.isMechanicalTask(promptLower)
	if assessment.LongButMechanical {
		assessment.BoundaryEligible = true
		assessment.UncertaintyScore += 0.3
		assessment.Reasons = append(assessment.Reasons, "long_but_mechanical")
	}

	// ============================================================================
	// 加权条件 (累加不确定性分数)
	// ============================================================================

	// Factor 1: Low rule confidence
	assessment.LowRuleConfidence = ruleDecision.Confidence < r.config.RuleConfidenceThreshold
	if assessment.LowRuleConfidence {
		assessment.UncertaintyScore += 0.2 * (1.0 - ruleDecision.Confidence)
		assessment.Reasons = append(assessment.Reasons, "low_rule_confidence")
	}

	// Factor 2: Near tier boundaries (且规则 tier 与复杂度分数不一致)
	assessment.MultiStep = r.isNearBoundary(complexityScore, boundaryMargin)
	if assessment.MultiStep {
		// 检查规则 tier 是否已经与复杂度分数一致
		ruleTierMatchesComplexity := false
		if complexityScore < r.config.WeakMediumBoundary && ruleDecision.PreferredTier == TierWeak {
			ruleTierMatchesComplexity = true
		} else if complexityScore >= r.config.MediumStrongBoundary && ruleDecision.PreferredTier == TierStrong {
			ruleTierMatchesComplexity = true
		} else if complexityScore >= r.config.WeakMediumBoundary && complexityScore < r.config.MediumStrongBoundary && ruleDecision.PreferredTier == TierMedium {
			ruleTierMatchesComplexity = true
		}

		if !ruleTierMatchesComplexity {
			// 距离边界越近，不确定性越高
			distancePenalty := math.Max(0, 1.0-assessment.DistanceToBoundary/boundaryMargin)
			assessment.UncertaintyScore += 0.25 * distancePenalty
			assessment.Reasons = append(assessment.Reasons, "near_tier_boundary")
		} else {
			assessment.Reasons = append(assessment.Reasons, "rule_and_complexity_agreed")
		}
	}

	// Factor 3: Multi-intent
	assessment.MultiIntent = r.hasMultipleIntents(promptLower)
	if assessment.MultiIntent {
		assessment.UncertaintyScore += 0.15
		assessment.Reasons = append(assessment.Reasons, "multi_intent_task")
	}

	// Factor 4: Professional task with low tier
	// Professional multi-step work is always eligible for shadow comparison;
	// even a strong rule result can disagree with the learned scorer and is
	// valuable for calibration. Keep the legacy reason for compatibility.
	if r.isProfessionalTask(promptLower) {
		// Professional tasks are valuable shadow candidates even when the
		// rule tier is not weak. The default 0.3 threshold must be crossed so
		// multi-step domain work is compared by Hybrid consistently.
		assessment.UncertaintyScore += 0.3
		assessment.Reasons = append(assessment.Reasons, "professional_task_low_tier")
	}

	// Factor 5: Hard minimum tier tasks
	if r.hasHardMinimumSignals(promptLower) {
		assessment.UncertaintyScore += 0.15
		assessment.Reasons = append(assessment.Reasons, "hard_minimum_task")
	}

	// Factor 6: Policy adjusted but raw tier confidence is very low
	if policyAdjusted && ruleDecision.Confidence < 0.3 {
		assessment.UncertaintyScore += 0.15
		assessment.Reasons = append(assessment.Reasons, "policy_adjusted_low_confidence")
	}

	// ============================================================================
	// 判断边界是否触发
	// ============================================================================

	// 强制触发条件已经处理 above
	// 额外检查：如果 uncertainty_score 超过阈值，也触发
	if assessment.UncertaintyScore >= r.config.UncertaintyScoreThreshold {
		assessment.BoundaryEligible = true
	}

	// 限制 uncertainty_score 最大为 1.0
	assessment.UncertaintyScore = math.Min(assessment.UncertaintyScore, 1.0)

	// 去重 reasons
	seen := make(map[string]bool)
	uniqueReasons := []string{}
	for _, reason := range assessment.Reasons {
		if !seen[reason] {
			seen[reason] = true
			uniqueReasons = append(uniqueReasons, reason)
		}
	}
	assessment.Reasons = uniqueReasons

	return assessment
}

// checkBoundaryEligibility 旧函数，保留用于向后兼容
// 新代码应使用 assessTierUncertainty
func (r *BoundaryHybridTierRouter) checkBoundaryEligibility(prompt string, ruleDecision TierRouteDecision, complexityScore float64) BoundaryEligibility {
	eligibility := BoundaryEligibility{
		Eligible: false,
		Reasons:  []string{},
	}

	promptLower := strings.ToLower(prompt)
	promptLen := len(prompt)

	// Check 1: Low rule confidence
	if ruleDecision.Confidence < r.config.RuleConfidenceThreshold {
		eligibility.Eligible = true
		eligibility.Reasons = append(eligibility.Reasons, "low_rule_confidence")
	}

	// Check 2: Near tier boundaries
	// 注意: 只有当规则 tier 与边界不匹配时才触发边界检测
	// 如果规则已经是 Weak (score < 0.15) 或 Strong (score >= 0.50)，则不需要边界检测
	boundaryMargin := r.config.ThresholdMargin
	if r.isNearBoundary(complexityScore, boundaryMargin) {
		// 检查规则 tier 是否已经与复杂度分数一致
		ruleTierMatchesComplexity := false
		if complexityScore < r.config.WeakMediumBoundary && ruleDecision.PreferredTier == TierWeak {
			ruleTierMatchesComplexity = true // 分数低 + 规则建议 Weak = 一致
		} else if complexityScore >= r.config.MediumStrongBoundary && ruleDecision.PreferredTier == TierStrong {
			ruleTierMatchesComplexity = true // 分数高 + 规则建议 Strong = 一致
		} else if complexityScore >= r.config.WeakMediumBoundary && complexityScore < r.config.MediumStrongBoundary && ruleDecision.PreferredTier == TierMedium {
			ruleTierMatchesComplexity = true // 分数中等 + 规则建议 Medium = 一致
		}

		if !ruleTierMatchesComplexity {
			eligibility.Eligible = true
			eligibility.Reasons = append(eligibility.Reasons, "near_tier_boundary")
		} else {
			eligibility.Reasons = append(eligibility.Reasons, "rule_and_complexity_agreed")
		}
	}

	// Check 3: Professional task with low tier
	if r.isProfessionalTask(promptLower) && ruleDecision.PreferredTier == TierWeak {
		eligibility.Eligible = true
		eligibility.Reasons = append(eligibility.Reasons, "professional_task_low_tier")
	}

	// Check 4: Multi-intent / multi-step
	if r.hasMultipleIntents(promptLower) {
		eligibility.Eligible = true
		eligibility.Reasons = append(eligibility.Reasons, "multi_intent_task")
	}

	// Check 5: Short prompt with high difficulty signals
	if promptLen < 100 && r.hasHighDifficultySignals(promptLower) {
		eligibility.Eligible = true
		eligibility.Reasons = append(eligibility.Reasons, "short_but_difficult")
	}

	// Check 6: Long prompt but mechanical/simple
	if promptLen > 500 && r.isMechanicalTask(promptLower) {
		eligibility.Eligible = true
		eligibility.Reasons = append(eligibility.Reasons, "long_but_mechanical")
	}

	// Check 7: Hard minimum tier tasks (these should trigger to verify)
	if r.hasHardMinimumSignals(promptLower) {
		eligibility.Eligible = true
		eligibility.Reasons = append(eligibility.Reasons, "hard_minimum_task")
	}

	// Remove duplicates
	seen := make(map[string]bool)
	uniqueReasons := []string{}
	for _, reason := range eligibility.Reasons {
		if !seen[reason] {
			seen[reason] = true
			uniqueReasons = append(uniqueReasons, reason)
		}
	}
	eligibility.Reasons = uniqueReasons

	return eligibility
}

// isNearBoundary checks if complexity score is near a tier boundary
func (r *BoundaryHybridTierRouter) isNearBoundary(score, margin float64) bool {
	// Near weak-medium boundary
	if math.Abs(score-r.config.WeakMediumBoundary) < margin {
		return true
	}
	// Near medium-strong boundary
	if math.Abs(score-r.config.MediumStrongBoundary) < margin {
		return true
	}
	return false
}

// isProfessionalTask checks if prompt is a professional data science task
func (r *BoundaryHybridTierRouter) isProfessionalTask(prompt string) bool {
	for _, signal := range r.config.ProfessionalTaskSignals {
		if strings.Contains(prompt, strings.ToLower(signal)) {
			return true
		}
	}
	return false
}

// hasMultipleIntents checks for multi-intent signals
func (r *BoundaryHybridTierRouter) hasMultipleIntents(prompt string) bool {
	intentSignals := []string{"而且", "并且", "和", "以及", "同时", "另外", "此外",
		"and", "also", "plus", "additionally", "moreover"}

	count := 0
	for _, signal := range intentSignals {
		count += strings.Count(prompt, signal)
	}

	// Also check for numbered lists which often indicate multiple tasks
	for i := 2; i <= 5; i++ {
		if strings.Contains(prompt, fmt.Sprintf("%d.", i)) || strings.Contains(prompt, fmt.Sprintf("%d.", i)) {
			count++
		}
	}

	return count >= 2
}

// hasHighDifficultySignals checks for signals of difficult tasks in short prompts
func (r *BoundaryHybridTierRouter) hasHighDifficultySignals(prompt string) bool {
	hardSignals := []string{
		"竞态", "race condition", " deadlock", "死锁",
		"证明", "prove", "证明题", "数学",
		"架构", "architecture", "系统设计",
		"优化", "optimize", "性能", "performance",
		"并发", "concurrent", "parallel",
		"调试", "debug", "bug", "修复",
		"安全", "security", "漏洞", "vulnerability",
	}

	for _, signal := range hardSignals {
		if strings.Contains(prompt, signal) {
			return true
		}
	}
	return false
}

// isMechanicalTask checks if prompt is long but mechanically simple
func (r *BoundaryHybridTierRouter) isMechanicalTask(prompt string) bool {
	mechanicalSignals := []string{
		"翻译", "translate", "转换", "convert",
		"排版", "format", "模板", "template",
		"提取", "extract", "复制", "copy",
		"按照", "按照模板", "按固定",
	}

	count := 0
	for _, signal := range mechanicalSignals {
		if strings.Contains(prompt, signal) {
			count++
		}
	}

	return count >= 1
}

// hasHardMinimumSignals checks for tasks that should never be downgraded
func (r *BoundaryHybridTierRouter) hasHardMinimumSignals(prompt string) bool {
	for _, signal := range r.config.HardMinimumTierSignals {
		if strings.Contains(prompt, strings.ToLower(signal)) {
			return true
		}
	}
	return false
}

// determineMinimumTier determines the hard minimum tier based on task signals
func (r *BoundaryHybridTierRouter) determineMinimumTier(prompt string) (PreferredTier, string) {
	promptLower := strings.ToLower(prompt)

	// Security, architecture, disaster recovery -> Strong
	highRiskSignals := []string{
		"安全", "security", "安全审计", "容灾", "disaster recovery",
		"跨区域", "multi-region", "高并发", "百万", "million",
		"权限", "permission", "authorization", "认证",
		"架构", "architecture", "系统设计", "支付", "payment",
		"金融", "financial", "审计", "audit",
	}

	for _, signal := range highRiskSignals {
		if strings.Contains(promptLower, signal) {
			return TierStrong, "high_risk_task"
		}
	}

	// Professional data science tasks -> Medium minimum
	professionalSignals := []string{
		"kaggle", "机器学习", "machine learning", "预测模型",
		"深度学习", "deep learning", "notebook", "数据科学",
		"baseline", "验证", "validation", "融合", "ensemble",
	}

	for _, signal := range professionalSignals {
		if strings.Contains(promptLower, signal) {
			return TierMedium, "professional_data_task"
		}
	}

	// Code implementation requests -> Medium minimum
	codeSignals := []string{
		"implement", "实现", "写代码", "代码", "api", "接口",
		"登录", "login", "注册", "register",
	}

	for _, signal := range codeSignals {
		if strings.Contains(promptLower, signal) {
			return TierMedium, "code_implementation"
		}
	}

	return TierWeak, ""
}

// calculateComplexityScore calculates a complexity score for the prompt
func (r *BoundaryHybridTierRouter) calculateComplexityScore(prompt string, taskType TaskType) float64 {
	score := 0.0

	// Prompt length factor
	promptLen := len(prompt)
	if promptLen > 1000 {
		score += 2.0
	} else if promptLen > 500 {
		score += 1.0
	} else if promptLen > 200 {
		score += 0.5
	}

	// Multi-intent factor
	if r.hasMultipleIntents(strings.ToLower(prompt)) {
		score += 1.5
	}

	// Professional task factor
	if r.isProfessionalTask(strings.ToLower(prompt)) {
		score += 2.0
	}

	// Hard minimum task factor
	if r.hasHardMinimumSignals(strings.ToLower(prompt)) {
		score += 1.5
	}

	// High difficulty signals
	if r.hasHighDifficultySignals(strings.ToLower(prompt)) {
		score += 1.5
	}

	// Task type factor
	switch taskType {
	case TaskTypeVision, TaskTypeDocument:
		score += 1.0
	case TaskTypeCode:
		score += 0.5
	}

	// ============================================================================
	// 归一化处理：将原始分数 (0~10+) 归一化到 0~1 范围
	// 最大可能分数约为: 2.0 + 1.5 + 2.0 + 1.5 + 1.5 + 1.0 = 9.5
	// 除以 10.0 进行归一化，与边界阈值保持一致
	// ============================================================================
	const MaxComplexityScore = 10.0
	normalizedScore := math.Min(score/MaxComplexityScore, 1.0)

	return normalizedScore
}

// applyHybridPolicy applies the hybrid shadow policy to determine suggested tier
// 注意: complexityScore 已归一化到 0~1 范围
// - < 0.15 → Weak
// - 0.15 ~ 0.50 → Medium
// - ≥ 0.50 → Strong
func (r *BoundaryHybridTierRouter) applyHybridPolicy(ruleTier, minimumTier PreferredTier, probability float64, weakThreshold, strongThreshold float64, suggestedTier PreferredTier, prompt string, complexityScore float64) (PreferredTier, string) {
	promptLower := strings.ToLower(prompt)

	// ============================================================================
	// 统一量纲: 使用复杂度分数 (0~1) 优先判定
	// ============================================================================

	// Rule 0: 基于复杂度分数的判定（统一量纲后）
	if complexityScore >= 0.50 {
		// 复杂度分数 >= 0.50 -> Strong
		return TierStrong, "complexity_score_strong"
	} else if complexityScore < 0.15 {
		// 复杂度分数 < 0.15 -> Weak（如果规则没有强制要求更高）
		if minimumTier <= TierWeak && !r.isProfessionalTask(promptLower) && !r.hasHardMinimumSignals(promptLower) {
			return TierWeak, "complexity_score_weak"
		}
		// 否则使用 minimumTier 或 Medium
		if minimumTier > TierWeak {
			return minimumTier, "complexity_weak_blocked_by_minimum"
		}
		return TierMedium, "complexity_weak_default_medium"
	}

	// Rule 1: Hard minimum tier always wins
	if minimumTier >= TierStrong {
		return minimumTier, "hard_minimum_strong"
	}

	// Rule 2: RouteLLM suggests strong upgrade
	if probability >= strongThreshold {
		if ruleTier < TierStrong && minimumTier < TierStrong {
			return TierStrong, "routellm_strong_upgrade"
		}
		return ruleTier, "routellm_suggestion_blocked_by_rule"
	}

	// Rule 3: RouteLLM suggests weak downgrade
	if probability <= weakThreshold {
		// Only allow weak if:
		// - rule_tier is not strong
		// - not a professional task
		// - not a hard minimum task
		if ruleTier != TierStrong &&
			!r.isProfessionalTask(promptLower) &&
			!r.hasHardMinimumSignals(promptLower) &&
			minimumTier == TierWeak {
			return TierWeak, "routellm_weak_downgrade"
		}
		return ruleTier, "routellm_weak_blocked_by_minimum"
	}

	// Rule 4: Medium zone - keep rule tier
	return ruleTier, "routellm_medium_zone"
}

// Config returns the current configuration
func (r *BoundaryHybridTierRouter) Config() BoundaryHybridConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.config
}

// GetName returns the router name
func (r *BoundaryHybridTierRouter) GetName() string {
	return "boundary-hybrid-tier-router"
}

// SetConfig updates the configuration
func (r *BoundaryHybridTierRouter) SetConfig(config BoundaryHybridConfig) error {
	if err := config.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.config = config
	return nil
}

// Note: BoundaryHybridTierRouter has a different Route signature (with prompt parameter)
// than the standard TierRouter interface. It's designed specifically for playground
// use cases where prompt-based boundary detection is needed.
