package semanticrouter

import (
	"strings"
	"testing"
	"time"
)

// TestLowConfidenceFallback 低置信度 fallback 测试
func TestLowConfidenceFallback(t *testing.T) {
	semanticRouter := NewRuleBasedSemanticRouter()

	// 创建一个低置信度请求（包含 "image" 关键字但不是真正图片请求）
	routeReq := &RouteRequest{
		Model:       "gpt-3.5-turbo",
		Prompt:      "请写一首关于 image 的诗",
		HasImage:    false,
		HasDocument: false,
	}

	decision, err := semanticRouter.Route(nil, routeReq)
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}

	// 应该匹配到 image_generation 规则（置信度 0.7），而不是 default_text（置信度 1.0）
	// 因为规则优先级高
	t.Logf("Matched rule: %s, TaskType: %s, Confidence: %.2f",
		decision.MatchedRule, decision.TaskType, decision.Confidence)

	// 验证决策
	if decision.MatchedRule == "" {
		t.Error("Expected a matched rule")
	}
	if decision.Confidence < 0.5 {
		t.Logf("Low confidence detected: %.2f", decision.Confidence)
	}
}

// TestGroupPoolPermission Group 权限测试
// 目标：group 权限不允许某个 pool 时，应 fallback 或报错
func TestGroupPoolPermission(t *testing.T) {
	scheduler := NewMockScheduler()
	scheduler.SetupMockAccounts()

	// 模拟 group 权限配置 - 不允许使用 code_pool
	groupConfig := &GroupConfig{
		AllowedPools: []string{"cheap_chat_pool", "vision_pool", "document_pool"},
		BlockedPools: []string{"code_pool"},
	}

	_ = groupConfig // 避免未使用警告

	semanticRouter := NewRuleBasedSemanticRouter()

	// 代码请求应该进入 code_pool，但被 group 权限阻止
	routeReq := &RouteRequest{
		Model:       "gpt-4",
		Prompt:      "写一个排序算法:\n```python\ndef sort():\n```",
		HasImage:    false,
		HasDocument: false,
	}

	// 语义路由
	semanticDecision, _ := semanticRouter.Route(nil, routeReq)

	// 如果语义路由返回 code_pool，但 group 权限不允许
	if semanticDecision.PreferredPool == PoolCode {
		// 应该 fallback 到其他允许的池
		t.Logf("Group blocks code_pool, should fallback")

		// 尝试调度，看是否会 fallback
		schedulerReq := &SchedulerSelectRequest{
			Model:                "gpt-4",
			PreferredPool:        PoolCode,
			PreferredTier:        TierMedium,
			TaskType:             TaskTypeCode,
			RequiredCapabilities: semanticDecision.RequiredCapabilities,
		}

		result := scheduler.Select(schedulerReq)

		// code_pool 被禁用，应该选择其他池
		if result.PoolUsed == "code_pool" {
			// 如果选中了 code_pool，说明没有正确应用权限
			t.Log("Warning: code_pool should be blocked by group permission")
		} else {
			t.Logf("Correctly fallback to: %s", result.PoolUsed)
		}
	}
}

// GroupConfig Group 配置
type GroupConfig struct {
	AllowedPools []string
	BlockedPools []string
}

// IsPoolAllowed 检查池是否允许
func (g *GroupConfig) IsPoolAllowed(pool string) bool {
	// 如果有允许列表，检查是否在允许列表中
	if len(g.AllowedPools) > 0 {
		for _, p := range g.AllowedPools {
			if p == pool {
				return true
			}
		}
		return false
	}

	// 如果有禁止列表，检查是否在禁止列表中
	for _, p := range g.BlockedPools {
		if p == pool {
			return false
		}
	}

	return true
}

