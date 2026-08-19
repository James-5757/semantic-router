package vllm_pool_client

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// =============================================================================
// Prototype-based Classifier 基于原型的分类器
// 使用关键词和模式匹配实现七类分类
// =============================================================================

// PrototypeData 原型数据
type PrototypeData struct {
	Categories map[string]CategoryPrototype `json:"categories"`
}

// CategoryPrototype 类别原型
type CategoryPrototype struct {
	Name             string   `json:"name"`
	DisplayName      string   `json:"display_name"`
	Description      string   `json:"description"`
	ChinesePositive  []string `json:"chinese_positive"`
	EnglishPositive  []string `json:"english_positive"`
	NegativeSamples  []string `json:"negative_samples"`
}

// PrototypeClassifier 基于原型的分类器
type PrototypeClassifier struct {
	prototypes *PrototypeData
	mu         sync.RWMutex
}

// NewPrototypeClassifier 创建分类器
func NewPrototypeClassifier(configPath string) (*PrototypeClassifier, error) {
	c := &PrototypeClassifier{}

	// 尝试加载原型数据
	if configPath != "" {
		if err := c.LoadPrototypes(configPath); err != nil {
			return nil, err
		}
	}

	// 如果没有加载数据，使用内置模式
	if c.prototypes == nil {
		c.prototypes = c.getDefaultPrototypes()
	}

	return c, nil
}

// LoadPrototypes 加载原型数据
func (c *PrototypeClassifier) LoadPrototypes(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var prototypes PrototypeData
	if err := json.Unmarshal(data, &prototypes); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.prototypes = &prototypes
	return nil
}

// ClassifyResult 分类结果
type ClassifyResult struct {
	Category     string              `json:"category"`
	Confidence   float64             `json:"confidence"`
	Scores       map[string]float64  `json:"scores"`
	TopK         []CategoryScore     `json:"top_k"`
	Ambiguous    bool                `json:"ambiguous"`
	MatchedSignals []string          `json:"matched_signals"`
}

// Classify 执行分类
func (c *PrototypeClassifier) Classify(prompt string) *ClassifyResult {
	c.mu.RLock()
	defer c.mu.RUnlock()

	promptLower := strings.ToLower(prompt)
	scores := make(map[string]float64)

	// 对每个类别计算分数
	for name, proto := range c.prototypes.Categories {
		score := c.calculateCategoryScore(prompt, promptLower, proto)
		scores[name] = score
	}

	// 排序获取 Top-K
	topK := c.getTopK(scores, 5)

	// 判断 ambiguous
	ambiguous := c.isAmbiguous(scores)

	// 置信度和类别
	var category string
	var confidence float64
	if len(topK) > 0 {
		category = topK[0].Category
		confidence = topK[0].Score
	} else {
		category = "general"
		confidence = 0.5
	}

	// 收集匹配的信号
	matchedSignals := c.collectMatchedSignals(prompt, promptLower)

	return &ClassifyResult{
		Category:      category,
		Confidence:    confidence,
		Scores:        scores,
		TopK:          topK,
		Ambiguous:     ambiguous,
		MatchedSignals: matchedSignals,
	}
}

// calculateCategoryScore 计算类别分数
func (c *PrototypeClassifier) calculateCategoryScore(prompt string, promptLower string, proto CategoryPrototype) float64 {
	score := 0.0
	matchCount := 0

	// 检查中文正例
	for _, pos := range proto.ChinesePositive {
		posLower := strings.ToLower(pos)
		if strings.Contains(promptLower, posLower) {
			score += 1.0
			matchCount++
		}
		// 部分匹配
		words := strings.Fields(posLower)
		matched := 0
		for _, w := range words {
			if len(w) > 2 && strings.Contains(promptLower, w) {
				matched++
			}
		}
		if matched >= len(words)/2 {
			score += 0.3
			matchCount++
		}
	}

	// 检查英文正例
	for _, pos := range proto.EnglishPositive {
		posLower := strings.ToLower(pos)
		if strings.Contains(promptLower, posLower) {
			score += 1.0
			matchCount++
		}
		// 部分匹配
		words := strings.Fields(posLower)
		matched := 0
		for _, w := range words {
			if len(w) > 3 && strings.Contains(promptLower, w) {
				matched++
			}
		}
		if matched >= len(words)/2 {
			score += 0.3
			matchCount++
		}
	}

	// 类别特定关键词加权
	keywordWeight := c.getCategoryKeywordWeight(promptLower, proto.Name)

	// 正则表达式模式匹配
	patternScore := c.matchPatterns(promptLower, proto.Name)

	// 计算最终分数：关键词权重 + 模式匹配 + 精确匹配
	// 精确匹配权重更高
	var finalScore float64
	if matchCount > 0 {
		finalScore = 0.3*float64(matchCount) + keywordWeight + patternScore
	} else {
		finalScore = keywordWeight + patternScore
	}

	// 归一化到 0-1 范围
	return math.Min(finalScore, 1.0)
}

