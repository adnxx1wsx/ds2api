# DS2API 接口文档

语言 / Language: [中文](API.md) | [English](API.en.md)

本文档描述当前 Go 代码库的实际 API 行为。

---

## 目录

- [基础信息](#基础信息)
- [鉴权规则](#鉴权规则)
- [路由总览](#路由总览)
- [健康检查](#健康检查)
- [OpenAI 兼容接口](#openai-兼容接口)
- [Claude 兼容接口](#claude-兼容接口)
- [Admin 接口](#admin-接口)
- [错误响应格式](#错误响应格式)
- [cURL 示例](#curl-示例)

---

## 基础信息

| 项目 | 说明 |
| --- | --- |
| Base URL | `http://localhost:5001` 或你的部署域名 |
| 默认 Content-Type | `application/json` |
| 健康检查 | `GET /healthz`、`GET /readyz` |
| CORS | 已启用（`Access-Control-Allow-Origin: *`） |

---

## 鉴权规则

### 业务接口（`/v1/*`、`/anthropic/*`）

支持两种传参方式：

| 方式 | 示例 |
| --- | --- |
| Bearer Token | `Authorization: Bearer <token>` |
| API Key Header | `x-api-key: <token>`（无 `Bearer` 前缀） |

**鉴权行为**：

- token 在 `config.keys` 中 → **托管账号模式**，自动轮询选择账号
- token 不在 `config.keys` 中 → **直通 token 模式**，直接作为 DeepSeek token 使用

**可选请求头**：`X-Ds2-Target-Account: <email_or_mobile>` — 指定使用某个托管账号。

### Admin 接口（`/admin/*`）

| 端点 | 鉴权 |
| --- | --- |
| `POST /admin/login` | 无需鉴权 |
| `GET /admin/verify` | `Authorization: Bearer <jwt>`（仅 JWT） |
| 其他 `/admin/*` | `Authorization: Bearer <jwt>` 或 `Authorization: Bearer <admin_key>`（直传管理密钥） |

---

## 路由总览

| 方法 | 路径 | 鉴权 | 说明 |
| --- | --- | --- | --- |
| GET | `/healthz` | 无 | 存活探针 |
| GET | `/readyz` | 无 | 就绪探针 |
| GET | `/v1/models` | 无 | OpenAI 模型列表 |
| POST | `/v1/chat/completions` | 业务 | OpenAI 对话补全 |
| GET | `/anthropic/v1/models` | 无 | Claude 模型列表 |
| POST | `/anthropic/v1/messages` | 业务 | Claude 消息接口 |
| POST | `/anthropic/v1/messages/count_tokens` | 业务 | Claude token 计数 |
| POST | `/admin/login` | 无 | 管理登录 |
| GET | `/admin/verify` | JWT | 校验管理 JWT |
| GET | `/admin/vercel/config` | Admin | 读取 Vercel 预配置 |
| GET | `/admin/config` | Admin | 读取配置（脱敏） |
| POST | `/admin/config` | Admin | 更新配置 |
| POST | `/admin/keys` | Admin | 添加 API key |
| DELETE | `/admin/keys/{key}` | Admin | 删除 API key |
| GET | `/admin/accounts` | Admin | 分页账号列表 |
| POST | `/admin/accounts` | Admin | 添加账号 |
| DELETE | `/admin/accounts/{identifier}` | Admin | 删除账号 |
| GET | `/admin/queue/status` | Admin | 账号队列状态 |
| POST | `/admin/accounts/test` | Admin | 测试单个账号 |
| POST | `/admin/accounts/test-all` | Admin | 测试全部账号 |
| POST | `/admin/import` | Admin | 批量导入 keys/accounts |
| POST | `/admin/test` | Admin | 测试当前 API 可用性 |
| POST | `/admin/vercel/sync` | Admin | 同步配置到 Vercel |
| GET | `/admin/vercel/status` | Admin | Vercel 同步状态 |
| GET | `/admin/export` | Admin | 导出配置 JSON/Base64 |

---

## 健康检查

### `GET /healthz`

```json
{"status": "ok"}
```

### `GET /readyz`

```json
{"status": "ready"}
```

---

## OpenAI 兼容接口

### `GET /v1/models`

无需鉴权。返回当前支持的模型列表。

**响应示例**：

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

**请求头**：

```http
Authorization: Bearer your-api-key
Content-Type: application/json
```

**请求体**：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | ✅ | `deepseek-chat` / `deepseek-reasoner` / `deepseek-chat-search` / `deepseek-reasoner-search` |
| `messages` | array | ✅ | OpenAI 风格消息数组 |
| `stream` | boolean | ❌ | 默认 `false` |
| `tools` | array | ❌ | Function Calling 定义 |
| `temperature` 等 | any | ❌ | 兼容透传字段（最终效果由上游决定） |

#### 非流式响应

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
        "content": "最终回复",
        "reasoning_content": "思考内容（reasoner 模型）"
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

#### 流式响应（`stream=true`）

SSE 格式：每段为 `data: <json>\n\n`，结束为 `data: [DONE]`。

```text
data: {"id":"...","object":"chat.completion.chunk","choices":[{"delta":{"role":"assistant"},"index":0}]}

data: {"id":"...","object":"chat.completion.chunk","choices":[{"delta":{"reasoning_content":"..."},"index":0}]}

data: {"id":"...","object":"chat.completion.chunk","choices":[{"delta":{"content":"..."},"index":0}]}

data: {"id":"...","object":"chat.completion.chunk","choices":[{"delta":{},"index":0,"finish_reason":"stop"}],"usage":{...}}

data: [DONE]
```

**字段说明**：

- 首个 delta 包含 `role: assistant`
- `deepseek-reasoner` / `deepseek-reasoner-search` 模型输出 `delta.reasoning_content`
- 普通文本输出 `delta.content`
- 最后一段包含 `finish_reason` 和 `usage`

#### Tool Calls

当请求中含 `tools` 时，DS2API 做防泄漏处理：

**非流式**：识别到工具调用时，返回 `message.tool_calls`，设置 `finish_reason=tool_calls`，`message.content=null`。

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

**流式**：先缓冲正文片段。识别到工具调用 → 仅输出结构化 `delta.tool_calls`（每个 tool call 带 `index`）；否则一次性输出普通文本。

---

## Claude 兼容接口

### `GET /anthropic/v1/models`

无需鉴权。

**响应示例**：

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

**请求头**：

```http
x-api-key: your-api-key
Content-Type: application/json
anthropic-version: 2023-06-01
```

**请求体**：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | ✅ | `claude-sonnet-4-20250514` / `-fast` / `-slow` |
| `messages` | array | ✅ | Claude 风格消息数组 |
| `max_tokens` | number | ❌ | 当前实现不会硬性截断上游输出 |
| `stream` | boolean | ❌ | 默认 `false` |
| `system` | string | ❌ | 可选系统提示 |
| `tools` | array | ❌ | Claude tool 定义 |

#### 非流式响应

```json
{
  "id": "msg_1738400000000000000",
  "type": "message",
  "role": "assistant",
  "model": "claude-sonnet-4-20250514",
  "content": [
    {"type": "text", "text": "回复内容"}
  ],
  "stop_reason": "end_turn",
  "stop_sequence": null,
  "usage": {
    "input_tokens": 12,
    "output_tokens": 34
  }
}
```

若识别到工具调用，`stop_reason=tool_use`，`content` 中返回 `tool_use` block。

#### 流式响应（`stream=true`）

SSE 使用 `event:` + `data:` 双行格式，JSON 中保留 `type` 字段。

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

**说明**：

- 思维模型（`-slow`）会输出 `thinking_delta`
- 不会输出 `signature_delta`（上游 DeepSeek 未提供可验证签名）
- `tools` 场景优先避免泄露原始工具 JSON，不强制发送 `input_json_delta`

### `POST /anthropic/v1/messages/count_tokens`

**请求**：

```json
{
  "model": "claude-sonnet-4-20250514",
  "messages": [
    {"role": "user", "content": "你好"}
  ]
}
```

**响应**：

```json
{
  "input_tokens": 5
}
```

---

## Admin 接口

### `POST /admin/login`

无需鉴权。

**请求**：

```json
{
  "admin_key": "admin",
  "expire_hours": 24
}
```

`expire_hours` 可省略，默认 `24`。

**响应**：

```json
{
  "success": true,
  "token": "<jwt>",
  "expires_in": 86400
}
```

### `GET /admin/verify`

需要 JWT：`Authorization: Bearer <jwt>`

**响应**：

```json
{
  "valid": true,
  "expires_at": 1738400000,
  "remaining_seconds": 72000
}
```

### `GET /admin/vercel/config`

返回 Vercel 预配置状态。

```json
{
  "has_token": true,
  "project_id": "prj_xxx",
  "team_id": null
}
```

### `GET /admin/config`

返回脱敏后的配置。

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

可更新 `keys`、`accounts`、`claude_mapping`。

**请求**：

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

**响应**：`{"success": true, "total_keys": 3}`

### `DELETE /admin/keys/{key}`

**响应**：`{"success": true, "total_keys": 2}`

### `GET /admin/accounts`

**查询参数**：

| 参数 | 默认 | 范围 |
| --- | --- | --- |
| `page` | `1` | ≥ 1 |
| `page_size` | `10` | 1–100 |

**响应**：

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

**响应**：`{"success": true, "total_accounts": 6}`

### `DELETE /admin/accounts/{identifier}`

`identifier` 为 email 或 mobile。

**响应**：`{"success": true, "total_accounts": 5}`

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

| 字段 | 说明 |
| --- | --- |
| `available` | 当前可用账号数 |
| `in_use` | 当前使用中的账号数 |
| `total` | 总账号数 |
| `max_inflight_per_account` | 每账号并发上限 |
| `recommended_concurrency` | 建议并发值（`total × max_inflight_per_account`） |

### `POST /admin/accounts/test`

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `identifier` | ✅ | email 或 mobile |
| `model` | ❌ | 默认 `deepseek-chat` |
| `message` | ❌ | 空字符串时仅测试会话创建 |

**响应**：

```json
{
  "account": "user@example.com",
  "success": true,
  "response_time": 1240,
  "message": "API 测试成功（仅会话创建）",
  "model": "deepseek-chat"
}
```

### `POST /admin/accounts/test-all`

可选请求字段：`model`

```json
{
  "total": 5,
  "success": 4,
  "failed": 1,
  "results": [...]
}
```

### `POST /admin/import`

批量导入 keys 与 accounts。

**请求**：

```json
{
  "keys": ["k1", "k2"],
  "accounts": [
    {"email": "user@example.com", "password": "pwd", "token": ""}
  ]
}
```

**响应**：

```json
{
  "success": true,
  "imported_keys": 2,
  "imported_accounts": 1
}
```

### `POST /admin/test`

测试当前 API 可用性（通过自身接口调用）。

| 字段 | 必填 | 默认值 |
| --- | --- | --- |
| `model` | ❌ | `deepseek-chat` |
| `message` | ❌ | `你好` |
| `api_key` | ❌ | 配置中第一个 key |

**响应**：

```json
{
  "success": true,
  "status_code": 200,
  "response": {"id": "..."}
}
```

### `POST /admin/vercel/sync`

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `vercel_token` | ❌ | 空或 `__USE_PRECONFIG__` 则读环境变量 |
| `project_id` | ❌ | 空则读 `VERCEL_PROJECT_ID` |
| `team_id` | ❌ | 空则读 `VERCEL_TEAM_ID` |
| `auto_validate` | ❌ | 默认 `true` |
| `save_credentials` | ❌ | 默认 `true` |

**成功响应**：

```json
{
  "success": true,
  "validated_accounts": 3,
  "message": "配置已同步，正在重新部署...",
  "deployment_url": "https://..."
}
```

或需要手动部署：

```json
{
  "success": true,
  "validated_accounts": 3,
  "message": "配置已同步到 Vercel，请手动触发重新部署",
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

## 错误响应格式

不同模块的错误格式略有差异：

| 模块 | 格式 |
| --- | --- |
| OpenAI 接口 | `{"error": {"message": "...", "type": "..."}}` |
| Claude 接口 | `{"error": {"type": "...", "message": "..."}}` |
| Admin 接口 | `{"detail": "..."}` |

建议客户端处理逻辑：检查 HTTP 状态码 + 解析 `error` 或 `detail` 字段。

**常见状态码**：

| 状态码 | 说明 |
| --- | --- |
| `401` | 鉴权失败（key/token 无效，或 Admin JWT 过期） |
| `429` | 请求过多（超出并发上限 + 等待队列） |
| `503` | 模型不可用或上游服务异常 |

---

## cURL 示例

### OpenAI 非流式

```bash
curl http://localhost:5001/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat",
    "messages": [{"role": "user", "content": "你好"}],
    "stream": false
  }'
```

### OpenAI 流式

```bash
curl http://localhost:5001/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-reasoner",
    "messages": [{"role": "user", "content": "解释一下量子纠缠"}],
    "stream": true
  }'
```

### OpenAI 带搜索

```bash
curl http://localhost:5001/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat-search",
    "messages": [{"role": "user", "content": "今天的新闻"}],
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
    "messages": [{"role": "user", "content": "北京今天天气怎么样？"}],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "get_weather",
          "description": "获取指定城市的天气",
          "parameters": {
            "type": "object",
            "properties": {
              "city": {"type": "string", "description": "城市名"}
            },
            "required": ["city"]
          }
        }
      }
    ]
  }'
```

### Claude 非流式

```bash
curl http://localhost:5001/anthropic/v1/messages \
  -H "x-api-key: your-api-key" \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-sonnet-4-20250514",
    "max_tokens": 1024,
    "messages": [{"role": "user", "content": "你好"}]
  }'
```

### Claude 流式

```bash
curl http://localhost:5001/anthropic/v1/messages \
  -H "x-api-key: your-api-key" \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-sonnet-4-20250514-slow",
    "max_tokens": 1024,
    "messages": [{"role": "user", "content": "解释相对论"}],
    "stream": true
  }'
```

### Admin 登录

```bash
curl http://localhost:5001/admin/login \
  -H "Content-Type: application/json" \
  -d '{"admin_key": "admin"}'
```

### 指定账号请求

```bash
curl http://localhost:5001/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "X-Ds2-Target-Account: user@example.com" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat",
    "messages": [{"role": "user", "content": "你好"}]
  }'
```
