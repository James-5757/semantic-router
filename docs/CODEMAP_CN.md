# 代码导航

本仓库是单个 Go module。多数生产 Go 文件有意保留在模块根目录，因为它们共享
同一个 package 和大量路由类型。不要为了让目录看起来更小就移动文件：这会改变
import 并增加集成风险。修改前请先通过本页找到正确的职责边界。

## 从哪里开始

| 目标 | 主要文件 |
| --- | --- |
| 启动 HTTP Selector | `cmd/server/`、`server.go` |
| 启动可视化调试页 | `cmd/playground/` |
| 理解用户请求 | `prompt_parser.go`、`task_understanding.go`、`output_contract.go` |
| 决定 Pool 或 Tier | `semantic_router.go`、`rules.go`、`tier_router.go`、`v2_router.go` |
| 候选模型排序 | `model_selector.go`、`platform_model_catalog.go`、`staging_accounts.go` |
| 应用实时账号状态 | `model_selector_synced_catalog.go`、`model_selector_api_key_groups.go` |
| 检查 Shadow 安全 | `shadow_mode.go`、`shadow_config.go`、`shadow_metrics.go`、`token_cloud_shadow.go` |
| 调用可选语义服务 | `vllm_pool_client/`、`routellm_tier.go`、`routellm_tier_service.py` |

## 目录职责

| 路径 | 内容 |
| --- | --- |
| `cmd/server/` | 可运行的 Selector HTTP 服务。 |
| `cmd/playground/` | 内部调试用 Playground 页面。 |
| `deploy/ubuntu/` | Ubuntu 的 systemd、Nginx 和环境变量示例。 |
| `docker/` | 可选 Official vLLM score-only 服务配置。 |
| `vllm_pool_client/` | 可选 vLLM 信号的客户端与协议边界。 |
| `testdata/playground/` | 可公开分享、已脱敏的 Playground 开发评估样本。 |
| `docs/` | 面向使用者、部署者、集成方和贡献者的维护文档。 |

## 测试与修改边界

- 每个组件旁边的 `*_test.go` 是最接近它的回归覆盖。
- 常见集成改动可先查看 `output_contract_test.go`、`model_selector_test.go`、
  `shadow_mode_test.go` 和 `model_selector_http_test.go`。
- `holdout_v2_eval_test.go` 与冻结 Pool fixture 是评估保护线；不要为了提升报告
  分数修改其预期样本。
- 共享环境与 staging 必须保持 `SEMANTIC_ROUTER_TAKEOVER_ENABLED=false`。Selector
  只返回旁路建议，网关 Scheduler 仍负责真实上游请求。

公共部署与支持的命令请回到根目录 [README_CN.md](../README_CN.md)。
