package semanticrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

type fakeOfficialVLLMScorer struct {
	result officialVLLMScore
	err    error
}

func (s fakeOfficialVLLMScorer) Score(context.Context, string) (officialVLLMScore, error) {
	return s.result, s.err
}

func TestModelSelectorHeartbeatV12(t *testing.T) {
	mux := http.NewServeMux()
	NewModelSelectorHTTPHandler(ModelSelectorHTTPConfig{Version: "1.2.0", SelectorSecret: "test-secret"}).Register(mux)
	req := httptest.NewRequest(http.MethodGet, "/v1/model-selector/heartbeat", nil)
	req.Header.Set("X-Request-ID", "heartbeat-test-1")
	req.Header.Set("X-Selector-Secret", "test-secret")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Success bool                       `json:"success"`
		Code    int                        `json:"code"`
		Data    ModelSelectorHeartbeatData `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Success || body.Code != http.StatusOK || body.Data.Status != "healthy" || body.Data.Timestamp == 0 {
		t.Fatalf("unexpected heartbeat body: %+v", body)
	}
}

func TestModelSelectorHeartbeatRejectsInvalidSecret(t *testing.T) {
	mux := http.NewServeMux()
	NewModelSelectorHTTPHandler(ModelSelectorHTTPConfig{SelectorSecret: "test-secret"}).Register(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/model-selector/heartbeat", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestModelSelectorSelectV12ReturnsAllRequestedModels(t *testing.T) {
	service, err := NewModelSelectionService(NewDefaultRealSchedulerDryRun(), DefaultIntegrationConfig())
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	NewModelSelectorHTTPHandler(ModelSelectorHTTPConfig{SelectorSecret: "test-secret"}, service).Register(mux)
	raw := []byte(`{"model":"gpt-5.4","messages":[{"role":"system","content":"You are helpful."},{"role":"user","content":"请用 Go 实现用户登录 API"}]}`)
	encoded, err := EncodeModelSelectorUserAPICall(raw)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(ModelSelectorSelectHTTPRequest{UserAPICall: encoded, ModelList: []string{"gpt-5.4", "gemini-2.5-pro", "gpt-5.4-low"}})
	req := httptest.NewRequest(http.MethodPost, "/v1/model-selector/select", bytes.NewReader(body))
	req.Header.Set("X-Request-ID", "select-test-1")
	req.Header.Set("X-Selector-Secret", "test-secret")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var result struct {
		Success bool                            `json:"success"`
		Data    ModelSelectorSelectHTTPResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Success || result.Data.UserAPICall != encoded || len(result.Data.ModelScoreList) != 3 {
		t.Fatalf("unexpected response: %+v", result)
	}
	if result.Data.LocalRouting.PreferredPool != string(PoolCode) || result.Data.LocalRouting.PreferredTier == "" || !result.Data.LocalRouting.ShadowOnly || result.Data.LocalRouting.UpstreamCalled {
		t.Fatalf("local routing missing or unsafe: %+v", result.Data.LocalRouting)
	}
	decoded, err := DecodeModelSelectorUserAPICall(result.Data.UserAPICall)
	if err != nil || string(decoded) != string(raw) {
		t.Fatalf("user_api_call was not preserved: decoded=%s err=%v", decoded, err)
	}
}

func TestModelSelectorSelectV12SeparatesLocalRoutingFromOfficialSemantics(t *testing.T) {
	mux := http.NewServeMux()
	handler := NewModelSelectorHTTPHandler(ModelSelectorHTTPConfig{SelectorSecret: "test-secret"})
	handler.vllm = fakeOfficialVLLMScorer{result: officialVLLMScore{TopSignal: "embedding:semantic_code", TopScore: 0.47, SignalValues: map[string]float64{"embedding:semantic_code": 0.47}}}
	handler.Register(mux)
	raw := []byte(`{"messages":[{"role":"user","content":"写一个冒泡算法的 C 语言程序"}]}`)
	encoded, err := EncodeModelSelectorUserAPICall(raw)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(ModelSelectorSelectHTTPRequest{UserAPICall: encoded, ModelList: []string{"gpt-5.4"}})
	req := httptest.NewRequest(http.MethodPost, "/v1/model-selector/select", bytes.NewReader(body))
	req.Header.Set("X-Selector-Secret", "test-secret")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, req)
	var result struct {
		Data ModelSelectorSelectHTTPResponse `json:"data"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil {
		t.Fatalf("body=%s", response.Body.String())
	}
	if result.Data.LocalRouting.PreferredPool != string(PoolCode) || result.Data.LocalRouting.Confidence <= 0 {
		t.Fatalf("local router result=%+v", result.Data.LocalRouting)
	}
	if len(result.Data.Semantics) != 1 || result.Data.Semantics[0].Dimension != "official_vllm_semantic_code" {
		t.Fatalf("official semantics=%+v", result.Data.Semantics)
	}
}

