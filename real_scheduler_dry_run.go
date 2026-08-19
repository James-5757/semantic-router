package semanticrouter

import (
	"context"
	"fmt"
	"sync"
)

type AccountRepository interface {
	ListRoutingAccounts(ctx context.Context, groupID *int64) ([]*RealSchedulerAccount, error)
}

// RealSchedulerAccount is the dry-run shape used to mirror production account
// scheduling fields without importing the backend Ent model into this module.
type RealSchedulerAccount struct {
	ID               int64
	GroupID          int64
	ModelGroup       string
	Name             string
	Platform         string
	Status           string
	Schedulable      bool
	Pool             string
	Group            string
	Tier             PreferredTier
	Model            string
	Priority         int
	Price            float64
	CodeCapable      bool
	DataCapable      bool
	VisionCapable    bool
	DocumentCapable  bool
	CurrentLoad      int
	ConcurrencyLimit int
}

type RealSchedulerDryRun struct {
	mu         sync.RWMutex
	accounts   []*RealSchedulerAccount
	repository AccountRepository
	groupID    *int64
	selector   *ModelCandidateSelector
}

func NewRealSchedulerDryRun(accounts []*RealSchedulerAccount) *RealSchedulerDryRun {
	copied := make([]*RealSchedulerAccount, 0, len(accounts))
	for _, account := range accounts {
		if account == nil {
			continue
		}
		cloned := *account
		copied = append(copied, &cloned)
	}
	return &RealSchedulerDryRun{accounts: copied}
}

func NewRealSchedulerDryRunWithRegistry(accounts []*RealSchedulerAccount, registry ModelRegistry) *RealSchedulerDryRun {
	scheduler := NewRealSchedulerDryRun(accounts)
	scheduler.selector = NewModelCandidateSelector(registry)
	return scheduler
}

func NewRealSchedulerDryRunFromRepository(repository AccountRepository, groupID *int64) *RealSchedulerDryRun {
	return &RealSchedulerDryRun{
		repository: repository,
		groupID:    groupID,
	}
}

func NewRealSchedulerDryRunFromRepositoryWithRegistry(repository AccountRepository, groupID *int64, registry ModelRegistry) *RealSchedulerDryRun {
	scheduler := NewRealSchedulerDryRunFromRepository(repository, groupID)
	scheduler.selector = NewModelCandidateSelector(registry)
	return scheduler
}

