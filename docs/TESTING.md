# Testing and Evaluation

## Fast regression suite

The focused selector suite covers HTTP compatibility, secret checks, history
redaction, API-key group isolation, v1.3 account eligibility, dynamic load,
quota handling, cost/TTFT ranking and conservative Official Pool handling.

```bash
# Run from the repository root.
go test -run 'TestModelSelector|TestOfficialPoolShadow|TestSelectorDocumentRunbookGuard' -count=1 -v
```

## Shadow and scheduler safety

```bash
go test -run 'TestRealSchedulerDryRun|TestShadowMode|TestTokenCloudShadow' -count=1 -v
```

These tests verify that disabled accounts and account ID 0 are not selected,
and that shadow failures do not replace the old scheduler result.

## Full suite

```bash
go test ./...
```

The repository contains frozen Pool holdout/regression fixtures that currently
expose known baseline failures in unrelated Pool categories. Do not rewrite or
delete those fixtures to make CI appear green. Track Pool calibration changes
against a versioned holdout report and keep selector-runtime changes covered by
the focused suites above.

## Manual selector smoke test

1. Send a gzip/Base64 `user_api_call` with a group-scoped `model_list`.
2. Confirm every listed model appears once in `model_score_list`.
3. Confirm `shadow_only=true` and no upstream invocation.
4. Mark one account `schedulable=false` or set its quota to exhausted; its
   model must score zero when no other eligible mapped account exists.
5. Increase an account's `load_rate` and `waiting_count`; a healthier candidate
   should gain rank without changing the gateway's main result.

## Evaluation dimensions

| Area | Metrics |
| --- | --- |
| Pool | overall accuracy, per-pool precision/recall, confusion matrix |
| Tier | accuracy, strong recall, over-routing rate |
| Candidate ranking | top-1 stability, load-driven switch rate, group compliance |
| Reliability | selector error/fallback rate, P50/P95 latency, account-zero count |
| Shadow agreement | old-vs-new model agreement and disagreement samples |

## Adding a regression case

1. Add only redacted, reproducible input.
2. State expected Pool/Tier/model-group behavior, not a desired implementation.
3. Add the smallest test around the affected boundary.
4. Run the focused suite and record any holdout change separately.
5. Never loosen a safety test simply to accept a new recommendation.
