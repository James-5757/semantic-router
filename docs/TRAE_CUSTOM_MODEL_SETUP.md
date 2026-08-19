# TRAE Custom Model Setup Guide

## Integrating vLLM Semantic Router with TRAE IDE

This guide explains how to configure TRAE IDE to use the local vLLM Semantic Router as a custom model provider.

---

## System Architecture

```
TRAE IDE (Client)
    |
    | HTTP (OpenAI-compatible API)
    v
TRAE Adapter (http://127.0.0.1:8082)
    |
    | HTTP Forward
    v
Envoy (http://127.0.0.1:8899)
    |
    |-- gRPC (ext_proc) --> vLLM Semantic Router --> Signal Evaluation --> Decision
    |
    v
Upstream Model API (External Provider)
    |
    v
OpenAI-compatible Response
```

**Key Points:**
- TRAE sends requests to the local TRAE Adapter
- The Adapter forwards to Envoy, which uses the Semantic Router to decide which upstream model to use
- The Semantic Router evaluates signals (code, data_analysis, document, vision, image_generation, etc.)
- The decision determines which external model API receives the request
- The response flows back through the same path

---

## TRAE Configuration

Open TRAE IDE Settings > Models > Add Custom Model, and fill in:

| Field | Value |
|-------|-------|
| **Provider/API Format** | `OpenAI` |
| **Base URL** | `http://127.0.0.1:8082` |
| **API Key** | *(leave empty)* |
| **Model ID** | `MoM` |

**Notes:**
- API Key can be left empty or set to any value
- If `TRAE_ROUTER_API_KEY` environment variable is set, use that value as the API Key

---

## Prerequisites

1. **Docker Desktop** - installed and running on Windows
2. **WSL 2** - installed with Docker integration enabled
3. **vLLM Semantic Router Stack** - Docker containers running (see Startup Sequence below)

---

## Startup Sequence

### Step 1: Start the Semantic Router Stack

Ensure all Docker containers are running. In WSL:

```bash
cd /mnt/e/university/API-Project/for_image_generate/semantic_router
docker compose up -d
```

Wait for all containers to be healthy:

```bash
docker ps
```

Expected containers:
- `vllm-sr-envoy-container` - Port 8899
- `vllm-sr-router-container` - Port 8080
- `vllm-sr-dashboard-container` - Port 8700
- `vllm-sr-sim-container` - Port 8810
- `vllm-sr-grafana` - Port 3000
- `vllm-sr-jaeger` - Port 16686
- `vllm-sr-postgres` - Port 5432
- `vllm-sr-redis` - Port 6379

### Step 2: Start the TRAE Adapter

**Option A: PowerShell (Recommended for Windows)**

```powershell
powershell.exe -File scripts/start_trae_router.ps1
```

**Option B: WSL Shell**

```bash
bash scripts/start_trae_router.sh
```

**Option C: Manual Start**

```powershell
cd e:\university\API-Project\for_image_generate\semantic_router
.\tools\trae_adapter\trae-adapter.exe
```

### Step 3: Verify

Run the check script:

```powershell
powershell.exe -File scripts/check_trae_router.ps1
```

Expected output:
```
STATUS: ALL SYSTEMS OPERATIONAL
Router running:       YES
Envoy running:        YES
OpenAI endpoint:      REACHABLE
Models endpoint:      OK
Chat endpoint:        OK
```

### Step 4: Configure TRAE

1. Open TRAE IDE
2. Go to Settings > Models
3. Add a new custom model:
   - API Format: `OpenAI`
   - Base URL: `http://127.0.0.1:8082`
   - API Key: *(leave empty)*
   - Model ID: `MoM`
4. Save and select the model

### Step 5: Smoke Test

Send a test message in TRAE IDE:

```
Say hello in one word
```

You should receive a response. To confirm it goes through the Semantic Router:

```bash
# Check Envoy logs for router decision headers
docker logs vllm-sr-envoy-container --tail 10

# Look for:
# x-vsr-selected-decision: route_xxx
# x-vsr-selected-model: xxx
```

---

## How to Stop

### Stop the Adapter

```powershell
powershell.exe -File scripts/stop_trae_router.ps1
```

Or manually:

```powershell
Stop-Process -Name "trae-adapter" -Force
```

### Stop the Full Stack

In WSL:

```bash
cd /mnt/e/university/API-Project/for_image_generate/semantic_router
docker compose down
```

