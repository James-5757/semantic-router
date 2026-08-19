# 运行与安全手册

[English](OPERATIONS.md)

## 安全基线

| 配置 | 必须值 |
| --- | --- |
| `SEMANTIC_ROUTER_TAKEOVER_ENABLED` | `false` |
| `SEMANTIC_ROUTER_DRY_RUN_ENABLED` | `true` |
| Selector 绑定地址 | `127.0.0.1` |
| vLLM 绑定地址 | `127.0.0.1` |
| Playground 暴露范围 | 仅私网 |

Shadow 模式下，旧网关 Scheduler 是真实账号与模型的唯一主决策者。Selector
不得直接发起上游完成请求。

## 健康检查

```bash
curl -fsS -H "X-Selector-Secret: $MODEL_SELECTOR_SECRET" \
  http://127.0.0.1:18080/v1/model-selector/heartbeat

curl -fsS -H "X-Selector-Secret: $MODEL_SELECTOR_SECRET" \
  http://127.0.0.1:18080/v1/model-selector/status

systemctl status semantic-router-selector --no-pager
journalctl -u semantic-router-selector -n 100 --no-pager
```

重点关注：选择总量/成功/失败、平均与 P95 延迟、Official vLLM 成功与 fallback
率、推荐模型计数、同步 group/model 数量、API Key 映射数量，以及
`shadow_only=true`、`takeover_enabled=false`。

## Shadow 排障

| 现象 | 预期行为 | 处理 |
| --- | --- | --- |
| Official vLLM 超时 | 本地 profile fallback 仍返回分数。 | 检查 vLLM，不阻塞网关流量。 |
| Selector 超时 | 网关主请求仍由旧 Scheduler 成功处理。 | 检查日志和超时，保持 bypass。 |
| 某模型为 0 分 | 无可用映射账号或账号不可用。 | 检查 group、限流、负载、配额。 |
| 出现跨组模型 | Selector 拒绝请求。 | 修正 TokenCloud 的 API Key group 载荷。 |
| Pool 分歧 | 记录为校准样本。 | 不要用账号权重掩盖语义问题。 |

## 数据处理

- 历史只保存脱敏元数据，不向外部用户开启完整 Prompt 保存。
- 持久化目录必须由服务账号持有，并设置受限权限。
- v1.3 `accounts` 禁止携带凭证、原始 API Key 和原始上游错误。
- 抓包、用户请求和 GitHub Issue 里的样本必须先脱敏。

## 事故处理

1. 首先确认 `takeover_enabled=false`。
2. 确认网关原 Scheduler 的主结果仍正常。
3. 若 Selector 产生运行噪音，可在 TokenCloud 移除该 endpoint 或关闭转发；
   Shadow-only 下主链路不受影响。
4. 仅保留脱敏的 request ID、状态和指标用于分析。
5. 若问题在 Selector 本身，回滚二进制。

## Pool 校准流程

Pool 质量与模型排序独立评估：从真实 Shadow 历史中按 code、data、document、
general、vision、image 分层采样并脱敏；标注边界样本；比较 Local、Official、
Hybrid 的各 Pool precision/recall 与分歧率；审核 holdout 影响后再调整规则。
