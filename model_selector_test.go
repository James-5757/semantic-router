package semanticrouter

import (
	"context"
	"testing"
)

func TestModelCandidateSelectorUsesGroupPoolAndCapability(t *testing.T) {
	selector := NewModelCandidateSelector(NewStaticModelRegistry([]*ModelCapabilityProfile{
		{
			ModelID:      "technical-code-model",
			Placements:   []ModelPlacement{{Group: "technical_models", Pools: []string{"code_pool"}, Tier: TierMedium}},
			Capabilities: map[string]bool{"code": true},
			Enabled:      true,
		},
		{
			ModelID:      "technical-data-model",
			Placements:   []ModelPlacement{{Group: "technical_models", Pools: []string{"data_pool"}, Tier: TierMedium}},
			Capabilities: map[string]bool{"data": true},
			Enabled:      true,
		},
	}))

	accounts := []*RealSchedulerAccount{
		{ID: 1, Model: "technical-data-model", Pool: "data_pool", Group: "technical_models", Status: "active", Schedulable: true, Tier: TierMedium, DataCapable: true, ConcurrencyLimit: 4},
		{ID: 2, Model: "technical-code-model", Pool: "code_pool", Group: "technical_models", Status: "active", Schedulable: true, Tier: TierMedium, CodeCapable: true, ConcurrencyLimit: 4},
	}

	result, err := selector.Select(context.Background(), accounts, &SchedulerSelectRequest{
		PreferredGroup: "technical_models",
		PreferredPool:  PoolCode,
		PreferredTier:  TierMedium,
	})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if result.Account.ID != 2 || result.Account.Model != "technical-code-model" {
		t.Fatalf("selected account/model = %d/%s, want 2/technical-code-model", result.Account.ID, result.Account.Model)
	}
}

func TestModelCandidateSelectorRanksLoadPriorityPriceAndID(t *testing.T) {
	selector := NewModelCandidateSelector(nil)
	accounts := []*RealSchedulerAccount{
		{ID: 3, Model: "a", Pool: "code_pool", Status: "active", Schedulable: true, Tier: TierMedium, CodeCapable: true, CurrentLoad: 1, Priority: 10, Price: 0.1, ConcurrencyLimit: 4},
		{ID: 2, Model: "b", Pool: "code_pool", Status: "active", Schedulable: true, Tier: TierMedium, CodeCapable: true, CurrentLoad: 0, Priority: 20, Price: 0.1, ConcurrencyLimit: 4},
		{ID: 1, Model: "c", Pool: "code_pool", Status: "active", Schedulable: true, Tier: TierMedium, CodeCapable: true, CurrentLoad: 0, Priority: 10, Price: 0.2, ConcurrencyLimit: 4},
	}
	result, err := selector.Select(context.Background(), accounts, &SchedulerSelectRequest{PreferredPool: PoolCode, PreferredTier: TierMedium})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if result.Account.ID != 1 {
		t.Fatalf("selected account = %d, want 1", result.Account.ID)
	}
}

func TestDefaultRealSchedulerDryRunUsesRegistrySelection(t *testing.T) {
	scheduler := NewDefaultRealSchedulerDryRun()
	result := scheduler.Select(&SchedulerSelectRequest{
		PreferredGroup:       "technical_models",
		PreferredPool:        PoolCode,
		PreferredTier:        TierMedium,
		RequiredCapabilities: RequiredCapabilities{},
	})
	if result.Error != nil {
		t.Fatalf("Select() error = %v", result.Error)
	}
	if result.SelectedAccountID == 0 || result.SelectedModel != "gpt-4.1-mini" {
		t.Fatalf("selected = %d/%s", result.SelectedAccountID, result.SelectedModel)
	}
	if result.DecisionSource != "model_registry_candidate_ranking" {
		t.Fatalf("decision source = %q", result.DecisionSource)
	}
	if len(result.CandidateDetails) == 0 || result.CandidateDetails[0].ModelID == "" {
		t.Fatalf("candidate profile details missing: %+v", result.CandidateDetails)
	}
}

func TestModelCandidateSelectorExposesPoolSpecificProfileScore(t *testing.T) {
	selector := NewModelCandidateSelector(NewStaticModelRegistry([]*ModelCapabilityProfile{
		{
			ModelID: "code-model", Provider: "test", Enabled: true,
			Placements:   []ModelPlacement{{Group: "technical_models", Pools: []string{"code_pool"}, Tier: TierStrong}},
			Capabilities: map[string]bool{"code": true}, CodingAgentScore: 0.91, ProfileSource: "test",
		},
	}))
	result, err := selector.Select(context.Background(), []*RealSchedulerAccount{{
		ID: 10, Model: "code-model", Pool: "code_pool", Group: "technical_models", Status: "active", Schedulable: true,
		Tier: TierStrong, CodeCapable: true,
	}}, &SchedulerSelectRequest{PreferredGroup: "technical_models", PreferredPool: PoolCode, PreferredTier: TierMedium})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if got := result.CandidateDetails[0].ProfileScore; got != 0.91 {
		t.Fatalf("profile score = %v, want 0.91", got)
	}
}

