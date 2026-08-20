package semanticrouter

import (
	"regexp"
	"strings"
)

// TaskSchema 结构化任务理解
// 这是路由决策的核心输入，必须先于 Pool 决策生成
type TaskSchema struct {
	// 动作：用户想要执行的操作
	Actions []string `json:"actions"`
	// 对象：操作的目标对象
	Objects []string `json:"objects"`
	// 领域：任务所属领域
	Domains []string `json:"domains"`
	// 输入模态：text, image, document, csv, audio
	InputModalities []string `json:"input_modalities"`
	// 输出产物：code, chart, image, text, document
	OutputArtifacts []string `json:"output_artifacts"`
	// 约束条件
	Constraints []string `json:"constraints"`
	// 主要意图
	PrimaryIntent string `json:"primary_intent"`
	// 次要意图
	SecondaryIntents []string `json:"secondary_intents"`
	// 所需能力
	RequiredCapabilities []string `json:"required_capabilities"`
	// 置信度
	Confidence float64 `json:"confidence"`
	// 是否歧义
	Ambiguous bool `json:"ambiguous"`
	// 缺失输入
	MissingInputs []string `json:"missing_inputs"`
	// 原始 Prompt
	OriginalPrompt string `json:"original_prompt"`
}

// TaskUnderstandingEngine 结构化任务理解引擎
type TaskUnderstandingEngine struct {
	// 动作模式
	actionPatterns map[string][]*regexp.Regexp
	// 对象模式
	objectPatterns map[string][]*regexp.Regexp
	// 领域模式
	domainPatterns map[string][]*regexp.Regexp
	// 编程语言
	programmingLanguages []string
	// 图像相关词汇
	imageKeywords []string
	// 文档相关词汇
	documentKeywords []string
	// 数据相关词汇
	dataKeywords []string
	// 代码动作关键词
	codeActionKeywords []string
	// 技术对象关键词
	technicalObjectKeywords []string
}

// NewTaskUnderstandingEngine 创建任务理解引擎
func NewTaskUnderstandingEngine() *TaskUnderstandingEngine {
	return &TaskUnderstandingEngine{
		programmingLanguages: []string{
			"python", "javascript", "java", "golang", "go", "rust", "typescript",
			"c++", "c#", "php", "ruby", "swift", "kotlin", "scala", "sql",
			"html", "css", "react", "vue", "angular", "node", "django", "flask",
			"spring", "laravel", "express", "fastapi", "pandas", "numpy", "torch",
			"tensorflow", "pytorch", "scikit", "sklearn", "keras",
		},
		imageKeywords: []string{
			"image", "photo", "picture", "screenshot", "图片", "照片", "截图",
			"这张图", "照片里", "图片中", "图上", "图中",
		},
		documentKeywords: []string{
			"document", "doc", "docx", "pdf", "word", "文档", "文章", "论文",
			"合同", "报告", "摘要", "大纲",
		},
		dataKeywords: []string{
			"data", "csv", "excel", "table", "数据", "表格", "分析", "统计",
			"chart", "graph", "可视化", "指标", "趋势",
		},
		codeActionKeywords: []string{
			"code", "编程", "写代码", "开发", "实现", "implement", "write",
			"debug", "调试", "fix", "修复", "refactor", "重构", "build",
			"create", "函数", "class", "method", "api", "接口", "算法",
			"程序", "脚本", "自动化", "爬虫",
		},
		technicalObjectKeywords: []string{
			"function", "class", "api", "接口", "database", "数据库", "sql",
			"algorithm", "算法", "code", "代码", "script", "脚本", "service",
			"微服务", "middleware", "中间件", "cache", "缓存", "queue", "队列",
		},
	}
}

