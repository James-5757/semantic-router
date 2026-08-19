# Staging Shadow-Only

> Current status: local/staging shadow-only. Candidate Ranking V2, model
> profiles, and Hybrid are observational; takeover remains disabled.

## Start

From the `semantic_router` directory:

```powershell
$env:SEMANTIC_ROUTER_TCP_LISTEN_ADDRESS = "127.0.0.1:9101"
$env:SEMANTIC_ROUTER_STAGING_GROUP_ID = "1001"
go run ./cmd/staging-shadow
```

The service uses `StagingAccountRepository` and `NewDefaultModelRegistry`.
The fixture contains all 14 model profiles from the candidate registry across
OpenAI, DeepSeek, Qwen, Claude, MiniMax, image, technical, data, document,
vision, cheap-chat, disabled, and unschedulable accounts. It contains 17
account records, 15 active/schedulable records, and no credentials or real
upstream endpoint.

## Safety

```text
shadow_enabled   = true
dry_run_enabled  = true
takeover_enabled = false
upstream_called  = false
```

The TCP response is a recommendation only. A Token Cloud caller must continue
using its old Scheduler result as the live result. `TokenCloudShadowAdapter`
provides this invariant explicitly and records old/new account, model, pool,
agreement, error, and latency fields.

## Candidate Checks

- `code_pool` selects only code-capable accounts.
- `data_pool` selects only data-capable accounts.
- `vision_pool` selects only vision-capable accounts.
- `document_pool` selects only document-capable accounts.
- Disabled and unschedulable accounts are excluded.
- Account ID `0` is rejected.
- Optional `model_ids` restricts selection to the group candidate list.

The staging fixture is not production account data and must be replaced by the
real repository adapter before any production rollout.

## Local V2 Observability

Playground scheduler output now exposes the old Scheduler result alongside the
V2 suggestion:

- `old_selected_account_id` / `new_suggested_account_id`
- `old_selected_model` / `new_suggested_model`
- `old_vs_new_agreement`
- `ranking_margin`

Model profile metadata includes `profile_source`, `evidence_source`,
`score_confidence`, `benchmark_version`, and `evaluated_at`. The current
`starter_prior_not_benchmark` and `catalog_prior_not_benchmark` values are
staging priors, not production benchmark claims.
