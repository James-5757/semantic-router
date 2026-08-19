# 测试与评估

[English](TESTING.md)

## 快速回归测试

Selector 的聚焦测试覆盖：HTTP 兼容、认证、历史脱敏、API Key group 隔离、
v1.3 账号可用性、动态负载、配额、成本/TTFT 以及 Official Pool 保守覆盖。

```bash
cd semantic_router
go test -run 'TestModelSelector|TestOfficialPoolShadow|TestSelectorDocumentRunbookGuard' -count=1 -v
```

## Shadow 与 Scheduler 安全测试

```bash
go test -run 'TestRealSchedulerDryRun|TestShadowMode|TestTokenCloudShadow' -count=1 -v
```

该组测试验证 disabled 账号和账号 ID 0 不会被选择，且 Shadow 失败不会替换旧
Scheduler 的主结果。

## 全量测试

```bash
go test ./...
```

仓库中的冻结 Pool holdout/regression fixture 目前会暴露一些既有基线失败。
不要为了让 CI 表面通过而重写或删除 fixture。Pool 校准应通过版本化 holdout
报告跟踪；Selector 运行时变更至少由上面的聚焦测试覆盖。

## 手工 Selector smoke test

1. 发送 gzip/Base64 的 `user_api_call` 与 group 范围的 `model_list`。
2. 确认 `model_score_list` 中每个输入模型恰好出现一次。
3. 确认 `shadow_only=true`，且未调用上游。
4. 将某个账号设为 `schedulable=false` 或配额耗尽；若没有其他可用映射账号，
   该模型必须为 0 分。
5. 提升某账号 `load_rate` 与 `waiting_count`；健康候选应提升排名，但网关
   主结果不能改变。

## 评估维度

| 范围 | 指标 |
| --- | --- |
| Pool | 总体准确率、每个 Pool precision/recall、混淆矩阵 |
| Tier | 准确率、strong recall、过度路由率 |
| 候选排序 | Top-1 稳定性、负载触发切换率、group 合规率 |
| 可靠性 | Selector error/fallback rate、P50/P95 延迟、账号 0 次数 |
| Shadow 一致性 | old-vs-new 模型一致率与分歧样本 |

## 新增回归样本

1. 只加入脱敏且可复现的输入。
2. 写清预期 Pool/Tier/model-group 行为，不要写死某个实现方式。
3. 在受影响边界周围添加最小化测试。
4. 运行聚焦测试，并单独记录 holdout 变化。
5. 不得为了接受新推荐而放松安全测试。