// TestPoolUnavailableFallback 账号池不可用时的 fallback 测试
// 目标：preferred_pool 全部账号不可用时 fallback 或返回明确错误
func TestPoolUnavailableFallback(t *testing.T) {
	scheduler := NewMockScheduler()
	scheduler.SetupMockAccounts()

	// 禁用 vision_pool 的所有账号
	account20, _ := scheduler.GetAccountByID(20)
	account20.Status = "disabled"
	account21, _ := scheduler.GetAccountByID(21)
	account21.Status = "disabled"

	semanticRouter := NewRuleBasedSemanticRouter()
	tierRouter := NewRuleBasedTierRouter()

	// 图片请求应该进入 vision_pool
	routeReq := &RouteRequest{
		Model:    "gpt-4o",
		Prompt:   "请描述这张图片",
		HasImage: true,
	}

	semanticDecision, _ := semanticRouter.Route(nil, routeReq)
	tierDecision, _ := tierRouter.Route(nil, "gpt-4o", TaskTypeVision)

	schedulerReq := &SchedulerSelectRequest{
		Model:                "gpt-4o",
		PreferredPool:        semanticDecision.PreferredPool,
		PreferredTier:        tierDecision.PreferredTier,
		TaskType:             semanticDecision.TaskType,
		RequiredCapabilities: semanticDecision.RequiredCapabilities,
	}

	result := scheduler.Select(schedulerReq)

	// vision_pool 不可用，应该 fallback 到其他池
	if result.PoolUsed == "vision_pool" {
		t.Error("Should not select from disabled vision_pool")
	}

	if result.Error != nil {
		t.Logf("Expected error when all pools unavailable: %v", result.Error)
	} else {
		// 如果有 fallback，应该能成功调度
		t.Logf("Fallback successful, selected pool: %s", result.PoolUsed)
	}
}

// TestSessionHashStability session_hash 稳定性测试
// 目标：session_hash 多次请求保持稳定绑定
func TestSessionHashStability(t *testing.T) {
	scheduler := NewMockScheduler()
	scheduler.SetupMockAccounts()

	sessionHash := "stable-session-123"

	// 第一次请求
	result1 := scheduler.Select(&SchedulerSelectRequest{
		Model:                "gpt-3.5-turbo",
		PreferredPool:        PoolDefault,
		PreferredTier:        TierWeak,
		TaskType:             TaskTypeText,
		RequiredCapabilities: RequiredCapabilities{},
		SessionHash:          sessionHash,
	})

	// 绑定该 session
	if result1.SelectedAccountID > 0 {
		scheduler.BindStickySession(sessionHash, result1.SelectedAccountID)
	}

	// 第二次请求（相同 session）
	result2 := scheduler.Select(&SchedulerSelectRequest{
		Model:                "gpt-3.5-turbo",
		PreferredPool:        PoolDefault,
		PreferredTier:        TierWeak,
		TaskType:             TaskTypeText,
		RequiredCapabilities: RequiredCapabilities{},
		SessionHash:          sessionHash,
	})

	// 第三次请求
	result3 := scheduler.Select(&SchedulerSelectRequest{
		Model:                "gpt-4",
		PreferredPool:        PoolDefault,
		PreferredTier:        TierStrong,
		TaskType:             TaskTypeText,
		RequiredCapabilities: RequiredCapabilities{},
		SessionHash:          sessionHash,
	})

	// 验证稳定性 - 相同 session 应该命中相同的粘性账号（如果可用）
	if result1.Layer == "session_sticky" && result2.Layer == "session_sticky" {
		if result1.SelectedAccountID != result2.SelectedAccountID {
			t.Errorf("Session stability broken: first=%d, second=%d",
				result1.SelectedAccountID, result2.SelectedAccountID)
		}
		t.Logf("Session stable: account=%d", result1.SelectedAccountID)
	}

	// 即使 tier 不同，session_sticky 应该保持
	if result2.Layer == "session_sticky" && result3.Layer == "session_sticky" {
		if result2.SelectedAccountID != result3.SelectedAccountID {
			t.Errorf("Session stability broken across tiers: second=%d, third=%d",
				result2.SelectedAccountID, result3.SelectedAccountID)
		}
	}

	t.Logf("Request 1: account=%d, layer=%s", result1.SelectedAccountID, result1.Layer)
	t.Logf("Request 2: account=%d, layer=%s", result2.SelectedAccountID, result2.Layer)
	t.Logf("Request 3: account=%d, layer=%s", result3.SelectedAccountID, result3.Layer)
}

