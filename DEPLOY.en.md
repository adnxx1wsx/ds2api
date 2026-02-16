# DS2API Deployment Guide (Go)

Language: [中文](DEPLOY.md) | [English](DEPLOY.en.md)

This guide is aligned with the current Go codebase.

## Deployment Modes

- Local run: `go run ./cmd/ds2api`
- Docker: `docker-compose up -d`
- Vercel: serverless entry at `api/index.go`
- Linux service mode: systemd

## 0. Prerequisites

- Go 1.24+
- Node.js 20+ (only if you need to build WebUI locally)
- `config.json` or `DS2API_CONFIG_JSON`

## 1. Local Run

```bash
git clone https://github.com/CJackHwang/ds2api.git
cd ds2api

cp config.example.json config.json
# edit config.json

go run ./cmd/ds2api
```

Default port is `5001` (override with `PORT`).

Build WebUI if `/admin` reports missing assets:

```bash
./scripts/build-webui.sh

# Or rely on startup auto-build (enabled locally by default)
# DS2API_AUTO_BUILD_WEBUI=true go run ./cmd/ds2api
```

## 2. Docker Deployment

```bash
cp .env.example .env
# edit .env

docker-compose up -d
docker-compose logs -f
```

Rebuild after updates:

```bash
docker-compose up -d --build
```

Notes:

- `Dockerfile` uses multi-stage build (WebUI + Go binary)
- Container entry command is `/usr/local/bin/ds2api`

## 3. Vercel Deployment

- Serverless entry: `api/index.go`
- Rewrites and cache headers: `vercel.json`
- Build stage runs `npm ci --prefix webui && npm run build --prefix webui` automatically
- `vercel.json` routes `/admin/assets/*` and the `/admin` page to static output, while `/admin/*` APIs still go to `api/index`
- To mitigate Go Runtime streaming buffering, `/v1/chat/completions` on Vercel is routed to `api/chat-stream.js` (Node Runtime)
- `api/chat-stream.js` automatically falls back to the Go entry for non-stream requests or requests with `tools` (internal `__go=1`)
- `api/chat-stream.js` is data-path only (stream relay + SSE conversion); auth/account/session/PoW preparation still comes from an internal Go prepare endpoint (enabled on Vercel only)

Minimum environment variables:

- `DS2API_ADMIN_KEY`
- `DS2API_CONFIG_JSON` (raw JSON or Base64)

Optional:

- `VERCEL_TOKEN`
- `VERCEL_PROJECT_ID`
- `VERCEL_TEAM_ID`
- `DS2API_ACCOUNT_MAX_INFLIGHT` (per-account inflight limit, default `2`)
- `DS2API_ACCOUNT_CONCURRENCY` (alias of the same setting)
- `DS2API_ACCOUNT_MAX_QUEUE` (waiting queue limit, default=`recommended_concurrency`)
- `DS2API_ACCOUNT_QUEUE_SIZE` (alias of the same setting)
- `DS2API_VERCEL_INTERNAL_SECRET` (optional internal auth secret for Vercel hybrid streaming path; falls back to `DS2API_ADMIN_KEY` when unset)

Recommended concurrency is computed dynamically as `account_count * per_account_inflight_limit` (default is `account_count * 2`).
When inflight slots are full, requests are queued first; with default queue size, 429 typically starts around `account_count * 4`.

Notes:
- `static/admin` build output is not committed
- Vercel/Docker generate WebUI assets during build

After deploy, verify:

- `/healthz`
- `/v1/models`
- `/admin`

## 3.1 GitHub Release Automation

This repo includes `.github/workflows/release-artifacts.yml`:

- Triggers only on Release `published`
- Does not run on `push`
- Builds Linux/macOS/Windows archives and uploads them to Release Assets
- Generates `sha256sums.txt` for integrity checks

## 3.2 Vercel Build Troubleshooting

If you see an error like:

```text
Error: Command failed: go build -ldflags -s -w -o .../bootstrap .../main__vc__go__.go
```

it is usually caused by invalid Go build flag settings in Vercel
(`-ldflags` not passed as a single argument).

How to fix:

1. Open Vercel Project Settings -> Build and Development Settings
2. Clear custom Go Build Flags / Build Command (recommended)
3. If ldflags must be used, set `-ldflags=\"-s -w\"` so it is passed as one argument
4. Ensure `go.mod` uses a supported version (this repo uses `go 1.24`)
5. Redeploy (preferably with cache cleared)

Another common root cause (Go monorepo + `internal/`):

```text
... use of internal package ds2api/internal/server not allowed
```

This usually happens when the Vercel Go entrypoint imports `internal/...` directly.
This repo now avoids that by using a public bridge package: `api/index.go` -> `ds2api/app` -> `internal/server`.

If you see:

```text
No Output Directory named "public" found after the Build completed.
```

Vercel is validating frontend output against `public`. This repo builds WebUI into `static/admin`, and uses the parent directory `static` as Vercel output root.
`vercel.json` now explicitly sets:

```json
"outputDirectory": "static"
```

If you manually changed Output Directory in Project Settings, set it to `static` (or clear it and let repo config apply).

Vercel streaming note (important):

- Vercel Go Runtime applies platform-level buffering, so this repo uses a hybrid path on Vercel (`Go prepare + Node stream`) to restore real-time SSE behavior.
- This adaptation is Vercel-only; local and Docker remain pure Go.

## 4. Reverse Proxy (Nginx)

Disable buffering for SSE:

```nginx
location / {
    proxy_pass http://127.0.0.1:5001;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
    proxy_buffering off;
    proxy_cache off;
    chunked_transfer_encoding on;
    tcp_nodelay on;
}
```

## 5. systemd Example (Linux)

```ini
[Unit]
Description=DS2API (Go)
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/ds2api
Environment=PORT=5001
Environment=DS2API_CONFIG_PATH=/opt/ds2api/config.json
Environment=DS2API_ADMIN_KEY=admin
ExecStart=/opt/ds2api/ds2api
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Common commands:

```bash
sudo systemctl daemon-reload
sudo systemctl enable ds2api
sudo systemctl start ds2api
sudo systemctl status ds2api
```

## 6. Post-Deploy Checks

```bash
curl -s http://127.0.0.1:5001/healthz
curl -s http://127.0.0.1:5001/readyz
curl -s http://127.0.0.1:5001/v1/models
```

If admin UI is required:

```bash
curl -s http://127.0.0.1:5001/admin
```
