package semanticrouter

import (
	"crypto/sha256"
	"encoding/json"
	"encoding/hex"
	"fmt"
	"time"
)

type ShadowModeRequest struct {
	RequestID string
	APIKeyID  int64
	GroupID   *int64

	Model               string
	RouteRequest        *RouteRequest
	OldSchedulerRequest *SchedulerSelectRequest
}

type ShadowModeResult struct {
	OldSchedulerResult *SchedulerSelectResult
	Suggestion         *SchedulerSelectResult
	TakeoverResult     *SchedulerSelectResult // 非 nil 时表示 semantic-router 接管主链路
	Decision           *MultiLayerDecision
	LogEntry           *RoutingDecisionLogEntry
	ShadowError        error
	ShadowLatencyMs    float64
}

// MainResult returns the result that should be used for the live request.
// If takeover is active and the semantic-router suggestion is valid, it returns
// the suggestion; otherwise it falls back to the old scheduler result.
func (r *ShadowModeResult) MainResult() *SchedulerSelectResult {
	if r == nil {
		return nil
	}
	if r.TakeoverResult != nil {
		return r.TakeoverResult
	}
	return r.OldSchedulerResult
}

type ShadowRouter struct {
	oldScheduler    SchedulerFacade
	shadowScheduler SchedulerFacade
	semanticRouter  *MultiLayerRouter
	tierRouter      TierRouter
	logger          RoutingDecisionLogWriter
	config          SemanticRouterRuntimeConfig
	metrics         *ShadowMetrics
}

func NewShadowRouter(
	oldScheduler SchedulerFacade,
	shadowScheduler SchedulerFacade,
	semanticRouter *MultiLayerRouter,
	tierRouter TierRouter,
	logger RoutingDecisionLogWriter,
) *ShadowRouter {
	if semanticRouter == nil {
		semanticRouter = NewMultiLayerRouter()
	}
	if tierRouter == nil {
		tierRouter = NewRuleBasedTierRouter()
	}
	return &ShadowRouter{
		oldScheduler:    oldScheduler,
		shadowScheduler: shadowScheduler,
		semanticRouter:  semanticRouter,
		tierRouter:      tierRouter,
		logger:          logger,
		config:          DefaultSemanticRouterRuntimeConfig(),
		metrics:         NewShadowMetrics(),
	}
}

func (s *ShadowRouter) SetRuntimeConfig(config SemanticRouterRuntimeConfig) error {
	if err := config.Validate(); err != nil {
		return err
	}
	s.config = config
	return nil
}

func (s *ShadowRouter) Stats() ShadowStatsSnapshot {
	return s.metrics.Snapshot()
}

// ShouldTakeover determines whether the current request should use the
// semantic-router suggestion as the main result based on a hash-based
// consistent percentage selection.
// percentage=0 means never takeover, percentage=100 means always takeover.
func (s *ShadowRouter) ShouldTakeover(req *ShadowModeRequest) bool {
	if !s.config.SemanticRouterTakeoverEnabled {
		return false
	}
	pct := s.config.TakeoverPercentage
	if pct <= 0 {
		return false
	}
	if pct >= 100 {
		return true
	}
	// Use the request identity to compute a stable hash for consistent
	// percentage-based selection.
	hashInput := fmt.Sprintf("%s-%d", req.RequestID, req.APIKeyID)
	if req.GroupID != nil {
		hashInput = fmt.Sprintf("%s-%d", hashInput, *req.GroupID)
	}
	h := sha256.Sum256([]byte(hashInput))
	// Take the first 4 bytes of the hash, interpret as uint32, mod 100.
	val := uint32(h[0])<<24 | uint32(h[1])<<16 | uint32(h[2])<<8 | uint32(h[3])
	return val%100 < uint32(pct)
}

