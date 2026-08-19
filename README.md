# Semantic Router

Semantic Router is a shadow-first model-selection service for LLM gateways. It
reads a user's request, identifies the task pool and required tier, ranks only
the models allowed by the current API-key group, and returns a scored candidate
list. It is designed to improve routing decisions without replacing an
existing gateway scheduler during evaluation.

> Current safety mode: **shadow-only**. The selector does not call an upstream
> model and does not replace the gateway's original scheduling result.

[中文文档](README_CN.md) | [Documentation index](docs/README.md)

## What it does

- Routes requests across code, data, document, vision, image generation, and
  chat/general pools.
- Uses local rules and optional Official vLLM semantic signals conservatively.
- Produces weak/medium/strong tier recommendations while retaining the existing
  rule-based tier path.
- Enforces API-key group boundaries: a candidate list cannot contain models
  outside the caller's authorized group.
- Consumes TokenCloud v1.3 account snapshots to exclude unavailable accounts
  and rank models by task fit, load, queueing, stability, cost, and TTFT.
- Provides a Playground, redacted request history, health/status endpoints, and
  shadow metrics for debugging.

## Architecture

```text
User request
  -> Prompt extraction
  -> Local Pool Router + Task Understanding
  -> Optional Official vLLM score-only signal (shadow)
  -> Rule-based Tier Router (+ optional learned tier shadow)
  -> API-key group allowlist
  -> Candidate ranking: task fit + live account runtime
  -> model_score_list (shadow recommendation)
  -> Existing gateway scheduler remains the main path
```

See [Architecture](docs/ARCHITECTURE.md) for component responsibilities and
scoring formulas.

## Quick start

Prerequisites: Go 1.21 or later. Docker is optional and is only needed for the
Official vLLM score-only service.

```bash
cd semantic_router
go test -run 'TestModelSelector' -v

# Start the selector on a local-only port.
export SEMANTIC_ROUTER_HTTP_PORT=18080
export SEMANTIC_ROUTER_HTTP_LISTEN_ADDRESS=127.0.0.1:18080
export SEMANTIC_ROUTER_TAKEOVER_ENABLED=false
export SEMANTIC_ROUTER_DRY_RUN_ENABLED=true
export MODEL_SELECTOR_SECRET='change-me'
go run ./cmd/server
```

In another terminal:

```bash
curl -H 'X-Selector-Secret: change-me' \
  http://127.0.0.1:18080/v1/model-selector/heartbeat
```

For the local visual debugger:

```bash
export PLAYGROUND_PORT=8081
export PLAYGROUND_LISTEN_ADDRESS=127.0.0.1:8081
export SEMANTIC_ROUTER_TAKEOVER_ENABLED=false
go run ./cmd/playground
```

Open `http://127.0.0.1:8081/debug/router-playground`.

## Documentation

- [Architecture and scoring](docs/ARCHITECTURE.md)
- [Deployment guide](docs/DEPLOYMENT.md)
- [Operations and safety](docs/OPERATIONS.md)
- [Testing and evaluation](docs/TESTING.md)
- [HTTP API and TokenCloud v1.3 integration](docs/API.md)
- [Remaining v1.3 work](docs/TOKENCLOUD_V13_REMAINING_WORK.md)

## Project status

The selector server and Playground are deployed internally on Ubuntu in
shadow-only mode. The server accepts v1.3 account snapshots and has controlled
replay coverage for dynamic load, disabled accounts, quota exhaustion, cost,
and TTFT. TokenCloud still needs to send `models` and `accounts` from its live
shadow forwarding path before real production account telemetry is available.

## Safety guarantees

- `SEMANTIC_ROUTER_TAKEOVER_ENABLED=false` is the expected deployment value.
- Selector errors and vLLM errors fall back without blocking the main request.
- `account_id=0`, disabled, limited, overloaded, temporarily unschedulable, or
  quota-exhausted accounts are never recommended.
- Do not commit API keys, account credentials, raw packet captures, or original
  user prompts. History and reports must be redacted before sharing.

## License

No open-source license has been selected yet. Do not reuse or redistribute the
code until the project owner adds a license to this repository.