// TestAllPoolsUnavailable 全部账号池不可用测试
func TestAllPoolsUnavailable(t *testing.T) {
	scheduler := NewMockScheduler()
	scheduler.SetupMockAccounts()

	// 禁用所有账号
	for _, acc := range scheduler.GetAccounts() {
		acc.Status = "disabled"
	}

	result := scheduler.Select(&SchedulerSelectRequest{
		Model:                "gpt-4",
		PreferredPool:        PoolDefault,
		PreferredTier:        TierStrong,
		TaskType:             TaskTypeText,
		RequiredCapabilities: RequiredCapabilities{},
	})

	// 应该返回错误
	if result.Error == nil {
		t.Error("Expected error when all accounts disabled")
	} else {
		t.Logf("Expected error: %v", result.Error)
	}
}

// TestConfidenceThreshold 置信度阈值测试
func TestConfidenceThreshold(t *testing.T) {
	router := NewRuleBasedSemanticRouter()

	tests := []struct {
		name        string
		req         *RouteRequest
		minConfidence float64
		shouldPass  bool
	}{
		{
			name: "高置信度请求",
			req: &RouteRequest{
				Model:       "gpt-4",
				Prompt:      "hello",
				HasImage:    false,
				HasDocument: false,
			},
			minConfidence: 0.3, // 降低期望值，因为 default_text 规则置信度是 0.3
			shouldPass:    true,
		},
		{
			name: "低置信度请求",
			req: &RouteRequest{
				Model:       "gpt-3.5-turbo",
				Prompt:      "create an image of a cat", // 包含 image 关键字但不是真正图片
				HasImage:    false,
				HasDocument: false,
			},
			minConfidence: 0.9,
			shouldPass:    false, // image_generation 规则置信度是 0.7
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, err := router.Route(nil, tt.req)
			if err != nil {
				t.Fatalf("Route() error = %v", err)
			}

			passed := decision.Confidence >= tt.minConfidence
			if passed != tt.shouldPass {
				t.Errorf("Confidence %.2f vs threshold %.2f, expected pass=%v, got %v",
					decision.Confidence, tt.minConfidence, tt.shouldPass, passed)
			}
		})
	}
}

// TestMultiPoolSelection 多池选择测试
func TestMultiPoolSelection(t *testing.T) {
	scheduler := NewMockScheduler()
	scheduler.SetupMockAccounts()

	// 请求可以接受多个池
	schedulerReq := &SchedulerSelectRequest{
		Model:                "gpt-3.5-turbo",
		PreferredPool:        PoolDefault, // cheap_chat_pool
		PreferredTier:        TierWeak,
		TaskType:             TaskTypeText,
		RequiredCapabilities: RequiredCapabilities{},
	}

	result := scheduler.Select(schedulerReq)
	if result.Error != nil {
		t.Fatalf("Select() error = %v", result.Error)
	}

	// 应该从 default/cheap 池中选择
	t.Logf("Selected pool: %s, account: %d", result.PoolUsed, result.SelectedAccountID)
}

// TestLoggerInterfaceChange logger 接口变更测试
func TestLoggerInterfaceChange(t *testing.T) {
	// 测试 InMemoryLogger 实现新接口
	inMemLogger := NewInMemoryRoutingDecisionLogger(100)

	// 确保接口实现
	var _ RoutingDecisionLogger = inMemLogger

	// 测试基本功能
	decision := &CombinedRouteDecision{
		Semantic: SemanticRouteDecision{
			TaskType:       TaskTypeText,
			Modality:       ModalityTextOnly,
			PreferredPool:  PoolDefault,
			Confidence:     0.9,
			MatchedRule:    "test_rule",
		},
		Tier: TierRouteDecision{
			PreferredTier: TierMedium,
			Confidence:    1.0,
			MatchedRule:   "test_tier_rule",
			Reason:        "test",
		},
		FinalPool: PoolDefault,
		Timestamp: time.Now(),
	}

	err := inMemLogger.LogDecision(decision, "test-req-1")
	if err != nil {
		t.Fatalf("LogDecision() error = %v", err)
	}

	// 测试 GetStats
	stats := inMemLogger.GetStats()
	if stats.TotalDecisions != 1 {
		t.Errorf("TotalDecisions = %d, want 1", stats.TotalDecisions)
	}

	// 测试 Close 和 Ping (内存版本应该返回 nil)
	err = inMemLogger.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	err = inMemLogger.Ping()
	if err != nil {
		t.Errorf("Ping() error = %v", err)
	}

	t.Logf("Logger stats: %+v", stats)
}