func NewDefaultRealSchedulerDryRun() *RealSchedulerDryRun {
	return NewRealSchedulerDryRunWithRegistry([]*RealSchedulerAccount{
		{
			ID:               101,
			Name:             "real-code-medium",
			Platform:         "openai",
			Status:           "active",
			Schedulable:      true,
			Pool:             "code_pool",
			Tier:             TierMedium,
			Model:            "gpt-4.1-mini",
			Priority:         10,
			CodeCapable:      true,
			ConcurrencyLimit: 8,
		},
		{
			ID:               102,
			Name:             "real-data-medium",
			Platform:         "openai",
			Status:           "active",
			Schedulable:      true,
			Pool:             "data_pool",
			Tier:             TierMedium,
			Model:            "gpt-4.1-mini",
			Priority:         10,
			DataCapable:      true,
			ConcurrencyLimit: 8,
		},
		{
			ID:               103,
			Name:             "real-vision-strong",
			Platform:         "openai",
			Status:           "active",
			Schedulable:      true,
			Pool:             "vision_pool",
			Tier:             TierStrong,
			Model:            "gpt-4o",
			Priority:         10,
			VisionCapable:    true,
			ConcurrencyLimit: 5,
		},
		{
			ID:               104,
			Name:             "real-document-medium",
			Platform:         "openai",
			Status:           "active",
			Schedulable:      true,
			Pool:             "document_pool",
			Tier:             TierMedium,
			Model:            "gpt-4.1-mini",
			Priority:         10,
			DocumentCapable:  true,
			ConcurrencyLimit: 8,
		},
		{
			ID:               105,
			Name:             "real-cheap-weak",
			Platform:         "openai",
			Status:           "active",
			Schedulable:      true,
			Pool:             "cheap_chat_pool",
			Tier:             TierWeak,
			Model:            "gpt-4.1-nano",
			Priority:         20,
			ConcurrencyLimit: 20,
		},
		{
			ID:               106,
			Name:             "real-default-medium",
			Platform:         "openai",
			Status:           "active",
			Schedulable:      true,
			Pool:             "default_pool",
			Tier:             TierMedium,
			Model:            "gpt-4.1-mini",
			Priority:         10,
			ConcurrencyLimit: 16,
		},
		{
			ID:               107,
			Name:             "real-image-generation-medium",
			Platform:         "openai",
			Status:           "active",
			Schedulable:      true,
			Pool:             "image_generation_pool",
			Tier:             TierMedium,
			Model:            "gpt-image-1",
			Priority:         10,
			VisionCapable:    true,
			ConcurrencyLimit: 8,
		},
		// Additional provider candidates. Priority 30 keeps the original
		// fixture deterministic while making the alternatives visible to the
		// candidate selector and Playground.
		{ID: 201, Name: "deepseek-code", Platform: "deepseek", Status: "active", Schedulable: true, Pool: "code_pool", Tier: TierMedium, Model: "deepseek-chat", Priority: 30, CodeCapable: true, DataCapable: true, ConcurrencyLimit: 8},
		{ID: 202, Name: "deepseek-reasoner-code", Platform: "deepseek", Status: "active", Schedulable: true, Pool: "code_pool", Tier: TierStrong, Model: "deepseek-reasoner", Priority: 30, CodeCapable: true, DataCapable: true, ConcurrencyLimit: 4},
		{ID: 203, Name: "deepseek-coder", Platform: "deepseek", Status: "active", Schedulable: true, Pool: "code_pool", Tier: TierStrong, Model: "deepseek-coder", Priority: 30, CodeCapable: true, ConcurrencyLimit: 4},
		{ID: 204, Name: "qwen-plus-code", Platform: "qwen", Status: "active", Schedulable: true, Pool: "code_pool", Tier: TierMedium, Model: "qwen-plus", Priority: 30, CodeCapable: true, DataCapable: true, DocumentCapable: true, ConcurrencyLimit: 8},
		{ID: 205, Name: "qwen-max-data", Platform: "qwen", Status: "active", Schedulable: true, Pool: "data_pool", Tier: TierStrong, Model: "qwen-max", Priority: 30, DataCapable: true, CodeCapable: true, DocumentCapable: true, ConcurrencyLimit: 4},
		{ID: 206, Name: "qwen-vl-vision", Platform: "qwen", Status: "active", Schedulable: true, Pool: "vision_pool", Tier: TierStrong, Model: "qwen-vl-max", Priority: 30, VisionCapable: true, DocumentCapable: true, DataCapable: true, ConcurrencyLimit: 4},
		{ID: 207, Name: "qwen-turbo-chat", Platform: "qwen", Status: "active", Schedulable: true, Pool: "cheap_chat_pool", Tier: TierWeak, Model: "qwen-turbo", Priority: 30, ConcurrencyLimit: 16},
		{ID: 208, Name: "minimax-text", Platform: "minimax", Status: "active", Schedulable: true, Pool: "default_pool", Tier: TierMedium, Model: "MiniMax-Text-01", Priority: 30, CodeCapable: true, DataCapable: true, ConcurrencyLimit: 8},
		{ID: 209, Name: "claude-sonnet-code", Platform: "anthropic", Status: "active", Schedulable: true, Pool: "code_pool", Tier: TierStrong, Model: "claude-3-5-sonnet", Priority: 30, CodeCapable: true, DataCapable: true, DocumentCapable: true, ConcurrencyLimit: 4},
		{ID: 210, Name: "claude-sonnet-latest-code", Platform: "anthropic", Status: "active", Schedulable: true, Pool: "code_pool", Tier: TierStrong, Model: "claude-3-7-sonnet", Priority: 30, CodeCapable: true, DataCapable: true, DocumentCapable: true, ConcurrencyLimit: 4},
		{
			ID:               999,
			Name:             "disabled-all-capable",
			Platform:         "openai",
			Status:           "disabled",
			Schedulable:      true,
			Pool:             "vision_pool",
			Tier:             TierStrong,
			Model:            "gpt-4o",
			Priority:         1,
			CodeCapable:      true,
			DataCapable:      true,
			VisionCapable:    true,
			DocumentCapable:  true,
			ConcurrencyLimit: 1,
		},
	}, NewDefaultModelRegistry())
}

