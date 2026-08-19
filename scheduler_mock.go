package semanticrouter

import (
	"fmt"
	"sync"
)

// SchedulerFacade Scheduler 接口抽象（用于测试）
type SchedulerFacade interface {
	Select(req *SchedulerSelectRequest) *SchedulerSelectResult
}

// SchedulerSelectRequest Scheduler 选择请求
type SchedulerSelectRequest struct {
	Model          string
	PreferredGroup string
	// ModelGroup is the API-key-scoped model group. When set, candidates from
	// other model groups are ineligible even if their pool/capabilities match.
	ModelGroup           string
	PreferredPool        PreferredPool
	PreferredTier        PreferredTier
	TaskType             TaskType
	// TaskSignals are prompt-derived ranking hints used only after group and
	// pool eligibility checks. They never expand the API-key model group.
	TaskSignals           []string
	RequiredCapabilities RequiredCapabilities
	PreviousResponseID   string
	SessionHash          string
	ContextTokens        int
	MaxOutputTokens      int
	RequiresStreaming    bool
	RequiresToolCall     bool
	// AllowedModelIDs is an optional Token Cloud group-level model allowlist.
	// Empty means all eligible models in the selected account group.
	AllowedModelIDs []string
}

// SchedulerSelectResult Scheduler 选择结果
type SchedulerSelectResult struct {
	SelectedAccountID int64
	SelectedModel     string
	Layer             string // previous_response_id, session_sticky, load_balance
	PoolUsed          string
	AccountHealth     string
	MatchedTier       string // weak, medium, strong
	CandidateCount    int
	CandidateModels   []string
	CandidateDetails  []ModelCandidateScore
	DecisionSource    string
	FallbackReason    string
	Error             error
}

// MockAccount Mock 账号
type MockAccount struct {
	ID              int64
	Name            string
	Pool            string // cheap_chat_pool, code_pool, vision_pool, document_pool
	Tier            PreferredTier
	VisionCapable   bool
	DocumentCapable bool
	Status          string // active, disabled
	Health          string // healthy, unhealthy
	Concurrency     int
	Model           string // 支持的模型
}

// MockScheduler 模拟 Scheduler
type MockScheduler struct {
	mu          sync.RWMutex
	accounts    map[int64]*MockAccount
	stickyMap   map[string]int64 // session_hash -> account_id
	prevRespMap map[string]int64 // previous_response_id -> account_id
}

// NewMockScheduler 创建模拟 Scheduler
func NewMockScheduler() *MockScheduler {
	return &MockScheduler{
		accounts:    make(map[int64]*MockAccount),
		stickyMap:   make(map[string]int64),
		prevRespMap: make(map[string]int64),
	}
}

// AddAccount 添加账号
func (s *MockScheduler) AddAccount(account *MockAccount) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accounts[account.ID] = account
}

// SetupMockAccounts 设置测试用的 mock 账号
func (s *MockScheduler) SetupMockAccounts() {
	accounts := []*MockAccount{
		// cheap_chat_pool - weak tier
		{
			ID:              1,
			Name:            "text-weak-account-1",
			Pool:            "cheap_chat_pool",
			Tier:            TierWeak,
			VisionCapable:   false,
			DocumentCapable: false,
			Status:          "active",
			Health:          "healthy",
			Concurrency:     10,
			Model:           "gpt-3.5-turbo",
		},
		{
			ID:              2,
			Name:            "text-weak-account-2",
			Pool:            "cheap_chat_pool",
			Tier:            TierWeak,
			VisionCapable:   false,
			DocumentCapable: false,
			Status:          "active",
			Health:          "healthy",
			Concurrency:     10,
			Model:           "gpt-3.5-turbo",
		},
		// cheap_chat_pool - medium tier (for gpt-3.5-turbo which is now medium)
		{
			ID:              3,
			Name:            "text-medium-account-1",
			Pool:            "cheap_chat_pool",
			Tier:            TierMedium,
			VisionCapable:   false,
			DocumentCapable: false,
			Status:          "active",
			Health:          "healthy",
			Concurrency:     10,
			Model:           "gpt-3.5-turbo",
		},
		// code_pool - medium tier
		{
			ID:              10,
			Name:            "code-medium-account-1",
			Pool:            "code_pool",
			Tier:            TierMedium,
			VisionCapable:   false,
			DocumentCapable: false,
			Status:          "active",
			Health:          "healthy",
			Concurrency:     8,
			Model:           "gpt-4",
		},
		// vision_pool - strong tier
		{
			ID:              20,
			Name:            "vision-strong-account-1",
			Pool:            "vision_pool",
			Tier:            TierStrong,
			VisionCapable:   true,
			DocumentCapable: false,
			Status:          "active",
			Health:          "healthy",
			Concurrency:     5,
			Model:           "gpt-4o",
		},
		{
			ID:              21,
			Name:            "vision-strong-account-2",
			Pool:            "vision_pool",
			Tier:            TierStrong,
			VisionCapable:   true,
			DocumentCapable: false,
			Status:          "active",
			Health:          "healthy",
			Concurrency:     5,
			Model:           "gpt-4o",
		},
		// document_pool - medium tier
		{
			ID:              30,
			Name:            "document-medium-account-1",
			Pool:            "document_pool",
			Tier:            TierMedium,
			VisionCapable:   false,
			DocumentCapable: true,
			Status:          "active",
			Health:          "healthy",
			Concurrency:     8,
			Model:           "gpt-4",
		},
		// disabled strong account
		{
			ID:              99,
			Name:            "disabled-strong-account",
			Pool:            "default_pool",
			Tier:            TierStrong,
			VisionCapable:   true,
			DocumentCapable: true,
			Status:          "disabled",
			Health:          "healthy",
			Concurrency:     5,
			Model:           "gpt-4",
		},
	}

	for _, acc := range accounts {
		s.AddAccount(acc)
	}
}

