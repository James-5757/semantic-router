package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	semanticrouter "semantic-router"
)

// ============================================================================
// Data types
// ============================================================================

// RunRecord represents a batch replay run
type RunRecord struct {
	RunID       string    `json:"run_id"`
	DatasetName string    `json:"dataset_name"`
	DatasetHash string    `json:"dataset_hash"`
	SourceType  string    `json:"source_type"` // "batch" or "single"
	TotalRows   int       `json:"total_rows"`
	CreatedAt   time.Time `json:"created_at"`
	Status      string    `json:"status"` // "running", "completed", "interrupted"
}

// RoutingRecord stores a single routing evaluation result
type RoutingRecord struct {
	ID         int64  `json:"id"`
	RunID      string `json:"run_id"`
	RowID      string `json:"row_id"`
	SourceType string `json:"source_type"` // "batch" or "single"

	// Input
	PromptHash    string `json:"prompt_hash"`
	PromptPreview string `json:"prompt_preview"`
	PromptText    string `json:"prompt_text,omitempty"`
	HasImage      bool   `json:"has_image"`
	HasDocument   bool   `json:"has_document"`
	HasCSV        bool   `json:"has_csv"`
	// ModelRanking contains the stable candidate view: rank, account_id, model, final_score.
	ModelRanking json.RawMessage `json:"model_ranking,omitempty"`

	// Intent
	PrimaryIntent string `json:"primary_intent"`

	// Local
	LocalProvider   string  `json:"local_provider"`
	LocalDecision   string  `json:"local_decision"`
	LocalCategory   string  `json:"local_category"`
	LocalPool       string  `json:"local_pool"`
	LocalConfidence float64 `json:"local_confidence"`

	// Official
	OfficialProvider   string  `json:"official_provider"`
	OfficialDecision   string  `json:"official_decision"`
	OfficialCategory   string  `json:"official_category"`
	OfficialPool       string  `json:"official_pool"`
	OfficialConfidence float64 `json:"official_confidence"`
	OfficialError      string  `json:"official_error,omitempty"`

	// Pool
	NormalizedPool         string `json:"normalized_pool"`
	PhysicalGroup          string `json:"physical_model_group"`
	OfficialPhysicalGroup  string `json:"official_physical_group"`
	PhysicalGroupAgreement bool   `json:"physical_group_agreement"`
	RouteLLMAgreement      bool   `json:"routellm_agreement"`

	// Tier
	ComplexityScore float64 `json:"complexity_score"`
	MinimumTier     string  `json:"minimum_tier"`
	RequestedTier   string  `json:"requested_tier"`
	SelectedTier    string  `json:"selected_tier"`

	// Hybrid
	HybridCandidatePool string  `json:"hybrid_candidate_pool"`
	HybridSource        string  `json:"hybrid_source"`
	HybridConfidence    float64 `json:"hybrid_confidence"`
	HybridReason        string  `json:"hybrid_reason"`
	HybridAbstain       bool    `json:"hybrid_abstain"`

	// Final
	FinalPool       string `json:"final_pool"`
	FinalPoolSource string `json:"final_pool_source"`

	// Agreement
	SemanticAgreement bool `json:"semantic_agreement"`
	PoolAgreement     bool `json:"pool_agreement"`

	// Runtime
	TotalLatencyMs float64 `json:"total_latency_ms"`
	Error          string  `json:"error,omitempty"`
	DryRun         bool    `json:"dry_run"`
	UpstreamCalled bool    `json:"upstream_called"`

	// V2 Fields (parallel, shadow only)
	V2CapabilityWindowRequiresMultimodal bool    `json:"v2_capability_window_requires_multimodal,omitempty"`
	V2LocalDomain                        string  `json:"v2_local_domain,omitempty"`
	V2LocalGroup                         string  `json:"v2_local_group,omitempty"`
	V2LocalConfidence                    float64 `json:"v2_local_confidence,omitempty"`
	V2OfficialDomain                     string  `json:"v2_official_domain,omitempty"`
	V2OfficialGroup                      string  `json:"v2_official_group,omitempty"`
	V2GroupAgreement                     bool    `json:"v2_group_agreement,omitempty"`
	V2HybridTriggered                    bool    `json:"v2_hybrid_triggered,omitempty"`
	V2HybridCandidateGroup               string  `json:"v2_hybrid_candidate_group,omitempty"`
	V2ToolProfile                        string  `json:"v2_tool_profile,omitempty"`
	V2SelectedTier                       string  `json:"v2_selected_tier,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

// ReviewLabel records a human review of a routing record
type ReviewLabel struct {
	ID                int64     `json:"id"`
	RecordID          int64     `json:"record_id"`
	ExpectedIntent    string    `json:"expected_intent"`
	ExpectedPool      string    `json:"expected_pool"`
	ExpectedTier      string    `json:"expected_tier"`
	Ambiguous         bool      `json:"ambiguous"`
	ReviewConfidence  string    `json:"review_confidence"` // "high", "medium", "low"
	ReviewNote        string    `json:"review_note,omitempty"`
	Reviewer          string    `json:"reviewer,omitempty"`
	NeedsAdjudication bool      `json:"needs_adjudication"`
	LabelVersion      string    `json:"label_version"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// RecordFilter for querying routing records
type RecordFilter struct {
	RunID         string
	SourceType    string
	LocalPool     string
	OfficialPool  string
	PoolAgreement *bool
	Tier          string
	HasReview     *bool
	Ambiguous     *bool
	HasError      *bool
	Limit         int
	Offset        int
}

// ExportFilter for record export
type ExportFilter struct {
	RunID   string
	Format  string // "jsonl" or "csv"
	Limit   int
	Columns []string
}

// ============================================================================
// FileStore — JSONL-based persistent storage (no external dependencies)
// ============================================================================

// FileStore implements persistent storage using JSONL files
type FileStore struct {
	dir string
	mu  sync.RWMutex
}

// NewFileStore creates a new FileStore in the given directory
func NewFileStore(dir string) (*FileStore, error) {
	if dir == "" {
		dir = "router_store"
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create store dir: %w", err)
	}
	return &FileStore{dir: dir}, nil
}

// Close is a no-op for FileStore
func (s *FileStore) Close() error {
	return nil
}

// ============================================================================
// File paths
// ============================================================================

func (s *FileStore) recordsFile() string {
	return filepath.Join(s.dir, "routing_records.jsonl")
}

func (s *FileStore) runsFile() string {
	return filepath.Join(s.dir, "replay_runs.jsonl")
}

func (s *FileStore) reviewsFile() string {
	return filepath.Join(s.dir, "review_labels.jsonl")
}

// ============================================================================
// Run operations
// ============================================================================

// CreateRun persists a run record
func (s *FileStore) CreateRun(run *RunRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return appendJSONL(s.runsFile(), run)
}

// UpdateRunStatus updates a run's status by re-writing all runs
func (s *FileStore) UpdateRunStatus(runID, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	runs, err := readJSONL[RunRecord](s.runsFile())
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Re-write all runs with updated status
	f, err := os.Create(s.runsFile())
	if err != nil {
		return err
	}
	defer f.Close()

	for _, r := range runs {
		if r.RunID == runID {
			r.Status = status
		}
		b, _ := json.Marshal(r)
		f.Write(b)
		f.Write([]byte("\n"))
	}
	return nil
}

// ListRuns returns recent runs
func (s *FileStore) ListRuns(limit int) ([]RunRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	runs, err := readJSONL[RunRecord](s.runsFile())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	sort.Slice(runs, func(i, j int) bool {
		return runs[i].CreatedAt.After(runs[j].CreatedAt)
	})

	if limit <= 0 || limit > len(runs) {
		return runs, nil
	}
	return runs[:limit], nil
}

// ============================================================================
// Record operations
// ============================================================================

// InsertRecord appends a routing record (immediately persisted)
func (s *FileStore) InsertRecord(rec *RoutingRecord) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get next ID
	maxID, err := s.maxRecordID()
	if err != nil {
		return 0, err
	}
	rec.ID = maxID + 1
	rec.CreatedAt = time.Now()

	return rec.ID, appendJSONL(s.recordsFile(), rec)
}

