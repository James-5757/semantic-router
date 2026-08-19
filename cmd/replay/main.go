package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"semantic-router/batchreplay"
)

func main() {
	// Command line flags
	inputPath := flag.String("input", "", "Path to input JSONL dataset file")
	outputDir := flag.String("output", "", "Output directory (default: replay_outputs/<timestamp>)")
	maxConcurrency := flag.Int("max-concurrency", 2, "Maximum concurrent requests")
	officialShadow := flag.Bool("official-shadow", true, "Evaluate official vLLM SR shadow")
	maxRows := flag.Int("max-rows", 0, "Maximum rows to process (0 = all)")
	smokeTest := flag.Bool("smoke", false, "Run 10-row smoke test first")
	timeoutSec := flag.Int("timeout", 60, "Request timeout in seconds")
	apiBase := flag.String("api-base", "http://127.0.0.1:8080", "Playground API base URL")
	flag.Parse()

	if *inputPath == "" {
		fmt.Println("Usage: go run ./cmd/replay --input <dataset.jsonl> [options]")
		fmt.Println("")
		fmt.Println("Options:")
		flag.PrintDefaults()
		fmt.Println("")
		fmt.Println("Example:")
		fmt.Println("  go run ./cmd/replay --input datasets/generated/OpenAssistant_oasst1_*/natural_replay.jsonl --smoke")
		fmt.Println("  go run ./cmd/replay --input datasets/generated/*/natural_replay.jsonl --max-rows 100")
		os.Exit(1)
	}

	// Expand glob patterns
	matches, err := filepath.Glob(*inputPath)
	if err != nil {
		log.Fatalf("Invalid input path: %v", err)
	}
	if len(matches) == 0 {
		log.Fatalf("No files matched: %s", *inputPath)
	}
	if len(matches) > 1 {
		log.Printf("Found %d matching files, using first: %s", len(matches), matches[0])
	}
	actualPath := matches[0]
	log.Printf("Input file: %s", actualPath)

	// Setup output directory
	if *outputDir == "" {
		ts := time.Now().Format("20060102_150405")
		*outputDir = fmt.Sprintf("replay_outputs/replay_%s", ts)
	}
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output dir: %v", err)
	}
	log.Printf("Output directory: %s", *outputDir)

	// Determine effective dataset path (apply smoke test / max-rows)
	datasetPath := actualPath
	needsFilter := *smokeTest || *maxRows > 0
	if needsFilter {
		entries, err := batchreplay.ReadDataset(actualPath)
		if err != nil {
			log.Fatalf("Failed to read dataset: %v", err)
		}

		limit := *maxRows
		if *smokeTest {
			limit = 10
		}
		if limit > 0 && limit < len(entries) {
			entries = entries[:limit]
			log.Printf("Limited to %d rows", len(entries))
		}

		limitedPath := filepath.Join(*outputDir, "limited_dataset.jsonl")
		f, err := os.Create(limitedPath)
		if err != nil {
			log.Fatalf("Failed to create limited dataset: %v", err)
		}
		for _, entry := range entries {
			line, _ := json.Marshal(entry)
			f.Write(append(line, '\n'))
		}
		f.Close()
		datasetPath = limitedPath
	}

	// Create evaluate function (calls playground HTTP API)
	apiURL := *apiBase
	evFn := batchreplay.EvaluateFunc(func(prompt, mode string, hasImage, hasDocument, hasCSV bool) (map[string]interface{}, error) {
		return callEvaluateAPI(apiURL, prompt, mode, hasImage, hasDocument, hasCSV, *timeoutSec)
	})

	config := batchreplay.DefaultConfig()
	config.InputPath = datasetPath
	config.OutputDir = *outputDir
	config.MaxConcurrency = *maxConcurrency
	config.OfficialShadow = *officialShadow
	config.RequestTimeout = time.Duration(*timeoutSec) * time.Second

	runner := batchreplay.NewReplayRunner(config, evFn)

	// Progress callback
	startTime := time.Now()
	runner.SetProgressCallback(func(current, total int, result batchreplay.ReplayResult) {
		elapsed := time.Since(startTime).Seconds()
		progress := float64(current) / float64(total) * 100
		status := "OK"
		if result.Error != "" {
			status = "ERR"
		}
		preview := result.PromptPreview
		if len(preview) > 30 {
			preview = preview[:30]
		}
		fmt.Printf("[%s] [%4d/%-4d] %5.1f%% | %s | intent: %-18s pool: %-20s tier: %-6s | %.1fs\n",
			status, current, total, progress,
			preview,
			result.PrimaryIntent,
			result.LocalNormalizedPool,
			result.SelectedTier,
			elapsed,
		)
	})

	// Run the replay - Run takes datasetPath, not entries
	results, summary, manifest, err := runner.Run(datasetPath)
	if err != nil {
		log.Printf("Replay completed with errors: %v", err)
	}

	// Save results
	if err := runner.SaveResults(results, summary, manifest); err != nil {
		log.Fatalf("Failed to save results: %v", err)
	}

	// Print summary
	log.Printf(strings.Repeat("=", 60))
	log.Printf("BATCH REPLAY COMPLETE")
	log.Printf("  Total:    %d", summary.Total)
	log.Printf("  Success:  %d", summary.Success)
	log.Printf("  Errors:   %d", summary.ErrorCount)
	log.Printf("  Latency:  avg=%.1fms p95=%.1fms", summary.AverageLatencyMs, summary.P95LatencyMs)
	if summary.Total > 0 {
		log.Printf("  Default Route Rate:  %.1f%%", summary.DefaultRouteRate*100)
		log.Printf("  Semantic Agreement:  %.1f%%", summary.SemanticAgreementRate*100)
		log.Printf("  Pool Agreement:      %.1f%%", summary.PoolAgreementRate*100)
		log.Printf("  Group Agreement:     %.1f%%", summary.GroupAgreementRate*100)
		log.Printf("  Official Error Rate: %.1f%%", summary.OfficialErrorRate*100)
	}
	log.Printf("  Pool Distribution:   %v", summary.PoolDistribution)
	log.Printf("  Tier Distribution:   %v", summary.TierDistribution)
	log.Printf("  Output: %s", *outputDir)
}

func callEvaluateAPI(apiURL, prompt, mode string, hasImage, hasDocument, hasCSV bool, timeoutSec int) (map[string]interface{}, error) {
	importReq, _ := json.Marshal(map[string]interface{}{
		"prompt":       prompt,
		"has_image":    hasImage,
		"has_document": hasDocument,
		"has_csv":      hasCSV,
		"mode":         mode,
	})

	// Use the batch evaluate endpoint
	apiURL = strings.TrimRight(apiURL, "/")
	req, err := http.NewRequest("POST", apiURL+"/v1/debug/events/evaluate", bytes.NewReader(importReq))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("api call: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return result, nil
}
