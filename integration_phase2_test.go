package semanticrouter

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestPhase2TCPSelectionDryRun(t *testing.T) {
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
	defer server.Close()
	config.ServiceAddress = server.Addr().String()
	client := NewModelSelectorTCPClient(config)
	response, err := client.Select(context.Background(), &ModelSelectionRequest{ProtocolVersion: ModelSelectorProtocolVersion, RequestID: "phase2-code-1", GroupID: 7, Prompt: "implement a Python API function", ModelIDs: []string{"deepseek-coder"}, RequestedTier: string(TierStrong)})
	if err != nil {
		t.Fatal(err)
	}
	if !response.Success || response.SelectedAccountID != 203 || response.SelectedModel != "deepseek-coder" {
		t.Fatalf("unexpected response: %+v", response)
	}
	if !response.DryRun || response.UpstreamCalled || !response.ShadowOnly {
		t.Fatalf("unsafe response flags: %+v", response)
	}
	if response.SelectedPool != "code_pool" || response.SchedulerLayer == "" {
		t.Fatalf("unexpected scheduler result: %+v", response)
	}
}

func TestPhase2TCPRejectsInvalidRequestAndTakeover(t *testing.T) {
	config := DefaultIntegrationConfig()
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	config.TakeoverEnabled = true
	if err := config.Validate(); err == nil {
		t.Fatal("takeover must be rejected")
	}
	service, err := NewModelSelectionService(NewDefaultRealSchedulerDryRun(), DefaultIntegrationConfig())
	if err != nil {
		t.Fatal(err)
	}
	response := service.Select(context.Background(), &ModelSelectionRequest{RequestID: "bad", Prompt: "hello"})
	if response.Success || response.Error == "" {
		t.Fatalf("invalid request should fail: %+v", response)
	}
	if response.SelectedAccountID != 0 || response.UpstreamCalled {
		t.Fatalf("invalid request produced unsafe result: %+v", response)
	}
}

func TestPhase2TCPUnavailableFallsBackWithoutUpstream(t *testing.T) {
	config := DefaultIntegrationConfig()
	config.ServiceAddress = "127.0.0.1:1"
	config.ConnectTimeout = 20 * time.Millisecond
	config.RequestTimeout = 20 * time.Millisecond
	client := NewModelSelectorTCPClient(config)
	_, err := client.Select(context.Background(), &ModelSelectionRequest{RequestID: "unavailable", GroupID: 7, Prompt: "hello"})
	if err == nil {
		t.Fatal("unavailable selector should return an error")
	}
}

func TestIntegrationResponseIncludesStableModelRanking(t *testing.T) {
	service, err := NewModelSelectionService(NewPlatformRealScheduler("超讯科技"), DefaultIntegrationConfig())
	if err != nil {
		t.Fatal(err)
	}
	response := service.Select(context.Background(), &ModelSelectionRequest{
		ProtocolVersion: ModelSelectorProtocolVersion, RequestID: "domestic-data", GroupID: 2006,
		Prompt: "Create a Kaggle baseline, validation, ensemble and hyperparameter tuning plan",
	})
	if !response.Success || response.ModelRanking == nil {
		t.Fatalf("missing shadow model ranking: %+v", response)
	}
	ranking := response.ModelRanking
	if !ranking.ShadowOnly || ranking.UsedForFinal || ranking.RecommendedModel == "" || ranking.RecommendedAccountID == 0 {
		t.Fatalf("unsafe or invalid ranking: %+v", ranking)
	}
	if ranking.PhysicalGroup != "technical_models" || len(ranking.Candidates) < 2 {
		t.Fatalf("unexpected ranking shape: %+v", ranking)
	}
	if ranking.Candidates[0].Rank != 1 || ranking.Candidates[0].Model != ranking.RecommendedModel {
		t.Fatalf("ranking order invalid: %+v", ranking)
	}
	if ranking.Scoring.PoolWeight != 0.75 || ranking.Scoring.RuntimeWeight != 0.25 {
		t.Fatalf("ranking weights missing: %+v", ranking.Scoring)
	}
	encoded, err := json.Marshal(ranking.Candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "pool_score") || strings.Contains(string(encoded), "task_fit_score") {
		t.Fatalf("consumer candidate must expose only final_score: %s", encoded)
	}
}