// GetRecord returns a single record by ID
func (s *FileStore) GetRecord(id int64) (*RoutingRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	records, err := readJSONL[RoutingRecord](s.recordsFile())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("record %d not found", id)
		}
		return nil, err
	}
	for _, r := range records {
		if r.ID == id {
			rec := r
			return &rec, nil
		}
	}
	return nil, fmt.Errorf("record %d not found", id)
}

// GetRecordByRowID returns a record by row_id (first match)
func (s *FileStore) GetRecordByRowID(runID, rowID string) (*RoutingRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	records, err := readJSONL[RoutingRecord](s.recordsFile())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("record %s/%s not found", runID, rowID)
		}
		return nil, err
	}
	for _, r := range records {
		if r.RunID == runID && r.RowID == rowID {
			rec := r
			return &rec, nil
		}
	}
	return nil, fmt.Errorf("record %s/%s not found", runID, rowID)
}

// ListRecords returns records with optional filters, and total count
func (s *FileStore) ListRecords(filter RecordFilter) ([]RoutingRecord, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	allRecords, err := readJSONL[RoutingRecord](s.recordsFile())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, nil
		}
		return nil, 0, err
	}

	var filtered []RoutingRecord
	for _, r := range allRecords {
		if filter.RunID != "" && r.RunID != filter.RunID {
			continue
		}
		if filter.SourceType != "" && r.SourceType != filter.SourceType {
			continue
		}
		if filter.LocalPool != "" && r.LocalPool != filter.LocalPool {
			continue
		}
		if filter.OfficialPool != "" && r.OfficialPool != filter.OfficialPool {
			continue
		}
		if filter.PoolAgreement != nil && r.PoolAgreement != *filter.PoolAgreement {
			continue
		}
		if filter.Tier != "" && r.SelectedTier != filter.Tier {
			continue
		}
		if filter.HasError != nil {
			hasErr := r.Error != ""
			if hasErr != *filter.HasError {
				continue
			}
		}
		filtered = append(filtered, r)
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})

	// Recompute physical group agreement using current mapping to ensure consistency
	// even for older records that may have been saved with incorrect agreement values.
	var poolToGroup = map[string]string{
		"general_text_pool":     "general_chat_models",
		"cheap_chat_pool":       "general_chat_models",
		"code_pool":             "technical_models",
		"data_pool":             "technical_models",
		"document_pool":         "technical_models",
		"vision_pool":           "vision_models",
		"image_generation_pool": "image_models",
		"general_pool":          "general_chat_models",
	}
	for i := range filtered {
		localPhys := poolToGroup[filtered[i].LocalPool]
		if localPhys == "" {
			localPhys = "general_chat_models"
		}
		officialPhys := poolToGroup[filtered[i].OfficialPool]
		if officialPhys == "" {
			officialPhys = localPhys
		}
		filtered[i].PhysicalGroupAgreement = localPhys == officialPhys
		// Also ensure OfficialPhysicalGroup is set
		if filtered[i].OfficialPhysicalGroup == "" {
			filtered[i].OfficialPhysicalGroup = localPhys
		}
	}

	total := len(filtered)

	start := filter.Offset
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + filter.Limit
	if end > len(filtered) {
		end = len(filtered)
	}
	if filter.Limit <= 0 {
		return filtered, total, nil
	}
	return filtered[start:end], total, nil
}