func (s *RealSchedulerDryRun) Select(req *SchedulerSelectRequest) *SchedulerSelectResult {
	accounts, err := s.loadAccounts(context.Background())
	if err != nil {
		return &SchedulerSelectResult{Error: err}
	}

	selector := s.selector
	if selector == nil {
		selector = NewModelCandidateSelector(nil)
	}
	selection, err := selector.Select(context.Background(), accounts, req)
	if err != nil {
		return &SchedulerSelectResult{Error: err, FallbackReason: "no_eligible_model_candidate"}
	}
	selected := selection.Account
	if selected.ID == 0 {
		return &SchedulerSelectResult{Error: fmt.Errorf("invalid account id 0")}
	}

	return &SchedulerSelectResult{
		SelectedAccountID: selected.ID,
		SelectedModel:     selected.Model,
		Layer:             "dry_run_load_balance",
		PoolUsed:          selected.Pool,
		AccountHealth:     "healthy",
		MatchedTier:       string(selected.Tier),
		CandidateCount:    selection.CandidateCount,
		CandidateModels:   selection.CandidateModels,
		CandidateDetails:  selection.CandidateDetails,
		DecisionSource:    selection.DecisionSource,
	}
}

func (s *RealSchedulerDryRun) GetAccountByID(id int64) (*RealSchedulerAccount, bool) {
	accounts, err := s.loadAccounts(context.Background())
	if err != nil {
		return nil, false
	}
	for _, account := range accounts {
		if account.ID == id {
			return account, true
		}
	}
	return nil, false
}

func (s *RealSchedulerDryRun) loadAccounts(ctx context.Context) ([]*RealSchedulerAccount, error) {
	if s.repository != nil {
		accounts, err := s.repository.ListRoutingAccounts(ctx, s.groupID)
		if err != nil {
			return nil, err
		}
		return cloneRealSchedulerAccounts(accounts), nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneRealSchedulerAccounts(s.accounts), nil
}

func cloneRealSchedulerAccounts(accounts []*RealSchedulerAccount) []*RealSchedulerAccount {
	copied := make([]*RealSchedulerAccount, 0, len(accounts))
	for _, account := range accounts {
		if account == nil {
			continue
		}
		cloned := *account
		copied = append(copied, &cloned)
	}
	return copied
}

func isRealAccountAvailable(account *RealSchedulerAccount) bool {
	if account == nil || account.ID == 0 {
		return false
	}
	if account.Status != "active" || !account.Schedulable {
		return false
	}
	if account.ConcurrencyLimit > 0 && account.CurrentLoad >= account.ConcurrencyLimit {
		return false
	}
	return true
}

func isRealAccountPoolMatch(account *RealSchedulerAccount, pool PreferredPool) bool {
	switch pool {
	case PoolCode:
		return account.Pool == "code_pool"
	case PoolData:
		return account.Pool == "data_pool"
	case PoolVision:
		return account.Pool == "vision_pool"
	case PoolDocument:
		return account.Pool == "document_pool"
	case PoolCheap:
		return account.Pool == "cheap_chat_pool"
	case PoolDefault:
		return account.Pool == "default_pool" || account.Pool == "cheap_chat_pool"
	case PoolImageGeneration:
		return account.Pool == "image_generation_pool" || account.Pool == "vision_pool"
	default:
		return false
	}
}

func isRealAccountCapabilityMatch(account *RealSchedulerAccount, req *SchedulerSelectRequest) bool {
	switch req.PreferredPool {
	case PoolCode:
		if !account.CodeCapable {
			return false
		}
	case PoolData:
		if !account.DataCapable {
			return false
		}
	case PoolVision:
		if !account.VisionCapable {
			return false
		}
	case PoolDocument:
		if !account.DocumentCapable {
			return false
		}
	}

	if req.RequiredCapabilities.VisionCapable && !account.VisionCapable {
		return false
	}
	if req.RequiredCapabilities.DocumentCapable && !account.DocumentCapable {
		return false
	}
	if req.RequiredCapabilities.ImageCapability != "" &&
		req.RequiredCapabilities.ImageCapability != ImageCapabilityNone &&
		!account.VisionCapable {
		return false
	}
	return true
}

func isRealAccountTierMatch(account *RealSchedulerAccount, tier PreferredTier) bool {
	switch tier {
	case TierWeak:
		return account.Tier == TierWeak || account.Tier == TierMedium
	case TierMedium:
		return account.Tier == TierMedium || account.Tier == TierStrong
	case TierStrong:
		return account.Tier == TierStrong || account.Tier == TierMedium
	default:
		return true
	}
}

var _ SchedulerFacade = (*RealSchedulerDryRun)(nil)
