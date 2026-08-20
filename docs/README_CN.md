# 文档索引

[English](README.md)

## 推荐阅读顺序

| 场景 | 先读什么 | 然后做什么 |
| --- | --- | --- |
| 第一次本地运行 | 根目录 `README_CN.md` | 启动 Selector、检查 `/heartbeat`、再启动 Playground。 |
| Ubuntu 内网部署 | [部署指南](DEPLOYMENT_CN.md) | 按[运行与安全](OPERATIONS_CN.md)验证服务、日志和回滚。 |
| TokenCloud 接入 | [HTTP API](API_CN.md) | 先用 Shadow `/select` 回传候选分数，不改变网关主调度。 |
| 理解路由和排序 | [架构设计](ARCHITECTURE_CN.md) | 查看 Playground 的 Pool、Tier、候选模型和原因。 |
| 开发或校准 | [测试与评估](TESTING_CN.md) | 运行聚焦测试，记录 holdout 变化，不删除冻结 fixture。 |

## 维护中的文档

- [架构设计](ARCHITECTURE_CN.md)：组件职责、边界和评分。
- [部署指南](DEPLOYMENT_CN.md)：Ubuntu、systemd、vLLM 与内网 Nginx。
- [运行与安全](OPERATIONS_CN.md)：监控、安全、排障和数据处理。
- [测试与评估](TESTING_CN.md)：聚焦测试、手工验证和评估指标。
- [可复现评估结果](EVALUATION_RESULTS_CN.md)：公开开发集的当前基线、命令和解读边界。
- [公开测试数据](../testdata/README.md)：Playground 导入、来源、哈希与使用限制。
- [HTTP API 与 TokenCloud v1.3](API_CN.md)：接口与真实账号快照协议。
- [v1.3 剩余工作](TOKENCLOUD_V13_REMAINING_WORK.md)：对接 backlog。

## 目录职责

| 路径 | 内容 |
| --- | --- |
| 根目录 `README_CN.md` | 项目入口、快速运行和安全边界。 |
| `docs/` | 面向使用者、部署者和集成方的维护文档。 |
| `cmd/server/` | Selector HTTP 服务入口。 |
| `cmd/playground/` | 内网调试页面及其静态资源。 |
| `deploy/ubuntu/` | systemd、Nginx 和环境变量示例。 |
| `docker/` | Optional Official vLLM score-only 配置。 |
| 根目录 `*_test.go` | 单元、边界和 Shadow 安全测试。 |

本目录和模块根目录中保留了历史实验报告，便于追溯；它们不替代上述维护文档。
不要将原始抓包、密钥、账号凭证或未脱敏 Prompt 放入仓库。
