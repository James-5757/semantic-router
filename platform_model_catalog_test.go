package semanticrouter

import (
	"context"
	"strings"
	"testing"
)

func TestPlatformCatalogContainsConfiguredGroupsAndModels(t *testing.T) {
	groups := PlatformModelGroups()
	if len(groups) < 8 {
		t.Fatalf("platform groups = %d, want at least 8", len(groups))
	}
	foundGPT := false
	for _, group := range groups {
		for _, model := range group.Models {
			if model == "gpt-5.5" {
				foundGPT = true
			}
		}
	}
	if !foundGPT {
		t.Fatal("platform catalog missing gpt-5.5")
	}
}

func TestPlatformModelGroupRestrictsCandidates(t *testing.T) {
	selector := NewModelCandidateSelector(NewPlatformModelRegistry())
	accounts, err := NewPlatformAccountRepository().ListRoutingAccounts(context.Background(), nil)
	if err != nil {
		t.Fatalf("list platform accounts: %v", err)
	}
	result, err := selector.Select(context.Background(), accounts, &SchedulerSelectRequest{
		PreferredGroup: "technical_models", ModelGroup: "国外OPENAI分组", PreferredPool: PoolCode, PreferredTier: TierMedium,
	})
	if err != nil {
		t.Fatalf("select platform model: %v", err)
	}
	if result.Account == nil || result.Account.ModelGroup != "国外OPENAI分组" {
		t.Fatalf("selected account escaped model group: %+v", result.Account)
	}
	for _, candidate := range result.CandidateDetails {
		if candidate.ModelID == "" {
			t.Fatal("empty candidate model")
		}
	}
}

func TestPlatformCatalogProvidesGeneralChatCandidates(t *testing.T) {
	repository := NewPlatformAccountRepository()
	groupID := int64(2002) // 国外OPENAI分组
	accounts, err := repository.ListRoutingAccounts(context.Background(), &groupID)
	if err != nil {
		t.Fatalf("list platform accounts: %v", err)
	}
	selector := NewModelCandidateSelector(NewPlatformModelRegistry())
	result, err := selector.Select(context.Background(), accounts, &SchedulerSelectRequest{
		ModelGroup:    "国外OPENAI分组",
		PreferredPool: PoolCheap,
		PreferredTier: TierWeak,
	})
	if err != nil {
		t.Fatalf("general chat should have a same-group candidate: %v", err)
	}
	if result.Account == nil || result.Account.ID == 0 || result.Account.Model == "" {
		t.Fatalf("invalid general chat candidate: %+v", result.Account)
	}
}

func TestPlatformCatalogKeepsPhysicalModelGroupsDistinct(t *testing.T) {
	if got := platformPoolsForModel("gpt-5.4"); containsCatalogString(got, "cheap_chat_pool") {
		t.Fatalf("gpt-5.4 general placement = %v, want technical-only policy", got)
	}
	if got := platformPoolsForModel("gemini-2.5-flash"); containsCatalogString(got, "code_pool") {
		t.Fatalf("gemini-2.5-flash technical placement = %v, want general-only policy", got)
	}
	if got := platformPoolsForModel("gemini-2.5-flash"); !containsCatalogString(got, "cheap_chat_pool") {
		t.Fatalf("gemini-2.5-flash placements = %v, want general candidate", got)
	}
}

func TestPlatformCatalogVariedPoolsDoNotCollapseToOneModel(t *testing.T) {
	scheduler := NewPlatformRealScheduler("国外OPENAI分组")
	tests := []struct {
		pool          PreferredPool
		tier          PreferredTier
		wantSubstring string
	}{
		{PoolCode, TierMedium, "gpt-5"},
		{PoolData, TierMedium, "gpt-5"},
		{PoolCheap, TierWeak, "gemini"},
	}
	seen := map[string]bool{}
	for _, test := range tests {
		result := scheduler.Select(&SchedulerSelectRequest{ModelGroup: "国外OPENAI分组", PreferredGroup: PhysicalGroupForPool(test.pool), PreferredPool: test.pool, PreferredTier: test.tier})
		if result.Error != nil {
			t.Fatalf("select %s: %v", test.pool, result.Error)
		}
		if !strings.Contains(strings.ToLower(result.SelectedModel), test.wantSubstring) {
			t.Fatalf("pool %s selected %q, want model containing %q", test.pool, result.SelectedModel, test.wantSubstring)
		}
		seen[result.SelectedModel] = true
	}
	if len(seen) < 2 {
		t.Fatalf("varied pools collapsed to one model: %v", seen)
	}
}

func containsCatalogString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
