# saga-sandbox

Internal gRPC service scaffolded by
[`service-gen`](https://github.com/kitti12911/service-gen).

## What's included

- gRPC server with the standard health service and reflection.
- Configuration via `config.yml` / environment variables (`lib-util/v3`).
- Structured logging, OpenTelemetry tracing, and Pyroscope profiling.
- `lib-orm/v3` database connection wiring (no migrations — those live in a
  migration repository such as `migration-sandbox`).
- Saga orchestration, relay, sweeper, NATS, and gRPC handler packages.
- Protobuf generation from `proto-sandbox` through `buf.gen.yaml`; generated
  code is written under `gen/grpc/`.
- GitHub Actions, Renovate, CODEOWNERS, golangci-lint, markdownlint,
  Prettier, `.air.toml`, and a multi-stage `Dockerfile`.

## Getting started

```sh
cp config.example.yml config.yml   # then edit values
make run
```

## Common commands

| Command       | Description                                 |
| ------------- | ------------------------------------------- |
| `make run`    | Start the gRPC server locally               |
| `make air`    | Run with live reload                        |
| `make gen`    | Generate protobuf code from `proto-sandbox` |
| `make test`   | Run tests with the race detector            |
| `make lint`   | Run Go and Markdown linting                 |
| `make format` | Format Go, Markdown, YAML, JSON             |
| `make cov`    | Generate and open an HTML coverage report   |

## Adding your service

1. Update the saga protobuf contract in `proto-sandbox`.
2. Point `buf.gen.yaml` at the released `proto-sandbox` tag.
3. Run `make gen`.
4. Implement handlers in `internal/server/` and business logic under
   `internal/saga/`, `internal/relay/`, or `internal/messaging/`.

## Deployment

Deployment state should not live in this service repository long-term. Keep
shared chart templates in `helm-sandbox` and app-specific values in
`homelab-devops/apps`.
