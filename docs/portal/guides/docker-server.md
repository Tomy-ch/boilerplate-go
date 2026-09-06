# server Dockerfile

This Dockerfile defines the application server images. It uses multi-stage builds to provide targets for production and local development.

## Role

`docker/server/Dockerfile` is the single source of truth for every server-side container the project ships. One Dockerfile produces three targets (`builder` / `runtime` / `tooling`) so that production deploys and local hot-reload development stay aligned on the same base layers and Go toolchain version. Keeping these in one place avoids drift between "what runs in production" and "what runs on a developer's laptop" while letting each target add only what it needs (e.g., dev tools live only in `tooling`). Schema migrations run from the `runtime` image via command override, so no dedicated migration target exists.

## Build Targets

|Target|Base Image|Purpose|
|---|---|---|
|`builder`|`golang:1.27.1-alpine`|Build the Go binary with `ldflags` (version/revision/build date)|
|`runtime`|`alpine:3.24`|Production runtime container (non-root `app` user); also runs migrations via command override|
|`tooling`|`golang:1.27.1-alpine`|Local development environment (hot reload + debugging)|

## runtime

- Runs as non-root user (`app`)
- Builds with `vendor` mode (`GOPROXY=off`)
- Embeds version / revision / build date via `-ldflags`
- Embeds `env/.env` and `database/migrations` into the binary. The `builder` stage materializes the target env via the `APP_ENV` build arg (`cp env/.env.${APP_ENV} env/.env`) before `go build`
- Default command: `./server serve`
- Migrations run from this same image via command override (`./server migrate-up`) as a one-shot job before deployment; no separate migration image is needed

## tooling (Local Development)

Pre-installed tools:

|Tool|Purpose|
|---|---|
|`air`|Hot reload|
|`dlv`|Go debugger|
|`golines`|Line-length-limited formatting|
|`gofumpt`|Enhanced gofmt|
|`golangci-lint`|Go linter|
|`lefthook`|Git hooks runner|

OS-level packages: `build-base`, `binutils-gold`, `bash`, `curl`, `git`, `upx`, `libc6-compat`, `gcompat`, `tzdata`, `make`

Default command: `air -c .air.toml`

## docker-compose Service

```yaml
api_server:
  dockerfile: docker/server/Dockerfile
  target: tooling
  ports: 8080 (API), 2345 (dlv), 6060 (pprof)
```

## Notes

- `tooling` target installs tools at the versions pinned in `mise.toml` (the version SSOT), keeping local and CI parity
- Working directory is `/app` for all targets
