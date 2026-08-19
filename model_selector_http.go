package semanticrouter

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ModelSelectorHTTPConfig contains the v1.2 HTTP boundary configuration.
// An empty SelectorSecret deliberately leaves authentication optional, matching
// the integration document. Production should set a non-empty value.
type ModelSelectorHTTPConfig struct {
	Version             string
	SelectorSecret      string
	OfficialVLLMEnabled bool
	OfficialVLLMURL     string
	OfficialVLLMTimeout time.Duration
	OfficialVLLMWeight  float64
	HistoryFile         string
	ModelCatalogFile    string
	APIKeyGroupFile     string
	StatusMaxConcurrent int
}

func DefaultModelSelectorHTTPConfig() ModelSelectorHTTPConfig {
	return ModelSelectorHTTPConfig{Version: "1.3.0", OfficialVLLMURL: "http://127.0.0.1:8080", OfficialVLLMTimeout: 800 * time.Millisecond, OfficialVLLMWeight: 0.25, StatusMaxConcurrent: 32}
}

type modelSelectorHTTPResponse struct {
	Success bool        `json:"success"`
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

type ModelSelectorHeartbeatData struct {
	Status    string  `json:"status"`
	Version   string  `json:"version"`
	Timestamp int64   `json:"timestamp"`
	Load      float64 `json:"load"`
}

// ModelSelectorHTTPHandler serves the TokenCloud/sub2api v1.2 HTTP contract.
// It is independent from the existing debug and shadow endpoints.
type ModelSelectorHTTPHandler struct {
	config    ModelSelectorHTTPConfig
	service   *ModelSelectionService
	vllm      officialVLLMScorer
	history   *modelSelectorHistory
	catalog   *modelSelectorSyncedCatalog
	keyGroups *modelSelectorAPIKeyGroupStore
	metrics   *modelSelectorMetrics
}

// ModelSelectorHistoryEntry is a redacted, shadow-only record of a TokenCloud
// selector call. It is deliberately separate from routing decisions and is
// never sent to an upstream model.
type ModelSelectorHistoryEntry struct {
	ID             int64                        `json:"id"`
	CreatedAt      time.Time                    `json:"created_at"`
	RequestID      string                       `json:"request_id,omitempty"`
	PromptPreview  string                       `json:"prompt_preview"`
	PromptHash     string                       `json:"prompt_hash"`
	SystemPresent  bool                         `json:"system_present"`
	MessageCount   int                          `json:"message_count"`
	ModelList      []string                     `json:"model_list"`
	Semantics      []ModelSelectorSemanticScore `json:"semantics"`
	ModelScoreList []ModelSelectorModelScore    `json:"model_score_list"`
	DecodedRequest map[string]interface{}       `json:"decoded_request"`
	ShadowOnly     bool                         `json:"shadow_only"`
	UpstreamCalled bool                         `json:"upstream_called"`
}

type modelSelectorHistory struct {
	mu      sync.RWMutex
	nextID  atomic.Int64
	entries []ModelSelectorHistoryEntry
	file    string
}

func newModelSelectorHistory(file string) *modelSelectorHistory {
	history := &modelSelectorHistory{file: strings.TrimSpace(file)}
	history.load()
	return history
}

func (h *modelSelectorHistory) append(entry ModelSelectorHistoryEntry) {
	entry.ID = h.nextID.Add(1)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.entries = append(h.entries, entry)
	if len(h.entries) > 200 {
		h.entries = append([]ModelSelectorHistoryEntry(nil), h.entries[len(h.entries)-200:]...)
	}
	h.appendToFile(entry)
}

func (h *modelSelectorHistory) recent(limit int) []ModelSelectorHistoryEntry {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	start := len(h.entries) - limit
	if start < 0 {
		start = 0
	}
	result := append([]ModelSelectorHistoryEntry(nil), h.entries[start:]...)
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}

func (h *modelSelectorHistory) load() {
	if h.file == "" {
		return
	}
	file, err := os.Open(h.file)
	if err != nil {
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), MaxIntegrationFrameBytes)
	for scanner.Scan() {
		var entry ModelSelectorHistoryEntry
		if json.Unmarshal(scanner.Bytes(), &entry) != nil || entry.ID <= 0 {
			continue
		}
		h.entries = append(h.entries, entry)
		if entry.ID > h.nextID.Load() {
			h.nextID.Store(entry.ID)
		}
	}
	if len(h.entries) > 200 {
		h.entries = append([]ModelSelectorHistoryEntry(nil), h.entries[len(h.entries)-200:]...)
	}
}

