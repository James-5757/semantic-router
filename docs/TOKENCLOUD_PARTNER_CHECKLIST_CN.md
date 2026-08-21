# TokenCloud 对接方清单

本页是给 TokenCloud 或网关开发方使用的可执行对接清单。它只描述**Shadow-only**
路径：Selector 只推荐模型，不调用上游模型，也不会替换网关已有 Scheduler。

## 开始前确认

与 Selector 运维方确认：

- 已通过密钥管理工具获得内网 Selector 地址和 `X-Selector-Secret`；不要通过聊天、
  源码或浏览器 URL 传递密钥。
- `SEMANTIC_ROUTER_TAKEOVER_ENABLED=false`，且
  `SEMANTIC_ROUTER_DRY_RUN_ENABLED=true`。
- 调用方可以发送 TokenCloud 内部的数字 `api_key_id` 和物理 `group_id`；绝不能
  发送原始 API Key。
- 一次请求只能描述一个物理模型 group。`model_list`、`models`、`accounts` 中不能
  混入国内与国外候选。

## 第 1 步：检查可达性

```bash
curl -fsS -H "X-Selector-Secret: $MODEL_SELECTOR_SECRET" \
  http://127.0.0.1:18080/v1/model-selector/heartbeat
```

预期：HTTP `200`，`success=true`，且 `data.status="healthy"`。

## 第 2 步：同步 group 允许的模型目录

在创建 group 或变更该 group 的可用模型时执行。默认会用完整快照替换旧目录。

```bash
curl -fsS -X POST http://127.0.0.1:18080/v1/model-selector/sync-models \
  -H 'Content-Type: application/json' \
  -H "X-Selector-Secret: $MODEL_SELECTOR_SECRET" \
  -d '{
    "group_id": 2006,
    "platform": "openai",
    "models": [
      {"model_id": "gpt-5.4", "supports_tools": true, "context_window": 128000},
      {"model_id": "gemini-2.5-flash", "supports_tools": true, "context_window": 1000000}
    ]
  }'
```

预期：HTTP `200`，`success=true`，`data.received_count=2`。

## 第 3 步：绑定 API Key ID 与 group

当 API Key 对应的物理 group 已知或发生变化时执行：

```bash
curl -fsS -X POST http://127.0.0.1:18080/v1/model-selector/sync-api-key-group \
  -H 'Content-Type: application/json' \
  -H "X-Selector-Secret: $MODEL_SELECTOR_SECRET" \
  -d '{"api_key_id": 900001, "group_id": 2006}'
```

预期：HTTP `200`、`success=true`，并且返回的 ID 与请求一致。

## 第 4 步：转发一条 Shadow 模型选择请求

`user_api_call` 是原始请求 JSON 的 `gzip + Base64` 编码。每一条真实转发请求都
应发送 `models` 与最新 `accounts` 快照。以下示例不包含任何凭证。

```json
{
  "user_api_call": "<gzip-base64>",
  "api_key_id": 900001,
  "model_list": ["gpt-5.4", "gemini-2.5-flash"],
  "models": [
    {"model_id": "gpt-5.4", "platform": "openai", "upstream_model": "gpt-5.4"},
    {"model_id": "gemini-2.5-flash", "platform": "google", "upstream_model": "gemini-2.5-flash"}
  ],
  "accounts": [
    {
      "account_id": 21046,
      "platform": "openai",
      "priority": 60,
      "concurrency": 10,
      "load_factor": 10,
      "models": [{"model_id": "gpt-5.4", "upstream_model": "gpt-5.4"}],
      "schedulable": true,
      "rate_limited": false,
      "overloaded": false,
      "temp_unschedulable": false,
      "current_concurrency": 2,
      "waiting_count": 0,
      "load_rate": 20,
      "ttft_ewma_ms": 650
    }
  ]
}
```

响应重点：

- `data.local_routing`：推荐 Pool、Tier、置信度与本地来源。
- `data.semantics`：可选 Official vLLM 观察分数，不是最终路由决策。
- `data.model_score_list`：`model_list` 中每个模型都有四位小数分数，并按降序排列；
  第一项是建议模型。

网关侧只记录该结果，真实请求仍由旧 Scheduler 选择模型与账号。

## 必须具备的失败行为

| 场景 | Selector 结果 | TokenCloud 行为 |
| --- | --- | --- |
| Selector 超时、`5xx` 或网络异常 | 无可用建议 | 继续走原 Scheduler。 |
| `401` | Secret 缺失或不正确 | 修复密钥分发；不得改用原始 API Key 重试。 |
| `400` | 请求无效、缺 Prompt/模型列表、group 未同步或模型越组 | 修正转发 payload，主请求仍走原 Scheduler。 |
| `409` | `api_key_id` 与已保存 group 冲突 | 停止该 Key 转发，核对 group 后重新同步。 |
| Official vLLM 不可用 | 仍返回本地评分并记录 fallback | 不阻塞或重试主请求。 |

## 联调验收清单

- 转发机可以得到健康心跳。
- group 模型目录已同步，且 API Key 到 group 的绑定存在。
- 一条脱敏请求返回 HTTP `200`，`model_list` 中每个候选都有四位小数分数。
- disabled、限流、过载、不可调度、配额耗尽以及 `account_id=0` 的账号不会被推荐。
- 网关记录 `model_score_list`，但真实上游仍使用旧 Scheduler 结果。
- 超时测试证明 Selector 失败不会让主请求失败。

全部字段定义见 [API_CN.md](API_CN.md)；部署、监控和故障处理见
[OPERATIONS_CN.md](OPERATIONS_CN.md)。