---

## How to Restart

```powershell
# Stop adapter
powershell.exe -File scripts/stop_trae_router.ps1

# Start adapter
powershell.exe -File scripts/start_trae_router.ps1
```

---

## API Endpoints

The TRAE Adapter exposes the following OpenAI-compatible endpoints:

### GET /v1/models

Returns available models.

**Example:**
```bash
curl http://127.0.0.1:8082/v1/models
```

**Response:**
```json
{
  "object": "list",
  "data": [
    {
      "id": "MoM",
      "object": "model",
      "created": 1234567890,
      "owned_by": "local-semantic-router"
    }
  ]
}
```

### POST /v1/chat/completions

Sends a chat completion request through the Semantic Router.

**Example:**
```bash
curl -X POST http://127.0.0.1:8082/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "MoM",
    "messages": [
      {"role": "user", "content": "Write a Python function to sort a list"}
    ],
    "temperature": 0.7,
    "max_tokens": 500
  }'
```

**Response Headers (for diagnostics):**
- `X-VSR-Decision` - The Semantic Router decision (e.g., `route_code`)
- `X-VSR-Model` - The selected upstream model

### GET /health

Health check endpoint.

```bash
curl http://127.0.0.1:8082/health
```

---

## Common Errors

### "Connection refused"

**Cause:** The TRAE Adapter or Docker containers are not running.

**Solution:**
```powershell
# Check if Docker is running
docker ps

# Check if adapter is running
Get-Process -Name "trae-adapter" -ErrorAction SilentlyContinue

# Start components
powershell.exe -File scripts/start_trae_router.ps1
```

### "404 Not Found"

**Cause:** Incorrect Base URL or path in TRAE configuration.

**Solution:** Verify TRAE settings:
- Base URL: `http://127.0.0.1:8082` (not `8899`)

### "502 Bad Gateway"

**Cause:** The Envoy cannot connect to the upstream model API.

**Solution:**
```bash
# Check Envoy logs
docker logs vllm-sr-envoy-container --tail 50

# Look for "no healthy upstream" or connection errors
```

### "503 no healthy upstream"

**Cause:** The upstream model provider is not available.

**Solution:**
1. Verify upstream model service is running
2. Check Envoy logs for more details

### Docker/WSL not running

**Cause:** Docker Desktop is not started.

**Solution:** Start Docker Desktop from the Start Menu and wait for it to be ready.

---

## How to Verify Semantic Router is Processing

Check the Envoy response headers:

```bash
curl -sI -X POST http://127.0.0.1:8899/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"MoM","messages":[{"role":"user","content":"hi"}],"max_tokens":5}'
```

Look for headers starting with `x-vsr-`:
- `x-vsr-selected-decision` - Which route was selected
- `x-vsr-selected-model` - Which upstream model was chosen
- `x-vsr-selected-confidence` - Confidence score of the decision

Or directly from the adapter:
```bash
curl -sD - -X POST http://127.0.0.1:8082/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"MoM","messages":[{"role":"user","content":"Write Python code"}],"max_tokens":10}'
```

---

## Viewing Logs

### Envoy Logs
```bash
docker logs vllm-sr-envoy-container --tail 100
```

### Router Logs
```bash
docker logs vllm-sr-router-container --tail 100
```

### Adapter Logs
```powershell
Get-Content "tools/trae_adapter/trae-adapter.log"
```

---

## How to Rollback

If you need to revert the changes made for TRAE integration:

### 1. Stop the Adapter
```powershell
powershell.exe -File scripts/stop_trae_router.ps1
```

### 2. Remove Adapter Binary
```powershell
Remove-Item tools/trae_adapter/trae-adapter.exe -Force
```

### 3. Revert Git Changes
```bash
git checkout -- tools/trae_adapter/
git checkout -- scripts/
git checkout -- docs/
```

### 4. Remove Added Files
```bash
git clean -fd tools/ scripts/ docs/
```

The existing Semantic Router stack (Docker containers, Envoy config, model routing) is unchanged.

---

## Security Notes

- **No upstream credentials are stored in the adapter**
- The adapter forwards requests without modifying credentials
- Upstream API keys are managed by the Envoy configuration only
- API Key validation is optional and only for local access
- The adapter does not log Authorization headers or sensitive prompt content
- All routing decisions remain with the vLLM Semantic Router
