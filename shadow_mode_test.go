package semanticrouter

import (
	"context"
	"fmt"
	"testing"
	"time"
)

type fakeAccountRepository struct {
	accounts []*RealSchedulerAccount
	called   int
}

func (r *fakeAccountRepository) ListRoutingAccounts(ctx context.Context, groupID *int64) ([]*RealSchedulerAccount, error) {
	r.called++
	return cloneRealSchedulerAccounts(r.accounts), nil
}

type fixedScheduler struct {
	result *SchedulerSelectResult
}

func (s *fixedScheduler) Select(req *SchedulerSelectRequest) *SchedulerSelectResult {
	if s.result == nil {
		return &SchedulerSelectResult{Error: fmt.Errorf("no fixed result")}
	}
	copied := *s.result
	return &copied
}

type failingScheduler struct{}

func (s *failingScheduler) Select(req *SchedulerSelectRequest) *SchedulerSelectResult {
	return &SchedulerSelectResult{Error: fmt.Errorf("shadow scheduler failed")}
}

type panickingTierRouter struct{}

func (p *panickingTierRouter) Route(ctx interface{}, model string, taskType TaskType) (*TierRouteDecision, error) {
	panic("tier shadow failure")
}

func (p *panickingTierRouter) GetName() string { return "panicking-tier-router" }

type unsafeScheduler struct {
	result  *SchedulerSelectResult
	account *RealSchedulerAccount
	delay   time.Duration
}

func (s *unsafeScheduler) Select(req *SchedulerSelectRequest) *SchedulerSelectResult {
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	copied := *s.result
	return &copied
}

func (s *unsafeScheduler) GetAccountByID(id int64) (*RealSchedulerAccount, bool) {
	if s.account == nil || s.account.ID != id {
		return nil, false
	}
	copied := *s.account
	return &copied, true
}

type panickingLogWriter struct{}

func (p *panickingLogWriter) LogRoutingDecision(entry *RoutingDecisionLogEntry) error {
	panic("log writer failure")
}

