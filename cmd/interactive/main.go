package main

import (
	"encoding/json"
	"fmt"
	"log"
	"semantic-router"
)

func main() {
	// 初始化组件
	semanticRouter := semanticrouter.NewRuleBasedSemanticRouter()
	tierRouter := semanticrouter.NewRuleBasedTierRouter()
	scheduler := semanticrouter.NewMockScheduler()
	scheduler.SetupMockAccounts()

	fmt.Println("=== Semantic Router Interactive Test ===")
	fmt.Println()

	// 测试用例
	testCases := []struct {
		name   string
		prompt string
		model  string
	}{
		{"普通文本", "你好，请介绍一下北京的历史", "gpt-3.5-turbo"},
		{"代码请求", "Write a quick sort in Python", "gpt-4"},
		{"图片请求", "请描述这张图片的内容", "gpt-4o"},
		{"文档请求", "请总结这个文档的内容", "gpt-4"},
		{"中文代码", "帮我写一个排序算法", "gpt-4"},
		{"调试请求", "Debug this code and fix the error", "gpt-4"},
	}

	for i, tc := range testCases {
		fmt.Printf("%d. %s: %s\n", i+1, tc.name, tc.prompt)
	}
	fmt.Println()
	fmt.Println("=== 测试结果 ===")
	fmt.Println()

	for _, tc := range testCases {
		// 语义路由
		routeReq := &semanticrouter.RouteRequest{
			Model:       tc.model,
			Prompt:      tc.prompt,
			HasImage:    false,
			HasDocument: false,
		}

		semanticDecision, err := semanticRouter.Route(nil, routeReq)
		if err != nil {
			log.Printf("Error: %v", err)
			continue
		}

		// Tier 路由
		tierDecision, err := tierRouter.Route(nil, tc.model, semanticDecision.TaskType)
		if err != nil {
			log.Printf("Error: %v", err)
			continue
		}

		// Scheduler 选择
		schedulerReq := &semanticrouter.SchedulerSelectRequest{
			Model:                tc.model,
			PreferredPool:        semanticDecision.PreferredPool,
			PreferredTier:        tierDecision.PreferredTier,
			TaskType:             semanticDecision.TaskType,
			RequiredCapabilities: semanticDecision.RequiredCapabilities,
		}

		schedulerResult := scheduler.Select(schedulerReq)

		// 输出结果
		fmt.Printf("[%s]\n", tc.name)
		fmt.Printf("  Prompt: %s\n", tc.prompt)
		fmt.Printf("  语义路由: pool=%s, task=%s, matched=%s\n",
			semanticDecision.PreferredPool,
			semanticDecision.TaskType,
			semanticDecision.MatchedRule)
		fmt.Printf("  Tier路由: tier=%s, reason=%s\n",
			tierDecision.PreferredTier,
			tierDecision.Reason)
		if schedulerResult.Error == nil {
			fmt.Printf("  调度结果: account_id=%d, pool=%s, layer=%s\n",
				schedulerResult.SelectedAccountID,
				schedulerResult.PoolUsed,
				schedulerResult.Layer)
		} else {
			fmt.Printf("  调度结果: error=%v\n", schedulerResult.Error)
		}
		fmt.Println()
	}

	// JSON 输出
	fmt.Println("=== JSON 输出示例 (代码请求) ===")
	exampleReq := testCases[1] // 代码请求
	routeReq := &semanticrouter.RouteRequest{
		Model:       exampleReq.model,
		Prompt:      exampleReq.prompt,
		HasImage:    false,
		HasDocument: false,
	}

	semanticDecision, _ := semanticRouter.Route(nil, routeReq)
	tierDecision, _ := tierRouter.Route(nil, exampleReq.model, semanticDecision.TaskType)
	schedulerReq := &semanticrouter.SchedulerSelectRequest{
		Model:                exampleReq.model,
		PreferredPool:        semanticDecision.PreferredPool,
		PreferredTier:        tierDecision.PreferredTier,
		TaskType:             semanticDecision.TaskType,
		RequiredCapabilities: semanticDecision.RequiredCapabilities,
	}
	schedulerResult := scheduler.Select(schedulerReq)

	result := map[string]interface{}{
		"success": true,
		"semantic_decision": map[string]string{
			"preferred_pool": string(semanticDecision.PreferredPool),
			"task_type":      string(semanticDecision.TaskType),
			"modality":       string(semanticDecision.Modality),
			"matched_rule":   semanticDecision.MatchedRule,
		},
		"tier_decision": map[string]string{
			"preferred_tier": string(tierDecision.PreferredTier),
			"reason":         tierDecision.Reason,
		},
		"scheduler_decision": map[string]interface{}{
			"selected_account_id": schedulerResult.SelectedAccountID,
			"pool_used":           schedulerResult.PoolUsed,
			"scheduler_layer":     schedulerResult.Layer,
		},
	}

	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(jsonBytes))
}