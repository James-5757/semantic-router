# External Playground Replay Sets

## Purpose

The development environment contains a larger local directory named
`router_test_datasets/`. These JSONL files can be imported into Playground for
batch replay: Playground reads the `prompt` field on each line, evaluates it,
and records the resulting routing trace.

They are useful for coverage, latency, failure inspection, and local-vs-Official
group agreement. They are **not** a labeled Pool/Tier benchmark and must not be
used to report routing accuracy.

## Local inventory

| File | Rows | Upstream source | Replay category |
| --- | ---: | --- | --- |
| `dolly_2000.jsonl` | 2,000 | `databricks/databricks-dolly-15k` | Instruction following |
| `codealpaca_2000.jsonl` | 2,000 | `HuggingFaceH4/CodeAlpaca_20K` | Code generation |
| `spider_2000.jsonl` | 2,000 | `xlangai/spider` | Text-to-SQL |
| `routellm_2000.jsonl` | 2,000 | `routellm/gpt4_judge_battles` | Model-comparison prompts |
| `diffusiondb_1000.jsonl` | 1,000 | `poloclub/diffusiondb` | Image-generation prompts |
| `mixed_router_test_5000.jsonl` | 5,000 | 1,000 sampled rows from each source | Mixed replay |

The five source-specific files contain 9,000 rows. The mixed file is a
separate, seeded 5,000-row replay sample, not an additional labeled dataset.

## Why they are not committed

- The records are third-party derivatives with different license and attribution
  requirements. For example, Dolly is CC BY-SA 3.0 and RouteLLM's published
  `gpt4_judge_battles` is Apache-2.0. Every source must be checked at its own
  upstream dataset page before redistribution.
- The data is not manually labeled with `expected_pool` or `expected_tier`.
  `source_category` describes its origin, not a correctness label for this
  Router.
- Some source prompts can include adult themes, named people, or other content
  unsuitable for a small public default fixture.
- `batch_results.json` is a local historical trace without a complete recorded
  service version and configuration. It is not a reproducible benchmark report.

The published [119-case routing fixture](../testdata/playground/routing_eval_v1.jsonl)
is the supported public Playground import and regression dataset.

## Local Playground usage

1. Start Playground in Shadow/DryRun mode.
2. Open its Dataset import view.
3. Select one local JSONL file.
4. Confirm the `prompt` field is used as the prompt column.
5. Export the replay output with its input SHA256, service version, configuration,
   date, and row count before quoting a group-agreement result.

No upstream model call is required for this workflow. Do not upload raw packet
captures, real production prompts, account snapshots, credentials, or an
unreviewed third-party replay file to this repository.
