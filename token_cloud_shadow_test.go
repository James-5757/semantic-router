package semanticrouter

import (
	"context"
	"testing"
	"time"
)

func startShadowTestAdapter(t *testing.T) (*TokenCloudShadowAdapter, func()) {
	t.Helper()
	config := DefaultIntegrationConfig()
	config.ListenAddress = "127.0.0.1:0"
	config.ConnectTimeout = time.Second
	config.RequestTimeout = time.Second
	service, err := NewModelSelectionService(NewDefaultRealSchedulerDryRun(), config)
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewModelSelectorTCPServer(service, config)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	config.ServiceAddress = server.Addr().String()
	return NewTokenCloudShadowAdapter(NewModelSelectorTCPClient(config)), func() { _ = server.Close() }
}

func TestTokenCloudShadowKeepsOldSchedulerResult(t *testing.T) {
	adapter, cleanup := startShadowTestAdapter(t)
	defer cleanup()
	old := &SchedulerSelectResult{SelectedAccountID: 106, SelectedModel: "old-model", PoolUsed: "default_pool", Layer: "old_scheduler"}
	result := adapter.Observe(context.Background(), func() *SchedulerSelectResult { return old }, &ModelSelectionRequest{RequestID: "shadow-1", GroupID: 7, Prompt: "implement a Python API", RequestedTier: string(TierMedium)})
	if result.Main() != old {
		t.Fatalf("shadow changed main result: got=%p want=%p", result.Main(), old)
	}
	if result.NewSuggestion == nil || !result.NewSuggestion.DryRun || result.NewSuggestion.UpstreamCalled {
		t.Fatalf("invalid shadow suggestion: %+v", result)
	}
	if result.ShadowError != nil {
		t.Fatal(result.ShadowError)
	}
}

func TestTokenCloudShadowFailureDoesNotChangeMain(t *testing.T) {
	config := DefaultIntegrationConfig()
	config.ServiceAddress = "127.0.0.1:1"
	config.ConnectTimeout = 20 * time.Millisecond
	config.RequestTimeout = 20 * time.Millisecond
	adapter := NewTokenCloudShadowAdapter(NewModelSelectorTCPClient(config))
	old := &SchedulerSelectResult{SelectedAccountID: 106, SelectedModel: "old-model", PoolUsed: "default_pool"}
	result := adapter.Observe(context.Background(), func() *SchedulerSelectResult { return old }, &ModelSelectionRequest{RequestID: "shadow-error", GroupID: 7, Prompt: "hello"})
	if result.Main() != old {
		t.Fatalf("shadow failure changed main result: %+v", result)
	}
	if result.ShadowError == nil {
		t.Fatal("expected shadow error")
	}
	if result.NewSuggestion != nil && result.NewSuggestion.UpstreamCalled {
		t.Fatal("shadow called upstream")
	}
}

func TestTokenCloudShadowPerformanceBaseline(t *testing.T) {
	adapter, cleanup := startShadowTestAdapter(t)
	defer cleanup()
	for i := 0; i < 50; i++ {
		result := adapter.Observe(context.Background(), func() *SchedulerSelectResult {
			return &SchedulerSelectResult{SelectedAccountID: 106, SelectedModel: "old-model", PoolUsed: "default_pool"}
		}, &ModelSelectionRequest{RequestID: "perf", GroupID: 7, Prompt: "implement a Python API", RequestedTier: string(TierMedium)})
		if result.ShadowError != nil {
			t.Fatal(result.ShadowError)
		}
	}
	snapshot := adapter.metrics.Snapshot()
	t.Logf("phase2 shadow baseline: total=%d success=%d error_rate=%.2f%% avg=%.2fms p95=%.2fms agreement=%.2f%%", snapshot.ShadowTotal, snapshot.ShadowSuccess, snapshot.ShadowErrorRate*100, snapshot.AverageShadowLatencyMs, snapshot.P95ShadowLatencyMs, snapshot.OldVsNewAgreementRate*100)
	if snapshot.ShadowError != 0 || snapshot.UpstreamCalledCount != 0 {
		t.Fatalf("unsafe baseline: %+v", snapshot)
	}
	if snapshot.P95ShadowLatencyMs >= 200 {
		t.Fatalf("phase2 local p95 exceeds 200ms: %+v", snapshot)
	}
}
