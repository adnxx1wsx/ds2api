# DS2API

[![License](https://img.shields.io/github/license/CJackHwang/ds2api.svg)](LICENSE)
![Stars](https://img.shields.io/github/stars/CJackHwang/ds2api.svg)
![Forks](https://img.shields.io/github/forks/CJackHwang/ds2api.svg)
[![Version](https://img.shields.io/badge/version-1.6.11-blue.svg)](version.txt)
[![Docker](https://img.shields.io/badge/docker-ready-blue.svg)](DEPLOY.en.md)

Language: [中文](README.MD) | [English](README.en.md)

DS2API converts DeepSeek Web chat capability into OpenAI-compatible and Claude-compatible APIs. The current repository is **Go backend only** with the existing React WebUI source in `webui/` and build output generated to `static/admin` during deployment.

## Implementation Boundary

- Backend: Go (`cmd/`, `api/`, `internal/`), no Python runtime
- Frontend: React admin panel (`webui/` source, static build served at runtime)
- Deployment: local run, Docker, Vercel serverless

## Key Capabilities

- OpenAI-compatible endpoints: `GET /v1/models`, `POST /v1/chat/completions`
- Claude-compatible endpoints: `GET /anthropic/v1/models`, `POST /anthropic/v1/messages`, `POST /anthropic/v1/messages/count_tokens`
- Multi-account rotation and automatic token refresh
- DeepSeek PoW solving via WASM
- Admin API: config management, account tests, import/export, Vercel sync
- WebUI SPA hosting at `/admin`
- Health probes: `GET /healthz`, `GET /readyz`

## Model Support

### OpenAI endpoint

| Model | thinking | search |
| --- | --- | --- |
| `deepseek-chat` | false | false |
| `deepseek-reasoner` | true | false |
| `deepseek-chat-search` | false | true |
| `deepseek-reasoner-search` | true | true |

### Claude endpoint

| Model | Default mapping |
| --- | --- |
| `claude-sonnet-4-20250514` | `deepseek-chat` |
| `claude-sonnet-4-20250514-fast` | `deepseek-chat` |
| `claude-sonnet-4-20250514-slow` | `deepseek-reasoner` |

You can override mapping via `claude_mapping` or `claude_model_mapping` in config.

## Quick Start

### 1) Local run

Requirement: Go 1.24+

```bash
git clone https://github.com/CJackHwang/ds2api.git
cd ds2api

cp config.example.json config.json
# edit config.json

go run ./cmd/ds2api
```

Default URL: `http://localhost:5001`

By default, local startup will auto-build WebUI when `static/admin` is missing (Node.js/npm required).
If you prefer manual build:

```bash
./scripts/build-webui.sh
```

### 2) Docker

```bash
cp .env.example .env
# edit .env

docker-compose up -d
docker-compose logs -f
```

### 3) Vercel

- Entrypoint: `api/index.go`
- Rewrites: `vercel.json`
- `vercel.json` runs `npm ci --prefix webui && npm run build --prefix webui` during build
- `/v1/chat/completions` is routed to `api/chat-stream.js` (Node Runtime) on Vercel to preserve real-time SSE
- `api/chat-stream.js` is data-path only; auth/account/session/PoW preparation still comes from an internal Go prepare endpoint
- Minimum env vars:
- `DS2API_ADMIN_KEY`
- `DS2API_CONFIG_JSON` (raw JSON or Base64)

Note: build artifacts under `static/admin` are not committed; Vercel generates them during build.

## Release Artifact Automation (GitHub Actions)

Built-in workflow: `.github/workflows/release-artifacts.yml`

- Trigger: only when a GitHub Release is `published`
- No build on normal `push`
- Outputs: multi-platform binaries (Linux/macOS/Windows) + `sha256sums.txt`
- Each archive includes:
- `ds2api` executable (`ds2api.exe` on Windows)
- `static/admin` (built WebUI assets)
- `sha3_wasm_bg.7b9ca65ddd.wasm`
- `config.example.json`, `.env.example`
- `README.MD`, `README.en.md`, `LICENSE`

Maintainer release flow:

1. Create and publish a GitHub Release (with tag, e.g. `v1.7.0`)
2. Wait for the `Release Artifacts` workflow to finish
3. Download the matching archive from Release Assets

Run from downloaded archive (Linux/macOS):

```bash
tar -xzf ds2api_v1.7.0_linux_amd64.tar.gz
cd ds2api_v1.7.0_linux_amd64
cp config.example.json config.json
./ds2api
```

## Configuration

### `config.json` example

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

### Core environment variables

| Variable | Purpose |
| --- | --- |
| `PORT` | Service port, default `5001` |
| `LOG_LEVEL` | `DEBUG/INFO/WARN/ERROR` |
| `DS2API_ACCOUNT_MAX_INFLIGHT` | Max in-flight requests per managed account, default `2` |
| `DS2API_ACCOUNT_CONCURRENCY` | Alias of the same setting (legacy compatibility) |
| `DS2API_ACCOUNT_MAX_QUEUE` | Waiting queue limit (managed-key mode), default=`recommended_concurrency` |
| `DS2API_ACCOUNT_QUEUE_SIZE` | Alias of the same setting (legacy compatibility) |
| `DS2API_ADMIN_KEY` | Admin login key, default `admin` |
| `DS2API_JWT_SECRET` | Admin JWT signing secret (optional) |
| `DS2API_JWT_EXPIRE_HOURS` | Admin JWT TTL in hours, default `24` |
| `DS2API_CONFIG_PATH` | Config file path, default `config.json` |
| `DS2API_CONFIG_JSON` | Inline config (JSON or Base64) |
| `DS2API_WASM_PATH` | PoW wasm path |
| `DS2API_STATIC_ADMIN_DIR` | Admin static assets dir |
| `DS2API_AUTO_BUILD_WEBUI` | Auto run npm build on startup when WebUI assets are missing (default: enabled locally, disabled on Vercel) |
| `DS2API_VERCEL_INTERNAL_SECRET` | Internal auth secret for Vercel hybrid streaming path (optional; falls back to `DS2API_ADMIN_KEY` if unset) |
| `VERCEL_TOKEN` | Vercel sync token (optional) |
| `VERCEL_PROJECT_ID` | Vercel project ID (optional) |
| `VERCEL_TEAM_ID` | Vercel team ID (optional) |

## Auth and Account Modes

For business endpoints (`/v1/*`, `/anthropic/*`), DS2API supports two modes:

1. Managed account mode: use a key from `config.keys` via `Authorization: Bearer ...` or `x-api-key`.
2. Direct token mode: if the incoming token is not in `config.keys`, DS2API treats it as a DeepSeek token directly.

Optional header: `X-Ds2-Target-Account` to pin one managed account.

## Recommended Concurrency

- DS2API computes recommended concurrency dynamically as: `account_count * per_account_inflight_limit`
- Default per-account inflight limit is `2`, so default recommendation is `account_count * 2`
- When inflight slots are full, requests enter a waiting queue instead of immediate 429
- Default queue limit equals `recommended_concurrency`, so default 429 threshold is about `account_count * 4`
- 429 is returned only after total load exceeds `inflight + waiting` capacity
- You can override per-account inflight via `DS2API_ACCOUNT_MAX_INFLIGHT` (or `DS2API_ACCOUNT_CONCURRENCY`)
- You can override waiting queue size via `DS2API_ACCOUNT_MAX_QUEUE` (or `DS2API_ACCOUNT_QUEUE_SIZE`)
- `GET /admin/queue/status` returns `max_inflight_per_account`, `recommended_concurrency`, `waiting`, and `max_queue_size`

## Tool Call Adaptation

Tool-call leakage is handled in the current implementation:

- With `tools` + `stream=true`, DS2API buffers text deltas first
- If a tool call is detected, DS2API returns structured `tool_calls` only
- If no tool call is detected, DS2API emits the buffered text once
- Parser supports mixed text, fenced JSON, and `function.arguments` payloads

## Docs and Testing

- API docs: `API.md` / `API.en.md`
- Deployment docs: `DEPLOY.md` / `DEPLOY.en.md`
- Contributing: `CONTRIBUTING.md` / `CONTRIBUTING.en.md`

```bash
go test ./...
```

## Disclaimer

This project is built through reverse engineering and is provided for learning and research only. Stability is not guaranteed. Do not use it in scenarios that violate terms of service or laws.