// Understand 执行结构化任务理解
// 必须在 Pool 决策之前执行
func (e *TaskUnderstandingEngine) Understand(prompt string, hasImage, hasDocument, hasCSV bool) *TaskSchema {
	schema := &TaskSchema{
		OriginalPrompt: prompt,
	}

	promptLower := strings.ToLower(prompt)

	// 1. 识别输入模态
	schema.InputModalities = e.recognizeInputModalities(promptLower, hasImage, hasDocument, hasCSV)

	// 2. 识别动作
	schema.Actions = e.recognizeActions(promptLower)

	// 3. 识别对象
	schema.Objects = e.recognizeObjects(promptLower)

	// 4. 识别领域
	schema.Domains = e.recognizeDomains(promptLower)

	// 5. 识别输出产物
	schema.OutputArtifacts = e.recognizeOutputArtifacts(promptLower)

	// 6. 识别约束
	schema.Constraints = e.recognizeConstraints(promptLower)

	// 7. 确定意图
	schema.PrimaryIntent, schema.SecondaryIntents = e.determineIntents(schema)

	// 8. 确定所需能力
	schema.RequiredCapabilities = e.determineRequiredCapabilities(schema)

	// 9. 计算置信度
	schema.Confidence = e.calculateConfidence(schema)

	// 10. 检查歧义
	schema.Ambiguous, schema.MissingInputs = e.checkAmbiguity(schema)

	return schema
}

// recognizeInputModalities 识别输入模态
func (e *TaskUnderstandingEngine) recognizeInputModalities(prompt string, hasImage, hasDocument, hasCSV bool) []string {
	modalities := []string{"text"}

	if hasImage || e.hasImageReference(prompt) {
		modalities = append(modalities, "image")
	}
	if hasDocument {
		modalities = append(modalities, "document")
	}
	if hasCSV {
		modalities = append(modalities, "csv")
	}

	return modalities
}

// hasImageReference 检查 prompt 中是否有图像引用
func (e *TaskUnderstandingEngine) hasImageReference(prompt string) bool {
	for _, kw := range e.imageKeywords {
		if strings.Contains(prompt, kw) {
			return true
		}
	}
	return false
}

// recognizeActions 识别动作
func (e *TaskUnderstandingEngine) recognizeActions(prompt string) []string {
	actions := []string{}

	// 编程相关动作
	codeActions := map[string][]string{
		"write":     {"写", "编写", "实现", "创建", "开发", "write", "code", "implement", "create", "build"},
		"debug":     {"调试", "debug", "fix", "修复", "报错", "error", "bug", "问题"},
		"refactor":  {"重构", "refactor", "优化", "优化代码"},
		"explain":   {"解释", "explain", "介绍", "说明", "讲解"},
		"analyze":   {"分析", "analyze", "分析", "分析一下"},
		"generate":  {"生成", "生成", "create", "generate"},
		"transform": {"转换", "convert", "transform", "改写"},
		"query":     {"查询", "query", "搜索", "查找"},
		"plan":      {"规划", "plan", "设计", "方案", "思路"},
	}

	for action, keywords := range codeActions {
		for _, kw := range keywords {
			if strings.Contains(prompt, kw) {
				actions = append(actions, action)
				break
			}
		}
	}

	return uniqueStrings(actions)
}

// recognizeObjects 识别对象
func (e *TaskUnderstandingEngine) recognizeObjects(prompt string) []string {
	objects := []string{}

	// 技术对象
	technicalObjects := []string{
		"function", "class", "method", "api", "接口", "database", "数据库",
		"sql", "algorithm", "算法", "code", "代码", "script", "脚本",
		"service", "微服务", "middleware", "中间件", "cache", "缓存",
		"queue", "队列", "endpoint", "authentication", "登录", "注册",
	}

	for _, obj := range technicalObjects {
		if strings.Contains(prompt, obj) {
			objects = append(objects, obj)
		}
	}

	// 检查是否有编程语言
	for _, lang := range e.programmingLanguages {
		if strings.Contains(prompt, lang) {
			objects = append(objects, "programming_language:"+lang)
		}
	}

	return uniqueStrings(objects)
}

// recognizeDomains 识别领域
func (e *TaskUnderstandingEngine) recognizeDomains(prompt string) []string {
	domains := []string{}

	domainKeywords := map[string][]string{
		"programming":  {"代码", "编程", "开发", "实现", "写代码", "code", "programming", "implement", "develop"},
		"data_science": {"数据", "分析", "统计", "模型", "预测", "data", "analysis", "statistics", "model", "ml"},
		"vision":       {"图片", "图像", "图片", "照片", "image", "photo", "picture", "vision"},
		"document":     {"文档", "文章", "pdf", "word", "doc", "document"},
		"general":      {"聊天", "问答", "解释", "介绍", "chat", "qa", "explain"},
	}

	for domain, keywords := range domainKeywords {
		for _, kw := range keywords {
			if strings.Contains(prompt, kw) {
				domains = append(domains, domain)
				break
			}
		}
	}

	return uniqueStrings(domains)
}

