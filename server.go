package semanticrouter

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// ServerConfig 服务器配置
type ServerConfig struct {
	Port          string
	ListenAddress string
	ReadTimeout   time.Duration
	WriteTimeout  time.Duration
}

// DefaultServerConfig 默认配置
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Port:          "8080",
		ListenAddress: "",
		ReadTimeout:   10 * time.Second,
		WriteTimeout:  10 * time.Second,
	}
}

// RouterServer HTTP 服务器
type RouterServer struct {
	config            *ServerConfig
	multiLayerRouter  *MultiLayerRouter
	semanticRouter    SemanticRouter
	tierRouter        TierRouter
	scheduler         SchedulerFacade
	logger            RoutingDecisionLogger
	shadowRouter      *ShadowRouter
	shadowLogStore    *InMemoryRoutingDecisionLogStore
	modelSelectorHTTP *ModelSelectorHTTPHandler
	httpServer        *http.Server
}

// NewRouterServer 创建路由服务器
func NewRouterServer(
	config *ServerConfig,
	semanticRouter SemanticRouter,
	tierRouter TierRouter,
	scheduler SchedulerFacade,
	logger RoutingDecisionLogger,
) *RouterServer {
	// 创建多层路由组合器
	multiLayerRouter := NewMultiLayerRouter()

	shadowLogStore := NewInMemoryRoutingDecisionLogStore()
	httpConfig := DefaultIntegrationConfig()
	selectionService, err := NewModelSelectionService(NewDefaultRealSchedulerDryRun(), httpConfig)
	if err != nil {
		panic(fmt.Sprintf("create model selector HTTP service: %v", err))
	}
	server := &RouterServer{
		config:            config,
		multiLayerRouter:  multiLayerRouter,
		semanticRouter:    semanticRouter,
		tierRouter:        tierRouter,
		scheduler:         scheduler,
		logger:            logger,
		shadowLogStore:    shadowLogStore,
		modelSelectorHTTP: NewModelSelectorHTTPHandler(DefaultModelSelectorHTTPConfig(), selectionService),
	}
	server.shadowRouter = NewShadowRouter(scheduler, NewDefaultRealSchedulerDryRun(), multiLayerRouter, tierRouter, shadowLogStore)
	return server
}

func (s *RouterServer) SetShadowRuntimeConfig(config SemanticRouterRuntimeConfig) error {
	return s.shadowRouter.SetRuntimeConfig(config)
}

// SetModelSelectorHTTPConfig configures only the TokenCloud v1.2 HTTP boundary.
func (s *RouterServer) SetModelSelectorHTTPConfig(config ModelSelectorHTTPConfig) {
	service := s.modelSelectorHTTP.service
	s.modelSelectorHTTP = NewModelSelectorHTTPHandler(config, service)
}

