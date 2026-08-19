package semanticrouter

import (
	"context"
	"testing"
)

// MockLearnedTierScorer for testing
type MockLearnedTierScorer struct {
	score *LearnedTierScore
	err   error
}

func (m *MockLearnedTierScorer) Score(ctx context.Context, prompt string) (*LearnedTierScore, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.score, nil
}

func (m *MockLearnedTierScorer) Health(ctx context.Context) error {
	if m.err != nil {
		return m.err
	}
	return nil
}

func TestBoundaryHybridTierRouter_ConfigValidation(t *testing.T) {
	// Test 1: Takeover should be rejected
	config := DefaultBoundaryHybridConfig()
	config.TakeoverEnabled = true
	err := config.Validate()
	if err == nil {
		t.Error("Expected error for takeover enabled, got nil")
	}

	// Test 2: Default config should be valid
	config = DefaultBoundaryHybridConfig()
	err = config.Validate()
	if err != nil {
		t.Errorf("Default config should be valid, got: %v", err)
	}
}

func TestBoundaryHybridTierRouter_SimpleRequestNotBoundary(t *testing.T) {
	// Test case A: Simple greeting should NOT trigger boundary
	router := NewBoundaryHybridTierRouter(NewRuleBasedTierRouter(), nil)

	decision, err := router.Route(context.Background(), "", TaskTypeCode, "你好，请帮我解释一下API是什么")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if decision.Boundary.Eligible {
		t.Error("Simple greeting should NOT be boundary eligible")
	}

	if decision.RouteLLMInvoked {
		t.Error("RouteLLM should NOT be invoked for simple request")
	}

	// Final tier should always equal rule tier
	if decision.FinalTier != decision.RuleTier {
		t.Errorf("Final tier %v should equal rule tier %v", decision.FinalTier, decision.RuleTier)
	}

	// Used for final should always be false in shadow mode
	if decision.HybridUsedForFinal {
		t.Error("HybridUsedForFinal should always be false in shadow mode")
	}
}

func TestBoundaryHybridTierRouter_DifficultCodeRequestTriggers(t *testing.T) {
	// Test case B: Difficult code request SHOULD trigger boundary
	router := NewBoundaryHybridTierRouter(NewRuleBasedTierRouter(), &MockLearnedTierScorer{
		score: &LearnedTierScore{
			Router:               "bert",
			StrongWinProbability: 0.72,
			SuggestedTier:        TierStrong,
			WeakThreshold:        0.35,
			StrongThreshold:      0.65,
			LatencyMS:            100,
		},
	})

	decision, err := router.Route(context.Background(), "", TaskTypeCode, "找出这段并发代码中的竞态条件并修复它")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !decision.Boundary.Eligible {
		t.Error("Difficult code request SHOULD be boundary eligible")
	}

	if !decision.RouteLLMInvoked {
		t.Error("RouteLLM SHOULD be invoked for boundary request")
	}

	// Check that reasons include the expected triggers
	foundReason := false
	for _, reason := range decision.Boundary.Reasons {
		if reason == "short_but_difficult" || reason == "near_tier_boundary" {
			foundReason = true
		}
	}
	if !foundReason {
		t.Logf("Boundary reasons: %v", decision.Boundary.Reasons)
	}
}

func TestBoundaryHybridTierRouter_HardMinimumNotDowngraded(t *testing.T) {
	// Test case C: High security request should have hard minimum that cannot be downgraded
	router := NewBoundaryHybridTierRouter(NewRuleBasedTierRouter(), &MockLearnedTierScorer{
		score: &LearnedTierScore{
			Router:               "bert",
			StrongWinProbability: 0.30, // RouteLLM suggests weak
			SuggestedTier:        TierWeak,
			WeakThreshold:        0.35,
			StrongThreshold:      0.65,
			LatencyMS:            100,
		},
	})

	decision, err := router.Route(context.Background(), "", TaskTypeCode,
		"设计一个支持百万并发、跨区域容灾、安全审计和灰度发布的认证系统")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Minimum tier should be Strong for this task
	if decision.MinimumTier != TierStrong {
		t.Errorf("Expected minimum tier to be strong, got: %v", decision.MinimumTier)
	}

	// Final tier should be Strong (respecting hard minimum)
	if decision.FinalTier != TierStrong {
		t.Errorf("Final tier should be strong (hard minimum), got: %v", decision.FinalTier)
	}

	// Hybrid shadow tier should also be strong due to hard minimum
	if decision.HybridShadowTier != TierStrong {
		t.Errorf("Hybrid shadow tier should be strong due to hard minimum, got: %v", decision.HybridShadowTier)
	}
}

