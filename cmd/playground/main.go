package main

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	semanticrouter "semantic-router"
	vllm_pool_client "semantic-router/vllm_pool_client"
)

// ============================================================================
// Routing Event Logging
// ============================================================================

// RoutingEvent records a single routing evaluation for offline analysis
type RoutingEvent struct {
	RequestID     string `json:"request_id"`
	CreatedAt     string `json:"created_at"`
	PromptHash    string `json:"prompt_hash"`
	PromptPreview string `json:"prompt_preview"` // first N chars when saved

	// Input Metadata
	HasImage    bool   `json:"has_image"`
	HasDocument bool   `json:"has_document"`
	HasCSV      bool   `json:"has_csv"`
	Mode        string `json:"mode"`

	// Intent
	PrimaryIntent    string   `json:"primary_intent"`
	SecondaryIntents []string `json:"secondary_intents,omitempty"`

	// Local Decision
	LocalProvider   string  `json:"local_provider"`
	LocalDecision   string  `json:"local_decision"`
	LocalCategory   string  `json:"local_category"`
	LocalPool       string  `json:"local_pool"`
	LocalConfidence float64 `json:"local_confidence"`
	LocalLatencyMs  float64 `json:"local_latency_ms"`

	// Official vLLM Decision
	OfficialProvider   string  `json:"official_provider"`
	OfficialDecision   string  `json:"official_decision"`
	OfficialCategory   string  `json:"official_category"`
	OfficialPool       string  `json:"official_pool"`
	OfficialConfidence float64 `json:"official_confidence"`
	OfficialLatencyMs  float64 `json:"official_latency_ms"`

	// Pool
	RawPool               string `json:"raw_pool"`
	NormalizedPool        string `json:"normalized_pool"`
	PhysicalModelGroup    string `json:"physical_model_group"`
	OfficialPhysicalGroup string `json:"official_physical_group"`

	// Tier
	ComplexityScore float64 `json:"complexity_score"`
	MinimumTier     string  `json:"minimum_tier"`
	RequestedTier   string  `json:"requested_tier"`
	SelectedTier    string  `json:"selected_tier"`

	// Agreement
	SemanticAgreement      bool    `json:"semantic_agreement"`
	PoolAgreement          bool    `json:"pool_agreement"`
	PhysicalGroupAgreement bool    `json:"physical_group_agreement"`
	RouteLLMAgreement      bool    `json:"routellm_agreement"`
	RouteLLMProbability    float64 `json:"routellm_probability,omitempty"`

	// Final
	FinalPool       string `json:"final_pool"`
	FinalPoolSource string `json:"final_pool_source"`
	SchedulerSource string `json:"scheduler_source"`

	// Runtime
	TotalLatencyMs   float64 `json:"total_latency_ms"`
	EmbeddingInvoked bool    `json:"embedding_invoked"`
	DryRun           bool    `json:"dry_run"`
	UpstreamCalled   bool    `json:"upstream_called"`
	Takeover         string  `json:"takeover"`

	// Review (optional, for human annotation)
	Reviewed       bool   `json:"reviewed,omitempty"`
	RouteCorrect   *bool  `json:"route_correct,omitempty"`
	ExpectedIntent string `json:"expected_intent,omitempty"`
	ExpectedPool   string `json:"expected_pool,omitempty"`
	ExpectedTier   string `json:"expected_tier,omitempty"`
	ReviewNote     string `json:"review_note,omitempty"`

	// Error
	Errors []string `json:"errors,omitempty"`
}

// RoutingEventLogger manages append-only JSONL logging
type RoutingEventLogger struct {
	dir             string
	savePrompt      atomic.Bool
	writeErrors     atomic.Int64
	writtenEvents   atomic.Int64
	mu              sync.Mutex
	currentFilePath string
	lastMonth       string
}

func NewRoutingEventLogger(dir string) *RoutingEventLogger {
	return &RoutingEventLogger{dir: dir, lastMonth: "never"}
}

// SetSavePrompt controls whether full prompt text is saved
func (l *RoutingEventLogger) SetSavePrompt(save bool) {
	l.savePrompt.Store(save)
}

// SavePrompt returns whether full prompt saving is enabled
func (l *RoutingEventLogger) SavePrompt() bool {
	return l.savePrompt.Load()
}

// WriteErrors returns count of write failures
func (l *RoutingEventLogger) WriteErrors() int64 {
	return l.writeErrors.Load()
}

// WrittenEvents returns count of successfully written events
func (l *RoutingEventLogger) WrittenEvents() int64 {
	return l.writtenEvents.Load()
}

// Append writes a routing event to the current JSONL file.
// The write is best-effort; errors do not propagate to the caller.
func (l *RoutingEventLogger) Append(event *RoutingEvent) {
	// Compute file path based on current month
	now := time.Now()
	ym := now.Format("2006-01")
	if ym != l.lastMonth {
		l.mu.Lock()
		if ym != l.lastMonth { // double-check
			dir := l.dir
			if dir == "" {
				dir = "playground_logs"
			}
			if err := os.MkdirAll(dir, 0755); err == nil {
				l.currentFilePath = filepath.Join(dir, "routing_events_"+ym+".jsonl")
			}
			l.lastMonth = ym
		}
		l.mu.Unlock()
	}

	fp := l.currentFilePath
	if fp == "" {
		l.writeErrors.Add(1)
		return
	}

	line, err := json.Marshal(event)
	if err != nil {
		l.writeErrors.Add(1)
		return
	}

	// Best-effort append; errors increment writeErrors but do not panic
	l.mu.Lock()
	defer l.mu.Unlock()

	f, err := os.OpenFile(fp, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		l.writeErrors.Add(1)
		return
	}
	defer f.Close()

	if _, err := f.Write(append(line, '\n')); err != nil {
		l.writeErrors.Add(1)
		return
	}
	l.writtenEvents.Add(1)
}

// ReadEvents reads all events from the current month's log file
func (l *RoutingEventLogger) ReadEvents(limit int) []*RoutingEvent {
	fp := l.currentFilePath
	if fp == "" {
		return nil
	}
	data, err := os.ReadFile(fp)
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if limit <= 0 || limit > len(lines) {
		limit = len(lines)
	}
	result := make([]*RoutingEvent, 0, limit)
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var ev RoutingEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		result = append(result, &ev)
		if len(result) >= limit {
			break
		}
	}
	return result
}

// ExportJSONL returns all events as a JSONL string
func (l *RoutingEventLogger) ExportJSONL() string {
	fp := l.currentFilePath
	if fp == "" {
		return ""
	}
	data, err := os.ReadFile(fp)
	if err != nil {
		return ""
	}
	return string(data)
}

// ExportCSV returns all events as a CSV string
func (l *RoutingEventLogger) ExportCSV() string {
	events := l.ReadEvents(100000)
	if len(events) == 0 {
		return ""
	}
	var buf strings.Builder
	buf.WriteString("request_id,created_at,prompt_hash,prompt_preview,has_image,has_document,has_csv,mode,")
	buf.WriteString("primary_intent,local_provider,local_decision,local_category,local_pool,local_confidence,official_provider,official_decision,official_pool,official_confidence,")
	buf.WriteString("raw_pool,normalized_pool,physical_model_group,complexity_score,minimum_tier,requested_tier,selected_tier,")
	buf.WriteString("semantic_agreement,pool_agreement,final_pool,final_pool_source,total_latency_ms,dry_run,upstream_called,takeover,")
	buf.WriteString("reviewed,expected_intent,expected_pool,expected_tier,review_note\n")

	for _, ev := range events {
		fmt.Fprintf(&buf, "%s,%s,%s,%s,%v,%v,%v,%s,",
			escapeCSV(ev.RequestID), escapeCSV(ev.CreatedAt), escapeCSV(ev.PromptHash), escapeCSV(ev.PromptPreview),
			ev.HasImage, ev.HasDocument, ev.HasCSV, escapeCSV(ev.Mode))
		fmt.Fprintf(&buf, "%s,%s,%s,%s,%s,%.4f,%s,%s,%s,%.4f,",
			escapeCSV(ev.PrimaryIntent), escapeCSV(ev.LocalProvider), escapeCSV(ev.LocalDecision), escapeCSV(ev.LocalCategory), escapeCSV(ev.LocalPool), ev.LocalConfidence,
			escapeCSV(ev.OfficialProvider), escapeCSV(ev.OfficialDecision), escapeCSV(ev.OfficialPool), ev.OfficialConfidence)
		fmt.Fprintf(&buf, "%s,%s,%s,%.4f,%s,%s,%s,",
			escapeCSV(ev.RawPool), escapeCSV(ev.NormalizedPool), escapeCSV(ev.PhysicalModelGroup), ev.ComplexityScore,
			escapeCSV(ev.MinimumTier), escapeCSV(ev.RequestedTier), escapeCSV(ev.SelectedTier))
		fmt.Fprintf(&buf, "%v,%v,%s,%s,%.1f,%v,%v,%s,",
			ev.SemanticAgreement, ev.PoolAgreement, escapeCSV(ev.FinalPool), escapeCSV(ev.FinalPoolSource), ev.TotalLatencyMs,
			ev.DryRun, ev.UpstreamCalled, escapeCSV(ev.Takeover))
		fmt.Fprintf(&buf, "%v,%s,%s,%s,%s\n",
			ev.Reviewed, escapeCSV(ev.ExpectedIntent), escapeCSV(ev.ExpectedPool), escapeCSV(ev.ExpectedTier), escapeCSV(ev.ReviewNote))
	}
	return buf.String()
}

func escapeCSV(s string) string {
	if s == "" {
		return ""
	}
	if strings.ContainsAny(s, "\",\n\r") {
		s = strings.ReplaceAll(s, "\"", "\"\"")
		return "\"" + s + "\""
	}
	return s
}

// hashPrompt creates a SHA-256 hash of the prompt
func hashPrompt(prompt string) string {
	h := sha256.Sum256([]byte(prompt))
	return base64.RawURLEncoding.EncodeToString(h[:16]) // 16 bytes = 22 chars
}

// sanitizePrompt truncates a prompt for preview
func sanitizePrompt(prompt string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 80
	}
	trimmed := strings.TrimSpace(prompt)
	runes := []rune(trimmed)
	if len(runes) <= maxLen {
		return string(runes)
	}
	return string(runes[:maxLen]) + "..."
}

// buildRoutingEvent constructs a RoutingEvent from playground results.
// prompt text and fullPrompt are stored based on savePrompt setting.
func (s *playgroundServer) buildRoutingEvent(req playgroundRequest, response playgroundResponse, totalLatencyMs float64, started time.Time) *RoutingEvent {
	event := &RoutingEvent{
		RequestID:     fmt.Sprintf("req-%d", started.UnixNano()),
		CreatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		PromptHash:    hashPrompt(req.Prompt),
		PromptPreview: sanitizePrompt(req.Prompt, 80),
		HasImage:      req.HasImage,
		HasDocument:   req.HasDocument,
		HasCSV:        req.HasCSV,
		Mode:          req.Mode,
	}

	// Intent
	tu := response.TaskUnderstandingCard
	event.PrimaryIntent = tu.PrimaryIntent
	event.SecondaryIntents = tu.SecondaryIntents

	// Local Provider
	local := response.LocalProviderResult
	event.LocalProvider = local.Provider
	event.LocalDecision = local.DecisionName
	event.LocalCategory = local.SemanticCategory
	event.LocalPool = local.MappedPool
	event.LocalConfidence = local.Confidence
	event.LocalLatencyMs = local.LatencyMs

	// Official Provider
	official := response.OfficialProviderResult
	event.OfficialProvider = official.Provider
	event.OfficialDecision = official.DecisionName
	event.OfficialCategory = official.SemanticCategory
	event.OfficialPool = official.MappedPool
	event.OfficialConfidence = official.Confidence
	event.OfficialLatencyMs = official.LatencyMs

	// Pool
	trace := response.DecisionTrace
	event.RawPool = trace.PrimaryPool
	event.NormalizedPool = normalizePoolName(trace.PrimaryPool)
	if pc := response.PoolCard; pc.PhysicalModelGroup != "" {
		event.PhysicalModelGroup = pc.PhysicalModelGroup
	} else {
		event.PhysicalModelGroup = poolToPhysicalGroup[event.NormalizedPool]
	}
	// Official Physical Group
	officialPool := normalizePoolName(response.OfficialProviderResult.MappedPool)
	event.OfficialPhysicalGroup = poolToPhysicalGroup[officialPool]
	if event.OfficialPhysicalGroup == "" {
		event.OfficialPhysicalGroup = event.PhysicalModelGroup
	}

	// Tier
	tc := response.TierCard
	event.ComplexityScore = tc.ComplexityScore
	event.MinimumTier = tc.MinimumTier
	event.RequestedTier = tc.RequestedTier
	event.SelectedTier = tc.SelectedTier

	// Agreement
	event.SemanticAgreement = response.SemanticAgreement
	event.PoolAgreement = response.PoolAgreement
	event.PhysicalGroupAgreement = response.PhysicalGroupAgreement
	event.RouteLLMAgreement = response.RouteLLMAgreement
	if response.RouteLLMTier.RouteLLMProbability != nil {
		event.RouteLLMProbability = *response.RouteLLMTier.RouteLLMProbability
	}

	// Final
	if lp := response.LocalPoolDecision; lp != nil {
		if lpMap, ok := lp.(map[string]interface{}); ok {
			event.FinalPool, _ = lpMap["pool"].(string)
			event.FinalPoolSource = "local_pool_decision"
		}
	}
	if event.FinalPool == "" {
		event.FinalPool = event.NormalizedPool
		event.FinalPoolSource = "normalized"
	}
	event.SchedulerSource = "mock"

	// Runtime
	event.TotalLatencyMs = totalLatencyMs
	event.EmbeddingInvoked = response.Embedding.Invoked
	event.DryRun = true
	event.UpstreamCalled = false
	event.Takeover = "disabled"

	// Errors
	if local.Error != "" {
		event.Errors = append(event.Errors, "local: "+local.Error)
	}
	if official.Error != "" {
		event.Errors = append(event.Errors, "official: "+official.Error)
	}

	return event
}

//go:embed static
var staticFiles embed.FS

const defaultEmbeddingEndpoint = "http://127.0.0.1:8001"

type playgroundRequest struct {
	Prompt      string `json:"prompt"`
	HasImage    bool   `json:"has_image"`
	HasDocument bool   `json:"has_document"`
	HasCSV      bool   `json:"has_csv"`
	Mode        string `json:"mode"`
}

type poolScore struct {
	Pool  string  `json:"pool"`
	Score float64 `json:"score"`
}

type taskUnderstanding struct {
	Actions               []string    `json:"actions"`
	Objects               []string    `json:"objects"`
	Modalities            []string    `json:"modalities"`
	OutputArtifacts       []string    `json:"output_artifacts"`
	Intents               []string    `json:"intents"`
	PrimaryIntent         string      `json:"primary_intent"`
	SecondaryIntents      []string    `json:"secondary_intents"`
	RequiredCapabilities  []string    `json:"required_capabilities"`
	Confidence            float64     `json:"confidence"`
	Ambiguous             bool        `json:"ambiguous"`
	PoolScores            []poolScore `json:"pool_scores"`
	PrimaryPool           string      `json:"primary_pool"`
	SecondaryPool         string      `json:"secondary_pool"`
	UnderstandingConflict bool        `json:"understanding_conflict"`
}

type tierDecision struct {
	RequestedTier    string  `json:"requested_tier"`
	SelectedTier     string  `json:"selected_tier"`
	MatchedRule      string  `json:"matched_rule"`
	Reason           string  `json:"reason"`
	Confidence       float64 `json:"confidence"`
	PromptLength     int     `json:"prompt_length"`
	StepCount        int     `json:"step_count"`
	ConstraintCount  int     `json:"constraint_count"`
	MultiIntentCount int     `json:"multi_intent_count"`
	ComplexityScore  float64 `json:"complexity_score"`
}

