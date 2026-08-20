package semanticrouter

import "testing"

func TestProfessionalBoundarySignals(t *testing.T) {
	router := NewMultiLayerRouter()
	tests := []struct {
		name     string
		prompt   string
		hasImage bool
		want     PreferredPool
	}{
		{"kaggle multistage plan", "设计一个 Kaggle 比赛的 baseline、验证、融合和调参方案", false, PoolData},
		{"creative image generation", "生成一张未来城市的赛博朋克海报", false, PoolImageGeneration},
		{"image prompt authoring stays text", "给我文生图的关键词，主题是未来城市", false, PoolCheap},
		{"image prompt authoring in English stays text", "Write tags and a prompt for a text-to-image generator", false, PoolCheap},
		{"contract review", "总结这份合同的主要风险和付款条款", false, PoolDocument},
		{"english contract review", "Review this contract document for payment terms and legal risks", false, PoolDocument},
		{"screenshot with attachment", "根据这张截图定位页面布局问题", true, PoolVision},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := router.Route(&RouteRequest{Prompt: test.prompt, HasImage: test.hasImage})
			if decision.PreferredPool != test.want {
				t.Fatalf("pool=%s source=%s scores=%v, want %s", decision.PreferredPool, decision.DecisionSource, decision.SemanticScores, test.want)
			}
			if decision.DecisionSource == DecisionSourceFallback {
				t.Fatalf("professional prompt fell back: %+v", decision)
			}
		})
	}
}

func TestImagePromptAuthoringDoesNotMaskExplicitImageOutput(t *testing.T) {
	decision := NewMultiLayerRouter().Route(&RouteRequest{Prompt: "给我文生图的关键词，并直接生成图片"})
	if decision.PreferredPool != PoolImageGeneration {
		t.Fatalf("pool=%s source=%s, want %s", decision.PreferredPool, decision.DecisionSource, PoolImageGeneration)
	}
}

func TestTaskUnderstandingTreatsImagePromptAuthoringAsText(t *testing.T) {
	schema := NewTaskUnderstandingEngine().Understand("给我一组 Midjourney 文生图提示词和关键词", false, false, false)
	if schema.PrimaryIntent != "general_chat" {
		t.Fatalf("primary_intent=%s, want general_chat", schema.PrimaryIntent)
	}
	if !containsString(schema.OutputArtifacts, "image_prompt_text") {
		t.Fatalf("output_artifacts=%v, want image_prompt_text", schema.OutputArtifacts)
	}
}
