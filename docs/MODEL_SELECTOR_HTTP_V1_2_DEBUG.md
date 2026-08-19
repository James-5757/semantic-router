# Model Selector HTTP v1.2 联调

服务端遵循 `model_selector_api_v1.2.md`，默认保持 shadow-only 和 dry-run：不会调用上游模型，也不会替换旧 Scheduler 主结果。

## TokenCloud v1.3 账号快照兼容

`POST /v1/model-selector/select` 同时兼容 v1.3 新增的 `models` 与
`accounts`。这些字段随每次请求实时发送，因此选择器无需另建轮询或
缓存同步器。它们只影响 Shadow 建议，`semantic_router_takeover_enabled`
始终保持 `false`，不会调用上游模型，也绝不会覆盖 TokenCloud 原 Scheduler。

`accounts` 应只包含当前 API Key 所属分组的非敏感快照。选择器使用
`account_id`、`models[].model_id`、优先级、并发和实时负载建立候选关系；
`schedulable=false`、限流、过载、临时不可调度、配额耗尽、账号 ID 为 `0`
或不支持该模型的账号都会被排除。其余账号按负载、排队、错误率和优先级
进行加权，并使用 `rate_multiplier` 作为成本偏好、`ttft_ewma_ms` 作为首
token 延迟偏好。没有可用账号的模型会返回 `score=0`，不会被推荐。

当前运行分的权重为：40% 空闲度、15% 排队、15% 稳定性、10% 优先级、
10% 成本、10% TTFT。未提供 TTFT 的平台会对已知项重新归一化，不会被当作
慢账号处罚；负载与可用性仍然优先于成本和延迟。

不得传递 credential、API Key、Authorization、原始 provider error 等敏感
信息。未发送 v1.3 快照时保持旧 `profile_shadow_fallback`；发送快照后响应
`local_routing.source` 为 `profile_shadow_account_aware_v1.3`。

## 启动

```powershell
$env:SEMANTIC_ROUTER_HTTP_PORT="18080"
$env:MODEL_SELECTOR_SECRET="replace-with-shared-secret"
$env:SEMANTIC_ROUTER_TCP_ENABLED="false"
.\bin\semantic-router-server.exe
```

## 3.1 心跳

```powershell
$headers = @{
  "X-Request-ID" = "tc-heartbeat-001"
  "X-Selector-Secret" = "replace-with-shared-secret"
}
Invoke-RestMethod `
  -Uri "http://127.0.0.1:18080/v1/model-selector/heartbeat" `
  -Headers $headers
```

期望：HTTP 200，且 `success=true`、`data.status="healthy"`。

## 3.2 模型选择

TokenCloud 对原始 OpenAI/Anthropic JSON 做 gzip + Base64 后请求：

```text
POST /v1/model-selector/select
Content-Type: application/json
X-Request-ID: <unique request id>
X-Selector-Secret: <shared secret>
```

请求必须包含 `user_api_call` 和当前 API Key 分组允许的全部 `model_list`。响应会原样回传 `user_api_call`，并返回：

- `local_routing`：本地 Semantic Router 的 Pool、Tier、任务类型、置信度和来源。
- `semantics`：仅包含 Official vLLM 的七类旁路语义分数；不混入本地规则、Tier 或延迟。
- `model_score_list`：当前 API Key 允许模型的候选排序分数，统一四位小数。

所有字段均是 shadow-only；`upstream_called=false`，不会改变 TokenCloud 原调度结果。

当前本地调试若未接入真实 Account Repository，服务会使用 `profile_shadow_fallback` 为每个传入模型返回画像评分；不会选择 account，也不会调用上游。接入真实账号仓库后，同一 HTTP 契约会走账号候选排序。

## 3.3 模型分组同步

TokenCloud 在 API Key 分组变更时，可先同步该分组的完整模型快照：

```text
POST /v1/model-selector/sync-models
X-Selector-Secret: <shared secret>
```

