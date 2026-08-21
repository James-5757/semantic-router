# Documentation Index

[中文](README_CN.md)

## Recommended reading order

| Situation | Read first | Then |
| --- | --- | --- |
| First local run | Root `README.md` | Start the Selector, verify `/heartbeat`, then start Playground. |
| Ubuntu private-network deployment | [Deployment](DEPLOYMENT.md) | Verify service, logs, and rollback with [Operations](OPERATIONS.md). |
| TokenCloud integration | [API](API.md) | Return candidate scores from Shadow `/select` without changing gateway scheduling. |
| Routing and ranking explanation | [Architecture](ARCHITECTURE.md) | Inspect Pool, Tier, candidates, and reasons in Playground. |
| Development or calibration | [Testing](TESTING.md) | Run focused checks and record holdout movement without deleting frozen fixtures. |

## Maintained documentation

- [Architecture](ARCHITECTURE.md): components, boundaries, and scoring.
- [Code map](CODEMAP.md): where to find routing, selection, Shadow, and integration code.
- [Deployment](DEPLOYMENT.md): Ubuntu, systemd, vLLM, and internal Nginx.
- [Operations](OPERATIONS.md): monitoring, safety, triage, and data handling.
- [Testing](TESTING.md): focused tests, manual smoke tests, and evaluation.
- [Reproducible results](EVALUATION_RESULTS.md): current public development baseline, command, and limits.
- [Public test data](../testdata/README.md): Playground import, provenance, hash, and use limits.
- [External Playground replay sets](PLAYGROUND_EXTERNAL_DATASETS.md): local-only third-party replay data and reporting boundaries.
- [API](API.md): Selector endpoints and TokenCloud v1.3 payload contract.
- [Remaining v1.3 work](TOKENCLOUD_V13_REMAINING_WORK.md): integration backlog.

## Repository guide

| Path | Purpose |
| --- | --- |
| Root `README.md` | Project entry point, local quick start, and safety boundary. |
| `docs/` | Maintained material for users, deployers, and integrators. |
| `docs/CODEMAP.md` | Source-level guide for contributors; use this before navigating the flat Go package. |
| `cmd/server/` | Selector HTTP service entry point. |
| `cmd/playground/` | Internal debugging UI and static assets. |
| `deploy/ubuntu/` | systemd, Nginx, and environment examples. |
| `docker/` | Optional Official vLLM score-only configuration. |
| Root `*_test.go` | Unit, boundary, and Shadow safety tests. |

Historical evaluation reports remain in this directory and at the module root.
They are useful evidence, but do not replace the maintained documents above.
Never put raw captures, secrets, account credentials, or unredacted prompts in
the repository.
