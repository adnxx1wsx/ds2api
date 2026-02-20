# DS2API

[![License](https://img.shields.io/github/license/CJackHwang/ds2api.svg)](LICENSE)
![Stars](https://img.shields.io/github/stars/CJackHwang/ds2api.svg)
![Forks](https://img.shields.io/github/forks/CJackHwang/ds2api.svg)
[![Version](https://img.shields.io/badge/version-1.6.12-blue.svg)](version.txt)
[![Docker](https://img.shields.io/badge/docker-ready-blue.svg)](DEPLOY.md)

璇█ / Language: [涓枃](README.MD) | [English](README.en.md)

灏?DeepSeek Web 瀵硅瘽鑳藉姏杞崲涓?OpenAI 涓?Claude 鍏煎 API銆傚悗绔负 **Go 鍏ㄩ噺瀹炵幇**锛屽墠绔负 React WebUI 绠＄悊鍙帮紙婧愮爜鍦?`webui/`锛岄儴缃叉椂鑷姩鏋勫缓鍒?`static/admin`锛夈€?

## 鏋舵瀯姒傝

```mermaid
flowchart LR
    Client["馃枼锔?瀹㈡埛绔痋n(OpenAI / Claude 鍏煎)"]

    subgraph DS2API["DS2API 鏈嶅姟"]
        direction TB
        CORS["CORS 涓棿浠?]
        Auth["馃攼 閴存潈涓棿浠?]

        subgraph Adapters["閫傞厤鍣ㄥ眰"]
            OA["OpenAI 閫傞厤鍣╘n/v1/*"]
            CA["Claude 閫傞厤鍣╘n/anthropic/*"]
        end

        subgraph Support["鏀拺妯″潡"]
            Pool["馃摝 璐﹀彿姹?/ 骞跺彂闃熷垪"]
            PoW["鈿欙笍 PoW WASM\n(wazero)"]
        end

        Admin["馃洜锔?Admin API\n/admin/*"]
        WebUI["馃寪 WebUI\n(/admin)"]
    end

    DS["鈽侊笍 DeepSeek API"]

    Client -- "璇锋眰" --> CORS --> Auth
    Auth --> OA & CA
    OA & CA -- "璋冪敤" --> DS
    Auth --> Admin
    OA & CA -. "杞閫夎处鍙? .-> Pool
    OA & CA -. "璁＄畻 PoW" .-> PoW
    DS -- "鍝嶅簲" --> Client
```

- **鍚庣**锛欸o锛坄cmd/ds2api/`銆乣api/`銆乣internal/`锛夛紝涓嶄緷璧?Python 杩愯鏃?
- **鍓嶇**锛歊eact 绠＄悊鍙帮紙`webui/`锛夛紝杩愯鏃舵墭绠￠潤鎬佹瀯寤轰骇鐗?
- **閮ㄧ讲**锛氭湰鍦拌繍琛屻€丏ocker銆乂ercel Serverless銆丩inux systemd

## 鏍稿績鑳藉姏

| 鑳藉姏 | 璇存槑 |
| --- | --- |
| OpenAI 鍏煎 | `GET /v1/models`銆乣GET /v1/models/{id}`銆乣POST /v1/chat/completions`銆乣POST /v1/responses`銆乣GET /v1/responses/{response_id}`銆乣POST /v1/embeddings` |
| Claude 鍏煎 | `GET /anthropic/v1/models`銆乣POST /anthropic/v1/messages`銆乣POST /anthropic/v1/messages/count_tokens` |
| 澶氳处鍙疯疆璇?| 鑷姩 token 鍒锋柊銆侀偖绠?鎵嬫満鍙峰弻鐧诲綍鏂瑰紡 |
| 骞跺彂闃熷垪鎺у埗 | 姣忚处鍙?in-flight 涓婇檺 + 绛夊緟闃熷垪锛屽姩鎬佽绠楀缓璁苟鍙戝€?|
| DeepSeek PoW | WASM 璁＄畻锛坄wazero`锛夛紝鏃犻渶澶栭儴 Node.js 渚濊禆 |
| Tool Calling | 闃叉硠婕忓鐞嗭細闈炰唬鐮佸潡楂樼疆淇＄壒寰佽瘑鍒€乣delta.tool_calls` 鏃╁彂銆佺粨鏋勫寲澧為噺杈撳嚭 |
| Admin API | 閰嶇疆绠＄悊銆佽处鍙锋祴璇?/ 鎵归噺娴嬭瘯銆佸鍏ュ鍑恒€乂ercel 鍚屾 |
| WebUI 绠＄悊鍙?| `/admin` 鍗曢〉搴旂敤锛堜腑鑻辨枃鍙岃銆佹繁鑹叉ā寮忥級 |
| 杩愮淮鎺㈤拡 | `GET /healthz`锛堝瓨娲伙級銆乣GET /readyz`锛堝氨缁級 |

## 骞冲彴鍏煎鐭╅樀

| 绾у埆 | 骞冲彴 | 褰撳墠鐘舵€?|
| --- | --- | --- |
| P0 | Codex CLI/SDK锛坄wire_api=chat` / `wire_api=responses`锛?| 鉁?|
| P0 | OpenAI SDK锛圝S/Python锛宑hat + responses锛?| 鉁?|
| P0 | Vercel AI SDK锛坥penai-compatible锛?| 鉁?|
| P0 | Anthropic SDK锛坢essages锛?| 鉁?|
| P1 | LangChain / LlamaIndex / OpenWebUI锛圤penAI 鍏煎鎺ュ叆锛?| 鉁?|
| P2 | MCP 鐙珛妗ユ帴灞?| 瑙勫垝涓?|

## 妯″瀷鏀寔

### OpenAI 鎺ュ彛

| 妯″瀷 | thinking | search |
| --- | --- | --- |
| `deepseek-chat` | 鉂?| 鉂?|
| `deepseek-reasoner` | 鉁?| 鉂?|
| `deepseek-chat-search` | 鉂?| 鉁?|
| `deepseek-reasoner-search` | 鉁?| 鉁?|

### Claude 鎺ュ彛

| 妯″瀷 | 榛樿鏄犲皠 |
| --- | --- |
| `claude-sonnet-4-5` | `deepseek-chat` |
| `claude-haiku-4-5`锛堝吋瀹?`claude-3-5-haiku-latest`锛?| `deepseek-chat` |
| `claude-opus-4-6` | `deepseek-reasoner` |

鍙€氳繃閰嶇疆涓殑 `claude_mapping` 鎴?`claude_model_mapping` 瑕嗙洊鏄犲皠鍏崇郴銆?
鍙﹀锛宍/anthropic/v1/models` 鐜板凡鍖呭惈 Claude 1.x/2.x/3.x/4.x 鍘嗗彶妯″瀷 ID 涓庡父瑙佸埆鍚嶏紝渚夸簬鏃у鎴风鐩存帴鍏煎銆?

## 蹇€熷紑濮?

### 閫氱敤绗竴姝ワ紙鎵€鏈夐儴缃叉柟寮忥級

鎶?`config.json` 浣滀负鍞竴閰嶇疆婧愶紙鎺ㄨ崘鍋氭硶锛夛細

```bash
cp config.example.json config.json
# 缂栬緫 config.json
```

鍚庣画閮ㄧ讲寤鸿锛?
- 鏈湴杩愯锛氱洿鎺ヨ鍙?`config.json`
- Docker / Vercel锛氱敱 `config.json` 鐢熸垚 `DS2API_CONFIG_JSON`锛圔ase64锛夋敞鍏ョ幆澧冨彉閲?

### 鏂瑰紡涓€锛氭湰鍦拌繍琛?

**鍓嶇疆瑕佹眰**锛欸o 1.24+锛孨ode.js 20+锛堜粎鍦ㄩ渶瑕佹瀯寤?WebUI 鏃讹級

```bash
# 1. 鍏嬮殕浠撳簱
git clone https://github.com/CJackHwang/ds2api.git
cd ds2api

# 2. 閰嶇疆
cp config.example.json config.json
# 缂栬緫 config.json锛屽～鍏ヤ綘鐨?DeepSeek 璐﹀彿淇℃伅鍜?API key

# 3. 鍚姩
go run ./cmd/ds2api
```

榛樿鐩戝惉鍦板潃锛歚http://localhost:5001`

> **WebUI 鑷姩鏋勫缓**锛氭湰鍦伴娆″惎鍔ㄦ椂锛岃嫢 `static/admin` 涓嶅瓨鍦紝浼氳嚜鍔ㄥ皾璇曟墽琛?`npm install && npm run build`锛堥渶瑕佹湰鏈烘湁 Node.js锛夈€備綘涔熷彲浠ユ墜鍔ㄦ瀯寤猴細`./scripts/build-webui.sh`

### 鏂瑰紡浜岋細Docker 杩愯

```bash
# 1. 鍑嗗鐜鍙橀噺鏂囦欢
cp .env.example .env

# 2. 浠?config.json 鐢熸垚 DS2API_CONFIG_JSON锛堝崟琛?Base64锛?
DS2API_CONFIG_JSON="$(base64 < config.json | tr -d '\n')"

# 3. 缂栬緫 .env锛岃缃細
#    DS2API_ADMIN_KEY=璇锋浛鎹负寮哄瘑鐮?
#    DS2API_CONFIG_JSON=${DS2API_CONFIG_JSON}

# 4. 鍚姩
docker-compose up -d

# 5. 鏌ョ湅鏃ュ織
docker-compose logs -f
```

鏇存柊闀滃儚锛歚docker-compose up -d --build`

### 鏂瑰紡涓夛細Vercel 閮ㄧ讲

1. Fork 浠撳簱鍒拌嚜宸辩殑 GitHub
2. 鍦?Vercel 涓婂鍏ラ」鐩?
3. 閰嶇疆鐜鍙橀噺锛堟渶灏戣缃?`DS2API_ADMIN_KEY`锛涙帹鑽愬悓鏃惰缃?`DS2API_CONFIG_JSON`锛?
4. 閮ㄧ讲

寤鸿鍏堝湪浠撳簱鐩綍澶嶅埗妯℃澘骞跺～鍐欙細

```bash
cp config.example.json config.json
# 缂栬緫 config.json
```

鎺ㄨ崘锛氬厛鏈湴鎶?`config.json` 杞垚 Base64锛屽啀绮樿创鍒?`DS2API_CONFIG_JSON`锛岄伩鍏?JSON 鏍煎紡閿欒锛?

```bash
base64 < config.json | tr -d '\n'
```

> **娴佸紡璇存槑**锛歚/v1/chat/completions` 鍦?Vercel 涓婇粯璁よ蛋 `api/chat-stream.js`锛圢ode Runtime锛変互淇濊瘉瀹炴椂 SSE銆傞壌鏉冦€佽处鍙烽€夋嫨銆佷細璇?PoW 鍑嗗浠嶇敱 Go 鍐呴儴 prepare 鎺ュ彛瀹屾垚锛涙祦寮忓搷搴旓紙鍚?`tools`锛夊湪 Node 渚ф墽琛屼笌 Go 瀵归綈鐨勮緭鍑虹粍瑁呬笌闃叉硠婕忓鐞嗐€?

璇︾粏閮ㄧ讲璇存槑璇峰弬闃?[閮ㄧ讲鎸囧崡](DEPLOY.md)銆?

### 鏂瑰紡鍥涳細涓嬭浇 Release 鏋勫缓鍖?

姣忔鍙戝竷 Release 鏃讹紝GitHub Actions 浼氳嚜鍔ㄦ瀯寤哄骞冲彴浜岃繘鍒跺寘锛?

```bash
# 涓嬭浇瀵瑰簲骞冲彴鐨勫帇缂╁寘鍚?
tar -xzf ds2api_v1.7.0_linux_amd64.tar.gz
cd ds2api_v1.7.0_linux_amd64
cp config.example.json config.json
# 缂栬緫 config.json
./ds2api
```

### 鏂瑰紡浜旓細OpenCode CLI 鎺ュ叆

1. 澶嶅埗绀轰緥閰嶇疆锛?

```bash
cp opencode.json.example opencode.json
```

2. 缂栬緫 `opencode.json`锛?
- 灏?`baseURL` 鏀逛负浣犵殑 DS2API 鍦板潃锛堜緥濡?`https://your-domain.com/v1`锛?
- 灏?`apiKey` 鏀逛负浣犵殑 DS2API key锛堝搴?`config.keys`锛?

3. 鍦ㄩ」鐩洰褰曞惎鍔?OpenCode CLI锛堟寜浣犵殑瀹夎鏂瑰紡杩愯 `opencode`锛夈€?

> 寤鸿浼樺厛浣跨敤 OpenAI 鍏煎璺緞锛坄/v1/*`锛夛紝鍗崇ず渚嬮噷鐨?`@ai-sdk/openai-compatible` provider銆?
> 鑻ュ鎴风鏀寔 `wire_api`锛屽彲鍒嗗埆娴嬭瘯 `responses` 涓?`chat`锛孌S2API 涓ゆ潯閾捐矾閮藉吋瀹广€?

## 閰嶇疆璇存槑

### `config.json` 绀轰緥

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
  "model_aliases": {
    "gpt-4o": "deepseek-chat",
    "gpt-5-codex": "deepseek-reasoner",
    "o3": "deepseek-reasoner"
  },
  "compat": {
    "wide_input_strict_output": true
  },
  "toolcall": {
    "mode": "feature_match",
    "early_emit_confidence": "high"
  },
  "responses": {
    "store_ttl_seconds": 900
  },
  "embeddings": {
    "provider": "deterministic"
  },
  "claude_model_mapping": {
    "fast": "deepseek-chat",
    "slow": "deepseek-reasoner"
  }
}
```

- `keys`锛欰PI 璁块棶瀵嗛挜鍒楄〃锛屽鎴风閫氳繃 `Authorization: Bearer <key>` 閴存潈
- `accounts`锛欴eepSeek 璐﹀彿鍒楄〃锛屾敮鎸?`email` 鎴?`mobile` 鐧诲綍
- `token`锛氱暀绌哄垯棣栨璇锋眰鏃惰嚜鍔ㄧ櫥褰曡幏鍙栵紱涔熷彲棰勫～宸叉湁 token
- `model_aliases`锛氬父瑙佹ā鍨嬪悕锛堝 GPT/Codex/Claude锛夊埌 DeepSeek 妯″瀷鐨勬槧灏?
- `compat.wide_input_strict_output`锛氬缓璁繚鎸?`true`锛堝綋鍓嶅疄鐜伴粯璁ゅ杩涗弗鍑猴級
- `toolcall`锛氬浐瀹氶噰鐢ㄧ壒寰佸尮閰?+ 楂樼疆淇℃棭鍙戠瓥鐣?
- `responses.store_ttl_seconds`锛歚/v1/responses/{id}` 鐨勫唴瀛樼紦瀛?TTL
- `embeddings.provider`锛歟mbedding 鎻愪緵鏂癸紙褰撳墠鍐呯疆 `deterministic/mock/builtin`锛?
- `claude_model_mapping`锛氬瓧鍏镐腑 `fast`/`slow` 鍚庣紑鏄犲皠鍒板搴?DeepSeek 妯″瀷

### 鐜鍙橀噺

| 鍙橀噺 | 鐢ㄩ€?| 榛樿鍊?|
| --- | --- | --- |
| `PORT` | 鏈嶅姟绔彛 | `5001` |
| `LOG_LEVEL` | 鏃ュ織绾у埆 | `INFO`锛堝彲閫夛細`DEBUG`/`WARN`/`ERROR`锛?|
| `DS2API_ADMIN_KEY` | Admin 鐧诲綍瀵嗛挜 | `admin` |
| `DS2API_JWT_SECRET` | Admin JWT 绛惧悕瀵嗛挜 | 绛夊悓 `DS2API_ADMIN_KEY` |
| `DS2API_JWT_EXPIRE_HOURS` | Admin JWT 杩囨湡灏忔椂鏁?| `24` |
| `DS2API_CONFIG_PATH` | 閰嶇疆鏂囦欢璺緞 | `config.json` |
| `DS2API_CONFIG_JSON` | 鐩存帴娉ㄥ叆閰嶇疆锛圝SON 鎴?Base64锛?| 鈥?|
| `DS2API_WASM_PATH` | PoW WASM 鏂囦欢璺緞 | 鑷姩鏌ユ壘 |
| `DS2API_STATIC_ADMIN_DIR` | 绠＄悊鍙伴潤鎬佹枃浠剁洰褰?| `static/admin` |
| `DS2API_AUTO_BUILD_WEBUI` | 鍚姩鏃惰嚜鍔ㄦ瀯寤?WebUI | 鏈湴寮€鍚紝Vercel 鍏抽棴 |
| `DS2API_ACCOUNT_MAX_INFLIGHT` | 姣忚处鍙锋渶澶у苟鍙?in-flight 璇锋眰鏁?| `2` |
| `DS2API_ACCOUNT_CONCURRENCY` | 鍚屼笂锛堝吋瀹规棫鍚嶏級 | 鈥?|
| `DS2API_ACCOUNT_MAX_QUEUE` | 绛夊緟闃熷垪涓婇檺 | `recommended_concurrency` |
| `DS2API_ACCOUNT_QUEUE_SIZE` | 鍚屼笂锛堝吋瀹规棫鍚嶏級 | 鈥?|
| `DS2API_VERCEL_INTERNAL_SECRET` | Vercel 娣峰悎娴佸紡鍐呴儴閴存潈瀵嗛挜 | 鍥為€€鐢?`DS2API_ADMIN_KEY` |
| `DS2API_VERCEL_STREAM_LEASE_TTL_SECONDS` | 娴佸紡 lease 杩囨湡绉掓暟 | `900` |
| `VERCEL_TOKEN` | Vercel 鍚屾 token | 鈥?|
| `VERCEL_PROJECT_ID` | Vercel 椤圭洰 ID | 鈥?|
| `VERCEL_TEAM_ID` | Vercel 鍥㈤槦 ID | 鈥?|
| `DS2API_VERCEL_PROTECTION_BYPASS` | Vercel 閮ㄧ讲淇濇姢缁曡繃瀵嗛挜锛堝唴閮?Node鈫扜o 璋冪敤锛?| 鈥?|

## 閴存潈妯″紡

璋冪敤涓氬姟鎺ュ彛锛坄/v1/*`銆乣/anthropic/*`锛夋椂鏀寔涓ょ妯″紡锛?

| 妯″紡 | 璇存槑 |
| --- | --- |
| **鎵樼璐﹀彿妯″紡** | `Bearer` 鎴?`x-api-key` 浼犲叆 `config.keys` 涓殑 key锛岀敱鏈嶅姟鑷姩杞閫夋嫨璐﹀彿 |
| **鐩撮€?token 妯″紡** | 浼犲叆 token 涓嶅湪 `config.keys` 涓椂锛岀洿鎺ヤ綔涓?DeepSeek token 浣跨敤 |

鍙€夎姹傚ご `X-Ds2-Target-Account`锛氭寚瀹氫娇鐢ㄦ煇涓墭绠¤处鍙凤紙鍊间负 email 鎴?mobile锛夈€?

## 骞跺彂妯″瀷

```
姣忚处鍙峰彲鐢ㄥ苟鍙?= DS2API_ACCOUNT_MAX_INFLIGHT锛堥粯璁?2锛?
寤鸿骞跺彂鍊?= 璐﹀彿鏁伴噺 脳 姣忚处鍙峰苟鍙戜笂闄?
绛夊緟闃熷垪涓婇檺 = DS2API_ACCOUNT_MAX_QUEUE锛堥粯璁?= 寤鸿骞跺彂鍊硷級
429 闃堝€?= in-flight + 绛夊緟闃熷垪 鈮?璐﹀彿鏁伴噺 脳 4
```

- 褰?in-flight 妲戒綅婊℃椂锛岃姹傝繘鍏ョ瓑寰呴槦鍒楋紝**涓嶄細绔嬪嵆 429**
- 瓒呭嚭鎬绘壙杞戒笂闄愬悗鎵嶈繑鍥?`429 Too Many Requests`
- `GET /admin/queue/status` 杩斿洖瀹炴椂骞跺彂鐘舵€?

## Tool Call 閫傞厤

褰撹姹備腑甯?`tools` 鏃讹紝DS2API 浼氬仛闃叉硠婕忓鐞嗭細

1. 鍙湪**闈炰唬鐮佸潡涓婁笅鏂?*鍚敤 toolcall 鐗瑰緛璇嗗埆锛堜唬鐮佸潡绀轰緥涓嶄細瑙﹀彂锛?
2. 涓€鏃﹀懡涓珮缃俊鐗瑰緛锛坄tool_calls` + `name` + `arguments/input` 璧峰锛夊氨绔嬪嵆杈撳嚭 `delta.tool_calls`
3. 宸茬‘璁ょ殑 toolcall JSON 鐗囨涓嶄細娉勬紡鍒?`delta.content`
4. 鍓嶆枃/鍚庢枃鑷劧璇█淇濇寔椤哄簭閫忎紶锛屾敮鎸佹贩鍚堟枃鏈笌澧為噺鍙傛暟杈撳嚭

## 椤圭洰缁撴瀯

```text
ds2api/
鈹溾攢鈹€ cmd/
鈹?  鈹溾攢鈹€ ds2api/              # 鏈湴 / 瀹瑰櫒鍚姩鍏ュ彛
鈹?  鈹斺攢鈹€ ds2api-tests/        # 绔埌绔祴璇曢泦鍏ュ彛
鈹溾攢鈹€ api/
鈹?  鈹溾攢鈹€ index.go             # Vercel Serverless Go 鍏ュ彛
鈹?  鈹溾攢鈹€ chat-stream.js       # Vercel Node.js 娴佸紡杞彂
鈹?  鈹斺攢鈹€ helpers/             # Node.js 杈呭姪妯″潡
鈹溾攢鈹€ internal/
鈹?  鈹溾攢鈹€ account/             # 璐﹀彿姹犱笌骞跺彂闃熷垪
鈹?  鈹溾攢鈹€ adapter/
鈹?  鈹?  鈹溾攢鈹€ openai/          # OpenAI 鍏煎閫傞厤鍣紙鍚?Tool Call 瑙ｆ瀽銆乂ercel 娴佸紡 prepare/release锛?
鈹?  鈹?  鈹斺攢鈹€ claude/          # Claude 鍏煎閫傞厤鍣?
鈹?  鈹溾攢鈹€ admin/               # Admin API handlers
鈹?  鈹溾攢鈹€ auth/                # 閴存潈涓?JWT
鈹?  鈹溾攢鈹€ config/              # 閰嶇疆鍔犺浇涓庣儹鏇存柊
鈹?  鈹溾攢鈹€ deepseek/            # DeepSeek API 瀹㈡埛绔€丳oW WASM
鈹?  鈹溾攢鈹€ server/              # HTTP 璺敱涓庝腑闂翠欢锛坈hi router锛?
鈹?  鈹溾攢鈹€ sse/                 # SSE 瑙ｆ瀽宸ュ叿
鈹?  鈹溾攢鈹€ util/                # 閫氱敤宸ュ叿鍑芥暟
鈹?  鈹斺攢鈹€ webui/               # WebUI 闈欐€佹枃浠舵墭绠′笌鑷姩鏋勫缓
鈹溾攢鈹€ webui/                   # React WebUI 婧愮爜锛圴ite + Tailwind锛?
鈹?  鈹斺攢鈹€ src/
鈹?      鈹溾攢鈹€ components/      # AccountManager / ApiTester / BatchImport / VercelSync / Login / LandingPage
鈹?      鈹斺攢鈹€ locales/         # 涓嫳鏂囪瑷€鍖咃紙zh.json / en.json锛?
鈹溾攢鈹€ scripts/
鈹?  鈹溾攢鈹€ build-webui.sh       # WebUI 鎵嬪姩鏋勫缓鑴氭湰
鈹?  鈹斺攢鈹€ testsuite/           # 娴嬭瘯闆嗚繍琛岃剼鏈?
鈹溾攢鈹€ static/admin/            # WebUI 鏋勫缓浜х墿锛堜笉鎻愪氦鍒?Git锛?
鈹溾攢鈹€ .github/
鈹?  鈹溾攢鈹€ workflows/           # GitHub Actions锛圧elease 鑷姩鏋勫缓锛?
鈹?  鈹溾攢鈹€ ISSUE_TEMPLATE/      # Issue 妯℃澘
鈹?  鈹斺攢鈹€ PULL_REQUEST_TEMPLATE.md
鈹溾攢鈹€ config.example.json      # 閰嶇疆鏂囦欢绀轰緥
鈹溾攢鈹€ .env.example             # 鐜鍙橀噺绀轰緥
鈹溾攢鈹€ Dockerfile               # 澶氶樁娈垫瀯寤猴紙WebUI + Go锛?
鈹溾攢鈹€ docker-compose.yml       # 鐢熶骇鐜 Docker Compose
鈹溾攢鈹€ docker-compose.dev.yml   # 寮€鍙戠幆澧?Docker Compose
鈹溾攢鈹€ vercel.json              # Vercel 璺敱涓庢瀯寤洪厤缃?
鈹溾攢鈹€ go.mod / go.sum          # Go 妯″潡渚濊禆
鈹斺攢鈹€ version.txt              # 鐗堟湰鍙?
```

## 鏂囨。绱㈠紩

| 鏂囨。 | 璇存槑 |
| --- | --- |
| [API.md](API.md) / [API.en.md](API.en.md) | API 鎺ュ彛鏂囨。锛堝惈璇锋眰/鍝嶅簲绀轰緥锛?|
| [DEPLOY.md](DEPLOY.md) / [DEPLOY.en.md](DEPLOY.en.md) | 閮ㄧ讲鎸囧崡锛堟湰鍦?Docker/Vercel/systemd锛?|
| [CONTRIBUTING.md](CONTRIBUTING.md) / [CONTRIBUTING.en.md](CONTRIBUTING.en.md) | 璐＄尞鎸囧崡 |
| [TESTING.md](TESTING.md) | 娴嬭瘯闆嗕娇鐢ㄦ寚鍗?|

## 娴嬭瘯

```bash
# 鍗曞厓娴嬭瘯
go test ./...

# 涓€閿鍒扮鍏ㄩ摼璺祴璇曪紙鐪熷疄璐﹀彿锛岀敓鎴愬畬鏁磋姹?鍝嶅簲鏃ュ織锛?
./scripts/testsuite/run-live.sh

# 鎴栬嚜瀹氫箟鍙傛暟
go run ./cmd/ds2api-tests \
  --config config.json \
  --admin-key admin \
  --out artifacts/testsuite \
  --timeout 120 \
  --retries 2
```

## Release 鑷姩鏋勫缓锛圙itHub Actions锛?

宸ヤ綔娴佹枃浠讹細`.github/workflows/release-artifacts.yml`

- **瑙﹀彂鏉′欢**锛氫粎鍦?GitHub Release `published` 鏃惰Е鍙戯紙鏅€?push 涓嶄細瑙﹀彂锛?
- **鏋勫缓浜х墿**锛氬骞冲彴浜岃繘鍒跺寘锛坄linux/amd64`銆乣linux/arm64`銆乣darwin/amd64`銆乣darwin/arm64`銆乣windows/amd64`锛? `sha256sums.txt`
- **姣忎釜鍘嬬缉鍖呭寘鍚?*锛歚ds2api` 鍙墽琛屾枃浠躲€乣static/admin`銆乄ASM 鏂囦欢銆侀厤缃ず渚嬨€丷EADME銆丩ICENSE

## 鍏嶈矗澹版槑

鏈」鐩熀浜庨€嗗悜鏂瑰紡瀹炵幇锛屼粎渚涘涔犱笌鐮旂┒浣跨敤銆傜ǔ瀹氭€у拰鍙敤鎬т笉浣滀繚璇侊紝璇峰嬁鐢ㄤ簬杩濆弽鏈嶅姟鏉℃鎴栨硶寰嬫硶瑙勭殑鍦烘櫙銆?