func TestModelSelectorSelectV12IncludesOfficialVLLMSemanticScores(t *testing.T) {
	mux := http.NewServeMux()
	handler := NewModelSelectorHTTPHandler(ModelSelectorHTTPConfig{SelectorSecret: "test-secret"})
	handler.vllm = fakeOfficialVLLMScorer{result: officialVLLMScore{
		TopSignal: "embedding:semantic_data_analysis", TopScore: 0.91, LatencyMS: 123,
		SignalValues: map[string]float64{"embedding:semantic_data_analysis": 0.91, "embedding:semantic_code": 0.44},
	}}
	handler.Register(mux)
	raw := []byte(`{"messages":[{"role":"user","content":"Analyze sales trends from this CSV"}]}`)
	encoded, err := EncodeModelSelectorUserAPICall(raw)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(ModelSelectorSelectHTTPRequest{UserAPICall: encoded, ModelList: []string{"gpt-5.4", "gemini-2.5-flash"}})
	req := httptest.NewRequest(http.MethodPost, "/v1/model-selector/select", bytes.NewReader(body))
	req.Header.Set("X-Selector-Secret", "test-secret")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, req)
	var result struct {
		Success bool                            `json:"success"`
		Data    ModelSelectorSelectHTTPResponse `json:"data"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil || !result.Success {
		t.Fatalf("unexpected response: status=%d body=%s", response.Code, response.Body.String())
	}
	if len(result.Data.ModelScoreList) != 2 {
		t.Fatalf("model scores=%+v", result.Data.ModelScoreList)
	}
	if len(result.Data.Semantics) != 2 || !hasSemanticDimension(result.Data.Semantics, "official_vllm_semantic_data_analysis", 0.91) {
		t.Fatalf("official semantic score missing: %+v", result.Data.Semantics)
	}
}

func TestOfficialPoolShadowDoesNotOverrideConfidentProfessionalLocalPool(t *testing.T) {
	decision := &MultiLayerDecision{PreferredPool: PoolData, Confidence: 0.82, DecisionSource: DecisionSourceRule}
	official := &officialVLLMScore{TopSignal: "embedding:semantic_document", TopScore: 0.58, SignalValues: map[string]float64{
		"embedding:semantic_document":      0.58,
		"embedding:semantic_data_analysis": 0.54,
	}}
	applyConservativeOfficialPoolShadow(decision, official)
	if decision.PreferredPool != PoolData || decision.Confidence != 0.82 {
		t.Fatalf("weak Official signal must not override confident data pool: %+v", decision)
	}
}

func TestOfficialPoolShadowOverridesOnlyClearLowConfidenceFallback(t *testing.T) {
	decision := &MultiLayerDecision{PreferredPool: PoolDefault, Confidence: 0.22, DecisionSource: DecisionSourceFallback}
	official := &officialVLLMScore{TopSignal: "embedding:semantic_code", TopScore: 0.82, SignalValues: map[string]float64{
		"embedding:semantic_code":    0.82,
		"embedding:semantic_general": 0.56,
	}}
	applyConservativeOfficialPoolShadow(decision, official)
	if decision.PreferredPool != PoolCode || decision.Confidence != 0.82 || decision.FallbackReason != "official_vllm_clear_shadow_signal" {
		t.Fatalf("clear Official signal should correct low-confidence fallback: %+v", decision)
	}
}

func TestOfficialScoreUsesFinalLocalPoolRatherThanUnrelatedTopSignal(t *testing.T) {
	official := &officialVLLMScore{TopSignal: "embedding:semantic_image_generation", TopScore: 0.81, SignalValues: map[string]float64{
		"embedding:semantic_image_generation": 0.81,
		"embedding:semantic_document":         0.41,
	}}
	if got := officialScoreForPool(official, PoolDocument); got != 0.41 {
		t.Fatalf("document score=%v, want matching official document signal", got)
	}
}

func TestSelectorDocumentRunbookGuardRequiresArtifactAndOperationalStage(t *testing.T) {
	decision := &MultiLayerDecision{PreferredPool: PoolCode, Confidence: 0.51, DecisionSource: DecisionSourceSemantic}
	applySelectorProfessionalTaskGuard(decision, "请编写一份完整的生产环境部署、回滚和故障排查运行手册。")
	if decision.PreferredPool != PoolDocument || decision.Confidence < 0.80 {
		t.Fatalf("runbook prompt must be document pool: %+v", decision)
	}

	codeDecision := &MultiLayerDecision{PreferredPool: PoolCode, Confidence: 0.70}
	applySelectorProfessionalTaskGuard(codeDecision, "这个部署脚本为什么运行失败，请帮我排查并修复代码。")
	if codeDecision.PreferredPool != PoolCode {
		t.Fatalf("ordinary deployment debugging must remain code: %+v", codeDecision)
	}
}

func TestModelSelectorSelectV12RoundsOutboundScoresToFourDecimals(t *testing.T) {
	mux := http.NewServeMux()
	handler := NewModelSelectorHTTPHandler(ModelSelectorHTTPConfig{SelectorSecret: "test-secret"})
	handler.vllm = fakeOfficialVLLMScorer{result: officialVLLMScore{
		TopSignal: "embedding:semantic_code", TopScore: 0.87654321,
		SignalValues: map[string]float64{"embedding:semantic_code": 0.87654321},
	}}
	handler.Register(mux)
	raw := []byte(`{"messages":[{"role":"user","content":"Write a Go API"}]}`)
	encoded, err := EncodeModelSelectorUserAPICall(raw)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(ModelSelectorSelectHTTPRequest{UserAPICall: encoded, ModelList: []string{"gpt-5.4"}})
	req := httptest.NewRequest(http.MethodPost, "/v1/model-selector/select", bytes.NewReader(body))
	req.Header.Set("X-Selector-Secret", "test-secret")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, req)
	var result struct {
		Data ModelSelectorSelectHTTPResponse `json:"data"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil {
		t.Fatalf("body=%s", response.Body.String())
	}
	if got := result.Data.Semantics[0].Score; got != 0.8765 {
		t.Fatalf("semantic score=%v", got)
	}
	if got := result.Data.ModelScoreList[0].Score; got != math.Round(got*10000)/10000 {
		t.Fatalf("model score was not rounded: %v", got)
	}
}

