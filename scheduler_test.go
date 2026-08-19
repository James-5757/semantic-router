package semanticrouter

import (
	"testing"
)

// TestScheduler_TextRequest 普通文本请求调度测试
// 目标：普通文本请求应进入 cheap_chat_pool
func TestScheduler_TextRequest(t *testing.T) {
	scheduler := NewMockScheduler()
	scheduler.SetupMockAccounts()

	semanticRouter := NewRuleBasedSemanticRouter()
	tierRouter := NewRuleBasedTierRouter()
	logger := NewInMemoryRoutingDecisionLogger(100)
	preRouter := NewPreRouter(semanticRouter, tierRouter, logger)

	tests := []struct {
		name       string
		model      string
		prompt     string
		wantPool   string // cheap_chat_pool
		wantTier   PreferredTier
		wantVision bool
	}{
		{
			name:       "普通文本请求应进入 cheap_chat_pool",
			model:      "gpt-3.5-turbo",
			prompt:     "你好，请介绍一下北京的历史",
			wantPool:   "cheap_chat_pool",
			wantTier:   TierWeak,
			wantVision: false,
		},
		{
			name:       "简单问答进入 cheap_chat_pool",
			model:      "gpt-3.5-turbo",
			prompt:     "今天天气怎么样？",
			wantPool:   "cheap_chat_pool",
			wantTier:   TierWeak,
			wantVision: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			routeReq := &RouteRequest{
				Model:       tt.model,
				Prompt:      tt.prompt,
				HasImage:    false,
				HasDocument: false,
			}

			// 预路由
			result, err := preRouter.Route(nil, tt.model, "", "", routeReq)
			if err != nil {
				t.Fatalf("PreRoute error = %v", err)
			}

			// Scheduler 选择
			schedulerReq := &SchedulerSelectRequest{
				Model:                tt.model,
				PreferredPool:        result.Decision.FinalPool,
				PreferredTier:        result.Decision.Tier.PreferredTier,
				TaskType:             result.Decision.Semantic.TaskType,
				RequiredCapabilities: result.Decision.Semantic.RequiredCapabilities,
			}

			schedulerResult := scheduler.Select(schedulerReq)
			if schedulerResult.Error != nil {
				t.Fatalf("Scheduler Select error = %v", schedulerResult.Error)
			}

			// 验证进入 cheap_chat_pool
			if schedulerResult.PoolUsed != tt.wantPool {
				t.Errorf("PoolUsed = %v, want %v", schedulerResult.PoolUsed, tt.wantPool)
			}

			// 验证选择的账号具有正确的能力
			account, ok := scheduler.GetAccountByID(schedulerResult.SelectedAccountID)
			if !ok {
				t.Fatal("Selected account not found")
			}
			if account.VisionCapable != tt.wantVision {
				t.Errorf("VisionCapable = %v, want %v", account.VisionCapable, tt.wantVision)
			}
		})
	}
}

// TestScheduler_CodeRequest 代码请求调度测试
// 目标：代码请求应进入 code_pool
func TestScheduler_CodeRequest(t *testing.T) {
	scheduler := NewMockScheduler()
	scheduler.SetupMockAccounts()

	semanticRouter := NewRuleBasedSemanticRouter()
	tierRouter := NewRuleBasedTierRouter()
	logger := NewInMemoryRoutingDecisionLogger(100)
	preRouter := NewPreRouter(semanticRouter, tierRouter, logger)

	routeReq := &RouteRequest{
		Model:    "gpt-4",
		Prompt:   "请用 Python 写一个快速排序函数:\n```python\ndef quick_sort(arr):\n```",
		HasImage: false,
	}

	result, err := preRouter.Route(nil, "gpt-4", "", "", routeReq)
	if err != nil {
		t.Fatalf("PreRoute error = %v", err)
	}

	schedulerReq := &SchedulerSelectRequest{
		Model:                "gpt-4",
		PreferredPool:        result.Decision.FinalPool,
		PreferredTier:        result.Decision.Tier.PreferredTier,
		TaskType:             result.Decision.Semantic.TaskType,
		RequiredCapabilities: result.Decision.Semantic.RequiredCapabilities,
	}

	schedulerResult := scheduler.Select(schedulerReq)
	if schedulerResult.Error != nil {
		t.Fatalf("Scheduler Select error = %v", schedulerResult.Error)
	}

	// 代码请求应进入 code_pool
	if schedulerResult.PoolUsed != "code_pool" {
		t.Errorf("PoolUsed = %v, want code_pool", schedulerResult.PoolUsed)
	}

	// 验证账号支持代码
	account, _ := scheduler.GetAccountByID(schedulerResult.SelectedAccountID)
	if account.Pool != "code_pool" {
		t.Errorf("Account pool = %v, want code_pool", account.Pool)
	}
}