// boundaryInfo Tier 边界不确定性信息
type boundaryInfo struct {
	// Raw Complexity Assessment
	RawComplexityScore float64 `json:"raw_complexity_score"` // 原始复杂度分数 (归一化 0~1)

	// Tier Uncertainty Assessment
	UncertaintyScore   float64  `json:"uncertainty_score"`    // 不确定性分数 0~1
	BoundaryEligible   bool     `json:"boundary_eligible"`    // 是否触发 RouteLLM
	Reasons            []string `json:"reasons"`              // 触发原因列表
	NearestBoundary    string   `json:"nearest_boundary"`     // "weak_medium" | "medium_strong" | "none"
	DistanceToBoundary float64  `json:"distance_to_boundary"` // 到边界的距离

	// Legacy compatibility
	Eligible      bool     `json:"eligible"`
	ReasonsLegacy []string `json:"reasons_legacy"`
}

// HybridConfig holds thresholds for hybrid pool shadow
type HybridConfig struct {
	LocalHighConfidence         float64
	OfficialHighConfidence      float64
	OfficialLowScore            float64
	MinMargin                   float64
	AbstainOnDoubleHighConflict bool
}

// HybridPoolInfo records the hybrid candidate pool decision (observational only)
type HybridPoolInfo struct {
	CandidatePool string  `json:"candidate_pool"`
	Source        string  `json:"source"`
	Confidence    float64 `json:"confidence"`
	Reason        string  `json:"reason"`
	Abstain       bool    `json:"abstain"`
	UsedForFinal  bool    `json:"used_for_final"`
}

// GroupFirstHybridInfo is a separate, observational recommendation. It never
// changes the local final Pool or the old Scheduler's selected account.
type GroupFirstHybridInfo struct {
	LocalGroup          string             `json:"local_group"`
	OfficialGroup       string             `json:"official_group"`
	SelectedGroup       string             `json:"selected_group"`
	SuggestedPool       string             `json:"suggested_pool"`
	LocalGroupScores    map[string]float64 `json:"local_group_scores"`
	OfficialGroupScores map[string]float64 `json:"official_group_scores"`
	FusedGroupScores    map[string]float64 `json:"fused_group_scores"`
	FusedPoolScores     []poolScore        `json:"fused_pool_scores"`
	Source              string             `json:"source"`
	Reason              string             `json:"reason"`
	Abstain             bool               `json:"abstain"`
	UsedForFinal        bool               `json:"used_for_final"`
	SuggestedAccountID  int64              `json:"suggested_account_id,omitempty"`
	SuggestedModel      string             `json:"suggested_model,omitempty"`
}

// hybridShadowInfo Hybrid Shadow Tier 信息
type hybridShadowInfo struct {
	// Raw Rule Tier → Policy Floor → Final Rule Tier
	RawRuleTier       string `json:"raw_rule_tier"`
	MinimumTier       string `json:"minimum_tier"`
	MinimumTierReason string `json:"minimum_tier_reason"`
	PolicyAdjusted    bool   `json:"policy_adjusted"`
	RuleTier          string `json:"rule_tier"`

	// Tier Uncertainty (new)
	UncertaintyScore float64 `json:"uncertainty_score"`
	BoundaryEligible bool    `json:"boundary_eligible"`

	// RouteLLM invocation context
	InvokedForComparison bool `json:"invoked_for_comparison"` // Compare All 模式
	InvokedForBoundary   bool `json:"invoked_for_boundary"`   // Boundary 模式
	RouteLLMUsedByHybrid bool `json:"routellm_used_by_hybrid"`

	// Hybrid suggestion
	SuggestedTier     string  `json:"suggested_tier"`
	DecisionReason    string  `json:"decision_reason"`
	Disagreement      bool    `json:"disagreement"`
	OverrideEligible  bool    `json:"override_eligible"`
	OverrideReason    string  `json:"override_reason"`
	OverrideThreshold float64 `json:"override_confidence_threshold"`
	UsedForFinal      bool    `json:"used_for_final"` // 永远为 false (shadow mode)
}

// vllmShadowInfo vLLM Semantic Shadow 信息 (四层结构)
type vllmShadowInfo struct {
	// Service status
	ServiceAvailable bool   `json:"service_available"`
	ServiceReady     bool   `json:"service_ready"`
	Invoked          bool   `json:"invoked"`
	InvocationReason string `json:"invocation_reason"`

	// Classification Method
	ClassificationMethod string `json:"classification_method"`

	// Layer 1: Raw Semantic Category
	RawCategory      string             `json:"raw_category"`
	RawConfidence    float64            `json:"raw_confidence"`
	RawProbabilities map[string]float64 `json:"raw_probabilities,omitempty"`

	// Legacy Mapped Pool (传统池)
	LegacyMappedPool string `json:"legacy_mapped_pool"`

	// Layer 2: Required Capabilities
	RequiredCapabilities []string `json:"required_capabilities"`

	// Layer 3: Proposed Execution Family
	ProposedExecutionFamily string `json:"proposed_execution_family"`

	// Local comparison
	LocalPool                string `json:"local_pool"`
	LegacyPoolAgreement      bool   `json:"legacy_pool_agreement"`
	ExecutionFamilyAgreement bool   `json:"execution_family_agreement"`

	// Final decision (永远使用本地)
	UsedForFinal   bool   `json:"used_for_final"` // 永远为 false
	FinalPool      string `json:"final_pool"`
	DecisionSource string `json:"decision_source"`

	// Error and latency
	LatencyMs float64 `json:"latency_ms"`
	Error     string  `json:"error,omitempty"`
}

type taskComplexity struct {
	StepCount        int
	ConstraintCount  int
	MultiIntentCount int
	Score            float64
	RequestedTier    semanticrouter.PreferredTier
}

// newArchitectureOutput 新架构可解释性输出
type newArchitectureOutput struct {
	Enabled           bool                  `json:"enabled"`
	TaskUnderstanding *taskUnderstandingNew `json:"task_understanding"`
	CandidatePools    []candidatePoolNew    `json:"candidate_pools"`
	RejectedPools     []rejectedPoolNew     `json:"rejected_pools"`
	FinalPool         string                `json:"final_pool"`
	Confidence        float64               `json:"confidence"`
	DecisionSource    string                `json:"decision_source"`
	EmbeddingUsed     bool                  `json:"embedding_used"`
	FallbackInfo      *fallbackInfoNew      `json:"fallback_info,omitempty"`
}

type taskUnderstandingNew struct {
	PrimaryIntent    string   `json:"primary_intent"`
	SecondaryIntents []string `json:"secondary_intents"`
	Actions          []string `json:"actions"`
	Objects          []string `json:"objects"`
	InputModalities  []string `json:"input_modalities"`
	OutputArtifacts  []string `json:"output_artifacts"`
	Confidence       float64  `json:"confidence"`
	Ambiguous        bool     `json:"ambiguous"`
	MissingInputs    []string `json:"missing_inputs"`
}

type candidatePoolNew struct {
	Pool      string   `json:"pool"`
	Score     float64  `json:"score"`
	Evidence  []string `json:"evidence"`
	Validated bool     `json:"validated"`
}

type rejectedPoolNew struct {
	Pool            string `json:"pool"`
	RejectionReason string `json:"rejection_reason"`
}

type fallbackInfoNew struct {
	FallbackPool    string   `json:"fallback_pool"`
	FallbackReason  string   `json:"fallback_reason"`
	ConflictDetails []string `json:"conflict_details"`
}

type baselineResult struct {
	BestPool    string      `json:"best_pool"`
	BestScore   float64     `json:"best_score"`
	SecondPool  string      `json:"second_pool"`
	SecondScore float64     `json:"second_score"`
	ScoreMargin float64     `json:"score_margin"`
	Tier        string      `json:"tier"`
	LatencyMS   float64     `json:"latency_ms"`
	TopK        []poolScore `json:"top_k"`
	AllScores   []poolScore `json:"all_scores"`
}

type embeddingResult struct {
	Invoked          bool               `json:"invoked"`
	InvocationReason string             `json:"invocation_reason"`
	Scores           map[string]float64 `json:"scores"`
	BestPool         string             `json:"best_pool"`
	BestScore        float64            `json:"best_score"`
	SecondBestPool   string             `json:"second_best_pool"`
	SecondBestScore  float64            `json:"second_best_score"`
	ScoreMargin      float64            `json:"score_margin"`
	ModelName        string             `json:"model_name"`
	InferenceLatency float64            `json:"inference_latency_ms"`
	HTTPLatency      float64            `json:"http_latency_ms"`
	RankedPools      []poolScore        `json:"ranked_pools"`
	ServiceAvailable bool               `json:"service_available"`
	Error            string             `json:"error,omitempty"`
}

type selectiveResult struct {
	Eligible       bool    `json:"eligible"`
	Invoked        bool    `json:"invoked"`
	Override       bool    `json:"override"`
	OverrideReason string  `json:"override_reason"`
	Reason         string  `json:"reason"`
	FinalPool      string  `json:"final_pool"`
	FinalTier      string  `json:"final_tier"`
	PrimaryPool    string  `json:"primary_pool"`
	SecondaryPool  string  `json:"secondary_pool"`
	DecisionSource string  `json:"decision_source"`
	LatencyMS      float64 `json:"latency_ms"`
}

type schedulerResult struct {
	SelectedAccountID     int64                                `json:"selected_account_id"`
	SelectedModel         string                               `json:"selected_model"`
	SelectedPool          string                               `json:"selected_pool"`
	SchedulerLayer        string                               `json:"scheduler_layer"`
	RequestedTier         string                               `json:"requested_tier"`
	SelectedTier          string                               `json:"selected_tier"`
	Source                string                               `json:"scheduler_source"`
	DryRun                bool                                 `json:"dry_run"`
	UpstreamCalled        bool                                 `json:"upstream_called"`
	CandidateCount        int                                  `json:"candidate_count"`
	CandidateModels       []string                             `json:"candidate_models"`
	CandidateDetails      []semanticrouter.ModelCandidateScore `json:"candidate_details"`
	ModelRanking          *semanticrouter.ModelRankingResult   `json:"model_ranking,omitempty"`
	RecommendedModel      string                               `json:"recommended_model"`
	RecommendationReason  string                               `json:"recommendation_reason"`
	OldSelectedAccountID  int64                                `json:"old_selected_account_id"`
	NewSuggestedAccountID int64                                `json:"new_suggested_account_id"`
	OldSelectedModel      string                               `json:"old_selected_model"`
	NewSuggestedModel     string                               `json:"new_suggested_model"`
	OldVsNewAgreement     bool                                 `json:"old_vs_new_agreement"`
	RankingMargin         float64                              `json:"ranking_margin"`
	DecisionSource        string                               `json:"decision_source"`
	FallbackReason        string                               `json:"fallback_reason,omitempty"`
	Error                 string                               `json:"error,omitempty"`
}

// IntentMapping maps semantic intent to logical pool
type intentMapping struct {
	PrimaryIntent    string   `json:"primary_intent"`
	SecondaryIntents []string `json:"secondary_intents"`
	Intents          []string `json:"intents"`
}

// PoolMapping maps logical pool to physical model group
type poolMapping struct {
	LogicalPool        string `json:"logical_pool"`
	PhysicalModelGroup string `json:"physical_model_group"`
}

// TierSummary summarizes tier decision
type tierSummary struct {
	ComplexityScore   float64 `json:"complexity_score"`
	RawTier           string  `json:"raw_tier"`
	MinimumTier       string  `json:"minimum_tier"`
	MinimumTierReason string  `json:"minimum_tier_reason"`
	RequestedTier     string  `json:"requested_tier"`
	SelectedTier      string  `json:"selected_tier"`
	TierSource        string  `json:"tier_source"`
}

// SchedulerSummary summarizes scheduler decision
type schedulerSummary struct {
	RequestedPool         string                               `json:"requested_pool"`
	RequestedTier         string                               `json:"requested_tier"`
	PhysicalModelGroup    string                               `json:"physical_model_group"`
	SelectedModel         string                               `json:"selected_model"`
	SelectedAccountID     int64                                `json:"selected_account_id"`
	SchedulerSource       string                               `json:"scheduler_source"`
	FallbackReason        string                               `json:"fallback_reason"`
	DryRun                bool                                 `json:"dry_run"`
	UpstreamCalled        bool                                 `json:"upstream_called"`
	CandidateCount        int                                  `json:"candidate_count"`
	CandidateModels       []string                             `json:"candidate_models"`
	CandidateDetails      []semanticrouter.ModelCandidateScore `json:"candidate_details"`
	RecommendedModel      string                               `json:"recommended_model"`
	RecommendationReason  string                               `json:"recommendation_reason"`
	OldSelectedAccountID  int64                                `json:"old_selected_account_id"`
	NewSuggestedAccountID int64                                `json:"new_suggested_account_id"`
	OldSelectedModel      string                               `json:"old_selected_model"`
	NewSuggestedModel     string                               `json:"new_suggested_model"`
	OldVsNewAgreement     bool                                 `json:"old_vs_new_agreement"`
	RankingMargin         float64                              `json:"ranking_margin"`
	DecisionSource        string                               `json:"decision_source"`
}

// ProviderResult summarizes a router provider's output
type providerResultSummary struct {
	Provider         string             `json:"provider"`
	SemanticCategory string             `json:"semantic_category"`
	DecisionName     string             `json:"decision_name"`
	MatchedSignals   map[string]float64 `json:"matched_signals"`
	Confidence       float64            `json:"confidence"`
	MappedPool       string             `json:"mapped_pool"`
	UsedForFinal     bool               `json:"used_for_final"`
	LatencyMs        float64            `json:"latency_ms"`
	Error            string             `json:"error"`
}

// intentToPool maps semantic intent to logical pool
var intentToPool = map[string]string{
	"code_generation":     "code_pool",
	"data_analysis":       "data_pool",
	"document_processing": "document_pool",
	"image_understanding": "vision_pool",
	"image_generation":    "image_generation_pool",
	"simple_chat":         "general_text_pool",
	"general_chat":        "general_text_pool",
}

// poolToPhysicalGroup maps logical pool to physical model group
var poolToPhysicalGroup = map[string]string{
	"code_pool":             "technical_models",
	"data_pool":             "technical_models",
	"document_pool":         "technical_models",
	"vision_pool":           "vision_models",
	"image_generation_pool": "image_models",
	"general_text_pool":     "general_chat_models",
	"cheap_chat_pool":       "general_chat_models",
	"general_pool":          "general_chat_models",
}

