# Semantic Router

面向 LLM 网关的 Shadow-first 智能模型选择服务。它读取用户请求，识别任务
Pool 和 Tier，只在当前 API Key 所属模型组中对候选模型打分并给出推荐。系统
用于逐步改善路由决策，在评估阶段不会替换既有网关调度器。

> 当前安全模式：**仅 Shadow**。选择器不调用上游模型，也不覆盖网关原有的
> 调度结果。

[English](README.md) | [中文文档索引](docs/README_CN.md)

## 能做什么

- 识别代码、数据、文档、视觉理解、图像生成、轻量聊天和通用问答任务。
- 将本地规则与可选 Official vLLM 语义分数保守结合。
- 给出 `weak`、`medium`、`strong` 的 Tier 建议，同时保留现有规则 Tier 路径。
- 强制 API Key 模型组边界，不会跨组混合国内和国外候选模型。
- 接收 TokenCloud v1.3 账号快照，依据任务适配、负载、排队、稳定性、成本和
  TTFT 对模型排序。
- 提供 Playground、脱敏历史、健康检查、状态指标和 Shadow 调试能力。

## 架构概览

```text
用户请求
  -> Prompt 抽取
  -> 本地 Pool Router + 任务理解
  -> Official vLLM 语义分数（可选，仅旁路）
  -> Rule-based Tier Router
  -> API Key 所属 group 白名单
  -> 任务适配度 + 实时账号状态的候选模型排序
  -> model_score_list（旁路建议）
  -> 网关原 Scheduler 仍是主链路
```

完整职责和评分公式见 [架构说明](docs/ARCHITECTURE_CN.md)。

## 快速启动

前置条件：Go 1.21+。Docker 仅在启用 Official vLLM score-only 时需要。

```bash
# 已 git clone 到 semantic-router 后，仓库根目录就是 Go module 根目录。
cd semantic_router
go test -run 'TestModelSelector' -v

export SEMANTIC_ROUTER_HTTP_PORT=18080
export SEMANTIC_ROUTER_HTTP_LISTEN_ADDRESS=127.0.0.1:18080
export SEMANTIC_ROUTER_TAKEOVER_ENABLED=false
export SEMANTIC_ROUTER_DRY_RUN_ENABLED=true
export MODEL_SELECTOR_SECRET='change-me'
go run ./cmd/server
```

另开终端检查：

```bash
curl -H 'X-Selector-Secret: change-me' \
  http://127.0.0.1:18080/v1/model-selector/heartbeat
```

启动可视化 Playground：

```bash
export PLAYGROUND_PORT=8081
export PLAYGROUND_LISTEN_ADDRESS=127.0.0.1:8081
export SEMANTIC_ROUTER_TAKEOVER_ENABLED=false
go run ./cmd/playground
```

浏览器访问 `http://127.0.0.1:8081/debug/router-playground`。

## 我该从哪里开始？

| 你的目标 | 建议路径 |
| --- | --- |
| 本地体验路由和模型推荐 | 按上面的快速启动运行 Selector 与 Playground。 |
| 部署到 Ubuntu 内网 | 阅读[部署指南](docs/DEPLOYMENT_CN.md)，再执行[运行检查](docs/OPERATIONS_CN.md)。 |
| 接入 TokenCloud / 网关 | 阅读[接口文档](docs/API_CN.md)，先调用 `/heartbeat`，再以 Shadow 方式调用 `/select`。 |
| 理解为什么推荐某个模型 | 阅读[架构与评分](docs/ARCHITECTURE_CN.md)，并在 Playground 查看候选、分数与原因。 |
| 修改路由或候选策略 | 先运行[测试与评估](docs/TESTING_CN.md)中的聚焦测试；不要修改冻结 holdout fixture。 |

### 最小使用流程

1. 网关把当前 API Key 所属 group 的 `model_list`、`models` 和可用 `accounts` 连同
   `user_api_call` 发送给 `/v1/model-selector/select`。
2. Selector 只在该 group 内抽取最新用户 Prompt，返回同一组模型的四位小数
   `model_score_list` 和推荐候选。
3. Shadow 阶段网关只记录该建议，真实调用仍使用原 Scheduler。
4. 在 `/status`、`/history` 或 Playground 观察分歧、延迟、fallback 与候选排序；确认
   充分后才单独讨论是否启用任何 takeover 策略。

## 文档

- [架构与评分](docs/ARCHITECTURE_CN.md)
- [部署指南](docs/DEPLOYMENT_CN.md)
- [运行与安全](docs/OPERATIONS_CN.md)
- [测试与评估](docs/TESTING_CN.md)
- [可复现评估结果](docs/EVALUATION_RESULTS_CN.md)
- [公开 Playground 测试数据](testdata/README.md)
- [HTTP API 与 TokenCloud v1.3](docs/API_CN.md)
- [TokenCloud v1.3 接入待办](docs/TOKENCLOUD_V13_REMAINING_WORK_CN.md)
- [模型画像与 Hybrid V2](docs/MODEL_PROFILE_AND_HYBRID_V2_CN.md)
- [文档阅读顺序与目录说明](docs/README_CN.md)

## 当前状态

Selector 和 Playground 已在 Ubuntu 内网以 Shadow-only 模式部署。Selector 已
支持 v1.3 账号快照，并具备动态负载、禁用账号、配额耗尽、成本和 TTFT 的
受控回放测试。TokenCloud 仍需在真实 Shadow 转发链路中发送 `models` 与
`accounts`，才能开始真实生产账号遥测观测。

## 安全保证

- 正常部署必须保持 `SEMANTIC_ROUTER_TAKEOVER_ENABLED=false`。
- Selector 或 vLLM 异常只会降级，不阻塞主请求。
- 账号 `0`、禁用、限流、过载、临时不可调度或配额耗尽账号不会被推荐。
- 禁止提交 API Key、账号凭证、原始抓包和原始用户 Prompt；公开历史与报告
  必须脱敏。

## 许可证

当前尚未选择开源许可证。在项目所有者为该仓库添加许可证前，请勿复用或再分发代码。