// BindStickySession 绑定粘性会话
func (s *MockScheduler) BindStickySession(sessionHash string, accountID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sessionHash != "" {
		s.stickyMap[sessionHash] = accountID
	}
}

// BindPreviousResponse 绑定 previous_response_id
func (s *MockScheduler) BindPreviousResponse(responseID string, accountID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if responseID != "" {
		s.prevRespMap[responseID] = accountID
	}
}

// GetStickyAccount 获取粘性账号
func (s *MockScheduler) GetStickyAccount(sessionHash string) (int64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	accountID, ok := s.stickyMap[sessionHash]
	return accountID, ok
}

// GetPreviousResponseAccount 获取 previous_response_id 对应的账号
func (s *MockScheduler) GetPreviousResponseAccount(responseID string) (int64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	accountID, ok := s.prevRespMap[responseID]
	return accountID, ok
}

// Select 实现 SchedulerFacade 接口
func (s *MockScheduler) Select(req *SchedulerSelectRequest) *SchedulerSelectResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 1. 尝试 previous_response_id 匹配
	if req.PreviousResponseID != "" {
		if accountID, ok := s.prevRespMap[req.PreviousResponseID]; ok {
			if account, exists := s.accounts[accountID]; exists {
				if s.isAccountCompatible(account, req) && account.Status == "active" {
					return &SchedulerSelectResult{
						SelectedAccountID: account.ID,
						SelectedModel:     account.Model,
						Layer:             "previous_response_id",
						PoolUsed:          account.Pool,
						AccountHealth:     account.Health,
						MatchedTier:       string(account.Tier),
					}
				}
				// previous_response_id 命中但能力不兼容，跳过
			}
		}
	}

	// 2. 尝试 session_hash 匹配
	if req.SessionHash != "" {
		if accountID, ok := s.stickyMap[req.SessionHash]; ok {
			if account, exists := s.accounts[accountID]; exists {
				if s.isAccountCompatible(account, req) && account.Status == "active" && account.Health == "healthy" {
					return &SchedulerSelectResult{
						SelectedAccountID: account.ID,
						SelectedModel:     account.Model,
						Layer:             "session_sticky",
						PoolUsed:          account.Pool,
						AccountHealth:     account.Health,
						MatchedTier:       string(account.Tier),
					}
				}
			}
		}
	}

	// 3. Load Balance - 根据 pool 和 tier 过滤
	var candidates []*MockAccount
	for _, account := range s.accounts {
		if account.Status != "active" {
			continue
		}
		if account.Health != "healthy" {
			continue
		}
		if !s.isAccountCompatible(account, req) {
			continue
		}
		// 按 pool 过滤
		if !s.isAccountInPool(account, req.PreferredPool) {
			continue
		}
		// 按 tier 过滤（弱模型优先选 weak，中等优先选 medium/strong，强模型优先选 strong）
		if !s.isAccountMatchTier(account, req.PreferredTier, req.PreferredPool) {
			continue
		}
		candidates = append(candidates, account)
	}

	if len(candidates) == 0 {
		return &SchedulerSelectResult{
			Error: fmt.Errorf("no available account"),
		}
	}

	// 选择第一个匹配的账号（简单实现）
	selected := candidates[0]
	return &SchedulerSelectResult{
		SelectedAccountID: selected.ID,
		SelectedModel:     selected.Model,
		Layer:             "load_balance",
		PoolUsed:          selected.Pool,
		AccountHealth:     selected.Health,
		MatchedTier:       string(selected.Tier),
	}
}