func TestShadowModeOldSchedulerUnaffectedAndLogsSuggestion(t *testing.T) {
	groupID := int64(7)
	repo := &fakeAccountRepository{accounts: []*RealSchedulerAccount{
		{
			ID:               0,
			Name:             "invalid-zero",
			Status:           "active",
			Schedulable:      true,
			Pool:             "code_pool",
			Tier:             TierMedium,
			Model:            "zero",
			Priority:         1,
			Price:            0.01,
			CodeCapable:      true,
			ConcurrencyLimit: 1,
		},
		{
			ID:               301,
			Name:             "disabled-code",
			Status:           "disabled",
			Schedulable:      true,
			Pool:             "code_pool",
			Tier:             TierMedium,
			Model:            "disabled",
			Priority:         1,
			Price:            0.01,
			CodeCapable:      true,
			ConcurrencyLimit: 1,
		},
		{
			ID:               302,
			Name:             "active-code",
			Status:           "active",
			Schedulable:      true,
			Pool:             "code_pool",
			Tier:             TierMedium,
			Model:            "gpt-4.1-mini",
			Priority:         5,
			Price:            0.20,
			CodeCapable:      true,
			ConcurrencyLimit: 4,
		},
	}}

	oldScheduler := &fixedScheduler{result: &SchedulerSelectResult{
		SelectedAccountID: 88,
		SelectedModel:     "legacy-model",
		PoolUsed:          "legacy_pool",
		Layer:             "legacy_scheduler",
	}}
	logStore := NewInMemoryRoutingDecisionLogStore()
	shadow := NewShadowRouter(
		oldScheduler,
		NewRealSchedulerDryRunFromRepository(repo, &groupID),
		NewMultiLayerRouter(),
		NewRuleBasedTierRouter(),
		logStore,
	)

	result := shadow.Route(&ShadowModeRequest{
		RequestID: "req-shadow-1",
		APIKeyID:  42,
		GroupID:   &groupID,
		Model:     "gpt-4",
		RouteRequest: &RouteRequest{
			Model:  "gpt-4",
			Prompt: "帮我写一个排序算法",
		},
		OldSchedulerRequest: &SchedulerSelectRequest{
			Model:         "gpt-4",
			PreferredPool: PoolDefault,
			PreferredTier: TierMedium,
			TaskType:      TaskTypeText,
		},
	})

	if result.OldSchedulerResult == nil || result.OldSchedulerResult.SelectedAccountID != 88 {
		t.Fatalf("old scheduler result changed: %+v", result.OldSchedulerResult)
	}
	if result.MainResult() != result.OldSchedulerResult {
		t.Fatalf("main result must be the old scheduler result")
	}
	if result.Suggestion == nil {
		t.Fatalf("expected semantic-router shadow suggestion")
	}
	if result.Suggestion.SelectedAccountID == 0 {
		t.Fatalf("shadow suggestion selected account 0")
	}
	if result.Suggestion.SelectedAccountID == 301 {
		t.Fatalf("shadow suggestion selected disabled account")
	}
	if result.Suggestion.SelectedAccountID != 302 {
		t.Fatalf("shadow selected account = %d, want 302", result.Suggestion.SelectedAccountID)
	}
	if result.Suggestion.SelectedModel != "gpt-4.1-mini" {
		t.Fatalf("selected_model = %s", result.Suggestion.SelectedModel)
	}
	if result.Suggestion.PoolUsed != "code_pool" {
		t.Fatalf("selected_pool = %s, want code_pool", result.Suggestion.PoolUsed)
	}
	if result.Suggestion.Layer != "dry_run_load_balance" {
		t.Fatalf("scheduler_layer = %s, want dry_run_load_balance", result.Suggestion.Layer)
	}
	if repo.called == 0 {
		t.Fatalf("expected account repository to be read")
	}
	if logStore.Size() != 1 {
		t.Fatalf("log store size = %d, want 1", logStore.Size())
	}

	entries := logStore.Entries()
	entry := entries[0]
	if entry.RequestID != "req-shadow-1" || entry.APIKeyID != 42 || entry.GroupID != groupID {
		t.Fatalf("bad log identity: %+v", entry)
	}
	if entry.PromptHash == "" {
		t.Fatalf("prompt_hash is empty")
	}
	if entry.PreferredPool != PoolCode {
		t.Fatalf("preferred_pool = %s, want code", entry.PreferredPool)
	}
	if entry.PreferredTier == "" {
		t.Fatalf("preferred_tier is empty")
	}
	if entry.TaskType != TaskTypeCode {
		t.Fatalf("task_type = %s, want code", entry.TaskType)
	}
	if entry.Confidence <= 0 {
		t.Fatalf("confidence = %f, want > 0", entry.Confidence)
	}
	if len(entry.MatchedRules) == 0 {
		t.Fatalf("matched_rules is empty")
	}
	if len(entry.SemanticScores) == 0 {
		t.Fatalf("semantic_scores is empty")
	}
	if entry.FinalDecisionSource == "" {
		t.Fatalf("final_decision_source is empty")
	}
	if entry.SelectedAccountID != 302 || entry.SelectedModel != "gpt-4.1-mini" || entry.SchedulerLayer != "dry_run_load_balance" {
		t.Fatalf("bad shadow selected fields: %+v", entry)
	}
	if entry.OldSchedulerAccountID != 88 {
		t.Fatalf("old_scheduler_account_id = %d, want 88", entry.OldSchedulerAccountID)
	}
	if entry.OldSelectedAccountID != 88 || entry.NewSuggestedAccountID != 302 {
		t.Fatalf("bad old/new account comparison: %+v", entry)
	}
	if entry.OldSelectedModel != "legacy-model" || entry.NewSuggestedModel != "gpt-4.1-mini" {
		t.Fatalf("bad old/new model comparison: %+v", entry)
	}
	if entry.OldSelectedPool != "legacy_pool" || entry.NewSuggestedPool != "code_pool" || entry.IsAgree {
		t.Fatalf("bad old/new pool or agreement comparison: %+v", entry)
	}
	if entry.ShadowLatencyMs <= 0 || result.ShadowLatencyMs <= 0 {
		t.Fatalf("shadow latency was not recorded: result=%f entry=%f", result.ShadowLatencyMs, entry.ShadowLatencyMs)
	}
}

