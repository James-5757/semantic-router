package semanticrouter

import (
	"math"
	"strings"
	"unicode"
)

// PoolDescription 定义每个 pool 的描述，用于相似度计算（双语）
var PoolDescriptions = map[PreferredPool]string{
	PoolDefault:         "general conversation, chat, simple question answering, daily communication, general Chinese chat, generic text processing, miscellaneous tasks, 通用对话, 普通聊天, 简单问答, 日常交流, 文本处理",
	PoolCheap:           "cheap chat, simple conversation, basic question answering, fast response, simple Chinese chat, casual talk, greeting, weather inquiry, date inquiry, 简单聊天, 基础问答, 快速回复, 日常闲聊, 打招呼, 问天气, 问日期, 简单解释",
	PoolCode:            "code generation, programming, debugging, fix errors, write functions, implement algorithms, software development, system design, api development, database queries, sql, scripting, automation, frontend, backend, devops, code review, engineering, 编程, 写代码, 开发, 调试, 修复, 实现功能, 算法, 数据结构, 接口开发, 数据库, SQL, 脚本, 自动化, 前端, 后端, 系统设计, API开发, 代码审查, 软件工程",
	PoolVision:          "image understanding, image analysis, describe images, OCR, image processing, screenshot analysis, chart recognition, visual question answering, 图片理解, 图片分析, 描述图片, OCR识别, 图像处理, 截图分析, 图表识别, 视觉问答, 图片内容理解",
	PoolDocument:        "document processing, Word files, PDF, summarize documents, extract text, document analysis, contract review, report generation, essay writing, document translation, document Q&A, 文档处理, Word文档, PDF处理, 文档总结, 文本提取, 文档分析, 合同审查, 报告生成, 文章写作, 文档翻译, 文档问答",
	PoolData:            "data analysis, charts, graphs, data visualization, statistics, Excel, CSV, data cleaning, data mining, predictive modeling, regression analysis, trend analysis, business intelligence, reporting, metrics, dashboards, 数据分析, 图表, 统计, Excel处理, CSV处理, 数据清洗, 数据挖掘, 预测建模, 回归分析, 趋势分析, 商业智能, 报表, 指标, 仪表盘",
	PoolPrivate:         "private conversation, confidential, sensitive topics, personal matters, secure communication, 私密对话, 机密内容, 敏感话题, 个人事务, 安全通信",
	PoolImageGeneration: "image generation, create images, draw pictures, generate photos, AI art, text to image, poster design, illustration, digital art, creative visual, image creation, 图片生成, 画图, 创建图片, AI绘画, 文生图, 海报设计, 插画, 数字艺术, 创意视觉, 图像创作",
}