// TestScheduler_VisionRequest 图片请求调度测试
// 目标：图片请求只能选择 vision-capable account
func TestScheduler_VisionRequest(t *testing.T) {
	scheduler := NewMockScheduler()
	scheduler.SetupMockAccounts()

	semanticRouter := NewRuleBasedSemanticRouter()
	tierRouter := NewRuleBasedTierRouter()
	logger := NewInMemoryRoutingDecisionLogger(100)
	preRouter := NewPreRouter(semanticRouter, tierRouter, logger)

	routeReq := &RouteRequest{
		Model:    "gpt-4o",
		Prompt:   "请描述这张图片的内容",
		HasImage: true,
	}

	result, err := preRouter.Route(nil, "gpt-4o", "", "", routeReq)
	if err != nil {
		t.Fatalf("PreRoute error = %v", err)
	}

	schedulerReq := &SchedulerSelectRequest{
		Model:                "gpt-4o",
		PreferredPool:        result.Decision.FinalPool,
		PreferredTier:        result.Decision.Tier.PreferredTier,
		TaskType:             result.Decision.Semantic.TaskType,
		RequiredCapabilities: result.Decision.Semantic.RequiredCapabilities,
	}

	schedulerResult := scheduler.Select(schedulerReq)
	if schedulerResult.Error != nil {
		t.Fatalf("Scheduler Select error = %v", schedulerResult.Error)
	}

	// 图片请求应进入 vision_pool
	if schedulerResult.PoolUsed != "vision_pool" {
		t.Errorf("PoolUsed = %v, want vision_pool", schedulerResult.PoolUsed)
	}

	// 验证账号有视觉能力
	account, ok := scheduler.GetAccountByID(schedulerResult.SelectedAccountID)
	if !ok {
		t.Fatal("Selected account not found")
	}
	if !account.VisionCapable {
		t.Error("Selected account should have VisionCapable")
	}
}

// TestScheduler_DocumentRequest 文档请求调度测试
// 目标：文档请求只能选择 document-capable account
func TestScheduler_DocumentRequest(t *testing.T) {
	scheduler := NewMockScheduler()
	scheduler.SetupMockAccounts()

	semanticRouter := NewRuleBasedSemanticRouter()
	tierRouter := NewRuleBasedTierRouter()
	logger := NewInMemoryRoutingDecisionLogger(100)
	preRouter := NewPreRouter(semanticRouter, tierRouter, logger)

	routeReq := &RouteRequest{
		Model:        "gpt-4",
		Prompt:       "请总结这个文档的内容",
		HasDocument:  true,
		DocumentType: "docx",
	}

	result, err := preRouter.Route(nil, "gpt-4", "", "", routeReq)
	if err != nil {
		t.Fatalf("PreRoute error = %v", err)
	}

	schedulerReq := &SchedulerSelectRequest{
		Model:                "gpt-4",
		PreferredPool:        result.Decision.FinalPool,
		PreferredTier:        result.Decision.Tier.PreferredTier,
		TaskType:             result.Decision.Semantic.TaskType,
		RequiredCapabilities: result.Decision.Semantic.RequiredCapabilities,
	}

	schedulerResult := scheduler.Select(schedulerReq)
	if schedulerResult.Error != nil {
		t.Fatalf("Scheduler Select error = %v", schedulerResult.Error)
	}

	// 文档请求应进入 document_pool
	if schedulerResult.PoolUsed != "document_pool" {
		t.Errorf("PoolUsed = %v, want document_pool", schedulerResult.PoolUsed)
	}

	// 验证账号有文档能力
	account, ok := scheduler.GetAccountByID(schedulerResult.SelectedAccountID)
	if !ok {
		t.Fatal("Selected account not found")
	}
	if !account.DocumentCapable {
		t.Error("Selected account should have DocumentCapable")
	}
}