func TestShadowModeFailureDoesNotAffectOldScheduler(t *testing.T) {
	oldScheduler := &fixedScheduler{result: &SchedulerSelectResult{
		SelectedAccountID: 77,
		SelectedModel:     "legacy-model",
		PoolUsed:          "legacy_pool",
		Layer:             "legacy_scheduler",
	}}
	logStore := NewInMemoryRoutingDecisionLogStore()
	shadow := NewShadowRouter(
		oldScheduler,
		&failingScheduler{},
		NewMultiLayerRouter(),
		NewRuleBasedTierRouter(),
		logStore,
	)

	result := shadow.Route(&ShadowModeRequest{
		RequestID: "req-shadow-fail",
		Model:     "gpt-4",
		RouteRequest: &RouteRequest{
			Model:  "gpt-4",
			Prompt: "帮我写一个排序算法",
		},
	})

	if result.OldSchedulerResult == nil || result.OldSchedulerResult.SelectedAccountID != 77 {
		t.Fatalf("old scheduler result changed on shadow failure: %+v", result.OldSchedulerResult)
	}
	if result.ShadowError == nil {
		t.Fatalf("expected shadow error")
	}
	if result.MainResult() == nil || result.MainResult().SelectedAccountID != 77 {
		t.Fatalf("main result changed on shadow failure: %+v", result.MainResult())
	}
}

func TestShadowModePanicDoesNotAffectOldScheduler(t *testing.T) {
	oldScheduler := &fixedScheduler{result: &SchedulerSelectResult{
		SelectedAccountID: 66,
		SelectedModel:     "legacy-model",
		Layer:             "legacy_scheduler",
	}}
	shadow := NewShadowRouter(oldScheduler, &failingScheduler{}, NewMultiLayerRouter(), &panickingTierRouter{}, nil)

	result := shadow.Route(&ShadowModeRequest{
		RequestID:    "req-shadow-panic",
		Model:        "gpt-4",
		RouteRequest: &RouteRequest{Prompt: "帮我写一个排序算法"},
	})

	if result == nil || result.MainResult() == nil || result.MainResult().SelectedAccountID != 66 {
		t.Fatalf("old scheduler result was lost after shadow panic: %+v", result)
	}
	if result.ShadowError == nil {
		t.Fatalf("expected recovered shadow panic")
	}
}

func TestShadowModeLoggerPanicDoesNotAffectOldScheduler(t *testing.T) {
	oldScheduler := &fixedScheduler{result: &SchedulerSelectResult{SelectedAccountID: 65}}
	shadowScheduler := &unsafeScheduler{
		result:  &SchedulerSelectResult{SelectedAccountID: 302, PoolUsed: "code_pool"},
		account: &RealSchedulerAccount{ID: 302, Status: "active", Schedulable: true},
	}
	shadow := NewShadowRouter(oldScheduler, shadowScheduler, NewMultiLayerRouter(), NewRuleBasedTierRouter(), &panickingLogWriter{})

	result := shadow.Route(&ShadowModeRequest{RouteRequest: &RouteRequest{Prompt: "帮我写一个排序算法"}})
	if result.MainResult() == nil || result.MainResult().SelectedAccountID != 65 {
		t.Fatalf("logger panic changed main result: %+v", result.MainResult())
	}
	if result.ShadowError == nil {
		t.Fatalf("logger panic must be captured as ShadowError")
	}
}

