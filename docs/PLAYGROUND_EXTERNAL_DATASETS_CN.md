# 外部 Playground 回放集

## 用途

开发环境中有一个较大的本地目录 `router_test_datasets/`。其中的 JSONL 文件可
导入 Playground 做批量回放：Playground 会读取每行的 `prompt` 字段，执行路由，
并记录完整 decision trace。

这些数据适合做覆盖率检查、延迟观察、失败样本分析，以及 local 与 Official 的
group 一致率观察。它们**不是**有 Pool/Tier 人工真值标签的 benchmark，不能用来
报告路由准确率。

## 本地数据清单

| 文件 | 条数 | 上游来源 | 回放类别 |
| --- | ---: | --- | --- |
| `dolly_2000.jsonl` | 2,000 | `databricks/databricks-dolly-15k` | 指令跟随 |
| `codealpaca_2000.jsonl` | 2,000 | `HuggingFaceH4/CodeAlpaca_20K` | 代码生成 |
| `spider_2000.jsonl` | 2,000 | `xlangai/spider` | Text-to-SQL |
| `routellm_2000.jsonl` | 2,000 | `routellm/gpt4_judge_battles` | 模型比较 Prompt |
| `diffusiondb_1000.jsonl` | 1,000 | `poloclub/diffusiondb` | 图像生成 Prompt |
| `mixed_router_test_5000.jsonl` | 5,000 | 每个来源抽取 1,000 条 | 混合回放 |

五个按来源拆分的文件一共 9,000 条。混合集是独立的、固定随机种子抽取的
5,000 条回放样本，不是额外带真值标签的数据集。

## 为什么不直接上传到 GitHub

- 它们是多种第三方数据的派生样本，许可证与署名要求并不相同。例如 Dolly 使用
  CC BY-SA 3.0，RouteLLM 发布的 `gpt4_judge_battles` 使用 Apache-2.0。再次分发
  前必须在各自上游数据集页面核对当前条款。
- 数据没有人工 `expected_pool`、`expected_tier` 标签。`source_category` 只是
  来源类别，不能等同于当前 Router 的正确答案。
- 部分来源 Prompt 可能涉及成人主题、真人姓名或不适合成为公开默认 fixture 的内容。
- `batch_results.json` 是本地历史 trace，没有完整服务版本和配置记录，不能作为
  可复现 benchmark 报告公开。

仓库中已公开的 [119 条路由 fixture](../testdata/playground/routing_eval_v1.jsonl)
才是推荐给使用者导入 Playground 和运行回归测试的数据集。

## 本地在 Playground 中使用

1. 以 Shadow/DryRun 模式启动 Playground。
2. 打开 Dataset 导入页面。
3. 选择任意本地 JSONL 文件。
4. 确认使用 `prompt` 作为 Prompt 列。
5. 在引用 group agreement 前，导出回放结果，并保留输入 SHA256、服务版本、配置、
   运行日期和行数。

这个流程不需要调用上游模型。不要把原始抓包、生产 Prompt、账号快照、凭证，或未
审查的第三方回放文件上传到本仓库。
