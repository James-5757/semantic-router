package semanticrouter

import (
	"sort"
	"strings"
)

// PoolPrototype Pool 的原型示例
type PoolPrototype struct {
	Pool        PreferredPool
	Prompts     []string
	IsNegative  bool // 是否是负例
	Description string
}

// PoolPrototypes 所有 Pool 的原型示例
// 每个 Pool 至少 30-50 个 prototype，包含中英文、不同句式、短长请求
var PoolPrototypes = []PoolPrototype{
	// ===== code_pool 正例 =====
	{
		Pool: PoolCode,
		Description: "代码开发：编程、调试、算法实现",
		Prompts: []string{
			// 英文
			"write a function to sort a list in Python",
			"implement a binary search algorithm in Java",
			"debug this code - it's throwing an error",
			"refactor this function to be more efficient",
			"create a REST API endpoint in Go",
			"write a Python script to scrape web data",
			"implement a linked list in C++",
			"fix the memory leak in this code",
			"write unit tests for this function",
			"optimize this SQL query",
			"create a class for handling user authentication",
			"implement the quick sort algorithm",
			"write a Docker Compose file for this service",
			"create a middleware for rate limiting",
			"implement a cache system using Redis",
			// 中文
			"用 Python 实现一个排序函数",
			"帮我写一个二分查找算法",
			"这段代码报错了帮我调试一下",
			"用 Go 写一个 REST API 接口",
			"用 Java 实现一个链表",
			"帮我优化这段 SQL 查询",
			"写一个 Python 爬虫抓取网页",
			"用 JavaScript 实现一个登录功能",
			"帮我修复这个 bug",
			"用 TypeScript 写一个前端组件",
			"实现快速排序算法",
			"写一个 Docker 部署配置",
			"用 React 写一个用户管理页面",
			"帮我重构这段代码让它更简洁",
			"用 Django 搭建一个博客系统",
			"实现一个消息队列消费者",
			"写一个自动化测试脚本",
			"用 Spring Boot 写一个接口",
			"帮我优化代码性能",
			"实现一个分布式锁",
			// 长请求
			"我需要用 Python 实现一个完整的用户认证系统，包括注册、登录、Token 验证、密码加密等功能",
			"帮我写一个能够处理高并发的 RESTful API，需要考虑缓存、限流、错误处理",
			"用 Go 实现一个微服务框架，包含服务发现、负载均衡、熔断器",
			// 短请求
			"写个函数",
			"实现排序",
			"帮我 debug",
			"写个 API",
		},
	},

	// ===== code_pool 负例（需要排除的） =====
	{
		Pool:        PoolCode,
		IsNegative:  true,
		Description: "不是代码开发：日常对话、简单排序",
		Prompts: []string{
			"帮我给歌曲排个序",
			"推荐几首好听的歌",
			"今天天气怎么样",
			"帮我按价格排序",
			"这个列表怎么排序",
			"帮我整理一下这个清单",
			"把商品按销量排序",
			"排序一下这个表格",
			"给我讲个故事",
			"今天几号了",
			"帮我推荐一部电影",
			"解释一下什么是 API",
			"什么是机器学习",
			"排序算法是什么意思",
			"介绍一下排序功能",
		},
	},

	// ===== data_pool 正例 =====
	{
		Pool: PoolData,
		Description: "数据分析：图表生成、统计分析、数据处理",
		Prompts: []string{
			// 英文
			"analyze this sales data and create a chart",
			"generate a trend chart from this CSV",
			"visualize the monthly revenue",
			"create a bar chart showing product categories",
			"analyze customer behavior data",
			"build a predictive model for sales",
			"clean this dataset and handle missing values",
			"perform regression analysis on the data",
			"create a dashboard with key metrics",
			"analyze the growth trend",
			"generate a pie chart of market share",
			"forecast next quarter revenue",
			"analyze user engagement metrics",
			"create a data visualization report",
			// 中文
			"帮我分析这份销售数据并生成图表",
			"用这个 CSV 生成趋势图",
			"帮我做个数据可视化",
			"分析用户增长趋势",
			"生成一张柱状图显示各渠道销量",
			"帮我清洗数据处理缺失值",
			"做一个销售数据分析报告",
			"用这些数据预测下季度收入",
			"创建数据仪表盘",
			"分析转化率变化趋势",
			"生成各产品类别的饼图",
			"分析用户行为数据",
			"做一下回归分析",
			"帮我算一下平均增长率",
			"用 Excel 做个数据统计",
			// 长请求
			"我有一个包含 10 万条用户行为数据的 CSV 文件，需要分析用户的留存率、活跃度、转化路径，并生成可视化报表",
			"帮我分析过去一年的销售数据，包括各月份的销售额、增长率、主要产品贡献，并预测下一年度的销售趋势",
			// 短请求
			"分析数据",
			"生成图表",
			"做个统计",
			"算一下平均值",
		},
	},

	// ===== data_pool 负例 =====
	{
		Pool:        PoolData,
		IsNegative:  true,
		Description: "不是数据分析：创意图片、艺术设计",
		Prompts: []string{
			"生成一张未来城市的海报",
			"画一幅科幻风格的插画",
			"设计一个 logo",
			"生成艺术图片",
			"创作一幅油画风格的图像",
			"生成一张海报",
			"帮我画个插画",
			"设计一个图标",
		},
	},

	// ===== vision_pool 正例 =====
	{
		Pool: PoolVision,
		Description: "图像理解：分析现有图片、描述内容、OCR",
		Prompts: []string{
			// 英文
			"describe what's in this image",
			"analyze this screenshot and tell me what you see",
			"extract text from this image using OCR",
			"what information is shown in this chart",
			"describe the contents of this photo",
			"analyze this diagram",
			"what objects are in this picture",
			"read the text in this screenshot",
			"describe the scene in this image",
			"what does this graph show",
			// 中文
			"描述这张图片的内容",
			"分析这张截图",
			"帮我看看这张照片里有什么",
			"提取图片中的文字",
			"这张图表显示了什么信息",
			"描述这个场景",
			"图片里有哪些物体",
			"帮我识别这张图的内容",
			"这张照片是在哪里拍的",
			"分析这张图片的构图",
			// 长请求
			"请详细分析这张图片中的所有元素，包括人物、物体、场景布局、光线氛围等",
			"帮我提取这张扫描文档中的所有文字内容，并总结关键信息",
			// 短请求
			"看看这张图",
			"这张是什么",
			"图里有什么",
		},
	},

	// ===== vision_pool 负例 =====
	{
		Pool:        PoolVision,
		IsNegative:  true,
		Description: "不是图像理解：生成新图片",
		Prompts: []string{
			"生成一张未来城市的图片",
			"帮我画一幅科幻画",
			"生成一张海报",
			"创作一个 logo",
			"生成一张艺术图片",
			"帮我设计一个图标",
			"生成一张插画",
			"画一幅画",
		},
	},

	// ===== image_generation_pool 正例 =====
	{
		Pool: PoolImageGeneration,
		Description: "图像生成：创意视觉、艺术设计",
		Prompts: []string{
			// 英文
			"generate an image of a futuristic city",
			"create a sci-fi illustration",
			"design a logo for my company",
			"generate an artistic poster",
			"create a digital art piece",
			"generate a fantasy landscape",
			"design a book cover",
			"create an album cover art",
			"generate a character design",
			"create an advertising banner",
			// 中文
			"生成一张未来城市的图片",
			"帮我画一幅科幻插画",
			"设计一个 logo",
			"生成一张创意海报",
			"创作一幅数字艺术",
			"生成一个奇幻场景",
			"帮我设计书籍封面",
			"生成专辑封面",
			"创作一个人物设计",
			"生成广告 banner",
			// 长请求
			"生成一张未来科技城市的概念图，包含摩天大楼、飞行汽车、全息广告牌等元素，赛博朋克风格",
			"帮我设计一个科幻主题的游戏角色，需要包含详细的服装、装备、外貌描述",
			// 短请求
			"生成图片",
			"画个画",
			"设计 logo",
		},
	},

	// ===== image_generation_pool 负例 =====
	{
		Pool:        PoolImageGeneration,
		IsNegative:  true,
		Description: "不是图像生成：理解现有图片",
		Prompts: []string{
			"分析这张图片",
			"描述这个照片",
			"帮我看看这张图",
			"这张图里有什么",
			"提取图片文字",
			"识别这张图片",
		},
	},

	// ===== document_pool 正例 =====
	{
		Pool: PoolDocument,
		Description: "文档处理：总结、翻译、提取",
		Prompts: []string{
			// 英文
			"summarize this document",
			"translate this article to English",
			"extract key points from this PDF",
			"review this contract",
			"write a report based on this document",
			"analyze this paper",
			"extract text from this docx",
			"create an outline for this article",
			"proofread this document",
			"compare these two documents",
			// 中文
			"帮我总结这篇文档",
			"把这段文章翻译成英文",
			"提取 PDF 的关键内容",
			"帮我审查这份合同",
			"基于这份文档写个报告",
			"分析这篇论文",
			"从 Word 文档提取文字",
			"给这篇文章写个大纲",
			"校对这份文档",
			"比较这两份文档",
			// 长请求
			"帮我总结这篇 50 页的技术报告，提取核心观点、关键数据和结论",
			"审查这份软件开发合同，检查条款是否合理、是否存在法律风险",
			// 短请求
			"总结文档",
			"翻译文章",
			"提取要点",
		},
	},

	// ===== document_pool 负例 =====
	{
		Pool:        PoolDocument,
		IsNegative:  true,
		Description: "不是文档处理：简单问答",
		Prompts: []string{
			"今天天气怎么样",
			"推荐一部电影",
			"帮我讲个笑话",
			"什么是机器学习",
			"解释一下这个概念",
			"你好",
		},
	},

	// ===== cheap_chat_pool 正例 =====
	{
		Pool: PoolCheap,
		Description: "简单聊天：日常问答、基础解释",
		Prompts: []string{
			// 英文
			"hello, how are you",
			"what's the weather today",
			"what time is it",
			"recommend a good movie",
			"what is Python",
			"explain what is an API",
			"translate hello to Chinese",
			"write a short poem",
			"tell me a joke",
			"what is the capital of France",
			// 中文
			"你好",
			"今天天气怎么样",
			"现在几点了",
			"推荐一部电影",
			"什么是 Python",
			"解释一下什么是 API",
			"把 hello 翻译成中文",
			"帮我写首诗",
			"给我讲个笑话",
			"法国的首都是哪里",
			"帮我推荐几本书",
			"今天星期几",
			"解释这个成语的意思",
			"什么是区块链",
			"介绍一下这个产品",
			// 短请求
			"hi",
			"你好",
			"干嘛呢",
			"在吗",
		},
	},

	// ===== cheap_chat_pool 负例（不应该去 cheap 的） =====
	{
		Pool:        PoolCheap,
		IsNegative:  true,
		Description: "不应该去 cheap：复杂任务",
		Prompts: []string{
			"用 Python 实现一个机器学习模型",
			"帮我开发一个完整的网站",
			"写一个复杂的算法",
			"分析这些数据并生成报告",
			"帮我调试这段代码",
		},
	},

	// ===== general_pool 正例 =====
	{
		Pool: PoolDefault,
		Description: "通用池：复杂或无法明确分类的请求",
		Prompts: []string{
			// 复杂但不确定分类的
			"帮我设计一个系统的架构方案",
			"如何提升产品的用户体验",
			"给我一个完整的项目计划",
			"分析这个业务场景并给出解决方案",
			"帮我规划一下职业发展",
			"如何做好团队管理",
			"给我讲讲这个技术领域的发展",
			"帮我评估这个方案可行性",
			"如何解决这个复杂的业务问题",
			"给我一个综合性的建议",
		},
	},
}

