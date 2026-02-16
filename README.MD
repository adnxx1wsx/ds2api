# DS2API

[![License](https://img.shields.io/github/license/CJackHwang/ds2api.svg)](LICENSE)
![Stars](https://img.shields.io/github/stars/CJackHwang/ds2api.svg)
![Forks](https://img.shields.io/github/forks/CJackHwang/ds2api.svg)
[![Version](https://img.shields.io/badge/version-1.6.11-blue.svg)](version.txt)
[![Docker](https://img.shields.io/badge/docker-ready-blue.svg)](DEPLOY.md)

语言 / Language: [中文](README.MD) | [English](README.en.md)

将 DeepSeek Web 对话能力转换为 OpenAI 与 Claude 兼容 API。当前仓库后端为 **Go 全量实现**，前端保留 React WebUI（源码在 `webui/`，部署时自动构建到 `static/admin`）。

## 当前实现边界

- 后端：Go（`cmd/`, `api/`, `internal/`），不再依赖 Python 运行时
- 前端：React 管理台（源码在 `webui/`，运行时托管静态构建）
- 部署：本地运行、Docker、Vercel Serverless

## 核心能力

- OpenAI 兼容：`/v1/models`、`/v1/chat/completions`
- Claude 兼容：`/anthropic/v1/models`、`/anthropic/v1/messages`、`/anthropic/v1/messages/count_tokens`
- 多账号轮询与自动 token 刷新
- DeepSeek PoW（WASM）计算
- Admin API：配置管理、账号测试、导入导出、Vercel 同步
- WebUI：`/admin` 单页应用托管
- 运维探针：`/healthz`、`/readyz`

## 模型支持

### OpenAI 接口

| 模型 | thinking | search |
| --- | --- | --- |
| `deepseek-chat` | false | false |
| `deepseek-reasoner` | true | false |
| `deepseek-chat-search` | false | true |
| `deepseek-reasoner-search` | true | true |

### Claude 接口

| 模型 | 默认映射 |
| --- | --- |
| `claude-sonnet-4-20250514` | `deepseek-chat` |
| `claude-sonnet-4-20250514-fast` | `deepseek-chat` |
| `claude-sonnet-4-20250514-slow` | `deepseek-reasoner` |

可通过配置中的 `claude_mapping` 或 `claude_model_mapping` 覆盖映射。

## 快速开始

### 1) 本地运行

要求：Go 1.24+

```bash
git clone https://github.com/CJackHwang/ds2api.git
cd ds2api

cp config.example.json config.json
# 编辑 config.json

go run ./cmd/ds2api
```

默认地址：`http://localhost:5001`

本地默认会在启动时自动尝试构建 WebUI（需要本机有 Node.js/npm）。
若你想手动构建，也可执行：

```bash
./scripts/build-webui.sh
```

### 2) Docker 运行

```bash
cp .env.example .env
# 编辑 .env

docker-compose up -d
docker-compose logs -f
```

### 3) Vercel 部署

- 入口：`api/index.go`
- 路由重写：`vercel.json`
- `vercel.json` 会在构建阶段自动执行 `npm ci --prefix webui && npm run build --prefix webui`
- `/v1/chat/completions` 在 Vercel 上默认走 `api/chat-stream.js`（Node Runtime）以保证实时 SSE
- `api/chat-stream.js` 仅负责流式数据转发；鉴权、账号选择、会话/PoW 准备仍由 Go 内部 prepare 接口处理
- 至少配置：
- `DS2API_ADMIN_KEY`
- `DS2API_CONFIG_JSON`（JSON 字符串或 Base64）

说明：仓库不提交 `static/admin` 构建产物，Vercel 构建时自动生成并打包。

## Release 自动构建产物（GitHub Actions）

仓库内置工作流：`.github/workflows/release-artifacts.yml`

- 触发条件：仅在 GitHub Release `published` 时触发
- 不会在普通 `push` 时构建
- 构建内容：多平台二进制包（Linux/macOS/Windows）+ `sha256sums.txt`
- 每个压缩包包含：
- `ds2api` 可执行文件（Windows 为 `ds2api.exe`）
- `static/admin`（WebUI 构建产物）
- `sha3_wasm_bg.7b9ca65ddd.wasm`
- `config.example.json`、`.env.example`
- `README.MD`、`README.en.md`、`LICENSE`

维护者发布步骤：

1. 在 GitHub 创建并发布 Release（带 tag，如 `v1.7.0`）
2. 等待 Actions 工作流 `Release Artifacts` 完成
3. 在 Release 的 Assets 下载对应平台压缩包

下载后运行示例（Linux/macOS）：

```bash
tar -xzf ds2api_v1.7.0_linux_amd64.tar.gz
cd ds2api_v1.7.0_linux_amd64
cp config.example.json config.json
./ds2api
```

## 配置说明

### `config.json` 示例

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

### 环境变量（核心）

| 变量 | 用途 |
| --- | --- |
| `PORT` | 服务端口，默认 `5001` |
| `LOG_LEVEL` | 日志级别：`DEBUG/INFO/WARN/ERROR` |
| `DS2API_ACCOUNT_MAX_INFLIGHT` | 每个账号最大并发 in-flight 请求数，默认 `2` |
| `DS2API_ACCOUNT_CONCURRENCY` | 同上别名（兼容旧写法） |
| `DS2API_ACCOUNT_MAX_QUEUE` | 等待队列上限，默认等于 `recommended_concurrency` |
| `DS2API_ACCOUNT_QUEUE_SIZE` | 同上别名（兼容旧写法） |
| `DS2API_ADMIN_KEY` | Admin 登录密钥，默认 `admin` |
| `DS2API_JWT_SECRET` | Admin JWT 签名密钥（可选） |
| `DS2API_JWT_EXPIRE_HOURS` | Admin JWT 过期小时数，默认 `24` |
| `DS2API_CONFIG_PATH` | 配置文件路径，默认 `config.json` |
| `DS2API_CONFIG_JSON` | 直接注入配置（JSON 或 Base64） |
| `DS2API_WASM_PATH` | PoW wasm 文件路径 |
| `DS2API_STATIC_ADMIN_DIR` | 管理台静态文件目录 |
| `DS2API_AUTO_BUILD_WEBUI` | 启动时缺失 WebUI 时是否自动执行 npm build（默认：本地开启，Vercel 关闭） |
| `DS2API_VERCEL_INTERNAL_SECRET` | Vercel 混合流式链路内部鉴权密钥（可选；未设置时回退用 `DS2API_ADMIN_KEY`） |
| `VERCEL_TOKEN` | Vercel 同步 token（可选） |
| `VERCEL_PROJECT_ID` | Vercel 项目 ID（可选） |
| `VERCEL_TEAM_ID` | Vercel 团队 ID（可选） |

## 鉴权与账号模式

调用业务接口时（`/v1/*`, `/anthropic/*`）支持两种模式：

1. 托管账号模式：`Bearer` 或 `x-api-key` 使用 `config.keys` 中的 key。
2. 直通 token 模式：当传入 token 不在 `config.keys` 中时，服务直接把它当作 DeepSeek token 使用。

可选请求头：`X-Ds2-Target-Account`，用于指定托管账号。

## 并发建议值

- 系统建议并发值按账号池动态计算：`账号数量 × 每账号并发上限`
- 默认每账号并发上限是 `2`，因此默认建议值是 `账号数量 × 2`
- 当 in-flight 槽位满时，请求会进入等待队列，不会立即 429
- 默认等待队列上限 = `recommended_concurrency`，因此默认总承载上限是 `账号数量 × 4`
- 超过总承载上限（in-flight + waiting）才返回 `429`
- 可通过 `DS2API_ACCOUNT_MAX_INFLIGHT`（或 `DS2API_ACCOUNT_CONCURRENCY`）手动覆盖每账号并发上限
- 可通过 `DS2API_ACCOUNT_MAX_QUEUE`（或 `DS2API_ACCOUNT_QUEUE_SIZE`）手动覆盖等待队列上限
- `GET /admin/queue/status` 会返回 `max_inflight_per_account`、`recommended_concurrency`、`waiting`、`max_queue_size`

## Tool Call 适配说明

当前实现对 toolcall 做了防泄漏处理：

- `tools` + `stream=true` 时，服务端会先缓冲正文片段
- 若识别到工具调用，会只输出结构化 `tool_calls`，不透传原始 JSON 文本
- 若最终不是工具调用，再一次性输出普通文本
- 解析器支持混合文本、fenced JSON、`function.arguments` 字符串等格式

## 文档与测试

- API 文档：`API.md` / `API.en.md`
- 部署文档：`DEPLOY.md` / `DEPLOY.en.md`
- 贡献指南：`CONTRIBUTING.md` / `CONTRIBUTING.en.md`

```bash
go test ./...
```

## 免责声明

本项目基于逆向方式实现，仅供学习与研究使用。稳定性和可用性不作保证，请勿用于违反服务条款或法律法规的场景。
