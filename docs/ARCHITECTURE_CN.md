# 架构设计

[English](ARCHITECTURE.md)

## 设计目标

Semantic Router 不是上游模型代理，而是为现有网关提供决策建议的服务。网关仍
负责鉴权、限流、账号槽位、重试和真实上游调用。职责分离让系统可以安全地
灰度、观测和回滚。

## 请求链路

```text
TokenCloud 请求（原始 JSON 经 gzip + Base64）
  -> 抽取最新用户 Prompt
  -> 本地 Pool Router
  -> 高精度专业任务保护规则
  -> Official vLLM 语义观察（可选）
  -> Rule-based Tier 决策
  -> group 白名单和账号可用性过滤
  -> 候选模型评分
  -> model_score_list
  -> TokenCloud 原 Scheduler（Shadow 模式不变）
```

## Pool 与 Tier

Pool 代表主要能力：`code`、`data`、`document`、`vision`、
`image_generation`、`cheap_chat`、`default/general`。Tier 独立输出
`weak`、`medium` 或 `strong`。

Pool 与模型排序必须分开评估。Pool 判错应通过 Pool 校准修复，不能用账号
负载排序来掩盖语义分类问题。

## Official vLLM 的使用方式

Official vLLM 仅提供 score-only 语义信号，并不调用任何强/弱上游模型。
其结果独立返回在 `semantics` 中。

为防止弱语义信号扰动本地路由：

- 同 Pool 的 Official 信号可以提升置信度。
- 本地专业 Pool 置信度达到 `0.50` 时受保护。
- 跨 Pool 修正要求 Official 顶分 `>= 0.68`，且 Top1-Top2 margin `>= 0.10`。
- 候选模型排序只读取最终 Pool 对应的 Official 分数，不使用无关类别的最高分。

## 候选模型评分

每个候选模型先依据最终 Pool、模型画像、语言、推理需求与匹配的语义信号
计算任务适配分：

```text
final_model_score = 0.70 * static_task_fit + 0.30 * runtime_score
```

账号过滤会排除：账号 ID 为 0、不可调度、限流、过载、临时屏蔽、配额耗尽，
或不支持当前模型映射的账号。

```text
runtime_score =
  0.40 * 空闲容量
+ 0.15 * 排队分
+ 0.15 * 稳定性分
+ 0.10 * 优先级分
+ 0.10 * 成本分
+ 0.10 * TTFT 分
```

成本分来自 `rate_multiplier`，倍率越低越好。TTFT 分来自 `ttft_ewma_ms`：
约 500ms 视为优秀，3000ms 及以上视为慢。没有 TTFT 样本的平台会对已知
指标重新归一化，不会被当作低性能账号。

## 模型组边界

一个 API Key 对应一个物理模型 group。`model_list`、`models`、`accounts`
只能描述该 group；选择器会拒绝组外模型，也不会在单次请求中混合国内和
国外候选模型。

## 部署组件

| 组件 | 默认绑定地址 | 职责 |
| --- | --- | --- |
| Selector Server | `127.0.0.1:18080` | 心跳、选择、状态、历史、group 同步。 |
| Official vLLM | `127.0.0.1:8080` | 可选的 score-only 语义观察。 |
| Playground | `127.0.0.1:8081` | 内网可视化调试与历史查看。 |
| Nginx | 内网入口 | 限制访问并反向代理 UI/API。 |

部署见 [DEPLOYMENT_CN.md](DEPLOYMENT_CN.md)，日常运行见
[OPERATIONS_CN.md](OPERATIONS_CN.md)。
