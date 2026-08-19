package semanticrouter

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type StagingLoadTestConfig struct {
	Requests    int
	Concurrency int
	GroupID     int64
	Prompts     []string
}

type StagingLoadTestResult struct {
	TotalRequests     int            `json:"total_requests"`
	Success           int            `json:"success"`
	Errors            int            `json:"errors"`
	ErrorRate         float64        `json:"error_rate"`
	UpstreamCalled    int            `json:"upstream_called"`
	InvalidAccount    int            `json:"invalid_account"`
	AverageLatencyMs  float64        `json:"average_latency_ms"`
	P50LatencyMs      float64        `json:"p50_latency_ms"`
	P95LatencyMs      float64        `json:"p95_latency_ms"`
	P99LatencyMs      float64        `json:"p99_latency_ms"`
	RequestsPerSecond float64        `json:"requests_per_second"`
	PoolCounts        map[string]int `json:"pool_counts"`
}

func RunStagingLoadTest(ctx context.Context, client *ModelSelectorTCPClient, config StagingLoadTestConfig) (*StagingLoadTestResult, error) {
	if client == nil {
		return nil, fmt.Errorf("client is required")
	}
	if config.Requests <= 0 || config.Concurrency <= 0 || config.GroupID == 0 {
		return nil, fmt.Errorf("requests, concurrency and group_id must be positive")
	}
	if len(config.Prompts) == 0 {
		config.Prompts = []string{"hello", "implement a Python API", "analyze this CSV and make a chart", "summarize this PDF", "describe this image", "generate an image"}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	start := time.Now()
	jobs := make(chan int)
	var success, errors, upstream, invalid atomic.Int64
	latencies := make([]float64, 0, config.Requests)
	var latencyMu sync.Mutex
	pools := make(map[string]int)
	var poolMu sync.Mutex
	workerCount := config.Concurrency
	if workerCount > config.Requests {
		workerCount = config.Requests
	}
	var wg sync.WaitGroup
	worker := func() {
		defer wg.Done()
		for index := range jobs {
			prompt := config.Prompts[index%len(config.Prompts)]
			request := &ModelSelectionRequest{ProtocolVersion: ModelSelectorProtocolVersion, RequestID: fmt.Sprintf("staging-load-%d", index), GroupID: config.GroupID, Prompt: prompt}
			requestStart := time.Now()
			response, err := client.Select(ctx, request)
			latency := float64(time.Since(requestStart)) / float64(time.Millisecond)
			latencyMu.Lock()
			latencies = append(latencies, latency)
			latencyMu.Unlock()
			if err != nil {
				errors.Add(1)
				continue
			}
			success.Add(1)
			if response.UpstreamCalled {
				upstream.Add(1)
			}
			if response.SelectedAccountID == 0 {
				invalid.Add(1)
			}
			poolMu.Lock()
			pools[response.SelectedPool]++
			poolMu.Unlock()
		}
	}
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go worker()
	}
	for i := 0; i < config.Requests; i++ {
		select {
		case jobs <- i:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return nil, ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	duration := time.Since(start)
	sort.Float64s(latencies)
	result := &StagingLoadTestResult{TotalRequests: config.Requests, Success: int(success.Load()), Errors: int(errors.Load()), UpstreamCalled: int(upstream.Load()), InvalidAccount: int(invalid.Load()), PoolCounts: pools}
	if result.TotalRequests > 0 {
		result.ErrorRate = float64(result.Errors) / float64(result.TotalRequests)
	}
	if len(latencies) > 0 {
		result.AverageLatencyMs = sumFloat64(latencies) / float64(len(latencies))
		result.P50LatencyMs = percentile(latencies, 0.50)
		result.P95LatencyMs = percentile(latencies, 0.95)
		result.P99LatencyMs = percentile(latencies, 0.99)
	}
	if duration > 0 {
		result.RequestsPerSecond = float64(result.TotalRequests) / duration.Seconds()
	}
	return result, nil
}

func sumFloat64(values []float64) float64 {
	total := 0.0
	for _, value := range values {
		total += value
	}
	return total
}
func percentile(values []float64, ratio float64) float64 {
	if len(values) == 0 {
		return 0
	}
	index := int(float64(len(values))*ratio) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return values[index]
}
