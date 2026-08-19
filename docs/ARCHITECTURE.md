# Architecture

## Design goals

Semantic Router is not an upstream model proxy. It is a decision service that
returns an observable recommendation to an existing gateway. The gateway owns
authentication, rate limiting, account reservation, and the final upstream
call. This separation makes incremental adoption and rollback safe.

## Request path

```text
TokenCloud request (gzip + Base64 original JSON)
  -> latest user prompt extraction
  -> Local Pool Router
  -> high-precision professional guards
  -> Optional Official vLLM semantic observation
  -> Rule-based tier decision
  -> group allowlist and account eligibility filtering
  -> candidate scoring and model_score_list
  -> TokenCloud original scheduler (unchanged in shadow mode)
```

## Pools and tiers

The Pool Router identifies the dominant capability: `code`, `data`,
`document`, `vision`, `image_generation`, `cheap_chat`, or `default/general`.
The Tier Router independently recommends `weak`, `medium`, or `strong`.

Pool and model ranking are separate decisions. A wrong Pool must be corrected
through Pool evaluation; it must not be hidden by dynamic account ranking.

## Official vLLM integration

Official vLLM is score-only. Its semantic categories are returned separately as
`semantics` and it never invokes a strong or weak upstream model.

To prevent weak semantic scores from destabilizing routing:

- A matching Official category may raise confidence.
- A local professional Pool with confidence of at least `0.50` is protected.
- Cross-Pool correction requires Official top score `>= 0.68` and top-two
  margin `>= 0.10`.
- Candidate ranking uses only the Official score for the final selected Pool,
  never an unrelated top category score.

## Candidate model scoring

For each model allowed by the API-key group, the selector first computes a
task-fit score from the active Pool, model profile, language, reasoning and the
matching semantic signal.

```text
final_model_score = 0.70 * static_task_fit + 0.30 * runtime_score
```

The selector filters accounts before runtime scoring. It rejects account ID 0,
unschedulable, rate-limited, overloaded, temporarily blocked and quota-exhausted
accounts, as well as accounts without a mapping for the candidate model.

```text
runtime_score =
  0.40 * free_capacity
+ 0.15 * queue_score
+ 0.15 * stability_score
+ 0.10 * priority_score
+ 0.10 * cost_score
+ 0.10 * ttft_score
```

`cost_score` comes from `rate_multiplier`; lower is better. `ttft_score` uses
`ttft_ewma_ms`, with 500 ms considered excellent and 3000 ms considered slow.
When a platform has no TTFT sample, known runtime dimensions are normalized so
missing telemetry is not treated as poor performance.

## Group boundary

An API key maps to one physical model group. `model_list`, `models`, and
`accounts` must describe that group only. The selector rejects models outside a
synchronized group and never mixes domestic and international groups for a
single request.

## Deployment components

| Component | Default binding | Responsibility |
| --- | --- | --- |
| Selector server | `127.0.0.1:18080` | Heartbeat, select, status, history, group sync. |
| Official vLLM | `127.0.0.1:8080` | Optional score-only semantic observations. |
| Playground | `127.0.0.1:8081` | Internal visual debugger and history viewer. |
| Nginx | private network entry | Restricts and proxies the internal UI/API. |

See [Deployment](DEPLOYMENT.md) and [Operations](OPERATIONS.md).
