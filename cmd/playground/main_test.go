package main

import (
	"reflect"
	"testing"

	semanticrouter "semantic-router"
	vllm_pool_client "semantic-router/vllm_pool_client"
)

func TestTaskUnderstandingRegression(t *testing.T) {
	tests := []struct {
		name       string
		prompt     string
		pool       semanticrouter.PreferredPool
		wantIntent string
		wantPool   string
		wantAction string
		wantTier   semanticrouter.PreferredTier
	}{
		{
			name:       "kaggle complete modeling roadmap",
			prompt:     "我在准备 Kaggle ROG 井类预测比赛，目标是冲击奖牌，请设计从 baseline、验证、模型融合、后处理到调参的完整思路。",
			pool:       semanticrouter.PoolData,
			wantIntent: "predictive_modeling_strategy",
			wantPool:   "data_pool",
			wantAction: "plan",
			wantTier:   semanticrouter.TierStrong,
		},
		{
			name:       "baseline validation design",
			prompt:     "设计 baseline 和验证方案",
			pool:       semanticrouter.PoolData,
			wantIntent: "predictive_modeling_strategy",
			wantPool:   "data_pool",
			wantAction: "design",
			wantTier:   semanticrouter.TierStrong,
		},
		{
			name:       "complete notebook code",
			prompt:     "给出完整 Notebook 代码用于 Kaggle 预测数据集",
			pool:       semanticrouter.PoolCode,
			wantIntent: "code_generation",
			wantPool:   "code_pool",
			wantAction: "write",
			wantTier:   semanticrouter.TierWeak,
		},
		{
			name:       "csv transform",
			prompt:     "将 CSV 转换成 JSON",
			pool:       semanticrouter.PoolData,
			wantIntent: "data_analysis",
			wantPool:   "data_pool",
			wantAction: "transform",
			wantTier:   semanticrouter.TierWeak,
		},
		{
			name:       "simple greeting",
			prompt:     "你好，请介绍一下自己",
			pool:       semanticrouter.PoolCheap,
			wantIntent: "general_chat",
			wantAction: "explain",
			wantPool:   "cheap_chat_pool",
			wantTier:   semanticrouter.TierWeak,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := &semanticrouter.MultiLayerDecision{PreferredPool: test.pool, Confidence: 0.8}
			understanding := understand(playgroundRequest{Prompt: test.prompt}, decision, nil, poolName(test.pool), "", nil)
			if understanding.PrimaryIntent != test.wantIntent || understanding.PrimaryPool != test.wantPool {
				t.Fatalf("understanding=%+v", understanding)
			}
			if !contains(understanding.Actions, test.wantAction) {
				t.Fatalf("actions=%v, missing %q", understanding.Actions, test.wantAction)
			}
			complexity := analyzeTaskComplexity(test.prompt, understanding)
			if complexity.RequestedTier != test.wantTier {
				t.Fatalf("tier=%q, want %q", complexity.RequestedTier, test.wantTier)
			}
		})
	}
}

func TestKaggleUnderstandingFields(t *testing.T) {
	prompt := "我在准备 Kaggle ROG 井类预测比赛，目标是冲击奖牌，请设计从 baseline、验证、模型融合、后处理到调参的完整思路。"
	decision := &semanticrouter.MultiLayerDecision{PreferredPool: semanticrouter.PoolData, Confidence: 0.9}
	got := understand(playgroundRequest{Prompt: prompt}, decision, nil, "data_pool", "", nil)
	wantIntents := []string{"competition_strategy", "predictive_modeling", "baseline_design", "validation_design", "model_ensemble", "post_processing", "hyperparameter_tuning"}
	wantCapabilities := []string{"data_science", "machine_learning", "validation_design", "ensemble_learning", "post_processing", "hyperparameter_optimization"}
	if !reflect.DeepEqual(got.Intents, wantIntents) || !reflect.DeepEqual(got.RequiredCapabilities, wantCapabilities) {
		t.Fatalf("got intents=%v capabilities=%v", got.Intents, got.RequiredCapabilities)
	}
	if got.UnderstandingConflict {
		t.Fatal("professional data understanding must agree with its baseline")
	}
}

func TestModelTaskSignalsAddPromptHintsWithoutChangingCapabilities(t *testing.T) {
	signals := modelTaskSignals("请用中文设计一个复杂的机器学习验证方案", []string{"machine_learning", "validation_design"})
	for _, want := range []string{"machine_learning", "validation_design", "chinese", "reasoning"} {
		if !contains(signals, want) {
			t.Fatalf("signals=%v, missing %q", signals, want)
		}
	}
}

func TestGroupFirstHybridFallsBackToLocalWithoutOfficialScores(t *testing.T) {
	result := computeGroupFirstHybrid(map[string]float64{"code": 0.82, "data": 0.48, "default": 0.31}, nil, "code_pool")
	if result.SelectedGroup != "technical_models" || result.SuggestedPool != "code_pool" {
		t.Fatalf("group-first fallback=%+v", result)
	}
	if result.UsedForFinal || result.Source != "local_fallback" {
		t.Fatalf("group-first safety/source=%+v", result)
	}
}

func TestGroupFirstHybridNormalizesOfficialSemanticCategory(t *testing.T) {
	official := &vllm_pool_client.OfficialVLLMShadowResult{TopK: []vllm_pool_client.CategoryScore{{Category: "semantic_code", Score: 0.91}}}
	result := computeGroupFirstHybrid(map[string]float64{"code": 0.82}, official, "code_pool")
	if result.OfficialGroup != "technical_models" || result.SelectedGroup != "technical_models" || result.SuggestedPool != "code_pool" {
		t.Fatalf("group-first semantic category mapping=%+v", result)
	}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
