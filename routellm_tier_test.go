package semanticrouter

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeLearnedTierScorer struct {
	score      *LearnedTierScore
	err        error
	panicValue any
}

func (f fakeLearnedTierScorer) Score(context.Context, string) (*LearnedTierScore, error) {
	if f.panicValue != nil {
		panic(f.panicValue)
	}
	return f.score, f.err
}
func (f fakeLearnedTierScorer) Health(context.Context) error { return f.err }

func TestTierForRouteLLMProbability(t *testing.T) {
	tests := []struct {
		probability float64
		want        PreferredTier
	}{
		{0.10, TierWeak}, {0.50, TierMedium}, {0.90, TierStrong},
	}
	for _, test := range tests {
		if got := TierForRouteLLMProbability(test.probability, 0.35, 0.65); got != test.want {
			t.Fatalf("score %.2f = %s, want %s", test.probability, got, test.want)
		}
	}
}

func TestHybridTierScorerShadowNeverOverridesRule(t *testing.T) {
	config := DefaultRouteLLMTierConfig()
	config.Enabled = true
	scorer := NewHybridTierScorer(config, fakeLearnedTierScorer{score: &LearnedTierScore{Router: "bert", StrongWinProbability: 0.99, SuggestedTier: TierStrong, WeakThreshold: 0.35, StrongThreshold: 0.65, LatencyMS: 4}})
	got := scorer.Decide(context.Background(), "simple greeting", TierWeak)
	if got.FinalTier != TierWeak || got.FinalTierSource != "rule_shadow" || got.IsAgreement {
		t.Fatalf("shadow decision unexpectedly changed rule: %+v", got)
	}
}

func TestHybridTierScorerFallbackOnErrorAndPanic(t *testing.T) {
	config := DefaultRouteLLMTierConfig()
	config.Enabled = true
	for _, learned := range []fakeLearnedTierScorer{{err: errors.New("service unavailable")}, {panicValue: "boom"}} {
		got := NewHybridTierScorer(config, learned).Decide(context.Background(), "prompt", TierMedium)
		if got.FinalTier != TierMedium || got.FinalTierSource != "rule_shadow" || got.RouteLLMError == "" {
			t.Fatalf("fallback unsafe: %+v", got)
		}
	}
}

func TestRouteLLMTierClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client := NewRouteLLMTierClient(RouteLLMTierConfig{ServiceURL: server.URL, TimeoutMS: 5})
	if _, err := client.Score(context.Background(), "prompt"); err == nil {
		t.Fatal("expected timeout")
	}
}

func TestRouteLLMShadowDoesNotAffectScheduler(t *testing.T) {
	scheduler := NewDefaultRealSchedulerDryRun()
	request := &SchedulerSelectRequest{PreferredPool: PoolCode, PreferredTier: TierMedium, TaskType: TaskTypeCode}
	before := scheduler.Select(request)
	config := DefaultRouteLLMTierConfig()
	config.Enabled = true
	_ = NewHybridTierScorer(config, fakeLearnedTierScorer{score: &LearnedTierScore{SuggestedTier: TierStrong}}).Decide(context.Background(), "complex prompt", TierMedium)
	after := scheduler.Select(request)
	if before.Error != nil || after.Error != nil || before.SelectedAccountID != after.SelectedAccountID {
		t.Fatalf("scheduler changed: before=%+v after=%+v", before, after)
	}
}