```json
{
  "group_id": 2006,
  "platform": "domestic",
  "replace": true,
  "models": [
    {
      "model_id": "Qwen3.6-35B-A3B",
      "context_window": 131072,
      "max_output": 8192,
      "supports_streaming": true,
      "supports_tools": true,
      "supports_thinking": true,
      "pricing_input_per_1k": 0.0,
      "pricing_output_per_1k": 0.0
    }
  ]
}
```

`replace` 默认为 `true`：这是一份 group 快照，缺席模型立即不再属于该组。服务返回同步数量、目录版本和更新时间。

同步后，`/select` 可追加非敏感的 `group_id` 与 `api_key_id`：

```json
{
  "group_id": 2006,
  "api_key_id": 12345,
  "user_api_call": "...",
  "model_list": ["Qwen3.6-35B-A3B"]
}
```

当提供 `group_id` 时，Selector 要求该 group 已同步，且 `model_list` 中每个模型都属于该快照；跨组模型返回 HTTP 400，绝不混入候选。`api_key_id` 仅用于后续审计与映射，禁止传递原始 API Key。未传 `group_id` 的旧调用仍兼容，但没有该强约束。

## API Key 到 Group 映射

TokenCloud 可显式同步一个内部 API Key ID 的所属 group：

```text
POST /v1/model-selector/sync-api-key-group
X-Selector-Secret: <shared secret>
```

```json
{"api_key_id": 12345, "group_id": 2006}
```

该 group 必须已通过 `/sync-models` 同步。Selector 只保存内部 `api_key_id`，不接收或保存原始 API Key。

之后 `/select` 可只发送 `api_key_id`，Selector 会自动解析所属 group 并验证候选模型：

```json
{
  "api_key_id": 12345,
  "user_api_call": "...",
  "model_list": ["Qwen3.6-35B-A3B"]
}
```

首次同时携带有效 `api_key_id` 与已同步 `group_id` 的选择请求会自动建立绑定，便于渐进接入。已绑定 API Key 若携带不同的 `group_id`，服务返回 HTTP 409；TokenCloud 需要换组时必须显式调用同步接口更新绑定。

Ubuntu service 推荐增加：

```ini
MODEL_SELECTOR_MODEL_CATALOG_FILE=/home/sts/semantic-router/router_store/model_catalog.json
MODEL_SELECTOR_API_KEY_GROUP_FILE=/home/sts/semantic-router/router_store/api_key_groups.json
```

## 3.4 运行状态

```text
GET /v1/model-selector/status
X-Selector-Secret: <shared secret>
```

返回当前 Selector 进程的无敏感运行指标：

- `total_selections`、`successful_selections`、`error_selections`
- `avg_selection_latency_ms`、`p95_selection_latency_ms`（最近最多 1000 次请求）
- `official_vllm.attempt_count/success_count/fallback_count/fallback_rate`
- `recommended_models`（当前进程内被推荐为第一名的次数）
- `synced_group_count`、`loaded_models_count`、`api_key_group_binding_count`
- `shadow_only=true`、`takeover_enabled=false`

状态接口不包含 prompt、解压请求体、原始 API Key 或模型凭据。当前未启用选择结果缓存，因此 `cache_enabled=false`、`cache_hit_rate=0`。

可选并发归一化参数：

```ini
MODEL_SELECTOR_STATUS_MAX_CONCURRENT=32
```

## Official vLLM 辅助语义评分（Shadow）

在 Ubuntu 的 score-only vLLM 容器已经就绪后，可以启用下面的可选配置：

```ini
MODEL_SELECTOR_OFFICIAL_VLLM_ENABLED=true
MODEL_SELECTOR_OFFICIAL_VLLM_URL=http://127.0.0.1:8080
MODEL_SELECTOR_OFFICIAL_VLLM_TIMEOUT_MS=800
```

该调用只访问本机 `/api/v1/eval`，将七类 `semantic_*` 自然分数附加到
`semantics`，并以 25% 权重辅助 profile fallback 的候选排序。HTTP 超时、
服务错误或无有效信号时会忽略 Official 结果，返回原有画像评分，TokenCloud
主请求和旧 Scheduler 不受影响。
