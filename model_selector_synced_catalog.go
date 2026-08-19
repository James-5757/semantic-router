package semanticrouter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ModelSelectorSyncedModel is non-secret metadata supplied by TokenCloud for
// one model in an API-key group. It describes availability and operational
// limits; it does not claim a benchmark quality score.
type ModelSelectorSyncedModel struct {
	ModelID            string  `json:"model_id"`
	DisplayName        string  `json:"display_name,omitempty"`
	ModelFamily        string  `json:"model_family,omitempty"`
	ContextWindow      int     `json:"context_window,omitempty"`
	MaxOutput          int     `json:"max_output,omitempty"`
	SupportsStreaming  bool    `json:"supports_streaming,omitempty"`
	SupportsTools      bool    `json:"supports_tools,omitempty"`
	SupportsThinking   bool    `json:"supports_thinking,omitempty"`
	PricingInputPer1K  float64 `json:"pricing_input_per_1k,omitempty"`
	PricingOutputPer1K float64 `json:"pricing_output_per_1k,omitempty"`
}

type ModelSelectorSyncHTTPRequest struct {
	GroupID  int64  `json:"group_id"`
	Platform string `json:"platform"`
	// Replace defaults to true. TokenCloud normally sends a complete group
	// snapshot, so removed models stop being eligible immediately.
	Replace *bool                      `json:"replace,omitempty"`
	Models  []ModelSelectorSyncedModel `json:"models"`
}

