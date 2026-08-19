package semanticrouter

import (
	"fmt"
	"time"
)

type IntegrationConfig struct {
	Enabled         bool          `json:"semantic_router_integration_enabled" yaml:"semantic_router_integration_enabled"`
	ShadowEnabled   bool          `json:"semantic_router_shadow_enabled" yaml:"semantic_router_shadow_enabled"`
	DryRunEnabled   bool          `json:"semantic_router_dry_run_enabled" yaml:"semantic_router_dry_run_enabled"`
	TakeoverEnabled bool          `json:"semantic_router_takeover_enabled" yaml:"semantic_router_takeover_enabled"`
	ListenAddress   string        `json:"semantic_router_tcp_listen_address" yaml:"semantic_router_tcp_listen_address"`
	ServiceAddress  string        `json:"semantic_router_service_address" yaml:"semantic_router_service_address"`
	ConnectTimeout  time.Duration `json:"-" yaml:"-"`
	RequestTimeout  time.Duration `json:"-" yaml:"-"`
	MaxFrameBytes   int           `json:"semantic_router_max_frame_bytes" yaml:"semantic_router_max_frame_bytes"`
}

func DefaultIntegrationConfig() IntegrationConfig {
	return IntegrationConfig{Enabled: true, ShadowEnabled: true, DryRunEnabled: true, ListenAddress: "127.0.0.1:9101", ServiceAddress: "127.0.0.1:9101", ConnectTimeout: 500 * time.Millisecond, RequestTimeout: 500 * time.Millisecond, MaxFrameBytes: MaxIntegrationFrameBytes}
}

func (c IntegrationConfig) Validate() error {
	if c.TakeoverEnabled {
		return fmt.Errorf("semantic_router_takeover_enabled must remain false during phase 2")
	}
	if c.DryRunEnabled && !c.ShadowEnabled {
		return fmt.Errorf("dry-run requires shadow mode")
	}
	if c.MaxFrameBytes <= 0 || c.MaxFrameBytes > MaxIntegrationFrameBytes {
		return fmt.Errorf("max frame bytes must be between 1 and %d", MaxIntegrationFrameBytes)
	}
	if c.RequestTimeout <= 0 || c.ConnectTimeout <= 0 {
		return fmt.Errorf("integration timeouts must be positive")
	}
	return nil
}
