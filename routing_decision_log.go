package semanticrouter

import (
	"sync"
	"time"
)

type RoutingDecisionLogEntry struct {
	RequestID             string
	APIKeyID              int64
	GroupID               int64
	PromptHash            string
	PreferredPool         PreferredPool
	PreferredTier         PreferredTier
	TaskType              TaskType
	Confidence            float64
	MatchedRules          []string
	SemanticScores        map[string]float64
	// ModelRankingJSON stores the consumer-facing candidate list with final_score.
	ModelRankingJSON      string
	FinalDecisionSource   string
	FallbackReason        string
	SelectedAccountID     int64
	SelectedModel         string
	SchedulerLayer        string
	OldSchedulerAccountID int64
	ShadowLatencyMs       float64
	OldSelectedAccountID  int64
	NewSuggestedAccountID int64
	OldSelectedModel      string
	NewSuggestedModel     string
	OldSelectedPool       string
	NewSuggestedPool      string
	IsAgree               bool
	CreatedAt             time.Time
}

type RoutingDecisionLogWriter interface {
	LogRoutingDecision(entry *RoutingDecisionLogEntry) error
}

type InMemoryRoutingDecisionLogStore struct {
	mu      sync.RWMutex
	entries []*RoutingDecisionLogEntry
}

func NewInMemoryRoutingDecisionLogStore() *InMemoryRoutingDecisionLogStore {
	return &InMemoryRoutingDecisionLogStore{
		entries: make([]*RoutingDecisionLogEntry, 0),
	}
}

func (s *InMemoryRoutingDecisionLogStore) LogRoutingDecision(entry *RoutingDecisionLogEntry) error {
	if entry == nil {
		return nil
	}
	cloned := cloneRoutingDecisionLogEntry(entry)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, cloned)
	return nil
}

func (s *InMemoryRoutingDecisionLogStore) Entries() []*RoutingDecisionLogEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries := make([]*RoutingDecisionLogEntry, 0, len(s.entries))
	for _, entry := range s.entries {
		entries = append(entries, cloneRoutingDecisionLogEntry(entry))
	}
	return entries
}

func (s *InMemoryRoutingDecisionLogStore) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

func cloneRoutingDecisionLogEntry(entry *RoutingDecisionLogEntry) *RoutingDecisionLogEntry {
	if entry == nil {
		return nil
	}
	cloned := *entry
	cloned.MatchedRules = append([]string(nil), entry.MatchedRules...)
	cloned.SemanticScores = cloneFloatMap(entry.SemanticScores)
	return &cloned
}

var _ RoutingDecisionLogWriter = (*InMemoryRoutingDecisionLogStore)(nil)