func TestModelCandidateSelectorRankingV2DifferentiatesRuntimeSignals(t *testing.T) {
	selector := NewModelCandidateSelector(NewStaticModelRegistry([]*ModelCapabilityProfile{
		{ModelID: "model-a", Provider: "a", Enabled: true, CodingAgentScore: 0.70, Capabilities: map[string]bool{"code": true}, Placements: []ModelPlacement{{Group: "technical_models", Pools: []string{"code_pool"}, Tier: TierMedium}}},
		{ModelID: "model-b", Provider: "b", Enabled: true, CodingAgentScore: 0.70, Capabilities: map[string]bool{"code": true}, Placements: []ModelPlacement{{Group: "technical_models", Pools: []string{"code_pool"}, Tier: TierMedium}}},
	}))
	result, err := selector.Select(context.Background(), []*RealSchedulerAccount{
		{ID: 1, Model: "model-a", Pool: "code_pool", Group: "technical_models", Status: "active", Schedulable: true, Tier: TierMedium, CodeCapable: true, CurrentLoad: 1, Priority: 10, Price: 0.10, ConcurrencyLimit: 8},
		{ID: 2, Model: "model-b", Pool: "code_pool", Group: "technical_models", Status: "active", Schedulable: true, Tier: TierMedium, CodeCapable: true, CurrentLoad: 4, Priority: 40, Price: 0.40, ConcurrencyLimit: 8},
	}, &SchedulerSelectRequest{PreferredGroup: "technical_models", PreferredPool: PoolCode, PreferredTier: TierMedium})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if len(result.CandidateDetails) != 2 {
		t.Fatalf("candidate details = %d, want 2", len(result.CandidateDetails))
	}
	first, second := result.CandidateDetails[0], result.CandidateDetails[1]
	if first.ProfileScore != second.ProfileScore {
		t.Fatalf("test setup profile scores differ: %v vs %v", first.ProfileScore, second.ProfileScore)
	}
	if first.FinalScore == second.FinalScore {
		t.Fatalf("ranking_v2 did not differentiate equal profile scores: %+v", result.CandidateDetails)
	}
	if first.RankingVersion != "candidate_ranking_v2" || first.CapabilityScore != first.ProfileScore {
		t.Fatalf("ranking metadata missing: %+v", first)
	}
}

func TestModelCandidateSelectorBlendsPoolAndTaskFit(t *testing.T) {
	selector := NewModelCandidateSelector(NewStaticModelRegistry([]*ModelCapabilityProfile{{
		ModelID: "reasoning-code-model", Provider: "test", Enabled: true,
		CodingAgentScore: 0.80, ReasoningScore: 0.96, ChineseScore: 0.90,
		Capabilities: map[string]bool{"code": true},
		Placements:   []ModelPlacement{{Group: "technical_models", Pools: []string{"code_pool"}, Tier: TierMedium}},
	}}))
	result, err := selector.Select(context.Background(), []*RealSchedulerAccount{{
		ID: 10, Model: "reasoning-code-model", Pool: "code_pool", Group: "technical_models", Status: "active", Schedulable: true,
		Tier: TierMedium, CodeCapable: true,
	}}, &SchedulerSelectRequest{PreferredGroup: "technical_models", PreferredPool: PoolCode, PreferredTier: TierMedium, TaskSignals: []string{"reasoning", "chinese"}})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	detail := result.CandidateDetails[0]
	if abs(detail.PoolScore-0.80) > 0.0001 || abs(detail.TaskFitScore-0.93) > 0.0001 {
		t.Fatalf("pool/task scores = %.2f/%.2f, want 0.80/0.93", detail.PoolScore, detail.TaskFitScore)
	}
	if detail.CapabilityScore <= detail.PoolScore || detail.CapabilityScore >= detail.TaskFitScore {
		t.Fatalf("capability score = %.3f, want a weighted blend between pool and task scores", detail.CapabilityScore)
	}
}

func TestTaskFitScoreUsesTechnicalSubtaskSignals(t *testing.T) {
	profile := &ModelCapabilityProfile{CodingAgentScore: 0.91, DataAnalysisScore: 0.88, DocumentScore: 0.84, ReasoningScore: 0.93}
	if got := taskFitScore(profile, []string{"api_design", "code_generation"}); abs(got-0.91) > 0.0001 {
		t.Fatalf("API task fit = %.2f, want code score 0.91", got)
	}
	if got := taskFitScore(profile, []string{"data_science", "machine_learning"}); abs(got-0.93) > 0.0001 {
		t.Fatalf("data-science task fit = %.2f, want reasoning score 0.93", got)
	}
	if got := taskFitScore(profile, []string{"document_processing"}); abs(got-0.84) > 0.0001 {
		t.Fatalf("document task fit = %.2f, want document score 0.84", got)
	}
}

func abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}