// PoolKeywords 每个 pool 的关键词权重（中英文）
var PoolKeywords = map[PreferredPool][]string{
	PoolCode: {
		// 英文关键词
		"code", "programming", "function", "algorithm", "debug", "fix", "error", "exception",
		"sql", "query", "api", "implement", "class", "method", "python", "javascript",
		"java", "golang", "rust", "typescript", "write code", "write a function",
		"quick sort", "implement", "program", "script", "frontend", "backend",
		"dockerfile", "deployment", "microservice", "distributed", "cache",
		"refactor", "review", "test", "coverage", "performance", "security",
		"ci", "cd", "cicd", "docker", "kubernetes", "k8s", "build", "develop",
		"memory leak", "内存泄漏", "deadlock", "race condition",
		// 中文关键词 - 更全面
		"写代码", "开发", "实现", "制作", "创建", "编写", "做一个", "搭建",
		"爬虫", "接口", "数据库", "登录功能", "修复bug", "报错", "修复",
		"前端页面", "后端开发", "自动化脚本", "正则表达式", "配置文件",
		"报错日志", "调试", "优化", "重构", "中间件", "微服务",
		"分布式", "消息队列", "缓存", "单元测试", "部署",
		"登录注册", "文件上传", "权限控制", "API接口", "API",
		"流程", "脚本", "容器化", "安全", "测试", "类型",
		"改写", "转换", "翻译代码", "组件", "插件", "系统",
		"功能", "页面", "服务器", "运行不了", "异常",
		"排序", "算法", "数据结构", "二叉树", "链表", "栈", "队列",
		"冒泡", "快速排序", "归并", "查找", "遍历",
		"返回", "请求", "响应", "JSON", "XML",
		// 新增 - 用户场景
		"排序算法", "用户登录", "注册功能", "数据结构不对",
		"数据格式不对", "结构化数据", "接口调用", "代码问题",
		"配置文件错误", "代码逻辑", "编程问题", "写个",
		// 开发任务常见前缀
		"写一个", "写个", "制作一个", "开发一个",
	},
	PoolData: {
		// 英文关键词
		"data", "chart", "graph", "analysis", "statistics", "excel", "visualization",
		"trend", "metric", "conversion", "ctr", "roi", "predict", "regression",
		"cleaning", "missing", "outlier", "analyze", "analytics", "dashboard",
		"classification", "clustering", "forecast",
		// 中文关键词
		"数据", "表格", "Excel", "CSV", "图表", "趋势", "分析", "统计",
		"可视化", "生成图表", "输入数据", "指标", "计算", "转化率",
		"CTR", "ROI", "预测", "回归", "清洗", "处理", "缺失值",
		"异常值", "聚类", "用户行为", "销售数据", "仪表盘",
		// 新增 - 数据专用词
		"建模", "模型训练", "分类", "增长率", "月度", "季度",
		"渠道", "每个渠道", "趋势图", "折线图", "柱状图", "饼图",
		"数据可视化", "数据分析", "数据报表", "数据清洗",
		"生成趋势图", "销售分析", "增长分析",
	},
	PoolVision: {
		"image", "photo", "picture", "screenshot", "截图", "图片", "照片", "vision",
		"describe", "ocr", "识别", "分析图片", "看图", "图像", "读取图片",
		"这张图", "图片中", "照片里", "描述这张",
		// 更明确的 vision 词（区别于 image_generation）
		"分析这张图片", "图片内容", "图像识别", "理解图片",
		"图片分析", "图中", "图上", "看到什么",
	},
	PoolImageGeneration: {
		"generate image", "create image", "draw a", "generate a picture", "create a photo",
		"generate a chart",
		// 中文精确关键词
		"生成图片", "生成图像", "生成一张图", "生成图片描述",
		"画一幅画", "画一张图", "创作图片", "创建图片",
		"海报", "插画", "poster", "illustration", "digital art",
		"AI艺术", "AI绘画", "文生图", "文字生成图片",
		"生成一张", "帮我画", "帮我生成图片", "创作一幅",
		"生成logo", "设计海报", "设计图片", "图片生成",
	},
	PoolDocument: {
		"document", "word", "pdf", "docx", "doc", "summarize", "总结", "文档",
		"文章", "文案", "提取", "编辑文档", "公众号", "报告",
		"合同", "论文", "技术方案", "翻译", "写作",
		"摘要", "大纲", "检查", "审查", "漏洞", "风险", "文档处理",
		// 新增
		"文档问答", "阅读理解", "文档分析", "文档总结",
		"分析文档", "读文档", "文档内容",
	},
	PoolCheap: {
		"hello", "hi", "你好", "简单", "basic", "quick", "fast", "天气", "几号",
		"推荐", "解释", "翻译", "写作文", "写诗", "是什么", "怎么样",
		"日常", "聊聊", "问答", "基础", "快速",
		"简单的", "快速回答", "简短",
	},
}

// SemanticSimilarityResult 相似度路由结果
type SemanticSimilarityResult struct {
	Pool            PreferredPool
	Score           float64
	MatchedKeywords []string
	Confidence      float64
	Reason          string
}

// SemanticSimilarityRouter 语义相似度路由接口
type SemanticSimilarityRouter interface {
	// Route 根据 prompt 进行相似度路由
	Route(prompt string, taskType TaskType, hasImages bool, hasDocuments bool) *SemanticSimilarityResult
}

// TokenOverlapSimilarityRouter 基于 token overlap 的相似度路由
type TokenOverlapSimilarityRouter struct {
	poolDescriptions map[PreferredPool]string
	poolKeywords     map[PreferredPool][]string
}

// NewTokenOverlapSimilarityRouter 创建新的相似度路由器
func NewTokenOverlapSimilarityRouter() *TokenOverlapSimilarityRouter {
	return &TokenOverlapSimilarityRouter{
		poolDescriptions: PoolDescriptions,
		poolKeywords:     PoolKeywords,
	}
}

// normalizeText 标准化文本
func (r *TokenOverlapSimilarityRouter) normalizeText(text string) string {
	text = strings.ToLower(text)

	var b strings.Builder
	lastWasSpace := false
	for _, rr := range text {
		if unicode.IsLetter(rr) || unicode.IsDigit(rr) {
			b.WriteRune(rr)
			lastWasSpace = false
			continue
		}
		if unicode.IsSpace(rr) && !lastWasSpace {
			b.WriteByte(' ')
			lastWasSpace = true
		}
	}

	return strings.TrimSpace(b.String())
}

// tokenize 分词
func (r *TokenOverlapSimilarityRouter) tokenize(text string) []string {
	normalized := r.normalizeText(text)
	// 处理中文 - 按字符分词
	var tokens []string
	runes := []rune(normalized)
	for _, r := range runes {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			tokens = append(tokens, string(r))
		}
	}
	// 处理英文 - 按空格分词
	words := strings.Fields(normalized)
	tokens = append(tokens, words...)
	return tokens
}

// CalculateKeywordScore 计算关键词匹配得分（导出方法）
func (r *TokenOverlapSimilarityRouter) CalculateKeywordScore(prompt string, pool PreferredPool) (float64, []string) {
	return r.calculateKeywordScore(prompt, pool)
}

