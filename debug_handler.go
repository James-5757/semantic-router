package semanticrouter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// DebugRouteRequest /v1/debug/route 接口请求
type DebugRouteRequest struct {
	Messages           []MessageContent `json:"messages"`
	Modality           string           `json:"modality"`  // text, image, document
	Images             []string         `json:"images"`    // image URLs
	Documents          []string         `json:"documents"` // document URLs
	PreviousResponseID string           `json:"previous_response_id"`
	SessionHash        string           `json:"session_hash"`
	Model              string           `json:"model"`
}

// DebugRouteResponse /v1/debug/route 接口响应
type DebugRouteResponse struct {
	Success bool `json:"success"`

	// 语义路由决策
	SemanticDecision SemanticDecisionOutput `json:"semantic_decision"`

	// Tier 路由决策
	TierDecision TierDecisionOutput `json:"tier_decision"`

	// Scheduler 调度决策
	SchedulerDecision SchedulerDecisionOutput `json:"scheduler_decision"`

	// 解释信息
	Explain *ExplainOutput `json:"explain,omitempty"`

	// 错误信息
	Error string `json:"error,omitempty"`
}

// ExplainOutput 路由决策解释
type ExplainOutput struct {
	MatchedRules        []string           `json:"matched_rules"`
	SemanticScores      map[string]float64 `json:"semantic_scores"`
	Confidence          float64            `json:"confidence"`
	FallbackReason      string             `json:"fallback_reason,omitempty"`
	FinalDecisionSource string             `json:"final_decision_source"` // rule / semantic / fallback
	RuleScore           float64            `json:"rule_score"`
	SemanticScore       float64            `json:"semantic_score"`
}

// SemanticDecisionOutput 语义决策输出
type SemanticDecisionOutput struct {
	PreferredPool        PreferredPool        `json:"preferred_pool"`
	TaskType             TaskType             `json:"task_type"`
	Modality             Modality             `json:"modality"`
	RequiredCapabilities RequiredCapabilities `json:"required_capabilities"`
	MatchedRule          string               `json:"matched_rule"`
	Confidence           float64              `json:"confidence"`
}

// TierDecisionOutput Tier 决策输出
type TierDecisionOutput struct {
	PreferredTier PreferredTier `json:"preferred_tier"`
	Reason        string        `json:"reason"`
	MatchedRule   string        `json:"matched_rule"`
}

// SchedulerDecisionOutput Scheduler 决策输出
type SchedulerDecisionOutput struct {
	SelectedAccountID int64  `json:"selected_account_id"`
	SelectedModel     string `json:"selected_model"`
	SchedulerLayer    string `json:"scheduler_layer"` // previous_response_id, session_sticky, load_balance
	PoolUsed          string `json:"pool_used"`
	AccountHealth     string `json:"account_health"`
	MatchedTier       string `json:"matched_tier"`
}

