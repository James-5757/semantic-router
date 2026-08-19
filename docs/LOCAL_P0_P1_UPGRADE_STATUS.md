# Local P0/P1 Upgrade Status

## P0 Completed

- Model profile v1 dimensions now include coding, data analysis, reasoning,
  Chinese-language fit, long-context fit, vision, document, and general fit.
- Every profile records evidence source, confidence, benchmark version, and
  evaluation date.
- Candidate Ranking V2 remains shadow-only and keeps the old Scheduler result.
- Playground exposes old/new selected account and model, agreement, and ranking
  margin.

## P1 In Progress

- Official vLLM natural scores are available locally through `/api/v1/eval`.
- Official and local pool decisions are already returned per request.
- The next local calibration run should aggregate those fields over replayed
  prompts and report pool agreement, top-score margin, fallback rate, and
  latency by pool.
- No takeover is enabled and no real upstream completion is requested.

## Remaining Preconditions

1. Replace catalog priors with dated benchmark or shadow-quality evidence before
   increasing profile confidence.
2. Connect the real Account Repository before production staging.
3. Review boundary disagreements with confirmed labels before enabling any
   takeover percentage.