// PrototypeScorer 基于原型的打分器
type PrototypeScorer struct {
	prototypes []PoolPrototype
	router     *TokenOverlapSimilarityRouter
}

// NewPrototypeScorer 创建原型打分器
func NewPrototypeScorer() *PrototypeScorer {
	return &PrototypeScorer{
		prototypes: PoolPrototypes,
		router:     NewTokenOverlapSimilarityRouter(),
	}
}

// ScoreWithPrototypes 使用原型进行打分
// 与单个原型描述不同，使用多个原型的 Top-K 聚合分数
func (s *PrototypeScorer) ScoreWithPrototypes(prompt string) map[string]float64 {
	scores := make(map[string]float64)

	// 收集每个 pool 的原型
	poolPrototypes := make(map[PreferredPool][]string)
	poolNegativePrototypes := make(map[PreferredPool][]string)

	for _, proto := range s.prototypes {
		if proto.IsNegative {
			poolNegativePrototypes[proto.Pool] = append(poolNegativePrototypes[proto.Pool], proto.Prompts...)
		} else {
			poolPrototypes[proto.Pool] = append(poolPrototypes[proto.Pool], proto.Prompts...)
		}
	}

	// 对每个 pool 计算分数
	for pool := range PreferredPoolNames {
		positivePrompts := poolPrototypes[pool]
		negativePrompts := poolNegativePrototypes[pool]

		if len(positivePrompts) == 0 {
			continue
		}

		// 计算与正例的相似度
		positiveScores := []float64{}
		for _, protoPrompt := range positivePrompts {
			score := s.calculateSimilarity(prompt, protoPrompt)
			positiveScores = append(positiveScores, score)
		}

		// 取 Top-K 平均
		k := 5
		if len(positiveScores) < k {
			k = len(positiveScores)
		}
		if k > 0 {
			sort.Float64s(positiveScores)
			topKScores := positiveScores[len(positiveScores)-k:]
			avgPositiveScore := 0.0
			for _, sc := range topKScores {
				avgPositiveScore += sc
			}
			avgPositiveScore /= float64(k)

			// 计算与负例的相似度（用于降低分数）
			negativeScores := []float64{}
			for _, negPrompt := range negativePrompts {
				score := s.calculateSimilarity(prompt, negPrompt)
				negativeScores = append(negativeScores, score)
			}

			avgNegativeScore := 0.0
			if len(negativeScores) > 0 {
				for _, sc := range negativeScores {
					avgNegativeScore += sc
				}
				avgNegativeScore /= float64(len(negativeScores))
			}

			// 最终分数 = 正例分数 - 负例分数 * 0.5
			finalScore := avgPositiveScore - avgNegativeScore*0.3
			if finalScore < 0 {
				finalScore = 0
			}

			scores[string(pool)] = Round(finalScore)
		}
	}

	return scores
}

