# Batch Replay 数据集提取

[English](BATCH_REPLAY_DATASET_EXTRACTION.md)

本工具从 Hugging Face 对话数据集中确定性提取用户 Prompt，供 Playground Batch Replay
使用。提取本身不调用 LLM、Router 或网络服务；仅在下载外部数据集时访问网络。

## 安装与提取

```bash
pip install datasets
python scripts/build_replay_dataset.py --max-rows 1000
```

PowerShell 示例：

```powershell
python scripts/build_replay_dataset.py `
  --dataset lmsys/lmsys-chat-1m `
  --max-rows 5000 `
  --languages en zh `
  --message-mode first_user `
  --seed 42
```

输出目录：

```text
datasets/generated/<run_id>/
  natural_replay.jsonl
  annotation_candidates.jsonl
  holdout_candidates.jsonl
  extraction_manifest.json
  extraction_errors.jsonl
```

`natural_replay` 用于回放；`annotation_candidates` 用于人工审核；`holdout_candidates`
必须从开发和调参中隔离。固定 `seed` 与 manifest 是可复现的前提。

## 导入 Playground

在 Dataset 标签上传 `natural_replay.jsonl`，确认 `prompt` 字段后运行 Batch Replay。
也可调用 `/v1/debug/events/import`。导入结果用于观测 Pool、Tier、Group agreement 和
候选模型变化，不得改变线上 Scheduler。

## 隐私与使用限制

- 先检查外部数据集许可和访问条件；部分数据集需要 Hugging Face 登录。
- 不上传生产 Prompt、账号、API Key、Authorization Header 或原始抓包。
- 提交到仓库前只保留脱敏、小规模、带来源和哈希的 fixture。
- 不要把 holdout 或真实流量数据用于规则调参后再当作独立评估。
