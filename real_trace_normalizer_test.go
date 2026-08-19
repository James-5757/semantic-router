package semanticrouter

import (
	"encoding/json"
	"testing"
)

func TestNormalizeOpenAIChatRequestUsesLatestUserTurn(t *testing.T) {
	body, err := json.Marshal(map[string]interface{}{
		"model":      "DeepSeek-V4-flash-gz",
		"max_tokens": 32768,
		"stream":     true,
		"tools":      []interface{}{map[string]interface{}{"type": "function"}},
		"messages": []interface{}{
			map[string]string{"role": "system", "content": "You are an AI coding assistant for pair programming."},
			map[string]string{"role": "user", "content": "请打开项目文件并修复 API 错误"},
			map[string]string{"role": "assistant", "content": "已检查"},
			map[string]string{"role": "user", "content": "继续修复这个接口并运行测试"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	request, err := NormalizeOpenAIChatRequest(body)
	if err != nil {
		t.Fatal(err)
	}
	if request.LatestUserPrompt != "继续修复这个接口并运行测试" {
		t.Fatalf("latest user prompt = %q", request.LatestUserPrompt)
	}
	if request.AgentFamily != "coding_agent" || !request.Stream || !request.HasTools {
		t.Fatalf("unexpected agent metadata: %+v", request)
	}
	if request.PromptHash == "" || request.EstimatedInputTokens == 0 {
		t.Fatalf("expected safe hash and context estimate: %+v", request)
	}
}

func TestNormalizeOpenAIChatRequestDoesNotRetainSystemPrompt(t *testing.T) {
	request, err := NormalizeOpenAIChatRequest([]byte(`{"messages":[{"role":"system","content":"secret-internal-rule"},{"role":"user","content":"你好"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if request.LatestUserPrompt != "你好" || request.AgentFamily != "unknown_agent" {
		t.Fatalf("unexpected normalized request: %+v", request)
	}
}

func TestModelSelectorRejectsContextAndToolIncompatibleProfiles(t *testing.T) {
	selector := NewModelCandidateSelector(NewStaticModelRegistry([]*ModelCapabilityProfile{
		{ModelID: "short", Enabled: true, ContextWindowTokens: 4096, Capabilities: map[string]bool{"code": true, "streaming": true}},
		{ModelID: "long", Enabled: true, ContextWindowTokens: 131072, Capabilities: map[string]bool{"code": true, "streaming": true, "tool_call": true}},
	}))
	accounts := []*RealSchedulerAccount{
		{ID: 1, Model: "short", Pool: "code_pool", Tier: TierStrong, Status: "active", Schedulable: true, CodeCapable: true},
		{ID: 2, Model: "long", Pool: "code_pool", Tier: TierStrong, Status: "active", Schedulable: true, CodeCapable: true},
	}
	result, err := selector.Select(nil, accounts, &SchedulerSelectRequest{
		PreferredPool: PoolCode, PreferredTier: TierStrong, ContextTokens: 80000, MaxOutputTokens: 32768, RequiresStreaming: true, RequiresToolCall: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Account.ID != 2 {
		t.Fatalf("selected account = %d, want long-context tool-capable account", result.Account.ID)
	}
}

func TestDryRunBridgeAppliesCapturedRequestConstraints(t *testing.T) {
	scheduler := NewRealSchedulerDryRunWithRegistry([]*RealSchedulerAccount{
		{ID: 11, Model: "long", Pool: "code_pool", Tier: TierStrong, Status: "active", Schedulable: true, CodeCapable: true},
	}, NewStaticModelRegistry([]*ModelCapabilityProfile{
		{ModelID: "long", Enabled: true, ContextWindowTokens: 131072, Capabilities: map[string]bool{"code": true, "streaming": true, "tool_call": true}},
	}))
	result, normalized, err := scheduler.SelectOpenAIChatRequest([]byte(`{"model":"long","stream":true,"max_tokens":1024,"tools":[{"type":"function"}],"messages":[{"role":"user","content":"修复这个 API"}]}`), &SchedulerSelectRequest{PreferredPool: PoolCode, PreferredTier: TierStrong})
	if err != nil {
		t.Fatal(err)
	}
	if result.Error != nil || result.SelectedAccountID != 11 {
		t.Fatalf("unexpected dry-run result: %+v", result)
	}
	if normalized.Model != "long" || !normalized.Stream || !normalized.HasTools {
		t.Fatalf("unexpected normalized request: %+v", normalized)
	}
}
