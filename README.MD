# DS2API

[![License](https://img.shields.io/github/license/CJackHwang/ds2api.svg)](LICENSE)
![Stars](https://img.shields.io/github/stars/CJackHwang/ds2api.svg)
![Forks](https://img.shields.io/github/forks/CJackHwang/ds2api.svg)
[![Version](https://img.shields.io/badge/version-1.6.11-blue.svg)](version.txt)
[![Docker](https://img.shields.io/badge/docker-ready-blue.svg)](DEPLOY.md#docker-部署推荐)

语言 / Language: [中文](README.MD) | [English](README.en.md)

将 DeepSeek 免费对话版转换为 **OpenAI & Claude 兼容 API**，支持多账号轮询、自动 Token 刷新、可视化管理界面。

![p1](https://github.com/user-attachments/assets/07296a50-50d4-4f05-a9e5-280df14e9532)
![p2](https://github.com/user-attachments/assets/03b4a763-766f-4050-aea8-1a183e70ae6a)
![p3](https://github.com/user-attachments/assets/fc8b9836-11e3-4c38-a684-eb2c79b80fe9)
![p4](https://github.com/user-attachments/assets/513e9ca7-aa9e-45a6-8f7e-f362b1650675)



## ✨ 特性

- 🔄 **双协议兼容** - 同时支持 OpenAI 和 Claude (Anthropic) API 格式
- 🚀 **多账号轮询** - Round-Robin 负载均衡，支持高并发场景
- 🔐 **Token 自动刷新** - 过期自动重新登录，无需手动维护
- 🌐 **WebUI 管理** - 可视化添加账号、测试 API、同步 Vercel 配置
- 🌍 **多语言切换** - WebUI 内置中英双语，可随时切换
- 🔍 **联网搜索** - 支持 DeepSeek 原生搜索增强模式
- 🧠 **深度思考** - 支持推理模式，输出思考过程
- 🛠️ **工具调用** - 兼容 OpenAI Function Calling 格式
- ☁️ **Vercel 一键部署** - 无需服务器，快速上线

## 📋 模型支持

### OpenAI 兼容接口 (`/v1/chat/completions`)

| 模型 | 深度思考 | 联网搜索 | 说明 |
|-----|:--------:|:--------:|------|
| `deepseek-chat` | ❌ | ❌ | 标准对话模式 |
| `deepseek-reasoner` | ✅ | ❌ | 推理模式（输出思考过程） |
| `deepseek-chat-search` | ❌ | ✅ | 联网搜索模式 |
| `deepseek-reasoner-search` | ✅ | ✅ | 推理 + 联网搜索 |

### Claude 兼容接口 (`/anthropic/v1/messages`)

| 模型 | 说明 |
|-----|------|
| `claude-sonnet-4-20250514` | 映射到 deepseek-chat（标准模式） |
| `claude-sonnet-4-20250514-fast` | 映射到 deepseek-chat（快速模式） |
| `claude-sonnet-4-20250514-slow` | 映射到 deepseek-reasoner（推理模式） |

> **提示**：Claude 接口实际调用的是 DeepSeek，响应格式会自动转换为 Anthropic 标准格式。

## 🚀 快速开始

### 方式一：Vercel 部署（推荐）

[![Deploy with Vercel](https://vercel.com/button)](https://vercel.com/new/clone?repository-url=https%3A%2F%2Fgithub.com%2FCJackHwang%2Fds2api&env=DS2API_ADMIN_KEY&envDescription=管理面板访问密码（必填）&envLink=https%3A%2F%2Fgithub.com%2FCJackHwang%2Fds2api%23环境变量&project-name=ds2api&repository-name=ds2api)

1. 点击上方按钮，设置管理密码 `DS2API_ADMIN_KEY`
2. 部署完成后访问 `/admin` 管理界面
3. 添加 DeepSeek 账号和自定义 API Key
4. 点击「同步到 Vercel」保存配置

> **首次同步会自动验证账号并保存 Token，后续操作无需重复输入凭证。**

### 方式二：本地开发

```bash
# 1. 克隆仓库
git clone https://github.com/CJackHwang/ds2api.git
cd ds2api

# 2. 准备配置
cp config.example.json config.json
# 编辑 config.json，添加 DeepSeek 账号信息

# 3. 启动服务（Go 版本）
go run ./cmd/ds2api
```

服务启动后访问 `http://localhost:5001`

## ⚙️ 配置说明

### 环境变量

| 变量 | 说明 | 必填 |
|-----|------|:----:|
| `DS2API_ADMIN_KEY` | 管理面板密码 | Vercel 必填 |
| `DS2API_CONFIG_JSON` | 配置 JSON 或 Base64 编码 | 可选 |
| `VERCEL_TOKEN` | Vercel API Token（用于同步） | 可选 |
| `VERCEL_PROJECT_ID` | Vercel 项目 ID | 可选 |
| `PORT` | 服务端口（默认 5001） | 可选 |

### 配置文件格式 (`config.json`)

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

> **说明**：
> - `keys`: 自定义的 API 密钥，用于调用本服务
> - `accounts`: DeepSeek 网页版账号，支持邮箱或手机号登录
> - `token`: 留空即可，系统会自动获取并刷新

## 📡 API 使用

完整 API 文档请参阅 **[API.md](API.md)**

### 快速示例

**获取模型列表**：
```bash
curl http://localhost:5001/v1/models
```

**OpenAI 格式调用**：
```bash
curl http://localhost:5001/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat",
    "messages": [{"role": "user", "content": "你好"}],
    "stream": true
  }'
```

**Claude 格式调用**：
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

### Python SDK 使用

```python
from openai import OpenAI

client = OpenAI(
    api_key="your-api-key",
    base_url="http://localhost:5001/v1"
)

response = client.chat.completions.create(
    model="deepseek-reasoner",
    messages=[{"role": "user", "content": "请解释量子纠缠"}],
    stream=True
)

for chunk in response:
    if chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="")
```

## 🔧 部署配置

### Nginx 反向代理

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

### 方式三：Docker 部署

```bash
# 1. 克隆仓库并进入目录
git clone https://github.com/CJackHwang/ds2api.git
cd ds2api

# 2. 配置环境变量
cp .env.example .env
# 编辑 .env，填写 DS2API_ADMIN_KEY 和 DS2API_CONFIG_JSON

# 3. 启动服务
docker-compose up -d

# 4. 查看日志
docker-compose logs -f
```

> **Docker 优势**：零侵入设计，主代码更新只需 `docker-compose up -d --build`，无需修改 Docker 配置。详见 [DEPLOY.md](DEPLOY.md#docker-部署推荐)。

## ⚠️ 免责声明

**本项目基于逆向工程实现，服务稳定性无法保证。**

- 仅供学习研究使用，**禁止商业用途或对外提供服务**
- 建议正式项目使用 [DeepSeek 官方 API](https://platform.deepseek.com/)
- 使用本项目产生的任何风险由用户自行承担

## 📜 鸣谢

本项目基于以下开源项目：

- [iidamie/deepseek2api](https://github.com/iidamie/deepseek2api)
- [LLM-Red-Team/deepseek-free-api](https://github.com/LLM-Red-Team/deepseek-free-api)

## 📊 Star History

[![Star History Chart](https://api.star-history.com/svg?repos=CJackHwang/ds2api&type=Date)](https://star-history.com/#CJackHwang/ds2api&Date)