// maxRecordID returns the highest ID across all records
func (s *FileStore) maxRecordID() (int64, error) {
	records, err := readJSONL[RoutingRecord](s.recordsFile())
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	var maxID int64
	for _, r := range records {
		if r.ID > maxID {
			maxID = r.ID
		}
	}
	return maxID, nil
}

// ============================================================================
// Review operations
// ============================================================================

// UpsertReview creates or updates a review label
func (s *FileStore) UpsertReview(review *ReviewLabel) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	reviews, err := readJSONL[ReviewLabel](s.reviewsFile())
	if err != nil && !os.IsNotExist(err) {
		return 0, err
	}

	now := time.Now()
	for i, r := range reviews {
		if r.RecordID == review.RecordID {
			reviews[i].ExpectedIntent = review.ExpectedIntent
			reviews[i].ExpectedPool = review.ExpectedPool
			reviews[i].ExpectedTier = review.ExpectedTier
			reviews[i].Ambiguous = review.Ambiguous
			reviews[i].ReviewConfidence = review.ReviewConfidence
			reviews[i].ReviewNote = review.ReviewNote
			reviews[i].Reviewer = review.Reviewer
			reviews[i].NeedsAdjudication = review.NeedsAdjudication
			reviews[i].LabelVersion = review.LabelVersion
			reviews[i].UpdatedAt = now
			if err := s.writeReviews(reviews); err != nil {
				return 0, err
			}
			return reviews[i].ID, nil
		}
	}

	maxID := int64(0)
	for _, r := range reviews {
		if r.ID > maxID {
			maxID = r.ID
		}
	}
	review.ID = maxID + 1
	review.CreatedAt = now
	review.UpdatedAt = now
	reviews = append(reviews, *review)

	return review.ID, s.writeReviews(reviews)
}