// getCategoryKeywordWeight 获取类别关键词权重
func (c *PrototypeClassifier) getCategoryKeywordWeight(promptLower string, category string) float64 {
	keywords := map[string][]string{
		"code": {
			"python", "javascript", "java", "golang", "go ", "c++", "rust", "typescript",
			"编程", "代码", "写代码", "算法", "排序", "实现", "函数", "class ", "def ",
			"import ", "interface", "implement", "debug", "bug", "修复", "api", "sdk",
			"program", "function", "algorithm", "sorting", "implement", "code",
		},
		"data_analysis": {
			"data", "analysis", "analytics", "csv", "excel", "chart", "graph", "plot",
			"分析", "数据", "统计", "图表", "趋势", "预测", "机器学习", "ml ",
			"cluster", "regression", "visualization", "dashboard", "metric",
			"分析数据", "趋势图", "数据处理", "统计分析",
		},
		"document": {
			"document", "pdf", "word", "docx", "summarize", "summary", "translate",
			"文档", "总结", "翻译", "提取", "写作", "文章", "论文", "报告",
			"translation", "extraction", "writing", "summary", "paragraph",
			"合同", "简历", "商业计划书", "会议纪要",
		},
		"vision_understanding": {
			"image", "photo", "picture", "screenshot", "analyze", "describe", "recognize",
			"图片", "照片", "图像", "分析", "描述", "识别", "这张", "这张图",
			"what's in", "describe this", "identify", "object detection",
			"图片中", "照片里", "截图", "看图",
		},
		"image_generation": {
			"generate", "create", "draw", "design", "generate image", "create image",
			"生成", "画", "设计", "创作", "海报", "插画", "logo", "封面",
			"painting", "art", "poster", "illustration", "logo design",
			"生成图片", "AI绘画", "文生图", "创作",
		},
		"simple_chat": {
			"hello", "hi", "你好", "推荐", "建议", "什么是", "怎么", "为什么",
			"天气", "音乐", "电影", "餐厅", "旅游", "推荐", "介绍",
			"recommend", "suggestion", "what is", "how to", "explain",
			"tell me", "what do you think", "question",
		},
		"general": {
			// 默认类别，权重较低
		},
	}

	keywordsList, ok := keywords[category]
	if !ok {
		return 0
	}

	weight := 0.0
	for _, kw := range keywordsList {
		if strings.Contains(promptLower, kw) {
			weight += 0.2
		}
	}

	return math.Min(weight, 0.5)
}

// matchPatterns 模式匹配
func (c *PrototypeClassifier) matchPatterns(promptLower string, category string) float64 {
	patterns := map[string][]string{
		"code": {
			"^写.*(算法|程序|代码|函数|class|def |function)",
			"帮我写.*代码",
			"实现.*(算法|功能|接口)",
			"write.*(code|program|algorithm|function)",
			"implement.*(algorithm|function|class)",
		},
		"data_analysis": {
			"分析.*数据",
			"生成.*图表",
			".*趋势图",
			"analyze.*data",
			"generate.*chart",
			".*graph",
		},
		"document": {
			"总结.*(文章|论文|文档|书)",
			"翻译.*(文档|文章|英文)",
			"summarize.*(doc|article|paper)",
			"translate.*(doc|article)",
		},
		"vision_understanding": {
			"分析.*图片",
			"描述.*照片",
			"这张图.*什么",
			"analyze.*image",
			"describe.*picture",
			"what.*in.*image",
		},
		"image_generation": {
			"生成.*图片",
			"画.*(画|图|海报)",
			"create.*image",
			"generate.*(image|picture|poster)",
			"design.*(logo|poster)",
		},
		"simple_chat": {
			"推荐.*",
			"帮我.*",
			"什么是.*",
			"recommend",
			"suggest",
		},
	}

	patternList, ok := patterns[category]
	if !ok {
		return 0
	}

	score := 0.0
	for _, pattern := range patternList {
		// 简单字符串包含检查
		if strings.Contains(promptLower, extractKeywordFromPattern(pattern)) {
			score += 0.3
		}
	}

	return math.Min(score, 0.5)
}

// extractKeywordFromPattern 从模式中提取关键词
func extractKeywordFromPattern(pattern string) string {
	// 简单实现：移除正则符号，提取核心词
	pattern = strings.ReplaceAll(pattern, "^", "")
	pattern = strings.ReplaceAll(pattern, ".*", "")
	pattern = strings.ReplaceAll(pattern, "(", "")
	pattern = strings.ReplaceAll(pattern, ")", "")
	parts := strings.Fields(pattern)
	if len(parts) > 0 {
		return strings.ToLower(parts[0])
	}
	return pattern
}

// getTopK 获取 Top-K 结果
func (c *PrototypeClassifier) getTopK(scores map[string]float64, k int) []CategoryScore {
	type scoredCategory struct {
		category string
		score    float64
	}

	var sorted []scoredCategory
	for cat, score := range scores {
		sorted = append(sorted, scoredCategory{category: cat, score: score})
	}

	// 排序
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].score > sorted[i].score {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// 取 Top-K
	result := make([]CategoryScore, 0, k)
	for i := 0; i < len(sorted) && i < k; i++ {
		if sorted[i].score > 0 {
			result = append(result, CategoryScore{
				Category: sorted[i].category,
				Score:    sorted[i].score,
			})
		}
	}

	return result
}

