# Ubuntu 22.04 部署与结果保存

本文用于把当前 semantic-router 阶段二版本部署到 Ubuntu 22.04。默认保持 shadow-only：旧 Scheduler 是线上主结果，semantic-router 只计算建议；不请求真实上游，`semantic_router_takeover_enabled=false`。

## 1. 代码与依赖

```bash
sudo apt-get update
sudo apt-get install -y git curl build-essential
git clone <your-repository-url> semantic-router
cd semantic-router
go version                 # 项目 go.mod 要求 Go 1.21+
go test -v ./...
go build -o bin/playground ./cmd/playground
```

不要把真实 API key、原始 pcap 或数据库密码提交到仓库。生产值通过 systemd `EnvironmentFile` 或 Secret 管理注入。

## 2. vLLM Semantic Router 容器

如果镜像和模型卷已经在 Ubuntu Docker 中存在，优先使用已有容器：

```bash
docker ps -a --filter name=vllm-sr-router-container
docker start vllm-sr-router-container
```

首次部署且已有本地配置时，可使用以下端口映射。是否加载模型由 vLLM 容器自己的配置和模型卷决定。

```bash
docker run -d --name vllm-sr-router-container --restart unless-stopped \
  -p 127.0.0.1:8080:8080 \
  -p 127.0.0.1:8899:8899 \
  -p 127.0.0.1:9190:9190 \
  -v "$PWD/cmd/playground/vllm_runtime_config.yaml:/app/config.yaml:ro" \
  -v vllm_sr_models:/app/models \
  ghcr.io/vllm-project/semantic-router:v0.3.0 /app/config.yaml
```

```bash
curl -fsS http://127.0.0.1:8080/health
curl -fsS http://127.0.0.1:8080/ready
```

## 3. Shadow-only Playground

下面的默认组可以替换成 `超讯科技`，但每次请求仍只在指定物理组内筛选模型。

```bash
export PLAYGROUND_PORT=8081
export PLAYGROUND_SCHEDULER_MODE=platform
export PLAYGROUND_MODEL_GROUP='国外OPENAI分组'
export VLLM_POOL_ENABLED=true
export VLLM_POOL_MOCK_MODE=false
export VLLM_POOL_SERVICE_URL=http://127.0.0.1:8080
export OFFICIAL_VLLM_URL=http://127.0.0.1:8080
export SEMANTIC_ROUTER_SHADOW_ENABLED=true
export SEMANTIC_ROUTER_DRY_RUN_ENABLED=true
export SEMANTIC_ROUTER_TAKEOVER_ENABLED=false
export ROUTER_STORE_DIR=/var/lib/semantic-router/router_store
sudo install -d -o "$USER" /var/lib/semantic-router/router_store
./bin/playground
```

启动后访问 `http://<ubuntu-host>:8081/debug/router-playground`。

## 4. 模型候选分数保存在哪里

接口响应的 `model_ranking.candidates` 是稳定的对外字段，每个候选格式为：

```json
{"rank":1,"account_id":21148,"model":"Qwen3.5-397B-A17B","final_score":0.87425}
```

shadow 持久化时，同一份 JSON 保存到数据库表 `routing_decision_log.model_ranking_json`。Playground 可点击 Raw JSON 保存整个响应；TCP Token Cloud 接入直接保存 `model_ranking` 对象。

已有数据库需要执行一次：

```sql
ALTER TABLE routing_decision_log
  ADD COLUMN model_ranking_json JSON NULL COMMENT '候选模型及 final_score 排名';
```

新环境直接执行 Go migration `RoutingDecisionLogMigration()` 即可。

## 5. 验证 shadow 安全边界

```bash
go test -run 'TestRealSchedulerDryRun|TestShadowMode|TestTokenCloudShadow' -v ./...
curl -fsS http://127.0.0.1:8081/v1/debug/shadow
```

必须满足：正常服务时 `shadow_error_rate=0`、`upstream_called_count=0`，建议账号不为 0，disabled account 不被选择；旧 Scheduler 的结果仍作为主结果返回。