// calculateSimilarity 计算两个文本的相似度
func (s *PrototypeScorer) calculateSimilarity(prompt, prototype string) float64 {
	// 使用简单的token overlap来比较
	// 由于prototype是示例文本，我们使用一种简化的方法
	normalizedPrompt := strings.ToLower(prompt)
	normalizedPrototype := strings.ToLower(prototype)

	// 计算词重叠
	promptWords := strings.Fields(normalizedPrompt)
	protoWords := strings.Fields(normalizedPrototype)

	overlap := 0
	protoWordSet := make(map[string]bool)
	for _, w := range protoWords {
		protoWordSet[w] = true
	}

	for _, w := range promptWords {
		if protoWordSet[w] && len(w) > 1 {
			overlap++
		}
	}

	// Jaccard 相似度
	if len(promptWords) == 0 || len(protoWords) == 0 {
		return 0
	}

	jaccard := float64(overlap) / float64(len(promptWords)+len(protoWords)-overlap)
	score := jaccard * 2 // 放大得分

	if score > 1.0 {
		score = 1.0
	}

	return score
}

// GetTopPools 获取 Top-K 候选池
func (s *PrototypeScorer) GetTopPools(prompt string, k int) []struct {
	Pool  PreferredPool
	Score float64
} {
	scores := s.ScoreWithPrototypes(prompt)

	// 排序
	poolScores := make([]struct {
		Pool  PreferredPool
		Score float64
	}, 0)

	for pool, score := range scores {
		poolScores = append(poolScores, struct {
			Pool  PreferredPool
			Score float64
		}{
			Pool:  PreferredPool(pool),
			Score: score,
		})
	}

	sort.Slice(poolScores, func(i, j int) bool {
		return poolScores[i].Score > poolScores[j].Score
	})

	if len(poolScores) < k {
		k = len(poolScores)
	}

	return poolScores[:k]
}

// PreferredPoolNames 有效的池名称
var PreferredPoolNames = map[PreferredPool]string{
	PoolCode:             "code_pool",
	PoolData:             "data_pool",
	PoolVision:           "vision_pool",
	PoolDocument:         "document_pool",
	PoolImageGeneration:  "image_generation_pool",
	PoolCheap:            "cheap_chat_pool",
	PoolDefault:          "general_pool",
}

// Ensure interface is satisfied
var _ = PrototypeScorer{}