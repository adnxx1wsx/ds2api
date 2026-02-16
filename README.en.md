# DS2API

[![License](https://img.shields.io/github/license/CJackHwang/ds2api.svg)](LICENSE)
![Stars](https://img.shields.io/github/stars/CJackHwang/ds2api.svg)
![Forks](https://img.shields.io/github/forks/CJackHwang/ds2api.svg)
[![Version](https://img.shields.io/badge/version-1.6.11-blue.svg)](version.txt)
[![Docker](https://img.shields.io/badge/docker-ready-blue.svg)](DEPLOY.en.md)

Language: [中文](README.MD) | [English](README.en.md)

DS2API converts DeepSeek Web chat capability into OpenAI-compatible and Claude-compatible APIs. The backend is a **pure Go implementation**, with a React WebUI admin panel (source in `webui/`, build output auto-generated to `static/admin` during deployment).

## Architecture Overview

```text
┌──────────────┐    ┌──────────────────────────────────────┐
│   Clients    │    │             DS2API                    │
│  (OpenAI /   │───▶│  ┌────────┐  ┌──────────┐  ┌──────┐ │
│   Claude     │    │  │Auth MW  │─▶│Adapter   │─▶│DeepSeek│
│   compat)    │    │  └────────┘  │OpenAI/   │  │Client │ │
│              │◀───│              │Claude    │  └──────┘ │
│              │    │  ┌────────┐  └──────────┘            │
│              │    │  │Admin API│  ┌──────────┐           │
│              │    │  └────────┘  │Account   │           │
│              │    │  ┌────────┐  │Pool/Queue│           │
│              │    │  │WebUI   │  └──────────┘           │
│              │    │  │(/admin)│  ┌──────────┐           │
│              │    │  └────────┘  │PoW WASM  │           │
└──────────────┘    └──────────────────────────────────────┘
```

- **Backend**: Go (`cmd/ds2api/`, `api/`, `internal/`), no Python runtime
- **Frontend**: React admin panel (`webui/`), served as static build at runtime
- **Deployment**: local run, Docker, Vercel serverless, Linux systemd

## Key Capabilities

| Capability | Details |
| --- | --- |
| OpenAI compatible | `GET /v1/models`, `POST /v1/chat/completions` (stream/non-stream) |
| Claude compatible | `GET /anthropic/v1/models`, `POST /anthropic/v1/messages`, `POST /anthropic/v1/messages/count_tokens` |
| Multi-account rotation | Auto token refresh, email/mobile dual login |
| Concurrency control | Per-account in-flight limit + waiting queue, dynamic recommended concurrency |
| DeepSeek PoW | WASM solving via `wazero`, no external Node.js dependency |
| Tool Calling | Anti-leak handling: auto buffer, detect, structured output |
| Admin API | Config management, account testing/batch test, import/export, Vercel sync |
| WebUI Admin Panel | SPA at `/admin` (bilingual Chinese/English, dark mode) |
| Health Probes | `GET /healthz` (liveness), `GET /readyz` (readiness) |

## Model Support

### OpenAI Endpoint

| Model | thinking | search |
| --- | --- | --- |
| `deepseek-chat` | ❌ | ❌ |
| `deepseek-reasoner` | ✅ | ❌ |
| `deepseek-chat-search` | ❌ | ✅ |
| `deepseek-reasoner-search` | ✅ | ✅ |

### Claude Endpoint

| Model | Default Mapping |
| --- | --- |
| `claude-sonnet-4-20250514` | `deepseek-chat` |
| `claude-sonnet-4-20250514-fast` | `deepseek-chat` |
| `claude-sonnet-4-20250514-slow` | `deepseek-reasoner` |

Override mapping via `claude_mapping` or `claude_model_mapping` in config.

## Quick Start

### Option 1: Local Run

**Prerequisites**: Go 1.24+, Node.js 20+ (only if building WebUI locally)

```bash
# 1. Clone
git clone https://github.com/CJackHwang/ds2api.git
cd ds2api

# 2. Configure
cp config.example.json config.json
# Edit config.json with your DeepSeek account info and API keys

# 3. Start
go run ./cmd/ds2api
```

Default URL: `http://localhost:5001`

> **WebUI auto-build**: On first local startup, if `static/admin` is missing, DS2API will auto-run `npm install && npm run build` (requires Node.js). You can also build manually: `./scripts/build-webui.sh`

### Option 2: Docker

```bash
# 1. Configure environment
cp .env.example .env
# Edit .env

# 2. Start
docker-compose up -d

# 3. View logs
docker-compose logs -f
```

Rebuild after updates: `docker-compose up -d --build`

### Option 3: Vercel

1. Fork this repo to your GitHub account
2. Import the project on Vercel
3. Set environment variables (minimum: `DS2API_ADMIN_KEY` and `DS2API_CONFIG_JSON`)
4. Deploy

> **Streaming note**: `/v1/chat/completions` on Vercel is routed to `api/chat-stream.js` (Node Runtime) for real-time SSE. Auth, account selection, session/PoW preparation are still handled by the Go internal prepare endpoint; Node only relays stream data.

For detailed deployment instructions, see the [Deployment Guide](DEPLOY.en.md).

### Option 4: Download Release Binaries

GitHub Actions automatically builds multi-platform archives on each Release:

```bash
# After downloading the archive for your platform
tar -xzf ds2api_v1.7.0_linux_amd64.tar.gz
cd ds2api_v1.7.0_linux_amd64
cp config.example.json config.json
# Edit config.json
./ds2api
```

## Configuration

### `config.json` Example

```json
{
  "keys": ["your-api-key-1", "your-api-key-2"],
  "accounts": [
    {
      "email": "user@example.com",
      "password": "your-password",
      "token": ""
    },
    {
      "mobile": "12345678901",
      "password": "your-password",
      "token": ""
    }
  ],
  "claude_model_mapping": {
    "fast": "deepseek-chat",
    "slow": "deepseek-reasoner"
  }
}
```

- `keys`: API access keys; clients authenticate via `Authorization: Bearer <key>`
- `accounts`: DeepSeek account list, supports `email` or `mobile` login
- `token`: Leave empty for auto-login on first request; or pre-fill an existing token
- `claude_model_mapping`: Maps `fast`/`slow` suffixes to corresponding DeepSeek models

### Environment Variables

| Variable | Purpose | Default |
| --- | --- | --- |
| `PORT` | Service port | `5001` |
| `LOG_LEVEL` | Log level | `INFO` (`DEBUG`/`WARN`/`ERROR`) |
| `DS2API_ADMIN_KEY` | Admin login key | `admin` |
| `DS2API_JWT_SECRET` | Admin JWT signing secret | Same as `DS2API_ADMIN_KEY` |
| `DS2API_JWT_EXPIRE_HOURS` | Admin JWT TTL in hours | `24` |
| `DS2API_CONFIG_PATH` | Config file path | `config.json` |
| `DS2API_CONFIG_JSON` | Inline config (JSON or Base64) | — |
| `DS2API_WASM_PATH` | PoW WASM file path | Auto-detect |
| `DS2API_STATIC_ADMIN_DIR` | Admin static assets dir | `static/admin` |
| `DS2API_AUTO_BUILD_WEBUI` | Auto-build WebUI on startup | Enabled locally, disabled on Vercel |
| `DS2API_ACCOUNT_MAX_INFLIGHT` | Max in-flight requests per account | `2` |
| `DS2API_ACCOUNT_CONCURRENCY` | Alias (legacy compat) | — |
| `DS2API_ACCOUNT_MAX_QUEUE` | Waiting queue limit | `recommended_concurrency` |
| `DS2API_ACCOUNT_QUEUE_SIZE` | Alias (legacy compat) | — |
| `DS2API_VERCEL_INTERNAL_SECRET` | Vercel hybrid streaming internal auth | Falls back to `DS2API_ADMIN_KEY` |
| `DS2API_VERCEL_STREAM_LEASE_TTL_SECONDS` | Stream lease TTL seconds | `900` |
| `VERCEL_TOKEN` | Vercel sync token | — |
| `VERCEL_PROJECT_ID` | Vercel project ID | — |
| `VERCEL_TEAM_ID` | Vercel team ID | — |

## Authentication Modes

For business endpoints (`/v1/*`, `/anthropic/*`), DS2API supports two modes:

| Mode | Description |
| --- | --- |
| **Managed account** | Use a key from `config.keys` via `Authorization: Bearer ...` or `x-api-key`; DS2API auto-selects an account |
| **Direct token** | If the token is not in `config.keys`, DS2API treats it as a DeepSeek token directly |

Optional header `X-Ds2-Target-Account`: Pin a specific managed account (value is email or mobile).

## Concurrency Model

```
Per-account inflight = DS2API_ACCOUNT_MAX_INFLIGHT (default 2)
Recommended concurrency = account_count × per_account_inflight
Queue limit = DS2API_ACCOUNT_MAX_QUEUE (default = recommended concurrency)
429 threshold = inflight + queue ≈ account_count × 4
```

- When inflight slots are full, requests enter a waiting queue — **no immediate 429**
- 429 is returned only when total load exceeds inflight + queue capacity
- `GET /admin/queue/status` returns real-time concurrency state

## Tool Call Adaptation

When `tools` is present in the request, DS2API performs anti-leak handling:

1. With `stream=true`, DS2API **buffers** text deltas first
2. If a tool call is detected → only structured `tool_calls` are emitted, raw JSON is not leaked
3. If no tool call → buffered text is emitted at once
4. Parser supports mixed text, fenced JSON, and `function.arguments` payloads

## Project Structure

```text
ds2api/
├── cmd/
│   ├── ds2api/              # Local / container entrypoint
│   └── ds2api-tests/        # End-to-end testsuite entrypoint
├── api/
│   ├── index.go             # Vercel Serverless Go entry
│   ├── chat-stream.js       # Vercel Node.js stream relay
│   └── helpers/             # Node.js helper modules
├── internal/
│   ├── account/             # Account pool and concurrency queue
│   ├── adapter/
│   │   ├── openai/          # OpenAI adapter (incl. tool call parsing, Vercel stream prepare/release)
│   │   └── claude/          # Claude adapter
│   ├── admin/               # Admin API handlers
│   ├── auth/                # Auth and JWT
│   ├── config/              # Config loading and hot-reload
│   ├── deepseek/            # DeepSeek API client, PoW WASM
│   ├── server/              # HTTP routing and middleware (chi router)
│   ├── sse/                 # SSE parsing utilities
│   ├── util/                # Common utilities
│   └── webui/               # WebUI static file serving and auto-build
├── webui/                   # React WebUI source (Vite + Tailwind)
│   └── src/
│       ├── components/      # AccountManager / ApiTester / BatchImport / VercelSync / Login / LandingPage
│       └── locales/         # Language packs (zh.json / en.json)
├── scripts/
│   ├── build-webui.sh       # Manual WebUI build script
│   └── testsuite/           # Testsuite runner scripts
├── static/admin/            # WebUI build output (not committed to Git)
├── .github/
│   ├── workflows/           # GitHub Actions (Release artifact automation)
│   ├── ISSUE_TEMPLATE/      # Issue templates
│   └── PULL_REQUEST_TEMPLATE.md
├── config.example.json      # Config file template
├── .env.example             # Environment variable template
├── Dockerfile               # Multi-stage build (WebUI + Go)
├── docker-compose.yml       # Production Docker Compose
├── docker-compose.dev.yml   # Development Docker Compose
├── vercel.json              # Vercel routing and build config
├── go.mod / go.sum          # Go module dependencies
└── version.txt              # Version number
```

## Documentation Index

| Document | Description |
| --- | --- |
| [API.md](API.md) / [API.en.md](API.en.md) | API reference with request/response examples |
| [DEPLOY.md](DEPLOY.md) / [DEPLOY.en.md](DEPLOY.en.md) | Deployment guide (local/Docker/Vercel/systemd) |
| [CONTRIBUTING.md](CONTRIBUTING.md) / [CONTRIBUTING.en.md](CONTRIBUTING.en.md) | Contributing guide |
| [TESTING.md](TESTING.md) | Testsuite guide |

## Testing

```bash
# Unit tests
go test ./...

# One-command live end-to-end tests (real accounts, full request/response logs)
./scripts/testsuite/run-live.sh

# Or with custom flags
go run ./cmd/ds2api-tests \
  --config config.json \
  --admin-key admin \
  --out artifacts/testsuite \
  --timeout 120 \
  --retries 2
```

## Release Artifact Automation (GitHub Actions)

Workflow: `.github/workflows/release-artifacts.yml`

- **Trigger**: only on GitHub Release `published` (normal pushes do not trigger builds)
- **Outputs**: multi-platform archives (`linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`) + `sha256sums.txt`
- **Each archive includes**: `ds2api` executable, `static/admin`, WASM file, config template, README, LICENSE

## Disclaimer

This project is built through reverse engineering and is provided for learning and research only. Stability is not guaranteed. Do not use it in scenarios that violate terms of service or laws.
