package main

import (
	"log"
	"os"
	"semantic-router"
	"strconv"
	"time"
)

func main() {
	// 使用默认配置启动服务器
	config := semanticrouter.DefaultServerConfig()
	config.Port = "8080"
	if value, ok := os.LookupEnv("SEMANTIC_ROUTER_HTTP_PORT"); ok && value != "" {
		config.Port = value
	}
	if value, ok := os.LookupEnv("SEMANTIC_ROUTER_HTTP_LISTEN_ADDRESS"); ok && value != "" {
		config.ListenAddress = value
	}

	// 初始化组件
	semanticRouter := semanticrouter.NewRuleBasedSemanticRouter()
	tierRouter := semanticrouter.NewRuleBasedTierRouter()
	logger := semanticrouter.NewInMemoryRoutingDecisionLogger(10000)
	scheduler := semanticrouter.NewMockScheduler()
	scheduler.SetupMockAccounts()

	// 创建并启动服务器
	server := semanticrouter.NewRouterServer(config, semanticRouter, tierRouter, scheduler, logger)
	selectorHTTPConfig := semanticrouter.DefaultModelSelectorHTTPConfig()
	if value, ok := os.LookupEnv("MODEL_SELECTOR_SECRET"); ok {
		selectorHTTPConfig.SelectorSecret = value
	}
	if value, ok := os.LookupEnv("MODEL_SELECTOR_VERSION"); ok && value != "" {
		selectorHTTPConfig.Version = value
	}
	if value, ok := os.LookupEnv("MODEL_SELECTOR_OFFICIAL_VLLM_ENABLED"); ok {
		selectorHTTPConfig.OfficialVLLMEnabled, _ = strconv.ParseBool(value)
	}
	if value, ok := os.LookupEnv("MODEL_SELECTOR_OFFICIAL_VLLM_URL"); ok && value != "" {
		selectorHTTPConfig.OfficialVLLMURL = value
	}
	if value, ok := os.LookupEnv("MODEL_SELECTOR_OFFICIAL_VLLM_TIMEOUT_MS"); ok {
		if milliseconds, parseErr := strconv.Atoi(value); parseErr == nil && milliseconds > 0 {
			selectorHTTPConfig.OfficialVLLMTimeout = time.Duration(milliseconds) * time.Millisecond
		}
	}
	if value, ok := os.LookupEnv("MODEL_SELECTOR_HISTORY_FILE"); ok && value != "" {
		selectorHTTPConfig.HistoryFile = value
	}
	if value, ok := os.LookupEnv("MODEL_SELECTOR_MODEL_CATALOG_FILE"); ok && value != "" {
		selectorHTTPConfig.ModelCatalogFile = value
	}
	if value, ok := os.LookupEnv("MODEL_SELECTOR_API_KEY_GROUP_FILE"); ok && value != "" {
		selectorHTTPConfig.APIKeyGroupFile = value
	}
	if value, ok := os.LookupEnv("MODEL_SELECTOR_STATUS_MAX_CONCURRENT"); ok {
		if maxConcurrent, parseErr := strconv.Atoi(value); parseErr == nil && maxConcurrent > 0 {
			selectorHTTPConfig.StatusMaxConcurrent = maxConcurrent
		}
	}
	server.SetModelSelectorHTTPConfig(selectorHTTPConfig)
	runtimeConfig := semanticrouter.DefaultSemanticRouterRuntimeConfig()
	if value, ok := os.LookupEnv("SEMANTIC_ROUTER_TAKEOVER_ENABLED"); ok {
		runtimeConfig.SemanticRouterTakeoverEnabled, _ = strconv.ParseBool(value)
	}
	if value, ok := os.LookupEnv("SEMANTIC_ROUTER_DRY_RUN_ENABLED"); ok {
		runtimeConfig.SemanticRouterDryRunEnabled, _ = strconv.ParseBool(value)
	}
	if value, ok := os.LookupEnv("TAKEOVER_PERCENTAGE"); ok {
		runtimeConfig.TakeoverPercentage, _ = strconv.Atoi(value)
	}
	if err := server.SetShadowRuntimeConfig(runtimeConfig); err != nil {
		log.Fatalf("Invalid semantic-router runtime config: %v", err)
	}

	// Phase 2 integration endpoint. This is a separate dry-run selector path;
	// the existing HTTP scheduler remains the main result path.
	integrationConfig := semanticrouter.DefaultIntegrationConfig()
	if value, ok := os.LookupEnv("SEMANTIC_ROUTER_TCP_ENABLED"); ok {
		integrationConfig.Enabled, _ = strconv.ParseBool(value)
	}
	if value, ok := os.LookupEnv("SEMANTIC_ROUTER_TCP_LISTEN_ADDRESS"); ok {
		integrationConfig.ListenAddress = value
	}
	if value, ok := os.LookupEnv("SEMANTIC_ROUTER_TCP_REQUEST_TIMEOUT_MS"); ok {
		if milliseconds, parseErr := strconv.Atoi(value); parseErr == nil && milliseconds > 0 {
			integrationConfig.RequestTimeout = time.Duration(milliseconds) * time.Millisecond
		}
	}
	if integrationConfig.Enabled {
		selectionService, err := semanticrouter.NewModelSelectionService(semanticrouter.NewDefaultRealSchedulerDryRun(), integrationConfig)
		if err != nil {
			log.Fatalf("Invalid phase 2 integration config: %v", err)
		}
		tcpServer, err := semanticrouter.NewModelSelectorTCPServer(selectionService, integrationConfig)
		if err != nil {
			log.Fatalf("Create phase 2 TCP server: %v", err)
		}
		if err := tcpServer.Start(); err != nil {
			log.Fatalf("Start phase 2 TCP server: %v", err)
		}
		defer tcpServer.Close()
		log.Printf("Phase 2 selector TCP dry-run listening on %s (shadow=%t takeover=%t)", tcpServer.Addr(), integrationConfig.ShadowEnabled, integrationConfig.TakeoverEnabled)
	}

	log.Printf("Starting semantic router server on port %s...", config.Port)
	log.Println("Endpoints:")
	log.Println("  - GET  /health")
	log.Println("  - GET  /v1/model-selector/heartbeat")
	log.Println("  - POST /v1/debug/route")
	log.Println("  - GET  /v1/debug/stats")
	log.Println("  - GET  /v1/debug/accounts")

	if err := server.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