// recognizeOutputArtifacts 识别输出产物
func (e *TaskUnderstandingEngine) recognizeOutputArtifacts(prompt string) []string {
	artifacts := []string{}
	if isImagePromptAuthoringRequest(prompt) {
		return []string{"image_prompt_text"}
	}

	// 输出产物关键词
	outputKeywords := map[string][]string{
		"executable_code": {"代码", "函数", "程序", "script", "code", "function", "program", "实现"},
		"chart":           {"图表", "图", "折线", "柱状", "饼图", "chart", "graph", "visualization"},
		"image":           {"图片", "生成图片", "海报", "画图", "image", "picture", "generate"},
		"text":            {"文本", "回答", "解释", "说明", "text", "answer", "explanation"},
		"document":        {"文档", "报告", "总结", "document", "report", "summary"},
		"data":            {"数据", "表格", "csv", "data", "table"},
		"analysis":        {"分析", "分析结果", "分析报告", "analysis", "result"},
	}

	for artifact, keywords := range outputKeywords {
		for _, kw := range keywords {
			if strings.Contains(prompt, kw) {
				artifacts = append(artifacts, artifact)
				break
			}
		}
	}

	return uniqueStrings(artifacts)
}

// recognizeConstraints 识别约束
func (e *TaskUnderstandingEngine) recognizeConstraints(prompt string) []string {
	constraints := []string{}

	constraintKeywords := []string{
		"安全", "security", "高并发", "high concurrency", "性能", "performance",
		"优化", "optimize", "debug", "调试", "容灾", "disaster",
	}

	for _, kw := range constraintKeywords {
		if strings.Contains(prompt, kw) {
			constraints = append(constraints, kw)
		}
	}

	return uniqueStrings(constraints)
}

// determineIntents 确定意图
func (e *TaskUnderstandingEngine) determineIntents(schema *TaskSchema) (string, []string) {
	if isImagePromptAuthoringRequest(schema.OriginalPrompt) {
		return "general_chat", []string{"image_prompt_authoring"}
	}

	// 基于动作和领域确定主要意图
	hasCodeAction := containsString(schema.Actions, "write", "debug", "refactor")
	hasDataDomain := containsString(schema.Domains, "data_science")
	hasVisionInput := containsString(schema.InputModalities, "image")
	hasDocumentInput := containsString(schema.InputModalities, "document")
	hasCodeOutput := containsString(schema.OutputArtifacts, "executable_code")
	hasImageOutput := containsString(schema.OutputArtifacts, "image")
	hasChartOutput := containsString(schema.OutputArtifacts, "chart")

	// 判断优先级：
	// 1. 有图像输入 -> 图像理解
	// 2. 有文档输入 -> 文档处理
	// 3. 有代码输出且有代码动作 -> 代码生成
	// 4. 有数据领域且有图表输出 -> 数据分析
	// 5. 有图像生成意图 -> 图像生成
	// 6. 有代码动作但无明确输出 -> 代码生成

	if hasVisionInput && !hasImageOutput {
		return "image_understanding", []string{}
	}

	if hasDocumentInput {
		return "document_processing", []string{}
	}

	if hasCodeOutput || (hasCodeAction && !hasDataDomain) {
		return "code_generation", []string{}
	}

	if hasDataDomain && (hasChartOutput || containsString(schema.OutputArtifacts, "data")) {
		return "data_analysis", []string{}
	}

	if hasImageOutput {
		return "image_generation", []string{}
	}

	// 检查是否有明确的生成图像意图
	promptLower := strings.ToLower(schema.OriginalPrompt)
	imageGenKeywords := []string{"生成图片", "画图", "海报", "插画", "generate image", "create image", "draw"}
	for _, kw := range imageGenKeywords {
		if strings.Contains(promptLower, kw) && !hasVisionInput {
			return "image_generation", []string{}
		}
	}

	return "general_chat", []string{}
}

