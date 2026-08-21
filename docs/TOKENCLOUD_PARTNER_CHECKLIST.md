# TokenCloud Partner Integration Checklist

This page is the implementation checklist for the TokenCloud or gateway team.
It describes the supported **Shadow-only** path. The Selector recommends a
model; it never calls an upstream model and never replaces the gateway's
existing Scheduler.

## Before you start

Confirm these facts with the Selector operator:

- You have the internal Selector base URL and `X-Selector-Secret` through a
  secret manager, not chat, source code, or a browser URL.
- `SEMANTIC_ROUTER_TAKEOVER_ENABLED=false` and
  `SEMANTIC_ROUTER_DRY_RUN_ENABLED=true`.
- Your caller can send TokenCloud's internal numeric `api_key_id` and the
  physical `group_id`. Never send the raw API key.
- A single request describes one physical group only. Do not mix domestic and
  international candidates in `model_list`, `models`, or `accounts`.

## Step 1: check reachability

```bash
curl -fsS -H "X-Selector-Secret: $MODEL_SELECTOR_SECRET" \
  http://127.0.0.1:18080/v1/model-selector/heartbeat
```

Expected: HTTP `200`, `success=true`, and `data.status="healthy"`.

## Step 2: synchronize the group's allowed model catalog

Run this when the group is created or its permitted model list changes. A full
snapshot replaces the previous catalog by default.

```bash
curl -fsS -X POST http://127.0.0.1:18080/v1/model-selector/sync-models \
  -H 'Content-Type: application/json' \
  -H "X-Selector-Secret: $MODEL_SELECTOR_SECRET" \
  -d '{
    "group_id": 2006,
    "platform": "openai",
    "models": [
      {"model_id": "gpt-5.4", "supports_tools": true, "context_window": 128000},
      {"model_id": "gemini-2.5-flash", "supports_tools": true, "context_window": 1000000}
    ]
  }'
```

Expected: HTTP `200`, `success=true`, `data.received_count=2`.

## Step 3: bind the API key ID to its group

Do this when the API key's physical group is known or changes.

```bash
curl -fsS -X POST http://127.0.0.1:18080/v1/model-selector/sync-api-key-group \
  -H 'Content-Type: application/json' \
  -H "X-Selector-Secret: $MODEL_SELECTOR_SECRET" \
  -d '{"api_key_id": 900001, "group_id": 2006}'
```

Expected: HTTP `200`, `success=true`, and the returned IDs match the request.

## Step 4: forward one Shadow selection request

`user_api_call` is the original request JSON encoded as `gzip + Base64`. Send
the `models` and current `accounts` snapshot on every real forwarding request.
The examples below contain no credentials.

```json
{
  "user_api_call": "<gzip-base64>",
  "api_key_id": 900001,
  "model_list": ["gpt-5.4", "gemini-2.5-flash"],
  "models": [
    {"model_id": "gpt-5.4", "platform": "openai", "upstream_model": "gpt-5.4"},
    {"model_id": "gemini-2.5-flash", "platform": "google", "upstream_model": "gemini-2.5-flash"}
  ],
  "accounts": [
    {
      "account_id": 21046,
      "platform": "openai",
      "priority": 60,
      "concurrency": 10,
      "load_factor": 10,
      "models": [{"model_id": "gpt-5.4", "upstream_model": "gpt-5.4"}],
      "schedulable": true,
      "rate_limited": false,
      "overloaded": false,
      "temp_unschedulable": false,
      "current_concurrency": 2,
      "waiting_count": 0,
      "load_rate": 20,
      "ttft_ewma_ms": 650
    }
  ]
}
```

The response contains:

- `data.local_routing`: preferred Pool, Tier, confidence, and local source.
- `data.semantics`: optional Official vLLM observations. These are not the
  final routing decision.
- `data.model_score_list`: every `model_list` member, sorted descending and
  rounded to four decimals. The first item is the suggested model.

On the gateway side, log this result only. Keep the existing Scheduler's model
and account as the actual request result.

## Required failure behavior

| Situation | Selector result | TokenCloud action |
| --- | --- | --- |
| Selector timeout, `5xx`, or network error | No usable suggestion | Continue the original Scheduler path. |
| `401` | Selector secret missing or invalid | Fix secret distribution; do not retry with a raw API key. |
| `400` | Invalid body, missing prompt/model list, unsynchronized group, or group model mismatch | Correct the forwarding payload; keep the original Scheduler path. |
| `409` | `api_key_id` conflicts with its stored group | Stop forwarding that key, verify the group mapping, then resync. |
| Official vLLM unavailable | Local scores still return; fallback is recorded | Do not block or retry the main request. |

## Sign-off checklist

- Heartbeat is healthy from the forwarding host.
- A group catalog is synchronized and API-key group binding is present.
- One redacted request returns HTTP `200` and a four-decimal score for every
  candidate sent in `model_list`.
- Disabled, rate-limited, overloaded, unschedulable, quota-exhausted, and
  `account_id=0` accounts are not recommended.
- The gateway logs `model_score_list`, but real upstream selection remains the
  old Scheduler result.
- A timeout test proves that Selector failure cannot fail the main request.

Use [API.md](API.md) for all field definitions and [OPERATIONS.md](OPERATIONS.md)
for deployment, monitoring, and incident handling.
