# 部署指南

[English](DEPLOYMENT.md)

本指南描述如何在 Ubuntu 22.04 部署 Selector、可选 Official vLLM score-only
服务和 Playground。所有组件默认保持 Shadow-only。

## 前置条件

- Ubuntu 22.04、Go 1.21+、Docker Engine、Nginx（推荐）。
- 私有网络或 VPN，不要公开暴露 Playground。
- 通过 Git 之外的 Secret 管理方式保存共享密钥。

## 构建

```bash
git clone <repository-url> semantic-router
cd semantic-router/semantic_router
go test -run 'TestModelSelector' -v
go build -o bin/semantic-router-server ./cmd/server
go build -o bin/router-playground ./cmd/playground
```

## Selector 的 systemd 服务

复制 [semantic-router-selector.service](../deploy/ubuntu/semantic-router-selector.service)
到 `/etc/systemd/system/`，然后按实际路径修改。必须保持以下安全值：

```ini
Environment=SEMANTIC_ROUTER_HTTP_PORT=18080
Environment=SEMANTIC_ROUTER_HTTP_LISTEN_ADDRESS=127.0.0.1:18080
Environment=SEMANTIC_ROUTER_DRY_RUN_ENABLED=true
Environment=SEMANTIC_ROUTER_TAKEOVER_ENABLED=false
EnvironmentFile=-/etc/semantic-router/selector.env
```

从 [selector.env.example](../deploy/ubuntu/selector.env.example) 创建
`/etc/semantic-router/selector.env`，填入真实密钥并设置权限：

```bash
sudo install -d -m 0750 /etc/semantic-router
sudo install -m 0600 /dev/null /etc/semantic-router/selector.env
sudoedit /etc/semantic-router/selector.env

sudo install -d -o "$USER" /var/lib/semantic-router
sudo systemctl daemon-reload
sudo systemctl enable --now semantic-router-selector
curl -fsS -H "X-Selector-Secret: $MODEL_SELECTOR_SECRET" \
  http://127.0.0.1:18080/v1/model-selector/heartbeat
```

## 可选 Official vLLM score-only 服务

Selector 可以不依赖 vLLM 工作。启用时仅作为最佳努力的语义辅助：

```ini
MODEL_SELECTOR_OFFICIAL_VLLM_ENABLED=true
MODEL_SELECTOR_OFFICIAL_VLLM_URL=http://127.0.0.1:8080
MODEL_SELECTOR_OFFICIAL_VLLM_TIMEOUT_MS=800
```

容器必须绑定 loopback。应使用已维护的内部镜像和配置，不要在生产主机上
隐式下载模型。vLLM 不可用时只能增加 fallback 指标，不能导致 `/select` 失败。

```bash
curl -fsS http://127.0.0.1:8080/health
curl -fsS http://127.0.0.1:8080/ready
```

## Playground

复制 [playground.service](../deploy/ubuntu/playground.service)，使用 loopback：

```ini
Environment=PLAYGROUND_PORT=8081
Environment=PLAYGROUND_LISTEN_ADDRESS=127.0.0.1:8081
Environment=SEMANTIC_ROUTER_TAKEOVER_ENABLED=false
Environment=PLAYGROUND_SCHEDULER_MODE=platform
Environment=PLAYGROUND_MODEL_GROUP=<demo-group-name>
```

```bash
sudo systemctl enable --now playground
curl -fsS http://127.0.0.1:8081/health
```

## Nginx 内网访问

只暴露内网地址：`/v1/model-selector/` 代理到 Selector，其他路径代理到
Playground。对可信私网设置 IP 白名单，并配置 Basic Auth 或内部 SSO。

```nginx
location /v1/model-selector/ {
  proxy_pass http://127.0.0.1:18080;
}

location / {
  proxy_pass http://127.0.0.1:8081;
}
```

## 升级与回滚

1. 构建并执行目标测试。
2. 上传为 `semantic-router-server.next`。
3. 保留当前二进制为 `semantic-router-server.bak`。
4. 原子安装新文件，只重启 Selector 服务。
5. 验证 `/heartbeat`、`/status` 和一条 Shadow `/select`。
6. 异常时恢复 `.bak` 并重启。升级或回滚时都不得开启 takeover。
