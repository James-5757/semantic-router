package semanticrouter

import (
	"time"
)

// ExtendedScheduleRequest 扩展后的调度请求
// 在原有 OpenAIAccountScheduleRequest 基础上添加路由决策信息
type ExtendedScheduleRequest struct {
	// 原有字段
	GroupID                 *int64
	SessionHash             string
	StickyAccountID         int64
	PreviousResponseID      string
	RequestedModel          string
	RequiredTransport       string // OpenAIUpstreamTransport
	RequiredImageCapability string // OpenAIImagesCapability
	RequireCompact          bool
	ExcludedIDs             map[int64]struct{}
	PreserveStickyBinding   bool

	// === 新增：预路由决策信息 ===
	RoutingDecision *CombinedRouteDecision // 语义+Tier 路由决策
	PreferredPool    PreferredPool          // 语义路由输出的账号池
	PreferredTier    PreferredTier          // Tier 路由输出的强弱模型
	TaskType         TaskType                // 任务类型
	Modality         Modality                // 模态类型

	// 扩展能力要求
	VisionCapable   bool // 需要视觉能力
	DocumentCapable bool // 需要文档处理能力
}

// ToAccountScheduleRequest 转换为原有的 Scheduler 请求格式
func (e *ExtendedScheduleRequest) ToAccountScheduleRequest() map[string]interface{} {
	req := map[string]interface{}{
		"group_id":                  e.GroupID,
		"session_hash":              e.SessionHash,
		"sticky_account_id":         e.StickyAccountID,
		"previous_response_id":      e.PreviousResponseID,
		"requested_model":           e.RequestedModel,
		"required_transport":        e.RequiredTransport,
		"required_image_capability": e.RequiredImageCapability,
		"require_compact":           e.RequireCompact,
		"excluded_ids":              e.ExcludedIDs,
		"preserve_sticky_binding":   e.PreserveStickyBinding,
		// 扩展字段
		"preferred_pool":     e.PreferredPool,
		"preferred_tier":     e.PreferredTier,
		"task_type":          e.TaskType,
		"modality":           e.Modality,
		"vision_capable":     e.VisionCapable,
		"document_capable":   e.DocumentCapable,
	}
	return req
}

// NewExtendedScheduleRequestFromRouteRequest 从路由请求创建扩展请求
func NewExtendedScheduleRequest(
	groupID *int64,
	model string,
	sessionHash string,
	previousResponseID string,
	routingDecision *CombinedRouteDecision,
) *ExtendedScheduleRequest {
	req := &ExtendedScheduleRequest{
		GroupID:            groupID,
		RequestedModel:     model,
		SessionHash:        sessionHash,
		PreviousResponseID: previousResponseID,
		ExcludedIDs:        make(map[int64]struct{}),
	}

	// 填充路由决策信息
	if routingDecision != nil {
		req.RoutingDecision = routingDecision
		req.PreferredPool = routingDecision.FinalPool
		req.PreferredTier = routingDecision.Tier.PreferredTier
		req.TaskType = routingDecision.Semantic.TaskType
		req.Modality = routingDecision.Semantic.Modality
		req.VisionCapable = routingDecision.Semantic.RequiredCapabilities.VisionCapable
		req.DocumentCapable = routingDecision.Semantic.RequiredCapabilities.DocumentCapable
	}

	return req
}

// PreRouter 预路由编排器
// 负责协调 SemanticRouter 和 TierRouter，并输出最终的路由决策
type PreRouter struct {
	semanticRouter SemanticRouter
	tierRouter     TierRouter
	logger         RoutingDecisionLogger
}

// NewPreRouter 创建预路由编排器
func NewPreRouter(semanticRouter SemanticRouter, tierRouter TierRouter, logger RoutingDecisionLogger) *PreRouter {
	return &PreRouter{
		semanticRouter: semanticRouter,
		tierRouter:     tierRouter,
		logger:         logger,
	}
}

// PreRouteResult 预路由结果
type PreRouteResult struct {
	ExtendedRequest *ExtendedScheduleRequest
	Decision        *CombinedRouteDecision
	ShouldParseFile bool   // 是否需要先解析文件
	DocumentType    string // 文档类型 (docx, pdf 等)
}

// Route 执行完整的预路由流程
func (p *PreRouter) Route(
	groupID *int64,
	model string,
	sessionHash string,
	previousResponseID string,
	routeReq *RouteRequest,
) (*PreRouteResult, error) {
	// 1. 语义路由
	semanticDecision, err := p.semanticRouter.Route(nil, routeReq)
	if err != nil {
		return nil, err
	}

	// 2. Tier 路由
	tierDecision, err := p.tierRouter.Route(map[string]interface{}{
		"prompt": routeReq.Prompt,
	}, model, semanticDecision.TaskType)
	if err != nil {
		return nil, err
	}

	// 3. 计算最终账号池
	finalPool := determineFinalPool(semanticDecision.PreferredPool, tierDecision.PreferredTier)

	// 4. 构建组合决策
	combinedDecision := &CombinedRouteDecision{
		Semantic:     *semanticDecision,
		Tier:         *tierDecision,
		FinalPool:    finalPool,
		RequiresFileParsing: semanticDecision.TaskType == TaskTypeDocument,
		Timestamp:    time.Now(),
	}

	// 5. 创建扩展的调度请求
	extendedReq := NewExtendedScheduleRequest(
		groupID,
		model,
		sessionHash,
		previousResponseID,
		combinedDecision,
	)

	// 6. 记录路由决策日志
	if p.logger != nil {
		requestID := generateRequestID()
		_ = p.logger.LogDecision(combinedDecision, requestID)
	}

	return &PreRouteResult{
		ExtendedRequest: extendedReq,
		Decision:        combinedDecision,
		ShouldParseFile: combinedDecision.RequiresFileParsing,
		DocumentType:    routeReq.DocumentType,
	}, nil
}

// determineFinalPool 根据语义池和 Tier 确定最终账号池
func determineFinalPool(semanticPool PreferredPool, tier PreferredTier) PreferredPool {
	// 视觉任务强制使用 vision 池
	if semanticPool == PoolVision {
		return PoolVision
	}

	// 文档任务强制使用 document 池
	if semanticPool == PoolDocument {
		return PoolDocument
	}

	// 代码任务
	if semanticPool == PoolCode {
		return PoolCode
	}

	// 弱模型优先使用 cheap 池
	if tier == TierWeak {
		return PoolCheap
	}

	return semanticPool
}

// generateRequestID 生成请求ID
func generateRequestID() string {
	return time.Now().Format("20060102150405.000000")
}

// GetSemanticRouter 获取语义路由器
func (p *PreRouter) GetSemanticRouter() SemanticRouter {
	return p.semanticRouter
}

// GetTierRouter 获取 Tier 路由器
func (p *PreRouter) GetTierRouter() TierRouter {
	return p.tierRouter
}