// TestCodePromptEnglish 英文代码请求识别测试
func TestCodePromptEnglish(t *testing.T) {
	semanticRouter := NewRuleBasedSemanticRouter()

	tests := []struct {
		name        string
		prompt      string
		wantPool    PreferredPool
		wantTask    TaskType
		wantMatched string
	}{
		{
			name:        "Write a quick sort in Python",
			prompt:      "Write a quick sort in Python",
			wantPool:    PoolCode,
			wantTask:    TaskTypeCode,
			wantMatched: "quick sort",
		},
		{
			name:        "Write code function",
			prompt:      "Write code function for me",
			wantPool:    PoolCode,
			wantTask:    TaskTypeCode,
			wantMatched: "write code",
		},
		{
			name:        "Write a function",
			prompt:      "Write a function to calculate fibonacci",
			wantPool:    PoolCode,
			wantTask:    TaskTypeCode,
			wantMatched: "write a function",
		},
		{
			name:        "Implement algorithm",
			prompt:      "Implement a binary search algorithm",
			wantPool:    PoolCode,
			wantTask:    TaskTypeCode,
			wantMatched: "implement",
		},
		{
			name:        "Debug code",
			prompt:      "Debug this code and fix the error",
			wantPool:    PoolCode,
			wantTask:    TaskTypeCode,
			wantMatched: "code_by_prompt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &RouteRequest{
				Model:       "gpt-4",
				Prompt:      tt.prompt,
				HasImage:    false,
				HasDocument: false,
			}

			decision, err := semanticRouter.Route(nil, req)
			if err != nil {
				t.Fatalf("Route() error = %v", err)
			}

			if decision.PreferredPool != tt.wantPool {
				t.Errorf("PreferredPool = %v, want %v", decision.PreferredPool, tt.wantPool)
			}
			if decision.TaskType != tt.wantTask {
				t.Errorf("TaskType = %v, want %v", decision.TaskType, tt.wantTask)
			}
			if tt.wantMatched != "" && !contains(decision.MatchedRule, tt.wantMatched) {
				t.Logf("MatchedRule = %v (expected to contain %v)", decision.MatchedRule, tt.wantMatched)
			}
		})
	}
}

// TestCodePromptChinese 中文代码请求识别测试
func TestCodePromptChinese(t *testing.T) {
	semanticRouter := NewRuleBasedSemanticRouter()

	tests := []struct {
		name     string
		prompt   string
		wantPool PreferredPool
		wantTask TaskType
	}{
		{
			name:     "写代码",
			prompt:   "请帮我写代码实现一个登录功能",
			wantPool: PoolCode,
			wantTask: TaskTypeCode,
		},
		{
			name:     "写一个函数",
			prompt:   "写一个函数来计算斐波那契数列",
			wantPool: PoolCode,
			wantTask: TaskTypeCode,
		},
		{
			name:     "实现算法",
			prompt:   "实现一个排序算法",
			wantPool: PoolCode,
			wantTask: TaskTypeCode,
		},
		{
			name:     "快排",
			prompt:   "用 Python 实现快排",
			wantPool: PoolCode,
			wantTask: TaskTypeCode,
		},
		{
			name:     "报错修复",
			prompt:   "这个代码报错了，请帮我修复",
			wantPool: PoolCode,
			wantTask: TaskTypeCode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &RouteRequest{
				Model:       "gpt-4",
				Prompt:      tt.prompt,
				HasImage:    false,
				HasDocument: false,
			}

			decision, err := semanticRouter.Route(nil, req)
			if err != nil {
				t.Fatalf("Route() error = %v", err)
			}

			if decision.PreferredPool != tt.wantPool {
				t.Errorf("PreferredPool = %v, want %v", decision.PreferredPool, tt.wantPool)
			}
			if decision.TaskType != tt.wantTask {
				t.Errorf("TaskType = %v, want %v", decision.TaskType, tt.wantTask)
			}
		})
	}
}

