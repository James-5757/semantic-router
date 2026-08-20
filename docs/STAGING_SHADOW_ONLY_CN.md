# Staging Shadow-Only

[English](STAGING_SHADOW_ONLY.md)

> 当前状态：本地/Staging 仅 Shadow。Candidate Ranking V2、模型画像与 Hybrid 都只做
> 观察，takeover 保持关闭。

## 启动

在仓库根目录执行：

```powershell
$env:SEMANTIC_ROUTER_TCP_LISTEN_ADDRESS = "127.0.0.1:9101"
$env:SEMANTIC_ROUTER_STAGING_GROUP_ID = "1001"
go run ./cmd/staging-shadow
```

服务使用 `StagingAccountRepository` 与 `NewDefaultModelRegistry`。其中的账号和模型
画像都是无凭证的 staging fixture，不是生产账号信息。

## 必须保持的安全值

```text
shadow_enabled   = true
dry_run_enabled  = true
takeover_enabled = false
upstream_called  = false
```

TCP 响应只是建议。TokenCloud 调用方必须继续把旧 Scheduler 结果作为真实结果。
`TokenCloudShadowAdapter` 会记录 old/new account、model、pool、一致性、错误和延迟。

## 最小验收

- `code/data/vision/document` 只选取支持对应能力的账号。
- disabled、不可调度和 `account_id=0` 不能被推荐。
- 可选 `model_ids` 必须限制在当前 group 候选范围。
- Playground 可显示旧 Scheduler 与 V2 建议的账号、模型和 ranking margin。

在真实账号仓库接入前，不得将 staging fixture 用于生产调度。