// HandleDebugRoute 处理 /v1/debug/route 请求
func HandleDebugRoute(
	w http.ResponseWriter,
	r *http.Request,
	multiLayerRouter *MultiLayerRouter,
	tierRouter TierRouter,
	scheduler SchedulerFacade,
	logger RoutingDecisionLogger,
) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error": "method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req DebugRouteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "invalid request: %v"}`, err), http.StatusBadRequest)
		return
	}

	// 构建路由请求
	routeReq := buildRouteRequest(&req)

	// 1. 多层语义路由（规则 + 语义相似度）
	multiLayerDecision := multiLayerRouter.Route(routeReq)

	// 转换为 SemanticDecision 用于后续处理
	semanticDecision := &SemanticRouteDecision{
		PreferredPool:        multiLayerDecision.PreferredPool,
		TaskType:             multiLayerDecision.TaskType,
		Modality:             multiLayerDecision.Modality,
		RequiredCapabilities: multiLayerDecision.RequiredCapabilities,
		MatchedRule:          multiLayerDecision.MatchedRules[0],
		Confidence:           multiLayerDecision.Confidence,
	}

	// 2. Tier 路由
	tierDecision, err := tierRouter.Route(map[string]interface{}{
		"prompt": extractPrompt(req.Messages),
	}, req.Model, semanticDecision.TaskType)
	if err != nil {
		sendError(w, fmt.Sprintf("tier routing failed: %v", err))
		return
	}

	// 3. Scheduler 调度
	schedulerResult := scheduler.Select(&SchedulerSelectRequest{
		Model:                req.Model,
		PreferredGroup:       PhysicalGroupForPool(determinePoolFromSemantic(semanticDecision)),
		PreferredPool:        determinePoolFromSemantic(semanticDecision),
		PreferredTier:        tierDecision.PreferredTier,
		TaskType:             semanticDecision.TaskType,
		RequiredCapabilities: semanticDecision.RequiredCapabilities,
		PreviousResponseID:   req.PreviousResponseID,
		SessionHash:          req.SessionHash,
	})

	// 检查调度是否成功
	if schedulerResult.Error != nil || schedulerResult.SelectedAccountID == 0 {
		// 调度失败，返回明确错误
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200) // 返回 200，但 success=false
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "no_schedulable_account",
			"reason":  "no available account in pool",
			"semantic_decision": map[string]string{
				"preferred_pool": string(semanticDecision.PreferredPool),
				"task_type":      string(semanticDecision.TaskType),
			},
			"tier_decision": map[string]string{
				"preferred_tier": string(tierDecision.PreferredTier),
			},
			"explain": map[string]interface{}{
				"matched_rules":         multiLayerDecision.MatchedRules,
				"confidence":            Round(multiLayerDecision.Confidence),
				"fallback_reason":       multiLayerDecision.FallbackReason,
				"final_decision_source": string(multiLayerDecision.DecisionSource),
				"rule_score":            Round(multiLayerDecision.RuleScore),
				"semantic_score":        Round(multiLayerDecision.SemanticScore),
			},
		})
		return
	}

	// 4. 记录路由决策
	if logger != nil {
		combined := &CombinedRouteDecision{
			Semantic:  *semanticDecision,
			Tier:      *tierDecision,
			FinalPool: determinePoolFromSemantic(semanticDecision),
			Timestamp: time.Now(),
		}
		_ = logger.LogDecision(combined, generateRequestID())
	}

	// 构建响应
	response := DebugRouteResponse{
		Success: true,
		SemanticDecision: SemanticDecisionOutput{
			PreferredPool:        semanticDecision.PreferredPool,
			TaskType:             semanticDecision.TaskType,
			Modality:             semanticDecision.Modality,
			RequiredCapabilities: semanticDecision.RequiredCapabilities,
			MatchedRule:          semanticDecision.MatchedRule,
			Confidence:           Round(semanticDecision.Confidence),
		},
		TierDecision: TierDecisionOutput{
			PreferredTier: tierDecision.PreferredTier,
			Reason:        tierDecision.Reason,
			MatchedRule:   tierDecision.MatchedRule,
		},
		SchedulerDecision: SchedulerDecisionOutput{
			SelectedAccountID: schedulerResult.SelectedAccountID,
			SelectedModel:     schedulerResult.SelectedModel,
			SchedulerLayer:    schedulerResult.Layer,
			PoolUsed:          schedulerResult.PoolUsed,
			AccountHealth:     schedulerResult.AccountHealth,
			MatchedTier:       string(tierDecision.PreferredTier),
		},
		Explain: &ExplainOutput{
			MatchedRules:        multiLayerDecision.MatchedRules,
			SemanticScores:      multiLayerDecision.SemanticScores,
			Confidence:          Round(multiLayerDecision.Confidence),
			FallbackReason:      multiLayerDecision.FallbackReason,
			FinalDecisionSource: string(multiLayerDecision.DecisionSource),
			RuleScore:           Round(multiLayerDecision.RuleScore),
			SemanticScore:       Round(multiLayerDecision.SemanticScore),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// buildRouteRequest 构建路由请求
func buildRouteRequest(req *DebugRouteRequest) *RouteRequest {
	hasImage := len(req.Images) > 0 || containsImageMessage(req.Messages)
	hasDocument := len(req.Documents) > 0 || containsDocumentMessage(req.Messages)

	return &RouteRequest{
		Model:        req.Model,
		Prompt:       extractPrompt(req.Messages),
		HasImage:     hasImage,
		HasDocument:  hasDocument,
		DocumentType: detectDocumentTypeFromReq(req),
		FileNames:    req.Documents,
		Messages:     req.Messages,
	}
}

// extractPrompt 从 messages 中提取 prompt
func extractPrompt(messages []MessageContent) string {
	var sb strings.Builder
	for _, msg := range messages {
		sb.WriteString(msg.Content)
		sb.WriteString("\n")
	}
	return sb.String()
}

// containsImageMessage 检查是否包含图片消息
func containsImageMessage(messages []MessageContent) bool {
	for _, msg := range messages {
		if msg.Type == "image_url" {
			return true
		}
	}
	return false
}

// containsDocumentMessage 检查是否包含文档消息
func containsDocumentMessage(messages []MessageContent) bool {
	for _, msg := range messages {
		if msg.Type == "document" {
			return true
		}
	}
	return false
}

// detectDocumentTypeFromReq 从请求中检测文档类型
func detectDocumentTypeFromReq(req *DebugRouteRequest) string {
	for _, doc := range req.Documents {
		lower := strings.ToLower(doc)
		if strings.HasSuffix(lower, ".docx") {
			return "docx"
		}
		if strings.HasSuffix(lower, ".doc") {
			return "doc"
		}
		if strings.HasSuffix(lower, ".pdf") {
			return "pdf"
		}
	}
	return ""
}

// determinePoolFromSemantic 从语义决策确定账号池
func determinePoolFromSemantic(decision *SemanticRouteDecision) PreferredPool {
	return decision.PreferredPool
}

// sendError 发送错误响应
func sendError(w http.ResponseWriter, err string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(map[string]string{"error": err})
}