// Start 启动服务器
func (s *RouterServer) Start() error {
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	s.modelSelectorHTTP.Register(mux)

	// Debug route endpoint
	mux.HandleFunc("/v1/debug/route", func(w http.ResponseWriter, r *http.Request) {
		HandleDebugRoute(w, r, s.multiLayerRouter, s.tierRouter, s.scheduler, s.logger)
	})

	// Stats endpoint
	mux.HandleFunc("/v1/debug/stats", func(w http.ResponseWriter, r *http.Request) {
		s.handleStats(w, r)
	})

	// Accounts endpoint
	mux.HandleFunc("/v1/debug/accounts", func(w http.ResponseWriter, r *http.Request) {
		s.handleAccounts(w, r)
	})
	mux.HandleFunc("/v1/debug/shadow", func(w http.ResponseWriter, r *http.Request) {
		HandleDebugShadow(w, r, s.shadowRouter, s.shadowLogStore)
	})
	mux.HandleFunc("/v1/debug/shadow/metrics", func(w http.ResponseWriter, r *http.Request) {
		HandleDebugShadowMetrics(w, r, s.shadowRouter)
	})
	mux.HandleFunc("/v1/debug/shadow/route", s.handleShadowRoute)
	mux.HandleFunc("/v1/debug/takeover/metrics", func(w http.ResponseWriter, r *http.Request) {
		HandleDebugTakeoverMetrics(w, r, s.shadowRouter)
	})
	mux.HandleFunc("/v1/debug/takeover/status", func(w http.ResponseWriter, r *http.Request) {
		HandleDebugTakeoverStatus(w, r, s.shadowRouter)
	})

	listenAddress := s.config.ListenAddress
	if listenAddress == "" {
		listenAddress = ":" + s.config.Port
	}
	s.httpServer = &http.Server{
		Addr:         listenAddress,
		Handler:      mux,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
	}

	log.Printf("Starting server on %s", listenAddress)
	log.Printf("Endpoints:")
	log.Printf("  - GET  /health")
	log.Printf("  - GET  /v1/model-selector/heartbeat")
	log.Printf("  - POST /v1/debug/route")
	log.Printf("  - GET  /v1/debug/stats")
	log.Printf("  - GET  /v1/debug/accounts")

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *RouterServer) handleShadowRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var request DebugRouteRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
		return
	}
	var prompt strings.Builder
	for _, message := range request.Messages {
		prompt.WriteString(message.Content)
	}
	result := s.shadowRouter.Route(&ShadowModeRequest{
		RequestID: fmt.Sprintf("local-replay-%d", time.Now().UnixNano()),
		Model:     request.Model,
		OldSchedulerRequest: &SchedulerSelectRequest{
			Model:         request.Model,
			PreferredPool: PoolCheap,
			PreferredTier: TierWeak,
			TaskType:      TaskTypeText,
		},
		RouteRequest: &RouteRequest{
			Model:       request.Model,
			Prompt:      prompt.String(),
			HasImage:    len(request.Images) > 0,
			HasDocument: len(request.Documents) > 0,
		},
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"main_result":     result.MainResult(),
		"suggestion":      result.Suggestion,
		"takeover_result": result.TakeoverResult,
		"decision": map[string]interface{}{
			"preferred_pool": result.Decision.PreferredPool,
			"preferred_tier": result.LogEntry.PreferredTier,
			"task_type":      result.Decision.TaskType,
		},
		"shadow_error": errorString(result.ShadowError),
	})
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// Stop 停止服务器
func (s *RouterServer) Stop() error {
	if s.httpServer != nil {
		return s.httpServer.Close()
	}
	return nil
}

