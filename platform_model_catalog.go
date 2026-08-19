package semanticrouter

import (
	"context"
	"sort"
	"strings"
)

// PlatformModelGroup is the non-secret catalog visible to an API-key group.
// API keys and provider credentials are deliberately not stored here.
type PlatformModelGroup struct {
	Name   string
	Models []string
}

// PlatformModelCatalog is a deterministic staging mirror of the platform
// model directory shown in the admin UI. Production should load this shape
// from the account/group repository.
var platformModelGroups = []PlatformModelGroup{
	{Name: "国外Anthropic分组", Models: []string{
		"claude-fable-5", "claude-haiku-4-5-20251001", "claude-haiku-4-5-20251001-thinking",
		"claude-opus-4-6-thinking", "claude-opus-4-7", "claude-opus-4-7-thinking", "claude-opus-4-8",
		"claude-sonnet-4-6", "claude-sonnet-4-6-thinking", "claude-sonnet-5",
	}},
	{Name: "jiecustome", Models: []string{"DeepSeek-V4-flash"}},
	{Name: "国外OPENAI分组", Models: []string{
		"gemini-2.5-flash", "gemini-2.5-pro", "gemini-3-flash-preview", "gemini-3.1-pro-preview",
		"gemini-3.5-flash", "gpt-5.4", "gpt-5.4-low", "gpt-5.4-mini", "gpt-5.5", "gpt-5.6-sol", "gpt-5.6-terra",
	}},
	{Name: "TH组订阅", Models: []string{"openthaigpt-1.6", "typhoon-v2.5"}},
	{Name: "测试客户组", Models: []string{
		"MiniMax-M2.5", "deepseek-v4-pro", "glm-5.2", "kimi-k2.6", "kimi-k2.7-code", "qwen3.7-max",
	}},
	{Name: "算力驿站分组", Models: []string{
		"DeepSeek-V4-flash", "claude-fable-5", "claude-haiku-4-5-20251001", "claude-haiku-4-5-20251001-thinking",
		"claude-opus-4-6", "claude-opus-4-6-thinking", "claude-opus-4-7", "claude-opus-4-7-thinking", "claude-opus-4-8",
		"claude-opus-5", "claude-sonnet-4-6", "claude-sonnet-4-6-thinking", "claude-sonnet-5",
		"gpt-5.4", "gpt-5.5", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-image-2",
	}},
	{Name: "超讯科技", Models: []string{
		"DeepSeek-V4-flash", "MiniMax-M2.5", "MiniMax-M2.7", "Qwen3.5-122B-A10B", "Qwen3.5-397B-A17B",
		"Qwen3.6-27B", "Qwen3.6-35B-A3B", "Step-3.7-Flash",
	}},
	{Name: "超讯闭源模型", Models: []string{
		"GPT-5.4", "GPT-5.5", "claude-fable-5", "claude-opus-4-6", "claude-opus-4-7", "claude-opus-4-8",
		"claude-sonnet-4-6", "claude-sonnet-5", "gpt-5.6-sol", "gpt-5.6-terra",
	}},
}

type PlatformAccountRepository struct {
	accounts []*RealSchedulerAccount
}

func NewPlatformAccountRepository() *PlatformAccountRepository {
	accounts := make([]*RealSchedulerAccount, 0)
	var id int64 = 21000
	for groupIndex, group := range platformModelGroups {
		for modelIndex, model := range group.Models {
			for _, pool := range platformPoolsForModel(model) {
				id++
				account := platformAccount(id, int64(2000+groupIndex), group.Name, model, pool, modelIndex)
				accounts = append(accounts, account)
			}
		}
	}
	return &PlatformAccountRepository{accounts: accounts}
}

func (r *PlatformAccountRepository) ListRoutingAccounts(_ context.Context, groupID *int64) ([]*RealSchedulerAccount, error) {
	result := make([]*RealSchedulerAccount, 0, len(r.accounts))
	for _, account := range r.accounts {
		if account == nil || (groupID != nil && account.GroupID != *groupID) {
			continue
		}
		copy := *account
		result = append(result, &copy)
	}
	return result, nil
}

func PlatformModelGroups() []PlatformModelGroup {
	groups := append([]PlatformModelGroup(nil), platformModelGroups...)
	for i := range groups {
		groups[i].Models = append([]string(nil), groups[i].Models...)
	}
	return groups
}

func NewPlatformRealScheduler(groupName string) *RealSchedulerDryRun {
	groupID := int64(2000)
	for index, group := range platformModelGroups {
		if group.Name == groupName {
			groupID = int64(2000 + index)
			break
		}
	}
	return NewRealSchedulerDryRunFromRepositoryWithRegistry(NewPlatformAccountRepository(), &groupID, NewPlatformModelRegistry())
}