func TestModelSelectorV13AccountSnapshotExcludesUnavailableAccounts(t *testing.T) {
	response := profileOnlyHTTPSelectionWithAccounts(
		"请用 Go 写一个贪吃蛇游戏", []string{"gpt-5.4", "gemini-2.5-flash"}, nil,
		[]ModelSelectorModelRef{{ModelID: "gpt-5.4", Platform: "openai"}, {ModelID: "gemini-2.5-flash", Platform: "gemini"}},
		[]ModelSelectorAccountInfo{
			{AccountID: 0, Schedulable: true, Models: []ModelSelectorModelRef{{ModelID: "gpt-5.4"}}},
			{AccountID: 101, Schedulable: false, Models: []ModelSelectorModelRef{{ModelID: "gpt-5.4"}}},
			{AccountID: 102, Schedulable: true, Overloaded: true, Models: []ModelSelectorModelRef{{ModelID: "gpt-5.4"}}},
			{AccountID: 103, Schedulable: true, LoadFactor: 5, Concurrency: 5, CurrentConcurrency: 1, WaitingCount: 0, LoadRate: 20, Priority: 50, Models: []ModelSelectorModelRef{{ModelID: "gpt-5.4"}}},
			{AccountID: 104, Schedulable: true, LoadFactor: 5, Concurrency: 5, CurrentConcurrency: 5, WaitingCount: 2, LoadRate: 100, Priority: 100, Models: []ModelSelectorModelRef{{ModelID: "gemini-2.5-flash"}}},
		},
	)
	if response.SchedulerSource != "profile_shadow_account_aware_v1.3" || !response.ShadowOnly || response.UpstreamCalled {
		t.Fatalf("unsafe v1.3 response=%+v", response)
	}
	byModel := make(map[string]ModelCandidateScore, len(response.CandidateDetails))
	for _, detail := range response.CandidateDetails {
		byModel[detail.ModelID] = detail
	}
	if got := byModel["gpt-5.4"].AccountID; got != 103 {
		t.Fatalf("gpt account=%d, want only eligible account 103; details=%+v", got, response.CandidateDetails)
	}
	if got := byModel["gemini-2.5-flash"].AccountID; got != 104 {
		t.Fatalf("gemini account=%d, want 104", got)
	}
	if response.ModelRanking == nil || response.ModelRanking.RecommendedAccountID != 103 || response.ModelRanking.RecommendedAccountID == 0 {
		t.Fatalf("ranking must recommend a real eligible account: %+v", response.ModelRanking)
	}
}

