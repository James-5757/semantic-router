# 文档索引

[English](README.md)

## 推荐阅读顺序

| 场景 | 先读什么 | 然后做什么 |
| --- | --- | --- |
| 第一次本地运行 | 根目录 `README_CN.md` | 启动 Selector、检查 `/heartbeat`、再启动 Playground。 |
| Ubuntu 内网部署 | [部署指南](DEPLOYMENT_CN.md) | 按[运行与安全](OPERATIONS_CN.md)验证服务、日志和回滚。 |
| TokenCloud 接入 | [HTTP API](API_CN.md) | 先用 Shadow `/select` 回传候选分数，不改变网关主调度。 |
| TokenCloud 开发对接 | [对接方清单](TOKENCLOUD_PARTNER_CHECKLIST_CN.md) | 同步 group、绑定 API Key，再验证 Shadow `/select`。 |
| 理解路由和排序 | [架构设计](ARCHITECTURE_CN.md) | 查看 Playground 的 Pool、Tier、候选模型和原因。 |
| 开发或校准 | [测试与评估](TESTING_CN.md) | 运行聚焦测试，记录 holdout 变化，不删除冻结 fixture。 |

## 维护中的文档

- [架构设计](ARCHITECTURE_CN.md)：组件职责、边界和评分。
- [代码导航](CODEMAP_CN.md)：Router、模型选择、Shadow 与集成代码的定位说明。
- [部署指南](DEPLOYMENT_CN.md)：Ubuntu、systemd、vLLM 与内网 Nginx。
- [运行与安全](OPERATIONS_CN.md)：监控、安全、排障和数据处理。
- [测试与评估](TESTING_CN.md)：聚焦测试、手工验证和评估指标。
- [可复现评估结果](EVALUATION_RESULTS_CN.md)：公开开发集的当前基线、命令和解读边界。
- [公开测试数据](../testdata/README.md)：Playground 导入、来源、哈希与使用限制。
- [外部 Playground 回放集](PLAYGROUND_EXTERNAL_DATASETS_CN.md)：本地第三方回放数据及其报告边界。
- [HTTP API 与 TokenCloud v1.3](API_CN.md)：接口与真实账号快照协议。
- [TokenCloud 对接方清单](TOKENCLOUD_PARTNER_CHECKLIST_CN.md)：可执行的联调和验收步骤。
- [v1.3 剩余工作](TOKENCLOUD_V13_REMAINING_WORK.md)：对接 backlog。
- [TokenCloud v1.3 剩余工作（中文）](TOKENCLOUD_V13_REMAINING_WORK_CN.md)：真实转发接入与验收条件。
- [模型画像与 Hybrid V2（中文）](MODEL_PROFILE_AND_HYBRID_V2_CN.md)：画像、候选排序与 tier 旁路。
- [Staging Shadow（中文）](STAGING_SHADOW_ONLY_CN.md)：安全启动与最小验收。
- [Staging 压测（中文）](STAGING_LOADTEST_CN.md)：模拟器、压测命令和基线。
- [Batch Replay 数据提取（中文）](BATCH_REPLAY_DATASET_EXTRACTION_CN.md)：外部数据集提取和 Playground 导入。

## 目录职责

| 路径 | 内容 |
| --- | --- |
| 根目录 `README_CN.md` | 项目入口、快速运行和安全边界。 |
| `docs/` | 面向使用者、部署者和集成方的维护文档。 |
| `docs/CODEMAP_CN.md` | 面向贡献者的源码导航；阅读后再进入根目录的 Go 文件。 |
| `cmd/server/` | Selector HTTP 服务入口。 |
| `cmd/playground/` | 内网调试页面及其静态资源。 |
| `deploy/ubuntu/` | systemd、Nginx 和环境变量示例。 |
| `docker/` | Optional Official vLLM score-only 配置。 |
| 根目录 `*_test.go` | 单元、边界和 Shadow 安全测试。 |

本目录和模块根目录中保留了历史实验报告，便于追溯；它们不替代上述维护文档。
不要将原始抓包、密钥、账号凭证或未脱敏 Prompt 放入仓库。

## 英文历史参考

`MODEL_SELECTOR_HTTP_V1_2_DEBUG.md`、`PHASE2_INTEGRATION_PROTOCOL_V1.md`、
`TRAE_CUSTOM_MODEL_SETUP.md` 和阶段性状态/报告主要用于追溯旧协议或实验背景，
不属于当前部署与接入主流程。需要使用时可参考英文原文，并以本索引中的中文维护文档
为当前行为准则。