func (s *ShadowRouter) Route(req *ShadowModeRequest) (result *ShadowModeResult) {
	result = &ShadowModeResult{}
	if req == nil {
		err := fmt.Errorf("nil shadow mode request")
		result.OldSchedulerResult = &SchedulerSelectResult{Error: err}
		result.ShadowError = err
		return result
	}

	oldReq := req.OldSchedulerRequest
	if oldReq == nil {
		oldReq = &SchedulerSelectRequest{
			Model:         req.Model,
			PreferredPool: PoolDefault,
			PreferredTier: TierMedium,
			TaskType:      TaskTypeText,
		}
	}
	if s.oldScheduler == nil {
		result.OldSchedulerResult = &SchedulerSelectResult{Error: fmt.Errorf("old scheduler is not configured")}
	} else {
		result.OldSchedulerResult = s.oldScheduler.Select(oldReq)
		if result.OldSchedulerResult == nil {
			result.OldSchedulerResult = &SchedulerSelectResult{Error: fmt.Errorf("old scheduler returned nil result")}
		}
	}
	if !s.config.SemanticRouterShadowEnabled {
		return result
	}

	shadowStartedAt := time.Now()
	var tierDecision *TierRouteDecision
	defer func() {
		if recovered := recover(); recovered != nil {
			result.ShadowError = fmt.Errorf("semantic-router shadow panic: %v", recovered)
		}
		result.ShadowLatencyMs = float64(time.Since(shadowStartedAt)) / float64(time.Millisecond)
		if result.ShadowLatencyMs <= 0 {
			result.ShadowLatencyMs = 0.001
		}
		result.LogEntry = NewRoutingDecisionLogEntry(req, result.Decision, tierDecision, result.Suggestion, result.OldSchedulerResult)
		result.LogEntry.ShadowLatencyMs = result.ShadowLatencyMs

		if err := s.logShadowDecision(result.LogEntry); err != nil && result.ShadowError == nil {
			result.ShadowError = err
		}
		if err := s.recordShadowMetrics(result); err != nil && result.ShadowError == nil {
			result.ShadowError = err
		}
	}()

	routeReq := req.RouteRequest
	if routeReq == nil {
		routeReq = &RouteRequest{Model: req.Model}
	}

	decision := s.semanticRouter.Route(routeReq)
	result.Decision = decision
	var err error
	tierDecision, err = s.tierRouter.Route(nil, req.Model, decision.TaskType)
	if err != nil {
		result.ShadowError = err
		return result
	}

	shadowReq := &SchedulerSelectRequest{
		Model:                req.Model,
		PreferredPool:        decision.PreferredPool,
		PreferredTier:        tierDecision.PreferredTier,
		TaskType:             decision.TaskType,
		RequiredCapabilities: decision.RequiredCapabilities,
	}
	// Takeover still needs a local scheduler suggestion even when standalone
	// dry-run reporting is disabled. This never invokes an upstream model.
	if (s.config.SemanticRouterDryRunEnabled || s.config.SemanticRouterTakeoverEnabled) && s.shadowScheduler != nil {
		result.Suggestion = s.shadowScheduler.Select(shadowReq)
		if result.Suggestion != nil && result.Suggestion.Error != nil {
			result.ShadowError = result.Suggestion.Error
		} else if result.Suggestion != nil && result.Suggestion.SelectedAccountID == 0 {
			result.ShadowError = fmt.Errorf("semantic-router shadow selected invalid account 0")
		} else if isDisabledShadowSelection(s.shadowScheduler, result.Suggestion) {
			result.ShadowError = fmt.Errorf("semantic-router shadow selected disabled account %d", result.Suggestion.SelectedAccountID)
		}
	}

	// Takeover logic: if enabled and ShouldTakeover returns true and the
	// suggestion is valid, use the suggestion as the main result.
	// On any error (including account 0 or disabled), fall back to old scheduler.
	if s.config.SemanticRouterTakeoverEnabled && s.ShouldTakeover(req) {
		if result.Suggestion != nil && result.Suggestion.Error == nil &&
			result.Suggestion.SelectedAccountID != 0 &&
			!isDisabledShadowSelection(s.shadowScheduler, result.Suggestion) {
			result.TakeoverResult = result.Suggestion
		} else {
			// Fallback to old scheduler on takeover failure
			result.TakeoverResult = result.OldSchedulerResult
			if result.ShadowError == nil {
				result.ShadowError = fmt.Errorf("takeover fallback: semantic-router suggestion invalid")
			}
		}
	}

	return result
}

