# Reproducible Evaluation Results

This page records only results reproducible from public repository data and a
documented command. It does not promote untraceable high scores or describe a
development fixture as production performance.

## Routing Eval V1

| Item | Value |
| --- | --- |
| Dataset | `testdata/playground/routing_eval_v1.jsonl` |
| Source | Project-maintained, manually labeled development routing fixture |
| Cases | 119 |
| Command | `go test -run '^TestRoutingEvalCases$' -count=1 -v` |
| Pool accuracy | 74.79% (89/119) |
| Tier accuracy | 77.31% (92/119) |
| Fallback match rate | 99.16% (118/119) |
| Run date | 2026-08-20 |
| Mode | Local offline routing evaluation; no upstream model call |

## Interpretation

- This is a development baseline for detecting regressions, not a claim of
  final generalization quality.
- Pool is the task capability category; Tier is the recommended model strength.
  They should be interpreted independently.
- The 119 records can be imported into Playground for batch replay. Playground
  local-vs-Official group agreement measures agreement between classifiers, not
  accuracy against these labels.
- The independent 280-case Holdout V2 is frozen and deliberately not public.
  It is reserved for final evaluation and must not be used for tuning, rules,
  thresholds, or examples.

## Result reporting rule

Every future result must state the dataset ID, case count, source, label status,
code version/commit, complete command, run date, Official/vLLM usage, and
whether any upstream model was called.
