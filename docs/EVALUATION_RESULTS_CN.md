# 可复现评估结果

本页只记录可由仓库内公开数据和命令复现的结果，不展示无法追溯来源的高分，也不把
开发集结果表述为生产效果。

## Routing Eval V1

| 项目 | 值 |
| --- | --- |
| 数据集 | `testdata/playground/routing_eval_v1.jsonl` |
| 数据来源 | 项目维护的人工标注开发路由 fixture |
| 样本数 | 119 |
| 执行命令 | `go test -run '^TestRoutingEvalCases$' -count=1 -v` |
| Pool 准确率 | 74.79% (89/119) |
| Tier 准确率 | 77.31% (92/119) |
| Fallback 匹配率 | 99.16% (118/119) |
| 执行日期 | 2026-08-20 |
| 运行模式 | 本地离线路由评估，不调用上游模型 |

## 如何解读

- 这是开发集的当前基线，用于发现回归，不能用于宣传最终泛化能力。
- `Pool` 是任务能力类别；`Tier` 是模型强度建议，两者应分别看待。
- 119 条数据可直接导入 Playground 做批量回放；Playground 展示的 local-vs-Official
  group agreement 是两个分类器的一致率，不是上述人工标签准确率。
- 独立 Holdout V2 有 280 条、每个 Pool 40 条，已冻结且不公开。它应只用于最终
  评估，不能拿来调整规则、阈值或展示示例。

## Playground 历史 Group Agreement 观察值

| 项目 | 记录 |
| --- | --- |
| 输入规模 | 项目使用者报告为 2,000+ 条本地导入 Prompt |
| 指标 | local group 与 Official group 相同的比例 |
| 观察值 | 约 81% |
| 数据来源 | 当时的本地 `router test` / Playground 导入数据；当前仓库未找到可发布导出文件 |
| 状态 | 历史观察，**不可复现，不是正式 benchmark** |

该指标回答的是两个分类器是否把同一 Prompt 放入同一粗粒度模型 group（例如
technical 或 general），不是人工标签准确率，也不能与上面的 Pool accuracy 直接比较。
在数据文件、导出结果、服务配置和运行日期补齐前，它只用于说明曾经观察到的系统行为，
不应用作效果宣传或阈值校准依据。

要将它升级为正式结果：在 Playground 导出 batch JSON，保存输入 JSONL 的 SHA256、
Official 服务版本、运行日期和总行数，并将脱敏数据登记到 `testdata/manifest.json`。

## 后续报告规范

任何新增结果必须同时写明：数据集 ID、样本数、数据来源、标签状态、代码版本/提交、
完整命令、运行日期、是否使用 Official/vLLM，以及是否发生上游调用。
