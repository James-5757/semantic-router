# Ubuntu Internal Demo Deployment

This directory deploys the Playground as an internal-only demo and keeps the
selector and vLLM score service on loopback addresses. It does not enable
takeover or invoke upstream models.

## Services

- `semantic-router-selector.service`: `127.0.0.1:18080`
- `vllm-sr-score-only` Docker container: `127.0.0.1:8080`
- `playground.service`: `127.0.0.1:8081`
- Nginx demo entry: `http://<internal-host>:8088/debug/router-playground`

Nginx should allow only the intended private subnet, for example
`10.0.0.0/8`. Enable HTTP basic authentication or an equivalent internal SSO
control before any broader demo.
The `/v1/model-selector/` path proxies to the local Selector on port `18080`;
all other paths proxy to Playground. Replace the network range if reviewers
are on a different private subnet.

The deployed Playground is pinned to `国外OPENAI分组`. Its candidate ranking
therefore never mixes models from another physical API-key group, including
the domestic `超讯科技` group. Change `PLAYGROUND_MODEL_GROUP` only when
demonstrating that specific API-key group.

## Selector vLLM shadow configuration

Create `/etc/semantic-router/selector.env` from `selector.env.example`, set
permissions to `0600`, and add these environment settings to
`semantic-router-selector.service`:

```ini
Environment=MODEL_SELECTOR_OFFICIAL_VLLM_ENABLED=true
Environment=MODEL_SELECTOR_OFFICIAL_VLLM_URL=http://127.0.0.1:8080
Environment=MODEL_SELECTOR_OFFICIAL_VLLM_TIMEOUT_MS=800
Environment=MODEL_SELECTOR_HISTORY_FILE=/home/sts/semantic-router/router_store/selector_history.jsonl
Environment=SEMANTIC_ROUTER_TAKEOVER_ENABLED=false
Environment=SEMANTIC_ROUTER_DRY_RUN_ENABLED=true
```

The unit references the secret file through:

```ini
EnvironmentFile=-/etc/semantic-router/selector.env
```

The Official vLLM call is best-effort: error or timeout returns the existing
profile-only result, without affecting TokenCloud's main request.

`MODEL_SELECTOR_HISTORY_FILE` persists the last 200 redacted selector calls
for the internal Playground History view. It never changes the TokenCloud
response or calls an upstream model.