func (h *modelSelectorHistory) appendToFile(entry ModelSelectorHistoryEntry) {
	if h.file == "" {
		return
	}
	if os.MkdirAll(filepath.Dir(h.file), 0755) != nil {
		return
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return
	}
	file, err := os.OpenFile(h.file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.Write(append(line, '\n'))
}

// officialVLLMScorer is deliberately a small HTTP boundary. It keeps the
// TokenCloud selector independent from the Playground-only vLLM package and
// ensures a vLLM failure can never fail the primary selector response.
type officialVLLMScorer interface {
	Score(context.Context, string) (officialVLLMScore, error)
}

type officialVLLMScore struct {
	SignalValues map[string]float64
	TopSignal    string
	TopScore     float64
	LatencyMS    float64
}

type officialVLLMHTTPScorer struct {
	url    string
	client *http.Client
}

func (s *officialVLLMHTTPScorer) Score(ctx context.Context, prompt string) (officialVLLMScore, error) {
	body, err := json.Marshal(map[string]string{"text": prompt})
	if err != nil {
		return officialVLLMScore{}, err
	}
	started := time.Now()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(s.url, "/")+"/api/v1/eval", bytes.NewReader(body))
	if err != nil {
		return officialVLLMScore{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return officialVLLMScore{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return officialVLLMScore{}, fmt.Errorf("official vLLM returned status %d", response.StatusCode)
	}
	var payload struct {
		SignalValues map[string]float64 `json:"signal_values"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, MaxIntegrationFrameBytes)).Decode(&payload); err != nil {
		return officialVLLMScore{}, err
	}
	result := officialVLLMScore{SignalValues: payload.SignalValues, LatencyMS: float64(time.Since(started).Milliseconds())}
	for key, value := range result.SignalValues {
		if strings.Count(key, ":") != 1 || !strings.HasPrefix(key, "embedding:semantic_") || value <= result.TopScore {
			continue
		}
		result.TopSignal, result.TopScore = key, value
	}
	if result.TopSignal == "" {
		return officialVLLMScore{}, fmt.Errorf("official vLLM returned no semantic signal values")
	}
	return result, nil
}

func NewModelSelectorHTTPHandler(config ModelSelectorHTTPConfig, service ...*ModelSelectionService) *ModelSelectorHTTPHandler {
	if config.Version == "" {
		config.Version = DefaultModelSelectorHTTPConfig().Version
	}
	var selectionService *ModelSelectionService
	if len(service) > 0 {
		selectionService = service[0]
	}
	handler := &ModelSelectorHTTPHandler{config: config, service: selectionService, history: newModelSelectorHistory(config.HistoryFile), catalog: newModelSelectorSyncedCatalog(config.ModelCatalogFile), keyGroups: newModelSelectorAPIKeyGroupStore(config.APIKeyGroupFile), metrics: newModelSelectorMetrics()}
	if config.OfficialVLLMEnabled {
		timeout := config.OfficialVLLMTimeout
		if timeout <= 0 {
			timeout = DefaultModelSelectorHTTPConfig().OfficialVLLMTimeout
		}
		handler.vllm = &officialVLLMHTTPScorer{url: config.OfficialVLLMURL, client: &http.Client{Timeout: timeout}}
	}
	return handler
}

func (h *ModelSelectorHTTPHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/v1/model-selector/heartbeat", h.handleHeartbeat)
	mux.HandleFunc("/v1/model-selector/status", h.handleStatus)
	mux.HandleFunc("/v1/model-selector/select", h.handleSelect)
	mux.HandleFunc("/v1/model-selector/sync-models", h.handleSyncModels)
	mux.HandleFunc("/v1/model-selector/sync-api-key-group", h.handleSyncAPIKeyGroup)
	// This is only reachable through the internal Nginx route. The selector
	// itself remains loopback-bound in the deployment.
	mux.HandleFunc("/v1/model-selector/history", h.handleHistory)
}

type ModelSelectorSelectHTTPRequest struct {
	UserAPICall string   `json:"user_api_call"`
	ModelList   []string `json:"model_list"`
	// Models and Accounts are supplied by TokenCloud v1.3 on every request.
	// They contain only selection metadata and live health/load observations;
	// credentials are deliberately not part of this boundary.
	Models   []ModelSelectorModelRef    `json:"models,omitempty"`
	Accounts []ModelSelectorAccountInfo `json:"accounts,omitempty"`
	// GroupID is optional for backwards compatibility. When it is supplied,
	// the request must use a previously synchronized group model snapshot.
	GroupID  int64 `json:"group_id,omitempty"`
	APIKeyID int64 `json:"api_key_id,omitempty"`
}

// ModelSelectorModelRef maps the public model ID to its upstream platform.
type ModelSelectorModelRef struct {
	ModelID       string `json:"model_id"`
	Platform      string `json:"platform,omitempty"`
	UpstreamModel string `json:"upstream_model,omitempty"`
}

// ModelSelectorAccountInfo is TokenCloud's v1.3 non-secret account snapshot.
// It intentionally has no credential, raw API key, or provider error payload.
type ModelSelectorAccountInfo struct {
	AccountID          int64                   `json:"account_id"`
	Name               string                  `json:"name,omitempty"`
	Platform           string                  `json:"platform,omitempty"`
	Type               string                  `json:"type,omitempty"`
	Priority           int                     `json:"priority"`
	RateMultiplier     float64                 `json:"rate_multiplier"`
	Concurrency        int                     `json:"concurrency"`
	LoadFactor         int                     `json:"load_factor"`
	Models             []ModelSelectorModelRef `json:"models,omitempty"`
	Schedulable        bool                    `json:"schedulable"`
	RateLimited        bool                    `json:"rate_limited"`
	Overloaded         bool                    `json:"overloaded"`
	TempUnschedulable  bool                    `json:"temp_unschedulable"`
	QuotaLimit         float64                 `json:"quota_limit"`
	QuotaUsed          float64                 `json:"quota_used"`
	QuotaDailyLimit    float64                 `json:"quota_daily_limit"`
	QuotaDailyUsed     float64                 `json:"quota_daily_used"`
	QuotaWeeklyLimit   float64                 `json:"quota_weekly_limit"`
	QuotaWeeklyUsed    float64                 `json:"quota_weekly_used"`
	ErrorRateEWMA      float64                 `json:"error_rate_ewma"`
	TTFTEWMAMs         float64                 `json:"ttft_ewma_ms"`
	CurrentConcurrency int                     `json:"current_concurrency"`
	WaitingCount       int                     `json:"waiting_count"`
	LoadRate           int                     `json:"load_rate"`
}

type ModelSelectorSemanticScore struct {
	Dimension   string  `json:"dimension"`
	Score       float64 `json:"score"`
	Description string  `json:"description,omitempty"`
}

type ModelSelectorModelScore struct {
	ModelID string  `json:"model_id"`
	Score   float64 `json:"score"`
}

type ModelSelectorSelectHTTPResponse struct {
	UserAPICall    string                       `json:"user_api_call"`
	LocalRouting   ModelSelectorLocalRouting    `json:"local_routing"`
	Semantics      []ModelSelectorSemanticScore `json:"semantics"`
	ModelScoreList []ModelSelectorModelScore    `json:"model_score_list"`
}

// ModelSelectorLocalRouting exposes the result of the local semantic-router
// pipeline. Official vLLM scores stay separate in semantics because they are
// a shadow observation, not the routing decision.
type ModelSelectorLocalRouting struct {
	GroupID        int64   `json:"group_id,omitempty"`
	APIKeyID       int64   `json:"api_key_id,omitempty"`
	PreferredPool  string  `json:"preferred_pool"`
	PreferredTier  string  `json:"preferred_tier"`
	TaskType       string  `json:"task_type"`
	Confidence     float64 `json:"confidence"`
	Source         string  `json:"source"`
	ShadowOnly     bool    `json:"shadow_only"`
	UpstreamCalled bool    `json:"upstream_called"`
}

func (h *ModelSelectorHTTPHandler) handleSelect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeResponse(w, http.StatusMethodNotAllowed, false, "method not allowed", nil)
		return
	}
	if !h.authorized(r) {
		h.writeResponse(w, http.StatusUnauthorized, false, "selector authentication failed", nil)
		return
	}
	started := time.Now()
	success := false
	officialAttempted := false
	officialSucceeded := false
	recommendedModel := ""
	h.metrics.beginSelection()
	defer func() {
		h.metrics.finishSelection(time.Since(started), success, officialAttempted, officialSucceeded, recommendedModel)
	}()
	defer r.Body.Close()
	var request ModelSelectorSelectHTTPRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, MaxIntegrationFrameBytes)).Decode(&request); err != nil {
		h.writeResponse(w, http.StatusBadRequest, false, "invalid JSON request", nil)
		return
	}
	if request.UserAPICall == "" || len(request.ModelList) == 0 {
		h.writeResponse(w, http.StatusBadRequest, false, "user_api_call and model_list are required", nil)
		return
	}
	raw, err := DecodeModelSelectorUserAPICall(request.UserAPICall)
	if err != nil {
		h.writeResponse(w, http.StatusBadRequest, false, "invalid user_api_call: "+err.Error(), nil)
		return
	}
	prompt, systemPrompt := promptFromUserAPICall(raw)
	if strings.TrimSpace(prompt) == "" {
		h.writeResponse(w, http.StatusBadRequest, false, "user_api_call contains no user prompt", nil)
		return
	}
	groupID, mapped, err := h.resolveRequestGroup(request.APIKeyID, request.GroupID)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "conflicts with mapped") {
			status = http.StatusConflict
		}
		h.writeResponse(w, status, false, err.Error(), nil)
		return
	}
	models, err := h.catalog.requireGroupModels(groupID, request.ModelList)
	if err != nil {
		h.writeResponse(w, http.StatusBadRequest, false, err.Error(), nil)
		return
	}
	if request.APIKeyID > 0 && !mapped {
		if _, err := h.keyGroups.bind(request.APIKeyID, groupID); err != nil {
			h.writeResponse(w, http.StatusInternalServerError, false, err.Error(), nil)
			return
		}
	}
	var official *officialVLLMScore
	if h.vllm != nil {
		officialAttempted = true
		if score, scoreErr := h.vllm.Score(r.Context(), prompt); scoreErr == nil {
			official = &score
			officialSucceeded = true
		}
	}
	response, err := h.runSelection(r.Context(), r.Header.Get("X-Request-ID"), groupID, prompt, systemPrompt, models)
	if err != nil {
		// The local integration server does not own TokenCloud accounts. Keep
		// debugging useful by scoring the caller's allowlist from the same
		// versioned platform profiles, without selecting an account or upstream.
		response = profileOnlyHTTPSelectionWithAccounts(prompt, models, official, request.Models, request.Accounts)
	}
	result := ModelSelectorSelectHTTPResponse{
		UserAPICall: request.UserAPICall, LocalRouting: localRoutingForSelection(response), Semantics: semanticsForSelection(response, official), ModelScoreList: modelScoresForSelection(response, models),
	}
	result.LocalRouting.GroupID = groupID
	result.LocalRouting.APIKeyID = request.APIKeyID
	h.history.append(buildModelSelectorHistoryEntry(r.Header.Get("X-Request-ID"), raw, prompt, systemPrompt, models, result))
	if len(result.ModelScoreList) > 0 {
		recommendedModel = result.ModelScoreList[0].ModelID
	}
	success = true
	h.writeResponse(w, http.StatusOK, true, "ok", result)
}

func (h *ModelSelectorHTTPHandler) resolveRequestGroup(apiKeyID, requestedGroupID int64) (groupID int64, mapped bool, err error) {
	if apiKeyID <= 0 {
		return requestedGroupID, false, nil
	}
	if groupID, mapped = h.keyGroups.lookup(apiKeyID); mapped {
		if requestedGroupID > 0 && requestedGroupID != groupID {
			return 0, true, fmt.Errorf("api_key_id %d conflicts with mapped group_id %d", apiKeyID, groupID)
		}
		return groupID, true, nil
	}
	if requestedGroupID <= 0 {
		return 0, false, fmt.Errorf("api_key_id %d has no mapped group_id", apiKeyID)
	}
	return requestedGroupID, false, nil
}

func (h *ModelSelectorHTTPHandler) handleSyncModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeResponse(w, http.StatusMethodNotAllowed, false, "method not allowed", nil)
		return
	}
	if !h.authorized(r) {
		h.writeResponse(w, http.StatusUnauthorized, false, "selector authentication failed", nil)
		return
	}
	defer r.Body.Close()
	var request ModelSelectorSyncHTTPRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, MaxIntegrationFrameBytes)).Decode(&request); err != nil {
		h.writeResponse(w, http.StatusBadRequest, false, "invalid JSON request", nil)
		return
	}
	result, err := h.catalog.sync(request)
	if err != nil {
		h.writeResponse(w, http.StatusBadRequest, false, err.Error(), nil)
		return
	}
	h.writeResponse(w, http.StatusOK, true, "models synced", result)
}

func (h *ModelSelectorHTTPHandler) handleSyncAPIKeyGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeResponse(w, http.StatusMethodNotAllowed, false, "method not allowed", nil)
		return
	}
	if !h.authorized(r) {
		h.writeResponse(w, http.StatusUnauthorized, false, "selector authentication failed", nil)
		return
	}
	defer r.Body.Close()
	var request ModelSelectorSyncAPIKeyGroupHTTPRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, MaxIntegrationFrameBytes)).Decode(&request); err != nil {
		h.writeResponse(w, http.StatusBadRequest, false, "invalid JSON request", nil)
		return
	}
	if _, err := h.catalog.requireGroupModels(request.GroupID, []string{}); err != nil {
		h.writeResponse(w, http.StatusBadRequest, false, err.Error(), nil)
		return
	}
	binding, err := h.keyGroups.bind(request.APIKeyID, request.GroupID)
	if err != nil {
		h.writeResponse(w, http.StatusBadRequest, false, err.Error(), nil)
		return
	}
	h.writeResponse(w, http.StatusOK, true, "api-key group synced", ModelSelectorSyncAPIKeyGroupHTTPResponse(binding))
}

func localRoutingForSelection(response *ModelSelectionResponse) ModelSelectorLocalRouting {
	if response == nil {
		return ModelSelectorLocalRouting{Source: "local_router_unavailable", ShadowOnly: true, UpstreamCalled: false}
	}
	pool := response.PreferredPool
	if pool == "" {
		pool = response.SelectedPool
	}
	tier := response.PreferredTier
	if tier == "" {
		tier = response.SelectedTier
	}
	source := response.SchedulerSource
	if source == "" {
		source = "local_semantic_router"
	}
	return ModelSelectorLocalRouting{
		PreferredPool: pool, PreferredTier: tier, TaskType: response.TaskType, Confidence: roundSelectorScore(response.Confidence),
		Source: source, ShadowOnly: true, UpstreamCalled: false,
	}
}

func (h *ModelSelectorHTTPHandler) handleHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeResponse(w, http.StatusMethodNotAllowed, false, "method not allowed", nil)
		return
	}
	limit := 50
	if value := strings.TrimSpace(r.URL.Query().Get("limit")); value != "" {
		if _, err := fmt.Sscanf(value, "%d", &limit); err != nil {
			limit = 50
		}
	}
	h.writeResponse(w, http.StatusOK, true, "ok", map[string]interface{}{
		"records": h.history.recent(limit), "shadow_only": true, "upstream_called": false,
	})
}

func (h *ModelSelectorHTTPHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeResponse(w, http.StatusMethodNotAllowed, false, "method not allowed", nil)
		return
	}
	if !h.authorized(r) {
		h.writeResponse(w, http.StatusUnauthorized, false, "selector authentication failed", nil)
		return
	}
	groupCount, modelCount := h.catalog.stats()
	h.writeResponse(w, http.StatusOK, true, "ok", h.metrics.snapshot(h.config.Version, h.vllm != nil, h.config.StatusMaxConcurrent, groupCount, modelCount, h.keyGroups.count()))
}

func buildModelSelectorHistoryEntry(requestID string, raw []byte, prompt, systemPrompt string, models []string, response ModelSelectorSelectHTTPResponse) ModelSelectorHistoryEntry {
	var decoded interface{}
	if json.Unmarshal(raw, &decoded) != nil {
		decoded = map[string]interface{}{}
	}
	clean, _ := redactSelectorValue(decoded).(map[string]interface{})
	if clean == nil {
		clean = map[string]interface{}{}
	}
	return ModelSelectorHistoryEntry{
		CreatedAt: time.Now().UTC(), RequestID: requestID, PromptPreview: truncateSelectorPreview(prompt, 300), PromptHash: selectorPromptHash(prompt),
		SystemPresent: strings.TrimSpace(systemPrompt) != "", MessageCount: selectorMessageCount(raw), ModelList: append([]string(nil), models...),
		Semantics: response.Semantics, ModelScoreList: response.ModelScoreList, DecodedRequest: clean, ShadowOnly: true, UpstreamCalled: false,
	}
}

func selectorPromptHash(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return fmt.Sprintf("%x", sum[:])
}

func truncateSelectorPreview(value string, max int) string {
	value = strings.TrimSpace(value)
	if len([]rune(value)) <= max {
		return value
	}
	return string([]rune(value)[:max]) + "..."
}

func selectorMessageCount(raw []byte) int {
	var call struct {
		Messages []json.RawMessage `json:"messages"`
	}
	_ = json.Unmarshal(raw, &call)
	return len(call.Messages)
}

func redactSelectorValue(value interface{}) interface{} {
	switch current := value.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{}, len(current))
		for key, child := range current {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "api_key") || strings.Contains(lower, "apikey") || strings.Contains(lower, "authorization") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") {
				result[key] = "[redacted]"
				continue
			}
			result[key] = redactSelectorValue(child)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(current))
		for index, child := range current {
			result[index] = redactSelectorValue(child)
		}
		return result
	default:
		return value
	}
}

func profileOnlyHTTPSelection(prompt string, models []string, official *officialVLLMScore) *ModelSelectionResponse {
	return profileOnlyHTTPSelectionWithAccounts(prompt, models, official, nil, nil)
}

func profileOnlyHTTPSelectionWithAccounts(prompt string, models []string, official *officialVLLMScore, modelRefs []ModelSelectorModelRef, accounts []ModelSelectorAccountInfo) *ModelSelectionResponse {
	decision := NewMultiLayerRouter().Route(&RouteRequest{Prompt: prompt})
	applySelectorProfessionalTaskGuard(decision, prompt)
	applyConservativeOfficialPoolShadow(decision, official)
	details := make([]ModelCandidateScore, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		profile := platformModelProfile(model)
		poolScore := profile.GeneralScore
		switch decision.PreferredPool {
		case PoolCode:
			poolScore = profile.CodingAgentScore
		case PoolData:
			poolScore = profile.DataAnalysisScore
		case PoolDocument:
			poolScore = profile.DocumentScore
		case PoolVision:
			poolScore = profile.VisionScore
		}
		taskFit := poolScore
		if containsHanText(prompt) {
			taskFit = 0.75*taskFit + 0.25*profile.ChineseScore
		}
		if strings.Contains(strings.ToLower(prompt), "reason") || strings.Contains(prompt, "推理") || strings.Contains(prompt, "分析") {
			taskFit = 0.75*taskFit + 0.25*profile.ReasoningScore
		}
		finalScore := 0.75*poolScore + 0.25*taskFit
		if officialPoolScore := officialScoreForPool(official, decision.PreferredPool); officialPoolScore > 0 {
			finalScore = (1-0.25)*finalScore + 0.25*officialPoolScore
		}
		candidate := ModelCandidateScore{AccountID: 0, ModelID: model, Provider: providerForModelRef(model, modelRefs, profile.Provider), PoolScore: poolScore, TaskFitScore: taskFit, CapabilityScore: poolScore, FinalScore: finalScore, ProfileSource: profile.ProfileSource, EvidenceSource: profile.EvidenceSource, ScoreConfidence: profile.ScoreConfidence, BenchmarkVersion: profile.BenchmarkVersion, EvaluatedAt: profile.EvaluatedAt, RankingVersion: "http_profile_shadow_v1", Reason: "profile_only_fallback_no_account_snapshot"}
		if len(accounts) > 0 {
			account, runtime, priority, cost, _, ok := bestLiveAccountForModel(model, accounts)
			if !ok {
				candidate.FinalScore = 0
				candidate.RankingVersion = "http_profile_shadow_v1_account_aware"
				candidate.Reason = "no eligible TokenCloud v1.3 account snapshot"
			} else {
				candidate.AccountID = account.AccountID
				candidate.LoadScore = runtime
				candidate.PriorityScore = priority
				candidate.CostScore = cost
				candidate.LatencyMS = account.TTFTEWMAMs
				candidate.RuntimeScore = runtime
				candidate.FinalScore = 0.70*finalScore + 0.30*runtime
				candidate.RankingVersion = "http_profile_shadow_v1_account_aware"
				candidate.Reason = "TokenCloud v1.3 live account snapshot"
			}
		}
		details = append(details, candidate)
	}
	sort.SliceStable(details, func(i, j int) bool {
		if details[i].FinalScore == details[j].FinalScore {
			return details[i].ModelID < details[j].ModelID
		}
		return details[i].FinalScore > details[j].FinalScore
	})
	tier := TierMedium
	if tierDecision, err := NewRuleBasedTierRouter().RouteWithPrompt(context.Background(), firstModelID(models), decision.TaskType, prompt); err == nil && tierDecision != nil && tierDecision.PreferredTier != "" {
		tier = tierDecision.PreferredTier
	}
	source := "profile_shadow_fallback"
	if len(accounts) > 0 {
		source = "profile_shadow_account_aware_v1.3"
	}
	response := &ModelSelectionResponse{Success: true, PreferredPool: string(decision.PreferredPool), PreferredTier: string(tier), TaskType: string(decision.TaskType), Confidence: decision.Confidence, CandidateCount: len(details), CandidateDetails: details, SchedulerSource: source, DryRun: true, ShadowOnly: true, UpstreamCalled: false}
	response.ModelRanking = buildModelRanking(PhysicalGroupForPool(decision.PreferredPool), details)
	return response
}

// applySelectorProfessionalTaskGuard corrects one high-precision boundary the
// generic pool router cannot infer from a single token: a request to create a
// runbook/SOP/document that covers operational stages is a document task, not
// a code implementation task. It requires both an explicit artifact and an
// operational stage signal so ordinary deployment/debugging requests are left
// unchanged.
func applySelectorProfessionalTaskGuard(decision *MultiLayerDecision, prompt string) {
	if decision == nil {
		return
	}
	lower := strings.ToLower(prompt)
	artifact := selectorContainsAny(lower, "运行手册", "操作手册", "技术文档", "部署文档", "sop", "runbook")
	operationalStages := selectorCountContains(lower, "部署", "回滚", "故障排查", "上线", "发布", "deployment", "rollback", "troubleshooting")
	if artifact && operationalStages >= 1 {
		decision.PreferredPool = PoolDocument
		decision.Confidence = math.Max(decision.Confidence, 0.80)
		decision.DecisionSource = DecisionSourceRule
		decision.MatchedRules = append(decision.MatchedRules, "selector_document_runbook_guard")
	}
}

func selectorContainsAny(value string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(value, term) {
			return true
		}
	}
	return false
}

func selectorCountContains(value string, terms ...string) int {
	count := 0
	for _, term := range terms {
		if strings.Contains(value, term) {
			count++
		}
	}
	return count
}

func providerForModelRef(model string, refs []ModelSelectorModelRef, fallback string) string {
	for _, ref := range refs {
		if canonicalModelID(ref.ModelID) == canonicalModelID(model) && strings.TrimSpace(ref.Platform) != "" {
			return strings.TrimSpace(ref.Platform)
		}
	}
	return fallback
}

func bestLiveAccountForModel(model string, accounts []ModelSelectorAccountInfo) (ModelSelectorAccountInfo, float64, float64, float64, float64, bool) {
	var best ModelSelectorAccountInfo
	bestRuntime := -1.0
	bestPriority := 0.0
	bestCost := 0.0
	bestTTFT := 0.0
	for _, account := range accounts {
		if account.AccountID <= 0 || !account.Schedulable || account.RateLimited || account.Overloaded || account.TempUnschedulable || accountQuotaExhausted(account) || !accountSupportsModel(account, model) {
			continue
		}
		load := math.Min(1, math.Max(0, float64(account.LoadRate)/100))
		if account.Concurrency > 0 {
			load = math.Max(load, math.Min(1, float64(account.CurrentConcurrency)/float64(account.Concurrency)))
		}
		waiting := 1.0 - math.Min(1, float64(account.WaitingCount)/float64(maxSelectorInt(account.Concurrency, 1)))
		errorScore := 1.0 - math.Min(1, math.Max(0, account.ErrorRateEWMA))
		priority := math.Min(1, math.Max(0, float64(account.Priority)/100))
		cost := selectorCostScore(account.RateMultiplier)
		latency, hasTTFT := selectorTTFTScore(account.TTFTEWMAMs)
		// Availability remains the dominant runtime concern. Cost and first-token
		// latency only refine the ordering among schedulable account candidates.
		runtime := 0.40*(1-load) + 0.15*waiting + 0.15*errorScore + 0.10*priority + 0.10*cost
		if hasTTFT {
			runtime += 0.10 * latency
		} else {
			// Some platforms do not yet emit TTFT. Renormalize rather than treating
			// missing telemetry as a slow account.
			runtime /= 0.90
		}
		if runtime > bestRuntime || (runtime == bestRuntime && account.AccountID < best.AccountID) {
			best, bestRuntime, bestPriority, bestCost, bestTTFT = account, runtime, priority, cost, account.TTFTEWMAMs
		}
	}
	return best, bestRuntime, bestPriority, bestCost, bestTTFT, bestRuntime >= 0
}

func selectorCostScore(rateMultiplier float64) float64 {
	// 0 is a valid TokenCloud rate multiplier and means a zero-cost account.
	// The curve keeps cost useful without allowing it to dominate availability.
	return 1 / (1 + math.Max(0, rateMultiplier))
}

func selectorTTFTScore(ttftMS float64) (float64, bool) {
	if ttftMS <= 0 {
		return 0, false
	}
	// 500 ms is treated as excellent; 3000 ms or slower receives zero.
	return math.Min(1, math.Max(0, (3000-ttftMS)/2500)), true
}

func accountSupportsModel(account ModelSelectorAccountInfo, model string) bool {
	for _, ref := range account.Models {
		if canonicalModelID(ref.ModelID) == canonicalModelID(model) {
			return true
		}
	}
	return false
}

func accountQuotaExhausted(account ModelSelectorAccountInfo) bool {
	return (account.QuotaLimit > 0 && account.QuotaUsed >= account.QuotaLimit) ||
		(account.QuotaDailyLimit > 0 && account.QuotaDailyUsed >= account.QuotaDailyLimit) ||
		(account.QuotaWeeklyLimit > 0 && account.QuotaWeeklyUsed >= account.QuotaWeeklyLimit)
}

func maxSelectorInt(value, minimum int) int {
	if value < minimum {
		return minimum
	}
	return value
}

func firstModelID(models []string) string {
	for _, model := range models {
		if model = strings.TrimSpace(model); model != "" {
			return model
		}
	}
	return ""
}

func (h *ModelSelectorHTTPHandler) runSelection(ctx context.Context, requestID string, groupID int64, prompt, systemPrompt string, models []string) (*ModelSelectionResponse, error) {
	if h.service == nil {
		return nil, fmt.Errorf("model selector service is not configured")
	}
	if requestID == "" {
		requestID = fmt.Sprintf("selector-%d", time.Now().UnixNano())
	}
	if groupID == 0 {
		groupID = 1
	}
	selection := h.service.Select(ctx, &ModelSelectionRequest{ProtocolVersion: ModelSelectorProtocolVersion, RequestID: requestID, GroupID: groupID, Prompt: prompt, SystemPrompt: systemPrompt, ModelIDs: models})
	if selection == nil || !selection.Success {
		if selection != nil && selection.Error != "" {
			return nil, fmt.Errorf("selection failed: %s", selection.Error)
		}
		return nil, fmt.Errorf("selection failed")
	}
	return selection, nil
}

func semanticsForSelection(response *ModelSelectionResponse, official *officialVLLMScore) []ModelSelectorSemanticScore {
	// semantics is intentionally restricted to comparable semantic scores. Do
	// not mix routing heuristics (tier/pool), confidence, or latency into this
	// array: TokenCloud treats each entry as a model-selection feature.
	semantics := make([]ModelSelectorSemanticScore, 0)
	if official == nil {
		return semantics
	}
	keys := make([]string, 0, len(official.SignalValues))
	for key := range official.SignalValues {
		if strings.Count(key, ":") == 1 && strings.HasPrefix(key, "embedding:semantic_") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		semantics = append(semantics, ModelSelectorSemanticScore{Dimension: "official_vllm_" + strings.TrimPrefix(key, "embedding:"), Score: roundSelectorScore(official.SignalValues[key]), Description: "Official vLLM score-only semantic classification (shadow only)"})
	}
	return semantics
}

func preferredPoolFromOfficialScore(score *officialVLLMScore) (PreferredPool, float64, bool) {
	if score == nil {
		return "", 0, false
	}
	pool, ok := officialSignalPool(score.TopSignal)
	return pool, score.TopScore, ok
}

func officialSignalPool(signal string) (PreferredPool, bool) {
	switch signal {
	case "embedding:semantic_code":
		return PoolCode, true
	case "embedding:semantic_data_analysis":
		return PoolData, true
	case "embedding:semantic_document":
		return PoolDocument, true
	case "embedding:semantic_vision_understanding":
		return PoolVision, true
	case "embedding:semantic_image_generation":
		return PoolImageGeneration, true
	case "embedding:semantic_simple_chat":
		return PoolCheap, true
	case "embedding:semantic_general":
		return PoolDefault, true
	default:
		return "", false
	}
}

const (
	officialOverrideMinimumScore  = 0.68
	officialOverrideMinimumMargin = 0.10
	localProfessionalProtection   = 0.50
)

// applyConservativeOfficialPoolShadow keeps Official vLLM observational unless
// it provides clear evidence that a low-confidence local fallback is wrong.
// It prevents a narrow 0.55 top score from replacing a local data/code/document
// decision, which was the source of the observed Pool calibration failures.
func applyConservativeOfficialPoolShadow(decision *MultiLayerDecision, official *officialVLLMScore) {
	if decision == nil {
		return
	}
	officialPool, score, ok := preferredPoolFromOfficialScore(official)
	if !ok {
		return
	}
	if officialPool == decision.PreferredPool {
		decision.Confidence = math.Max(decision.Confidence, score)
		return
	}
	if isProfessionalPool(decision.PreferredPool) && decision.Confidence >= localProfessionalProtection {
		return
	}
	if score < officialOverrideMinimumScore || officialPoolMargin(official) < officialOverrideMinimumMargin {
		return
	}
	decision.PreferredPool = officialPool
	decision.Confidence = score
	decision.DecisionSource = DecisionSourceSemantic
	decision.FallbackReason = "official_vllm_clear_shadow_signal"
}

func isProfessionalPool(pool PreferredPool) bool {
	switch pool {
	case PoolCode, PoolData, PoolDocument, PoolVision, PoolImageGeneration:
		return true
	default:
		return false
	}
}

func officialPoolMargin(official *officialVLLMScore) float64 {
	if official == nil {
		return 0
	}
	best, second := 0.0, 0.0
	for signal, score := range official.SignalValues {
		if _, ok := officialSignalPool(signal); !ok {
			continue
		}
		if score > best {
			best, second = score, best
		} else if score > second {
			second = score
		}
	}
	return best - second
}

func officialScoreForPool(official *officialVLLMScore, pool PreferredPool) float64 {
	if official == nil {
		return 0
	}
	for signal, score := range official.SignalValues {
		if mapped, ok := officialSignalPool(signal); ok && mapped == pool {
			return score
		}
	}
	return 0
}

func modelScoresForSelection(response *ModelSelectionResponse, models []string) []ModelSelectorModelScore {
	byModel := make(map[string]float64, len(response.CandidateDetails))
	for _, candidate := range response.CandidateDetails {
		if candidate.FinalScore > byModel[candidate.ModelID] {
			byModel[candidate.ModelID] = candidate.FinalScore
		}
	}
	scores := make([]ModelSelectorModelScore, 0, len(models))
	for _, model := range models {
		if model = strings.TrimSpace(model); model != "" {
			scores = append(scores, ModelSelectorModelScore{ModelID: model, Score: roundSelectorScore(byModel[model])})
		}
	}
	sort.SliceStable(scores, func(i, j int) bool {
		if scores[i].Score == scores[j].Score {
			return scores[i].ModelID < scores[j].ModelID
		}
		return scores[i].Score > scores[j].Score
	})
	return scores
}

func roundSelectorScore(value float64) float64 {
	return math.Round(value*10000) / 10000
}

func DecodeModelSelectorUserAPICall(encoded string) ([]byte, error) {
	compressed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(io.LimitReader(reader, MaxIntegrationFrameBytes))
}

func EncodeModelSelectorUserAPICall(raw []byte) (string, error) {
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := writer.Write(raw); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buffer.Bytes()), nil
}

func promptFromUserAPICall(raw []byte) (string, string) {
	var call struct {
		System   json.RawMessage `json:"system"`
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if json.Unmarshal(raw, &call) != nil {
		return "", ""
	}
	system := rawContentText(call.System)
	var lastUser string
	for _, message := range call.Messages {
		text := rawContentText(message.Content)
		if text == "" {
			continue
		}
		if message.Role == "system" && system == "" {
			system = text
		} else if message.Role == "user" {
			// TokenCloud can forward a long multi-turn agent transcript. The last
			// user turn is the current task; using earlier turns or system context
			// dilutes the score-only classifier and causes avoidable timeouts.
			lastUser = text
		}
	}
	return lastUser, system
}

func rawContentText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return strings.TrimSpace(text)
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		values := make([]string, 0, len(parts))
		for _, part := range parts {
			if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
				values = append(values, strings.TrimSpace(part.Text))
			}
		}
		return strings.Join(values, "\n")
	}
	return ""
}

func (h *ModelSelectorHTTPHandler) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeResponse(w, http.StatusMethodNotAllowed, false, "method not allowed", nil)
		return
	}
	if !h.authorized(r) {
		h.writeResponse(w, http.StatusUnauthorized, false, "selector authentication failed", nil)
		return
	}
	h.metrics.recordHeartbeat()
	groupCount, modelCount := h.catalog.stats()
	status := h.metrics.snapshot(h.config.Version, h.vllm != nil, h.config.StatusMaxConcurrent, groupCount, modelCount, h.keyGroups.count())
	h.writeResponse(w, http.StatusOK, true, "ok", ModelSelectorHeartbeatData{
		Status: "healthy", Version: h.config.Version, Timestamp: time.Now().Unix(), Load: status.Load,
	})
}

func (h *ModelSelectorHTTPHandler) authorized(r *http.Request) bool {
	return h.config.SelectorSecret == "" || r.Header.Get("X-Selector-Secret") == h.config.SelectorSecret
}

func (h *ModelSelectorHTTPHandler) writeResponse(w http.ResponseWriter, status int, success bool, message string, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(modelSelectorHTTPResponse{Success: success, Code: status, Message: message, Data: data})
}
