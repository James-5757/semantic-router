# HTTP API and TokenCloud Integration

Base path: `/v1/model-selector`.

All protected calls use `X-Selector-Secret`; use `X-Request-ID` for tracing.
Every response has `success`, `code`, `message`, and `data`.

## Endpoints

| Method | Path | Use |
| --- | --- | --- |
| `GET` | `/heartbeat` | Liveness and weighted endpoint health checks. |
| `GET` | `/status` | Selector metrics and safe runtime status. |
| `POST` | `/select` | Shadow model recommendation. |
| `POST` | `/sync-models` | Persist an API-key group's model catalog. |
| `POST` | `/sync-api-key-group` | Persist an internal API-key-ID to group-ID binding. |
| `GET` | `/history` | Recent redacted shadow records. |

## v1.3 select request

`user_api_call` contains the original user request as gzip-compressed Base64.
The selector extracts the latest user message for routing and returns the same
encoded value unchanged.

```json
{
  "user_api_call": "<gzip-base64>",
  "api_key_id": 900001,
  "group_id": 2006,
  "model_list": ["Qwen3.5-397B-A17B", "Qwen3.6-35B-A3B"],
  "models": [
    {"model_id": "Qwen3.5-397B-A17B", "platform": "qwen", "upstream_model": "..."}
  ],
  "accounts": [
    {
      "account_id": 21148,
      "platform": "qwen",
      "priority": 60,
      "rate_multiplier": 1.0,
      "concurrency": 10,
      "load_factor": 10,
      "models": [{"model_id": "Qwen3.5-397B-A17B", "upstream_model": "..."}],
      "schedulable": true,
      "rate_limited": false,
      "overloaded": false,
      "temp_unschedulable": false,
      "quota_limit": 0,
      "quota_used": 0,
      "error_rate_ewma": 0.01,
      "ttft_ewma_ms": 650,
      "current_concurrency": 2,
      "waiting_count": 0,
      "load_rate": 20
    }
  ]
}
```

The response retains the original `user_api_call` and returns `semantics` plus
one four-decimal score for every member of `model_list`.

```json
{
  "success": true,
  "code": 200,
  "message": "ok",
  "data": {
    "user_api_call": "<unchanged gzip-base64>",
    "local_routing": {"preferred_pool": "data", "shadow_only": true},
    "semantics": [{"dimension": "official_vllm_semantic_data_analysis", "score": 0.8123}],
    "model_score_list": [{"model_id": "Qwen3.5-397B-A17B", "score": 0.8431}]
  }
}
```

## First integration: heartbeat, then Shadow selection

Run these commands locally on the Selector host. `MODEL_SELECTOR_SECRET` must
come from an environment file or secret manager; never place a real value in a
script or commit it to the repository.

```bash
curl -fsS -H "X-Selector-Secret: $MODEL_SELECTOR_SECRET" \
  http://127.0.0.1:18080/v1/model-selector/heartbeat
```

Prepare a redacted example containing only a user message and encode it as
`gzip + Base64`:

```bash
USER_API_CALL=$(printf '%s' '{"messages":[{"role":"user","content":"Implement a Go login API"}]}' \
  | gzip -c | base64 -w 0)

curl -fsS -X POST http://127.0.0.1:18080/v1/model-selector/select \
  -H 'Content-Type: application/json' \
  -H "X-Selector-Secret: $MODEL_SELECTOR_SECRET" \
  -d "{\"user_api_call\":\"$USER_API_CALL\",\"model_list\":[\"gpt-5.4\",\"gemini-2.5-flash\"]}"
```

For acceptance, confirm HTTP `200`, exactly one score per input model, scores
rounded to four decimals, `shadow_only=true`, and `upstream_called=false`. Add
same-group `models` and `accounts` in the real TokenCloud integration to enable
account availability filtering and dynamic runtime ranking.

## TokenCloud sender rules

1. Send only models and accounts allowed by the current API key's group.
2. Build `accounts` immediately before forwarding so load fields are current.
3. Do not send credentials, raw API keys, authorization headers, or raw
   provider-error bodies.
4. In bypass/shadow mode, do not wait for or apply selector output to the main
   request. Timeouts and errors must fail open to the original scheduler.
5. Skip forwarding for a single unique usable model mapping and for continuing
   conversations when cache preservation is required.

For legacy detail and examples, see [MODEL_SELECTOR_HTTP_V1_2_DEBUG.md](MODEL_SELECTOR_HTTP_V1_2_DEBUG.md).