func TestModelSelectorV13AccountSnapshotExhaustedQuotaGetsNoScore(t *testing.T) {
	response := profileOnlyHTTPSelectionWithAccounts("请实现登录接口", []string{"gpt-5.4"}, nil, nil, []ModelSelectorAccountInfo{{
		AccountID: 201, Schedulable: true, QuotaLimit: 10, QuotaUsed: 10, Models: []ModelSelectorModelRef{{ModelID: "gpt-5.4"}},
	}})
	if len(response.CandidateDetails) != 1 || response.CandidateDetails[0].AccountID != 0 || response.CandidateDetails[0].FinalScore != 0 {
		t.Fatalf("quota exhausted account was retained: %+v", response.CandidateDetails)
	}
}

func TestModelSelectorV13RuntimeScoreUsesCostAndTTFTWithoutOverridingAvailability(t *testing.T) {
	// Both accounts are equally schedulable and equally loaded. The lower cost
	// and faster TTFT account should break the tie.
	accounts := []ModelSelectorAccountInfo{
		{AccountID: 301, Schedulable: true, Concurrency: 10, CurrentConcurrency: 2, LoadRate: 20, Priority: 50, RateMultiplier: 2, TTFTEWMAMs: 2500, Models: []ModelSelectorModelRef{{ModelID: "gpt-5.4"}}},
		{AccountID: 302, Schedulable: true, Concurrency: 10, CurrentConcurrency: 2, LoadRate: 20, Priority: 50, RateMultiplier: 0.5, TTFTEWMAMs: 500, Models: []ModelSelectorModelRef{{ModelID: "gpt-5.4"}}},
	}
	account, runtime, _, cost, ttft, ok := bestLiveAccountForModel("gpt-5.4", accounts)
	if !ok || account.AccountID != 302 || runtime <= 0 || cost <= 0 || ttft != 500 {
		t.Fatalf("cost/TTFT account selection=%+v runtime=%v cost=%v ttft=%v ok=%t", account, runtime, cost, ttft, ok)
	}
	if score, observed := selectorTTFTScore(0); observed || score != 0 {
		t.Fatalf("missing TTFT must be neutral/absent, score=%v observed=%t", score, observed)
	}
}