func TestShadowModeTakeoverDisabledNeverOverridesMainResult(t *testing.T) {
	oldScheduler := &fixedScheduler{result: &SchedulerSelectResult{SelectedAccountID: 55, SelectedModel: "legacy"}}
	shadowScheduler := &unsafeScheduler{
		result:  &SchedulerSelectResult{SelectedAccountID: 302, SelectedModel: "new", PoolUsed: "code_pool"},
		account: &RealSchedulerAccount{ID: 302, Status: "active", Schedulable: true},
	}
	shadow := NewShadowRouter(oldScheduler, shadowScheduler, NewMultiLayerRouter(), NewRuleBasedTierRouter(), nil)
	if err := shadow.SetRuntimeConfig(DefaultSemanticRouterRuntimeConfig()); err != nil {
		t.Fatalf("set default shadow config: %v", err)
	}

	result := shadow.Route(&ShadowModeRequest{RouteRequest: &RouteRequest{Prompt: "帮我写一个排序算法"}})
	if result.MainResult().SelectedAccountID != 55 || result.MainResult().SelectedModel != "legacy" {
		t.Fatalf("takeover-disabled route replaced main result: %+v", result.MainResult())
	}
	if result.Suggestion == nil || result.Suggestion.SelectedAccountID != 302 {
		t.Fatalf("expected independent shadow suggestion: %+v", result.Suggestion)
	}
}

func TestShadowModeRuntimeConfigDefaults(t *testing.T) {
	config := DefaultSemanticRouterRuntimeConfig()
	if !config.SemanticRouterShadowEnabled || !config.SemanticRouterDryRunEnabled {
		t.Fatalf("shadow and dry-run must be enabled by default: %+v", config)
	}
	if config.SemanticRouterTakeoverEnabled {
		t.Fatalf("takeover must be disabled by default")
	}
	if config.TakeoverPercentage != 0 {
		t.Fatalf("takeover_percentage must be 0 by default")
	}
	// Setting takeover_enabled=true without percentage must be rejected
	badConfig := config
	badConfig.SemanticRouterTakeoverEnabled = true
	badConfig.TakeoverPercentage = 0
	if err := badConfig.Validate(); err == nil {
		t.Fatalf("takeover_enabled=true and takeover_percentage=0 must be rejected")
	}
	// Setting takeover enabled with percentage > 0 must be allowed
	goodConfig := config
	goodConfig.SemanticRouterTakeoverEnabled = true
	goodConfig.TakeoverPercentage = 5
	if err := goodConfig.Validate(); err != nil {
		t.Fatalf("takeover with percentage=5 must be valid: %v", err)
	}
	// 100% is allowed
	fullConfig := config
	fullConfig.SemanticRouterTakeoverEnabled = true
	fullConfig.TakeoverPercentage = 100
	if err := fullConfig.Validate(); err != nil {
		t.Fatalf("takeover with percentage=100 must be valid: %v", err)
	}
	// Percentage > 100 must be rejected
	invalidConfig := config
	invalidConfig.TakeoverPercentage = 200
	if err := invalidConfig.Validate(); err == nil {
		t.Fatalf("takeover_percentage=200 must be rejected")
	}
	// Percentage > 0 but takeover disabled must be rejected
	pctNoTakeover := config
	pctNoTakeover.TakeoverPercentage = 10
	if err := pctNoTakeover.Validate(); err == nil {
		t.Fatalf("takeover_percentage=10 with takeover_enabled=false must be rejected")
	}
}

