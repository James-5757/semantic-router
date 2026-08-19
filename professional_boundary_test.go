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
