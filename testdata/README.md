# Public Test Data

This directory contains small, public, non-sensitive development fixtures. It
exists so a new user can exercise the Playground and reproduce documented
development metrics without relying on private captures or account data.

## Included dataset

| Dataset | Rows | Intended use | Label status |
| --- | ---: | --- | --- |
| `playground/routing_eval_v1.jsonl` | 119 | Playground batch import and development routing regression | Development labels, not a final holdout |

### Provenance and limits

- Source: project-maintained, manually labeled routing development fixture
  previously stored as `routing_eval_cases.jsonl` in the local project.
- Contents: generic Chinese and English prompts with expected Pool and Tier;
  no raw packet capture, credentials, account snapshot, or production prompt is
  included.
- It is suitable for regression and smoke evaluation, not for training a model
  or claiming production-quality accuracy.
- The independent 280-case Holdout V2 is deliberately not published here. Its
  labels must remain frozen and unavailable to tuning.

## Use in Playground

1. Open the Dataset tab in Playground.
2. Upload `playground/routing_eval_v1.jsonl`.
3. Confirm that `prompt` is selected as the prompt column.
4. Run the batch in Shadow/DryRun mode and export the result.

The Playground's local-vs-Official **group agreement** is an agreement metric,
not accuracy against this fixture's labels. The `expected_pool` and
`expected_tier` fields are used by the offline Go regression test.

## Reproduce the documented development result

```bash
go test -run '^TestRoutingEvalCases$' -count=1 -v
```

See `docs/EVALUATION_RESULTS.md` and `docs/EVALUATION_RESULTS_CN.md` for the
command, date, result, and interpretation. Do not remove failing cases merely
to improve a displayed percentage.
