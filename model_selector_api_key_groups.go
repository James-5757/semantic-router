package semanticrouter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// ModelSelectorAPIKeyGroupBinding stores only TokenCloud's internal API-key
// identifier. Raw API keys must never be sent to, or stored by, Selector.
type ModelSelectorAPIKeyGroupBinding struct {
	APIKeyID  int64     `json:"api_key_id"`
	GroupID   int64     `json:"group_id"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ModelSelectorSyncAPIKeyGroupHTTPRequest struct {
	APIKeyID int64 `json:"api_key_id"`
	GroupID  int64 `json:"group_id"`
}

type ModelSelectorSyncAPIKeyGroupHTTPResponse struct {
	APIKeyID  int64     `json:"api_key_id"`
	GroupID   int64     `json:"group_id"`
	UpdatedAt time.Time `json:"updated_at"`
}

type modelSelectorAPIKeyGroupStore struct {
	mu       sync.RWMutex
	file     string
	bindings map[int64]ModelSelectorAPIKeyGroupBinding
}

func newModelSelectorAPIKeyGroupStore(file string) *modelSelectorAPIKeyGroupStore {
	store := &modelSelectorAPIKeyGroupStore{file: file, bindings: make(map[int64]ModelSelectorAPIKeyGroupBinding)}
	store.load()
	return store
}

func (s *modelSelectorAPIKeyGroupStore) lookup(apiKeyID int64) (int64, bool) {
	if apiKeyID <= 0 {
		return 0, false
	}
	s.mu.RLock()
	binding, ok := s.bindings[apiKeyID]
	s.mu.RUnlock()
	return binding.GroupID, ok
}

func (s *modelSelectorAPIKeyGroupStore) count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.bindings)
}

// bind is an explicit TokenCloud group assignment. Rebinding is allowed only
// through the authenticated sync endpoint or a first valid select bootstrap.
func (s *modelSelectorAPIKeyGroupStore) bind(apiKeyID, groupID int64) (ModelSelectorAPIKeyGroupBinding, error) {
	if apiKeyID <= 0 {
		return ModelSelectorAPIKeyGroupBinding{}, fmt.Errorf("api_key_id must be non-zero")
	}
	if groupID <= 0 {
		return ModelSelectorAPIKeyGroupBinding{}, fmt.Errorf("group_id must be non-zero")
	}
	binding := ModelSelectorAPIKeyGroupBinding{APIKeyID: apiKeyID, GroupID: groupID, UpdatedAt: time.Now().UTC()}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, existed := s.bindings[apiKeyID]
	s.bindings[apiKeyID] = binding
	if err := s.persistLocked(); err != nil {
		if existed {
			s.bindings[apiKeyID] = previous
		} else {
			delete(s.bindings, apiKeyID)
		}
		return ModelSelectorAPIKeyGroupBinding{}, fmt.Errorf("persist api-key group binding: %w", err)
	}
	return binding, nil
}

func (s *modelSelectorAPIKeyGroupStore) load() {
	if s.file == "" {
		return
	}
	contents, err := os.ReadFile(s.file)
	if err != nil || len(contents) == 0 {
		return
	}
	var bindings []ModelSelectorAPIKeyGroupBinding
	if json.Unmarshal(contents, &bindings) != nil {
		return
	}
	for _, binding := range bindings {
		if binding.APIKeyID > 0 && binding.GroupID > 0 {
			s.bindings[binding.APIKeyID] = binding
		}
	}
}

func (s *modelSelectorAPIKeyGroupStore) persistLocked() error {
	if s.file == "" {
		return nil
	}
	bindings := make([]ModelSelectorAPIKeyGroupBinding, 0, len(s.bindings))
	for _, binding := range s.bindings {
		bindings = append(bindings, binding)
	}
	sort.Slice(bindings, func(i, j int) bool { return bindings[i].APIKeyID < bindings[j].APIKeyID })
	contents, err := json.MarshalIndent(bindings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.file), 0755); err != nil {
		return err
	}
	temporary := s.file + ".new"
	if err := os.WriteFile(temporary, contents, 0640); err != nil {
		return err
	}
	return os.Rename(temporary, s.file)
}