func NewPlatformModelRegistry() ModelRegistry {
	profiles := make([]*ModelCapabilityProfile, 0)
	seen := make(map[string]bool)
	for _, group := range platformModelGroups {
		for _, model := range group.Models {
			if seen[model] {
				continue
			}
			seen[model] = true
			profiles = append(profiles, platformModelProfile(model))
		}
	}
	return NewStaticModelRegistry(profiles)
}

func platformPoolsForModel(model string) []string {
	lower := strings.ToLower(model)
	if strings.Contains(lower, "image") {
		return []string{"image_generation_pool"}
	}
	if strings.Contains(lower, "vl") {
		return []string{"vision_pool", "document_pool"}
	}

	// This is a deployment policy, not a claim that one model is incapable of
	// every other task. It keeps the physical candidate groups distinct during
	// shadow evaluation so a vague Pool decision cannot make every prompt pick
	// the same general-purpose model.
	if strings.Contains(lower, "gemini") && strings.Contains(lower, "flash") {
		return []string{"cheap_chat_pool", "default_pool"}
	}
	if strings.Contains(lower, "minimax") || strings.Contains(lower, "typhoon") {
		return []string{"cheap_chat_pool", "default_pool"}
	}
	if strings.Contains(lower, "gpt-5") || strings.Contains(lower, "claude") ||
		strings.Contains(lower, "deepseek") || strings.Contains(lower, "qwen") ||
		strings.Contains(lower, "kimi") || strings.Contains(lower, "glm") ||
		strings.Contains(lower, "gemini") {
		return []string{"code_pool", "data_pool", "document_pool"}
	}
	return []string{"cheap_chat_pool", "default_pool"}
}

func platformAccount(id, groupID int64, modelGroup, model, pool string, index int) *RealSchedulerAccount {
	profile := platformModelProfile(model)
	return &RealSchedulerAccount{
		ID: id, GroupID: groupID, ModelGroup: modelGroup, Name: modelGroup + ":" + model,
		Platform: profile.Provider, Status: "active", Schedulable: true, Pool: pool,
		Group: physicalGroupForPlatformPool(pool),
		Tier:  tierForPlatformModel(model), Model: model, Priority: 10 + index%5, Price: profile.CostPerTask,
		CodeCapable: profile.Capabilities["code"], DataCapable: profile.Capabilities["data"],
		VisionCapable: profile.Capabilities["vision"], DocumentCapable: profile.Capabilities["document"],
		CurrentLoad: index % 4, ConcurrencyLimit: 32,
	}
}

