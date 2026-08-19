package vllm_pool_client

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// =============================================================================
// Prototype-based Classifier 基于原型的分类器 (V2)
// 使用关键词和模式匹配实现七类分类
// =============================================================================

// PrototypeClassifierV2 基于原型的分类器 V2
type PrototypeClassifierV2 struct {
	prototypes *PrototypeData
	mu         sync.RWMutex
}

// NewPrototypeClassifierV2 创建分类器
func NewPrototypeClassifierV2(configPath string) (*PrototypeClassifierV2, error) {
	c := &PrototypeClassifierV2{}

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
func (c *PrototypeClassifierV2) LoadPrototypes(path string) error {
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
type ClassifyResultV2 struct {
	Category      string              `json:"category"`
	Confidence    float64             `json:"confidence"`
	Scores        map[string]float64  `json:"scores"`
	TopK          []CategoryScore     `json:"top_k"`
	Ambiguous     bool                `json:"ambiguous"`
	MatchedSignals []string           `json:"matched_signals"`
}

// Classify 执行分类
func (c *PrototypeClassifierV2) Classify(prompt string) *ClassifyResultV2 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	promptLower := strings.ToLower(prompt)
	scores := make(map[string]float64)

	// 定义每个类别的关键词和模式（优先级从高到低）
	categoryRules := map[string]struct {
		keywords  []string
		patterns  []string
		weight    float64
	}{
		"code": {
			keywords: []string{"python", "javascript", "java", "golang", "go ", "c++", "rust", "typescript", "编程", "代码", "写代码", "算法", "排序", "实现", "函数", "class ", "def ", "import ", "interface", "implement", "debug", "bug", "修复", "api", "program", "algorithm"},
			patterns: []string{`写.*(python|排序|算法|程序|代码|函数)`, `帮我写`, `实现.*(算法|功能|接口)`, `write.*(code|program|algorithm|function)`, `implement`},
			weight:   1.0,
		},
		"data_analysis": {
			keywords: []string{"data", "analysis", "csv", "excel", "chart", "graph", "plot", "分析", "数据", "统计", "图表", "趋势", "预测", "机器学习", "cluster", "regression", "visualization"},
			patterns: []string{`分析.*数据`, `生成.*图表`, `趋势图`, `data.*analysis`, `analyze.*data`, `generate.*chart`, `statistic`},
			weight:   1.0,
		},
		"document": {
			keywords: []string{"document", "pdf", "word", "summarize", "summary", "translate", "文档", "总结", "翻译", "提取", "写作", "文章", "论文", "report"},
			patterns: []string{`总结.*(文章|论文|文档|书)`, `翻译.*(文档|文章|英文)`, `summarize`, `translate`, `extract`},
			weight:   1.0,
		},
		"vision_understanding": {
			keywords: []string{"image", "photo", "picture", "screenshot", "图片", "照片", "图像", "分析", "描述", "识别", "这张", "what's in", "describe this"},
			patterns: []string{`分析.*(图片|照片|图像)`, `描述.*(照片|图片)`, `这张.*什么`, `analyze.*image`, `describe.*picture`, `identify.*(image|object)`},
			weight:   1.0,
		},
		"image_generation": {
			keywords: []string{"generate", "create", "draw", "design", "生成", "画", "设计", "创作", "海报", "插画", "logo", "painting", "art", "poster"},
			patterns: []string{`生成.*(图片|图像|海报)`, `画.*(画|图|海报)`, `create.*image`, `generate.*(image|poster)`, `design.*(logo|poster)`},
			weight:   1.0,
		},
		"simple_chat": {
			keywords: []string{"hello", "hi", "你好", "推荐", "建议", "天气", "音乐", "电影", "餐厅", "旅游", "介绍", "what is", "how to", "recommend"},
			patterns: []string{`推荐`, `天气`, `.*怎么样`, `是什么`, `帮我`, `hello`, `hi`, `recommend`, `suggest`},
			weight:   0.8,
		},
		"general": {
			keywords: []string{},
			patterns: []string{},
			weight:   0.3,
		},
	}

	// 计算每个类别的分数
	for category, rules := range categoryRules {
		score := 0.0

		// 检查关键词匹配
		for _, kw := range rules.keywords {
			if strings.Contains(promptLower, kw) {
				score += rules.weight * 0.3
			}
		}

		// 检查正则表达式模式匹配
		for _, pattern := range rules.patterns {
			matched, err := regexp.MatchString(pattern, promptLower)
			if err == nil && matched {
				score += rules.weight * 0.5
			}
		}

		scores[category] = math.Min(score, 1.0)
	}

	// 排序获取 Top-K
	topK := c.getTopK(scores, 5)

	// 判断 ambiguous
	ambiguous := c.isAmbiguous(scores)

	// 置信度和类别
	var category string
	var confidence float64
	if len(topK) > 0 && topK[0].Score > 0 {
		category = topK[0].Category
		confidence = topK[0].Score
	} else {
		category = "general"
		confidence = 0.5
	}

	// 收集匹配的信号
	matchedSignals := c.collectMatchedSignals(prompt, promptLower)

	return &ClassifyResultV2{
		Category:       category,
		Confidence:     confidence,
		Scores:         scores,
		TopK:           topK,
		Ambiguous:      ambiguous,
		MatchedSignals: matchedSignals,
	}
}

// getTopK 获取 Top-K 结果
func (c *PrototypeClassifierV2) getTopK(scores map[string]float64, k int) []CategoryScore {
	type scoredCategory struct {
		category string
		score    float64
	}

	var sorted []scoredCategory
	for cat, score := range scores {
		sorted = append(sorted, scoredCategory{category: cat, score: score})
	}

	// 按分数降序排序
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
func (c *PrototypeClassifierV2) isAmbiguous(scores map[string]float64) bool {
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
func (c *PrototypeClassifierV2) collectMatchedSignals(prompt string, promptLower string) []string {
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

// getDefaultPrototypes 获取默认原型
func (c *PrototypeClassifierV2) getDefaultPrototypes() *PrototypeData {
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