// TestScheduler_PreviousResponseID_Hit previous_response_id 命中测试
// 目标：previous_response_id 命中且能力兼容时优先使用粘性账号
func TestScheduler_PreviousResponseID_Hit(t *testing.T) {
	scheduler := NewMockScheduler()
	scheduler.SetupMockAccounts()

	// 预先绑定 previous_response_id 到账号 1
	scheduler.BindPreviousResponse("resp-123", 1)

	schedulerReq := &SchedulerSelectRequest{
		Model:                "gpt-3.5-turbo",
		PreferredPool:        PoolDefault,
		PreferredTier:        TierWeak,
		TaskType:             TaskTypeText,
		RequiredCapabilities: RequiredCapabilities{},
		PreviousResponseID:   "resp-123",
	}

	schedulerResult := scheduler.Select(schedulerReq)
	if schedulerResult.Error != nil {
		t.Fatalf("Scheduler Select error = %v", schedulerResult.Error)
	}

	// 应该命中 previous_response_id
	if schedulerResult.Layer != "previous_response_id" {
		t.Errorf("Layer = %v, want previous_response_id", schedulerResult.Layer)
	}

	// 应该选中账号 1
	if schedulerResult.SelectedAccountID != 1 {
		t.Errorf("SelectedAccountID = %v, want 1", schedulerResult.SelectedAccountID)
	}
}

// TestScheduler_PreviousResponseID_Incompatible previous_response_id 命中但不兼容
// 目标：previous_response_id 命中但能力不兼容时跳过粘性账号
func TestScheduler_PreviousResponseID_Incompatible(t *testing.T) {
	scheduler := NewMockScheduler()
	scheduler.SetupMockAccounts()

	// 预先绑定 previous_response_id 到文本账号(无视觉能力)
	scheduler.BindPreviousResponse("resp-123", 1)

	// 但请求需要视觉能力
	schedulerReq := &SchedulerSelectRequest{
		Model:                "gpt-4o",
		PreferredPool:        PoolVision,
		PreferredTier:        TierStrong,
		TaskType:             TaskTypeVision,
		RequiredCapabilities: RequiredCapabilities{VisionCapable: true},
		PreviousResponseID:   "resp-123",
	}

	schedulerResult := scheduler.Select(schedulerReq)
	if schedulerResult.Error != nil {
		t.Fatalf("Scheduler Select error = %v", schedulerResult.Error)
	}

	// 应该跳过 previous_response_id，走 load_balance
	if schedulerResult.Layer != "load_balance" {
		t.Errorf("Layer = %v, want load_balance (should skip incompatible sticky)", schedulerResult.Layer)
	}

	// 应该选中有视觉能力的账号
	account, ok := scheduler.GetAccountByID(schedulerResult.SelectedAccountID)
	if !ok {
		t.Fatal("Selected account not found")
	}
	if !account.VisionCapable {
		t.Error("Selected account should have VisionCapable")
	}
}

// TestScheduler_SessionHash_Hit session_hash 命中测试
// 目标：session_hash 命中且健康时优先使用绑定账号
func TestScheduler_SessionHash_Hit(t *testing.T) {
	scheduler := NewMockScheduler()
	scheduler.SetupMockAccounts()

	// 预先绑定 session_hash 到账号 1
	scheduler.BindStickySession("session-abc", 1)

	schedulerReq := &SchedulerSelectRequest{
		Model:                "gpt-3.5-turbo",
		PreferredPool:        PoolDefault,
		PreferredTier:        TierWeak,
		TaskType:             TaskTypeText,
		RequiredCapabilities: RequiredCapabilities{},
		SessionHash:          "session-abc",
	}

	schedulerResult := scheduler.Select(schedulerReq)
	if schedulerResult.Error != nil {
		t.Fatalf("Scheduler Select error = %v", schedulerResult.Error)
	}

	// 应该命中 session_sticky
	if schedulerResult.Layer != "session_sticky" {
		t.Errorf("Layer = %v, want session_sticky", schedulerResult.Layer)
	}

	// 应该选中账号 1
	if schedulerResult.SelectedAccountID != 1 {
		t.Errorf("SelectedAccountID = %v, want 1", schedulerResult.SelectedAccountID)
	}
}