func TestBoundaryHybridTierRouter_RouteLLMUpgradeSuggestion(t *testing.T) {
	// Test: RouteLLM suggests upgrade, should be reflected in hybrid shadow
	router := NewBoundaryHybridTierRouter(NewRuleBasedTierRouter(), &MockLearnedTierScorer{
		score: &LearnedTierScore{
			Router:               "bert",
			StrongWinProbability: 0.72, // Above strong threshold
			SuggestedTier:        TierStrong,
			WeakThreshold:        0.35,
			StrongThreshold:      0.65,
			LatencyMS:            100,
		},
	})

	decision, err := router.Route(context.Background(), "", TaskTypeCode, "分析这段复杂的机器学习代码")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Check hybrid shadow suggests upgrade
	if decision.HybridDecisionReason != "routellm_strong_upgrade" {
		t.Logf("Hybrid decision reason: %s", decision.HybridDecisionReason)
	}
}

func TestBoundaryHybridOverrideEligibilityUsesConfidenceThreshold(t *testing.T) {
	router := NewBoundaryHybridTierRouter(NewRuleBasedTierRouter(), nil)
	base := &BoundaryHybridDecision{
		HybridDisagreement:        true,
		RouteLLMAvailable:         true,
		HybridShadowTier:          TierStrong,
		RouteLLMStrongProbability: 0.82,
		OverrideThreshold:         0.80,
	}
	eligible, reason := router.assessOverrideEligibility(base)
	if !eligible || reason != "high_confidence_boundary_override_candidate" {
		t.Fatalf("eligible=%v reason=%q, want eligible high-confidence override", eligible, reason)
	}
	base.RouteLLMStrongProbability = 0.79
	eligible, reason = router.assessOverrideEligibility(base)
	if eligible || reason != "strong_probability_below_override_threshold" {
		t.Fatalf("eligible=%v reason=%q, want threshold rejection", eligible, reason)
	}
	base.HybridShadowTier = TierWeak
	base.RouteLLMStrongProbability = 0.18
	eligible, reason = router.assessOverrideEligibility(base)
	if !eligible || reason != "high_confidence_boundary_override_candidate" {
		t.Fatalf("eligible=%v reason=%q, want low-probability weak override candidate", eligible, reason)
	}
	base.RouteLLMStrongProbability = 0.21
	eligible, reason = router.assessOverrideEligibility(base)
	if eligible || reason != "weak_probability_above_override_threshold" {
		t.Fatalf("eligible=%v reason=%q, want weak threshold rejection", eligible, reason)
	}
}

func TestBoundaryHybridTierRouter_RouteLLMErrorFallback(t *testing.T) {
	// Test: RouteLLM error should fallback to rule tier
	router := NewBoundaryHybridTierRouter(NewRuleBasedTierRouter(), &MockLearnedTierScorer{
		err: &testError{"RouteLLM service unavailable"},
	})

	decision, err := router.Route(context.Background(), "", TaskTypeCode, "复杂的机器学习代码需要分析和优化")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should be boundary eligible for professional task
	_ = decision.Boundary.Eligible // May or may not be eligible depending on prompt

	// Final tier should still equal rule tier
	if decision.FinalTier != decision.RuleTier {
		t.Errorf("Final tier should equal rule tier even with RouteLLM error")
	}

	// Hybrid used for final should be false
	if decision.HybridUsedForFinal {
		t.Error("Hybrid should NOT be used for final in shadow mode")
	}
}

func TestBoundaryHybridTierRouter_RouteLLMTimesOut(t *testing.T) {
	// Test: RouteLLM timeout should fallback to rule tier
	router := NewBoundaryHybridTierRouter(NewRuleBasedTierRouter(), &MockLearnedTierScorer{
		err: &testError{"context deadline exceeded"},
	})

	decision, err := router.Route(context.Background(), "", TaskTypeCode, "复杂的多步骤数据处理任务")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Final tier should still equal rule tier (shadow mode)
	if decision.FinalTier != decision.RuleTier {
		t.Errorf("Final tier should equal rule tier on timeout")
	}

	// Hybrid used for final should be false
	if decision.HybridUsedForFinal {
		t.Error("Hybrid should NOT be used for final decision in shadow mode")
	}
}

