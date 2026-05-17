# saga-sandbox

Internal gRPC service scaffolded by
[`service-gen`](https://github.com/kitti12911/service-gen).

## What's included

- gRPC server with the standard health service and reflection.
- Configuration via `config.yml` / environment variables (`lib-util/v3`).
- Structured logging, OpenTelemetry tracing, and Pyroscope profiling.
- `lib-orm/v3` database connection wiring (no migrations — those live in a
  migration repository such as `migration-sandbox`).
- A `internal/feature/starter` package with a single `Ping` function as a
  starting point.
- A local `proto/` directory plus `buf.gen.yaml`. Drop `.proto` files in
  `proto/` and run `make gen` to produce code under `gen/grpc/`.
- GitHub Actions and/or GitLab CI, Renovate, CODEOWNERS, golangci-lint,
  markdownlint, Prettier, `.air.toml`, and a multi-stage `Dockerfile`.

## Getting started

```sh
cp config.example.yml config.yml   # then edit values
make run
```

## Common commands

| Command       | Description                               |
| ------------- | ----------------------------------------- |
| `make run`    | Start the gRPC server locally             |
| `make air`    | Run with live reload                      |
| `make gen`    | Generate protobuf code from `proto/`      |
| `make test`   | Run tests with the race detector          |
| `make lint`   | Run Go and Markdown linting               |
| `make format` | Format Go, Markdown, YAML, JSON           |
| `make cov`    | Generate and open an HTML coverage report |

## Adding your service

1. Add `.proto` files under `proto/`.
2. Run `make gen`.
3. Implement handlers (use `internal/feature/starter` as a template) and
   register them in `internal/server/grpc.go`.

## Deployment

A starter Helm chart is included in [`helm/`](helm/) so the service is
deployable out of the box. Deployment state should not live in the service
repository long-term — move the chart to your `helm-sandbox` repository (or
your preferred chart repository), then delete `helm/` from here:

```sh
mv helm /path/to/helm-sandbox/charts/saga-sandbox
```

Keep app-specific values (image, env, scaling, config) in the chart
repository, not in the service repo.
