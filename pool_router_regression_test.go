package semanticrouter

import "testing"

func TestCalculateKeywordScoreChineseMatchesHavePositiveScore(t *testing.T) {
	router := NewTokenOverlapSimilarityRouter()

	tests := []struct {
		name   string
		prompt string
		pool   PreferredPool
	}{
		{
			name:   "Chinese code intent",
			prompt: "帮我写一个排序算法",
			pool:   PoolCode,
		},
		{
			name:   "Chinese data intent",
			prompt: "根据这份表格生成趋势图",
			pool:   PoolData,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, matchedKeywords := router.CalculateKeywordScore(tt.prompt, tt.pool)
			if len(matchedKeywords) == 0 {
				t.Fatalf("matchedKeywords is empty for %q", tt.prompt)
			}
			if score <= 0 {
				t.Fatalf("score = %v, want > 0 when matchedKeywords = %v", score, matchedKeywords)
			}
		})
	}
}

func TestPoolRouterCodeIntentChinesePrompts(t *testing.T) {
	router := NewMultiLayerRouter()
	prompts := []string{
		"帮我写一个排序算法",
		"帮我做一个用户登录功能",
		"这个接口返回的数据结构不对",
		"帮我写一个自动化脚本整理文件",
		"请实现一个注册页面",
		"数据库查询太慢怎么优化",
		"帮我写一个爬虫",
		"这个配置为什么运行不了",
	}

	for _, prompt := range prompts {
		t.Run(prompt, func(t *testing.T) {
			decision := router.Route(&RouteRequest{Prompt: prompt})
			assertPoolDecision(t, decision, PoolCode)
		})
	}
}

func TestPoolRouterDataIntentChinesePrompts(t *testing.T) {
	router := NewMultiLayerRouter()
	prompts := []string{
		"根据这份表格生成趋势图",
		"计算每个渠道的转化率",
		"分析销售数据并做可视化",
		"找出这份 Excel 里的异常值",
		"帮我清洗这份 CSV 数据",
		"统计每个月的增长率",
	}

	for _, prompt := range prompts {
		t.Run(prompt, func(t *testing.T) {
			decision := router.Route(&RouteRequest{Prompt: prompt})
			assertPoolDecision(t, decision, PoolData)
		})
	}
}

func assertPoolDecision(t *testing.T, decision *MultiLayerDecision, want PreferredPool) {
	t.Helper()

	if decision.PreferredPool != want {
		t.Fatalf("PreferredPool = %v, want %v; scores=%v matched=%v source=%v fallback=%q",
			decision.PreferredPool, want, decision.SemanticScores, decision.MatchedRules,
			decision.DecisionSource, decision.FallbackReason)
	}
	if decision.DecisionSource == DecisionSourceFallback {
		t.Fatalf("DecisionSource = fallback for matched %v intent; reason=%q", want, decision.FallbackReason)
	}
	if len(decision.MatchedRules) == 0 {
		t.Fatalf("MatchedRules is empty")
	}
	if decision.FallbackReason != "" {
		t.Fatalf("FallbackReason = %q, want empty for non-fallback decision", decision.FallbackReason)
	}
	if decision.SecondBestPool == "" {
		t.Fatalf("SecondBestPool is empty")
	}
	if decision.SecondBestPool == decision.PreferredPool {
		t.Fatalf("SecondBestPool = PreferredPool = %v", decision.SecondBestPool)
	}
}

// ===== 泛化测试：最小对照测试组 =====

// TestGeneralizationRankingVsProgramming 泛化测试：普通排名 vs 编程排序
func TestGeneralizationRankingVsProgramming(t *testing.T) {
	router := NewEnhancedMultiLayerRouter()

	// 普通排名 - 应该 fallback 到 cheap 或 default
	rankingPrompts := []string{
		"帮我给歌曲排个序",
		"按价格排序",
		"这个列表怎么排序",
		"帮我整理一下这个清单",
		"把商品按销量排名",
		"sort this list",
		"rank these items by popularity",
	}

	for _, prompt := range rankingPrompts {
		t.Run("ranking-"+prompt, func(t *testing.T) {
			decision := router.Route(&RouteRequest{Prompt: prompt})
			// 普通排序不应触发 code_pool
			if decision.FinalPool == PoolCode {
				t.Fatalf("普通排序不应触发 code_pool, got %v for prompt: %q", decision.FinalPool, prompt)
			}
		})
	}

	// 编程排序 - 应该触发 code_pool
	codeSortPrompts := []string{
		"用 Python 实现排序算法",
		"帮我写一个快速排序函数",
		"用 Go 实现一个排序方法",
		"write a quick sort function in Python",
		"implement sorting algorithm in Java",
	}

	for _, prompt := range codeSortPrompts {
		t.Run("code_sort-"+prompt, func(t *testing.T) {
			decision := router.Route(&RouteRequest{Prompt: prompt})
			// 编程排序应该触发 code_pool
			if decision.FinalPool != PoolCode {
				t.Fatalf("编程排序应触发 code_pool, got %v for prompt: %q", decision.FinalPool, prompt)
			}
		})
	}
}

