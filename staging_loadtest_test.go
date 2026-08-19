package semanticrouter

import (
	"context"
	"testing"
	"time"
)

func TestStagingLoadTestHarness(t *testing.T) {
	config := DefaultIntegrationConfig()
	config.ListenAddress = "127.0.0.1:0"
	config.ConnectTimeout = time.Second
	config.RequestTimeout = time.Second
	service, err := NewModelSelectionService(NewStagingRealScheduler(1001), config)
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewModelSelectorTCPServer(service, config)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	config.ServiceAddress = server.Addr().String()
	result, err := RunStagingLoadTest(context.Background(), NewModelSelectorTCPClient(config), StagingLoadTestConfig{Requests: 100, Concurrency: 8, GroupID: 1001})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("staging load test: %+v", result)
	if result.Success != 100 || result.Errors != 0 || result.UpstreamCalled != 0 || result.InvalidAccount != 0 {
		t.Fatalf("unsafe staging result: %+v", result)
	}
	if result.P95LatencyMs <= 0 || result.P99LatencyMs <= 0 {
		t.Fatalf("latency percentiles missing: %+v", result)
	}
	if len(result.PoolCounts) < 3 {
		t.Fatalf("expected multiple pool paths: %+v", result.PoolCounts)
	}
}
