# Staging Shadow Load Test

## Start the local staging selector

```powershell
$env:SEMANTIC_ROUTER_TCP_LISTEN_ADDRESS="127.0.0.1:9101"
$env:SEMANTIC_ROUTER_STAGING_GROUP_ID="1001"
go run ./cmd/staging-shadow
```

## Run interface simulation

```powershell
go run ./cmd/staging-simulator -address 127.0.0.1:9101
```

The simulator covers code, data, vision, document, chat, and image-generation
requests and prints each dry-run response as JSON.

## Run load test

```powershell
go run ./cmd/staging-loadtest -address 127.0.0.1:9101 -requests 1000 -concurrency 16 -group-id 1001
```

The result reports success/error rate, P50/P95/P99 latency, throughput, pool
distribution, account-zero count, and upstream-called count. This is a local
protocol baseline, not a production capacity result.

## Current local result

The automated 100-request/8-worker test completed with:

- Success: 100/100
- Error rate: 0%
- P95: about 5.55ms
- P99: about 6.46ms
- Throughput: about 1858 requests/second
- Upstream calls: 0
- Account-zero selections: 0