// TestScheduler_SessionHash_Unhealthy session_hash 命中但不健康
// 目标：session_hash 命中但不健康时跳过绑定账号
func TestScheduler_SessionHash_Unhealthy(t *testing.T) {
	scheduler := NewMockScheduler()
	scheduler.SetupMockAccounts()

	// 绑定 session 到账号 1，然后设置账号不健康
	scheduler.BindStickySession("session-abc", 1)
	scheduler.SetAccountUnhealthy(1)

	schedulerReq := &SchedulerSelectRequest{
		Model:                "gpt-3.5-turbo",
		PreferredPool:        PoolDefault,
		PreferredTier:        TierWeak,
		TaskType:             TaskTypeText,
		RequiredCapabilities: RequiredCapabilities{},
		SessionHash:          "session-abc",
	}

	schedulerResult := scheduler.Select(schedulerReq)
	if schedulerResult.Error != nil {
		t.Fatalf("Scheduler Select error = %v", schedulerResult.Error)
	}

	// 应该跳过不健康的账号，走 load_balance
	if schedulerResult.Layer != "load_balance" {
		t.Errorf("Layer = %v, want load_balance (should skip unhealthy sticky)", schedulerResult.Layer)
	}

	// 应该选中其他健康的账号
	if schedulerResult.SelectedAccountID == 1 {
		t.Error("Should not select unhealthy account")
	}
}