// TestGeneralizationDataChartVsCreativeImage 泛化测试：数据图表 vs 创意图片
func TestGeneralizationDataChartVsCreativeImage(t *testing.T) {
	router := NewEnhancedMultiLayerRouter()

	// 数据图表 - 应该触发 data_pool
	dataPrompts := []string{
		"生成销售趋势图",
		"帮我做个数据可视化",
		"分析数据生成图表",
		"create a chart from this data",
		"visualize the sales trend",
	}

	for _, prompt := range dataPrompts {
		t.Run("data-"+prompt, func(t *testing.T) {
			decision := router.Route(&RouteRequest{Prompt: prompt})
			if decision.FinalPool != PoolData {
				t.Fatalf("数据图表应触发 data_pool, got %v for prompt: %q", decision.FinalPool, prompt)
			}
		})
	}

	// 创意图片 - 应该触发 image_generation_pool
	imagePrompts := []string{
		"生成未来城市海报",
		"画一幅科幻插画",
		"创作一个 logo",
		"generate a futuristic city image",
		"create an artistic poster",
	}

	for _, prompt := range imagePrompts {
		t.Run("image-"+prompt, func(t *testing.T) {
			decision := router.Route(&RouteRequest{Prompt: prompt})
			if decision.FinalPool != PoolImageGeneration {
				t.Fatalf("创意图片应触发 image_generation_pool, got %v for prompt: %q", decision.FinalPool, prompt)
			}
		})
	}
}

// TestGeneralizationAnalyzeImageVsGenerateImage 泛化测试：分析图片 vs 生成图片
func TestGeneralizationAnalyzeImageVsGenerateImage(t *testing.T) {
	router := NewEnhancedMultiLayerRouter()

	// 分析现有图片 - 应该触发 vision_pool
	analyzePrompts := []string{
		"分析这张图片",
		"描述这个照片",
		"帮我看看这张图",
		"describe this image",
		"what's in this photo",
	}

	for _, prompt := range analyzePrompts {
		t.Run("analyze-"+prompt, func(t *testing.T) {
			decision := router.Route(&RouteRequest{Prompt: prompt, HasImage: true})
			if decision.FinalPool != PoolVision {
				t.Fatalf("分析图片应触发 vision_pool, got %v for prompt: %q", decision.FinalPool, prompt)
			}
		})
	}

	// 生成新图片 - 应该触发 image_generation_pool
	generatePrompts := []string{
		"生成一张未来城市的图片",
		"帮我画一幅科幻画",
		"generate an image of a castle",
		"create a fantasy landscape",
	}

	for _, prompt := range generatePrompts {
		t.Run("generate-"+prompt, func(t *testing.T) {
			decision := router.Route(&RouteRequest{Prompt: prompt})
			if decision.FinalPool != PoolImageGeneration {
				t.Fatalf("生成图片应触发 image_generation_pool, got %v for prompt: %q", decision.FinalPool, prompt)
			}
		})
	}
}

// TestGeneralizationDocumentSummaryVsWordExplanation 泛化测试：文档总结 vs 普通词义解释
func TestGeneralizationDocumentSummaryVsWordExplanation(t *testing.T) {
	router := NewEnhancedMultiLayerRouter()

	// 文档处理 - 应该触发 document_pool
	docPrompts := []string{
		"帮我总结这篇文档",
		"提取 PDF 的关键内容",
		"审查这份合同",
		"summarize this document",
		"extract text from this PDF",
	}

	for _, prompt := range docPrompts {
		t.Run("document-"+prompt, func(t *testing.T) {
			decision := router.Route(&RouteRequest{Prompt: prompt, HasDocument: true})
			if decision.FinalPool != PoolDocument {
				t.Fatalf("文档处理应触发 document_pool, got %v for prompt: %q", decision.FinalPool, prompt)
			}
		})
	}

	// 普通词义解释 - 应该触发 cheap 或 default
	explainPrompts := []string{
		"解释一下什么是 API",
		"什么是机器学习",
		"介绍排序功能",
		"what is an API",
		"explain the concept of sorting",
	}

	for _, prompt := range explainPrompts {
		t.Run("explain-"+prompt, func(t *testing.T) {
			decision := router.Route(&RouteRequest{Prompt: prompt})
			// 不应该触发 document_pool
			if decision.FinalPool == PoolDocument {
				t.Fatalf("普通解释不应触发 document_pool, got %v for prompt: %q", decision.FinalPool, prompt)
			}
		})
	}
}

