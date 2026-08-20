# TokenCloud v1.3 剩余工作

[English](TOKENCLOUD_V13_REMAINING_WORK.md)

更新日期：2026-08-19。

## 已完成

- Ubuntu Selector 以 v1.3 Shadow-only 部署。
- 接收 `models`、`accounts`，返回四位小数 `model_score_list`。
- `account_id=0` 保护、不可用账号过滤、负载变化的受控回放。
- `sync-models`、API Key 到 group 的隔离及 `/status` 指标。

## TokenCloud 仍需完成

1. 在真实 Shadow 转发点填入非空的 `models` 与 `accounts`。
2. 在设置侧支持 enabled/bypass/endpoint/heartbeat/timeout/retry。
3. 验证 Selector 超时或错误时旧 Scheduler 仍成功返回。
4. 单一可用模型 group 跳过转发；连续会话考虑 cache 命中保护。
5. 多实例时实现加权轮询与心跳 failover。
6. 收集脱敏真实 v1.3 请求/响应对，与 TokenCloud 原选择结果对比。

## 发送字段规则

每次 Shadow 转发只发送当前 API Key 所属 group：

- `model_list`：group 允许的去重外部模型 ID。
- `models`：`model_id`、platform 与上游映射。
- `accounts`：可调度状态、限流/配额和最新并发/等待/负载字段。

不得发送凭证、原始 API Key、Authorization Header、原始上游错误体。Selector 响应在
bypass 模式只能记录或展示，TokenCloud 仍调用旧 Scheduler。

## Pool 校准应独立进行

Pool 语义问题不能和账号排序混在一起。下一轮应收集分层、脱敏的真实 Shadow 样本，
审核边界案例，分别报告 Local/Official/Hybrid 的 per-pool 指标和分歧率，并只允许高
置信度、有明显 margin 的 Official 信号提出 Shadow 修正建议。

## 下一阶段验收

- 三条脱敏真实请求证明正确 group 的 `models`、`accounts` 非空。
- 禁用或限流账号会改变建议分数，但不改变 TokenCloud 主结果。
- Selector timeout/error 不影响主请求。
- Pool 校准报告独立于动态调度评审。