// TestScheduler_RoutingDecisionLog 调度日志测试
// 目标：Scheduler 最终必须记录 routing_decision_log
func TestScheduler_RoutingDecisionLog(t *testing.T) {
	scheduler := NewMockScheduler()
	scheduler.SetupMockAccounts()

	semanticRouter := NewRuleBasedSemanticRouter()
	tierRouter := NewRuleBasedTierRouter()
	logger := NewInMemoryRoutingDecisionLogger(100)
	preRouter := NewPreRouter(semanticRouter, tierRouter, logger)

	routeReq := &RouteRequest{
		Model:       "gpt-4",
		Prompt:      "你好，请介绍一下北京的历史",
		HasImage:    false,
		HasDocument: false,
	}

	result, err := preRouter.Route(nil, "gpt-4", "session-123", "prev-123", routeReq)
	if err != nil {
		t.Fatalf("PreRoute error = %v", err)
	}

	schedulerReq := &SchedulerSelectRequest{
		Model:                "gpt-4",
		PreferredPool:        result.Decision.FinalPool,
		PreferredTier:        result.Decision.Tier.PreferredTier,
		TaskType:             result.Decision.Semantic.TaskType,
		RequiredCapabilities: result.Decision.Semantic.RequiredCapabilities,
		PreviousResponseID:   "prev-123",
		SessionHash:          "session-123",
	}

	_ = scheduler.Select(schedulerReq)

	// 验证日志记录
	if logger.Size() == 0 {
		t.Error("Expected routing_decision_log to be recorded")
	}

	// 验证日志内容
	entries, err := logger.GetRecentDecisions(10)
	if err != nil {
		t.Fatalf("GetRecentDecisions error = %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("No log entries found")
	}

	// 验证记录的任务类型
	lastEntry := entries[len(entries)-1]
	if lastEntry.TaskType != TaskTypeText {
		t.Errorf("Logged TaskType = %v, want text", lastEntry.TaskType)
	}
}

// TestScheduler_DisabledAccount 禁用账号测试
// 目标：不应选择禁用的账号
func TestScheduler_DisabledAccount(t *testing.T) {
	scheduler := NewMockScheduler()
	scheduler.SetupMockAccounts()

	// 账号 99 已被禁用
	account, ok := scheduler.GetAccountByID(99)
	if !ok {
		t.Fatal("Test setup: account 99 not found")
	}
	if account.Status != "disabled" {
		t.Error("Test setup: account 99 should be disabled")
	}

	// 请求需要视觉能力，只有 99 账号有（但被禁用）
	// 应该选择其他 vision-capable 账号
	schedulerReq := &SchedulerSelectRequest{
		Model:                "gpt-4o",
		PreferredPool:        PoolVision,
		PreferredTier:        TierStrong,
		TaskType:             TaskTypeVision,
		RequiredCapabilities: RequiredCapabilities{VisionCapable: true},
	}

	schedulerResult := scheduler.Select(schedulerReq)
	if schedulerResult.Error != nil {
		t.Fatalf("Scheduler Select error = %v", schedulerResult.Error)
	}

	// 不应该选择禁用的账号
	if schedulerResult.SelectedAccountID == 99 {
		t.Error("Should not select disabled account")
	}

	// 应该选择账号 20 或 21（vision strong accounts）
	if schedulerResult.SelectedAccountID != 20 && schedulerResult.SelectedAccountID != 21 {
		t.Errorf("SelectedAccountID = %v, want 20 or 21", schedulerResult.SelectedAccountID)
	}
}

// TestScheduler_Integration 完整调度集成测试
func TestScheduler_Integration(t *testing.T) {
	scheduler := NewMockScheduler()
	scheduler.SetupMockAccounts()

	semanticRouter := NewRuleBasedSemanticRouter()
	tierRouter := NewRuleBasedTierRouter()
	logger := NewInMemoryRoutingDecisionLogger(100)
	preRouter := NewPreRouter(semanticRouter, tierRouter, logger)

	testCases := []struct {
		name            string
		model           string
		prompt          string
		hasImage        bool
		hasDocument     bool
		documentType    string
		expectedPool    string
		expectedTier    PreferredTier
		expectedLayer   string // empty means any
	}{
		{
			name:          "普通文本",
			model:         "gpt-3.5-turbo",
			prompt:        "你好",
			expectedPool:  "cheap_chat_pool",
			expectedTier:  TierMedium, // gpt-3.5-turbo 是中等强度模型
		},
		{
			name:          "代码生成",
			model:         "gpt-4",
			prompt:        "写一个排序算法:\n```python\ndef sort():\n```",
			expectedPool:  "code_pool",
			expectedTier:  TierMedium,
		},
		{
			name:          "图片理解",
			model:         "gpt-4o",
			prompt:        "描述图片",
			hasImage:      true,
			expectedPool:  "vision_pool",
			expectedTier:  TierStrong,
		},
		{
			name:          "Word文档",
			model:         "gpt-4",
			prompt:        "总结文档",
			hasDocument:   true,
			documentType:  "docx",
			expectedPool:  "document_pool",
			expectedTier:  TierMedium,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			routeReq := &RouteRequest{
				Model:        tc.model,
				Prompt:       tc.prompt,
				HasImage:     tc.hasImage,
				HasDocument:  tc.hasDocument,
				DocumentType: tc.documentType,
			}

			result, err := preRouter.Route(nil, tc.model, "", "", routeReq)
			if err != nil {
				t.Fatalf("PreRoute error = %v", err)
			}

			schedulerReq := &SchedulerSelectRequest{
				Model:                tc.model,
				PreferredPool:        result.Decision.FinalPool,
				PreferredTier:        result.Decision.Tier.PreferredTier,
				TaskType:             result.Decision.Semantic.TaskType,
				RequiredCapabilities: result.Decision.Semantic.RequiredCapabilities,
			}

			schedulerResult := scheduler.Select(schedulerReq)
			if schedulerResult.Error != nil {
				t.Fatalf("Scheduler Select error = %v", schedulerResult.Error)
			}

			// 验证池
			if schedulerResult.PoolUsed != tc.expectedPool {
				t.Errorf("Pool = %v, want %v", schedulerResult.PoolUsed, tc.expectedPool)
			}

			// 验证 tier
			account, _ := scheduler.GetAccountByID(schedulerResult.SelectedAccountID)
			if account.Tier != tc.expectedTier {
				t.Errorf("Account tier = %v, want %v", account.Tier, tc.expectedTier)
			}

			// 验证日志
			if logger.Size() == 0 {
				t.Error("Expected routing_decision_log")
			}
		})
	}
}