func TestShadowModeStats(t *testing.T) {
	oldScheduler := &fixedScheduler{result: &SchedulerSelectResult{SelectedAccountID: 302}}
	active := &RealSchedulerAccount{ID: 302, Status: "active", Schedulable: true}
	shadowScheduler := &unsafeScheduler{
		result:  &SchedulerSelectResult{SelectedAccountID: 302, PoolUsed: "code_pool"},
		account: active,
		delay:   time.Millisecond,
	}
	shadow := NewShadowRouter(oldScheduler, shadowScheduler, NewMultiLayerRouter(), NewRuleBasedTierRouter(), nil)
	req := &ShadowModeRequest{RouteRequest: &RouteRequest{Prompt: "帮我写一个排序算法"}}

	shadow.Route(req)
	oldScheduler.result.SelectedAccountID = 999
	shadow.Route(req)

	shadowScheduler.result = &SchedulerSelectResult{SelectedAccountID: 0, PoolUsed: "code_pool"}
	shadow.Route(req)

	shadowScheduler.result = &SchedulerSelectResult{SelectedAccountID: 401, PoolUsed: "code_pool"}
	shadowScheduler.account = &RealSchedulerAccount{ID: 401, Status: "disabled", Schedulable: true}
	shadow.Route(req)

	stats := shadow.Stats()
	t.Logf("shadow_total=%d shadow_success=%d shadow_error=%d shadow_error_rate=%.2f shadow_latency_ms=%.2f average_shadow_latency_ms=%.2f p95_shadow_latency_ms=%.2f pool_suggestion_count=%v old_vs_new_agreement_rate=%.2f account_zero_count=%d disabled_account_selected_count=%d",
		stats.ShadowTotal, stats.ShadowSuccess, stats.ShadowError, stats.ShadowErrorRate,
		stats.ShadowLatencyMs, stats.AverageShadowLatencyMs, stats.P95ShadowLatencyMs,
		stats.PoolSuggestionCount, stats.OldVsNewAgreementRate, stats.AccountZeroCount, stats.DisabledAccountSelectedCount)
	if stats.ShadowTotal != 4 || stats.ShadowSuccess != 2 || stats.ShadowError != 2 {
		t.Fatalf("unexpected shadow totals: %+v", stats)
	}
	if stats.PoolSuggestionCount["code_pool"] != 4 {
		t.Fatalf("pool_suggestion_count = %+v", stats.PoolSuggestionCount)
	}
	if stats.OldVsNewAgreementRate != 0.5 {
		t.Fatalf("old_vs_new_agreement_rate = %f, want 0.5", stats.OldVsNewAgreementRate)
	}
	if stats.ShadowErrorRate != 0.5 {
		t.Fatalf("shadow_error_rate = %f, want 0.5", stats.ShadowErrorRate)
	}
	if stats.ShadowLatencyMs <= 0 || stats.AverageShadowLatencyMs <= 0 || stats.P95ShadowLatencyMs <= 0 {
		t.Fatalf("shadow latency metrics = %+v", stats)
	}
	if stats.AccountZeroCount != 1 || stats.DisabledAccountSelectedCount != 1 {
		t.Fatalf("safety counters = %+v", stats)
	}
}

// ============================================================
// Takeover tests
// ============================================================

func TestTakeoverDisabledAlwaysReturnsOldScheduler(t *testing.T) {
	oldScheduler := &fixedScheduler{result: &SchedulerSelectResult{SelectedAccountID: 55, SelectedModel: "legacy"}}
	shadowScheduler := &unsafeScheduler{
		result:  &SchedulerSelectResult{SelectedAccountID: 302, SelectedModel: "new", PoolUsed: "code_pool"},
		account: &RealSchedulerAccount{ID: 302, Status: "active", Schedulable: true},
	}
	shadow := NewShadowRouter(oldScheduler, shadowScheduler, NewMultiLayerRouter(), NewRuleBasedTierRouter(), nil)
	// takeover disabled by default
	req := &ShadowModeRequest{
		RequestID:    "takeover-disabled-1",
		Model:        "gpt-4",
		RouteRequest: &RouteRequest{Prompt: "帮我写一个排序算法"},
	}

	// Result 1: takeover disabled - should always return old scheduler
	result := shadow.Route(req)
	if result.MainResult().SelectedAccountID != 55 || result.MainResult().SelectedModel != "legacy" {
		t.Fatalf("takeover-disabled route should return old scheduler result: %+v", result.MainResult())
	}
	if result.TakeoverResult != nil {
		t.Fatalf("takeover-disabled should have nil TakeoverResult: %+v", result.TakeoverResult)
	}

	// Result 2: explicitly set TakeoverEnabled=false
	config := DefaultSemanticRouterRuntimeConfig()
	shadow.SetRuntimeConfig(config)
	result2 := shadow.Route(req)
	if result2.MainResult().SelectedAccountID != 55 {
		t.Fatalf("takeover-disabled config should return old scheduler: %+v", result2.MainResult())
	}
	if result2.TakeoverResult != nil {
		t.Fatalf("takeover-disabled config should have nil TakeoverResult")
	}
}