// determineRequiredCapabilities 确定所需能力
func (e *TaskUnderstandingEngine) determineRequiredCapabilities(schema *TaskSchema) []string {
	capabilities := []string{}

	switch schema.PrimaryIntent {
	case "image_understanding":
		capabilities = append(capabilities, "vision", "image_understanding")
	case "code_generation":
		capabilities = append(capabilities, "code", "code_generation")
	case "data_analysis":
		capabilities = append(capabilities, "data_science", "data_analysis")
	case "document_processing":
		capabilities = append(capabilities, "document", "document_processing")
	case "image_generation":
		capabilities = append(capabilities, "image_generation", "creative_design")
	default:
		capabilities = append(capabilities, "general_chat")
	}

	return uniqueStrings(capabilities)
}

// calculateConfidence 计算置信度
func (e *TaskUnderstandingEngine) calculateConfidence(schema *TaskSchema) float64 {
	confidence := 0.5

	// 明确的输入模态提高置信度
	if len(schema.InputModalities) > 1 || schema.InputModalities[0] != "text" {
		confidence += 0.15
	}

	// 明确的动作提高置信度
	if len(schema.Actions) > 0 {
		confidence += 0.1
	}

	// 明确的对象提高置信度
	if len(schema.Objects) > 0 {
		confidence += 0.1
	}

	// 明确的输出产物提高置信度
	if len(schema.OutputArtifacts) > 0 {
		confidence += 0.1
	}

	// 编程语言明确提高置信度
	for _, obj := range schema.Objects {
		if strings.HasPrefix(obj, "programming_language:") {
			confidence += 0.15
			break
		}
	}

	// 歧义降低置信度
	if schema.Ambiguous {
		confidence -= 0.2
	}

	// 约束降低置信度（任务复杂）
	if len(schema.Constraints) > 2 {
		confidence -= 0.1
	}

	// 限制范围
	if confidence > 1.0 {
		confidence = 1.0
	}
	if confidence < 0.1 {
		confidence = 0.1
	}

	return confidence
}

// checkAmbiguity 检查歧义
func (e *TaskUnderstandingEngine) checkAmbiguity(schema *TaskSchema) (bool, []string) {
	ambiguous := false
	missingInputs := []string{}

	// 1. 没有明确动作
	if len(schema.Actions) == 0 {
		missingInputs = append(missingInputs, "action")
		ambiguous = true
	}

	// 2. 有歧义的关键词（同时触发多个领域）
	hasCodeKeyword := containsAnyString(schema.Objects, "function", "class", "api", "code")
	hasDataKeyword := containsAnyString(schema.Objects, "data", "chart", "table")

	if hasCodeKeyword && hasDataKeyword {
		ambiguous = true
		missingInputs = append(missingInputs, "domain_conflict")
	}

	// 3. "排序"这类多义词检查
	promptLower := strings.ToLower(schema.OriginalPrompt)
	sortKeywords := []string{"排序", "排名", "列表", "顺序"}
	hasSortKeyword := false
	for _, kw := range sortKeywords {
		if strings.Contains(promptLower, kw) {
			hasSortKeyword = true
			break
		}
	}

	if hasSortKeyword && !hasCodeKeyword && !hasDataKeyword {
		// 只有"排序"但没有代码或数据明确指向，可能只是普通聊天
		ambiguous = true
		missingInputs = append(missingInputs, "ambiguous_sort")
	}

	// 4. 没有明确的输出产物
	if len(schema.OutputArtifacts) == 0 && schema.PrimaryIntent != "general_chat" {
		ambiguous = true
		missingInputs = append(missingInputs, "output_artifact")
	}

	return ambiguous, uniqueStrings(missingInputs)
}

// uniqueStrings 去重字符串数组
func uniqueStrings(ss []string) []string {
	seen := make(map[string]bool)
	result := []string{}
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

// containsString 检查字符串数组是否包含指定字符串
func containsString(ss []string, s ...string) bool {
	for _, item := range ss {
		for _, target := range s {
			if item == target {
				return true
			}
		}
	}
	return false
}

// containsAnyString 检查字符串数组是否包含任一指定字符串
func containsAnyString(ss []string, s ...string) bool {
	for _, item := range ss {
		for _, target := range s {
			if item == target {
				return true
			}
		}
	}
	return false
}

// Ensure interface is satisfied
var _ = TaskSchema{}
