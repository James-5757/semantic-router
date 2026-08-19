package semanticrouter

import (
	"regexp"
	"strings"
)

// SemanticRoutingRule 语义路由规则定义
type SemanticRoutingRule struct {
	Name            string
	Priority        int
	Enabled         bool
	ModelPattern    string
	PromptContains  string
	PromptRegex     string
	ContentType     string
	HasImage        *bool
	HasDocument     *bool
	DocumentType    string // 如 "docx", "pdf", "doc"
	TaskType        TaskType
	Modality        Modality
	PreferredPool   PreferredPool
	VisionCapable   bool
	DocumentCapable bool
	Confidence      float64

	// 运行时编译的正则表达式
	promptRegexCompiled *regexp.Regexp
	modelRegexCompiled  *regexp.Regexp
}

// Compile 编译正则表达式
func (r *SemanticRoutingRule) Compile() error {
	if r.PromptRegex != "" {
		re, err := regexp.Compile(r.PromptRegex)
		if err != nil {
			return err
		}
		r.promptRegexCompiled = re
	}
	if r.ModelPattern != "" {
		pattern := convertGlobToRegex(r.ModelPattern)
		re, err := regexp.Compile(pattern)
		if err != nil {
			return err
		}
		r.modelRegexCompiled = re
	}
	return nil
}

// Match 检查请求是否匹配此规则
func (r *SemanticRoutingRule) Match(req *RouteRequest) bool {
	if !r.Enabled {
		return false
	}

	// 检查模型匹配
	if r.modelRegexCompiled != nil {
		if req.Model == "" || !r.modelRegexCompiled.MatchString(req.Model) {
			return false
		}
	}

	// 检查 prompt 包含（不区分大小写）
	if r.PromptContains != "" && req.Prompt != "" {
		if !strings.Contains(strings.ToLower(req.Prompt), strings.ToLower(r.PromptContains)) {
			return false
		}
	}

	// 检查 prompt 正则
	if r.promptRegexCompiled != nil && req.Prompt != "" {
		if !r.promptRegexCompiled.MatchString(req.Prompt) {
			return false
		}
	}

	// 检查 Content-Type
	if r.ContentType != "" && req.ContentType != "" {
		if !strings.Contains(req.ContentType, r.ContentType) {
			return false
		}
	}

	// 检查是否包含图片
	if r.HasImage != nil {
		if req.HasImage != *r.HasImage {
			return false
		}
	}

	// 检查是否包含文档
	if r.HasDocument != nil {
		if req.HasDocument != *r.HasDocument {
			return false
		}
	}

	return true
}

// RouteRequest 路由请求
type RouteRequest struct {
	Model        string
	Prompt       string
	ContentType  string
	HasImage     bool
	HasDocument  bool
	HasCSV       bool   // 新增：是否有 CSV 附件
	DocumentType string // 如 "docx", "pdf", "doc"
	FileNames    []string
	Messages     []MessageContent // 用于更详细的分析
}

// MessageContent 消息内容
type MessageContent struct {
	Role    string
	Content string
	Type    string // "text", "image_url", "document"
}