func TestTakeoverWithPercentageOnlyAffectsSubset(t *testing.T) {
	oldScheduler := &fixedScheduler{result: &SchedulerSelectResult{SelectedAccountID: 55, SelectedModel: "legacy"}}
	shadowScheduler := &unsafeScheduler{
		result:  &SchedulerSelectResult{SelectedAccountID: 302, SelectedModel: "new", PoolUsed: "code_pool"},
		account: &RealSchedulerAccount{ID: 302, Status: "active", Schedulable: true},
	}
	shadow := NewShadowRouter(oldScheduler, shadowScheduler, NewMultiLayerRouter(), NewRuleBasedTierRouter(), nil)
	err := shadow.SetRuntimeConfig(SemanticRouterRuntimeConfig{
		SemanticRouterShadowEnabled:   true,
		SemanticRouterDryRunEnabled:   true,
		SemanticRouterTakeoverEnabled: true,
		TakeoverPercentage:            5,
	})
	if err != nil {
		t.Fatalf("set 5%% takeover config: %v", err)
	}

	takeoverCount := 0
	totalRequests := 200
	for i := 0; i < totalRequests; i++ {
		req := &ShadowModeRequest{
			RequestID:    fmt.Sprintf("takeover-pct-%d", i),
			APIKeyID:     int64(i % 10),
			Model:        "gpt-4",
			RouteRequest: &RouteRequest{Prompt: "帮我写一个排序算法"},
		}
		result := shadow.Route(req)
		if result.TakeoverResult != nil {
			// Takeover only happens for subset
			if result.MainResult().SelectedAccountID != 302 {
				t.Fatalf("takeover should return new suggestion: %+v", result.MainResult())
			}
			takeoverCount++
		} else {
			// Non-takeover should still have suggestion
			if result.MainResult().SelectedAccountID != 55 {
				t.Fatalf("non-takeover should return old scheduler: %+v", result.MainResult())
			}
		}
	}
	// With 5% percentage, expect approximately 5% takeover
	// Allow a generous range: 0.5% - 15%
	if takeoverCount < 1 || takeoverCount > 30 {
		t.Fatalf("5%% takeover: expected ~10 takeovers, got %d/%d", takeoverCount, totalRequests)
	}
	t.Logf("5%% takeover: %d/%d requests used semantic-router (expected ~10)", takeoverCount, totalRequests)

	// Verify shadow is still recording ALL requests
	stats := shadow.Stats()
	if stats.ShadowTotal != uint64(totalRequests) {
		t.Fatalf("shadow_total = %d, want %d (shadow must record all requests)", stats.ShadowTotal, totalRequests)
	}
	if stats.ShadowSuccess != uint64(totalRequests) {
		t.Fatalf("shadow_success = %d, want %d", stats.ShadowSuccess, totalRequests)
	}
}

func TestTakeoverInvalidSuggestionFallsBackToOldScheduler(t *testing.T) {
	oldScheduler := &fixedScheduler{result: &SchedulerSelectResult{SelectedAccountID: 55, SelectedModel: "legacy"}}
	shadowScheduler := &failingScheduler{}
	shadow := NewShadowRouter(oldScheduler, shadowScheduler, NewMultiLayerRouter(), NewRuleBasedTierRouter(), nil)
	err := shadow.SetRuntimeConfig(SemanticRouterRuntimeConfig{
		SemanticRouterShadowEnabled:   true,
		SemanticRouterDryRunEnabled:   true,
		SemanticRouterTakeoverEnabled: true,
		TakeoverPercentage:            100,
	})
	if err != nil {
		t.Fatalf("set 100%% takeover config: %v", err)
	}

	// Request hash must hit takeover (100% means always)
	req := &ShadowModeRequest{
		RequestID:    "takeover-fallback-test",
		APIKeyID:     999,
		Model:        "gpt-4",
		RouteRequest: &RouteRequest{Prompt: "帮我写一个排序算法"},
	}
	result := shadow.Route(req)

	// Since shadow scheduler failed, MainResult should fall back to old scheduler
	if result.MainResult() == nil || result.MainResult().SelectedAccountID != 55 {
		t.Fatalf("takeover should fallback to old scheduler on error: %+v", result.MainResult())
	}
	// TakeoverResult should be set (to old scheduler fallback) so MainResult() can use it
	if result.TakeoverResult == nil {
		t.Fatalf("TakeoverResult should be set even during fallback: %+v", result)
	}
	if result.ShadowError == nil {
		t.Fatalf("shadow error must be set on takeover fallback")
	}
}

