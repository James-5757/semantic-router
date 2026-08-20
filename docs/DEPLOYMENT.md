# Deployment Guide

This guide deploys the selector, optional Official vLLM score-only service, and
Playground on Ubuntu 22.04. It assumes an internal network and keeps every
component in shadow-only mode.

## Prerequisites

- Ubuntu 22.04, Go 1.21+, Docker Engine, Nginx (optional but recommended).
- A private network address or VPN. Do not expose the Playground publicly.
- A shared selector secret stored outside Git.

## Build

```bash
git clone <repository-url> semantic-router
cd semantic-router
go test -run 'TestModelSelector' -v
go build -o bin/semantic-router-server ./cmd/server
go build -o bin/router-playground ./cmd/playground
```

## Selector systemd service

Copy [semantic-router-selector.service](../deploy/ubuntu/semantic-router-selector.service)
to `/etc/systemd/system/semantic-router-selector.service`. Replace all example
paths and secrets. Keep the following safety values:

```ini
Environment=SEMANTIC_ROUTER_HTTP_PORT=18080
Environment=SEMANTIC_ROUTER_HTTP_LISTEN_ADDRESS=127.0.0.1:18080
Environment=SEMANTIC_ROUTER_DRY_RUN_ENABLED=true
Environment=SEMANTIC_ROUTER_TAKEOVER_ENABLED=false
Environment=MODEL_SELECTOR_SECRET=<stored-in-environment-file>
Environment=MODEL_SELECTOR_HISTORY_FILE=/var/lib/semantic-router/selector_history.jsonl
Environment=MODEL_SELECTOR_MODEL_CATALOG_FILE=/var/lib/semantic-router/model_catalog.json
Environment=MODEL_SELECTOR_API_KEY_GROUP_FILE=/var/lib/semantic-router/api_key_groups.json
```

Prefer `EnvironmentFile=/etc/semantic-router/selector.env` with mode `0600`
over putting a secret directly in the unit file.

```bash
sudo install -d -o "$USER" /var/lib/semantic-router
sudo systemctl daemon-reload
sudo systemctl enable --now semantic-router-selector
curl -fsS -H "X-Selector-Secret: $MODEL_SELECTOR_SECRET" \
  http://127.0.0.1:18080/v1/model-selector/heartbeat
```

## Optional Official vLLM score-only service

The selector runs without vLLM. Enable it only as a best-effort semantic signal:

```ini
Environment=MODEL_SELECTOR_OFFICIAL_VLLM_ENABLED=true
Environment=MODEL_SELECTOR_OFFICIAL_VLLM_URL=http://127.0.0.1:8080
Environment=MODEL_SELECTOR_OFFICIAL_VLLM_TIMEOUT_MS=800
```

Bind the container to loopback. The exact image/configuration depends on the
installed vLLM Semantic Router release; use the maintained internal Docker
configuration instead of downloading models implicitly in a production host.

```bash
curl -fsS http://127.0.0.1:8080/health
curl -fsS http://127.0.0.1:8080/ready
```

An unavailable vLLM service must only increment fallback metrics. It must not
make `/select` fail.

## Playground

Copy [playground.service](../deploy/ubuntu/playground.service), adjust its
paths, and start it on loopback:

```ini
Environment=PLAYGROUND_PORT=8081
Environment=PLAYGROUND_LISTEN_ADDRESS=127.0.0.1:8081
Environment=SEMANTIC_ROUTER_TAKEOVER_ENABLED=false
Environment=PLAYGROUND_SCHEDULER_MODE=platform
Environment=PLAYGROUND_MODEL_GROUP=<demo-group-name>
```

```bash
sudo systemctl enable --now playground
curl -fsS http://127.0.0.1:8081/health
```

## Nginx internal access

Expose only a private network route. Proxy `/v1/model-selector/` to the
Selector and `/` to Playground. Apply an IP allowlist and basic authentication
before giving access to users outside the trusted internal subnet.

```nginx
location /v1/model-selector/ {
  proxy_pass http://127.0.0.1:18080;
}

location / {
  proxy_pass http://127.0.0.1:8081;
}
```

## Upgrade and rollback

1. Build and run targeted tests before upload.
2. Upload the binary as `semantic-router-server.next`.
3. Keep the current binary as `semantic-router-server.bak`.
4. Atomically install the new binary and restart only the selector service.
5. Verify `/heartbeat`, `/status`, and one shadow `/select` request.
6. On failure, reinstall `.bak` and restart. Never enable takeover as part of
   an upgrade or rollback.

See [Operations](OPERATIONS.md) for live checks.
