# kvmrun-dashboard

A browser dashboard for [kvmrun](https://github.com/0xef53/kvmrun). It visualizes and manages virtual machines through the `kvmrund` gRPC API, exposing the same information and controls as the `vmm` CLI.

## Features

- Live data from the `kvmrund` daemon, with no local database
- Go + [Gin](https://gin-gonic.com) backend with server-rendered pages and a small JSON API
- gRPC clients generated from the `kvmrun` module, using [go-grpc](https://github.com/0xef53/go-grpc) request-ID and request-logging interceptors
- PAM authentication through the host PAM stack, with in-memory cookie sessions
- Machine list and detail pages, VM power controls, VNC and SSH consoles, task and daemon configuration information
- Embedded CSS and JS assets via `go:embed`, producing a single self-contained binary

## Architecture

The dashboard communicates directly with `kvmrund` using gRPC clients generated from the `kvmrun` module. It uses [go-grpc](https://github.com/0xef53/go-grpc) for request-ID and request-logging interceptors. Static frontend assets are embedded with `go:embed`, so the result is a single binary.

## Prerequisites

- Go 1.25 or later
- Linux with `libpam` and a C toolchain, because PAM authentication is implemented with cgo
- A running `kvmrund` daemon, reachable over the configured Unix socket or TCP address
- Optional: kvmrun TLS client certificates (`client.crt` and `client.key`) in the `--cert-dir` directory

## Build and Run

```sh
make build
./bin/dashboard --listen :8080
```

You can also use:

```sh
make run
make test
make vet
```

## Command-Line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--listen` | `:8080` | HTTP server address |
| `--daemon` | `unix:@/run/kvmrund.sock` | `kvmrund` daemon address (`host:port` or Unix/abstract socket) |
| `--cert-dir` | `/usr/share/kvmrun/tls` | Directory containing `client.crt` and `client.key` |
| `--pam-service` | `login` | PAM service used to verify passwords (`/etc/pam.d/<name>`) |
| `--session-ttl` | `12h` | Session lifetime |
| `--cookie-name` | `kvmrun-dashboard-session` | Session cookie name |
| `--debug` | `false` | Enable debug logging; can also be set with `KVMRUND_DEBUG` or `DEBUG` |

The dashboard uses the same default daemon connection as the `vmm` CLI. If no certificates are present in `--cert-dir`, it connects without TLS, matching `vmm` behavior.

## Authentication

- `GET/POST /login` renders a login form; passwords are verified with the host PAM stack using `--pam-service`.
- Successful login sets an `HttpOnly` cookie containing a 32-byte random session ID.
- Sessions are stored in memory, live for `--session-ttl`, and are removed on restart.
- All routes except `/login`, `/logout`, `/static/*`, and `/healthz` require a session. Browsers are redirected to `/login`; JSON API requests receive `401`.