// convertGlobToRegex 将通配符模式转换为正则表达式
func convertGlobToRegex(pattern string) string {
	// 转义特殊正则字符，但保留通配符
	result := strings.ReplaceAll(pattern, `\`, `\\`)
	result = strings.ReplaceAll(result, `.`, `\.`)
	result = strings.ReplaceAll(result, `+`, `\+`)
	result = strings.ReplaceAll(result, `(`, `\(`)
	result = strings.ReplaceAll(result, `)`, `\)`)
	result = strings.ReplaceAll(result, `[`, `\[`)
	result = strings.ReplaceAll(result, `]`, `\]`)
	result = strings.ReplaceAll(result, `{`, `\{`)
	result = strings.ReplaceAll(result, `}`, `\}`)
	result = strings.ReplaceAll(result, `^`, `\^`)
	result = strings.ReplaceAll(result, `$`, `\$`)
	result = strings.ReplaceAll(result, `|`, `\|`)

	// 转换通配符
	result = strings.ReplaceAll(result, `*`, `.*`)
	result = strings.ReplaceAll(result, `?`, `.`)

	return `(?i)` + result // 不区分大小写
}

// DefaultSemanticRules 返回默认的语义路由规则
func DefaultSemanticRules() []*SemanticRoutingRule {
	rules := []*SemanticRoutingRule{
		// === 图片生成规则 (最高优先级) ===
		// 暂时禁用 dalle 规则，因为它太宽泛
		// {
		// 	Name:           "image_generation_dalle",
		// 	Priority:       100,
		// 	Enabled:        true,
		// 	ModelPattern:   "dalle-*",
		// 	TaskType:       TaskTypeImageGenerate,
		// 	Modality:       ModalityTextOnly,
		// 	PreferredPool:  PoolDefault,
		// 	Confidence:     1.0,
		// },
		// 恢复 dalle 规则用于测试 - 只精确匹配 dalle-3
		{
			Name:          "image_generation_dalle",
			Priority:      100,
			Enabled:       true,
			ModelPattern:  "dalle-3",
			TaskType:      TaskTypeImageGenerate,
			Modality:      ModalityTextOnly,
			PreferredPool: PoolDefault,
			Confidence:    1.0,
		},
		{
			Name:          "image_generation_gpt_image",
			Priority:      100,
			Enabled:       false, // 暂时禁用：规则匹配逻辑有问题，需要修复
			ModelPattern:  "gpt-image.*",
			TaskType:      TaskTypeImageGenerate,
			Modality:      ModalityTextOnly,
			PreferredPool: PoolVision,
			VisionCapable: true,
			Confidence:    1.0,
		},
		{
			Name:           "image_generation",
			Priority:       90,
			Enabled:        true,
			HasImage:       boolPtr(false),
			PromptContains: "image",
			TaskType:       TaskTypeImageGenerate,
			Modality:       ModalityTextOnly,
			PreferredPool:  PoolDefault,
			Confidence:     0.7,
		},

		// === 视觉/图片理解规则 ===
		// 暂时禁用 vision_by_model，因为它会误匹配
		// {
		// 	Name:          "vision_by_model",
		// 	Priority:     80,
		// 	Enabled:      true,
		// 	ModelPattern: "*-vision-*",
		// 	TaskType:     TaskTypeVision,
		// 	Modality:     ModalityTextImage,
		// 	PreferredPool: PoolVision,
		// 	VisionCapable: true,
		// 	Confidence:   1.0,
		// },
		// 暂时禁用所有视觉模型规则，因为它们会误匹配
		// {
		// 	Name:          "vision_gpt4o",
		// 	Priority:     80,
		// 	Enabled:      true,
		// 	ModelPattern: "gpt-4o",
		// 	TaskType:     TaskTypeVision,
		// 	Modality:     ModalityTextImage,
		// 	PreferredPool: PoolVision,
		// 	VisionCapable: true,
		// 	Confidence:   1.0,
		// },
		{
			Name:          "vision_by_content",
			Priority:      70,
			Enabled:       true,
			HasImage:      boolPtr(true),
			TaskType:      TaskTypeVision,
			Modality:      ModalityTextImage,
			PreferredPool: PoolVision,
			VisionCapable: true,
			Confidence:    0.9,
		},

		// === 文档处理规则 ===
		{
			Name:            "document_docx",
			Priority:        85,
			Enabled:         true,
			HasDocument:     boolPtr(true),
			DocumentType:    "docx",
			TaskType:        TaskTypeDocument,
			Modality:        ModalityDocument,
			PreferredPool:   PoolDocument,
			DocumentCapable: true,
			Confidence:      1.0,
		},

		// === 代码生成规则 ===
		// 暂时禁用 code_by_model，因为它会误匹配普通请求
		// {
		// 	Name:           "code_by_model",
		// 	Priority:       95,
		// 	Enabled:        true,
		// 	ModelPattern:   "code-*",
		// 	TaskType:       TaskTypeCode,
		// 	Modality:       ModalityTextOnly,
		// 	PreferredPool:  PoolCode,
		// 	Confidence:     1.0,
		// },
		{
			Name:           "code_by_prompt",
			Priority:       75,
			Enabled:        true,
			PromptContains: "```",
			TaskType:       TaskTypeCode,
			Modality:       ModalityTextOnly,
			PreferredPool:  PoolCode,
			Confidence:     0.6,
		},
		{
			Name:           "code_by_keywords_en",
			Priority:       70,
			Enabled:        true,
			PromptContains: "write code",
			TaskType:       TaskTypeCode,
			Modality:       ModalityTextOnly,
			PreferredPool:  PoolCode,
			Confidence:     0.8,
		},
		{
			Name:           "code_by_keywords_en2",
			Priority:       70,
			Enabled:        true,
			PromptContains: "write a function",
			TaskType:       TaskTypeCode,
			Modality:       ModalityTextOnly,
			PreferredPool:  PoolCode,
			Confidence:     0.8,
		},
		{
			Name:           "code_by_keywords_en3",
			Priority:       70,
			Enabled:        true,
			PromptContains: "implement",
			TaskType:       TaskTypeCode,
			Modality:       ModalityTextOnly,
			PreferredPool:  PoolCode,
			Confidence:     0.7,
		},
		{
			Name:           "code_by_keywords_en4",
			Priority:       70,
			Enabled:        true,
			PromptContains: "quick sort",
			TaskType:       TaskTypeCode,
			Modality:       ModalityTextOnly,
			PreferredPool:  PoolCode,
			Confidence:     0.9,
		},
		{
			Name:           "code_by_keyword_code",
			Priority:       72,
			Enabled:        true,
			PromptContains: "code",
			TaskType:       TaskTypeCode,
			Modality:       ModalityTextOnly,
			PreferredPool:  PoolCode,
			Confidence:     0.65,
		},
		{
			Name:           "code_by_keyword_debug",
			Priority:       73,
			Enabled:        true,
			PromptContains: "debug",
			TaskType:       TaskTypeCode,
			Modality:       ModalityTextOnly,
			PreferredPool:  PoolCode,
			Confidence:     0.7,
		},
		{
			Name:           "code_by_keyword_fix",
			Priority:       73,
			Enabled:        true,
			PromptContains: "fix",
			TaskType:       TaskTypeCode,
			Modality:       ModalityTextOnly,
			PreferredPool:  PoolCode,
			Confidence:     0.7,
		},
		{
			Name:           "code_by_keyword_error",
			Priority:       73,
			Enabled:        true,
			PromptContains: "error",
			TaskType:       TaskTypeCode,
			Modality:       ModalityTextOnly,
			PreferredPool:  PoolCode,
			Confidence:     0.7,
		},
		{
			Name:           "code_by_language_python",
			Priority:       74,
			Enabled:        true,
			PromptContains: "python",
			TaskType:       TaskTypeCode,
			Modality:       ModalityTextOnly,
			PreferredPool:  PoolCode,
			Confidence:     0.7,
		},
		{
			Name:           "code_by_language_javascript",
			Priority:       74,
			Enabled:        true,
			PromptContains: "javascript",
			TaskType:       TaskTypeCode,
			Modality:       ModalityTextOnly,
			PreferredPool:  PoolCode,
			Confidence:     0.7,
		},
		{
			Name:           "code_by_keyword_program",
			Priority:       73,
			Enabled:        true,
			PromptContains: "program",
			TaskType:       TaskTypeCode,
			Modality:       ModalityTextOnly,
			PreferredPool:  PoolCode,
			Confidence:     0.7,
		},
		{
			Name:           "code_by_keyword_function",
			Priority:       73,
			Enabled:        true,
			PromptContains: "function",
			TaskType:       TaskTypeCode,
			Modality:       ModalityTextOnly,
			PreferredPool:  PoolCode,
			Confidence:     0.7,
		},
		{
			Name:           "code_by_keyword_algorithm",
			Priority:       74,
			Enabled:        true,
			PromptContains: "algorithm",
			TaskType:       TaskTypeCode,
			Modality:       ModalityTextOnly,
			PreferredPool:  PoolCode,
			Confidence:     0.8,
		},
		{
			Name:           "code_by_keyword_machine_learning",
			Priority:       74,
			Enabled:        true,
			PromptContains: "machine learning",
			TaskType:       TaskTypeCode,
			Modality:       ModalityTextOnly,
			PreferredPool:  PoolCode,
			Confidence:     0.8,
		},
		{
			Name:           "code_by_language_sql",
			Priority:       74,
			Enabled:        true,
			PromptContains: "sql",
			TaskType:       TaskTypeCode,
			Modality:       ModalityTextOnly,
			PreferredPool:  PoolCode,
			Confidence:     0.7,
		},
		{
			Name:           "code_by_keywords_cn5",
			Priority:       70,
			Enabled:        true,
			PromptContains: "报错",
			TaskType:       TaskTypeCode,
			Modality:       ModalityTextOnly,
			PreferredPool:  PoolCode,
			Confidence:     0.8,
		},
		{
			Name:           "code_by_keywords_cn6",
			Priority:       70,
			Enabled:        true,
			PromptContains: "修复",
			TaskType:       TaskTypeCode,
			Modality:       ModalityTextOnly,
			PreferredPool:  PoolCode,
			Confidence:     0.7,
		},
		{
			Name:           "code_by_keywords_cn7",
			Priority:       70,
			Enabled:        true,
			PromptContains: "调试",
			TaskType:       TaskTypeCode,
			Modality:       ModalityTextOnly,
			PreferredPool:  PoolCode,
			Confidence:     0.7,
		},
		{
			Name:           "code_by_keywords_cn",
			Priority:       70,
			Enabled:        true,
			PromptContains: "写代码",
			TaskType:       TaskTypeCode,
			Modality:       ModalityTextOnly,
			PreferredPool:  PoolCode,
			Confidence:     0.8,
		},
		{
			Name:           "code_by_keywords_cn2",
			Priority:       70,
			Enabled:        true,
			PromptContains: "写一个函数",
			TaskType:       TaskTypeCode,
			Modality:       ModalityTextOnly,
			PreferredPool:  PoolCode,
			Confidence:     0.8,
		},
		{
			Name:           "code_by_keywords_cn3",
			Priority:       70,
			Enabled:        true,
			PromptContains: "实现",
			TaskType:       TaskTypeCode,
			Modality:       ModalityTextOnly,
			PreferredPool:  PoolCode,
			Confidence:     0.7,
		},
		{
			Name:           "code_by_keywords_cn4",
			Priority:       70,
			Enabled:        true,
			PromptContains: "快排",
			TaskType:       TaskTypeCode,
			Modality:       ModalityTextOnly,
			PreferredPool:  PoolCode,
			Confidence:     0.9,
		},

		// === 默认文本对话规则 ===
		{
			Name:          "default_text",
			Priority:      1, // 最低优先级，确保不会被优先匹配
			Enabled:       true,
			HasImage:      boolPtr(false),
			HasDocument:   boolPtr(false),
			TaskType:      TaskTypeText,
			Modality:      ModalityTextOnly,
			PreferredPool: PoolDefault,
			Confidence:    0.3,
		},
	}

	// 编译所有规则
	for _, r := range rules {
		_ = r.Compile()
	}

	return rules
}

func boolPtr(b bool) *bool {
	return &b
}