type playgroundResponse struct {
	Input         playgroundRequest           `json:"input"`
	PromptParsing semanticrouter.ParsedPrompt `json:"prompt_parsing"`
	HardRules     struct {
		Matched      []string `json:"matched"`
		Decision     string   `json:"decision"`
		Capabilities []string `json:"capabilities"`
	} `json:"hard_rules"`
	DecisionTrace struct {
		MatchedRules          []string          `json:"matched_rules"`
		MatchedKeywords       []string          `json:"matched_keywords"`
		PrimaryPool           string            `json:"primary_pool"`
		SecondaryPool         string            `json:"secondary_pool"`
		DetectedIntents       []string          `json:"detected_intents"`
		RequiredCapabilities  []string          `json:"required_capabilities"`
		PoolTopScores         []poolScore       `json:"pool_top_scores"`
		Top1Top2Margin        float64           `json:"top1_top2_margin"`
		Confidence            float64           `json:"confidence"`
		DecisionSource        string            `json:"final_decision_source"`
		FallbackReason        string            `json:"fallback_reason,omitempty"`
		UnderstandingConflict bool              `json:"understanding_conflict"`
		TaskUnderstanding     taskUnderstanding `json:"task_understanding"`
		TierDecision          tierDecision      `json:"tier_decision"`
		Boundary              boundaryInfo      `json:"boundary"`
	} `json:"decision_trace"`
	Baseline     baselineResult                    `json:"baseline"`
	Embedding    embeddingResult                   `json:"embedding"`
	Selective    selectiveResult                   `json:"selective"`
	RouteLLMTier semanticrouter.HybridTierDecision `json:"routellm_tier"`
	HybridShadow hybridShadowInfo                  `json:"hybrid_shadow"`
	// 三种 Provider 结果
	LocalPoolDecision  interface{}     `json:"local_pool_decision"`           // LocalPoolDecisionResult
	LocalE5Shadow      interface{}     `json:"local_e5_semantic_shadow"`      // LocalE5ShadowResult
	OfficialVLLMShadow interface{}     `json:"official_vllm_semantic_shadow"` // OfficialVLLMShadowResult
	VLLMShadow         vllmShadowInfo  `json:"vllm_shadow"`                   // 保留兼容
	Scheduler          schedulerResult `json:"scheduler"`
	// 新架构可解释性输出
	NewArchitecture *newArchitectureOutput `json:"new_architecture,omitempty"`
	// Hybrid Pool Shadow — observational, never used for final
	HybridPool       HybridPoolInfo       `json:"hybrid_pool"`
	GroupFirstHybrid GroupFirstHybridInfo `json:"group_first_hybrid"`
	// New layered routing architecture
	TaskUnderstandingCard  intentMapping         `json:"task_understanding_card"`
	LocalProviderResult    providerResultSummary `json:"local_provider_result"`
	OfficialProviderResult providerResultSummary `json:"official_provider_result"`
	PoolCard               poolMapping           `json:"pool_card"`
	OfficialPhysicalGroup  string                `json:"official_physical_group"`
	TierCard               tierSummary           `json:"tier_card"`
	SchedulerCard          schedulerSummary      `json:"scheduler_card"`
	SemanticAgreement      bool                  `json:"semantic_agreement"`
	PoolAgreement          bool                  `json:"pool_agreement"`
	PhysicalGroupAgreement bool                  `json:"physical_group_agreement"`
	RouteLLMAgreement      bool                  `json:"routellm_agreement"`
	RouteLLMProbability    float64               `json:"routellm_probability,omitempty"`
	// V2 Decision Trace (parallel, shadow only)
	V2Decision *semanticrouter.V2Decision `json:"v2_decision,omitempty"`
	Debug      struct {
		TakeoverDisabled           bool   `json:"takeover_disabled"`
		EmbeddingHealth            string `json:"embedding_health"`
		SchedulerSource            string `json:"scheduler_source"`
		DryRun                     bool   `json:"dry_run"`
		UpstreamCalled             bool   `json:"upstream_called"`
		ComparisonEmbeddingInvoked bool   `json:"comparison_embedding_invoked"`
		SelectiveEligible          bool   `json:"selective_eligible"`
		SelectiveEmbeddingInvoked  bool   `json:"selective_embedding_invoked"`
		EmbeddingUsedForDecision   bool   `json:"embedding_used_for_final_decision"`
		FinalDecisionSource        string `json:"final_decision_source"`
		Timestamp                  string `json:"timestamp"`
		UseNewArchitecture         bool   `json:"use_new_architecture"`
		EventsWritten              int64  `json:"events_written"`
		WriteErrors                int64  `json:"write_errors"`
	} `json:"debug"`
}

type playgroundServer struct {
	router             *semanticrouter.MultiLayerRouter
	enhancedRouter     *semanticrouter.EnhancedMultiLayerRouter
	v2Router           *semanticrouter.V2Router
	tierRouter         *semanticrouter.RuleBasedTierRouter
	scheduler          *semanticrouter.RealSchedulerDryRun
	modelGroup         string
	routeLLMConfig     semanticrouter.RouteLLMTierConfig
	routeLLMClient     *semanticrouter.RouteLLMTierClient
	routeLLM           *semanticrouter.HybridTierScorer
	boundaryHybrid     *semanticrouter.BoundaryHybridTierRouter
	embeddingEndpoint  string
	httpClient         *http.Client
	useNewArchitecture bool // 新增：是否使用新架构
	vllmClient         *vllm_pool_client.VLLMSemanticPoolRouterClient
	vllmAdapter        *vllm_pool_client.VLLMPoolDecisionAdapter
	eventLogger        *RoutingEventLogger
	store              *FileStore // 新增：JSONL 持久化存储
}

func main() {
	server := newPlaygroundServer()

	// Serve static files (HTML, CSS, JS)
	http.HandleFunc("/debug/router-playground", server.handleHTML)
	http.Handle("/static/", http.FileServer(http.FS(staticFiles)))

	// API endpoints
	http.HandleFunc("/v1/debug/router/playground", server.handleAPI)
	http.HandleFunc("/debug/embedding-health", server.handleEmbeddingHealth)
	http.HandleFunc("/debug/routellm-tier-health", server.handleRouteLLMHealth)
	http.HandleFunc("/v1/debug/events", server.handleEventsAPI)
	http.HandleFunc("/v1/debug/events/export", server.handleExportAPI)
	http.HandleFunc("/v1/debug/events/import", server.handleImportAPI)
	http.HandleFunc("/v1/debug/events/evaluate", server.handleBatchEvaluate)
	http.HandleFunc("/debug/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "router-playground", "takeover_disabled": true})
	})
	http.HandleFunc("/v1/debug/staging/accounts", server.handleStagingAccounts)
	http.HandleFunc("/v1/debug/platform/model-groups", server.handlePlatformModelGroups)
	// === 新记录闭环端点 ===
	http.HandleFunc("/v1/debug/records/save", server.handleSaveResult)
	http.HandleFunc("/v1/debug/records/list", server.handleRecordsList)
	http.HandleFunc("/v1/debug/records/get", server.handleGetRecord)
	http.HandleFunc("/v1/debug/records/review", server.handleSaveReview)
	http.HandleFunc("/v1/debug/records/export", server.handleExportRecords)
	http.HandleFunc("/v1/debug/records/stats", server.handleRecordsStats)

	port := os.Getenv("PLAYGROUND_PORT")
	if port == "" {
		port = "8081"
	}
	listenAddress := strings.TrimSpace(os.Getenv("PLAYGROUND_LISTEN_ADDRESS"))
	if listenAddress == "" {
		listenAddress = "127.0.0.1:" + port
	}
	fmt.Printf("Router Playground listening on http://%s/debug/router-playground\n", listenAddress)
	if err := http.ListenAndServe(listenAddress, nil); err != nil {
		fmt.Println("playground stopped:", err)
	}
}

// handleStagingAccounts exposes the repository-backed staging catalog for
// inspection only. It never exposes credentials and never calls an upstream.
func (s *playgroundServer) handleStagingAccounts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "GET required"})
		return
	}
	groupID := int64(1001)
	if raw := strings.TrimSpace(r.URL.Query().Get("group_id")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid group_id"})
			return
		}
		groupID = parsed
	}
	accounts, err := semanticrouter.NewStagingAccountRepository().ListRoutingAccounts(r.Context(), &groupID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	type accountView struct {
		AccountID        int64   `json:"account_id"`
		Model            string  `json:"model"`
		Provider         string  `json:"provider"`
		Pool             string  `json:"pool"`
		Tier             string  `json:"tier"`
		SupportsCode     bool    `json:"supports_code"`
		SupportsData     bool    `json:"supports_data"`
		SupportsVision   bool    `json:"supports_vision"`
		SupportsDocument bool    `json:"supports_document"`
		Status           string  `json:"status"`
		Schedulable      bool    `json:"schedulable"`
		Priority         int     `json:"priority"`
		Price            float64 `json:"price"`
		Concurrency      int     `json:"concurrency"`
	}
	views := make([]accountView, 0, len(accounts))
	modelSet := make(map[string]struct{})
	for _, account := range accounts {
		if account == nil {
			continue
		}
		views = append(views, accountView{
			AccountID: account.ID, Model: account.Model, Provider: account.Platform,
			Pool: account.Pool, Tier: string(account.Tier), SupportsCode: account.CodeCapable,
			SupportsData: account.DataCapable, SupportsVision: account.VisionCapable,
			SupportsDocument: account.DocumentCapable, Status: account.Status,
			Schedulable: account.Schedulable, Priority: account.Priority, Price: account.Price,
			Concurrency: account.ConcurrencyLimit,
		})
		modelSet[account.Model] = struct{}{}
	}
	models := make([]string, 0, len(modelSet))
	for model := range modelSet {
		models = append(models, model)
	}
	sort.Strings(models)
	sort.Slice(views, func(i, j int) bool { return views[i].AccountID < views[j].AccountID })
	writeJSON(w, http.StatusOK, map[string]any{
		"group_id": groupID, "account_count": len(views), "model_count": len(models),
		"models": models, "accounts": views, "shadow_only": true, "upstream_called": false,
	})
}

func (s *playgroundServer) handlePlatformModelGroups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "GET required"})
		return
	}
	groups := semanticrouter.PlatformModelGroups()
	modelSet := make(map[string]struct{})
	for _, group := range groups {
		for _, model := range group.Models {
			modelSet[model] = struct{}{}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"group_count": len(groups), "model_count": len(modelSet), "groups": groups,
		"api_key_scope": "one_model_group_per_api_key", "shadow_only": true, "upstream_called": false,
	})
}

func newPlaygroundServer() *playgroundServer {
	endpoint := os.Getenv("EMBEDDING_ENDPOINT")
	if endpoint == "" {
		endpoint = defaultEmbeddingEndpoint
	}
	routeLLMConfig := semanticrouter.DefaultRouteLLMTierConfig()
	routeLLMConfig.Enabled = envBool("ROUTELLM_TIER_ENABLED", true) // Enable by default for playground
	routeLLMConfig.ShadowOnly = true
	routeLLMConfig.TakeoverEnabled = false
	routeLLMConfig.ServiceURL = "http://127.0.0.1:8002" // Default RouteLLM service URL
	if value := strings.TrimSpace(os.Getenv("ROUTELLM_TIER_SERVICE_URL")); value != "" {
		routeLLMConfig.ServiceURL = value
	}
	if value := envPositiveInt("ROUTELLM_TIER_TIMEOUT_MS"); value > 0 {
		routeLLMConfig.TimeoutMS = value
	}
	routeLLMClient := semanticrouter.NewRouteLLMTierClient(routeLLMConfig)
	useNewArch := envBool("USE_NEW_ARCHITECTURE", false) // 默认使用旧架构，保持兼容

	// 初始化 vLLM 客户端
	vllmConfig := vllm_pool_client.DefaultVLLMPoolConfig()
	vllmConfig.Enabled = envBool("VLLM_POOL_ENABLED", false) // 默认关闭，需要显式启用
	vllmConfig.ShadowOnly = true
	vllmConfig.MockMode = envBool("VLLM_POOL_MOCK_MODE", false) // 默认关闭 mock，使用真实 vLLM
	if value := strings.TrimSpace(os.Getenv("VLLM_POOL_SERVICE_URL")); value != "" {
		vllmConfig.BaseURL = value
	}
	vllmClient := vllm_pool_client.NewVLLMSemanticPoolRouterClient(vllmConfig)
	vllmAdapter := vllm_pool_client.NewVLLMPoolDecisionAdapter(vllmClient)
	vllmAdapter.SetMockMode(vllmConfig.MockMode) // 设置 mock 模式

	// 初始化 V2 Router，共享 vllm adapter 的 E5 分类器
	v2RouterInstance := semanticrouter.NewV2Router(semanticrouter.NewMultiLayerRouter())
	if vllmAdapter != nil {
		v2RouterInstance.SetE5Classifier(vllmAdapter.E5Classifier())
		v2RouterInstance.SetPrototypeClassifier(vllmAdapter.PrototypeClassifierV2())
	}

	// 初始化 JSONL 存储
	storeDir := os.Getenv("ROUTER_STORE_DIR")
	if storeDir == "" {
		storeDir = "router_store"
	}
	store, err := NewFileStore(storeDir)
	if err != nil {
		fmt.Printf("WARNING: failed to initialize store at %s: %v\n", storeDir, err)
	}

	// Staging mode uses the repository-backed candidate catalog while keeping
	// the scheduler dry-run and never calling an upstream model.
	scheduler := semanticrouter.NewDefaultRealSchedulerDryRun()
	mode := strings.TrimSpace(os.Getenv("PLAYGROUND_SCHEDULER_MODE"))
	modelGroup := ""
	if strings.EqualFold(mode, "platform") {
		modelGroup = strings.TrimSpace(os.Getenv("PLAYGROUND_MODEL_GROUP"))
		if modelGroup == "" {
			modelGroup = "国外OPENAI分组"
		}
		scheduler = semanticrouter.NewPlatformRealScheduler(modelGroup)
	} else if strings.EqualFold(mode, "staging") {
		scheduler = semanticrouter.NewStagingRealScheduler(1001)
	}

	return &playgroundServer{
		router:             semanticrouter.NewMultiLayerRouter(),
		enhancedRouter:     semanticrouter.NewEnhancedMultiLayerRouter(),
		v2Router:           v2RouterInstance,
		tierRouter:         semanticrouter.NewRuleBasedTierRouter(),
		scheduler:          scheduler,
		modelGroup:         modelGroup,
		routeLLMConfig:     routeLLMConfig,
		routeLLMClient:     routeLLMClient,
		routeLLM:           semanticrouter.NewHybridTierScorer(routeLLMConfig, routeLLMClient),
		boundaryHybrid:     semanticrouter.NewBoundaryHybridTierRouter(semanticrouter.NewRuleBasedTierRouter(), routeLLMClient),
		embeddingEndpoint:  strings.TrimRight(endpoint, "/"),
		httpClient:         &http.Client{Timeout: 1500 * time.Millisecond},
		useNewArchitecture: useNewArch,
		vllmClient:         vllmClient,
		vllmAdapter:        vllmAdapter,
		eventLogger:        NewRoutingEventLogger(""),
		store:              store,
	}
}

func (s *playgroundServer) handleAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
		return
	}
	defer r.Body.Close()
	var req playgroundRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "prompt is required"})
		return
	}
	if !validMode(req.Mode) {
		req.Mode = "baseline"
	}

	started := time.Now()
	response := s.evaluate(req)
	totalLatencyMs := elapsedMS(started)

	// Auto-record routing event if not disabled via request
	recordEvent := true
	if r.URL.Query().Get("record") == "false" {
		recordEvent = false
	}
	// Also check request body for auto_record flag
	if !recordEvent {
		// checked via query param
	} else if req.Mode == "import" {
		// Don't auto-record imports as separate events
		recordEvent = false
	}

	if recordEvent && s.eventLogger != nil {
		event := s.buildRoutingEvent(req, response, totalLatencyMs, started)
		s.eventLogger.Append(event)
	}

	// Add event count and write errors to response
	type loggingInfo struct {
		EventsWritten int64 `json:"events_written"`
		WriteErrors   int64 `json:"write_errors"`
	}
	response.Debug.EventsWritten = s.eventLogger.WrittenEvents()
	response.Debug.WriteErrors = s.eventLogger.WriteErrors()

	writeJSON(w, http.StatusOK, response)
}

// handleEventsAPI serves saved routing events
func (s *playgroundServer) handleEventsAPI(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 1000 {
		limit = n
	}
	events := s.eventLogger.ReadEvents(limit)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"events":       events,
		"count":        len(events),
		"events_total": s.eventLogger.WrittenEvents(),
		"write_errors": s.eventLogger.WriteErrors(),
	})
}

// handleExportAPI exports events as JSONL or CSV
func (s *playgroundServer) handleExportAPI(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	if format == "csv" {
		data := s.eventLogger.ExportCSV()
		if data == "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=routing_events.csv")
		w.Write([]byte(data))
		return
	}
	// default: JSONL
	data := s.eventLogger.ExportJSONL()
	if data == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/jsonl; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=routing_events.jsonl")
	w.Write([]byte(data))
}

// DatasetEntry represents a single record in an imported dataset
type DatasetEntry struct {
	ID             string `json:"id"`
	Prompt         string `json:"prompt"`
	Language       string `json:"language"`
	Source         string `json:"source"`
	HasImage       bool   `json:"has_image"`
	HasDocument    bool   `json:"has_document"`
	HasData        bool   `json:"has_data"`
	ExpectedIntent string `json:"expected_intent"`
	ExpectedPool   string `json:"expected_pool"`
	ExpectedTier   string `json:"expected_tier"`
}