// calculateKeywordScore 计算关键词匹配得分
func (r *TokenOverlapSimilarityRouter) calculateKeywordScore(prompt string, pool PreferredPool) (float64, []string) {
	normalizedPrompt := r.normalizeText(prompt)
	promptTokens := r.tokenize(prompt)
	promptTokenSet := make(map[string]bool)
	for _, t := range promptTokens {
		promptTokenSet[t] = true
	}

	var matchedKeywords []string
	var totalWeight float64

	keywords, ok := r.poolKeywords[pool]
	if !ok {
		return 0, nil
	}

	for _, kw := range keywords {
		normalizedKw := r.normalizeText(kw)
		if normalizedKw == "" {
			continue
		}
		// 检查关键词是否在 prompt 中
		if strings.Contains(normalizedPrompt, normalizedKw) {
			matchedKeywords = append(matchedKeywords, kw)
			// 根据关键词长度计算权重（越长的关键词权重越高）
			weight := math.Sqrt(float64(len([]rune(normalizedKw))))
			totalWeight += weight
		}
	}

	// 归一化得分（最大可能得分设为 10）
	score := math.Min(totalWeight/10, 1.0)
	return score, matchedKeywords
}

// CalculateDescriptionSimilarity 计算描述相似度（导出方法）
func (r *TokenOverlapSimilarityRouter) CalculateDescriptionSimilarity(prompt string, pool PreferredPool) float64 {
	return r.calculateDescriptionSimilarity(prompt, pool)
}

// calculateDescriptionSimilarity 计算描述相似度
func (r *TokenOverlapSimilarityRouter) calculateDescriptionSimilarity(prompt string, pool PreferredPool) float64 {
	desc, ok := r.poolDescriptions[pool]
	if !ok {
		return 0
	}

	promptTokens := r.tokenize(prompt)
	descTokens := r.tokenize(desc)

	// 计算 token overlap
	descTokenSet := make(map[string]bool)
	for _, t := range descTokens {
		descTokenSet[t] = true
	}

	overlap := 0
	for _, t := range promptTokens {
		if descTokenSet[t] && len(t) > 1 { // 忽略单字符
			overlap++
		}
	}

	// Jaccard 相似度
	if len(promptTokens) == 0 || len(descTokens) == 0 {
		return 0
	}

	jaccard := float64(overlap) / float64(len(promptTokens)+len(descTokens)-overlap)
	return math.Min(jaccard*5, 1.0) // 放大得分
}

// Route 根据 prompt 进行相似度路由
func (r *TokenOverlapSimilarityRouter) Route(prompt string, taskType TaskType, hasImages bool, hasDocuments bool) *SemanticSimilarityResult {
	var bestPool PreferredPool
	var bestScore float64
	var matchedKeywords []string
	var reason string

	// 优先级：先根据 hasImages/hasDocuments 快速判断
	if hasImages {
		score, kws := r.calculateKeywordScore(prompt, PoolVision)
		if score > 0.3 {
			return &SemanticSimilarityResult{
				Pool:            PoolVision,
				Score:           score,
				MatchedKeywords: kws,
				Confidence:      0.8,
				Reason:          "detected images in request",
			}
		}
	}

	if hasDocuments {
		score, kws := r.calculateKeywordScore(prompt, PoolDocument)
		if score > 0.3 {
			return &SemanticSimilarityResult{
				Pool:            PoolDocument,
				Score:           score,
				MatchedKeywords: kws,
				Confidence:      0.8,
				Reason:          "detected documents in request",
			}
		}
	}

	// 计算所有 pool 的得分
	pools := []PreferredPool{PoolDefault, PoolCheap, PoolCode, PoolVision, PoolDocument, PoolData, PoolPrivate, PoolImageGeneration}

	for _, pool := range pools {
		// 跳过已经判断过的 pool
		if pool == PoolVision && hasImages {
			continue
		}
		if pool == PoolDocument && hasDocuments {
			continue
		}

		keywordScore, kws := r.calculateKeywordScore(prompt, pool)
		descScore := r.calculateDescriptionSimilarity(prompt, pool)

		// 综合得分：关键词权重 0.7，描述相似度权重 0.3
		totalScore := keywordScore*0.7 + descScore*0.3

		if totalScore > bestScore {
			bestScore = totalScore
			bestPool = pool
			matchedKeywords = kws
		}
	}

	// 根据得分计算置信度
	var confidence float64
	if bestScore >= 0.5 {
		confidence = 0.75 + bestScore*0.25
	} else if bestScore >= 0.2 {
		confidence = 0.45 + bestScore*0.3
	} else {
		confidence = bestScore + 0.25
	}
	confidence = math.Min(confidence, 1.0)

	reason = "semantic similarity"
	if len(matchedKeywords) > 0 {
		reason = "keyword match: " + strings.Join(matchedKeywords[:min(3, len(matchedKeywords))], ", ")
	}

	return &SemanticSimilarityResult{
		Pool:            bestPool,
		Score:           bestScore,
		MatchedKeywords: matchedKeywords,
		Confidence:      confidence,
		Reason:          reason,
	}
}

// min 返回较小的值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
