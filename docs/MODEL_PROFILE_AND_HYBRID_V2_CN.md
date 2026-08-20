# 模型画像与 Hybrid V2

[English](MODEL_PROFILE_AND_HYBRID_V2.md)

## 当前安全状态

- API Key 仍只能在配置的模型 group 内选择。
- Candidate Ranking V2、模型画像和 Hybrid 均为 Shadow/DryRun 观察结果。
- `final_tier` 仍由 RuleBasedTierRouter 决定。
- `takeover_enabled=false`，评估路径不调用真实上游。

## 四类分数不要混淆

```text
capability_score = 模型级画像先验或带版本的基准证据
semantic_score   = 当前 Prompt 的 Official 分类器分数
runtime_score    = 当前账号的负载、优先级、成本等实时信号
final_score      = 候选模型排序结果
```

模型画像包含 `profile_source`、`score_confidence`、`benchmark_version` 和
`evaluated_at`。`starter_registry`、`platform_catalog_prior` 目前是 E0 staging
先验，不等于生产 Benchmark 事实；上线前应逐步替换为有日期的外部基准、本地评估或
真实 Shadow 结果。

## Candidate Ranking V2

```text
final_score = 0.55 * capability_score
            + 0.20 * tier_fit_score
            + 0.25 * runtime_score
```

其中能力分负责区分“模型是否适合任务”，Tier 分保证强度要求，运行时分在可用候选中
处理负载、优先级和成本。旧 Scheduler 主结果保持不变，V2 只供观测。

## Hybrid Tier 的覆盖资格

只有同时满足以下条件，Hybrid 才会产生“可覆盖候选”：

1. RuleBased 与 Hybrid tier 不一致。
2. RouteLLM 服务可用。
3. learned strong/weak 概率达到配置阈值。
4. 不违反策略规定的最低 tier。

默认阈值：

```text
routellm_boundary_override_confidence_threshold=0.80
```

结果会记录 `hybrid_disagreement`、`override_eligible`、`override_reason` 和
`override_confidence_threshold`。即使满足资格，只要 takeover 关闭，`final_tier`
也不会改变。

## 验证原则

历史 Hybrid 验证使用的是 `truth_proxy`，不是人工确认标签。后续评估应在审核过的
边界样本上分别报告：分歧覆盖率、修正分歧数、新增分歧数、弃权率、strong recall 和
过度路由率。