// handleImportAPI processes uploaded dataset files
func (s *playgroundServer) handleImportAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
		return
	}
	defer r.Body.Close()

	contentType := r.Header.Get("Content-Type")
	var entries []DatasetEntry

	if strings.Contains(contentType, "multipart/form-data") {
		// File upload
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to parse upload: " + err.Error()})
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no file uploaded"})
			return
		}
		defer file.Close()

		content, err := io.ReadAll(file)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read file"})
			return
		}

		entries = parseDatasetFile(string(content), header.Filename)
	} else {
		// Direct JSON body
		var direct struct {
			Entries []DatasetEntry `json:"entries"`
		}
		if err := json.NewDecoder(r.Body).Decode(&direct); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		entries = direct.Entries
	}

	if len(entries) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "no entries found", "count": 0})
		return
	}

	// Import as events: evaluate each entry and record
	results := make([]map[string]interface{}, 0, len(entries))
	importErrors := 0
	for _, entry := range entries {
		if strings.TrimSpace(entry.Prompt) == "" {
			importErrors++
			continue
		}
		// Create a playground request
		importReq := playgroundRequest{
			Prompt:      entry.Prompt,
			HasImage:    entry.HasImage,
			HasDocument: entry.HasDocument,
			HasCSV:      entry.HasData,
			Mode:        "baseline",
		}
		started := time.Now()
		resp := s.evaluate(importReq)
		totalLatencyMs := elapsedMS(started)

		// Build routing event
		event := s.buildRoutingEvent(importReq, resp, totalLatencyMs, started)
		// Add expected values if provided
		if entry.ExpectedIntent != "" {
			event.ExpectedIntent = entry.ExpectedIntent
		}
		if entry.ExpectedPool != "" {
			event.ExpectedPool = entry.ExpectedPool
		}
		if entry.ExpectedTier != "" {
			event.ExpectedTier = entry.ExpectedTier
		}
		event.PromptPreview = sanitizePrompt(entry.Prompt, 120)
		// Check if this exact hash already exists
		s.eventLogger.Append(event)

		results = append(results, map[string]interface{}{
			"id":               entry.ID,
			"prompt_hash":      event.PromptHash,
			"prompt_preview":   event.PromptPreview,
			"local_decision":   event.LocalDecision,
			"local_pool":       event.LocalPool,
			"requested_tier":   event.RequestedTier,
			"total_latency_ms": totalLatencyMs,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"imported":     len(results),
		"errors":       importErrors,
		"total_events": s.eventLogger.WrittenEvents(),
		"write_errors": s.eventLogger.WriteErrors(),
		"results":      results,
	})
}

// handleBatchEvaluate is a lightweight endpoint for batch replay.
// It accepts a single prompt and returns the evaluation result as a flat map.
func (s *playgroundServer) handleBatchEvaluate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
		return
	}
	defer r.Body.Close()

	var req playgroundRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "prompt is required"})
		return
	}
	if !validMode(req.Mode) {
		req.Mode = "baseline"
	}

	result := s.evaluate(req)
	writeJSON(w, http.StatusOK, result)
}

func parseDatasetFile(content string, filename string) []DatasetEntry {
	lower := strings.ToLower(filename)
	if strings.HasSuffix(lower, ".csv") {
		return parseCSVDataset(content)
	}
	// default: JSONL
	return parseJSONLDataset(content)
}

func parseJSONLDataset(content string) []DatasetEntry {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	entries := make([]DatasetEntry, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry DatasetEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	return entries
}

func parseCSVDataset(content string) []DatasetEntry {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) < 2 {
		return nil
	}
	header := parseCSVLine(lines[0])
	colMap := make(map[string]int)
	for i, h := range header {
		colMap[strings.TrimSpace(strings.ToLower(h))] = i
	}
	entries := make([]DatasetEntry, 0, len(lines)-1)
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := parseCSVLine(line)
		var entry DatasetEntry
		if idx, ok := colMap["id"]; ok && idx < len(fields) {
			entry.ID = fields[idx]
		}
		if idx, ok := colMap["prompt"]; ok && idx < len(fields) {
			entry.Prompt = fields[idx]
		}
		if idx, ok := colMap["language"]; ok && idx < len(fields) {
			entry.Language = fields[idx]
		}
		if idx, ok := colMap["source"]; ok && idx < len(fields) {
			entry.Source = fields[idx]
		}
		if idx, ok := colMap["has_image"]; ok && idx < len(fields) {
			entry.HasImage = fields[idx] == "true" || fields[idx] == "1"
		}
		if idx, ok := colMap["has_document"]; ok && idx < len(fields) {
			entry.HasDocument = fields[idx] == "true" || fields[idx] == "1"
		}
		if idx, ok := colMap["has_data"]; ok && idx < len(fields) {
			entry.HasData = fields[idx] == "true" || fields[idx] == "1"
		}
		if idx, ok := colMap["expected_intent"]; ok && idx < len(fields) {
			entry.ExpectedIntent = fields[idx]
		}
		if idx, ok := colMap["expected_pool"]; ok && idx < len(fields) {
			entry.ExpectedPool = fields[idx]
		}
		if idx, ok := colMap["expected_tier"]; ok && idx < len(fields) {
			entry.ExpectedTier = fields[idx]
		}
		entries = append(entries, entry)
	}
	return entries
}

func parseCSVLine(line string) []string {
	var fields []string
	var current strings.Builder
	inQuotes := false
	for _, ch := range line {
		switch {
		case ch == '"':
			inQuotes = !inQuotes
		case ch == ',' && !inQuotes:
			fields = append(fields, current.String())
			current.Reset()
		default:
			current.WriteRune(ch)
		}
	}
	fields = append(fields, current.String())
	return fields
}

