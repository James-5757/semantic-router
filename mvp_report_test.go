package semanticrouter

import (
	"encoding/json"
	"os"
	"testing"
)

type mvpFailureReportCase struct {
	Index               int                `json:"index"`
	Prompt              string             `json:"prompt"`
	ExpectedPool        string             `json:"expected_pool"`
	GotPool             string             `json:"got_pool"`
	ExpectedTier        string             `json:"expected_tier"`
	GotTier             string             `json:"got_tier"`
	MatchedRules        []string           `json:"matched_rules"`
	SemanticScores      map[string]float64 `json:"semantic_scores"`
	Confidence          float64            `json:"confidence"`
	FinalDecisionSource string             `json:"final_decision_source"`
	FallbackReason      string             `json:"fallback_reason,omitempty"`
}

func TestMVPV01FailureReport(t *testing.T) {
	if os.Getenv("SEMANTIC_ROUTER_REPORT") != "1" {
		t.Skip("set SEMANTIC_ROUTER_REPORT=1 to emit the MVP v0.1 failure report")
	}

	cases, err := loadRoutingEvalCases()
	if err != nil {
		t.Fatalf("load eval cases: %v", err)
	}

	multiLayerRouter := NewMultiLayerRouter()
	tierRouter := NewRuleBasedTierRouter()
	labelMap := map[string]string{
		"code":                  "code",
		"data":                  "data",
		"vision":                "vision",
		"document":              "document",
		"image_generation":      "image_generation",
		"default":               "default",
		"cheap":                 "cheap",
		"general":               "default",
		"private":               "private",
		"code_pool":             "code",
		"data_pool":             "data",
		"vision_pool":           "vision",
		"document_pool":         "document",
		"image_generation_pool": "image_generation",
	}

	codeToDefault := make([]mvpFailureReportCase, 0)
	tierFailures := make([]mvpFailureReportCase, 0)

	for i, c := range cases {
		expectedPool := normalizeLabel(c.ExpectedPool, labelMap)
		expectedTier := normalizeLabel(c.ExpectedTier, labelMap)
		req := &RouteRequest{
			Prompt:      c.Prompt,
			Model:       c.Model,
			HasImage:    len(c.Images) > 0,
			HasDocument: len(c.Documents) > 0,
		}

		semanticDecision := multiLayerRouter.Route(req)
		tierDecision, _ := tierRouter.RouteWithPrompt(nil, c.Model, semanticDecision.TaskType, c.Prompt)

		reportCase := mvpFailureReportCase{
			Index:               i,
			Prompt:              c.Prompt,
			ExpectedPool:        expectedPool,
			GotPool:             string(semanticDecision.PreferredPool),
			ExpectedTier:        expectedTier,
			GotTier:             string(tierDecision.PreferredTier),
			MatchedRules:        semanticDecision.MatchedRules,
			SemanticScores:      semanticDecision.SemanticScores,
			Confidence:          semanticDecision.Confidence,
			FinalDecisionSource: string(semanticDecision.DecisionSource),
			FallbackReason:      semanticDecision.FallbackReason,
		}

		if expectedPool == string(PoolCode) && semanticDecision.PreferredPool == PoolDefault {
			codeToDefault = append(codeToDefault, reportCase)
		}
		if string(tierDecision.PreferredTier) != expectedTier {
			tierFailures = append(tierFailures, reportCase)
		}
	}

	payload := map[string][]mvpFailureReportCase{
		"code_to_default": codeToDefault,
		"tier_failures":   tierFailures,
	}
	out, _ := json.MarshalIndent(payload, "", "  ")
	t.Logf("\n%s", out)
}
