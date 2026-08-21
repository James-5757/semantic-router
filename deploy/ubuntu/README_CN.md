# Ubuntu 内网演示部署

[English](README.md)

本目录用于在内网部署 Playground 演示，并让 Selector 与 vLLM 评分服务保持在
loopback 地址。它不会开启 takeover，也不会调用真实上游模型。

## 服务地址

- `semantic-router-selector.service`：`127.0.0.1:18080`
- `vllm-sr-score-only` Docker 容器：`127.0.0.1:8080`
- `playground.service`：`127.0.0.1:8081`
- Nginx 演示入口：`http://<internal-host>:8088/debug/router-playground`

Nginx 只能允许预期的内网网段，例如 `10.0.0.0/8`。扩大演示范围前必须增加
HTTP Basic Auth 或等效的内部 SSO。`/v1/model-selector/` 会代理到本地 18080 的
Selector，其他路径代理到 Playground。若评审人员不在该网段，请修改对应白名单。

部署的 Playground 固定使用 `国外OPENAI分组`，因此候选排序不会混入其他物理
API Key group 的模型，包括国内 `超讯科技` group。只有需要演示对应 group 时，
才修改 `PLAYGROUND_MODEL_GROUP`。

## Selector 的 vLLM Shadow 配置

从 `selector.env.example` 创建 `/etc/semantic-router/selector.env`，将权限设为
`0600`，并为 `semantic-router-selector.service` 添加：

```ini
Environment=MODEL_SELECTOR_OFFICIAL_VLLM_ENABLED=true
Environment=MODEL_SELECTOR_OFFICIAL_VLLM_URL=http://127.0.0.1:8080
Environment=MODEL_SELECTOR_OFFICIAL_VLLM_TIMEOUT_MS=800
Environment=MODEL_SELECTOR_HISTORY_FILE=/home/sts/semantic-router/router_store/selector_history.jsonl
Environment=SEMANTIC_ROUTER_TAKEOVER_ENABLED=false
Environment=SEMANTIC_ROUTER_DRY_RUN_ENABLED=true
```

service 通过以下配置读取密钥文件：

```ini
EnvironmentFile=-/etc/semantic-router/selector.env
```

Official vLLM 调用是 best-effort：出错或超时时应返回既有 profile-only 结果，
不会影响 TokenCloud 主请求。

`MODEL_SELECTOR_HISTORY_FILE` 会为内网 Playground History 页面保存最近 200 条
已脱敏 Selector 调用。它不会改变 TokenCloud 响应，也不会调用上游模型。

完整安装、升级、回滚与运行检查请阅读
[部署指南](../../docs/DEPLOYMENT_CN.md)、[运行与安全](../../docs/OPERATIONS_CN.md)，
以及 [TokenCloud 对接方清单](../../docs/TOKENCLOUD_PARTNER_CHECKLIST_CN.md)。