func (s *playgroundServer) evaluate(req playgroundRequest) playgroundResponse {
	started := time.Now()
	parsedPrompt := semanticrouter.ParsePrompt(req.Prompt, req.HasImage, req.HasDocument, req.HasCSV)
	routingPrompt := parsedPrompt.RoutingPrompt
	normalizedReq := req
	normalizedReq.Prompt = routingPrompt
	routeReq := &semanticrouter.RouteRequest{Model: "gpt-4", Prompt: routingPrompt, HasImage: req.HasImage, HasDocument: req.HasDocument}
	decision := s.router.Route(routeReq)
	tier, err := s.tierRouter.RouteWithPrompt(nil, routeReq.Model, decision.TaskType, routingPrompt)
	if err != nil {
		tier = &semanticrouter.TierRouteDecision{PreferredTier: semanticrouter.TierMedium, Reason: "tier router fallback"}
	}
	scores := rankedScores(decision.SemanticScores)
	primary, secondary := poolName(decision.PreferredPool), poolName(decision.SecondBestPool)
	baselineKeywords := matchedKeywords(req.Prompt, decision.PreferredPool)
	understanding := understand(normalizedReq, decision, scores, primary, secondary, baselineKeywords)
	complexity := analyzeTaskComplexity(routingPrompt, understanding)
	// Tier 决策: 结合规则判定和复杂度分数
	// 统一量纲: 复杂度分数已归一化到 0~1
	// - < 0.15 → Weak
	// - 0.15 ~ 0.50 → Medium
	// - ≥ 0.50 → Strong
	requestedTier := tier.PreferredTier

	// 复杂度判定优先级高于规则判定
	if complexity.RequestedTier == semanticrouter.TierStrong {
		requestedTier = semanticrouter.TierStrong
	} else if complexity.RequestedTier == semanticrouter.TierWeak {
		// 复杂度为 Weak 时，降级 tier（如果规则没有强制要求更高）
		requestedTier = semanticrouter.TierWeak
	}
	// RouteLLM is an observational scorer.
	// 注意: 传入基于复杂度分数的 requestedTier（统一量纲后），而不是规则判定的 tier.PreferredTier
	routeLLMDecision := s.routeLLM.Decide(context.Background(), routingPrompt, requestedTier)
	baseline := baselineResult{BestPool: primary, BestScore: scoreFor(decision.SemanticScores, decision.PreferredPool), SecondPool: secondary, SecondScore: decision.SecondBestScore, ScoreMargin: decision.ScoreMargin, Tier: string(requestedTier), TopK: topK(scores, 3), AllScores: scores}
	baseline.LatencyMS = elapsedMS(started)

	embeddingRequested := req.Mode == "full_embedding" || req.Mode == "compare"
	eligible := decision.ScoreMargin < 0.15 || decision.DecisionSource == semanticrouter.DecisionSourceFallback
	if req.Mode == "selective" && eligible {
		embeddingRequested = true
	}
	embedding := s.embedding(routingPrompt, embeddingRequested)
	finalPool, finalTier, source := decision.PreferredPool, requestedTier, "baseline"
	selective := selectiveResult{Eligible: eligible, Invoked: embedding.Invoked, FinalPool: poolName(finalPool), FinalTier: string(finalTier), PrimaryPool: primary, SecondaryPool: secondary, DecisionSource: source}
	if !eligible {
		selective.Reason = "baseline_margin_is_clear"
	} else if !embedding.Invoked {
		selective.Reason = "embedding_unavailable_fallback_to_baseline"
		selective.OverrideReason = embedding.InvocationReason
	} else {
		selective.Reason = "embedding_checked"
	}
	if req.Mode == "selective" || req.Mode == "compare" {
		if embedding.Invoked && embedding.BestPool != "" && embedding.BestPool != primary && embedding.ScoreMargin >= 0.10 {
			finalPool = preferredPool(embedding.BestPool)
			finalTier = tierForPool(finalPool, finalTier)
			selective.Override, selective.OverrideReason, selective.DecisionSource = true, "embedding_margin_is_clear", "selective_override"
			selective.FinalPool, selective.FinalTier = poolName(finalPool), string(finalTier)
			source = selective.DecisionSource
		} else if embedding.Invoked {
			selective.OverrideReason = "embedding_low_confidence"
		}
	}
	selective.LatencyMS = elapsedMS(started) - baseline.LatencyMS

	scheduled, schedulerFallback := s.scheduleWithinModelGroup(finalPool, finalTier, decision.TaskType, modelTaskSignals(routingPrompt, understanding.RequiredCapabilities), decision.RequiredCapabilities)
	modelName := scheduled.SelectedModel
	if modelName == "" {
		modelName = "none"
	}
	recommendedModel, recommendationReason := recommendedModelFromProfiles(modelName, scheduled.CandidateDetails)
	recommendedAccountID, rankingMargin := recommendedAccountAndMargin(scheduled.CandidateDetails)
	scheduler := schedulerResult{SelectedAccountID: scheduled.SelectedAccountID, SelectedModel: modelName, SelectedPool: scheduled.PoolUsed, SchedulerLayer: scheduled.Layer, RequestedTier: string(finalTier), SelectedTier: scheduled.MatchedTier, Source: "mock", DryRun: true, UpstreamCalled: false, CandidateCount: scheduled.CandidateCount, CandidateModels: scheduled.CandidateModels, CandidateDetails: scheduled.CandidateDetails, ModelRanking: buildPlaygroundModelRanking(scheduled.PoolUsed, scheduled.CandidateDetails), RecommendedModel: recommendedModel, RecommendationReason: recommendationReason, OldSelectedAccountID: scheduled.SelectedAccountID, NewSuggestedAccountID: recommendedAccountID, OldSelectedModel: modelName, NewSuggestedModel: recommendedModel, OldVsNewAgreement: modelName != "" && modelName == recommendedModel, RankingMargin: rankingMargin, DecisionSource: scheduled.DecisionSource, FallbackReason: schedulerFallback}
	if scheduled.Error != nil {
		scheduler.Error = scheduled.Error.Error()
	}

	var response playgroundResponse
	response.Input = req
	response.PromptParsing = parsedPrompt
	response.HardRules.Matched = nonEmpty(decision.MatchedRules)
	response.HardRules.Decision, response.HardRules.Capabilities = primary, capabilities(decision.RequiredCapabilities, req)
	response.DecisionTrace.MatchedRules = response.HardRules.Matched
	response.DecisionTrace.MatchedKeywords = baselineKeywords
	response.DecisionTrace.PrimaryPool, response.DecisionTrace.SecondaryPool = primary, secondary
	response.DecisionTrace.DetectedIntents = understanding.Intents
	response.DecisionTrace.RequiredCapabilities = understanding.RequiredCapabilities
	response.DecisionTrace.PoolTopScores, response.DecisionTrace.Top1Top2Margin = topK(scores, 3), decision.ScoreMargin
	response.DecisionTrace.Confidence, response.DecisionTrace.DecisionSource, response.DecisionTrace.FallbackReason = decision.Confidence, string(decision.DecisionSource), decision.FallbackReason
	response.DecisionTrace.TaskUnderstanding = understanding
	response.DecisionTrace.UnderstandingConflict = understanding.UnderstandingConflict
	tierReason := tier.Reason
	if complexity.RequestedTier == semanticrouter.TierStrong {
		tierReason = "multi-stage professional task: " + tierReason
	}
	response.DecisionTrace.TierDecision = tierDecision{RequestedTier: string(requestedTier), SelectedTier: string(finalTier), MatchedRule: tier.MatchedRule, Reason: tierReason, Confidence: tier.Confidence, PromptLength: len(req.Prompt), StepCount: complexity.StepCount, ConstraintCount: complexity.ConstraintCount, MultiIntentCount: complexity.MultiIntentCount, ComplexityScore: complexity.Score}

	// Get boundary hybrid decision
	boundaryDecision, err := s.boundaryHybrid.Route(context.Background(), "", semanticrouter.TaskTypeCode, routingPrompt)
	if err == nil && boundaryDecision != nil {
		// Tier Uncertainty Assessment
		response.DecisionTrace.Boundary = boundaryInfo{
			RawComplexityScore: boundaryDecision.ComplexityScore,
			UncertaintyScore:   boundaryDecision.TierUncertainty.UncertaintyScore,
			BoundaryEligible:   boundaryDecision.TierUncertainty.BoundaryEligible,
			Reasons:            boundaryDecision.TierUncertainty.Reasons,
			NearestBoundary:    boundaryDecision.TierUncertainty.NearestBoundary,
			DistanceToBoundary: boundaryDecision.TierUncertainty.DistanceToBoundary,
			// Legacy compatibility
			Eligible:      boundaryDecision.Boundary.Eligible,
			ReasonsLegacy: boundaryDecision.Boundary.Reasons,
		}

		// Hybrid Shadow Tier
		response.HybridShadow = hybridShadowInfo{
			// Raw Rule Tier → Policy Floor → Final Rule Tier
			RawRuleTier:       string(boundaryDecision.RawTier),
			MinimumTier:       string(boundaryDecision.MinimumTier),
			MinimumTierReason: boundaryDecision.MinimumTierReason,
			PolicyAdjusted:    boundaryDecision.PolicyAdjusted,
			RuleTier:          string(boundaryDecision.RuleTier),

			// Tier Uncertainty
			UncertaintyScore: boundaryDecision.TierUncertainty.UncertaintyScore,
			BoundaryEligible: boundaryDecision.TierUncertainty.BoundaryEligible,

			// RouteLLM invocation context
			InvokedForComparison: boundaryDecision.RouteLLMInvokedForComparison,
			InvokedForBoundary:   boundaryDecision.RouteLLMInvokedForBoundary,
			RouteLLMUsedByHybrid: boundaryDecision.RouteLLMUsedByHybrid,

			// Hybrid suggestion
			SuggestedTier:     string(boundaryDecision.HybridShadowTier),
			DecisionReason:    boundaryDecision.HybridDecisionReason,
			Disagreement:      boundaryDecision.HybridDisagreement,
			OverrideEligible:  boundaryDecision.OverrideEligible,
			OverrideReason:    boundaryDecision.OverrideReason,
			OverrideThreshold: boundaryDecision.OverrideThreshold,
			UsedForFinal:      boundaryDecision.HybridUsedForFinal, // 永远为 false
		}
	}

	// 三种 Provider 结果
	var e5Scores map[string]float64
	var officialVLLMShadow *vllm_pool_client.OfficialVLLMShadowResult
	if s.vllmAdapter != nil {
		shadowResult := s.vllmAdapter.Decide(context.Background(), routingPrompt, primary, string(requestedTier))
		officialVLLMShadow = shadowResult.OfficialVLLMShadow

		// Extract E5 scores for V2
		if shadowResult.LocalE5Shadow != nil {
			e5Scores = make(map[string]float64)
			for _, tk := range shadowResult.LocalE5Shadow.TopK {
				e5Scores[tk.Category] = tk.Score
			}
		}

		// 1. Local Pool Decision - 决定 final_pool
		response.LocalPoolDecision = struct {
			Pool         string `json:"pool"`
			Tier         string `json:"tier"`
			Source       string `json:"source"`
			UsedForFinal bool   `json:"used_for_final"`
		}{
			Pool:         shadowResult.LocalPoolDecision.Pool,
			Tier:         shadowResult.LocalPoolDecision.Tier,
			Source:       shadowResult.LocalPoolDecision.Source,
			UsedForFinal: shadowResult.LocalPoolDecision.UsedForFinal,
		}

		// 2. Local E5 Prototype Shadow
		if shadowResult.LocalE5Shadow != nil {
			var localE5TopK []struct {
				Category string  `json:"category"`
				Score    float64 `json:"score"`
			}
			for _, tk := range shadowResult.LocalE5Shadow.TopK {
				localE5TopK = append(localE5TopK, struct {
					Category string  `json:"category"`
					Score    float64 `json:"score"`
				}{Category: tk.Category, Score: tk.Score})
			}

			response.LocalE5Shadow = struct {
				Provider         string   `json:"provider"`
				ServiceReady     bool     `json:"service_ready"`
				MatchedSignals   []string `json:"matched_signals"`
				SemanticCategory string   `json:"semantic_category"`
				Confidence       float64  `json:"confidence"`
				TopK             []struct {
					Category string  `json:"category"`
					Score    float64 `json:"score"`
				} `json:"top_k"`
				LegacyMappedPool     string   `json:"legacy_mapped_pool"`
				ExecutionFamily      string   `json:"execution_family"`
				RequiredCapabilities []string `json:"required_capabilities"`
				LocalAgreement       bool     `json:"local_agreement"`
				LatencyMs            float64  `json:"latency_ms"`
				Error                string   `json:"error"`
				UsedForFinal         bool     `json:"used_for_final"`
			}{
				Provider:             shadowResult.LocalE5Shadow.Provider,
				ServiceReady:         shadowResult.LocalE5Shadow.ServiceReady,
				MatchedSignals:       shadowResult.LocalE5Shadow.MatchedSignals,
				SemanticCategory:     shadowResult.LocalE5Shadow.SemanticCategory,
				Confidence:           shadowResult.LocalE5Shadow.Confidence,
				TopK:                 localE5TopK,
				LegacyMappedPool:     shadowResult.LocalE5Shadow.LegacyMappedPool,
				ExecutionFamily:      shadowResult.LocalE5Shadow.ExecutionFamily,
				RequiredCapabilities: shadowResult.LocalE5Shadow.RequiredCapabilities,
				LocalAgreement:       shadowResult.LocalE5Shadow.LocalAgreement,
				LatencyMs:            shadowResult.LocalE5Shadow.LatencyMs,
				Error:                shadowResult.LocalE5Shadow.Error,
				UsedForFinal:         shadowResult.LocalE5Shadow.UsedForFinal,
			}
		}

		// 3. Official vLLM Semantic Router Shadow
		if shadowResult.OfficialVLLMShadow != nil {
			var officialTopK []struct {
				Category string  `json:"category"`
				Score    float64 `json:"score"`
			}
			for _, tk := range shadowResult.OfficialVLLMShadow.TopK {
				officialTopK = append(officialTopK, struct {
					Category string  `json:"category"`
					Score    float64 `json:"score"`
				}{Category: tk.Category, Score: tk.Score})
			}

			response.OfficialVLLMShadow = struct {
				Provider         string                 `json:"provider"`
				APIAvailable     bool                   `json:"api_available"`
				ReadyEndpointOK  bool                   `json:"ready_endpoint_ok"`
				ReadyFlag        bool                   `json:"ready_flag"`
				ReadyModels      int                    `json:"ready_models"`
				TotalModels      int                    `json:"total_models"`
				ClassifierReady  bool                   `json:"classifier_ready"`
				EvaluationStatus string                 `json:"evaluation_status"`
				MatchedSignals   map[string]float64     `json:"matched_signals"`
				MatchedDecision  string                 `json:"matched_decision"`
				RoutingDecision  string                 `json:"routing_decision"`
				RawTrace         map[string]interface{} `json:"raw_trace"`
				SemanticCategory string                 `json:"semantic_category"`
				Confidence       float64                `json:"confidence"`
				TopK             []struct {
					Category string  `json:"category"`
					Score    float64 `json:"score"`
				} `json:"top_k"`
				LegacyMappedPool     string             `json:"legacy_mapped_pool"`
				ExecutionFamily      string             `json:"execution_family"`
				RequiredCapabilities []string           `json:"required_capabilities"`
				SignalValues         map[string]float64 `json:"signal_values"`
				TopSignal            string             `json:"top_signal"`
				TopScore             float64            `json:"top_score"`
				EmbeddingConfidence  float64            `json:"embedding_confidence"`
				EmbeddingTimeMs      float64            `json:"embedding_time_ms"`
				OperationalForShadow bool               `json:"operational_for_shadow"`
				LastDecision         string             `json:"last_decision"`
				LocalAgreement       bool               `json:"local_agreement"`
				LatencyMs            float64            `json:"latency_ms"`
				Error                string             `json:"error"`
				UsedForFinal         bool               `json:"used_for_final"`
			}{
				Provider:             shadowResult.OfficialVLLMShadow.Provider,
				APIAvailable:         shadowResult.OfficialVLLMShadow.APIAvailable,
				ReadyEndpointOK:      shadowResult.OfficialVLLMShadow.ReadyEndpointOK,
				ReadyFlag:            shadowResult.OfficialVLLMShadow.ReadyFlag,
				ReadyModels:          shadowResult.OfficialVLLMShadow.ReadyModels,
				TotalModels:          shadowResult.OfficialVLLMShadow.TotalModels,
				ClassifierReady:      shadowResult.OfficialVLLMShadow.ClassifierReady,
				EvaluationStatus:     shadowResult.OfficialVLLMShadow.EvaluationStatus,
				MatchedSignals:       shadowResult.OfficialVLLMShadow.MatchedSignals,
				MatchedDecision:      shadowResult.OfficialVLLMShadow.MatchedDecision,
				RoutingDecision:      shadowResult.OfficialVLLMShadow.RoutingDecision,
				RawTrace:             shadowResult.OfficialVLLMShadow.RawTrace,
				SemanticCategory:     shadowResult.OfficialVLLMShadow.SemanticCategory,
				Confidence:           shadowResult.OfficialVLLMShadow.Confidence,
				TopK:                 officialTopK,
				LegacyMappedPool:     shadowResult.OfficialVLLMShadow.LegacyMappedPool,
				ExecutionFamily:      shadowResult.OfficialVLLMShadow.ExecutionFamily,
				RequiredCapabilities: shadowResult.OfficialVLLMShadow.RequiredCapabilities,
				SignalValues:         shadowResult.OfficialVLLMShadow.SignalValues,
				TopSignal:            shadowResult.OfficialVLLMShadow.TopSignal,
				TopScore:             shadowResult.OfficialVLLMShadow.TopScore,
				EmbeddingConfidence:  shadowResult.OfficialVLLMShadow.EmbeddingConfidence,
				EmbeddingTimeMs:      shadowResult.OfficialVLLMShadow.EmbeddingTimeMs,
				OperationalForShadow: shadowResult.OfficialVLLMShadow.OperationalForShadow,
				LastDecision:         shadowResult.OfficialVLLMShadow.LastDecision,
				LocalAgreement:       shadowResult.OfficialVLLMShadow.LocalAgreement,
				LatencyMs:            shadowResult.OfficialVLLMShadow.LatencyMs,
				Error:                shadowResult.OfficialVLLMShadow.Error,
				UsedForFinal:         shadowResult.OfficialVLLMShadow.UsedForFinal,
			}
		}

		// === Fill new layered routing cards ===

		// Task Understanding Card
		intents := understanding.Intents
		if len(intents) == 0 {
			intents = []string{understanding.PrimaryIntent}
		}
		response.TaskUnderstandingCard = intentMapping{
			PrimaryIntent:    understanding.PrimaryIntent,
			SecondaryIntents: understanding.SecondaryIntents,
			Intents:          intents,
		}

		// Local Provider Result
		localDecision := ""
		localCategory := ""
		localPool := primary
		localSignals := map[string]float64{}
		if shadowResult.LocalE5Shadow != nil {
			localDecision = shadowResult.LocalE5Shadow.SemanticCategory
			localCategory = shadowResult.LocalE5Shadow.SemanticCategory
			localPool = shadowResult.LocalE5Shadow.LegacyMappedPool
			if localPool == "" {
				localPool = poolCategoryToPool(shadowResult.LocalE5Shadow.SemanticCategory)
			}
			if shadowResult.LocalE5Shadow.MatchedSignals != nil {
				for _, s := range shadowResult.LocalE5Shadow.MatchedSignals {
					localSignals["keyword:"+s] = 1.0
				}
			}
		}
		if localDecision == "" {
			localDecision = primary
			localCategory = intentForPool(preferredPool(primary))
		}
		// Determine the correct provider name based on execution path
		localProviderName := "local_rule_baseline"
		if embedding.Invoked {
			localProviderName = "local_e5_prototype"
		} else if req.Mode == "full_embedding" || req.Mode == "compare" || req.Mode == "selective" {
			localProviderName = "local_rule_baseline_embedding_unavailable"
		}
		response.LocalProviderResult = providerResultSummary{
			Provider:         localProviderName,
			SemanticCategory: localCategory,
			DecisionName:     localDecision,
			MatchedSignals:   localSignals,
			Confidence:       decision.Confidence,
			MappedPool:       normalizePoolName(localPool),
			UsedForFinal:     true, // Local Pool decides final
			LatencyMs:        0,
			Error:            "",
		}

		// Official Provider Result
		officialCategory := ""
		officialDecision := ""
		officialPool := ""
		officialSignals := map[string]float64{}
		if shadowResult.OfficialVLLMShadow != nil {
			officialCategory = shadowResult.OfficialVLLMShadow.SemanticCategory
			officialDecision = shadowResult.OfficialVLLMShadow.MatchedDecision
			officialPool = shadowResult.OfficialVLLMShadow.LegacyMappedPool
			if officialPool == "" {
				officialPool = poolCategoryToPool(shadowResult.OfficialVLLMShadow.SemanticCategory)
			}
			if shadowResult.OfficialVLLMShadow.MatchedSignals != nil {
				officialSignals = shadowResult.OfficialVLLMShadow.MatchedSignals
			}
		}
		response.OfficialProviderResult = providerResultSummary{
			Provider:         "official_vllm_sr",
			SemanticCategory: officialCategory,
			DecisionName:     officialDecision,
			MatchedSignals:   officialSignals,
			Confidence:       officialResultConfidence(shadowResult.OfficialVLLMShadow),
			MappedPool:       normalizePoolName(officialPool),
			UsedForFinal:     false,
			LatencyMs:        shadowLatency(shadowResult.OfficialVLLMShadow),
			Error:            shadowError(shadowResult.OfficialVLLMShadow),
		}

		// Pool Card - use the final local pool (MappedPool) for the physical model group
		// to ensure consistency with agreement calculations.
		finalPoolName := normalizePoolName(primary)
		finalGroup := poolToPhysicalGroup[finalPoolName]
		if response.LocalProviderResult.MappedPool != "" {
			mappedGroup := poolToPhysicalGroup[response.LocalProviderResult.MappedPool]
			if mappedGroup != "" {
				finalGroup = mappedGroup
				finalPoolName = response.LocalProviderResult.MappedPool
			}
		}
		response.PoolCard = poolMapping{
			LogicalPool:        finalPoolName,
			PhysicalModelGroup: finalGroup,
		}

		// Tier Card with constraint validation
		minTier := string(tier.PreferredTier)
		tierReason := tier.Reason
		// Enforce requested_tier >= minimum_tier and selected_tier >= minimum_tier
		requestedTierStr := string(requestedTier)
		selectedTierStr := string(finalTier)
		if tierRank(requestedTierStr) < tierRank(minTier) {
			requestedTierStr = minTier
		}
		if tierRank(selectedTierStr) < tierRank(minTier) {
			selectedTierStr = minTier
		}
		response.TierCard = tierSummary{
			ComplexityScore:   complexity.Score,
			RawTier:           string(tier.PreferredTier),
			MinimumTier:       minTier,
			MinimumTierReason: tierReason,
			RequestedTier:     requestedTierStr,
			SelectedTier:      selectedTierStr,
			TierSource:        string(decision.DecisionSource),
		}

		// Scheduler Card - use normalized pool names
		schedulerPool := normalizePoolName(poolName(finalPool))
		schedulerGroup := poolToPhysicalGroup[schedulerPool]
		if schedulerGroup == "" {
			schedulerGroup = "general_chat_models"
		}
		response.SchedulerCard = schedulerSummary{
			RequestedPool:         schedulerPool,
			RequestedTier:         string(finalTier),
			PhysicalModelGroup:    schedulerGroup,
			SelectedModel:         modelName,
			SelectedAccountID:     scheduled.SelectedAccountID,
			SchedulerSource:       "mock",
			DryRun:                true,
			UpstreamCalled:        false,
			CandidateCount:        scheduled.CandidateCount,
			CandidateModels:       scheduled.CandidateModels,
			CandidateDetails:      scheduled.CandidateDetails,
			RecommendedModel:      recommendedModel,
			RecommendationReason:  recommendationReason,
			OldSelectedAccountID:  scheduled.SelectedAccountID,
			NewSuggestedAccountID: recommendedAccountID,
			OldSelectedModel:      modelName,
			NewSuggestedModel:     recommendedModel,
			OldVsNewAgreement:     modelName != "" && modelName == recommendedModel,
			RankingMargin:         rankingMargin,
			DecisionSource:        scheduled.DecisionSource,
		}

		// Agreement calculations
		response.SemanticAgreement = localCategory == officialCategory
		response.PoolAgreement = localPool == officialPool

		// Physical group agreement:
		// - Both sides must have a known mapping
		// - If official pool cannot be mapped (e.g. default-route), fall back to agreed (no evidence of disagreement)
		// - If both are mapped, compare them
		localPhys := poolToPhysicalGroup[localPool]
		officialPhys := poolToPhysicalGroup[officialPool]
		if localPhys == "" {
			localPhys = "general_chat_models"
		}
		if officialPhys == "" {
			// Unmapped official pool → no evidence of disagreement, treat as agreed
			officialPhys = localPhys
		}
		response.PhysicalGroupAgreement = localPhys == officialPhys
		response.OfficialPhysicalGroup = officialPhys

		// RouteLLM Tier agreement: compare rule tier vs routellm suggested tier
		response.RouteLLMAgreement = false
		if routeLLMDecision.RouteLLMTier != nil {
			ruleTier := string(routeLLMDecision.FinalTier)
			rllmTier := string(*routeLLMDecision.RouteLLMTier)
			response.RouteLLMAgreement = ruleTier != "" && rllmTier != "" && ruleTier == rllmTier
		}

		// Hybrid Pool Shadow (observational, never used for final)
		officialErrStr := ""
		if shadowResult.OfficialVLLMShadow != nil {
			officialErrStr = shadowResult.OfficialVLLMShadow.Error
		}
		officialConf := officialResultConfidence(shadowResult.OfficialVLLMShadow)
		response.HybridPool = computeHybridPool(normalizePoolName(localPool), normalizePoolName(officialPool), decision.Confidence, officialConf, officialErrStr, localCategory, defaultHybridConfig)
		response.GroupFirstHybrid = computeGroupFirstHybrid(decision.SemanticScores, shadowResult.OfficialVLLMShadow, poolName(finalPool))
		if !response.GroupFirstHybrid.Abstain && response.GroupFirstHybrid.SelectedGroup != "" && response.GroupFirstHybrid.SuggestedPool != "" {
			hybridScheduled := s.scheduler.Select(&semanticrouter.SchedulerSelectRequest{
				PreferredGroup:       response.GroupFirstHybrid.SelectedGroup,
				ModelGroup:           s.modelGroup,
				PreferredPool:        preferredPool(response.GroupFirstHybrid.SuggestedPool),
				PreferredTier:        finalTier,
				TaskType:             decision.TaskType,
				TaskSignals:          modelTaskSignals(routingPrompt, understanding.RequiredCapabilities),
				RequiredCapabilities: decision.RequiredCapabilities,
			})
			if hybridScheduled != nil && hybridScheduled.Error == nil {
				response.GroupFirstHybrid.SuggestedAccountID = hybridScheduled.SelectedAccountID
				response.GroupFirstHybrid.SuggestedModel, _ = recommendedModelFromProfiles(hybridScheduled.SelectedModel, hybridScheduled.CandidateDetails)
			}
		}
	}

	response.Baseline, response.Embedding, response.Selective, response.RouteLLMTier, response.Scheduler = baseline, embedding, selective, routeLLMDecision, scheduler

	// V2 Decision (parallel shadow, never influences V1 final_pool)
	response.V2Decision = s.v2Router.RouteWithScores(&semanticrouter.RouteRequest{
		Prompt:      req.Prompt,
		HasImage:    req.HasImage,
		HasDocument: req.HasDocument,
		HasCSV:      req.HasCSV,
	}, e5Scores)
	if response.V2Decision != nil && officialVLLMShadow != nil {
		response.V2Decision.Official = enrichOfficialV2Scores(response.V2Decision.Official, officialVLLMShadow)
	}
	response.Debug.TakeoverDisabled, response.Debug.EmbeddingHealth = true, healthLabel(embedding)
	response.Debug.SchedulerSource, response.Debug.DryRun, response.Debug.UpstreamCalled = "mock", true, false
	response.Debug.ComparisonEmbeddingInvoked, response.Debug.SelectiveEligible = embedding.Invoked, eligible
	response.Debug.SelectiveEmbeddingInvoked, response.Debug.EmbeddingUsedForDecision, response.Debug.FinalDecisionSource = req.Mode == "selective" && embedding.Invoked, selective.Override, source
	response.Debug.Timestamp = time.Now().UTC().Format(time.RFC3339)
	response.Debug.UseNewArchitecture = s.useNewArchitecture

	// 新架构输出（如果启用）
	if s.useNewArchitecture {
		newArchResult := s.enhancedRouter.Route(&semanticrouter.RouteRequest{
			Prompt:      req.Prompt,
			HasImage:    req.HasImage,
			HasDocument: req.HasDocument,
			HasCSV:      req.HasCSV,
		})
		explanation := newArchResult.GetExplanatoryOutput()

		response.NewArchitecture = &newArchitectureOutput{
			Enabled:           true,
			TaskUnderstanding: convertTaskUnderstanding(explanation.TaskUnderstanding),
			CandidatePools:    convertCandidatePools(explanation.CandidatePools),
			RejectedPools:     convertRejectedPools(explanation.RejectedPools),
			FinalPool:         explanation.FinalPool,
			Confidence:        explanation.Confidence,
			DecisionSource:    explanation.DecisionSource,
			EmbeddingUsed:     explanation.EmbeddingUsed,
			FallbackInfo:      convertFallbackInfo(explanation.FallbackInfo),
		}
	}

	return response
}

