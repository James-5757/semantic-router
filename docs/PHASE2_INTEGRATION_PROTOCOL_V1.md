# Phase 2 Integration Protocol V1

## Scope

Phase 2 adds the Token Cloud integration boundary without changing the main
Scheduler result. The selector only returns a candidate from the existing
`RealSchedulerDryRun` and never sends a request to an upstream model.

## Runtime Defaults

```yaml
semantic_router_integration_enabled: true
semantic_router_shadow_enabled: true
semantic_router_dry_run_enabled: true
semantic_router_takeover_enabled: false
semantic_router_tcp_listen_address: 127.0.0.1:9101
semantic_router_service_address: 127.0.0.1:9101
semantic_router_max_frame_bytes: 1048576
```

`semantic_router_takeover_enabled=true` is rejected by `IntegrationConfig` in
phase 2. Any connection or selection error remains an integration error and
does not provide a model completion or replace an old Scheduler result.

## Transport

TCP uses one request per connection. Each frame is four-byte unsigned
big-endian JSON byte length followed by a UTF-8 JSON payload. The maximum
frame is 1 MiB and both sides apply connect/request deadlines.

## Request and Response

`ModelSelectionRequest` and `ModelSelectionResponse` are the canonical Go
schema. A request must contain `request_id`, non-zero `group_id`, and
`prompt`. `model_ids` is an optional group allowlist. The response always
contains `dry_run=true`, `shadow_only=true`, and `upstream_called=false`.

The response includes `selected_account_id`, `selected_model`,
`selected_pool`, `selected_tier`, and `scheduler_layer`, plus candidate details
for observability.

## Safety Invariants

- Account ID `0` is rejected.
- Disabled or unschedulable accounts are filtered by the existing scheduler.
- Pool capability and tier checks remain mandatory.
- Model allowlists are applied before candidate selection.
- The existing Shadow/Takeover path remains unchanged and takeover remains off.

## Current Status

- Protocol types: implemented.
- Selector TCP server/client: implemented.
- Dry-run integration tests: implemented.
- Token Cloud TCP Shadow Adapter: implemented; old Scheduler remains `Main()`.
- Local shadow performance baseline: implemented (50 requests, P95 target < 200ms).
- Token Cloud production request interception wiring: not enabled yet.
- Real upstream model calls: none.