// isAccountCompatible 检查账号是否兼容请求
func (s *MockScheduler) isAccountCompatible(account *MockAccount, req *SchedulerSelectRequest) bool {
	// 检查视觉能力
	if req.RequiredCapabilities.VisionCapable && !account.VisionCapable {
		return false
	}
	// 检查文档能力
	if req.RequiredCapabilities.DocumentCapable && !account.DocumentCapable {
		return false
	}
	// 检查图片能力 - 需要同时满足：1) 有图片能力要求 2) 账号无视觉能力
	// ImageCapability 为空或 "none" 表示无需图片能力
	hasImageRequirement := req.RequiredCapabilities.ImageCapability != "" && req.RequiredCapabilities.ImageCapability != ImageCapabilityNone
	if hasImageRequirement && !account.VisionCapable {
		return false
	}
	return true
}

// isAccountInPool 检查账号是否在指定池中
func (s *MockScheduler) isAccountInPool(account *MockAccount, pool PreferredPool) bool {
	switch pool {
	case PoolDefault, PoolCheap:
		// 便宜池可以是 cheap_chat_pool
		return account.Pool == "cheap_chat_pool" || account.Pool == "default_pool"
	case PoolVision:
		return account.Pool == "vision_pool"
	case PoolDocument:
		return account.Pool == "document_pool"
	case PoolCode:
		return account.Pool == "code_pool"
	default:
		return true
	}
}

// isAccountMatchTier 检查账号是否匹配 tier
// 注意：当强模型请求没有 strong 账号时，允许 fallback 到 medium
// 对于 default pool，也允许 fallback 到 weak 账号
func (s *MockScheduler) isAccountMatchTier(account *MockAccount, tier PreferredTier, pool PreferredPool) bool {
	switch tier {
	case TierWeak:
		// 弱模型请求可以用 weak 或 medium
		return account.Tier == TierWeak || account.Tier == TierMedium
	case TierMedium:
		// 中等模型请求可以用 medium 或 strong
		return account.Tier == TierMedium || account.Tier == TierStrong
	case TierStrong:
		// 强模型请求优先用 strong，如果没有 strong 则 fallback 到 medium
		// 对于 default pool，还可以 fallback 到 weak
		if account.Tier == TierStrong || account.Tier == TierMedium {
			return true
		}
		// 对于 default pool，允许 weak 账号 fallback
		if pool == PoolDefault && account.Tier == TierWeak {
			return true
		}
		return false
	default:
		return true
	}
}

// GetAccounts 获取所有账号
func (s *MockScheduler) GetAccounts() []*MockAccount {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*MockAccount, 0, len(s.accounts))
	for _, acc := range s.accounts {
		result = append(result, acc)
	}
	return result
}

// 确保接口实现
var _ SchedulerFacade = (*MockScheduler)(nil)

// getPoolByPreferredPool 将 PreferredPool 转换为池名称
func getPoolByPreferredPool(pool PreferredPool) string {
	switch pool {
	case PoolDefault:
		return "default_pool"
	case PoolVision:
		return "vision_pool"
	case PoolDocument:
		return "document_pool"
	case PoolCode:
		return "code_pool"
	case PoolCheap:
		return "cheap_chat_pool"
	default:
		return "default_pool"
	}
}

// GetAccountByID 根据 ID 获取账号
func (s *MockScheduler) GetAccountByID(id int64) (*MockAccount, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	account, ok := s.accounts[id]
	return account, ok
}

// SetAccountDisabled 设置账号为 disabled
func (s *MockScheduler) SetAccountDisabled(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if account, ok := s.accounts[id]; ok {
		account.Status = "disabled"
	}
}

// SetAccountUnhealthy 设置账号为 unhealthy
func (s *MockScheduler) SetAccountUnhealthy(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if account, ok := s.accounts[id]; ok {
		account.Health = "unhealthy"
	}
}