func TestTakeoverSkipsAccountZero(t *testing.T) {
	oldScheduler := &fixedScheduler{result: &SchedulerSelectResult{SelectedAccountID: 55, SelectedModel: "legacy"}}
	shadowScheduler := &unsafeScheduler{
		result:  &SchedulerSelectResult{SelectedAccountID: 0, PoolUsed: "code_pool"},
		account: &RealSchedulerAccount{ID: 0, Status: "active", Schedulable: true},
	}
	shadow := NewShadowRouter(oldScheduler, shadowScheduler, NewMultiLayerRouter(), NewRuleBasedTierRouter(), nil)
	err := shadow.SetRuntimeConfig(SemanticRouterRuntimeConfig{
		SemanticRouterShadowEnabled:   true,
		SemanticRouterDryRunEnabled:   true,
		SemanticRouterTakeoverEnabled: true,
		TakeoverPercentage:            100,
	})
	if err != nil {
		t.Fatalf("set 100%% takeover config: %v", err)
	}

	req := &ShadowModeRequest{
		RequestID:    "takeover-account-zero",
		APIKeyID:     1,
		Model:        "gpt-4",
		RouteRequest: &RouteRequest{Prompt: "写段代码"},
	}
	result := shadow.Route(req)

	// account 0 should prevent takeover
	if result.TakeoverResult == nil {
		t.Fatalf("TakeoverResult must be set (even if fallback to old)")
	}
	if result.MainResult().SelectedAccountID != 55 {
		t.Fatalf("account zero should fallback to old scheduler: %+v", result.MainResult())
	}
}

func TestTakeoverSkipsDisabledAccount(t *testing.T) {
	oldScheduler := &fixedScheduler{result: &SchedulerSelectResult{SelectedAccountID: 55, SelectedModel: "legacy"}}
	shadowScheduler := &unsafeScheduler{
		result:  &SchedulerSelectResult{SelectedAccountID: 301, PoolUsed: "code_pool"},
		account: &RealSchedulerAccount{ID: 301, Status: "disabled", Schedulable: true},
	}
	shadow := NewShadowRouter(oldScheduler, shadowScheduler, NewMultiLayerRouter(), NewRuleBasedTierRouter(), nil)
	err := shadow.SetRuntimeConfig(SemanticRouterRuntimeConfig{
		SemanticRouterShadowEnabled:   true,
		SemanticRouterDryRunEnabled:   true,
		SemanticRouterTakeoverEnabled: true,
		TakeoverPercentage:            100,
	})
	if err != nil {
		t.Fatalf("set 100%% takeover config: %v", err)
	}

	req := &ShadowModeRequest{
		RequestID:    "takeover-disabled-acct",
		APIKeyID:     1,
		Model:        "gpt-4",
		RouteRequest: &RouteRequest{Prompt: "写段代码"},
	}
	result := shadow.Route(req)

	// disabled account should prevent takeover
	if result.TakeoverResult == nil {
		t.Fatalf("TakeoverResult must be set (even if fallback to old)")
	}
	if result.MainResult().SelectedAccountID != 55 {
		t.Fatalf("disabled account should fallback to old scheduler: %+v", result.MainResult())
	}
}

