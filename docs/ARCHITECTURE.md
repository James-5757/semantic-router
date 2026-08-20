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

For each model allowed by the API-key group, the selector computes
`static_task_fit` before it incorporates live account state. **Static** does
not mean immutable: it is recalculated for each request from the selected Pool,
language and reasoning signals. It only means this component does not use live
load, queue or TTFT telemetry. It is not a measured model success rate.

```text
final_model_score = 0.70 * static_task_fit + 0.30 * runtime_score
```

### What `static_task_fit` means

All values are normalized to `0..1` and come from three sources:

1. **Pool base score (`pool_score`)** from the model profile: code uses
   `coding_agent_score`, data uses `data_analysis_score`, document uses
   `document_score`, vision/image generation uses `vision_score`, and general
   tasks use `general_score`.
2. **Prompt-specific task fit (`task_fit_score`)**: Chinese prompts blend in
   `chinese_score`; reasoning/analysis prompts blend in `reasoning_score`.
   The internal Scheduler path can additionally map task signals to code, data,
   document and long-context profile fields.
3. **Matching Official vLLM score (optional, Shadow)**: only the score for the
   final Pool is eligible. A data task can use the data-analysis score, never an
   unrelated top category score.

The HTTP selector follows this calculation:

```text
task_fit = pool_score
if Chinese prompt:          task_fit = 0.75 * task_fit + 0.25 * chinese_score
if reasoning/analysis cue:  task_fit = 0.75 * task_fit + 0.25 * reasoning_score

profile_task_fit = 0.75 * pool_score + 0.25 * task_fit
static_task_fit = profile_task_fit
if matching Official score exists:
  static_task_fit = 0.75 * profile_task_fit + 0.25 * official_pool_score
```

For example, a Chinese code request with `coding_agent_score=0.88` and
`chinese_score=0.78` produces `task_fit=0.855` and
`profile_task_fit=0.8738`. With an Official code score of `0.68`, the resulting
`static_task_fit` is `0.8253`.

Profile values are versioned routing priors, auditable through `profile_source`,
`evidence_source`, `benchmark_version`, `evaluated_at`, and `score_confidence`.
They are not copied from an external leaderboard or inferred from the current
request. The current `platform_catalog_prior` has deliberately low confidence
and should be replaced progressively with dated benchmark evidence, Shadow
outcomes, or reviewed calibration data.

Native image generation also has a hard compatibility gate: only models placed
in `image_generation_pool` can receive a nonzero candidate score. Incompatible
models remain visible for deterministic reconciliation with TokenCloud, with a
score of `0` and reason `incompatible_with_selected_pool`.

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

In short, `static_task_fit` answers whether a model is suited to this request;
`runtime_score` answers whether its account is suitable to serve it now.

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