// GetReview returns the review for a record
func (s *FileStore) GetReview(recordID int64) (*ReviewLabel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	reviews, err := readJSONL[ReviewLabel](s.reviewsFile())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("review for record %d not found", recordID)
		}
		return nil, err
	}
	for _, r := range reviews {
		if r.RecordID == recordID {
			review := r
			return &review, nil
		}
	}
	return nil, fmt.Errorf("review for record %d not found", recordID)
}

// ListReviews returns all reviews
func (s *FileStore) ListReviews(limit int) ([]ReviewLabel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	reviews, err := readJSONL[ReviewLabel](s.reviewsFile())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	sort.Slice(reviews, func(i, j int) bool {
		return reviews[i].UpdatedAt.After(reviews[j].UpdatedAt)
	})

	if limit <= 0 || limit > len(reviews) {
		return reviews, nil
	}
	return reviews[:limit], nil
}

// writeReviews overwrites all reviews
func (s *FileStore) writeReviews(reviews []ReviewLabel) error {
	return writeJSONLLines(s.reviewsFile(), reviews)
}

// ============================================================================
// Export
// ============================================================================

// ExportRecords exports records in JSONL or CSV format
func (s *FileStore) ExportRecords(filter ExportFilter) (string, error) {
	records, _, err := s.ListRecords(RecordFilter{
		RunID: filter.RunID,
		Limit: filter.Limit,
	})
	if err != nil {
		return "", err
	}

	if filter.Format == "csv" {
		return s.recordsToCSV(records)
	}
	return s.recordsToJSONL(records)
}

