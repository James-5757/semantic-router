package main

import (
	"context"
	"testing"

	semanticrouter "semantic-router"
)

func TestDomesticModelRankingExample(t *testing.T) {
	service, err := semanticrouter.NewModelSelectionService(semanticrouter.NewPlatformRealScheduler("超讯科技"), semanticrouter.DefaultIntegrationConfig())
	if err != nil {
		t.Fatal(err)
	}
	response := service.Select(context.Background(), &semanticrouter.ModelSelectionRequest{
		ProtocolVersion: semanticrouter.ModelSelectorProtocolVersion,
		RequestID:       "token-cloud-demo-001",
		APIKeyID:        "key-demo-redacted",
		GroupID:         2006,
		Prompt:          "Create a Kaggle baseline, validation, ensemble and hyperparameter tuning plan",
	})
	if !response.Success || response.ModelRanking == nil {
		t.Fatalf("response=%+v", response)
	}
	t.Logf("model_ranking=%+v", response.ModelRanking)
}
