# Code Map

This repository is a single Go module. Most production Go files intentionally
remain in the module root because they share one package and many routing types.
Do not move files only to make the directory look smaller: package moves would
change imports and increase integration risk. Use this map to find the correct
ownership boundary first.

## Start here

| Task | Primary files |
| --- | --- |
| Start the HTTP selector | `cmd/server/` and `server.go` |
| Start the visual debugger | `cmd/playground/` |
| Understand a request | `prompt_parser.go`, `task_understanding.go`, `output_contract.go` |
| Choose a Pool or Tier | `semantic_router.go`, `rules.go`, `tier_router.go`, `v2_router.go` |
| Rank candidate models | `model_selector.go`, `platform_model_catalog.go`, `staging_accounts.go` |
| Apply live account state | `model_selector_synced_catalog.go`, `model_selector_api_key_groups.go` |
| Inspect Shadow safety | `shadow_mode.go`, `shadow_config.go`, `shadow_metrics.go`, `token_cloud_shadow.go` |
| Call optional semantic services | `vllm_pool_client/`, `routellm_tier.go`, `routellm_tier_service.py` |

## Directory responsibilities

| Path | Responsibility |
| --- | --- |
| `cmd/server/` | Runnable Selector HTTP service. |
| `cmd/playground/` | Runnable Playground UI used for internal debugging. |
| `deploy/ubuntu/` | Ubuntu systemd, Nginx, and environment examples. |
| `docker/` | Optional Official vLLM score-only service configuration. |
| `vllm_pool_client/` | Client and protocol boundary for the optional vLLM signal. |
| `testdata/playground/` | Shareable, redacted development fixture for Playground evaluation. |
| `docs/` | Maintained user, deployment, API, and contributor guidance. |

## Tests and change boundaries

- A `*_test.go` file beside a component is its closest regression coverage.
- `output_contract_test.go`, `model_selector_test.go`, `shadow_mode_test.go`, and
  `model_selector_http_test.go` are good first tests for common integration work.
- `holdout_v2_eval_test.go` and frozen Pool fixtures are evaluation safeguards.
  Do not edit their expected cases merely to improve a reported score.
- Keep `SEMANTIC_ROUTER_TAKEOVER_ENABLED=false` in shared and staging setups.
  The Selector returns a shadow recommendation; the gateway scheduler remains
  responsible for the real upstream request.

For public setup and supported commands, return to the root [README](../README.md).
