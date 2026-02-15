# DS2API

[![License](https://img.shields.io/github/license/CJackHwang/ds2api.svg)](LICENSE)
![Stars](https://img.shields.io/github/stars/CJackHwang/ds2api.svg)
![Forks](https://img.shields.io/github/forks/CJackHwang/ds2api.svg)
[![Version](https://img.shields.io/badge/version-1.6.11-blue.svg)](version.txt)
[![Docker](https://img.shields.io/badge/docker-ready-blue.svg)](DEPLOY.md#docker-deployment-recommended)

Language: [中文](README.MD) | [English](README.en.md)

Convert DeepSeek Web into an **OpenAI & Claude compatible API**, with multi-account rotation, automatic token refresh, and a visual admin console.

![p1](https://github.com/user-attachments/assets/07296a50-50d4-4f05-a9e5-280df14e9532)
![p2](https://github.com/user-attachments/assets/03b4a763-766f-4050-aea8-1a183e70ae6a)
![p3](https://github.com/user-attachments/assets/fc8b9836-11e3-4c38-a684-eb2c79b80fe9)
![p4](https://github.com/user-attachments/assets/513e9ca7-aa9e-45a6-8f7e-f362b1650675)

## ✨ Features

- 🔄 **Dual-protocol support** - OpenAI and Claude (Anthropic) compatible APIs
- 🚀 **Multi-account rotation** - Round-robin load balancing for high concurrency
- 🔐 **Automatic token refresh** - Re-auth on expiry without manual maintenance
- 🌐 **WebUI management** - Add accounts, test APIs, and sync Vercel settings visually
- 🌍 **Language toggle** - Built-in Chinese and English UI switcher
- 🔍 **Web search** - DeepSeek native search enhancement mode
- 🧠 **Deep reasoning** - Reasoning mode with trace output
- 🛠️ **Tool calling** - OpenAI Function Calling compatible
- ☁️ **One-click Vercel deploy** - No server required

## 📋 Model Support

### OpenAI compatible endpoint (`/v1/chat/completions`)

| Model | Reasoning | Search | Notes |
|-----|:--------:|:------:|------|
| `deepseek-chat` | ❌ | ❌ | Standard chat |
| `deepseek-reasoner` | ✅ | ❌ | Reasoning (shows trace) |
| `deepseek-chat-search` | ❌ | ✅ | Web search mode |
| `deepseek-reasoner-search` | ✅ | ✅ | Reasoning + search |

### Claude compatible endpoint (`/anthropic/v1/messages`)

| Model | Notes |
|-----|------|
| `claude-sonnet-4-20250514` | Maps to deepseek-chat (standard) |
| `claude-sonnet-4-20250514-fast` | Maps to deepseek-chat (fast) |
| `claude-sonnet-4-20250514-slow` | Maps to deepseek-reasoner (reasoning) |

> **Tip**: The Claude endpoint actually calls DeepSeek and returns Anthropic-format responses.

## 🚀 Quick Start

### Option 1: Vercel deployment (recommended)

[![Deploy with Vercel](https://vercel.com/button)](https://vercel.com/new/clone?repository-url=https%3A%2F%2Fgithub.com%2FCJackHwang%2Fds2api&env=DS2API_ADMIN_KEY&envDescription=Admin%20console%20access%20key%20%28required%29&envLink=https%3A%2F%2Fgithub.com%2FCJackHwang%2Fds2api%23environment-variables&project-name=ds2api&repository-name=ds2api)

1. Click the button above and set `DS2API_ADMIN_KEY`
2. After deployment, visit `/admin`
3. Add DeepSeek accounts and custom API keys
4. Click "Sync to Vercel" to persist configuration

> **First sync validates accounts and stores tokens automatically.**

### Option 2: Local development

```bash
# 1. Clone the repo
git clone https://github.com/CJackHwang/ds2api.git
cd ds2api

# 2. Configure accounts
cp config.example.json config.json
# Edit config.json to add DeepSeek account info

# 3. Start the service (Go runtime)
go run ./cmd/ds2api
```

Visit `http://localhost:5001` after startup.

## ⚙️ Configuration

### Environment variables

| Variable | Description | Required |
|-----|------|:----:|
| `DS2API_ADMIN_KEY` | Admin console password | Required on Vercel |
| `DS2API_CONFIG_JSON` | Config JSON or Base64 | Optional |
| `VERCEL_TOKEN` | Vercel API token (for sync) | Optional |
| `VERCEL_PROJECT_ID` | Vercel project ID | Optional |
| `PORT` | Service port (default 5001) | Optional |

### Config file format (`config.json`)

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
  ]
}
```

> **Notes**:
> - `keys`: Custom API keys for calling this service
> - `accounts`: DeepSeek Web accounts (email or mobile)
> - `token`: Leave blank; DS2API will fetch and refresh automatically

## 📡 API Usage

See **[API.md](API.md)** for full API documentation.

### Quick examples

**List models**:
```bash
curl http://localhost:5001/v1/models
```

**OpenAI-compatible call**:
```bash
curl http://localhost:5001/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat",
    "messages": [{"role": "user", "content": "Hello"}],
    "stream": true
  }'
```

**Claude-compatible call**:
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

### Python SDK usage

```python
from openai import OpenAI

client = OpenAI(
    api_key="your-api-key",
    base_url="http://localhost:5001/v1"
)

response = client.chat.completions.create(
    model="deepseek-reasoner",
    messages=[{"role": "user", "content": "Explain quantum entanglement"}],
    stream=True
)

for chunk in response:
    if chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="")
```

## 🔧 Deployment Notes

### Nginx reverse proxy

```nginx
location / {
    proxy_pass http://localhost:5001;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
    proxy_buffering off;
    proxy_cache off;
    chunked_transfer_encoding on;
    tcp_nopush on;
    tcp_nodelay on;
    keepalive_timeout 120;
}
```

### Option 3: Docker deployment

```bash
# 1. Clone the repo and enter the directory
git clone https://github.com/CJackHwang/ds2api.git
cd ds2api

# 2. Configure environment variables
cp .env.example .env
# Edit .env and fill in DS2API_ADMIN_KEY and DS2API_CONFIG_JSON

# 3. Start the service
docker-compose up -d

# 4. Check logs
docker-compose logs -f
```

> **Docker advantage**: Zero-intrusion design; update the main code with `docker-compose up -d --build` without changing Docker configuration. See [DEPLOY.md](DEPLOY.md#docker-deployment-recommended).

## ⚠️ Disclaimer

**This project is based on reverse engineering and stability is not guaranteed.**

- For learning and research only. **No commercial use or public service is allowed.**
- For production, use the official [DeepSeek API](https://platform.deepseek.com/)
- You assume all risks from using this project

## 📜 Acknowledgements

This project is based on the following open-source projects:

- [iidamie/deepseek2api](https://github.com/iidamie/deepseek2api)
- [LLM-Red-Team/deepseek-free-api](https://github.com/LLM-Red-Team/deepseek-free-api)

## 📊 Star History

[![Star History Chart](https://api.star-history.com/svg?repos=CJackHwang/ds2api&type=Date)](https://star-history.com/#CJackHwang/ds2api&Date)
