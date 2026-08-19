package semanticrouter

type MultiLayerSemanticRouter struct {
	router *MultiLayerRouter
}

func NewMultiLayerSemanticRouter() *MultiLayerSemanticRouter {
	return &MultiLayerSemanticRouter{router: NewMultiLayerRouter()}
}

func (r *MultiLayerSemanticRouter) Route(ctx interface{}, request interface{}) (*SemanticRouteDecision, error) {
	routeReq, ok := request.(*RouteRequest)
	if !ok || routeReq == nil {
		return NewRuleBasedSemanticRouter().Route(ctx, request)
	}

	decision := r.router.Route(routeReq)
	return &SemanticRouteDecision{
		PreferredPool:        decision.PreferredPool,
		RequiredCapabilities: decision.RequiredCapabilities,
		TaskType:             decision.TaskType,
		Modality:             decision.Modality,
		Confidence:           decision.Confidence,
		MatchedRule:          firstMatchedRule(decision.MatchedRules),
	}, nil
}

func (r *MultiLayerSemanticRouter) GetName() string {
	return "MultiLayerSemanticRouter"
}

func firstMatchedRule(rules []string) string {
	if len(rules) == 0 {
		return ""
	}
	return rules[0]
}

var _ SemanticRouter = (*MultiLayerSemanticRouter)(nil)