// scheduleWithinModelGroup keeps an API-key group as a hard boundary. If the
// group has no model with the requested specialized capability, Playground
// still returns a dry-run recommendation from that exact group and labels it
// as a fallback rather than silently crossing into another group.
func (s *playgroundServer) scheduleWithinModelGroup(pool semanticrouter.PreferredPool, tier semanticrouter.PreferredTier, taskType semanticrouter.TaskType, signals []string, capabilities semanticrouter.RequiredCapabilities) (*semanticrouter.SchedulerSelectResult, string) {
	request := &semanticrouter.SchedulerSelectRequest{PreferredGroup: semanticrouter.PhysicalGroupForPool(pool), ModelGroup: s.modelGroup, PreferredPool: pool, PreferredTier: tier, TaskType: taskType, TaskSignals: signals, RequiredCapabilities: capabilities}
	result := s.scheduler.Select(request)
	if result == nil || result.Error == nil || s.modelGroup == "" {
		return result, ""
	}

	// A specialized pool is unavailable in this group. Never cross the API-key
	// boundary: reselect an eligible general model in the same group instead.
	fallback := s.scheduler.Select(&semanticrouter.SchedulerSelectRequest{PreferredGroup: semanticrouter.PhysicalGroupForPool(semanticrouter.PoolDefault), ModelGroup: s.modelGroup, PreferredPool: semanticrouter.PoolDefault, PreferredTier: semanticrouter.TierWeak, TaskType: taskType, TaskSignals: signals})
	if fallback != nil && fallback.Error == nil && fallback.SelectedModel != "" && fallback.SelectedAccountID != 0 {
		return fallback, "same_group_fallback_no_" + string(pool) + "_candidate"
	}
	return result, ""
}

func (s *playgroundServer) embedding(prompt string, requested bool) embeddingResult {
	result := embeddingResult{Scores: map[string]float64{}, InvocationReason: "not_requested"}
	if !requested {
		return result
	}
	start := time.Now()
	scorer := semanticrouter.NewRealEmbeddingScorer(s.embeddingEndpoint)
	scorer.SetTimeout(1200 * time.Millisecond)
	scorer.Enable()
	best, err := scorer.ScoreAllWithBest(prompt)
	result.HTTPLatency = elapsedMS(start)
	if err != nil {
		result.InvocationReason, result.Error = "service_unavailable_fallback_to_baseline", err.Error()
		return result
	}
	result.Invoked, result.ServiceAvailable, result.InvocationReason = true, true, "requested_by_mode"
	result.Scores, result.BestPool, result.BestScore = normalizeScores(best.Scores), displayPoolName(best.BestPool), best.BestScore
	result.SecondBestPool, result.SecondBestScore, result.ScoreMargin = displayPoolName(best.SecondBestPool), best.SecondBestScore, best.ScoreMargin
	result.RankedPools, result.ModelName = rankedScoreMap(result.Scores), "RealEmbeddingScorer"
	return result
}

func (s *playgroundServer) handleEmbeddingHealth(w http.ResponseWriter, r *http.Request) {
	resp, err := s.httpClient.Get(s.embeddingEndpoint + "/health")
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"status": "unavailable", "available": false, "fallback": "baseline", "error": err.Error()})
		return
	}
	defer resp.Body.Close()
	var body any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		body = map[string]any{"status": "invalid_response"}
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "available": resp.StatusCode == http.StatusOK, "embedding": body})
}

func (s *playgroundServer) handleRouteLLMHealth(w http.ResponseWriter, r *http.Request) {
	if !s.routeLLMConfig.Enabled {
		writeJSON(w, http.StatusOK, map[string]any{"status": "disabled", "enabled": false, "shadow_only": true, "upstream_called": false})
		return
	}
	if err := s.routeLLMClient.Health(r.Context()); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"status": "unavailable", "enabled": true, "shadow_only": true, "upstream_called": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "enabled": true, "shadow_only": true, "upstream_called": false})
}

// ============================================================================
// Record saving & retrieval handlers
// ============================================================================

// handleSaveResult saves the last evaluate result (called from Playground UI).
// It reads the result from the request body, which was already computed client-side.
// This MUST NOT re-run the router.
func (s *playgroundServer) handleSaveResult(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
		return
	}
	if s.store == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "store not available"})
		return
	}

	defer r.Body.Close()
	var body struct {
		Prompt      string `json:"prompt"`
		HasImage    bool   `json:"has_image"`
		HasDocument bool   `json:"has_document"`
		HasCSV      bool   `json:"has_csv"`
		RunID       string `json:"run_id"`
		RowID       string `json:"row_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	body.Prompt = strings.TrimSpace(body.Prompt)
	if body.Prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "prompt is required"})
		return
	}

	// Re-evaluate to get fresh routing result
	req := playgroundRequest{
		Prompt:      body.Prompt,
		HasImage:    body.HasImage,
		HasDocument: body.HasDocument,
		HasCSV:      body.HasCSV,
		Mode:        "baseline",
	}
	started := time.Now()
	resp := s.evaluate(req)
	latency := elapsedMS(started)

	runID := body.RunID
	if runID == "" {
		runID = fmt.Sprintf("single-%d", started.UnixNano())
	}
	rowID := body.RowID
	if rowID == "" {
		rowID = fmt.Sprintf("row-%d", started.UnixNano())
	}

	rec := BuildRoutingRecordFromResponse(req, resp, runID, rowID, latency, started)

	// Save individual run if not exists
	existingRun, _ := s.store.GetRecordByRowID(runID, rowID)
	if existingRun == nil {
		_ = s.store.CreateRun(&RunRecord{
			RunID:       runID,
			DatasetName: "single",
			SourceType:  "single",
			TotalRows:   1,
			Status:      "completed",
			CreatedAt:   started,
		})
	}

	recordID, err := s.store.InsertRecord(rec)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "save failed: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"saved":     true,
		"record_id": recordID,
		"run_id":    runID,
		"saved_at":  started.Format(time.RFC3339Nano),
	})
}

// handleRecordsList returns paginated, filtered routing records
func (s *playgroundServer) handleRecordsList(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "store not available"})
		return
	}

	q := r.URL.Query()
	filter := RecordFilter{
		RunID:        q.Get("run_id"),
		SourceType:   q.Get("source_type"),
		LocalPool:    q.Get("local_pool"),
		OfficialPool: q.Get("official_pool"),
		Tier:         q.Get("tier"),
		Limit:        parseInt(q.Get("limit"), 50),
		Offset:       parseInt(q.Get("offset"), 0),
	}

	if v := q.Get("pool_agreement"); v == "true" {
		b := true
		filter.PoolAgreement = &b
	} else if v == "false" {
		b := false
		filter.PoolAgreement = &b
	}
	if v := q.Get("has_error"); v == "true" {
		b := true
		filter.HasError = &b
	} else if v == "false" {
		b := false
		filter.HasError = &b
	}
	if v := q.Get("has_review"); v == "true" {
		b := true
		filter.HasReview = &b
	} else if v == "false" {
		b := false
		filter.HasReview = &b
	}

	records, total, err := s.store.ListRecords(filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Attach review labels if present
	type recordWithReview struct {
		RoutingRecord
		Review *ReviewLabel `json:"review,omitempty"`
	}
	results := make([]recordWithReview, len(records))
	for i, rec := range records {
		rwr := recordWithReview{RoutingRecord: rec}
		if review, err := s.store.GetReview(rec.ID); err == nil {
			rwr.Review = review
		}
		results[i] = rwr
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"records": results,
		"total":   total,
		"limit":   filter.Limit,
		"offset":  filter.Offset,
	})
}

// handleGetRecord returns a single record with full details
func (s *playgroundServer) handleGetRecord(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "store not available"})
		return
	}

	id := int64(parseInt(r.URL.Query().Get("id"), 0))
	if id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "valid id required"})
		return
	}

	rec, err := s.store.GetRecord(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "record not found"})
		return
	}

	// Attach review
	type recordWithReview struct {
		RoutingRecord
		Review *ReviewLabel `json:"review,omitempty"`
	}
	result := recordWithReview{RoutingRecord: *rec}
	if review, err := s.store.GetReview(rec.ID); err == nil {
		result.Review = review
	}

	writeJSON(w, http.StatusOK, result)
}

// handleSaveReview saves or updates a review label for a record
func (s *playgroundServer) handleSaveReview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
		return
	}
	if s.store == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "store not available"})
		return
	}

	defer r.Body.Close()
	var body struct {
		RecordID          int64  `json:"record_id"`
		ExpectedIntent    string `json:"expected_intent"`
		ExpectedPool      string `json:"expected_pool"`
		ExpectedTier      string `json:"expected_tier"`
		Ambiguous         bool   `json:"ambiguous"`
		ReviewConfidence  string `json:"review_confidence"`
		ReviewNote        string `json:"review_note"`
		Reviewer          string `json:"reviewer"`
		NeedsAdjudication bool   `json:"needs_adjudication"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if body.RecordID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "record_id required"})
		return
	}
	if body.ReviewConfidence == "" {
		body.ReviewConfidence = "medium"
	}
	validConfs := map[string]bool{"high": true, "medium": true, "low": true}
	if !validConfs[body.ReviewConfidence] {
		body.ReviewConfidence = "medium"
	}

	review := &ReviewLabel{
		RecordID:          body.RecordID,
		ExpectedIntent:    body.ExpectedIntent,
		ExpectedPool:      body.ExpectedPool,
		ExpectedTier:      body.ExpectedTier,
		Ambiguous:         body.Ambiguous,
		ReviewConfidence:  body.ReviewConfidence,
		ReviewNote:        body.ReviewNote,
		Reviewer:          body.Reviewer,
		NeedsAdjudication: body.NeedsAdjudication,
		LabelVersion:      "v1",
	}

	id, err := s.store.UpsertReview(review)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"saved":     true,
		"review_id": id,
	})
}

// handleExportRecords exports records in JSONL or CSV format
func (s *playgroundServer) handleExportRecords(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "store not available"})
		return
	}

	q := r.URL.Query()
	format := q.Get("format")
	if format != "csv" {
		format = "jsonl"
	}

	filter := ExportFilter{
		RunID:  q.Get("run_id"),
		Format: format,
		Limit:  parseInt(q.Get("limit"), 10000),
	}

	data, err := s.store.ExportRecords(filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=routing_records.csv")
	} else {
		w.Header().Set("Content-Type", "application/jsonl; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=routing_records.jsonl")
	}
	w.Write([]byte(data))
}

// handleRecordsStats returns aggregate statistics
func (s *playgroundServer) handleRecordsStats(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "store not available"})
		return
	}

	// Get recent records for stats
	records, total, err := s.store.ListRecords(RecordFilter{Limit: 10000})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	type stats struct {
		TotalRecords      int            `json:"total_records"`
		PoolDistribution  map[string]int `json:"pool_distribution"`
		TierDistribution  map[string]int `json:"tier_distribution"`
		AgreementRate     float64        `json:"agreement_rate"`
		PoolAgreementRate float64        `json:"pool_agreement_rate"`
		ReviewCount       int            `json:"review_count"`
		ReviewedRate      float64        `json:"reviewed_rate"`
	}

	statsData := stats{
		TotalRecords:     total,
		PoolDistribution: map[string]int{},
		TierDistribution: map[string]int{},
	}

	agreeCount := 0
	poolAgreeCount := 0
	for _, rec := range records {
		if rec.LocalPool != "" {
			statsData.PoolDistribution[rec.LocalPool]++
		}
		if rec.SelectedTier != "" {
			statsData.TierDistribution[rec.SelectedTier]++
		}
		if rec.SemanticAgreement {
			agreeCount++
		}
		if rec.PoolAgreement {
			poolAgreeCount++
		}
		if _, err := s.store.GetReview(rec.ID); err == nil {
			statsData.ReviewCount++
		}
	}

	if len(records) > 0 {
		statsData.AgreementRate = float64(agreeCount) / float64(len(records))
		statsData.PoolAgreementRate = float64(poolAgreeCount) / float64(len(records))
	}
	statsData.ReviewedRate = float64(statsData.ReviewCount) / float64(total)

	writeJSON(w, http.StatusOK, statsData)
}

func parseInt(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return defaultVal
	}
	return n
}

func (s *playgroundServer) handleHTML(w http.ResponseWriter, r *http.Request) {
	data, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "index.html not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Write(data)
}

