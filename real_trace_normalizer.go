package semanticrouter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// NormalizedLLMRequest is the safe routing view of a captured request. It
// intentionally does not retain system prompts or the full message history.
type NormalizedLLMRequest struct {
	Protocol             string
	Model                string
	LatestUserPrompt     string
	PromptHash           string
	AgentFamily          string
	Stream               bool
	HasTools             bool
	HasImage             bool
	HasDocument          bool
	HasCSV               bool
	EstimatedInputTokens int
	MaxOutputTokens      int
	Temperature          float64
	RequiredCapabilities []string
}

type capturedChatRequest struct {
	Model       string            `json:"model"`
	MaxTokens   int               `json:"max_tokens"`
	Temperature float64           `json:"temperature"`
	Stream      bool              `json:"stream"`
	Messages    []capturedMessage `json:"messages"`
	Tools       []json.RawMessage `json:"tools"`
}

type capturedMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// NormalizeOpenAIChatRequest extracts fields useful for routing. The latest
// user turn drives task classification; historical context only contributes
// to the context-budget estimate.
func NormalizeOpenAIChatRequest(body []byte) (*NormalizedLLMRequest, error) {
	var request capturedChatRequest
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, fmt.Errorf("invalid chat request: %w", err)
	}
	result := &NormalizedLLMRequest{
		Protocol:        "openai_chat_completions",
		Model:           request.Model,
		Stream:          request.Stream,
		HasTools:        len(request.Tools) > 0,
		MaxOutputTokens: request.MaxTokens,
		Temperature:     request.Temperature,
	}
	var systemPrompt string
	totalChars := 0
	for _, message := range request.Messages {
		text := messageText(message.Content)
		totalChars += len([]rune(text))
		switch strings.ToLower(message.Role) {
		case "system":
			if systemPrompt == "" {
				systemPrompt = text
			}
		case "user":
			if strings.TrimSpace(text) != "" {
				result.LatestUserPrompt = text
			}
		}
	}
	result.PromptHash = hashCapturedPrompt(result.LatestUserPrompt)
	result.EstimatedInputTokens = estimateTokens(totalChars)
	result.AgentFamily = detectAgentFamily(systemPrompt, request.Messages)
	result.HasImage, result.HasDocument, result.HasCSV = detectModalities(result.LatestUserPrompt)
	result.RequiredCapabilities = inferTraceCapabilities(result)
	return result, nil
}

// ApplyToSchedulerRequest transfers hard request constraints into model
// selection without changing the existing pool or tier decision.
func (r *NormalizedLLMRequest) ApplyToSchedulerRequest(request *SchedulerSelectRequest) {
	if r == nil || request == nil {
		return
	}
	request.Model = r.Model
	request.ContextTokens = r.EstimatedInputTokens
	request.MaxOutputTokens = r.MaxOutputTokens
	request.RequiresStreaming = r.Stream
	request.RequiresToolCall = r.HasTools
	if r.HasImage {
		request.RequiredCapabilities.VisionCapable = true
	}
	if r.HasDocument {
		request.RequiredCapabilities.DocumentCapable = true
	}
}

// SelectOpenAIChatRequest is the dry-run bridge for captured or live
// OpenAI-compatible payloads. It enriches an existing scheduler request with
// hard protocol constraints and leaves pool/tier ownership unchanged.
func (s *RealSchedulerDryRun) SelectOpenAIChatRequest(body []byte, request *SchedulerSelectRequest) (*SchedulerSelectResult, *NormalizedLLMRequest, error) {
	normalized, err := NormalizeOpenAIChatRequest(body)
	if err != nil {
		return nil, nil, err
	}
	normalized.ApplyToSchedulerRequest(request)
	return s.Select(request), normalized, nil
}

func messageText(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		var builder strings.Builder
		for _, part := range parts {
			if part.Type == "text" {
				builder.WriteString(part.Text)
			}
		}
		return builder.String()
	}
	return ""
}

func detectAgentFamily(systemPrompt string, messages []capturedMessage) string {
	joined := strings.ToLower(systemPrompt)
	if strings.Contains(joined, "pair programming") || strings.Contains(joined, "ai coding assistant") || strings.Contains(joined, ".codebuddy") {
		return "coding_agent"
	}
	for _, message := range messages {
		if strings.EqualFold(message.Role, "tool") || strings.Contains(strings.ToLower(messageText(message.Content)), "workspace") {
			return "tool_agent"
		}
	}
	return "unknown_agent"
}

func detectModalities(prompt string) (bool, bool, bool) {
	lower := strings.ToLower(prompt)
	return strings.Contains(lower, "image") || strings.Contains(prompt, "图片") || strings.Contains(prompt, "截图"),
		strings.Contains(lower, "pdf") || strings.Contains(lower, "document") || strings.Contains(prompt, "文档") || strings.Contains(prompt, "合同"),
		strings.Contains(lower, "csv") || strings.Contains(lower, "excel") || strings.Contains(prompt, "表格") || strings.Contains(prompt, "数据")
}

func inferTraceCapabilities(request *NormalizedLLMRequest) []string {
	capabilities := make([]string, 0, 5)
	if request.AgentFamily == "coding_agent" || request.AgentFamily == "tool_agent" {
		capabilities = append(capabilities, "code", "streaming")
	}
	if request.HasTools {
		capabilities = append(capabilities, "tool_call")
	}
	if request.HasImage {
		capabilities = append(capabilities, "vision")
	}
	if request.HasDocument {
		capabilities = append(capabilities, "document")
	}
	if request.HasCSV {
		capabilities = append(capabilities, "data")
	}
	return uniqueStrings(capabilities)
}

func estimateTokens(chars int) int {
	if chars <= 0 {
		return 0
	}
	return (chars + 3) / 4
}

func hashCapturedPrompt(prompt string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(prompt)))
	return hex.EncodeToString(digest[:])
}
