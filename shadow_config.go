package semanticrouter

import "fmt"

type SemanticRouterRuntimeConfig struct {
	SemanticRouterShadowEnabled   bool  `json:"semantic_router_shadow_enabled" yaml:"semantic_router_shadow_enabled"`
	SemanticRouterDryRunEnabled   bool  `json:"semantic_router_dry_run_enabled" yaml:"semantic_router_dry_run_enabled"`
	SemanticRouterTakeoverEnabled bool  `json:"semantic_router_takeover_enabled" yaml:"semantic_router_takeover_enabled"`
	TakeoverPercentage            int   `json:"takeover_percentage" yaml:"takeover_percentage"` // 0-100, 灰度接管百分比
}

func DefaultSemanticRouterRuntimeConfig() SemanticRouterRuntimeConfig {
	return SemanticRouterRuntimeConfig{
		SemanticRouterShadowEnabled:   true,
		SemanticRouterDryRunEnabled:   true,
		SemanticRouterTakeoverEnabled: false,
		TakeoverPercentage:            0,
	}
}

func (c SemanticRouterRuntimeConfig) Validate() error {
	if c.TakeoverPercentage < 0 || c.TakeoverPercentage > 100 {
		return fmt.Errorf("takeover_percentage must be between 0 and 100, got %d", c.TakeoverPercentage)
	}
	if !c.SemanticRouterTakeoverEnabled && c.TakeoverPercentage > 0 {
		return fmt.Errorf("takeover_percentage > 0 requires semantic_router_takeover_enabled=true")
	}
	if c.SemanticRouterTakeoverEnabled && c.TakeoverPercentage == 0 {
		return fmt.Errorf("semantic_router_takeover_enabled requires takeover_percentage > 0")
	}
	if c.SemanticRouterDryRunEnabled && !c.SemanticRouterShadowEnabled {
		return fmt.Errorf("semantic_router_dry_run_enabled requires semantic_router_shadow_enabled")
	}
	return nil
}