func (s *FileStore) recordsToJSONL(records []RoutingRecord) (string, error) {
	var sb strings.Builder
	for _, r := range records {
		data, err := json.Marshal(r)
		if err != nil {
			return "", err
		}
		sb.Write(data)
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}

func (s *FileStore) recordsToCSV(records []RoutingRecord) (string, error) {
	var sb strings.Builder
	sb.WriteString("id,run_id,row_id,prompt_hash,prompt_preview,primary_intent,local_pool,local_confidence,official_pool,official_confidence,official_error,normalized_pool,minimum_tier,requested_tier,selected_tier,hybrid_candidate_pool,hybrid_source,final_pool,final_pool_source,semantic_agreement,pool_agreement,total_latency_ms,error\n")
	for _, r := range records {
		sb.WriteString(fmt.Sprintf("%d,%s,%s,%s,%s,%s,%s,%.4f,%s,%.4f,%s,%s,%s,%s,%s,%s,%s,%s,%s,%v,%v,%.1f,%s\n",
			r.ID, csvEsc(r.RunID), csvEsc(r.RowID), csvEsc(r.PromptHash), csvEsc(r.PromptPreview),
			csvEsc(r.PrimaryIntent), csvEsc(r.LocalPool), r.LocalConfidence,
			csvEsc(r.OfficialPool), r.OfficialConfidence, csvEsc(r.OfficialError),
			csvEsc(r.NormalizedPool),
			csvEsc(r.MinimumTier), csvEsc(r.RequestedTier), csvEsc(r.SelectedTier),
			csvEsc(r.HybridCandidatePool), csvEsc(r.HybridSource),
			csvEsc(r.FinalPool), csvEsc(r.FinalPoolSource),
			r.SemanticAgreement, r.PoolAgreement, r.TotalLatencyMs,
			csvEsc(r.Error),
		))
	}
	return sb.String(), nil
}

func csvEsc(s string) string {
	if strings.ContainsAny(s, ",\"\n") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}

// BuildRoutingRecordFromResponse constructs a RoutingRecord from an evaluate response
func BuildRoutingRecordFromResponse(req playgroundRequest, resp playgroundResponse, runID string, rowID string, totalLatencyMs float64, started time.Time) *RoutingRecord {
	rec := &RoutingRecord{
		RunID:      runID,
		RowID:      rowID,
		SourceType: "single",
		CreatedAt:  started,
	}

	rec.PromptHash = hashPrompt(req.Prompt)
	rec.PromptPreview = sanitizePrompt(req.Prompt, 80)
	rec.PromptText = req.Prompt
	rec.HasImage = req.HasImage
	rec.HasDocument = req.HasDocument
	rec.HasCSV = req.HasCSV
	rec.ModelRanking = buildSavedModelRanking(resp.Scheduler.CandidateDetails, resp.Scheduler.RecommendedModel, resp.Scheduler.RankingMargin)
	rec.PrimaryIntent = resp.TaskUnderstandingCard.PrimaryIntent

	// Local
	rec.LocalProvider = resp.LocalProviderResult.Provider
	rec.LocalDecision = resp.LocalProviderResult.DecisionName
	rec.LocalCategory = resp.LocalProviderResult.SemanticCategory
	rec.LocalPool = resp.LocalProviderResult.MappedPool
	rec.LocalConfidence = resp.LocalProviderResult.Confidence

	// Official
	rec.OfficialProvider = resp.OfficialProviderResult.Provider
	rec.OfficialDecision = resp.OfficialProviderResult.DecisionName
	rec.OfficialCategory = resp.OfficialProviderResult.SemanticCategory
	rec.OfficialPool = resp.OfficialProviderResult.MappedPool
	rec.OfficialConfidence = resp.OfficialProviderResult.Confidence
	rec.OfficialError = resp.OfficialProviderResult.Error

	// Pool
	rec.NormalizedPool = resp.PoolCard.LogicalPool
	rec.PhysicalGroup = resp.PoolCard.PhysicalModelGroup
	rec.PhysicalGroupAgreement = resp.PhysicalGroupAgreement

	// Compute official physical group from official provider pool
	var poolToGroup = map[string]string{
		"general_text_pool":     "general_chat_models",
		"cheap_chat_pool":       "general_chat_models",
		"code_pool":             "technical_models",
		"data_pool":             "technical_models",
		"document_pool":         "technical_models",
		"vision_pool":           "vision_models",
		"image_generation_pool": "image_models",
		"general_pool":          "general_chat_models",
	}
	officialPool := resp.OfficialProviderResult.MappedPool
	rec.OfficialPhysicalGroup = poolToGroup[officialPool]
	if rec.OfficialPhysicalGroup == "" {
		rec.OfficialPhysicalGroup = poolToGroup[resp.PoolCard.LogicalPool]
	}
	if rec.OfficialPhysicalGroup == "" {
		rec.OfficialPhysicalGroup = rec.PhysicalGroup
	}

	// Compute physical group agreement: both local and official pools
	// map to the same physical model group
	// If official pool cannot be mapped (e.g. default-route), fall back to agreed
	localPhys := poolToGroup[resp.LocalProviderResult.MappedPool]
	if localPhys == "" {
		localPhys = "general_chat_models"
	}
	officialPhys := poolToGroup[resp.OfficialProviderResult.MappedPool]
	if officialPhys == "" {
		officialPhys = localPhys
	}
	rec.PhysicalGroupAgreement = localPhys == officialPhys

	// Tier
	rec.ComplexityScore = resp.TierCard.ComplexityScore
	rec.MinimumTier = resp.TierCard.MinimumTier
	rec.RequestedTier = resp.TierCard.RequestedTier
	rec.SelectedTier = resp.TierCard.SelectedTier

	// Hybrid
	rec.HybridCandidatePool = resp.HybridPool.CandidatePool
	rec.HybridSource = resp.HybridPool.Source
	rec.HybridConfidence = resp.HybridPool.Confidence
	rec.HybridReason = resp.HybridPool.Reason
	rec.HybridAbstain = resp.HybridPool.Abstain

	// Final
	rec.FinalPool = resp.LocalProviderResult.MappedPool
	rec.FinalPoolSource = "local_router"

	// Agreement
	rec.SemanticAgreement = resp.SemanticAgreement
	rec.PoolAgreement = resp.PoolAgreement
	rec.PhysicalGroupAgreement = resp.PhysicalGroupAgreement
	rec.RouteLLMAgreement = resp.RouteLLMAgreement

	// Runtime
	rec.TotalLatencyMs = totalLatencyMs
	rec.DryRun = true
	rec.UpstreamCalled = false

	// V2 Fields
	if resp.V2Decision != nil {
		rec.V2CapabilityWindowRequiresMultimodal = resp.V2Decision.CapabilityWindow.RequiresMultimodal
		rec.V2LocalDomain = string(resp.V2Decision.Local.Domain)
		rec.V2LocalGroup = string(resp.V2Decision.Local.Group)
		rec.V2LocalConfidence = resp.V2Decision.Local.Confidence
		rec.V2OfficialDomain = string(resp.V2Decision.Official.Domain)
		rec.V2OfficialGroup = string(resp.V2Decision.Official.Group)
		rec.V2GroupAgreement = resp.V2Decision.Local.Group == resp.V2Decision.Official.Group
		rec.V2HybridTriggered = resp.V2Decision.Hybrid.Triggered
		rec.V2HybridCandidateGroup = string(resp.V2Decision.Hybrid.CandidateGroup)
		rec.V2ToolProfile = string(resp.V2Decision.ToolProfile.Primary)
		rec.V2SelectedTier = string(resp.V2Decision.Tier.SelectedTier)
	}

	return rec
}

func buildSavedModelRanking(details []semanticrouter.ModelCandidateScore, recommendedModel string, margin float64) json.RawMessage {
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
	type candidate struct {
		Rank       int     `json:"rank"`
		AccountID  int64   `json:"account_id"`
		Model      string  `json:"model"`
		FinalScore float64 `json:"final_score"`
	}
	ranking := struct {
		RecommendedModel string      `json:"recommended_model,omitempty"`
		RankingMargin    float64     `json:"ranking_margin"`
		Candidates       []candidate `json:"candidates"`
	}{RecommendedModel: recommendedModel, RankingMargin: margin, Candidates: make([]candidate, 0, len(sorted))}
	for i, detail := range sorted {
		ranking.Candidates = append(ranking.Candidates, candidate{Rank: i + 1, AccountID: detail.AccountID, Model: detail.ModelID, FinalScore: detail.FinalScore})
	}
	encoded, err := json.Marshal(ranking)
	if err != nil {
		return nil
	}
	return encoded
}

// ============================================================================
// Helpers
// ============================================================================

func readJSONL[T any](path string) ([]T, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var results []T
	decoder := json.NewDecoder(f)
	for {
		var item T
		if err := decoder.Decode(&item); err == io.EOF {
			break
		} else if err != nil {
			continue
		}
		results = append(results, item)
	}
	return results, nil
}

func appendJSONL(path string, v any) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = f.Write(data)
	return err
}

func writeJSONLLines(path string, items any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	return encoder.Encode(items)
}