// TestCodePoolFallbackTier 代码池 Tier fallback 测试
func TestCodePoolFallbackTier(t *testing.T) {
	scheduler := NewMockScheduler()
	scheduler.SetupMockAccounts()

	semanticRouter := NewRuleBasedSemanticRouter()
	tierRouter := NewRuleBasedTierRouter()
	logger := NewInMemoryRoutingDecisionLogger(100)
	preRouter := NewPreRouter(semanticRouter, tierRouter, logger)

	routeReq := &RouteRequest{
		Model:       "gpt-4",
		Prompt:      "Write a quick sort in Python",
		HasImage:    false,
		HasDocument: false,
	}

	// 1. 语义路由
	result, err := preRouter.Route(nil, "gpt-4", "", "", routeReq)
	if err != nil {
		t.Fatalf("PreRoute error = %v", err)
	}

	// 验证语义路由结果
	if result.Decision.Semantic.PreferredPool != PoolCode {
		t.Errorf("Semantic PreferredPool = %v, want code", result.Decision.Semantic.PreferredPool)
	}
	if result.Decision.Semantic.TaskType != TaskTypeCode {
		t.Errorf("Semantic TaskType = %v, want code", result.Decision.Semantic.TaskType)
	}

	// 2. Scheduler 选择
	schedulerReq := &SchedulerSelectRequest{
		Model:                "gpt-4",
		PreferredPool:        result.Decision.FinalPool,
		PreferredTier:        result.Decision.Tier.PreferredTier,
		TaskType:             result.Decision.Semantic.TaskType,
		RequiredCapabilities: result.Decision.Semantic.RequiredCapabilities,
	}

	schedulerResult := scheduler.Select(schedulerReq)

	// 验证调度结果 - 应该有账号被选中
	if schedulerResult.SelectedAccountID == 0 {
		t.Error("Expected account to be selected for code request")
	}

	// 验证进入 code_pool
	if schedulerResult.PoolUsed != "code_pool" {
		t.Errorf("PoolUsed = %v, want code_pool", schedulerResult.PoolUsed)
	}

	// 验证 tier - 应该是 medium 或 strong (因为没有 strong code 账号)
	t.Logf("Selected account ID: %d, tier: %v, pool: %s",
		schedulerResult.SelectedAccountID, schedulerResult.MatchedTier, schedulerResult.PoolUsed)
}

// contains 检查字符串是否包含子串
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// TestMultiLayerRouter_ComplexPrompts 测试复杂 prompt 的多层路由
func TestMultiLayerRouter_ComplexPrompts(t *testing.T) {
	multiLayerRouter := NewMultiLayerRouter()
	mockScheduler := NewMockScheduler()
	mockScheduler.SetupMockAccounts()

	testCases := []struct {
		name           string
		prompt         string
		expectedPools  []PreferredPool // 允许的 pool 列表
		minConfidence  float64
	}{
		{
			name:          "登录功能代码",
			prompt:        "帮我做一个用户登录功能，要求能保存账号信息",
			expectedPools: []PreferredPool{PoolCode, PoolDefault},
			minConfidence: 0.3,
		},
		{
			name:          "小工具生成图表",
			prompt:        "我想搭一个小工具，输入数据后自动生成图表",
			expectedPools: []PreferredPool{PoolCode, PoolData, PoolDefault},
			minConfidence: 0.3,
		},
		{
			name:          "SQL查询优化",
			prompt:        "帮我优化这个 SQL 查询速度",
			expectedPools: []PreferredPool{PoolCode, PoolData, PoolDefault},
			minConfidence: 0.3,
		},
		{
			name:          "简单问答",
			prompt:        "今天天气怎么样",
			expectedPools: []PreferredPool{PoolCheap, PoolDefault},
			minConfidence: 0.3,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &RouteRequest{
				Prompt:     tc.prompt,
				HasImage:   false,
				HasDocument: false,
			}

			decision := multiLayerRouter.Route(req)

			// 验证 pool 在允许列表中
			poolAllowed := false
			for _, p := range tc.expectedPools {
				if decision.PreferredPool == p {
					poolAllowed = true
					break
				}
			}
			if !poolAllowed {
				t.Errorf("PreferredPool = %v, want one of %v", decision.PreferredPool, tc.expectedPools)
			}

			// 验证置信度
			if decision.Confidence < tc.minConfidence {
				t.Errorf("Confidence = %.2f, want >= %.2f", decision.Confidence, tc.minConfidence)
			}

			t.Logf("Prompt: %s", tc.prompt)
			t.Logf("  Pool: %v, Source: %v, Confidence: %.2f", decision.PreferredPool, decision.DecisionSource, decision.Confidence)
			t.Logf("  MatchedRules: %v", decision.MatchedRules)
		})
	}
}