func platformModelProfile(model string) *ModelCapabilityProfile {
	lower := strings.ToLower(model)
	code, data, vision, document := 0.72, 0.70, 0.30, 0.55
	reasoning, chinese, longContext := 0.68, 0.62, 0.64
	if strings.Contains(lower, "claude") {
		code, data, document = 0.86, 0.82, 0.88
		reasoning, chinese, longContext = 0.90, 0.72, 0.92
	}
	if strings.Contains(lower, "gpt-5") || strings.Contains(lower, "gpt-5.") {
		code, data, document = 0.88, 0.84, 0.84
		reasoning, chinese, longContext = 0.90, 0.78, 0.86
	}
	if strings.Contains(lower, "deepseek") || strings.Contains(lower, "kimi") || strings.Contains(lower, "qwen") {
		code, data = 0.82, 0.80
		reasoning, chinese = 0.84, 0.86
	}
	if strings.Contains(lower, "code") || strings.Contains(lower, "reasoner") {
		code += 0.06
	}
	if strings.Contains(lower, "vl") || strings.Contains(lower, "gemini") {
		vision = 0.84
		longContext = 0.88
	}
	if strings.Contains(lower, "image") {
		vision = 0.88
	}
	// These are intentionally named routing priors, not benchmark claims. They
	// give different technical candidates distinct roles during shadow ranking.
	// Replace them with versioned provider/production evidence before takeover.
	switch lower {
	case "deepseek-v4-flash":
		code, data, document = 0.74, 0.72, 0.62
		reasoning, chinese, longContext = 0.72, 0.88, 0.62
	case "qwen3.5-122b-a10b":
		code, data, document = 0.84, 0.84, 0.76
		reasoning, chinese, longContext = 0.86, 0.91, 0.78
	case "qwen3.5-397b-a17b":
		code, data, document = 0.88, 0.91, 0.82
		reasoning, chinese, longContext = 0.94, 0.92, 0.88
	case "qwen3.6-27b":
		code, data, document = 0.82, 0.79, 0.70
		reasoning, chinese, longContext = 0.80, 0.90, 0.68
	case "qwen3.6-35b-a3b":
		code, data, document = 0.85, 0.85, 0.86
		reasoning, chinese, longContext = 0.88, 0.91, 0.91
	case "minimax-m2.5", "minimax-m2.7", "step-3.7-flash":
		code, data, document = 0.70, 0.68, 0.62
		reasoning, chinese, longContext = 0.70, 0.84, 0.70
	case "gpt-5.4":
		code, data, document = 0.88, 0.84, 0.84
		reasoning, chinese, longContext = 0.90, 0.78, 0.86
	case "gpt-5.4-low", "gpt-5.4-mini":
		code, data, document = 0.80, 0.76, 0.74
		reasoning, chinese, longContext = 0.78, 0.76, 0.72
	case "gpt-5.5":
		code, data, document = 0.91, 0.88, 0.85
		reasoning, chinese, longContext = 0.93, 0.80, 0.88
	case "gpt-5.6-sol":
		code, data, document = 0.87, 0.90, 0.82
		reasoning, chinese, longContext = 0.95, 0.81, 0.88
	case "gpt-5.6-terra":
		code, data, document = 0.89, 0.86, 0.91
		reasoning, chinese, longContext = 0.92, 0.79, 0.94
	case "gemini-2.5-pro", "gemini-3.1-pro-preview":
		code, data, document = 0.82, 0.88, 0.83
		reasoning, chinese, longContext = 0.88, 0.74, 0.93
	}
	return &ModelCapabilityProfile{
		ModelID: model, Provider: platformProvider(model), Enabled: true,
		Placements:       platformPlacements(model),
		Capabilities:     map[string]bool{"code": code >= 0.70, "data": data >= 0.70, "vision": vision >= 0.70, "document": document >= 0.70, "streaming": true, "json": true},
		CodingAgentScore: code, DataAnalysisScore: data, ReasoningScore: reasoning, ChineseScore: chinese, LongContextScore: longContext, VisionScore: vision, DocumentScore: document,
		GeneralScore: 0.75, CostPerTask: 0.25, ProfileSource: "platform_catalog_prior", EvidenceSource: "catalog_prior_not_benchmark", ProfileSnapshot: "2026-08-10",
		ScoreConfidence: 0.20, BenchmarkVersion: "routing-prior-v2", EvaluatedAt: "2026-08-12",
	}
}

func platformPlacements(model string) []ModelPlacement {
	tier := tierForPlatformModel(model)
	placements := make([]ModelPlacement, 0)
	for _, pool := range platformPoolsForModel(model) {
		preferred := preferredPoolName(pool)
		placements = append(placements, ModelPlacement{Group: PhysicalGroupForPool(preferred), Pools: []string{pool}, Tier: tier})
	}
	return placements
}

func preferredPoolName(pool string) PreferredPool {
	switch pool {
	case "code_pool":
		return PoolCode
	case "data_pool":
		return PoolData
	case "vision_pool":
		return PoolVision
	case "document_pool":
		return PoolDocument
	case "image_generation_pool":
		return PoolImageGeneration
	default:
		return PoolDefault
	}
}

func physicalGroupForPlatformPool(pool string) string {
	return PhysicalGroupForPool(preferredPoolName(pool))
}

func platformProvider(model string) string {
	lower := strings.ToLower(model)
	switch {
	case strings.Contains(lower, "claude"):
		return "anthropic"
	case strings.Contains(lower, "gpt"):
		return "openai"
	case strings.Contains(lower, "gemini"):
		return "google"
	case strings.Contains(lower, "deepseek"):
		return "deepseek"
	case strings.Contains(lower, "qwen"):
		return "qwen"
	case strings.Contains(lower, "minimax"):
		return "minimax"
	default:
		return "platform"
	}
}

func tierForPlatformModel(model string) PreferredTier {
	lower := strings.ToLower(model)
	if strings.Contains(lower, "opus") || strings.Contains(lower, "5.5") || strings.Contains(lower, "5.6") || strings.Contains(lower, "reasoner") || strings.Contains(lower, "397b") {
		return TierStrong
	}
	return TierMedium
}

func SortedPlatformGroupNames() []string {
	groups := PlatformModelGroups()
	result := make([]string, 0, len(groups))
	for _, group := range groups {
		result = append(result, group.Name)
	}
	sort.Strings(result)
	return result
}