type ModelSelectorSyncHTTPResponse struct {
	GroupID        int64     `json:"group_id"`
	Platform       string    `json:"platform"`
	ReceivedCount  int       `json:"received_count"`
	CatalogVersion int64     `json:"catalog_version"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type modelSelectorSyncedGroup struct {
	GroupID        int64                      `json:"group_id"`
	Platform       string                     `json:"platform"`
	Models         []ModelSelectorSyncedModel `json:"models"`
	CatalogVersion int64                      `json:"catalog_version"`
	UpdatedAt      time.Time                  `json:"updated_at"`
}

type modelSelectorSyncedCatalog struct {
	mu     sync.RWMutex
	file   string
	groups map[int64]modelSelectorSyncedGroup
}

func newModelSelectorSyncedCatalog(file string) *modelSelectorSyncedCatalog {
	catalog := &modelSelectorSyncedCatalog{file: strings.TrimSpace(file), groups: make(map[int64]modelSelectorSyncedGroup)}
	catalog.load()
	return catalog
}

func (c *modelSelectorSyncedCatalog) sync(request ModelSelectorSyncHTTPRequest) (ModelSelectorSyncHTTPResponse, error) {
	if request.GroupID <= 0 {
		return ModelSelectorSyncHTTPResponse{}, fmt.Errorf("group_id must be non-zero")
	}
	if strings.TrimSpace(request.Platform) == "" {
		return ModelSelectorSyncHTTPResponse{}, fmt.Errorf("platform is required")
	}
	models, err := normalizeSyncedModels(request.Models)
	if err != nil {
		return ModelSelectorSyncHTTPResponse{}, err
	}
	replace := request.Replace == nil || *request.Replace

	c.mu.Lock()
	defer c.mu.Unlock()
	previous := c.groups[request.GroupID]
	if !replace {
		models = mergeSyncedModels(previous.Models, models)
	}
	group := modelSelectorSyncedGroup{
		GroupID: request.GroupID, Platform: strings.TrimSpace(request.Platform), Models: models,
		CatalogVersion: previous.CatalogVersion + 1, UpdatedAt: time.Now().UTC(),
	}
	c.groups[request.GroupID] = group
	if err := c.persistLocked(); err != nil {
		c.groups[request.GroupID] = previous
		if previous.GroupID == 0 {
			delete(c.groups, request.GroupID)
		}
		return ModelSelectorSyncHTTPResponse{}, fmt.Errorf("persist synchronized model catalog: %w", err)
	}
	return ModelSelectorSyncHTTPResponse{GroupID: group.GroupID, Platform: group.Platform, ReceivedCount: len(group.Models), CatalogVersion: group.CatalogVersion, UpdatedAt: group.UpdatedAt}, nil
}

// requireGroupModels makes a synchronized group an explicit security boundary.
// Legacy callers without group_id are supported temporarily, but callers that
// do send group_id cannot mix in a model from a different group.
func (c *modelSelectorSyncedCatalog) requireGroupModels(groupID int64, requested []string) ([]string, error) {
	models := uniqueModelIDs(requested)
	if groupID == 0 {
		return models, nil
	}
	c.mu.RLock()
	group, ok := c.groups[groupID]
	c.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("group_id %d has not been synchronized", groupID)
	}
	allowed := make(map[string]struct{}, len(group.Models))
	for _, model := range group.Models {
		allowed[canonicalModelID(model.ModelID)] = struct{}{}
	}
	for _, model := range models {
		if _, ok := allowed[canonicalModelID(model)]; !ok {
			return nil, fmt.Errorf("model_list contains %q outside synchronized group_id %d", model, groupID)
		}
	}
	return models, nil
}

func (c *modelSelectorSyncedCatalog) stats() (groupCount, modelCount int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, group := range c.groups {
		groupCount++
		modelCount += len(group.Models)
	}
	return groupCount, modelCount
}

func normalizeSyncedModels(models []ModelSelectorSyncedModel) ([]ModelSelectorSyncedModel, error) {
	if len(models) == 0 {
		return nil, fmt.Errorf("models is required")
	}
	seen := make(map[string]struct{}, len(models))
	result := make([]ModelSelectorSyncedModel, 0, len(models))
	for _, model := range models {
		model.ModelID = strings.TrimSpace(model.ModelID)
		if model.ModelID == "" {
			return nil, fmt.Errorf("models contains an empty model_id")
		}
		key := canonicalModelID(model.ModelID)
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("models contains duplicate model_id %q", model.ModelID)
		}
		seen[key] = struct{}{}
		result = append(result, model)
	}
	sort.Slice(result, func(i, j int) bool { return canonicalModelID(result[i].ModelID) < canonicalModelID(result[j].ModelID) })
	return result, nil
}

func mergeSyncedModels(previous, update []ModelSelectorSyncedModel) []ModelSelectorSyncedModel {
	byID := make(map[string]ModelSelectorSyncedModel, len(previous)+len(update))
	for _, model := range previous {
		byID[canonicalModelID(model.ModelID)] = model
	}
	for _, model := range update {
		byID[canonicalModelID(model.ModelID)] = model
	}
	result := make([]ModelSelectorSyncedModel, 0, len(byID))
	for _, model := range byID {
		result = append(result, model)
	}
	sort.Slice(result, func(i, j int) bool { return canonicalModelID(result[i].ModelID) < canonicalModelID(result[j].ModelID) })
	return result
}

func uniqueModelIDs(models []string) []string {
	seen := make(map[string]struct{}, len(models))
	result := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		key := canonicalModelID(model)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, model)
	}
	return result
}

func canonicalModelID(model string) string {
	return strings.ToLower(strings.TrimSpace(model))
}

func (c *modelSelectorSyncedCatalog) load() {
	if c.file == "" {
		return
	}
	contents, err := os.ReadFile(c.file)
	if err != nil || len(contents) == 0 {
		return
	}
	var groups []modelSelectorSyncedGroup
	if json.Unmarshal(contents, &groups) != nil {
		return
	}
	for _, group := range groups {
		if group.GroupID > 0 {
			c.groups[group.GroupID] = group
		}
	}
}

func (c *modelSelectorSyncedCatalog) persistLocked() error {
	if c.file == "" {
		return nil
	}
	groups := make([]modelSelectorSyncedGroup, 0, len(c.groups))
	for _, group := range c.groups {
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].GroupID < groups[j].GroupID })
	contents, err := json.MarshalIndent(groups, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.file), 0755); err != nil {
		return err
	}
	temporary := c.file + ".new"
	if err := os.WriteFile(temporary, contents, 0640); err != nil {
		return err
	}
	return os.Rename(temporary, c.file)
}
