# Operations and Safety Runbook

## Safety baseline

| Setting | Required value |
| --- | --- |
| `SEMANTIC_ROUTER_TAKEOVER_ENABLED` | `false` |
| `SEMANTIC_ROUTER_DRY_RUN_ENABLED` | `true` |
| Selector bind address | `127.0.0.1` |
| vLLM bind address | `127.0.0.1` |
| Playground exposure | private network only |

The selector must never issue an upstream completion request. In shadow mode,
the old gateway scheduler is the sole authority for the real account/model.

## Health checks

```bash
curl -fsS -H "X-Selector-Secret: $MODEL_SELECTOR_SECRET" \
  http://127.0.0.1:18080/v1/model-selector/heartbeat

curl -fsS -H "X-Selector-Secret: $MODEL_SELECTOR_SECRET" \
  http://127.0.0.1:18080/v1/model-selector/status

systemctl status semantic-router-selector --no-pager
journalctl -u semantic-router-selector -n 100 --no-pager
```

Watch these status fields:

- `total_selections`, `successful_selections`, `error_selections`
- average/P95 selection latency
- Official vLLM attempt, success, and fallback rates
- `recommended_models`
- synchronized group/model counts and API-key group binding count
- `shadow_only=true` and `takeover_enabled=false`

## Shadow triage

| Symptom | Expected behavior | Action |
| --- | --- | --- |
| Official vLLM timeout | Local profile fallback still returns scores. | Inspect vLLM health; do not block gateway traffic. |
| Selector timeout | Gateway main request succeeds using old scheduler. | Reduce timeout, inspect service logs, keep bypass enabled. |
| Account model gets score 0 | It is unavailable, unmapped, or has no eligible account. | Inspect group mapping, schedulable/limit/quota status. |
| Unexpected cross-group model | Selector rejects the request. | Correct TokenCloud API-key group payload. |
| Pool disagreement | Record as a calibration sample. | Do not tune account weights to mask it. |

## Data handling

- Selector history stores redacted request metadata; do not enable full prompt
  storage for external users.
- Keep persistent files under a service-owned directory with restrictive modes.
- Never include credentials or raw authorization fields in v1.3 `accounts`.
- Redact packet captures and user requests before placing them in test data or
  GitHub issues.

## Incident response

1. Confirm `takeover_enabled=false`.
2. Confirm the gateway's original scheduler is returning successful results.
3. Disable selector forwarding or remove the endpoint from TokenCloud if its
   errors cause operational noise; this is safe because it is shadow-only.
4. Preserve only redacted request IDs, status samples, and metrics for analysis.
5. Roll back the selector binary if the issue is local to the service.

## Pool calibration workflow

Pool quality is measured separately from model ranking. Collect redacted,
stratified samples across code, data, document, general, vision and image tasks;
label ambiguous cases; then compare Local, Official and Hybrid decisions by
per-pool precision/recall and disagreement rate. Update only after a review of
the holdout impact.
