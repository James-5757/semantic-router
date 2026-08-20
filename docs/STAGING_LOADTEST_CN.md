# Staging Shadow 压测

[English](STAGING_LOADTEST.md)

## 启动 staging Selector

```powershell
$env:SEMANTIC_ROUTER_TCP_LISTEN_ADDRESS="127.0.0.1:9101"
$env:SEMANTIC_ROUTER_STAGING_GROUP_ID="1001"
go run ./cmd/staging-shadow
```

## 接口模拟

```powershell
go run ./cmd/staging-simulator -address 127.0.0.1:9101
```

模拟器覆盖 code、data、vision、document、chat 和 image-generation，并打印每条
dry-run JSON 响应。

## 压测

```powershell
go run ./cmd/staging-loadtest -address 127.0.0.1:9101 -requests 1000 -concurrency 16 -group-id 1001
```

输出包括成功/错误率、P50/P95/P99、吞吐、Pool 分布、账号 0 次数和上游调用次数。
这只是本地协议和性能基线，不是生产容量承诺。

历史 100 请求、8 worker 运行记录：成功 `100/100`、错误率 `0%`、P95 约 `5.55ms`、
P99 约 `6.46ms`、吞吐约 `1858 req/s`、上游调用 `0`、账号 0 选择 `0`。