func (s *ShadowRouter) logShadowDecision(entry *RoutingDecisionLogEntry) (err error) {
	if s.logger == nil {
		return nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("routing decision logger panic: %v", recovered)
		}
	}()
	return s.logger.LogRoutingDecision(entry)
}

func (s *ShadowRouter) recordShadowMetrics(result *ShadowModeResult) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("shadow metrics panic: %v", recovered)
		}
	}()
	// Determine if takeover was attempted: TakeoverResult is set when takeover
	// was attempted (either successful or fallback).
	didAttemptTakeover := result.TakeoverResult != nil
	takeoverUsedSuggestion := didAttemptTakeover && result.TakeoverResult == result.Suggestion
	s.metrics.Record(result.OldSchedulerResult, result.Suggestion, result.ShadowError, s.shadowScheduler, result.ShadowLatencyMs, result.TakeoverResult, didAttemptTakeover, takeoverUsedSuggestion)
	return nil
}

func NewRoutingDecisionLogEntry(
	req *ShadowModeRequest,
	decision *MultiLayerDecision,
	tierDecision *TierRouteDecision,
	suggestion *SchedulerSelectResult,
	oldResult *SchedulerSelectResult,
) *RoutingDecisionLogEntry {
	entry := &RoutingDecisionLogEntry{
		RequestID:           req.RequestID,
		APIKeyID:            req.APIKeyID,
		PromptHash:          hashPrompt(""),
		MatchedRules:        []string{},
		SemanticScores:      map[string]float64{},
		FinalDecisionSource: string(DecisionSourceFallback),
		CreatedAt:           time.Now(),
	}
	if req.GroupID != nil {
		entry.GroupID = *req.GroupID
	}
	if req.RouteRequest != nil {
		entry.PromptHash = hashPrompt(req.RouteRequest.Prompt)
	}
	if decision != nil {
		entry.PreferredPool = decision.PreferredPool
		entry.TaskType = decision.TaskType
		entry.Confidence = decision.Confidence
		entry.MatchedRules = append([]string(nil), decision.MatchedRules...)
		entry.SemanticScores = cloneFloatMap(decision.SemanticScores)
		entry.FinalDecisionSource = string(decision.DecisionSource)
		entry.FallbackReason = decision.FallbackReason
	}
	if tierDecision != nil {
		entry.PreferredTier = tierDecision.PreferredTier
	}
	if suggestion != nil && suggestion.Error == nil {
		entry.SelectedAccountID = suggestion.SelectedAccountID
		entry.SelectedModel = suggestion.SelectedModel
		entry.SchedulerLayer = suggestion.Layer
		entry.NewSuggestedAccountID = suggestion.SelectedAccountID
		entry.NewSuggestedModel = suggestion.SelectedModel
		entry.NewSuggestedPool = suggestion.PoolUsed
		if ranking := buildModelRanking(PhysicalGroupForPool(PreferredPool(suggestion.PoolUsed)), suggestion.CandidateDetails); ranking != nil {
			if encoded, err := json.Marshal(ranking); err == nil {
				entry.ModelRankingJSON = string(encoded)
			}
		}
	}
	if oldResult != nil && oldResult.Error == nil {
		entry.OldSchedulerAccountID = oldResult.SelectedAccountID
		entry.OldSelectedAccountID = oldResult.SelectedAccountID
		entry.OldSelectedModel = oldResult.SelectedModel
		entry.OldSelectedPool = oldResult.PoolUsed
	}
	entry.IsAgree = entry.OldSelectedAccountID != 0 &&
		entry.NewSuggestedAccountID != 0 &&
		entry.OldSelectedAccountID == entry.NewSuggestedAccountID
	return entry
}

func hashPrompt(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(sum[:])
}

func cloneFloatMap(src map[string]float64) map[string]float64 {
	dst := make(map[string]float64, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
