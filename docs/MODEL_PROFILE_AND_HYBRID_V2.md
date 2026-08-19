# Model Profile And Hybrid V2

## Current Safety State

- API-key model group remains restricted to the configured group.
- Candidate Ranking V2 is shadow/dry-run only.
- Hybrid tier override is advisory only.
- `final_tier` remains the RuleBased result.
- `takeover_enabled=false`.
- No real upstream model is called by the evaluation path.

## Model Profile Evidence

Model capability fields are task-specific and must not be confused with the
prompt-level Official vLLM semantic score.

```text
capability_score = model-level prior or benchmark evidence
semantic_score   = prompt-level classifier score
runtime_score    = current account load/priority/cost signal
final_score      = candidate ranking score
```

Every model profile now carries:

- `profile_source`
- `score_confidence`
- `benchmark_version`
- `evaluated_at`

`starter_registry` and `platform_catalog_prior` are E0 staging priors. They
are not production benchmark truth. Production promotion requires dated
external benchmark evidence, local evaluation, or shadow outcome data.

## Candidate Ranking V2

```text
final_score = 0.55 * capability_score
            + 0.20 * tier_fit_score
            + 0.25 * runtime_score
```

The runtime score combines load, priority, and cost. The old Scheduler main
result is preserved; the V2 result is observable through Shadow only.

## Hybrid Override Eligibility

Hybrid produces an override candidate only when:

1. RuleBased and Hybrid tiers disagree.
2. RouteLLM is available.
3. The learned strong/weak probability reaches the configured threshold.
4. A policy minimum tier is not violated.

Default:

```text
routellm_boundary_override_confidence_threshold=0.80
```

The result records:

- `hybrid_disagreement`
- `override_eligible`
- `override_reason`
- `override_confidence_threshold`

An eligible result still does not change `final_tier` while takeover is
disabled.

## Validation Interpretation

The existing Hybrid validation used `truth_proxy`, not human-confirmed truth.
It showed a small improvement but did not eliminate disagreement. The next
evaluation should report disagreement coverage, corrected disagreements,
new disagreements, abstention rate, strong recall, and over-routing rate on a
reviewed boundary slice.