func TestTakeoverKillSwitch(t *testing.T) {
	oldScheduler := &fixedScheduler{result: &SchedulerSelectResult{SelectedAccountID: 55, SelectedModel: "legacy"}}
	shadowScheduler := &unsafeScheduler{
		result:  &SchedulerSelectResult{SelectedAccountID: 302, SelectedModel: "new", PoolUsed: "code_pool"},
		account: &RealSchedulerAccount{ID: 302, Status: "active", Schedulable: true},
	}
	shadow := NewShadowRouter(oldScheduler, shadowScheduler, NewMultiLayerRouter(), NewRuleBasedTierRouter(), nil)

	// Start with 100% takeover
	err := shadow.SetRuntimeConfig(SemanticRouterRuntimeConfig{
		SemanticRouterShadowEnabled:   true,
		SemanticRouterDryRunEnabled:   true,
		SemanticRouterTakeoverEnabled: true,
		TakeoverPercentage:            100,
	})
	if err != nil {
		t.Fatalf("set 100%% takeover config: %v", err)
	}

	req := &ShadowModeRequest{
		RequestID:    "kill-switch-test",
		APIKeyID:     1,
		Model:        "gpt-4",
		RouteRequest: &RouteRequest{Prompt: "写段代码"},
	}
	result := shadow.Route(req)
	if result.MainResult().SelectedAccountID != 302 {
		t.Fatalf("100%% takeover should use new scheduler: %+v", result.MainResult())
	}

	// Kill switch: set takeover_enabled=false, percentage=0
	err = shadow.SetRuntimeConfig(SemanticRouterRuntimeConfig{
		SemanticRouterShadowEnabled:   true,
		SemanticRouterDryRunEnabled:   true,
		SemanticRouterTakeoverEnabled: false,
		TakeoverPercentage:            0,
	})
	if err != nil {
		t.Fatalf("disable takeover config: %v", err)
	}

	result2 := shadow.Route(req)
	if result2.MainResult().SelectedAccountID != 55 {
		t.Fatalf("after kill switch should return old scheduler: %+v", result2.MainResult())
	}
	if result2.TakeoverResult != nil {
		t.Fatalf("after kill switch TakeoverResult should be nil: %+v", result2.TakeoverResult)
	}
}

func TestTakeoverShouldTakeoverHashDistribution(t *testing.T) {
	config := SemanticRouterRuntimeConfig{
		SemanticRouterTakeoverEnabled: true,
		TakeoverPercentage:            30,
	}
	shadow := &ShadowRouter{config: config}

	total := 10000
	takeoverCount := 0
	for i := 0; i < total; i++ {
		req := &ShadowModeRequest{
			RequestID: fmt.Sprintf("hash-test-%d", i),
			APIKeyID:  int64(i),
		}
		if shadow.ShouldTakeover(req) {
			takeoverCount++
		}
	}
	// With 30% percentage, expect approximately 30%
	// Allow generous range: 25% - 35%
	rate := float64(takeoverCount) / float64(total)
	if rate < 0.25 || rate > 0.35 {
		t.Fatalf("30%% takeover: got %.2f%% (%d/%d), expected ~30%%", rate*100, takeoverCount, total)
	}
	t.Logf("30%% takeover hash distribution: %.2f%% (%d/%d)", rate*100, takeoverCount, total)
}

func TestTakeoverSameRequestAlwaysSameDecision(t *testing.T) {
	config := SemanticRouterRuntimeConfig{
		SemanticRouterTakeoverEnabled: true,
		TakeoverPercentage:            50,
	}
	shadow := &ShadowRouter{config: config}

	req := &ShadowModeRequest{
		RequestID: "consistent-request",
		APIKeyID:  42,
	}

	first := shadow.ShouldTakeover(req)
	// Same request should always get same result
	for i := 0; i < 100; i++ {
		if shadow.ShouldTakeover(req) != first {
			t.Fatalf("same request gave different takeover decisions on iteration %d", i)
		}
	}
}

func TestTakeoverDifferentRequestsDifferentDecisions(t *testing.T) {
	config := SemanticRouterRuntimeConfig{
		SemanticRouterTakeoverEnabled: true,
		TakeoverPercentage:            50,
	}
	shadow := &ShadowRouter{config: config}

	// With 50% percentage, different requests should produce a mix
	results := make(map[bool]int)
	for i := 0; i < 100; i++ {
		req := &ShadowModeRequest{
			RequestID: fmt.Sprintf("diff-request-%d", i),
			APIKeyID:  int64(i),
		}
		results[shadow.ShouldTakeover(req)]++
	}
	if results[true] == 0 || results[false] == 0 {
		t.Fatalf("expected a mix of takeover decisions with 50%%: %+v", results)
	}
	t.Logf("50%% mix: takeover=%d, no_takeover=%d", results[true], results[false])
}