func understand(req playgroundRequest, d *semanticrouter.MultiLayerDecision, scores []poolScore, primary, secondary string, baselineKeywords []string) taskUnderstanding {
	p := strings.ToLower(req.Prompt)

	// Check for explicit image references in prompt
	hasImageRef := containsAnyText(p, "image", "photo", "screenshot", "图片", "截图", "照片", "识图", "ocr", "this image", "this picture", "existing image")

	// Check if vision should be triggered: has_image=true OR explicit image reference
	hasVisionContext := req.HasImage || hasImageRef

	professionalData := isProfessionalDataRequest(p)
	codeDataRequest := isCodeDataRequest(p)
	apiRequest := isAPIRequest(p)

	// Classify actions
	actions := classifyActions(p, professionalData && !codeDataRequest)
	objects, outputs := []string{}, []string{}

	if professionalData && !codeDataRequest {
		objects = append(objects, "kaggle_competition", "wellbore_geology_prediction", "ml_pipeline")
		outputs = append(outputs, "modeling_strategy", "experiment_roadmap")
	}

	// Handle vision context properly
	if hasVisionContext {
		objects = append(objects, "existing_image")
	}

	if req.HasDocument {
		objects = append(objects, "document")
	}
	if req.HasCSV {
		objects = append(objects, "structured_data")
	}

	// Determine intent - but override if vision context doesn't match
	intent := intentForPool(d.PreferredPool)

	// If baseline says vision but no image context, this is likely an error
	if intent == "image_understanding" && !hasVisionContext {
		// Check if it's actually a code generation request
		if codeDataRequest || apiRequest {
			intent = "code_generation"
		} else {
			intent = "general_chat"
		}
	}

	primaryPool, secondaryPool := primary, secondary
	intents := []string{intent}
	requiredCapabilities := capabilities(d.RequiredCapabilities, req)

	if len(requiredCapabilities) == 0 && (len(baselineKeywords) > 0 || isProfessionalPool(primary)) {
		requiredCapabilities = capabilitiesForPool(primary)
	}

	if professionalData && !codeDataRequest {
		intent = "predictive_modeling_strategy"
		intents = []string{"competition_strategy", "predictive_modeling", "baseline_design", "validation_design", "model_ensemble", "post_processing", "hyperparameter_tuning"}
		primaryPool = "data_pool"
		secondaryPool = ""
		requiredCapabilities = []string{"data_science", "machine_learning", "validation_design", "ensemble_learning", "post_processing", "hyperparameter_optimization"}
	} else if codeDataRequest || apiRequest {
		// Better code request detection
		intent = "code_generation"
		intents = []string{"code_generation"}
		requiredCapabilities = []string{"code", "code_generation"}

		// Add specific intents based on the request type
		if apiRequest {
			intents = append(intents, "api_implementation", "backend_development")
			requiredCapabilities = append(requiredCapabilities, "api_design", "backend_development")
			objects = append(objects, "api_endpoint", "authentication")
			outputs = append(outputs, "api_implementation", "executable_code")
		}

		primaryPool = "code_pool"
		secondaryPool = ""
	}

	// Handle image generation (not understanding) - requires explicit generation intent
	if containsAnyText(p, "generate", "生成", "create", "design", "海报", "插画", "logo", "图片", "image") && !hasVisionContext {
		if !codeDataRequest && !apiRequest && !professionalData {
			intent = "image_generation"
			intents = []string{"image_generation", "creative_visual_generation"}
			primaryPool = "image_generation_pool"
			secondaryPool = ""
			requiredCapabilities = []string{"image_generation", "creative_design"}
			outputs = append(outputs, "creative_image")
		}
	}

	// Vision pool should only be for understanding existing images
	if hasVisionContext && !containsAnyText(p, "generate", "生成", "create", "design") {
		intent = "image_understanding"
		intents = []string{"image_understanding"}
		primaryPool = "vision_pool"
		secondaryPool = ""
		requiredCapabilities = []string{"vision", "image_understanding"}
		outputs = append(outputs, "image_analysis")
	}

	if intent == "code_generation" && len(outputs) == 0 {
		outputs = append(outputs, "executable_code")
	}

	// Intent suppression: professional intents should not be diluted with general chat
	// This is a structural rule, not a prompt-specific hack
	if isProfessionalPool(primaryPool) && len(intents) > 1 {
		// Remove general_chat/simple_chat from professional intents
		filtered := make([]string, 0, len(intents))
		for _, in := range intents {
			if in == "general_chat" || in == "simple_chat" {
				continue
			}
			filtered = append(filtered, in)
		}
		if len(filtered) > 0 {
			intents = filtered
			// Recalculate primary intent
			if len(intents) > 0 && intent == "general_chat" {
				intent = intents[0]
			}
		}
	}

	if intent == "data_analysis" {
		outputs = append(outputs, "data_visualization")
	}
	// Check if outputs already has creative_image
	hasCreativeImage := false
	for _, o := range outputs {
		if o == "creative_image" {
			hasCreativeImage = true
			break
		}
	}
	if intent == "image_generation" && !hasCreativeImage {
		outputs = append(outputs, "creative_image")
	}

	modalities := []string{"text"}
	if hasVisionContext {
		modalities = append(modalities, "image")
	}
	if req.HasDocument {
		modalities = append(modalities, "document")
	}
	if req.HasCSV {
		modalities = append(modalities, "csv")
	}

	// Don't add secondary intents for professional requests
	if !professionalData && !codeDataRequest && !apiRequest && secondaryPool != "" {
		intents = append(intents, intentForPool(preferredPool(secondaryPool)))
	}
	intents = unique(intents)
	conflict := isProfessionalPool(primary) && (intent == "general_chat" || (intent == "image_understanding" && !hasVisionContext))
	confidence := d.Confidence
	if conflict {
		intent = intentForPool(d.PreferredPool)
		intents = []string{intent}
		confidence = math.Min(confidence, 0.35)
	}
	return taskUnderstanding{Actions: unique(actions), Objects: unique(objects), Modalities: modalities, OutputArtifacts: unique(outputs), Intents: intents, PrimaryIntent: intent, SecondaryIntents: dropFirst(intents), RequiredCapabilities: requiredCapabilities, Confidence: confidence, Ambiguous: d.ScoreMargin < 0.15 || conflict, PoolScores: scores, PrimaryPool: primaryPool, SecondaryPool: secondaryPool, UnderstandingConflict: conflict}
}

func isProfessionalDataRequest(prompt string) bool {
	competition := containsAnyText(prompt, "kaggle", "竞赛", "比赛", "奖牌", "leaderboard")
	stages := countStageSignals(prompt)
	return (competition && (stages >= 2 || containsAnyText(prompt, "预测", "prediction", "建模", "modeling"))) ||
		(containsAnyText(prompt, "baseline") && containsAnyText(prompt, "验证", "validation"))
}

func isCodeDataRequest(prompt string) bool {
	return containsAnyText(prompt, "notebook") ||
		(containsAnyText(prompt, "代码", "code") && containsAnyText(prompt, "csv", "数据", "data", "预测", "model"))
}

func isAPIRequest(prompt string) bool {
	// Detect API/implementation requests
	return containsAnyText(prompt, "api", "endpoint", "接口", "login", "register", "认证", "authentication", "rest", "crud", "service", "微服务")
}

func classifyActions(prompt string, professionalData bool) []string {
	if professionalData {
		return []string{"plan", "design", "analyze", "optimize"}
	}
	actions := []string{}
	for _, candidate := range []struct {
		words  []string
		action string
	}{
		{[]string{"plan", "规划", "路线", "思路"}, "plan"},
		{[]string{"design", "设计", "方案"}, "design"},
		{[]string{"analyze", "分析"}, "analyze"},
		{[]string{"optimize", "优化", "调参"}, "optimize"},
		{[]string{"解释", "介绍", "explain", "介绍一下"}, "explain"},
		{[]string{"write", "build", "implement", "provide", "写", "实现", "开发", "给出", "提供"}, "write"},
		// Transform is deliberately narrow: planning/design must not match it.
		{[]string{"转换", "转成", "convert", "transform", "格式转换", "改写"}, "transform"},
		{[]string{"generate", "生成"}, "generate"},
	} {
		if containsAnyText(prompt, candidate.words...) {
			actions = append(actions, candidate.action)
		}
	}
	return actions
}

// ============================================================================
// analyzeTaskComplexity 分析任务复杂度
//
// 注意: 为与 RouteLLM 边界检测保持一致，复杂度分数已归一化到 0~1 范围
// 量纲说明:
//   - 原始分数范围: ~0~10+ (加分制)
//   - 归一化后: 0~1 (除以 MaxComplexityScore)
//   - Tier 判断: <0.15 = Weak, 0.15~0.50 = Medium, >=0.50 = Strong
//
// ============================================================================
func analyzeTaskComplexity(prompt string, understanding taskUnderstanding) taskComplexity {
	stages := countStageSignals(strings.ToLower(prompt))
	constraints := countConstraintSignals(strings.ToLower(prompt))
	multiIntent := stages
	if len(understanding.Intents) > 1 {
		multiIntent += len(understanding.Intents) - 1
	}

	// 原始复杂度分数计算 (0~10+)
	rawScore := float64(stages) + float64(multiIntent)*0.6 + float64(constraints)*0.3
	if understanding.PrimaryIntent == "predictive_modeling_strategy" {
		rawScore += 3.0
	}

	// 归一化到 0~1 范围，与 RouteLLM 边界保持一致
	const MaxComplexityScore = 10.0
	normalizedScore := math.Min(rawScore/MaxComplexityScore, 1.0)

	// Tier 判断 (基于归一化后的分数)
	// 统一量纲后: 0~1 范围
	// - < 0.15 → Weak
	// - 0.15 ~ 0.50 → Medium
	// - ≥ 0.50 → Strong
	var requested semanticrouter.PreferredTier
	if normalizedScore >= 0.50 {
		requested = semanticrouter.TierStrong
	} else if normalizedScore >= 0.15 {
		requested = semanticrouter.TierMedium
	} else {
		requested = semanticrouter.TierWeak
	}

	return taskComplexity{
		StepCount:        stages,
		ConstraintCount:  constraints,
		MultiIntentCount: multiIntent,
		Score:            normalizedScore, // 已归一化到 0~1
		RequestedTier:    requested,
	}
}

func countStageSignals(prompt string) int {
	stages := []string{"baseline", "验证", "validation", "融合", "ensemble", "后处理", "post-processing", "调参", "hyperparameter", "完整思路", "完整路线", "roadmap"}
	count := 0
	for _, stage := range stages {
		if strings.Contains(prompt, stage) {
			count++
		}
	}
	return count
}

func countConstraintSignals(prompt string) int {
	constraints := []string{"安全", "security", "高并发", "high concurrency", "百万", "million", "容灾", "disaster recovery", "性能", "performance", "优化", "optimize", "debug", "调试", "error", "bug", "fix"}
	count := 0
	for _, c := range constraints {
		if strings.Contains(prompt, c) {
			count++
		}
	}
	return count
}

// modelTaskSignals is intentionally a ranking hint, not a routing decision.
// The scheduler remains dry-run/shadow-only and still enforces the API-key
// model group, pool, tier, and capability eligibility before it ranks models.
func modelTaskSignals(prompt string, capabilities []string) []string {
	signals := append([]string(nil), capabilities...)
	lowerPrompt := strings.ToLower(prompt)
	if strings.ContainsAny(prompt, "\u4e00\u4e8c\u4e09\u56db\u4e94\u516d\u4e03\u516b\u4e5d") || containsHanRune(prompt) {
		signals = append(signals, "chinese")
	}
	if containsAnyText(lowerPrompt, "reason", "analysis", "architecture", "design", "\u63a8\u7406", "\u5206\u6790", "\u65b9\u6848", "\u8bbe\u8ba1") {
		signals = append(signals, "reasoning")
	}
	if len([]rune(prompt)) >= 2000 {
		signals = append(signals, "long_context")
	}
	return unique(signals)
}

func containsHanRune(value string) bool {
	for _, r := range value {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func containsAnyText(text string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(text, strings.ToLower(value)) {
			return true
		}
	}
	return false
}

func isProfessionalPool(pool string) bool {
	return pool == "code_pool" || pool == "data_pool" || pool == "document_pool" || pool == "vision_pool" || pool == "image_generation_pool"
}

func capabilitiesForPool(pool string) []string {
	switch pool {
	case "code_pool":
		return []string{"code"}
	case "data_pool":
		return []string{"data_science"}
	case "document_pool":
		return []string{"document"}
	case "vision_pool", "image_generation_pool":
		return []string{"vision"}
	default:
		return nil
	}
}

func rankedScores(scores map[string]float64) []poolScore {
	return rankedScoreMap(normalizeScores(scores))
}
func rankedScoreMap(scores map[string]float64) []poolScore {
	result := make([]poolScore, 0, len(scores))
	for pool, score := range scores {
		result = append(result, poolScore{Pool: displayPoolName(pool), Score: score})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Score == result[j].Score {
			return result[i].Pool < result[j].Pool
		}
		return result[i].Score > result[j].Score
	})
	return result
}
func normalizeScores(scores map[string]float64) map[string]float64 {
	result := make(map[string]float64, len(scores))
	for pool, score := range scores {
		result[displayPoolName(pool)] = score
	}
	return result
}
func topK(scores []poolScore, count int) []poolScore {
	if len(scores) < count {
		count = len(scores)
	}
	return append([]poolScore(nil), scores[:count]...)
}
func scoreFor(scores map[string]float64, pool semanticrouter.PreferredPool) float64 {
	return scores[string(pool)]
}
func poolName(pool semanticrouter.PreferredPool) string { return displayPoolName(string(pool)) }
func displayPoolName(pool string) string {
	switch pool {
	case "code":
		return "code_pool"
	case "data":
		return "data_pool"
	case "vision":
		return "vision_pool"
	case "document":
		return "document_pool"
	case "image_generation":
		return "image_generation_pool"
	case "cheap":
		return "cheap_chat_pool"
	case "default":
		return "general_pool"
	default:
		return pool
	}
}
func preferredPool(pool string) semanticrouter.PreferredPool {
	switch pool {
	case "code_pool":
		return semanticrouter.PoolCode
	case "data_pool":
		return semanticrouter.PoolData
	case "vision_pool":
		return semanticrouter.PoolVision
	case "document_pool":
		return semanticrouter.PoolDocument
	case "image_generation_pool":
		return semanticrouter.PoolImageGeneration
	case "cheap_chat_pool":
		return semanticrouter.PoolCheap
	default:
		return semanticrouter.PoolDefault
	}
}
func tierForPool(pool semanticrouter.PreferredPool, fallback semanticrouter.PreferredTier) semanticrouter.PreferredTier {
	if pool == semanticrouter.PoolVision {
		return semanticrouter.TierStrong
	}
	if pool == semanticrouter.PoolCode || pool == semanticrouter.PoolData || pool == semanticrouter.PoolDocument || pool == semanticrouter.PoolImageGeneration {
		return semanticrouter.TierMedium
	}
	return fallback
}

func recommendedModelFromProfiles(selected string, candidates []semanticrouter.ModelCandidateScore) (string, string) {
	best := selected
	bestScore := 0.0
	reason := "scheduler_fallback_selection"
	for _, candidate := range candidates {
		score := candidate.FinalScore
		if score <= 0 {
			score = candidate.ProfileScore
		}
		if candidate.ModelID == "" || score <= bestScore {
			continue
		}
		best = candidate.ModelID
		bestScore = score
		if candidate.FinalScore > 0 {
			reason = fmt.Sprintf("highest_%s_final_score=%.4f", candidate.RankingVersion, candidate.FinalScore)
		} else {
			reason = fmt.Sprintf("highest_%s_profile_score=%.4f", candidate.ProfileSource, candidate.ProfileScore)
		}
	}
	if best == selected && len(candidates) > 0 && candidates[0].Reason != "" {
		reason = candidates[0].Reason
	}
	return best, reason
}

func recommendedAccountAndMargin(candidates []semanticrouter.ModelCandidateScore) (int64, float64) {
	best, second := 0.0, 0.0
	var accountID int64
	for _, candidate := range candidates {
		score := candidate.FinalScore
		if score <= 0 {
			score = candidate.ProfileScore
		}
		if score > best {
			second, best = best, score
			accountID = candidate.AccountID
		} else if score > second {
			second = score
		}
	}
	if best == 0 {
		return 0, 0
	}
	return accountID, best - second
}

