package semanticrouter

import (
	"regexp"
	"strings"
)

// ParsedPrompt separates the user task from agent scaffolding. RawPrompt is
// retained for audit/replay, while RoutingPrompt is the only text intended for
// semantic classifiers.
type ParsedPrompt struct {
	RawPrompt             string   `json:"-"`
	UserQuery             string   `json:"user_query"`
	SystemPromptPresent   bool     `json:"system_prompt_present"`
	AgentContextPresent   bool     `json:"agent_context_present"`
	ProjectContextPresent bool     `json:"project_context_present"`
	ToolContextPresent    bool     `json:"tool_context_present"`
	ConversationPresent   bool     `json:"conversation_history_present"`
	AttachmentModalities  []string `json:"attachment_modalities"`
	RoutingPrompt         string   `json:"routing_prompt"`
}

var promptTagPattern = regexp.MustCompile(`(?is)<([a-z_][a-z0-9_-]*)\b[^>]*>(.*?)</[a-z_][a-z0-9_-]*\s*>`)

// ParsePrompt extracts the explicit user_query block when an Agent request
// contains one. For ordinary Playground input the full prompt is the query.
func ParsePrompt(raw string, hasImage, hasDocument, hasCSV bool) ParsedPrompt {
	parsed := ParsedPrompt{RawPrompt: raw}
	blocks := map[string]string{}
	for _, match := range promptTagPattern.FindAllStringSubmatch(raw, -1) {
		blocks[strings.ToLower(match[1])] = strings.TrimSpace(match[2])
	}

	parsed.UserQuery = blocks["user_query"]
	if parsed.UserQuery == "" {
		parsed.UserQuery = strings.TrimSpace(raw)
	}
	parsed.SystemPromptPresent = blocks["system_prompt"] != "" || blocks["system"] != ""
	parsed.AgentContextPresent = blocks["additional_data"] != "" || blocks["cb_summary"] != ""
	parsed.ProjectContextPresent = blocks["project_context"] != "" || blocks["project_layout"] != ""
	parsed.ToolContextPresent = blocks["tool_context"] != "" || blocks["tools"] != ""
	parsed.ConversationPresent = blocks["previous_user_message"] != "" || blocks["previous_assistant_message"] != "" || blocks["previous_tool_call"] != ""

	if hasImage {
		parsed.AttachmentModalities = append(parsed.AttachmentModalities, "image")
	}
	if hasDocument {
		parsed.AttachmentModalities = append(parsed.AttachmentModalities, "document")
	}
	if hasCSV {
		parsed.AttachmentModalities = append(parsed.AttachmentModalities, "csv")
	}

	parsed.RoutingPrompt = parsed.UserQuery
	if len(parsed.AttachmentModalities) > 0 {
		parsed.RoutingPrompt += "\n[attachment_modalities: " + strings.Join(parsed.AttachmentModalities, ", ") + "]"
	}
	return parsed
}
