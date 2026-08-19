package semanticrouter

import (
	"sort"
	"strings"
)

// RuleBasedSemanticRouter 基于规则的语义路由器
type RuleBasedSemanticRouter struct {
	name   string
	rules  []*SemanticRoutingRule
}

// NewRuleBasedSemanticRouter 创建基于规则的语义路由器
func NewRuleBasedSemanticRouter() *RuleBasedSemanticRouter {
	router := &RuleBasedSemanticRouter{
		name:   "RuleBasedSemanticRouter",
		rules:  DefaultSemanticRules(),
	}
	// 按优先级排序
	sort.Slice(router.rules, func(i, j int) bool {
		return router.rules[i].Priority > router.rules[j].Priority
	})
	return router
}

// NewRuleBasedSemanticRouterWithRules 使用自定义规则创建路由器
func NewRuleBasedSemanticRouterWithRules(rules []*SemanticRoutingRule) *RuleBasedSemanticRouter {
	router := &RuleBasedSemanticRouter{
		name:   "RuleBasedSemanticRouter",
		rules:  rules,
	}
	// 编译所有规则
	for _, r := range router.rules {
		_ = r.Compile()
	}
	// 按优先级排序
	sort.Slice(router.rules, func(i, j int) bool {
		return router.rules[i].Priority > router.rules[j].Priority
	})
	return router
}

// Route 执行语义路由
func (r *RuleBasedSemanticRouter) Route(ctx interface{}, req interface{}) (*SemanticRouteDecision, error) {
	// 类型断言获取 RouteRequest
	routeReq, ok := req.(*RouteRequest)
	if !ok {
		// 尝试从 map 获取
		if m, ok := req.(map[string]interface{}); ok {
			routeReq = &RouteRequest{
				Model:       getStringFromMap(m, "model"),
				Prompt:      getStringFromMap(m, "prompt"),
				ContentType: getStringFromMap(m, "content_type"),
				HasImage:    getBoolFromMap(m, "has_image"),
				HasDocument: getBoolFromMap(m, "has_document"),
			}
		} else {
			// 默认返回文本路由
			return &SemanticRouteDecision{
				TaskType:       TaskTypeText,
				Modality:       ModalityTextOnly,
				PreferredPool:  PoolDefault,
				Confidence:     1.0,
				MatchedRule:    "default_fallback",
			}, nil
		}
	}

	// 检测文档类型
	if routeReq.HasDocument && routeReq.DocumentType == "" {
		routeReq.DocumentType = detectDocumentType(routeReq)
	}

	// 按优先级遍历规则，找到第一个匹配的
	for _, rule := range r.rules {
		if rule.Match(routeReq) {
			decision := &SemanticRouteDecision{
				TaskType:      rule.TaskType,
				Modality:      rule.Modality,
				PreferredPool: rule.PreferredPool,
				Confidence:    rule.Confidence,
				MatchedRule:   rule.Name,
				RequiredCapabilities: RequiredCapabilities{
					VisionCapable:   rule.VisionCapable,
					DocumentCapable: rule.DocumentCapable,
					ImageCapability: getImageCapability(rule),
				},
			}

			// 设置处理提示
			if rule.TaskType == TaskTypeDocument {
				decision.ProcessingHint = "requires_file_parsing"
			}

			return decision, nil
		}
	}

	// 默认返回文本路由
	return &SemanticRouteDecision{
		TaskType:       TaskTypeText,
		Modality:       ModalityTextOnly,
		PreferredPool:  PoolDefault,
		Confidence:     1.0,
		MatchedRule:    "default_fallback",
	}, nil
}

// GetName 获取路由名称
func (r *RuleBasedSemanticRouter) GetName() string {
	return r.name
}

// AddRule 添加路由规则
func (r *RuleBasedSemanticRouter) AddRule(rule *SemanticRoutingRule) error {
	if err := rule.Compile(); err != nil {
		return err
	}
	r.rules = append(r.rules, rule)
	// 重新排序
	sort.Slice(r.rules, func(i, j int) bool {
		return r.rules[i].Priority > r.rules[j].Priority
	})
	return nil
}

// GetRules 获取所有规则
func (r *RuleBasedSemanticRouter) GetRules() []*SemanticRoutingRule {
	return r.rules
}

// detectDocumentType 检测文档类型
func detectDocumentType(req *RouteRequest) string {
	// 检查文件名
	for _, fname := range req.FileNames {
		lower := strings.ToLower(fname)
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
	// 检查 Content-Type
	if strings.Contains(req.ContentType, "wordprocessingml") {
		return "docx"
	}
	if strings.Contains(req.ContentType, "pdf") {
		return "pdf"
	}
	return "unknown"
}

// getImageCapability 根据规则获取图片能力要求
func getImageCapability(rule *SemanticRoutingRule) ImageCapabilityType {
	if !rule.VisionCapable {
		return ImageCapabilityNone
	}
	// 对于图片生成使用 native，其他视觉任务使用 basic
	if rule.TaskType == TaskTypeImageGenerate {
		return ImageCapabilityNative
	}
	return ImageCapabilityBasic
}

// getStringFromMap 从 map 获取字符串
func getStringFromMap(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// getBoolFromMap 从 map 获取布尔值
func getBoolFromMap(m map[string]interface{}, key string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// 确保接口实现
var _ SemanticRouter = (*RuleBasedSemanticRouter)(nil)