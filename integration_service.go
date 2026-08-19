package semanticrouter

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type ModelSelectionService struct {
	router     *MultiLayerRouter
	tierRouter *RuleBasedTierRouter
	scheduler  SchedulerFacade
	config     IntegrationConfig
}

func NewModelSelectionService(scheduler SchedulerFacade, config IntegrationConfig) (*ModelSelectionService, error) {
	if scheduler == nil {
		return nil, fmt.Errorf("scheduler is required")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &ModelSelectionService{router: NewMultiLayerRouter(), tierRouter: NewRuleBasedTierRouter(), scheduler: scheduler, config: config}, nil
}

func (s *ModelSelectionService) Select(ctx context.Context, req *ModelSelectionRequest) *ModelSelectionResponse {
	id := ""
	if req != nil {
		id = req.RequestID
	}
	response := &ModelSelectionResponse{ProtocolVersion: ModelSelectorProtocolVersion, RequestID: id, SchedulerSource: "semantic_router", DryRun: true, ShadowOnly: true, UpstreamCalled: false}
	if err := req.Validate(); err != nil {
		response.Error = err.Error()
		return response
	}
	if ctx == nil {
		ctx = context.Background()
	}
	prompt := composeIntegrationPrompt(req)
	decision := s.router.Route(&RouteRequest{Prompt: prompt, HasImage: req.HasImage, HasDocument: req.HasDocument, ContentType: contentTypeForRequest(req)})
	tier := PreferredTier(req.RequestedTier)
	if tier == "" {
		if result, err := s.tierRouter.RouteWithPrompt(ctx, req.ModelIDsFirst(), decision.TaskType, prompt); err == nil && result != nil {
			tier = result.PreferredTier
		}
	}
	if tier == "" {
		tier = TierMedium
	}
	selected := s.scheduler.Select(&SchedulerSelectRequest{Model: req.ModelIDsFirst(), PreferredGroup: PhysicalGroupForPool(decision.PreferredPool), PreferredPool: decision.PreferredPool, PreferredTier: tier, TaskType: decision.TaskType, TaskSignals: integrationTaskSignals(req, decision), RequiredCapabilities: decision.RequiredCapabilities, AllowedModelIDs: append([]string(nil), req.ModelIDs...), PreviousResponseID: req.PreviousResponseID, SessionHash: req.SessionHash, RequiresStreaming: req.RequiresStreaming, RequiresToolCall: req.RequiresToolCall})
	response.PreferredPool, response.PreferredTier, response.TaskType = string(decision.PreferredPool), string(tier), string(decision.TaskType)
	response.Confidence = decision.Confidence
	if len(decision.MatchedRules) > 0 {
		response.MatchedRule = decision.MatchedRules[0]
	}
	if selected == nil || selected.Error != nil || selected.SelectedAccountID == 0 {
		if selected != nil && selected.Error != nil {
			response.Error = selected.Error.Error()
		} else {
			response.Error = "scheduler returned invalid account id"
		}
		return response
	}
	response.Success = true
	response.SelectedAccountID, response.SelectedModel, response.SelectedPool, response.SelectedTier, response.SchedulerLayer = selected.SelectedAccountID, selected.SelectedModel, selected.PoolUsed, selected.MatchedTier, selected.Layer
	response.CandidateCount, response.CandidateModels, response.CandidateDetails = selected.CandidateCount, selected.CandidateModels, selected.CandidateDetails
	response.ModelRanking = buildModelRanking(PhysicalGroupForPool(decision.PreferredPool), selected.CandidateDetails)
	return response
}

func integrationTaskSignals(req *ModelSelectionRequest, decision *MultiLayerDecision) []string {
	signals := make([]string, 0, 4)
	if decision != nil {
		switch decision.PreferredPool {
		case PoolCode:
			signals = append(signals, "code_generation")
		case PoolData:
			signals = append(signals, "data_analysis")
		case PoolDocument:
			signals = append(signals, "document_processing")
		}
	}
	prompt := ""
	if req != nil {
		prompt = composeIntegrationPrompt(req)
	}
	lower := strings.ToLower(prompt)
	if strings.ContainsAny(prompt, "\u4e2d\u6587\u8bf7\u7528\u5206\u6790\u8bbe\u8ba1") || containsHanText(prompt) {
		signals = append(signals, "chinese")
	}
	if containsOne(lower, "reason", "analysis", "architecture", "design", "\u63a8\u7406", "\u5206\u6790", "\u8bbe\u8ba1") {
		signals = append(signals, "reasoning")
	}
	if len([]rune(prompt)) >= 2000 {
		signals = append(signals, "long_context")
	}
	return uniqueIntegrationSignals(signals)
}

func containsHanText(value string) bool {
	for _, r := range value {
		if r >= '\u4e00' && r <= '\u9fff' {
			return true
		}
	}
	return false
}

func containsOne(text string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(text, strings.ToLower(value)) {
			return true
		}
	}
	return false
}

func uniqueIntegrationSignals(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func buildModelRanking(group string, details []ModelCandidateScore) *ModelRankingResult {
	if len(details) == 0 {
		return nil
	}
	sorted := append([]ModelCandidateScore(nil), details...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].FinalScore == sorted[j].FinalScore {
			return sorted[i].ModelID < sorted[j].ModelID
		}
		return sorted[i].FinalScore > sorted[j].FinalScore
	})
	ranking := &ModelRankingResult{
		Version: "model_ranking_v1", PhysicalGroup: group,
		Scoring:    ModelRankingWeights{PoolWeight: 0.75, TaskFitWeight: 0.25, CapabilityWeight: 0.55, TierWeight: 0.20, RuntimeWeight: 0.25},
		ShadowOnly: true, UsedForFinal: false,
		Candidates: make([]ModelRankingCandidate, 0, len(sorted)),
	}
	for index, detail := range sorted {
		ranking.Candidates = append(ranking.Candidates, ModelRankingCandidate{Rank: index + 1, AccountID: detail.AccountID, Model: detail.ModelID, FinalScore: detail.FinalScore})
	}
	ranking.RecommendedAccountID = ranking.Candidates[0].AccountID
	ranking.RecommendedModel = ranking.Candidates[0].Model
	if len(ranking.Candidates) > 1 {
		ranking.RankingMargin = ranking.Candidates[0].FinalScore - ranking.Candidates[1].FinalScore
	}
	return ranking
}

func composeIntegrationPrompt(req *ModelSelectionRequest) string {
	parts := []string{req.SystemPrompt, req.AgentContext, req.ProjectContext, req.ToolContext, strings.Join(req.ConversationHistory, "\n"), req.Prompt}
	kept := parts[:0]
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, "\n\n")
}

func contentTypeForRequest(req *ModelSelectionRequest) string {
	if req.HasCSV {
		return "text/csv"
	}
	if req.HasDocument {
		return "application/document"
	}
	return "text/plain"
}
func (r *ModelSelectionRequest) ModelIDsFirst() string {
	if r != nil && len(r.ModelIDs) > 0 {
		return r.ModelIDs[0]
	}
	return ""
}
