# DS2API API Reference

Language: [中文](API.md) | [English](API.en.md)

This document describes the actual behavior of the current Go codebase.

---

## Table of Contents

- [Basics](#basics)
- [Authentication](#authentication)
- [Route Index](#route-index)
- [Health Endpoints](#health-endpoints)
- [OpenAI-Compatible API](#openai-compatible-api)
- [Claude-Compatible API](#claude-compatible-api)
- [Admin API](#admin-api)
- [Error Payloads](#error-payloads)
- [cURL Examples](#curl-examples)

---

## Basics

| Item | Details |
| --- | --- |
| Base URL | `http://localhost:5001` or your deployment domain |
| Default Content-Type | `application/json` |
| Health probes | `GET /healthz`, `GET /readyz` |
| CORS | Enabled (`Access-Control-Allow-Origin: *`) |

---

## Authentication

### Business Endpoints (`/v1/*`, `/anthropic/*`)

Two header formats accepted:

| Method | Example |
| --- | --- |
| Bearer Token | `Authorization: Bearer <token>` |
| API Key Header | `x-api-key: <token>` (no `Bearer` prefix) |

**Auth behavior**:

- Token is in `config.keys` → **Managed account mode**: DS2API auto-selects an account via rotation
- Token is not in `config.keys` → **Direct token mode**: treated as a DeepSeek token directly

**Optional header**: `X-Ds2-Target-Account: <email_or_mobile>` — Pin a specific managed account.

### Admin Endpoints (`/admin/*`)

| Endpoint | Auth |
| --- | --- |
| `POST /admin/login` | Public |
| `GET /admin/verify` | `Authorization: Bearer <jwt>` (JWT only) |
| Other `/admin/*` | `Authorization: Bearer <jwt>` or `Authorization: Bearer <admin_key>` |

---

## Route Index

| Method | Path | Auth | Description |
| --- | --- | --- | --- |
| GET | `/healthz` | None | Liveness probe |
| GET | `/readyz` | None | Readiness probe |
| GET | `/v1/models` | None | OpenAI model list |
| POST | `/v1/chat/completions` | Business | OpenAI chat completions |
| GET | `/anthropic/v1/models` | None | Claude model list |
| POST | `/anthropic/v1/messages` | Business | Claude messages |
| POST | `/anthropic/v1/messages/count_tokens` | Business | Claude token counting |
| POST | `/admin/login` | None | Admin login |
| GET | `/admin/verify` | JWT | Verify admin JWT |
| GET | `/admin/vercel/config` | Admin | Read preconfigured Vercel creds |
| GET | `/admin/config` | Admin | Read sanitized config |
| POST | `/admin/config` | Admin | Update config |
| POST | `/admin/keys` | Admin | Add API key |
| DELETE | `/admin/keys/{key}` | Admin | Delete API key |
| GET | `/admin/accounts` | Admin | Paginated account list |
| POST | `/admin/accounts` | Admin | Add account |
| DELETE | `/admin/accounts/{identifier}` | Admin | Delete account |
| GET | `/admin/queue/status` | Admin | Account queue status |
| POST | `/admin/accounts/test` | Admin | Test one account |
| POST | `/admin/accounts/test-all` | Admin | Test all accounts |
| POST | `/admin/import` | Admin | Batch import keys/accounts |
| POST | `/admin/test` | Admin | Test API through service |
| POST | `/admin/vercel/sync` | Admin | Sync config to Vercel |
| GET | `/admin/vercel/status` | Admin | Vercel sync status |
| GET | `/admin/export` | Admin | Export config JSON/Base64 |

---

## Health Endpoints

### `GET /healthz`

```json
{"status": "ok"}
```

### `GET /readyz`

```json
{"status": "ready"}
```

---

## OpenAI-Compatible API

### `GET /v1/models`

No auth required. Returns supported models.

**Response**:

```json
{
  "object": "list",
  "data": [
    {"id": "deepseek-chat", "object": "model", "created": 1677610602, "owned_by": "deepseek", "permission": []},
    {"id": "deepseek-reasoner", "object": "model", "created": 1677610602, "owned_by": "deepseek", "permission": []},
    {"id": "deepseek-chat-search", "object": "model", "created": 1677610602, "owned_by": "deepseek", "permission": []},
    {"id": "deepseek-reasoner-search", "object": "model", "created": 1677610602, "owned_by": "deepseek", "permission": []}
  ]
}
```

### `POST /v1/chat/completions`

**Headers**:

```http
Authorization: Bearer your-api-key
Content-Type: application/json
```

**Request body**:

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `model` | string | ✅ | `deepseek-chat` / `deepseek-reasoner` / `deepseek-chat-search` / `deepseek-reasoner-search` |
| `messages` | array | ✅ | OpenAI-style messages |
| `stream` | boolean | ❌ | Default `false` |
| `tools` | array | ❌ | Function calling schema |
| `temperature`, etc. | any | ❌ | Accepted but final behavior depends on upstream |

#### Non-Stream Response

```json
{
  "id": "<chat_session_id>",
  "object": "chat.completion",
  "created": 1738400000,
  "model": "deepseek-reasoner",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "final response",
        "reasoning_content": "reasoning trace (reasoner models)"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 10,
    "completion_tokens": 20,
    "total_tokens": 30,
    "completion_tokens_details": {
      "reasoning_tokens": 5
    }
  }
}
```

#### Streaming (`stream=true`)

SSE format: each frame is `data: <json>\n\n`, terminated by `data: [DONE]`.

```text
data: {"id":"...","object":"chat.completion.chunk","choices":[{"delta":{"role":"assistant"},"index":0}]}

data: {"id":"...","object":"chat.completion.chunk","choices":[{"delta":{"reasoning_content":"..."},"index":0}]}

data: {"id":"...","object":"chat.completion.chunk","choices":[{"delta":{"content":"..."},"index":0}]}

data: {"id":"...","object":"chat.completion.chunk","choices":[{"delta":{},"index":0,"finish_reason":"stop"}],"usage":{...}}

data: [DONE]
```

**Field notes**:

- First delta includes `role: assistant`
- `deepseek-reasoner` / `deepseek-reasoner-search` models emit `delta.reasoning_content`
- Text emits `delta.content`
- Last chunk includes `finish_reason` and `usage`

#### Tool Calls

When `tools` is present, DS2API performs anti-leak handling:

**Non-stream**: If detected, returns `message.tool_calls`, `finish_reason=tool_calls`, `message.content=null`.

```json
{
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": null,
        "tool_calls": [
          {
            "id": "call_xxx",
            "type": "function",
            "function": {
              "name": "get_weather",
              "arguments": "{\"city\":\"beijing\"}"
            }
          }
        ]
      },
      "finish_reason": "tool_calls"
    }
  ]
}
```

**Stream**: DS2API buffers text first. If tool call detected → only structured `delta.tool_calls` (each with `index`); otherwise emits buffered text at once.

---

## Claude-Compatible API

### `GET /anthropic/v1/models`

No auth required.

**Response**:

```json
{
  "object": "list",
  "data": [
    {"id": "claude-sonnet-4-20250514", "object": "model", "created": 1715635200, "owned_by": "anthropic"},
    {"id": "claude-sonnet-4-20250514-fast", "object": "model", "created": 1715635200, "owned_by": "anthropic"},
    {"id": "claude-sonnet-4-20250514-slow", "object": "model", "created": 1715635200, "owned_by": "anthropic"}
  ]
}
```

### `POST /anthropic/v1/messages`

**Headers**:

```http
x-api-key: your-api-key
Content-Type: application/json
anthropic-version: 2023-06-01
```

**Request body**:

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `model` | string | ✅ | `claude-sonnet-4-20250514` / `-fast` / `-slow` |
| `messages` | array | ✅ | Claude-style messages |
| `max_tokens` | number | ❌ | Not strictly enforced by upstream bridge |
| `stream` | boolean | ❌ | Default `false` |
| `system` | string | ❌ | Optional system prompt |
| `tools` | array | ❌ | Claude tool schema |

#### Non-Stream Response

```json
{
  "id": "msg_1738400000000000000",
  "type": "message",
  "role": "assistant",
  "model": "claude-sonnet-4-20250514",
  "content": [
    {"type": "text", "text": "response"}
  ],
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "usage": {
    "input_tokens": 12,
    "output_tokens": 34
  }
}
```

If tool use is detected, `stop_reason` becomes `tool_use` and `content` contains `tool_use` blocks.

#### Streaming (`stream=true`)

SSE uses paired `event:` + `data:` lines. Event type is also in JSON `type`.

```text
event: message_start
data: {"type":"message_start","message":{...}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}

event: ping
data: {"type":"ping"}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":12}}

event: message_stop
data: {"type":"message_stop"}
```

**Notes**:

- Thinking-enabled models (`-slow`) stream `thinking_delta`
- `signature_delta` is not emitted (DeepSeek does not provide verifiable thinking signatures)
- In `tools` mode, the stream avoids leaking raw tool JSON and does not force `input_json_delta`

### `POST /anthropic/v1/messages/count_tokens`

**Request**:

```json
{
  "model": "claude-sonnet-4-20250514",
  "messages": [
    {"role": "user", "content": "Hello"}
  ]
}
```

**Response**:

```json
{
  "input_tokens": 5
}
```

---

## Admin API

### `POST /admin/login`

Public endpoint.

**Request**:

```json
{
  "admin_key": "admin",
  "expire_hours": 24
}
```

`expire_hours` is optional, default `24`.

**Response**:

```json
{
  "success": true,
  "token": "<jwt>",
  "expires_in": 86400
}
```

### `GET /admin/verify`

Requires JWT: `Authorization: Bearer <jwt>`

**Response**:

```json
{
  "valid": true,
  "expires_at": 1738400000,
  "remaining_seconds": 72000
}
```

### `GET /admin/vercel/config`

Returns Vercel preconfiguration status.

```json
{
  "has_token": true,
  "project_id": "prj_xxx",
  "team_id": null
}
```

### `GET /admin/config`

Returns sanitized config.

```json
{
  "keys": ["k1", "k2"],
  "accounts": [
    {
      "email": "user@example.com",
      "mobile": "",
      "has_password": true,
      "has_token": true,
      "token_preview": "abcde..."
    }
  ],
  "claude_mapping": {
    "fast": "deepseek-chat",
    "slow": "deepseek-reasoner"
  }
}
```

### `POST /admin/config`

Updatable fields: `keys`, `accounts`, `claude_mapping`.

**Request**:

```json
{
  "keys": ["k1", "k2"],
  "accounts": [
    {"email": "user@example.com", "password": "pwd", "token": ""}
  ],
  "claude_mapping": {
    "fast": "deepseek-chat",
    "slow": "deepseek-reasoner"
  }
}
```

### `POST /admin/keys`

```json
{"key": "new-api-key"}
```

**Response**: `{"success": true, "total_keys": 3}`

### `DELETE /admin/keys/{key}`

**Response**: `{"success": true, "total_keys": 2}`

### `GET /admin/accounts`

**Query params**:

| Param | Default | Range |
| --- | --- | --- |
| `page` | `1` | ≥ 1 |
| `page_size` | `10` | 1–100 |

**Response**:

```json
{
  "items": [
    {
      "email": "user@example.com",
      "mobile": "",
      "has_password": true,
      "has_token": true,
      "token_preview": "abc..."
    }
  ],
  "total": 25,
  "page": 1,
  "page_size": 10,
  "total_pages": 3
}
```

### `POST /admin/accounts`

```json
{"email": "user@example.com", "password": "pwd"}
```

**Response**: `{"success": true, "total_accounts": 6}`

### `DELETE /admin/accounts/{identifier}`

`identifier` is email or mobile.

**Response**: `{"success": true, "total_accounts": 5}`

### `GET /admin/queue/status`

```json
{
  "available": 3,
  "in_use": 1,
  "total": 4,
  "available_accounts": ["a@example.com"],
  "in_use_accounts": ["b@example.com"],
  "max_inflight_per_account": 2,
  "recommended_concurrency": 8
}
```

| Field | Description |
| --- | --- |
| `available` | Currently available accounts |
| `in_use` | Currently in-use accounts |
| `total` | Total accounts |
| `max_inflight_per_account` | Per-account inflight limit |
| `recommended_concurrency` | Suggested concurrency (`total × max_inflight_per_account`) |

### `POST /admin/accounts/test`

| Field | Required | Notes |
| --- | --- | --- |
| `identifier` | ✅ | email or mobile |
| `model` | ❌ | default `deepseek-chat` |
| `message` | ❌ | if empty, only session creation is tested |

**Response**:

```json
{
  "account": "user@example.com",
  "success": true,
  "response_time": 1240,
  "message": "API test successful (session creation only)",
  "model": "deepseek-chat"
}
```

### `POST /admin/accounts/test-all`

Optional request field: `model`.

```json
{
  "total": 5,
  "success": 4,
  "failed": 1,
  "results": [...]
}
```

### `POST /admin/import`

Batch import keys and accounts.

**Request**:

```json
{
  "keys": ["k1", "k2"],
  "accounts": [
    {"email": "user@example.com", "password": "pwd", "token": ""}
  ]
}
```

**Response**:

```json
{
  "success": true,
  "imported_keys": 2,
  "imported_accounts": 1
}
```

### `POST /admin/test`

Test API availability through the service itself.

| Field | Required | Default |
| --- | --- | --- |
| `model` | ❌ | `deepseek-chat` |
| `message` | ❌ | `你好` |
| `api_key` | ❌ | First key in config |

**Response**:

```json
{
  "success": true,
  "status_code": 200,
  "response": {"id": "..."}
}
```

### `POST /admin/vercel/sync`

| Field | Required | Notes |
| --- | --- | --- |
| `vercel_token` | ❌ | If empty or `__USE_PRECONFIG__`, read env |
| `project_id` | ❌ | Fallback: `VERCEL_PROJECT_ID` |
| `team_id` | ❌ | Fallback: `VERCEL_TEAM_ID` |
| `auto_validate` | ❌ | Default `true` |
| `save_credentials` | ❌ | Default `true` |

**Success response**:

```json
{
  "success": true,
  "validated_accounts": 3,
  "message": "Config synced, redeploying...",
  "deployment_url": "https://..."
}
```

Or manual deploy required:

```json
{
  "success": true,
  "validated_accounts": 3,
  "message": "Config synced to Vercel, please trigger redeploy manually",
  "manual_deploy_required": true
}
```

### `GET /admin/vercel/status`

```json
{
  "synced": true,
  "last_sync_time": 1738400000,
  "has_synced_before": true
}
```

### `GET /admin/export`

```json
{
  "json": "{...}",
  "base64": "ey4uLn0="
}
```

---

## Error Payloads

Error formats vary by module:

| Module | Format |
| --- | --- |
| OpenAI routes | `{"error": {"message": "...", "type": "..."}}` |
| Claude routes | `{"error": {"type": "...", "message": "..."}}` |
| Admin routes | `{"detail": "..."}` |

Clients should handle HTTP status code plus `error` / `detail` fields.

**Common status codes**:

| Code | Meaning |
| --- | --- |
| `401` | Authentication failed (invalid key/token, or expired admin JWT) |
| `429` | Too many requests (exceeded inflight + queue capacity) |
| `503` | Model unavailable or upstream error |

---

## cURL Examples

### OpenAI Non-Stream

```bash
curl http://localhost:5001/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat",
    "messages": [{"role": "user", "content": "Hello"}],
    "stream": false
  }'
```

### OpenAI Stream

```bash
curl http://localhost:5001/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-reasoner",
    "messages": [{"role": "user", "content": "Explain quantum entanglement"}],
    "stream": true
  }'
```

### OpenAI with Search

```bash
curl http://localhost:5001/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat-search",
    "messages": [{"role": "user", "content": "Latest news today"}],
    "stream": true
  }'
```

### OpenAI Tool Calling

```bash
curl http://localhost:5001/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat",
    "messages": [{"role": "user", "content": "What is the weather in Beijing?"}],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "get_weather",
          "description": "Get weather for a city",
          "parameters": {
            "type": "object",
            "properties": {
              "city": {"type": "string", "description": "City name"}
            },
            "required": ["city"]
          }
        }
      }
    ]
  }'
```

### Claude Non-Stream

```bash
curl http://localhost:5001/anthropic/v1/messages \
  -H "x-api-key: your-api-key" \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-sonnet-4-20250514",
    "max_tokens": 1024,
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```

### Claude Stream

```bash
curl http://localhost:5001/anthropic/v1/messages \
  -H "x-api-key: your-api-key" \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-sonnet-4-20250514-slow",
    "max_tokens": 1024,
    "messages": [{"role": "user", "content": "Explain relativity"}],
    "stream": true
  }'
```

### Admin Login

```bash
curl http://localhost:5001/admin/login \
  -H "Content-Type: application/json" \
  -d '{"admin_key": "admin"}'
```

### Pin Specific Account

```bash
curl http://localhost:5001/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "X-Ds2-Target-Account: user@example.com" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat",
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```