// handleStats 处理 stats 请求
func (s *RouterServer) handleStats(w http.ResponseWriter, r *http.Request) {
	if s.logger == nil {
		http.Error(w, `{"error": "logger not configured"}`, http.StatusInternalServerError)
		return
	}

	stats := s.logger.GetStats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleAccounts 处理 accounts 请求
func (s *RouterServer) handleAccounts(w http.ResponseWriter, r *http.Request) {
	if s.scheduler == nil {
		http.Error(w, `{"error": "scheduler not configured"}`, http.StatusInternalServerError)
		return
	}

	// 获取账号列表（需要类型断言）
	if mockSched, ok := s.scheduler.(*MockScheduler); ok {
		accounts := mockSched.GetAccounts()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(accounts)
		return
	}

	http.Error(w, `{"error": "scheduler does not support account listing"}`, http.StatusInternalServerError)
}

// RunMain 主函数 - 演示用
func RunMain() {
	// 初始化组件
	semanticRouter := NewRuleBasedSemanticRouter()
	tierRouter := NewRuleBasedTierRouter()
	logger := NewInMemoryRoutingDecisionLogger(10000)

	// 初始化 scheduler
	scheduler := NewMockScheduler()
	scheduler.SetupMockAccounts()

	// 创建服务器
	config := DefaultServerConfig()
	server := NewRouterServer(config, semanticRouter, tierRouter, scheduler, logger)

	// 优雅关闭
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("Shutting down server...")
		server.Stop()
	}()

	// 启动服务器
	if err := server.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

// DryRunExamples 提供 4 个 dry-run 示例请求
func DryRunExamples() []DryRunExample {
	return []DryRunExample{
		{
			Name:        "普通文本请求",
			Description: "普通对话请求，应进入 cheap_chat_pool",
			Request: DebugRouteRequest{
				Model: "gpt-3.5-turbo",
				Messages: []MessageContent{
					{Role: "user", Content: "你好，请介绍一下北京的历史"},
				},
			},
		},
		{
			Name:        "代码请求",
			Description: "代码生成请求，应进入 code_pool",
			Request: DebugRouteRequest{
				Model: "gpt-4",
				Messages: []MessageContent{
					{Role: "user", Content: "请用 Python 写一个快速排序函数:\n```python\ndef quick_sort(arr):\n```"},
				},
			},
		},
		{
			Name:        "图片请求",
			Description: "图片理解请求，应进入 vision_pool",
			Request: DebugRouteRequest{
				Model: "gpt-4o",
				Messages: []MessageContent{
					{Role: "user", Content: "请描述这张图片的内容"},
					{Role: "user", Type: "image_url", Content: "https://example.com/image.jpg"},
				},
				Images: []string{"https://example.com/image.jpg"},
			},
		},
		{
			Name:        "文档请求",
			Description: "Word 文档处理请求，应进入 document_pool",
			Request: DebugRouteRequest{
				Model: "gpt-4",
				Messages: []MessageContent{
					{Role: "user", Content: "请总结这个文档的内容"},
				},
				Documents: []string{"report.docx"},
			},
		},
	}
}

// DryRunExample 示例请求
type DryRunExample struct {
	Name        string
	Description string
	Request     DebugRouteRequest
}

// PrintDryRunExamples 打印 dry-run 示例
func PrintDryRunExamples() {
	examples := DryRunExamples()
	fmt.Println("=== 4 个 Dry-Run 示例请求 ===")
	for i, ex := range examples {
		fmt.Printf("%d. %s\n", i+1, ex.Name)
		fmt.Printf("   描述: %s\n", ex.Description)
		jsonBytes, _ := json.MarshalIndent(ex.Request, "", "   ")
		fmt.Printf("   请求:\n%s\n\n", string(jsonBytes))
	}
}

// ExecuteDryRun 执行 dry-run 示例
func ExecuteDryRun(serverURL string) error {
	examples := DryRunExamples()

	client := &http.Client{Timeout: 10 * time.Second}

	for i, ex := range examples {
		fmt.Printf("\n=== 执行示例 %d: %s ===\n", i+1, ex.Name)

		resp, err := client.Post(serverURL+"/v1/debug/route", "application/json", nil)
		if err != nil {
			// 如果服务器没运行，使用本地模拟
			fmt.Println("服务器未运行，使用本地模拟...")

			// 本地模拟执行
			semanticRouter := NewRuleBasedSemanticRouter()
			tierRouter := NewRuleBasedTierRouter()
			logger := NewInMemoryRoutingDecisionLogger(10000)
			scheduler := NewMockScheduler()
			scheduler.SetupMockAccounts()

			routeReq := &RouteRequest{
				Model: ex.Request.Model,
				Prompt: func() string {
					var sb strings.Builder
					for _, m := range ex.Request.Messages {
						sb.WriteString(m.Content)
					}
					return sb.String()
				}(),
				HasImage:     len(ex.Request.Images) > 0 || containsImageMessage(ex.Request.Messages),
				HasDocument:  len(ex.Request.Documents) > 0,
				DocumentType: detectDocumentTypeFromReq(&ex.Request),
			}

			preRouter := NewPreRouter(semanticRouter, tierRouter, logger)
			result, err := preRouter.Route(nil, ex.Request.Model, "", "", routeReq)
			if err != nil {
				fmt.Printf("错误: %v\n", err)
				continue
			}

			schedulerReq := &SchedulerSelectRequest{
				Model:                ex.Request.Model,
				PreferredPool:        result.Decision.FinalPool,
				PreferredTier:        result.Decision.Tier.PreferredTier,
				TaskType:             result.Decision.Semantic.TaskType,
				RequiredCapabilities: result.Decision.Semantic.RequiredCapabilities,
			}
			schedulerResult := scheduler.Select(schedulerReq)

			fmt.Printf("语义决策: preferred_pool=%s, task_type=%s, modality=%s\n",
				result.Decision.Semantic.PreferredPool,
				result.Decision.Semantic.TaskType,
				result.Decision.Semantic.Modality)
			fmt.Printf("Tier决策: preferred_tier=%s, reason=%s\n",
				result.Decision.Tier.PreferredTier,
				result.Decision.Tier.Reason)
			fmt.Printf("调度决策: account_id=%d, pool=%s, layer=%s\n",
				schedulerResult.SelectedAccountID,
				schedulerResult.PoolUsed,
				schedulerResult.Layer)
			continue
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("响应 (%d):\n%s\n", resp.StatusCode, string(body))
	}

	return nil
}
