package semanticrouter

import "testing"

func TestParsePromptUsesUserQueryAndDropsAgentScaffolding(t *testing.T) {
	raw := `<additional_data>tool and workspace details</additional_data>
<project_context>large project context</project_context>
<user_query>帮我修复 CSV 导入异常</user_query>
<previous_tool_call>tool output</previous_tool_call>`
	parsed := ParsePrompt(raw, false, false, true)
	if parsed.UserQuery != "帮我修复 CSV 导入异常" {
		t.Fatalf("user_query = %q", parsed.UserQuery)
	}
	if parsed.RoutingPrompt != "帮我修复 CSV 导入异常\n[attachment_modalities: csv]" {
		t.Fatalf("routing_prompt = %q", parsed.RoutingPrompt)
	}
	if !parsed.AgentContextPresent || !parsed.ProjectContextPresent || !parsed.ConversationPresent {
		t.Fatalf("agent context flags were not detected: %+v", parsed)
	}
}

func TestParsePromptPlainPlaygroundInput(t *testing.T) {
	parsed := ParsePrompt("实现一个登录 API", false, false, false)
	if parsed.UserQuery != "实现一个登录 API" || parsed.RoutingPrompt != parsed.UserQuery {
		t.Fatalf("plain prompt was changed: %+v", parsed)
	}
}
