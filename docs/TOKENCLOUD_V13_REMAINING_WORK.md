# TokenCloud v1.3 Remaining Work

Updated: 2026-08-19

## Current status

The Ubuntu selector is deployed as version `1.3.0`. It accepts v1.3
`models`/`accounts`, returns four-decimal `model_score_list`, and remains
shadow-only. Controlled replay verified that model ranking changes when account
load changes in both international and domestic groups.

Pool calibration is intentionally a separate workstream. It must not be mixed
with account-runtime ranking, otherwise a semantic classification issue can be
mistaken for a scheduling issue.

## v1.3 document checklist

| v1.3 item | Status | Owner / next action |
| --- | --- | --- |
| Heartbeat endpoint | Complete | TokenCloud already observes it as healthy. |
| `user_api_call` gzip/Base64 decode and original return | Complete | Selector preserves the encoded request. |
| `model_list` score for every allowed model | Complete | Scores are rounded to four decimals. |
| Request `models` mapping consumption | Complete | Selector consumes model ID/platform/upstream mapping. |
| Request `accounts` runtime snapshot consumption | Complete | Excludes unavailable accounts; load-aware model scoring. |
| Account zero protection | Complete | `account_id=0` is ignored. |
| `sync-models`, API-key-to-group boundary | Complete | Existing selector endpoints protect group isolation. |
| Status endpoint | Complete | Includes selection latency, Official fallback and recommendation counts. |
| TokenCloud v1.3 request builder | Pending | Add `models` and `accounts` to actual Shadow forwarding requests. |
| TokenCloud settings | Pending | Add enabled/bypass/endpoints/heartbeat/timeout/retry settings from v1.3. |
| Main request fail-open | Pending verification | Selector errors/timeouts must never block or alter the old scheduler in bypass mode. |
| Single-model-group skip | Pending | Do not forward when a group has one unique usable upstream mapping. |
| Conversation-cache skip | Pending | Respect `conversation_id` and `is_new_conversation=false` to protect cache hits. |
| Multiple selector endpoint balancing | Pending | Implement weighted round robin plus heartbeat-based failover when more than one selector is configured. |
| Real-traffic evidence | Pending | Capture redacted real v1.3 request/response pairs and compare against TokenCloud's original selection. |

## TokenCloud v1.3 forwarding payload

At the existing shadow forwarding point, build the request from the current API
key's group only:

1. `model_list`: deduplicated external model IDs allowed by the group.
2. `models`: `model_id`, platform, and `model_mapping` upstream value.
3. `accounts`: group accounts only; include scheduling, limit/quota and live
   concurrency fields. Do not include credentials, raw API keys, authorization
   headers or raw upstream errors.
4. Obtain current concurrency/wait/load through the existing batch Redis load
   reader immediately before forwarding.

The selector's response remains observational while bypass is enabled. TokenCloud
continues to invoke its old scheduler and can log the returned score list for
comparison.

## Separate P0: Pool calibration

Observed replay showed that the Official vLLM shadow score can classify a
document/general request as `image_generation` or `document` despite the local
task signal. This is a Pool decision issue, not an account-ranking issue.

1. Collect a stratified, redacted set from real Shadow history: code, data,
   document, general/cheap-chat, vision and image-generation.
2. Label only the ambiguous boundary samples, with a second-review pass for
   disagreements.
3. Evaluate local Pool, Official Pool and Hybrid Pool separately. Report
   per-pool precision/recall, disagreement rate and false technical routing.
4. Calibrate a conservative Official override threshold and require a meaningful
   margin over the second signal. Do not let a weak Official score override a
   high-confidence local professional signal.
5. Add regression tests from approved labels. Keep all Pool changes shadow-only
   until the new holdout report is reviewed.

## Acceptance gate for the next stage

- Three redacted real requests prove that TokenCloud sends non-empty `models`
  and `accounts` for the right API-key group.
- A deliberately disabled or rate-limited account changes its model score
  without changing the TokenCloud main result.
- Selector timeout/error leaves the main request successful.
- Pool calibration report is reviewed independently from dynamic scheduling.
