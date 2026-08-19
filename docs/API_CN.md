# HTTP API 与 TokenCloud 集成

[English](API.md)

基础路径：`/v1/model-selector`。受保护接口使用 `X-Selector-Secret`；
`X-Request-ID` 用于链路追踪。所有响应都包含 `success`、`code`、`message`
和 `data`。

## 接口列表

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| `GET` | `/heartbeat` | 存活检查和多实例健康探测。 |
| `GET` | `/status` | 安全运行状态与指标。 |
| `POST` | `/select` | Shadow 模型候选推荐。 |
| `POST` | `/sync-models` | 同步 API Key group 模型目录。 |
| `POST` | `/sync-api-key-group` | 同步内部 API Key ID 到 group ID 映射。 |
| `GET` | `/history` | 最近脱敏 Shadow 历史。 |

## v1.3 `/select` 请求

`user_api_call` 是用户原始请求 JSON 的 gzip + Base64 编码。Selector 从中
抽取最新用户消息，并在响应中原样返回。

```json
{
  "user_api_call": "<gzip-base64>",
  "api_key_id": 900001,
  "group_id": 2006,
  "model_list": ["Qwen3.5-397B-A17B", "Qwen3.6-35B-A3B"],
  "models": [
    {"model_id": "Qwen3.5-397B-A17B", "platform": "qwen", "upstream_model": "..."}
  ],
  "accounts": [
    {
      "account_id": 21148,
      "platform": "qwen",
      "priority": 60,
      "rate_multiplier": 1.0,
      "concurrency": 10,
      "load_factor": 10,
      "models": [{"model_id": "Qwen3.5-397B-A17B", "upstream_model": "..."}],
      "schedulable": true,
      "rate_limited": false,
      "overloaded": false,
      "temp_unschedulable": false,
      "quota_limit": 0,
      "quota_used": 0,
      "error_rate_ewma": 0.01,
      "ttft_ewma_ms": 650,
      "current_concurrency": 2,
      "waiting_count": 0,
      "load_rate": 20
    }
  ]
}
```

响应保留原始 `user_api_call`，并返回 `semantics` 与 `model_score_list`。
每个 `model_list` 成员都有一个四位小数的分数。

## TokenCloud 发送端规则

1. 只发送当前 API Key group 允许的模型与账号。
2. 在转发前立即读取账号负载，以保证 `accounts` 是新鲜快照。
3. 禁止发送凭证、原始 API Key、Authorization Header 或原始上游错误体。
4. bypass/shadow 模式下，不得将 Selector 结果用于主请求；超时和错误必须
   自动回退到原 Scheduler。
5. 对只有一个唯一可用上游映射的 group，以及需保持缓存命中的连续会话，应
   跳过转发。

更多旧协议示例参见 [MODEL_SELECTOR_HTTP_V1_2_DEBUG.md](MODEL_SELECTOR_HTTP_V1_2_DEBUG.md)。
