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

每个候选模型先计算 `static_task_fit`，再结合实时账号状态计算最终排序分。
这里的 **static** 不是“固定不变的分数”，而是指不依赖账号当前负载、排队和
TTFT 的任务适配部分；它会随每次请求的最终 Pool、语言和推理信号重新计算。
它也不是模型的真实成功率或线上质量结论。

```text
final_model_score = 0.70 * static_task_fit + 0.30 * runtime_score
```

### static_task_fit 的来源

`static_task_fit` 由以下三类输入组成，所有数值均归一化到 `0~1`：

1. **Pool 基础适配分 (`pool_score`)**：根据最终 Pool 读取模型画像中的对应字段。
   - `code` -> `coding_agent_score`
   - `data` -> `data_analysis_score`
   - `document` -> `document_score`
   - `vision` / `image_generation` -> `vision_score`
   - `cheap/default` -> `general_score`
2. **请求特征修正 (`task_fit_score`)**：从同一模型画像中读取与请求相符的能力。
   中文请求混入 `chinese_score`；包含“推理 / 分析 / reason”等信号的请求混入
   `reasoning_score`。在内部 Scheduler 路径中，`TaskSignals` 还可对应代码、数据、
   长上下文、文档等专项字段。
3. **同 Pool 的 Official vLLM 语义分（可选，Shadow）**：只使用最终 Pool 对应的
   分数，例如 `data` 只读取 `official_vllm_semantic_data_analysis`；绝不会拿不相关
   类别的最高分覆盖当前 Pool。

HTTP 选择器的简化计算过程如下：

```text
task_fit = pool_score
if 中文请求:
  task_fit = 0.75 * task_fit + 0.25 * chinese_score
if 有推理/分析信号:
  task_fit = 0.75 * task_fit + 0.25 * reasoning_score

profile_task_fit = 0.75 * pool_score + 0.25 * task_fit
static_task_fit = profile_task_fit
if 有当前 Pool 的 Official 分数:
  static_task_fit = 0.75 * profile_task_fit + 0.25 * official_pool_score
```

例：最终 Pool 为 `code`，某模型 `coding_agent_score=0.88`、
`chinese_score=0.78`，中文代码请求先得到
`task_fit = 0.75*0.88 + 0.25*0.78 = 0.855`，再得到
`profile_task_fit = 0.75*0.88 + 0.25*0.855 = 0.8738`。若该请求的
Official code 分为 `0.68`，则 `static_task_fit = 0.75*0.8738 + 0.25*0.68 = 0.8253`。

模型画像字段目前是版本化的路由先验（可通过 `profile_source`、`evidence_source`、
`benchmark_version`、`evaluated_at` 和 `score_confidence` 审计），不是从本次请求
生成，也不是直接复制外部排行榜。当前 `platform_catalog_prior` 的 `score_confidence`
较低，后续应以带日期的基准评测、真实 Shadow 结果或人工校准逐步替换。

原生图片生成还有一条硬约束：只有被画像部署到 `image_generation_pool` 的模型可获得
有效候选分；其他传入模型仍会展示在结果中，但其分数为 `0`，原因是
`incompatible_with_selected_pool`。

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

因此，`static_task_fit` 回答的是“这个模型**按当前任务特征是否合适**”，
`runtime_score` 回答的是“这个模型对应的账号**现在是否适合接单**”。前者帮助区分
模型能力，后者只在可用候选之间处理负载、成本与时延。

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
