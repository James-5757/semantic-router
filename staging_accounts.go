package semanticrouter

import "context"

// StagingAccountRepository mirrors the production repository boundary while
// remaining deterministic and local. It is intended for staging shadow only.
type StagingAccountRepository struct {
	accounts []*RealSchedulerAccount
}

func NewStagingAccountRepository() *StagingAccountRepository {
	const groupID int64 = 1001
	return &StagingAccountRepository{accounts: []*RealSchedulerAccount{
		{ID: 1101, GroupID: groupID, Name: "stg-openai-code", Platform: "openai", Status: "active", Schedulable: true, Pool: "code_pool", Group: "technical_models", Tier: TierMedium, Model: "gpt-4.1-mini", Priority: 10, Price: 0.40, CodeCapable: true, DataCapable: true, DocumentCapable: true, CurrentLoad: 2, ConcurrencyLimit: 32},
		{ID: 1102, GroupID: groupID, Name: "stg-deepseek-coder", Platform: "deepseek", Status: "active", Schedulable: true, Pool: "code_pool", Group: "technical_models", Tier: TierStrong, Model: "deepseek-coder", Priority: 20, Price: 0.22, CodeCapable: true, CurrentLoad: 1, ConcurrencyLimit: 16},
		{ID: 1103, GroupID: groupID, Name: "stg-claude-code", Platform: "anthropic", Status: "active", Schedulable: true, Pool: "code_pool", Group: "technical_models", Tier: TierStrong, Model: "claude-3-7-sonnet", Priority: 30, Price: 0.85, CodeCapable: true, DataCapable: true, DocumentCapable: true, CurrentLoad: 4, ConcurrencyLimit: 12},
		{ID: 1104, GroupID: groupID, Name: "stg-deepseek-chat", Platform: "deepseek", Status: "active", Schedulable: true, Pool: "code_pool", Group: "technical_models", Tier: TierMedium, Model: "deepseek-chat", Priority: 40, Price: 0.18, CodeCapable: true, DataCapable: true, CurrentLoad: 3, ConcurrencyLimit: 24},
		{ID: 1105, GroupID: groupID, Name: "stg-qwen-plus", Platform: "qwen", Status: "active", Schedulable: true, Pool: "code_pool", Group: "technical_models", Tier: TierMedium, Model: "qwen-plus", Priority: 40, Price: 0.20, CodeCapable: true, DataCapable: true, DocumentCapable: true, CurrentLoad: 2, ConcurrencyLimit: 24},
		{ID: 1201, GroupID: groupID, Name: "stg-qwen-data", Platform: "qwen", Status: "active", Schedulable: true, Pool: "data_pool", Group: "technical_models", Tier: TierStrong, Model: "qwen-max", Priority: 10, Price: 0.35, CodeCapable: true, DataCapable: true, DocumentCapable: true, CurrentLoad: 2, ConcurrencyLimit: 24},
		{ID: 1202, GroupID: groupID, Name: "stg-openai-data", Platform: "openai", Status: "active", Schedulable: true, Pool: "data_pool", Group: "technical_models", Tier: TierMedium, Model: "gpt-4.1-mini", Priority: 20, Price: 0.40, CodeCapable: true, DataCapable: true, DocumentCapable: true, CurrentLoad: 1, ConcurrencyLimit: 32},
		{ID: 1301, GroupID: groupID, Name: "stg-qwen-vision", Platform: "qwen", Status: "active", Schedulable: true, Pool: "vision_pool", Group: "vision_models", Tier: TierStrong, Model: "qwen-vl-max", Priority: 10, Price: 0.48, DataCapable: true, VisionCapable: true, DocumentCapable: true, CurrentLoad: 2, ConcurrencyLimit: 16},
		{ID: 1302, GroupID: groupID, Name: "stg-openai-vision", Platform: "openai", Status: "active", Schedulable: true, Pool: "vision_pool", Group: "vision_models", Tier: TierStrong, Model: "gpt-4o", Priority: 20, Price: 0.70, CodeCapable: true, DataCapable: true, VisionCapable: true, DocumentCapable: true, CurrentLoad: 3, ConcurrencyLimit: 24},
		{ID: 1401, GroupID: groupID, Name: "stg-claude-document", Platform: "anthropic", Status: "active", Schedulable: true, Pool: "document_pool", Group: "technical_models", Tier: TierStrong, Model: "claude-3-5-sonnet", Priority: 10, Price: 0.75, CodeCapable: true, DataCapable: true, DocumentCapable: true, CurrentLoad: 1, ConcurrencyLimit: 12},
		{ID: 1402, GroupID: groupID, Name: "stg-openai-document", Platform: "openai", Status: "active", Schedulable: true, Pool: "document_pool", Group: "technical_models", Tier: TierMedium, Model: "gpt-4.1-mini", Priority: 20, Price: 0.40, DataCapable: true, DocumentCapable: true, CurrentLoad: 2, ConcurrencyLimit: 32},
		{ID: 1501, GroupID: groupID, Name: "stg-qwen-chat", Platform: "qwen", Status: "active", Schedulable: true, Pool: "cheap_chat_pool", Group: "general_chat_models", Tier: TierWeak, Model: "qwen-turbo", Priority: 10, Price: 0.08, CurrentLoad: 2, ConcurrencyLimit: 64},
		{ID: 1502, GroupID: groupID, Name: "stg-minimax-chat", Platform: "minimax", Status: "active", Schedulable: true, Pool: "default_pool", Group: "general_chat_models", Tier: TierMedium, Model: "MiniMax-Text-01", Priority: 20, Price: 0.18, CodeCapable: true, DataCapable: true, CurrentLoad: 1, ConcurrencyLimit: 32},
		{ID: 1503, GroupID: groupID, Name: "stg-openai-nano", Platform: "openai", Status: "active", Schedulable: true, Pool: "cheap_chat_pool", Group: "general_chat_models", Tier: TierWeak, Model: "gpt-4.1-nano", Priority: 20, Price: 0.05, CurrentLoad: 3, ConcurrencyLimit: 64},
		{ID: 1601, GroupID: groupID, Name: "stg-openai-image", Platform: "openai", Status: "active", Schedulable: true, Pool: "image_generation_pool", Group: "image_models", Tier: TierMedium, Model: "gpt-image-1", Priority: 10, Price: 0.55, VisionCapable: true, CurrentLoad: 1, ConcurrencyLimit: 16},
		{ID: 1701, GroupID: groupID, Name: "stg-disabled-code", Platform: "deepseek", Status: "disabled", Schedulable: true, Pool: "code_pool", Group: "technical_models", Tier: TierStrong, Model: "deepseek-reasoner", Priority: 1, Price: 0.20, CodeCapable: true, DataCapable: true, CurrentLoad: 0, ConcurrencyLimit: 16},
		{ID: 1702, GroupID: groupID, Name: "stg-unschedulable-data", Platform: "qwen", Status: "active", Schedulable: false, Pool: "data_pool", Group: "technical_models", Tier: TierStrong, Model: "qwen-max", Priority: 1, Price: 0.30, DataCapable: true, CurrentLoad: 0, ConcurrencyLimit: 16},
	}}
}

func (r *StagingAccountRepository) ListRoutingAccounts(_ context.Context, groupID *int64) ([]*RealSchedulerAccount, error) {
	accounts := make([]*RealSchedulerAccount, 0, len(r.accounts))
	for _, account := range r.accounts {
		if account == nil || (groupID != nil && account.GroupID != *groupID) {
			continue
		}
		copy := *account
		accounts = append(accounts, &copy)
	}
	return accounts, nil
}

func NewStagingRealScheduler(groupID int64) *RealSchedulerDryRun {
	repository := NewStagingAccountRepository()
	return NewRealSchedulerDryRunFromRepositoryWithRegistry(repository, &groupID, NewDefaultModelRegistry())
}