func TestModelSelectorSelectV12FallsBackWhenOfficialVLLMFails(t *testing.T) {
	mux := http.NewServeMux()
	handler := NewModelSelectorHTTPHandler(ModelSelectorHTTPConfig{SelectorSecret: "test-secret"})
	handler.vllm = fakeOfficialVLLMScorer{err: errors.New("timeout")}
	handler.Register(mux)
	raw := []byte(`{"messages":[{"role":"user","content":"Write a Go API"}]}`)
	encoded, err := EncodeModelSelectorUserAPICall(raw)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(ModelSelectorSelectHTTPRequest{UserAPICall: encoded, ModelList: []string{"gpt-5.4", "gemini-2.5-flash"}})
	req := httptest.NewRequest(http.MethodPost, "/v1/model-selector/select", bytes.NewReader(body))
	req.Header.Set("X-Selector-Secret", "test-secret")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, req)
	var result struct {
		Success bool                            `json:"success"`
		Data    ModelSelectorSelectHTTPResponse `json:"data"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil || !result.Success || len(result.Data.ModelScoreList) != 2 {
		t.Fatalf("official failure must preserve selector response: status=%d body=%s", response.Code, response.Body.String())
	}
	for _, semantic := range result.Data.Semantics {
		if strings.HasPrefix(semantic.Dimension, "official_vllm_") {
			t.Fatalf("unexpected official score during fallback: %+v", result.Data.Semantics)
		}
	}
}

func TestModelSelectorHistoryPreservesDecodedAuditAndRedactsSecrets(t *testing.T) {
	mux := http.NewServeMux()
	handler := NewModelSelectorHTTPHandler(ModelSelectorHTTPConfig{SelectorSecret: "test-secret"})
	handler.Register(mux)
	raw := []byte(`{"api_key":"must-not-appear","messages":[{"role":"user","content":"请分析 CSV 销售趋势"}]}`)
	encoded, err := EncodeModelSelectorUserAPICall(raw)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(ModelSelectorSelectHTTPRequest{UserAPICall: encoded, ModelList: []string{"gpt-5.4", "gemini-2.5-flash"}})
	selectRequest := httptest.NewRequest(http.MethodPost, "/v1/model-selector/select", bytes.NewReader(body))
	selectRequest.Header.Set("X-Request-ID", "history-test-1")
	selectRequest.Header.Set("X-Selector-Secret", "test-secret")
	selectResponse := httptest.NewRecorder()
	mux.ServeHTTP(selectResponse, selectRequest)
	if selectResponse.Code != http.StatusOK {
		t.Fatalf("select status=%d body=%s", selectResponse.Code, selectResponse.Body.String())
	}

	historyRequest := httptest.NewRequest(http.MethodGet, "/v1/model-selector/history?limit=10", nil)
	historyResponse := httptest.NewRecorder()
	mux.ServeHTTP(historyResponse, historyRequest)
	if historyResponse.Code != http.StatusOK {
		t.Fatalf("history status=%d body=%s", historyResponse.Code, historyResponse.Body.String())
	}
	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Records []ModelSelectorHistoryEntry `json:"records"`
		} `json:"data"`
	}
	if err := json.Unmarshal(historyResponse.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Success || len(result.Data.Records) != 1 {
		t.Fatalf("history=%+v", result)
	}
	record := result.Data.Records[0]
	if record.RequestID != "history-test-1" || record.PromptPreview != "请分析 CSV 销售趋势" || len(record.ModelScoreList) != 2 {
		t.Fatalf("unexpected record=%+v", record)
	}
	if record.DecodedRequest["api_key"] != "[redacted]" || !record.ShadowOnly || record.UpstreamCalled {
		t.Fatalf("unsafe history record=%+v", record)
	}
}

func TestModelSelectorHistorySurvivesHandlerRestart(t *testing.T) {
	historyFile := filepath.Join(t.TempDir(), "selector_history.jsonl")
	handler := NewModelSelectorHTTPHandler(ModelSelectorHTTPConfig{SelectorSecret: "test-secret", HistoryFile: historyFile})
	raw := []byte(`{"messages":[{"role":"user","content":"write a Go API"}]}`)
	encoded, err := EncodeModelSelectorUserAPICall(raw)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(ModelSelectorSelectHTTPRequest{UserAPICall: encoded, ModelList: []string{"gpt-5.4"}})
	mux := http.NewServeMux()
	handler.Register(mux)
	request := httptest.NewRequest(http.MethodPost, "/v1/model-selector/select", bytes.NewReader(body))
	request.Header.Set("X-Selector-Secret", "test-secret")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("select status=%d", response.Code)
	}

	restarted := NewModelSelectorHTTPHandler(ModelSelectorHTTPConfig{SelectorSecret: "test-secret", HistoryFile: historyFile})
	if got := len(restarted.history.recent(10)); got != 1 {
		t.Fatalf("persisted history records=%d, want 1", got)
	}
}

func TestModelSelectorSyncModelsEnforcesGroupBoundary(t *testing.T) {
	mux := http.NewServeMux()
	handler := NewModelSelectorHTTPHandler(ModelSelectorHTTPConfig{SelectorSecret: "test-secret", ModelCatalogFile: filepath.Join(t.TempDir(), "models.json")})
	handler.Register(mux)
	replace := true
	syncBody, _ := json.Marshal(ModelSelectorSyncHTTPRequest{
		GroupID: 2006, Platform: "domestic", Replace: &replace,
		Models: []ModelSelectorSyncedModel{{ModelID: "Qwen3.6-35B-A3B", SupportsThinking: true}, {ModelID: "DeepSeek-V4-flash", SupportsStreaming: true}},
	})
	syncRequest := httptest.NewRequest(http.MethodPost, "/v1/model-selector/sync-models", bytes.NewReader(syncBody))
	syncRequest.Header.Set("X-Selector-Secret", "test-secret")
	syncResponse := httptest.NewRecorder()
	mux.ServeHTTP(syncResponse, syncRequest)
	if syncResponse.Code != http.StatusOK {
		t.Fatalf("sync status=%d body=%s", syncResponse.Code, syncResponse.Body.String())
	}

	encoded, err := EncodeModelSelectorUserAPICall([]byte(`{"messages":[{"role":"user","content":"帮我写一份项目方案文档"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	validBody, _ := json.Marshal(ModelSelectorSelectHTTPRequest{UserAPICall: encoded, GroupID: 2006, ModelList: []string{"Qwen3.6-35B-A3B", "DeepSeek-V4-flash"}})
	validRequest := httptest.NewRequest(http.MethodPost, "/v1/model-selector/select", bytes.NewReader(validBody))
	validRequest.Header.Set("X-Selector-Secret", "test-secret")
	validResponse := httptest.NewRecorder()
	mux.ServeHTTP(validResponse, validRequest)
	if validResponse.Code != http.StatusOK {
		t.Fatalf("group-bound select status=%d body=%s", validResponse.Code, validResponse.Body.String())
	}

	crossGroupBody, _ := json.Marshal(ModelSelectorSelectHTTPRequest{UserAPICall: encoded, GroupID: 2006, ModelList: []string{"Qwen3.6-35B-A3B", "gpt-5.5"}})
	crossGroupRequest := httptest.NewRequest(http.MethodPost, "/v1/model-selector/select", bytes.NewReader(crossGroupBody))
	crossGroupRequest.Header.Set("X-Selector-Secret", "test-secret")
	crossGroupResponse := httptest.NewRecorder()
	mux.ServeHTTP(crossGroupResponse, crossGroupRequest)
	if crossGroupResponse.Code != http.StatusBadRequest || !strings.Contains(crossGroupResponse.Body.String(), "outside synchronized group_id 2006") {
		t.Fatalf("cross-group request must be rejected: status=%d body=%s", crossGroupResponse.Code, crossGroupResponse.Body.String())
	}
}

func TestModelSelectorSyncedCatalogSurvivesRestart(t *testing.T) {
	catalogFile := filepath.Join(t.TempDir(), "models.json")
	catalog := newModelSelectorSyncedCatalog(catalogFile)
	replace := true
	if _, err := catalog.sync(ModelSelectorSyncHTTPRequest{GroupID: 88, Platform: "qwen", Replace: &replace, Models: []ModelSelectorSyncedModel{{ModelID: "Qwen3.5-397B-A17B"}}}); err != nil {
		t.Fatal(err)
	}
	restarted := newModelSelectorSyncedCatalog(catalogFile)
	if _, err := restarted.requireGroupModels(88, []string{"qwen3.5-397b-a17b"}); err != nil {
		t.Fatalf("persisted catalog did not enforce valid group model: %v", err)
	}
	if _, err := restarted.requireGroupModels(88, []string{"gpt-5.5"}); err == nil {
		t.Fatal("persisted catalog accepted a cross-group model")
	}
}

func TestModelSelectorAPIKeyGroupMappingResolvesAndRejectsConflicts(t *testing.T) {
	mux := http.NewServeMux()
	config := ModelSelectorHTTPConfig{
		SelectorSecret: "test-secret", ModelCatalogFile: filepath.Join(t.TempDir(), "models.json"),
		APIKeyGroupFile: filepath.Join(t.TempDir(), "api-key-groups.json"),
	}
	handler := NewModelSelectorHTTPHandler(config)
	handler.Register(mux)
	replace := true
	for _, group := range []struct {
		id       int64
		platform string
		model    string
	}{{77, "qwen", "Qwen3.6-35B-A3B"}, {78, "openai", "gpt-5.5"}} {
		body, _ := json.Marshal(ModelSelectorSyncHTTPRequest{GroupID: group.id, Platform: group.platform, Replace: &replace, Models: []ModelSelectorSyncedModel{{ModelID: group.model}}})
		request := httptest.NewRequest(http.MethodPost, "/v1/model-selector/sync-models", bytes.NewReader(body))
		request.Header.Set("X-Selector-Secret", "test-secret")
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("sync group %d status=%d body=%s", group.id, response.Code, response.Body.String())
		}
	}

	bindingBody, _ := json.Marshal(ModelSelectorSyncAPIKeyGroupHTTPRequest{APIKeyID: 901, GroupID: 77})
	bindingRequest := httptest.NewRequest(http.MethodPost, "/v1/model-selector/sync-api-key-group", bytes.NewReader(bindingBody))
	bindingRequest.Header.Set("X-Selector-Secret", "test-secret")
	bindingResponse := httptest.NewRecorder()
	mux.ServeHTTP(bindingResponse, bindingRequest)
	if bindingResponse.Code != http.StatusOK {
		t.Fatalf("binding status=%d body=%s", bindingResponse.Code, bindingResponse.Body.String())
	}

	encoded, err := EncodeModelSelectorUserAPICall([]byte(`{"messages":[{"role":"user","content":"请实现一个登录接口"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	validBody, _ := json.Marshal(ModelSelectorSelectHTTPRequest{UserAPICall: encoded, APIKeyID: 901, ModelList: []string{"Qwen3.6-35B-A3B"}})
	validRequest := httptest.NewRequest(http.MethodPost, "/v1/model-selector/select", bytes.NewReader(validBody))
	validRequest.Header.Set("X-Selector-Secret", "test-secret")
	validResponse := httptest.NewRecorder()
	mux.ServeHTTP(validResponse, validRequest)
	if validResponse.Code != http.StatusOK {
		t.Fatalf("api-key-only select status=%d body=%s", validResponse.Code, validResponse.Body.String())
	}
	var result struct {
		Data ModelSelectorSelectHTTPResponse `json:"data"`
	}
	if err := json.Unmarshal(validResponse.Body.Bytes(), &result); err != nil || result.Data.LocalRouting.GroupID != 77 || result.Data.LocalRouting.APIKeyID != 901 {
		t.Fatalf("resolved mapping=%+v err=%v", result.Data.LocalRouting, err)
	}

	conflictBody, _ := json.Marshal(ModelSelectorSelectHTTPRequest{UserAPICall: encoded, APIKeyID: 901, GroupID: 78, ModelList: []string{"gpt-5.5"}})
	conflictRequest := httptest.NewRequest(http.MethodPost, "/v1/model-selector/select", bytes.NewReader(conflictBody))
	conflictRequest.Header.Set("X-Selector-Secret", "test-secret")
	conflictResponse := httptest.NewRecorder()
	mux.ServeHTTP(conflictResponse, conflictRequest)
	if conflictResponse.Code != http.StatusConflict || !strings.Contains(conflictResponse.Body.String(), "conflicts with mapped") {
		t.Fatalf("mapping conflict status=%d body=%s", conflictResponse.Code, conflictResponse.Body.String())
	}
}

func TestModelSelectorAPIKeyGroupFirstValidSelectBootstrapsPersistentBinding(t *testing.T) {
	catalogFile := filepath.Join(t.TempDir(), "models.json")
	bindingFile := filepath.Join(t.TempDir(), "api-key-groups.json")
	handler := NewModelSelectorHTTPHandler(ModelSelectorHTTPConfig{SelectorSecret: "test-secret", ModelCatalogFile: catalogFile, APIKeyGroupFile: bindingFile})
	replace := true
	if _, err := handler.catalog.sync(ModelSelectorSyncHTTPRequest{GroupID: 88, Platform: "qwen", Replace: &replace, Models: []ModelSelectorSyncedModel{{ModelID: "Qwen3.5-397B-A17B"}}}); err != nil {
		t.Fatal(err)
	}
	encoded, err := EncodeModelSelectorUserAPICall([]byte(`{"messages":[{"role":"user","content":"分析销售数据"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	handler.Register(mux)
	body, _ := json.Marshal(ModelSelectorSelectHTTPRequest{UserAPICall: encoded, APIKeyID: 902, GroupID: 88, ModelList: []string{"Qwen3.5-397B-A17B"}})
	request := httptest.NewRequest(http.MethodPost, "/v1/model-selector/select", bytes.NewReader(body))
	request.Header.Set("X-Selector-Secret", "test-secret")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("bootstrap status=%d body=%s", response.Code, response.Body.String())
	}
	restarted := NewModelSelectorHTTPHandler(ModelSelectorHTTPConfig{SelectorSecret: "test-secret", ModelCatalogFile: catalogFile, APIKeyGroupFile: bindingFile})
	if groupID, ok := restarted.keyGroups.lookup(902); !ok || groupID != 88 {
		t.Fatalf("persisted api-key mapping group=%d ok=%t", groupID, ok)
	}
}

func TestModelSelectorStatusReportsSelectionAndOfficialFallbackMetrics(t *testing.T) {
	mux := http.NewServeMux()
	handler := NewModelSelectorHTTPHandler(ModelSelectorHTTPConfig{SelectorSecret: "test-secret", StatusMaxConcurrent: 2})
	handler.vllm = fakeOfficialVLLMScorer{err: errors.New("official timeout")}
	handler.Register(mux)
	encoded, err := EncodeModelSelectorUserAPICall([]byte(`{"messages":[{"role":"user","content":"写一个 Go 登录接口"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(ModelSelectorSelectHTTPRequest{UserAPICall: encoded, ModelList: []string{"gpt-5.4", "gemini-2.5-flash"}})
	selectRequest := httptest.NewRequest(http.MethodPost, "/v1/model-selector/select", bytes.NewReader(body))
	selectRequest.Header.Set("X-Selector-Secret", "test-secret")
	selectResponse := httptest.NewRecorder()
	mux.ServeHTTP(selectResponse, selectRequest)
	if selectResponse.Code != http.StatusOK {
		t.Fatalf("select status=%d body=%s", selectResponse.Code, selectResponse.Body.String())
	}

	heartbeatRequest := httptest.NewRequest(http.MethodGet, "/v1/model-selector/heartbeat", nil)
	heartbeatRequest.Header.Set("X-Selector-Secret", "test-secret")
	mux.ServeHTTP(httptest.NewRecorder(), heartbeatRequest)
	statusRequest := httptest.NewRequest(http.MethodGet, "/v1/model-selector/status", nil)
	statusRequest.Header.Set("X-Selector-Secret", "test-secret")
	statusResponse := httptest.NewRecorder()
	mux.ServeHTTP(statusResponse, statusRequest)
	if statusResponse.Code != http.StatusOK {
		t.Fatalf("status endpoint=%d body=%s", statusResponse.Code, statusResponse.Body.String())
	}
	var result struct {
		Success bool                    `json:"success"`
		Data    ModelSelectorStatusData `json:"data"`
	}
	if err := json.Unmarshal(statusResponse.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	status := result.Data
	if !result.Success || status.TotalSelections != 1 || status.SuccessfulSelections != 1 || status.ErrorSelections != 0 || status.OfficialVLLM.AttemptCount != 1 || status.OfficialVLLM.FallbackCount != 1 || status.OfficialVLLM.FallbackRate != 1 || status.LastHeartbeatAt == 0 {
		t.Fatalf("unexpected status=%+v", status)
	}
	if len(status.RecommendedModels) != 1 || status.RecommendedModels[0].ModelID == "" || status.CacheEnabled || status.CacheHitRate != 0 || !status.ShadowOnly || status.TakeoverEnabled {
		t.Fatalf("unsafe or incomplete status=%+v", status)
	}
}

func TestPromptFromUserAPICallUsesOnlyLatestUserTask(t *testing.T) {
	raw := []byte(`{
		"system":"long agent system context that must not be scored",
		"messages":[
			{"role":"user","content":"earlier unrelated question"},
			{"role":"assistant","content":"long previous answer"},
			{"role":"user","content":"请写一个冒泡算法的 C 语言程序"}
		]
	}`)
	prompt, system := promptFromUserAPICall(raw)
	if prompt != "请写一个冒泡算法的 C 语言程序" {
		t.Fatalf("prompt=%q, want latest user task", prompt)
	}
	if system != "long agent system context that must not be scored" {
		t.Fatalf("system=%q", system)
	}
}

func TestLocalRoutingClassifiesChineseAlgorithmImplementationAsCode(t *testing.T) {
	response := profileOnlyHTTPSelection("写一个冒泡算法的 C 语言程序", []string{"gpt-5.4"}, nil)
	local := localRoutingForSelection(response)
	if local.PreferredPool != string(PoolCode) || local.TaskType != string(TaskTypeCode) || local.Confidence <= 0.5 {
		t.Fatalf("local routing=%+v", local)
	}
}

func hasSemanticDimension(values []ModelSelectorSemanticScore, dimension string, score float64) bool {
	for _, value := range values {
		if value.Dimension == dimension && value.Score == score {
			return true
		}
	}
	return false
}