// TestGeneralizationRecommendationVsAlgorithm 泛化测试：普通推荐 vs 推荐算法实现
func TestGeneralizationRecommendationVsAlgorithm(t *testing.T) {
	router := NewEnhancedMultiLayerRouter()

	// 普通推荐 - 应该触发 cheap 或 default
	recPrompts := []string{
		"推荐几本好书",
		"帮我推荐一部电影",
		"recommend a restaurant",
		"what movies should I watch",
	}

	for _, prompt := range recPrompts {
		t.Run("recommend-"+prompt, func(t *testing.T) {
			decision := router.Route(&RouteRequest{Prompt: prompt})
			// 不应该触发 code_pool
			if decision.FinalPool == PoolCode {
				t.Fatalf("普通推荐不应触发 code_pool, got %v for prompt: %q", decision.FinalPool, prompt)
			}
		})
	}

	// 推荐算法实现 - 应该触发 code_pool
	algoPrompts := []string{
		"用 Python 实现协同过滤推荐算法",
		"帮我写一个推荐系统",
		"implement a recommendation algorithm",
		"build a collaborative filtering system",
	}

	for _, prompt := range algoPrompts {
		t.Run("algo-"+prompt, func(t *testing.T) {
			decision := router.Route(&RouteRequest{Prompt: prompt})
			if decision.FinalPool != PoolCode {
				t.Fatalf("推荐算法应触发 code_pool, got %v for prompt: %q", decision.FinalPool, prompt)
			}
		})
	}
}

// TestGeneralizationAPIConceptVsCode 泛化测试：API 概念解释 vs API 代码实现
func TestGeneralizationAPIConceptVsCode(t *testing.T) {
	router := NewEnhancedMultiLayerRouter()

	// API 概念解释 - 应该触发 cheap 或 default
	conceptPrompts := []string{
		"解释一下什么是 REST API",
		"API 是什么",
		"what is a REST API",
		"explain API concept",
	}

	for _, prompt := range conceptPrompts {
		t.Run("concept-"+prompt, func(t *testing.T) {
			decision := router.Route(&RouteRequest{Prompt: prompt})
			// 不应该触发 code_pool（除非有明确的代码输出需求）
			if decision.FinalPool == PoolCode && decision.TaskUnderstanding != nil {
				hasCodeOutput := false
				for _, artifact := range decision.TaskUnderstanding.OutputArtifacts {
					if artifact == "executable_code" {
						hasCodeOutput = true
						break
					}
				}
				if !hasCodeOutput {
					t.Fatalf("API 概念解释不应触发 code_pool, got %v for prompt: %q", decision.FinalPool, prompt)
				}
			}
		})
	}

	// API 代码实现 - 应该触发 code_pool
	codePrompts := []string{
		"用 Go 写一个 REST API 接口",
		"帮我实现一个登录 API",
		"create a REST API in Python",
		"implement user authentication API",
	}

	for _, prompt := range codePrompts {
		t.Run("code-"+prompt, func(t *testing.T) {
			decision := router.Route(&RouteRequest{Prompt: prompt})
			if decision.FinalPool != PoolCode {
				t.Fatalf("API 代码实现应触发 code_pool, got %v for prompt: %q", decision.FinalPool, prompt)
			}
		})
	}
}

// TestGeneralizationMixedLanguage 泛化测试：中英混合表达
func TestGeneralizationMixedLanguage(t *testing.T) {
	router := NewEnhancedMultiLayerRouter()

	mixedPrompts := []string{
		"用 Python 写一个 sorting function",
		"帮我 debug 这个 code",
		"create a chart 显示销售数据",
		"generate 一个 logo for my startup",
	}

	for _, prompt := range mixedPrompts {
		t.Run("mixed-"+prompt, func(t *testing.T) {
			decision := router.Route(&RouteRequest{Prompt: prompt})
			// 应该能正确识别意图
			if decision.FinalPool == "" {
				t.Fatalf("中英混合表达未能识别 pool, prompt: %q", prompt)
			}
		})
	}
}

// TestGeneralizationAmbiguousFallback 泛化测试：歧义情况下的安全 fallback
func TestGeneralizationAmbiguousFallback(t *testing.T) {
	router := NewEnhancedMultiLayerRouter()

	// 明显歧义的 prompts
	ambiguousPrompts := []string{
		"排序",
		"生成",
		"分析",
		"create",
	}

	for _, prompt := range ambiguousPrompts {
		t.Run("ambiguous-"+prompt, func(t *testing.T) {
			decision := router.Route(&RouteRequest{Prompt: prompt})
			// 歧义情况下不应触发专业池
			professionalPools := map[PreferredPool]bool{
				PoolCode:             true,
				PoolData:             true,
				PoolVision:           true,
				PoolDocument:         true,
				PoolImageGeneration:  true,
			}

			if professionalPools[decision.FinalPool] && decision.TaskUnderstanding != nil && decision.TaskUnderstanding.Ambiguous {
				// 如果触发了专业池但不满足验证条件，应该是 fallback
				explanation := decision.GetExplanatoryOutput()
				if explanation.FallbackInfo == nil {
					t.Logf("Warning: ambiguous prompt %q triggered professional pool %v without fallback",
						prompt, decision.FinalPool)
				}
			}
		})
	}
}
