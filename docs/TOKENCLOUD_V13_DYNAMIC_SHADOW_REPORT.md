# TokenCloud v1.3 Dynamic Shadow Smoke Report

Date: 2026-08-19

## Scope

The selector service on Ubuntu accepts TokenCloud v1.3 `models` and `accounts`
in `POST /v1/model-selector/select`. The deployment remains shadow-only:

- `semantic_router_takeover_enabled=false`
- no upstream model call
- TokenCloud's original scheduler remains the main result

The current TokenCloud source checkout contains the account repository,
model-mapping and Redis load primitives, but does not yet contain the v1.3
selector-forwarding call site. Therefore the account snapshots in this report
are controlled runtime simulations, not production traffic.

## Dynamic replay

Each replay used the same three models and live-account-shaped snapshots:
`gpt-5.4`, `gemini-2.5-flash`, and `deepseek-v4-flash`.

| Prompt type | Balanced runtime winner | GPT saturated runtime winner | Observed effect |
| --- | --- | --- | --- |
| Code | gpt-5.4, 0.8412 | gemini-2.5-flash, 0.7625 | GPT score 0.8412 -> 0.6687 |
| Data | gpt-5.4, 0.8414 | gemini-2.5-flash, 0.7701 | GPT score 0.8414 -> 0.6689 |
| Document | gpt-5.4, 0.7829 | gemini-2.5-flash, 0.7829 | GPT score 0.7829 -> 0.6104 |
| General | gpt-5.4, 0.7612 | gemini-2.5-flash, 0.7612 | GPT score 0.7612 -> 0.5887 |

Balanced scenario: GPT load 10%, Gemini 35%, DeepSeek 50%.

Saturated scenario: GPT load 100% with four queued requests, Gemini 5%,
DeepSeek 20%.

The selector excluded account ID `0` and an explicitly non-schedulable account
in the same smoke request. This confirms that the score change is caused by
the v1.3 account snapshot rather than a change to the prompt.

## Pool calibration boundary

The dynamic ranking result is valid even when a Pool classification is not. In
one document/general replay, the Official vLLM shadow score selected a less
appropriate Pool. This is tracked separately as Pool calibration work; it does
not alter the account-aware model-ranking result in this report.

## TokenCloud forwarding acceptance checklist

For each shadow-forwarded request, TokenCloud should include:

1. `model_list`: the de-duplicated models allowed by the current API-key group.
2. `models`: every model's `model_id`, `platform`, and `upstream_model`.
3. `accounts`: only the current group accounts, with model mappings,
   schedulability, rate-limit/overload/quota state, and runtime load fields.
4. `X-Request-ID` and `X-Selector-Secret`.

It must not include account credentials, API keys, authorization headers, or
unredacted provider error bodies. A selector failure or timeout must be ignored
by the main request in shadow mode.

## Production shadow verification

After the TokenCloud forwarding implementation is deployed, validate at least:

- A normal request contains non-empty `models` and `accounts` in selector
  history or a redacted trace.
- A disabled, rate-limited, overloaded, or quota-exhausted account's model
  receives score `0` when no alternative account supports it.
- Raising one account's live load changes only the candidate scores; the
  TokenCloud upstream result remains unchanged while bypass/shadow is enabled.
- Different API keys never send accounts or models outside their own group.