// isAmbiguous 判断是否 ambiguous
func (c *PrototypeClassifier) isAmbiguous(scores map[string]float64) bool {
	if len(scores) < 2 {
		return false
	}

	// 找到最高分和次高分
	var sorted []float64
	for _, s := range scores {
		sorted = append(sorted, s)
	}
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j] > sorted[i] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	if len(sorted) < 2 || sorted[0] == 0 {
		return false
	}

	// 如果最高分和次高分差距小于 0.15，认为 ambiguous
	margin := sorted[0] - sorted[1]
	return margin < 0.15
}

// collectMatchedSignals 收集匹配的信号
func (c *PrototypeClassifier) collectMatchedSignals(prompt string, promptLower string) []string {
	signals := []string{}

	// 检测语言
	if containsChinese(prompt) {
		signals = append(signals, "language:chinese")
	} else {
		signals = append(signals, "language:english")
	}

	// 检测长度
	if len(prompt) < 50 {
		signals = append(signals, "length:short")
	} else if len(prompt) > 200 {
		signals = append(signals, "length:long")
	} else {
		signals = append(signals, "length:medium")
	}

	// 检测关键词类别
	if strings.Contains(promptLower, "python") || strings.Contains(promptLower, "code") {
		signals = append(signals, "keyword:code")
	}
	if strings.Contains(promptLower, "data") || strings.Contains(promptLower, "analysis") {
		signals = append(signals, "keyword:data")
	}
	if strings.Contains(promptLower, "image") || strings.Contains(promptLower, "生成") {
		signals = append(signals, "keyword:vision")
	}
	if strings.Contains(promptLower, "document") || strings.Contains(promptLower, "总结") {
		signals = append(signals, "keyword:document")
	}
	if strings.Contains(promptLower, "recommend") || strings.Contains(promptLower, "推荐") {
		signals = append(signals, "keyword:chat")
	}

	return signals
}

// containsChinese 检查是否包含中文
func containsChinese(s string) bool {
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

// getDefaultPrototypes 获取默认原型
func (c *PrototypeClassifier) getDefaultPrototypes() *PrototypeData {
	// 尝试从文件加载
	exePath, _ := os.Executable()
	dir := filepath.Dir(exePath)
	defaultPath := filepath.Join(dir, "prototypes.json")

	if data, err := os.ReadFile(defaultPath); err == nil {
		var prototypes PrototypeData
		if json.Unmarshal(data, &prototypes) == nil {
			return &prototypes
		}
	}

	// 返回硬编码的最小集合
	return &PrototypeData{
		Categories: map[string]CategoryPrototype{
			"code": {
				Name:        "code",
				DisplayName: "代码开发",
				ChinesePositive: []string{
					"写一个Python排序算法", "用Java实现", "帮我写个函数",
					"实现一个算法", "写一段代码", "编程", "修复bug",
				},
				EnglishPositive: []string{
					"Write a Python", "Implement in Java", "Write a function",
					"algorithm", "programming", "code",
				},
			},
			"data_analysis": {
				Name:        "data_analysis",
				DisplayName: "数据分析",
				ChinesePositive: []string{
					"分析数据", "生成图表", "趋势图", "统计分析", "数据处理",
				},
				EnglishPositive: []string{
					"analyze data", "generate chart", "trend", "statistics",
				},
			},
			"document": {
				Name:        "document",
				DisplayName: "文档处理",
				ChinesePositive: []string{
					"总结", "翻译", "提取", "文档", "文章",
				},
				EnglishPositive: []string{
					"summarize", "translate", "document", "extract",
				},
			},
			"vision_understanding": {
				Name:        "vision_understanding",
				DisplayName: "图片理解",
				ChinesePositive: []string{
					"分析图片", "描述照片", "这张图", "识别图像", "图片中",
				},
				EnglishPositive: []string{
					"analyze image", "describe picture", "what's in", "identify",
				},
			},
			"image_generation": {
				Name:        "image_generation",
				DisplayName: "图片生成",
				ChinesePositive: []string{
					"生成图片", "画图", "生成海报", "AI绘画", "创作",
				},
				EnglishPositive: []string{
					"generate image", "create image", "draw", "painting", "poster",
				},
			},
			"simple_chat": {
				Name:        "simple_chat",
				DisplayName: "简单对话",
				ChinesePositive: []string{
					"推荐", "你好", "天气", "帮我", "是什么",
				},
				EnglishPositive: []string{
					"recommend", "hello", "weather", "suggest", "what is",
				},
			},
			"general": {
				Name:        "general",
				DisplayName: "通用",
				ChinesePositive: []string{
					"你好", "谢谢", "再见", "这个", "怎么",
				},
				EnglishPositive: []string{
					"hello", "thank", "goodbye", "this", "how",
				},
			},
		},
	}
}