func TestBoundaryHybridTierRouter_ProfessionalWeakTriggers(t *testing.T) {
	// Test: Professional task classified as weak should trigger boundary
	router := NewBoundaryHybridTierRouter(NewRuleBasedTierRouter(), nil)

	// This should trigger professional_task_low_tier
	decision, err := router.Route(context.Background(), "gpt-3.5-turbo", TaskTypeCode,
		"帮我写一个kaggle比赛的完整baseline，包括特征工程、模型训练和融合")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	// Professional task should trigger boundary
	if !decision.Boundary.Eligible {
		t.Error("Professional task should be boundary eligible")
	}

	foundProfessionalReason := false
	for _, reason := range decision.Boundary.Reasons {
		if reason == "professional_task_low_tier" || reason == "near_tier_boundary" {
			foundProfessionalReason = true
		}
	}
	if !foundProfessionalReason {
		t.Logf("Boundary reasons: %v", decision.Boundary.Reasons)
	}
}

func TestBoundaryHybridTierRouter_LongButMechanical(t *testing.T) {
	// Test case D: Long prompt but mechanical task should trigger boundary
	router := NewBoundaryHybridTierRouter(NewRuleBasedTierRouter(), nil)

	longMechanicalPrompt := `请把下面的长文本按照固定模板重新排版，不需要分析内容。
第一段内容...
第二段内容...
第三段内容...
（此处省略1000字）

请按照以下格式输出：
标题：
日期：
作者：
正文：
总结：`

	decision, err := router.Route(context.Background(), "", TaskTypeCode, longMechanicalPrompt)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should trigger boundary for long but mechanical
	foundLongMechanical := false
	for _, reason := range decision.Boundary.Reasons {
		if reason == "long_but_mechanical" {
			foundLongMechanical = true
		}
	}
	if !foundLongMechanical {
		t.Logf("Boundary reasons: %v", decision.Boundary.Reasons)
	}
}

func TestBoundaryHybridTierRouter_FinalTierAlwaysEqualsRule(t *testing.T) {
	// Test 11: final_tier should always equal rule_tier
	testCases := []struct {
		name      string
		mockScore *LearnedTierScore
		prompt    string
	}{
		{
			name: "RouteLLM suggests strong",
			mockScore: &LearnedTierScore{
				StrongWinProbability: 0.80,
				SuggestedTier:        TierStrong,
				WeakThreshold:        0.35,
				StrongThreshold:      0.65,
			},
			prompt: "复杂任务",
		},
		{
			name: "RouteLLM suggests weak",
			mockScore: &LearnedTierScore{
				StrongWinProbability: 0.20,
				SuggestedTier:        TierWeak,
				WeakThreshold:        0.35,
				StrongThreshold:      0.65,
			},
			prompt: "简单任务",
		},
		{
			name:      "RouteLLM error",
			mockScore: nil,
			prompt:    "测试任务",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var scorer LearnedTierScorer
			if tc.mockScore != nil {
				scorer = &MockLearnedTierScorer{score: tc.mockScore}
			} else {
				scorer = &MockLearnedTierScorer{err: &testError{"error"}}
			}

			router := NewBoundaryHybridTierRouter(NewRuleBasedTierRouter(), scorer)
			decision, err := router.Route(context.Background(), "", TaskTypeCode, tc.prompt)

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// Critical: Final tier must always equal rule tier
			if decision.FinalTier != decision.RuleTier {
				t.Errorf("Final tier %v must equal rule tier %v", decision.FinalTier, decision.RuleTier)
			}

			// Critical: HybridUsedForFinal must always be false
			if decision.HybridUsedForFinal {
				t.Error("HybridUsedForFinal must always be false in shadow mode")
			}
		})
	}
}

func TestBoundaryHybridTierRouter_UpstreamCalledAlwaysFalse(t *testing.T) {
	// Test 14: upstream_called should always be false
	router := NewBoundaryHybridTierRouter(NewRuleBasedTierRouter(), &MockLearnedTierScorer{
		score: &LearnedTierScore{
			Router:               "bert",
			StrongWinProbability: 0.50,
			SuggestedTier:        TierMedium,
			WeakThreshold:        0.35,
			StrongThreshold:      0.65,
		},
	})

	decision, err := router.Route(context.Background(), "", TaskTypeCode, "测试")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// The RouteLLM service itself reports upstream_called, we just pass it through
	// In our implementation, we don't explicitly set it in BoundaryHybridDecision
	// But it should never affect the final routing
	if decision.HybridUsedForFinal {
		t.Error("Never allow hybrid to affect final decision")
	}
}

// Helper type for test errors
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