func buildPlaygroundModelRanking(pool string, details []semanticrouter.ModelCandidateScore) *semanticrouter.ModelRankingResult {
	if len(details) == 0 {
		return nil
	}
	sorted := append([]semanticrouter.ModelCandidateScore(nil), details...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].FinalScore == sorted[j].FinalScore {
			return sorted[i].ModelID < sorted[j].ModelID
		}
		return sorted[i].FinalScore > sorted[j].FinalScore
	})
	ranking := &semanticrouter.ModelRankingResult{
		Version: "model_ranking_v1", PhysicalGroup: semanticrouter.PhysicalGroupForPool(preferredPool(pool)),
		Scoring:    semanticrouter.ModelRankingWeights{PoolWeight: 0.75, TaskFitWeight: 0.25, CapabilityWeight: 0.55, TierWeight: 0.20, RuntimeWeight: 0.25},
		ShadowOnly: true, UsedForFinal: false, Candidates: make([]semanticrouter.ModelRankingCandidate, 0, len(sorted)),
	}
	for i, candidate := range sorted {
		ranking.Candidates = append(ranking.Candidates, semanticrouter.ModelRankingCandidate{Rank: i + 1, AccountID: candidate.AccountID, Model: candidate.ModelID, FinalScore: candidate.FinalScore})
	}
	if len(ranking.Candidates) > 0 {
		ranking.RecommendedAccountID = ranking.Candidates[0].AccountID
		ranking.RecommendedModel = ranking.Candidates[0].Model
	}
	if len(ranking.Candidates) > 1 {
		ranking.RankingMargin = ranking.Candidates[0].FinalScore - ranking.Candidates[1].FinalScore
	}
	return ranking
}

func intentForPool(pool semanticrouter.PreferredPool) string {
	switch pool {
	case semanticrouter.PoolCode:
		return "code_generation"
	case semanticrouter.PoolData:
		return "data_analysis"
	case semanticrouter.PoolVision:
		return "image_understanding"
	case semanticrouter.PoolDocument:
		return "document_processing"
	case semanticrouter.PoolImageGeneration:
		return "image_generation"
	default:
		return "general_chat"
	}
}

func poolCategoryToPool(category string) string {
	switch category {
	case "code", "code_generation":
		return "code_pool"
	case "data", "data_analysis":
		return "data_pool"
	case "document", "document_processing":
		return "document_pool"
	case "vision", "image_understanding":
		return "vision_pool"
	case "image_generation":
		return "image_generation_pool"
	case "simple_chat":
		return "general_text_pool"
	case "general", "general_chat", "":
		return "general_text_pool"
	default:
		return "general_text_pool"
	}
}

// normalizePoolName converts legacy pool names to standard names
func normalizePoolName(pool string) string {
	switch pool {
	case "cheap_chat_pool", "general_pool", "default_pool", "default":
		return "general_text_pool"
	case "general_text_pool":
		return "general_text_pool"
	default:
		return pool
	}
}

// tierRank returns numeric rank for tier comparison. Higher number = stronger tier.
func tierRank(tier string) int {
	switch tier {
	case "weak":
		return 0
	case "medium":
		return 1
	case "strong":
		return 2
	default:
		return 0
	}
}

func capabilities(c semanticrouter.RequiredCapabilities, req playgroundRequest) []string {
	result := []string{}
	if c.VisionCapable || req.HasImage {
		result = append(result, "vision")
	}
	if c.DocumentCapable || req.HasDocument {
		result = append(result, "document")
	}
	if req.HasCSV {
		result = append(result, "data")
	}
	return result
}
func matchedKeywords(prompt string, pool semanticrouter.PreferredPool) []string {
	_, keywords := semanticrouter.NewTokenOverlapSimilarityRouter().CalculateKeywordScore(prompt, pool)
	return keywords
}
func healthLabel(e embeddingResult) string {
	if e.ServiceAvailable {
		return "available"
	}
	if e.Error != "" {
		return "unavailable_fallback"
	}
	return "not_checked"
}
func elapsedMS(t time.Time) float64 {
	return math.Round(float64(time.Since(t).Microseconds())/100.0) / 10
}
func validMode(mode string) bool {
	return mode == "baseline" || mode == "full_embedding" || mode == "selective" || mode == "compare"
}

func envBool(name string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func envPositiveInt(name string) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return 0
	}
	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil || parsed <= 0 {
		return 0
	}
	return parsed
}
func nonEmpty(values []string) []string {
	result := []string{}
	for _, value := range values {
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}
func unique(values []string) []string {
	seen, result := map[string]bool{}, []string{}
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
func dropFirst(values []string) []string {
	if len(values) < 2 {
		return []string{}
	}
	return values[1:]
}

// 新架构输出转换函数
func convertTaskUnderstanding(tu *semanticrouter.TaskUnderstandingOutput) *taskUnderstandingNew {
	if tu == nil {
		return nil
	}
	return &taskUnderstandingNew{
		PrimaryIntent:    tu.PrimaryIntent,
		SecondaryIntents: tu.SecondaryIntents,
		Actions:          tu.Actions,
		Objects:          tu.Objects,
		InputModalities:  tu.InputModalities,
		OutputArtifacts:  tu.OutputArtifacts,
		Confidence:       tu.Confidence,
		Ambiguous:        tu.Ambiguous,
		MissingInputs:    tu.MissingInputs,
	}
}

func convertCandidatePools(pools []semanticrouter.CandidatePoolOutput) []candidatePoolNew {
	result := make([]candidatePoolNew, len(pools))
	for i, p := range pools {
		result[i] = candidatePoolNew{
			Pool:      p.Pool,
			Score:     p.Score,
			Evidence:  p.Evidence,
			Validated: p.Validated,
		}
	}
	return result
}

func convertRejectedPools(pools []semanticrouter.RejectedPoolOutput) []rejectedPoolNew {
	result := make([]rejectedPoolNew, len(pools))
	for i, p := range pools {
		result[i] = rejectedPoolNew{
			Pool:            p.Pool,
			RejectionReason: p.RejectionReason,
		}
	}
	return result
}

func convertFallbackInfo(fi *semanticrouter.FallbackInfoOutput) *fallbackInfoNew {
	if fi == nil {
		return nil
	}
	return &fallbackInfoNew{
		FallbackPool:    fi.FallbackPool,
		FallbackReason:  fi.FallbackReason,
		ConflictDetails: fi.ConflictDetails,
	}
}

func officialResultConfidence(shadow *vllm_pool_client.OfficialVLLMShadowResult) float64 {
	if shadow == nil {
		return 0
	}
	if shadow.EmbeddingConfidence > 0 {
		return shadow.EmbeddingConfidence
	}
	return shadow.Confidence
}

// enrichOfficialV2Scores keeps the V2 card's local E5 scores intact while
// adding the real official /eval scores used by the Official vLLM card.
func enrichOfficialV2Scores(result semanticrouter.V2OfficialResult, shadow *vllm_pool_client.OfficialVLLMShadowResult) semanticrouter.V2OfficialResult {
	if shadow == nil || len(shadow.SignalValues) == 0 {
		return result
	}

	score := func(category string) float64 {
		candidates := []string{
			"embedding:semantic_" + category,
			"semantic_" + category,
			"semantic_" + category + ":best",
		}
		for _, key := range candidates {
			if value, ok := shadow.SignalValues[key]; ok {
				return value
			}
		}
		return 0
	}

	officialScores := map[string]float64{
		"code":                 score("code"),
		"data_analysis":        score("data_analysis"),
		"document":             score("document"),
		"vision_understanding": score("vision_understanding"),
		"image_generation":     score("image_generation"),
		"simple_chat":          score("simple_chat"),
		"general":              score("general"),
	}
	technical := math.Max(officialScores["code"], math.Max(officialScores["data_analysis"], officialScores["document"]))
	general := math.Max(officialScores["general"], officialScores["simple_chat"])

	result.OfficialScores = officialScores
	result.TechnicalScore = technical
	result.GeneralScore = general
	result.OfficialDecision = shadow.RoutingDecision
	result.ScoreSource = "official_vllm_eval"
	result.DomainScore = technical
	result.SecondDomain = semanticrouter.V2DomainGeneral
	if general > technical {
		result.Domain = semanticrouter.V2DomainGeneral
		result.DomainScore = general
		result.SecondDomain = semanticrouter.V2DomainTechnical
	} else {
		result.Domain = semanticrouter.V2DomainTechnical
	}
	result.DomainMargin = math.Abs(technical - general)
	return result
}

func shadowLatency(shadow *vllm_pool_client.OfficialVLLMShadowResult) float64 {
	if shadow == nil {
		return 0
	}
	return shadow.LatencyMs
}

func shadowError(shadow *vllm_pool_client.OfficialVLLMShadowResult) string {
	if shadow == nil {
		return "no_shadow_data"
	}
	return shadow.Error
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// defaultHybridConfig holds thresholds for hybrid pool shadow
var defaultHybridConfig = HybridConfig{
	LocalHighConfidence:         0.80,
	OfficialHighConfidence:      0.70,
	OfficialLowScore:            0.35,
	MinMargin:                   0.10,
	AbstainOnDoubleHighConflict: true,
}

// computeHybridPool computes the hybrid candidate pool (observational, never used for final).
func computeHybridPool(localPool, officialPool string, localConf, officialConf float64, officialErr string, localIntent string, cfg HybridConfig) HybridPoolInfo {
	// 0. Official error/invalid -> fallback to local
	if officialErr != "" {
		return HybridPoolInfo{CandidatePool: localPool, Source: "local_fallback", Confidence: localConf, Reason: "official_error: " + officialErr, Abstain: false, UsedForFinal: false}
	}

	// 1. Pool一致
	if localPool == officialPool {
		return HybridPoolInfo{CandidatePool: localPool, Source: "consensus", Confidence: localConf, Reason: "pool_consensus", Abstain: false, UsedForFinal: false}
	}

	// 2. 模态: image_generation intent
	if strings.Contains(localIntent, "image_generation") {
		return HybridPoolInfo{CandidatePool: "image_generation_pool", Source: "modality", Confidence: 0.9, Reason: "image_generation_intent", Abstain: false, UsedForFinal: false}
	}

	// 3. Local高置信, Official低分 -> 选Local
	if localConf >= cfg.LocalHighConfidence && officialConf < cfg.OfficialLowScore {
		return HybridPoolInfo{CandidatePool: localPool, Source: "local_high_confidence", Confidence: localConf, Reason: fmt.Sprintf("local_conf=%.2f official_conf=%.2f", localConf, officialConf), Abstain: false, UsedForFinal: false}
	}

	// 4. Official高置信, Local为general -> 选Official
	if officialConf >= cfg.OfficialHighConfidence && localPool == "general_text_pool" {
		return HybridPoolInfo{CandidatePool: officialPool, Source: "official_high_confidence", Confidence: officialConf, Reason: fmt.Sprintf("official_conf=%.2f local_is_general", officialConf), Abstain: false, UsedForFinal: false}
	}

	// 5. 双高置信冲突 -> abstain
	if cfg.AbstainOnDoubleHighConflict && localConf >= cfg.LocalHighConfidence && officialConf >= cfg.OfficialHighConfidence {
		return HybridPoolInfo{CandidatePool: "", Source: "conflict", Confidence: 0, Reason: fmt.Sprintf("both_high_conf local=%.2f official=%.2f", localConf, officialConf), Abstain: true, UsedForFinal: false}
	}

	// 6. 默认 -> 选Local
	return HybridPoolInfo{CandidatePool: localPool, Source: "local_default", Confidence: localConf, Reason: "default_to_local", Abstain: false, UsedForFinal: false}
}

func computeGroupFirstHybrid(localScores map[string]float64, official *vllm_pool_client.OfficialVLLMShadowResult, localPool string) GroupFirstHybridInfo {
	localPools := canonicalPoolScores(localScores)
	officialPools := officialPoolScores(official)
	localGroups := groupScores(localPools)
	officialGroups := groupScores(officialPools)
	localGroup, localScore := highestScore(localGroups)
	officialGroup, officialScore := highestScore(officialGroups)

	info := GroupFirstHybridInfo{
		LocalGroup: localGroup, OfficialGroup: officialGroup,
		LocalGroupScores: localGroups, OfficialGroupScores: officialGroups,
		FusedGroupScores: map[string]float64{}, Source: "group_first_shadow", UsedForFinal: false,
	}
	if official == nil || official.Error != "" || len(officialPools) == 0 {
		info.FusedGroupScores = localGroups
		info.SelectedGroup = localGroup
		info.SuggestedPool, _ = highestScoreInGroup(localPools, localGroup)
		if info.SuggestedPool == "" {
			info.SuggestedPool = canonicalPool(localPool)
		}
		info.FusedPoolScores = rankedScoreMap(localPools)
		info.Source, info.Reason = "local_fallback", "official_scores_unavailable"
		return info
	}

	for _, group := range []string{"technical_models", "general_chat_models", "vision_models", "image_models"} {
		info.FusedGroupScores[group] = 0.55*localGroups[group] + 0.45*officialGroups[group]
	}
	// High-confidence cross-group disagreement is recorded as abstention. The
	// normal local candidate remains intact, and no hybrid account is suggested.
	if localGroup != "" && officialGroup != "" && localGroup != officialGroup && localScore >= 0.70 && officialScore >= 0.70 {
		info.SelectedGroup, info.Abstain = localGroup, true
		info.Reason = fmt.Sprintf("cross_group_high_conflict local=%s:%.3f official=%s:%.3f", localGroup, localScore, officialGroup, officialScore)
		info.FusedPoolScores = rankedScoreMap(localPools)
		return info
	}
	// A weak-to-moderate Official score must not move the physical model group.
	// Cross-group promotion requires a clear, high-confidence lead; otherwise
	// Pool fusion continues only inside the local Group.
	if localGroup != "" && officialGroup != "" && localGroup != officialGroup && (officialScore < 0.75 || officialScore < localScore+0.10) {
		info.SelectedGroup = localGroup
		info.SuggestedPool, _ = highestScoreInGroup(localPools, localGroup)
		info.FusedPoolScores = rankedScoreMap(localPools)
		info.Source = "local_group_guard"
		info.Reason = fmt.Sprintf("official_cross_group_evidence_insufficient local=%s:%.3f official=%s:%.3f", localGroup, localScore, officialGroup, officialScore)
		return info
	}
	info.SelectedGroup, _ = highestScore(info.FusedGroupScores)
	fusedPools := map[string]float64{}
	for pool := range localPools {
		fusedPools[pool] = 0.55 * localPools[pool]
	}
	for pool, score := range officialPools {
		fusedPools[pool] += 0.45 * score
	}
	info.SuggestedPool, _ = highestScoreInGroup(fusedPools, info.SelectedGroup)
	info.FusedPoolScores = rankedScoreMap(fusedPools)
	info.Reason = "fused_pool_scores=0.55_local+0.45_official"
	return info
}

func canonicalPoolScores(scores map[string]float64) map[string]float64 {
	result := map[string]float64{}
	for pool, score := range scores {
		result[canonicalPool(pool)] = score
	}
	return result
}

func officialPoolScores(official *vllm_pool_client.OfficialVLLMShadowResult) map[string]float64 {
	result := map[string]float64{}
	if official == nil {
		return result
	}
	for _, item := range official.TopK {
		category := strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(item.Category), "embedding:"), "semantic_")
		pool := canonicalPool(poolCategoryToPool(category))
		if item.Score > result[pool] {
			result[pool] = item.Score
		}
	}
	if len(result) == 0 && official.LegacyMappedPool != "" {
		result[canonicalPool(official.LegacyMappedPool)] = officialResultConfidence(official)
	}
	return result
}

func canonicalPool(pool string) string {
	pool = displayPoolName(strings.ToLower(strings.TrimSpace(pool)))
	if pool == "general_text_pool" || pool == "default_pool" || pool == "default" {
		return "general_pool"
	}
	return pool
}

func groupScores(scores map[string]float64) map[string]float64 {
	result := map[string]float64{}
	for pool, score := range scores {
		group := poolToPhysicalGroup[canonicalPool(pool)]
		if score > result[group] {
			result[group] = score
		}
	}
	return result
}

func highestScore(scores map[string]float64) (string, float64) {
	bestName, bestScore := "", -1.0
	for name, score := range scores {
		if score > bestScore || (score == bestScore && (bestName == "" || name < bestName)) {
			bestName, bestScore = name, score
		}
	}
	return bestName, bestScore
}

func highestScoreInGroup(scores map[string]float64, group string) (string, float64) {
	filtered := map[string]float64{}
	for pool, score := range scores {
		if poolToPhysicalGroup[canonicalPool(pool)] == group {
			filtered[canonicalPool(pool)] = score
		}
	}
	return highestScore(filtered)
}
