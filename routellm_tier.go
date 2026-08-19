package semanticrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// RouteLLMTierConfig controls an optional learned tier scorer. It is deliberately
// shadow-only: enabling it must never change Scheduler input or RuleBasedTierRouter.
type RouteLLMTierConfig struct {
	Enabled         bool   `json:"routellm_tier_enabled" yaml:"routellm_tier_enabled"`
	ShadowOnly      bool   `json:"routellm_tier_shadow_only" yaml:"routellm_tier_shadow_only"`
	ServiceURL      string `json:"routellm_tier_service_url" yaml:"routellm_tier_service_url"`
	TimeoutMS       int    `json:"routellm_tier_timeout_ms" yaml:"routellm_tier_timeout_ms"`
	TakeoverEnabled bool   `json:"routellm_tier_takeover_enabled" yaml:"routellm_tier_takeover_enabled"`
}

func DefaultRouteLLMTierConfig() RouteLLMTierConfig {
	return RouteLLMTierConfig{
		Enabled:         false,
		ShadowOnly:      true,
		ServiceURL:      "http://127.0.0.1:8002",
		TimeoutMS:       1200,
		TakeoverEnabled: false,
	}
}

func (c RouteLLMTierConfig) Validate() error {
	if c.TimeoutMS <= 0 {
		return fmt.Errorf("routellm_tier_timeout_ms must be positive")
	}
	if c.Enabled && strings.TrimSpace(c.ServiceURL) == "" {
		return fmt.Errorf("routellm_tier_service_url is required when enabled")
	}
	if !c.ShadowOnly {
		return fmt.Errorf("routellm_tier_shadow_only must remain true")
	}
	if c.TakeoverEnabled {
		return fmt.Errorf("routellm_tier_takeover_enabled must remain false")
	}
	return nil
}

// LearnedTierScore contains the learned model's recommendation only. It is not a
// scheduling decision and cannot initiate a model completion.
type LearnedTierScore struct {
	Router               string        `json:"router"`
	StrongWinProbability float64       `json:"strong_win_probability"`
	SuggestedTier        PreferredTier `json:"suggested_tier"`
	WeakThreshold        float64       `json:"weak_threshold"`
	StrongThreshold      float64       `json:"strong_threshold"`
	LatencyMS            float64       `json:"latency_ms"`
}

func TierForRouteLLMProbability(probability, weakThreshold, strongThreshold float64) PreferredTier {
	if probability < weakThreshold {
		return TierWeak
	}
	if probability < strongThreshold {
		return TierMedium
	}
	return TierStrong
}

type LearnedTierScorer interface {
	Score(ctx context.Context, prompt string) (*LearnedTierScore, error)
	Health(ctx context.Context) error
}

// RouteLLMTierClient is a small HTTP adapter around the local RouteLLM tier service.
type RouteLLMTierClient struct {
	baseURL string
	client  *http.Client
}

func NewRouteLLMTierClient(config RouteLLMTierConfig) *RouteLLMTierClient {
	timeout := time.Duration(config.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 1200 * time.Millisecond
	}
	return &RouteLLMTierClient{baseURL: strings.TrimRight(config.ServiceURL, "/"), client: &http.Client{Timeout: timeout}}
}

func (c *RouteLLMTierClient) Score(ctx context.Context, prompt string) (*LearnedTierScore, error) {
	body, err := json.Marshal(map[string]string{"prompt": prompt})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/score", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("routellm tier service returned %d", resp.StatusCode)
	}
	var score LearnedTierScore
	if err := json.NewDecoder(resp.Body).Decode(&score); err != nil {
		return nil, err
	}
	if score.SuggestedTier != TierWeak && score.SuggestedTier != TierMedium && score.SuggestedTier != TierStrong {
		return nil, fmt.Errorf("invalid routellm suggested_tier %q", score.SuggestedTier)
	}
	if score.WeakThreshold >= score.StrongThreshold {
		return nil, fmt.Errorf("invalid routellm thresholds")
	}
	if expected := TierForRouteLLMProbability(score.StrongWinProbability, score.WeakThreshold, score.StrongThreshold); score.SuggestedTier != expected {
		return nil, fmt.Errorf("routellm suggested_tier %q does not match score", score.SuggestedTier)
	}
	return &score, nil
}

func (c *RouteLLMTierClient) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("routellm tier health returned %d", resp.StatusCode)
	}
	return nil
}

// HybridTierDecision keeps the rule tier authoritative during the entire shadow phase.
type HybridTierDecision struct {
	RuleTier            PreferredTier  `json:"rule_tier"`
	RouteLLMProbability *float64       `json:"routellm_probability,omitempty"`
	RouteLLMTier        *PreferredTier `json:"routellm_tier,omitempty"`
	Router              string         `json:"router,omitempty"`
	WeakThreshold       float64        `json:"weak_threshold"`
	StrongThreshold     float64        `json:"strong_threshold"`
	ShadowOnly          bool           `json:"shadow_only"`
	UpstreamCalled      bool           `json:"upstream_called"`
	FinalTier           PreferredTier  `json:"final_tier"`
	FinalTierSource     string         `json:"final_tier_source"`
	IsAgreement         bool           `json:"is_agreement"`
	RouteLLMLatencyMS   float64        `json:"routellm_latency_ms"`
	RouteLLMError       string         `json:"routellm_error,omitempty"`
}

type HybridTierScorer struct {
	config  RouteLLMTierConfig
	learned LearnedTierScorer
}

func NewHybridTierScorer(config RouteLLMTierConfig, learned LearnedTierScorer) *HybridTierScorer {
	return &HybridTierScorer{config: config, learned: learned}
}

func (s *HybridTierScorer) Decide(ctx context.Context, prompt string, ruleTier PreferredTier) (result HybridTierDecision) {
	result = HybridTierDecision{RuleTier: ruleTier, FinalTier: ruleTier, FinalTierSource: "rule_shadow"}
	if !s.config.Enabled {
		return result
	}
	if s.learned == nil {
		result.RouteLLMError = "learned tier scorer is unavailable"
		return result
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			result.RouteLLMError = fmt.Sprintf("learned tier scorer panic: %v", recovered)
			result.FinalTier, result.FinalTierSource, result.IsAgreement = ruleTier, "rule_shadow", false
		}
	}()
	score, err := s.learned.Score(ctx, prompt)
	if err != nil {
		result.RouteLLMError = err.Error()
		return result
	}
	result.RouteLLMProbability = &score.StrongWinProbability
	result.RouteLLMTier = &score.SuggestedTier
	result.Router = score.Router
	result.WeakThreshold = score.WeakThreshold
	result.StrongThreshold = score.StrongThreshold
	result.ShadowOnly = s.config.ShadowOnly
	result.UpstreamCalled = false // RouteLLM tier service never calls real upstream
	result.RouteLLMLatencyMS = score.LatencyMS
	result.IsAgreement = score.SuggestedTier == ruleTier
	// Do not add a takeover branch here. RouteLLM remains observational only.
	return result
}

var _ LearnedTierScorer = (*RouteLLMTierClient)(nil)
