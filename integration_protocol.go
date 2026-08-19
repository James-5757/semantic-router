package semanticrouter

import "fmt"

const (
	ModelSelectorProtocolVersion = "v1"
	MaxIntegrationFrameBytes     = 1 << 20
)

type ModelSelectionRequest struct {
	ProtocolVersion     string   `json:"protocol_version"`
	RequestID           string   `json:"request_id"`
	APIKeyID            string   `json:"api_key_id,omitempty"`
	GroupID             int64    `json:"group_id"`
	Prompt              string   `json:"prompt"`
	SystemPrompt        string   `json:"system_prompt,omitempty"`
	AgentContext        string   `json:"agent_context,omitempty"`
	ProjectContext      string   `json:"project_context,omitempty"`
	ToolContext         string   `json:"tool_context,omitempty"`
	ConversationHistory []string `json:"conversation_history,omitempty"`
	ModelIDs            []string `json:"model_ids,omitempty"`
	HasImage            bool     `json:"has_image,omitempty"`
	HasDocument         bool     `json:"has_document,omitempty"`
	HasCSV              bool     `json:"has_csv,omitempty"`
	RequestedTier       string   `json:"requested_tier,omitempty"`
	RequiresStreaming   bool     `json:"requires_streaming,omitempty"`
	RequiresToolCall    bool     `json:"requires_tool_call,omitempty"`
	PreviousResponseID  string   `json:"previous_response_id,omitempty"`
	SessionHash         string   `json:"session_hash,omitempty"`
}

type ModelSelectionResponse struct {
	ProtocolVersion   string                `json:"protocol_version"`
	RequestID         string                `json:"request_id"`
	Success           bool                  `json:"success"`
	SelectedAccountID int64                 `json:"selected_account_id"`
	SelectedModel     string                `json:"selected_model"`
	SelectedPool      string                `json:"selected_pool"`
	SelectedTier      string                `json:"selected_tier"`
	SchedulerLayer    string                `json:"scheduler_layer"`
	PreferredPool     string                `json:"preferred_pool"`
	PreferredTier     string                `json:"preferred_tier"`
	TaskType          string                `json:"task_type"`
	Confidence        float64               `json:"confidence"`
	MatchedRule       string                `json:"matched_rule,omitempty"`
	CandidateCount    int                   `json:"candidate_count"`
	CandidateModels   []string              `json:"candidate_models,omitempty"`
	CandidateDetails  []ModelCandidateScore `json:"candidate_details,omitempty"`
	// ModelRanking is a stable, consumer-facing view of candidate scoring.
	// CandidateDetails remains the full internal debug payload for compatibility.
	ModelRanking    *ModelRankingResult `json:"model_ranking,omitempty"`
	SchedulerSource string              `json:"scheduler_source"`
	DryRun          bool                `json:"dry_run"`
	UpstreamCalled  bool                `json:"upstream_called"`
	ShadowOnly      bool                `json:"shadow_only"`
	Error           string              `json:"error,omitempty"`
}

// ModelRankingResult is the shadow recommendation supplied to a Token Cloud
// caller. It never changes SelectedModel or asks an upstream model to run.
type ModelRankingResult struct {
	Version              string                  `json:"version"`
	PhysicalGroup        string                  `json:"physical_group"`
	RecommendedAccountID int64                   `json:"recommended_account_id"`
	RecommendedModel     string                  `json:"recommended_model"`
	RankingMargin        float64                 `json:"ranking_margin"`
	Scoring              ModelRankingWeights     `json:"scoring"`
	Candidates           []ModelRankingCandidate `json:"candidates"`
	ShadowOnly           bool                    `json:"shadow_only"`
	UsedForFinal         bool                    `json:"used_for_final"`
}

type ModelRankingWeights struct {
	PoolWeight       float64 `json:"pool_weight"`
	TaskFitWeight    float64 `json:"task_fit_weight"`
	CapabilityWeight float64 `json:"capability_weight"`
	TierWeight       float64 `json:"tier_weight"`
	RuntimeWeight    float64 `json:"runtime_weight"`
}

type ModelRankingCandidate struct {
	Rank       int     `json:"rank"`
	AccountID  int64   `json:"account_id"`
	Model      string  `json:"model"`
	FinalScore float64 `json:"final_score"`
}

func (r *ModelSelectionRequest) Validate() error {
	if r == nil {
		return fmt.Errorf("request is nil")
	}
	if r.ProtocolVersion != "" && r.ProtocolVersion != ModelSelectorProtocolVersion {
		return fmt.Errorf("unsupported protocol_version %q", r.ProtocolVersion)
	}
	if r.RequestID == "" {
		return fmt.Errorf("request_id is required")
	}
	if r.GroupID == 0 {
		return fmt.Errorf("group_id must be non-zero")
	}
	if r.Prompt == "" {
		return fmt.Errorf("prompt is required")
	}
	return nil
}
