'use strict';

const DEEPSEEK_COMPLETION_URL = 'https://chat.deepseek.com/api/v0/chat/completion';

const BASE_HEADERS = {
  Host: 'chat.deepseek.com',
  'User-Agent': 'DeepSeek/1.6.11 Android/35',
  Accept: 'application/json',
  'Content-Type': 'application/json',
  'x-client-platform': 'android',
  'x-client-version': '1.6.11',
  'x-client-locale': 'zh_CN',
  'accept-charset': 'UTF-8',
};

const SKIP_PATTERNS = [
  'quasi_status',
  'elapsed_secs',
  'token_usage',
  'pending_fragment',
  'conversation_mode',
  'fragments/-1/status',
  'fragments/-2/status',
  'fragments/-3/status',
];

module.exports = async function handler(req, res) {
  setCorsHeaders(res);
  if (req.method === 'OPTIONS') {
    res.statusCode = 204;
    res.end();
    return;
  }
  if (req.method !== 'POST') {
    writeOpenAIError(res, 405, 'method not allowed');
    return;
  }

  const rawBody = await readRawBody(req);
  let payload;
  try {
    payload = JSON.parse(rawBody.toString('utf8') || '{}');
  } catch (_err) {
    writeOpenAIError(res, 400, 'invalid json');
    return;
  }

  // Keep all non-stream behavior on Go side to avoid compatibility regressions.
  if (!toBool(payload.stream) || (Array.isArray(payload.tools) && payload.tools.length > 0)) {
    await proxyToGo(req, res, rawBody);
    return;
  }

  const prep = await fetchStreamPrepare(req, rawBody);
  if (!prep.ok) {
    relayPreparedFailure(res, prep);
    return;
  }

  const model = asString(prep.body.model) || asString(payload.model);
  const sessionID = asString(prep.body.session_id) || `chatcmpl-${Date.now()}`;
  const deepseekToken = asString(prep.body.deepseek_token);
  const powHeader = asString(prep.body.pow_header);
  const completionPayload = prep.body.payload && typeof prep.body.payload === 'object' ? prep.body.payload : null;
  const finalPrompt = asString(prep.body.final_prompt);
  const thinkingEnabled = toBool(prep.body.thinking_enabled);
  const searchEnabled = toBool(prep.body.search_enabled);

  if (!model || !deepseekToken || !powHeader || !completionPayload) {
    writeOpenAIError(res, 500, 'invalid vercel prepare response');
    return;
  }

  const completionRes = await fetch(DEEPSEEK_COMPLETION_URL, {
    method: 'POST',
    headers: {
      ...BASE_HEADERS,
      authorization: `Bearer ${deepseekToken}`,
      'x-ds-pow-response': powHeader,
    },
    body: JSON.stringify(completionPayload),
  });

  if (!completionRes.ok || !completionRes.body) {
    const detail = await safeReadText(completionRes);
    writeOpenAIError(res, 500, detail ? `Failed to get completion: ${detail}` : 'Failed to get completion.');
    return;
  }

  res.statusCode = 200;
  res.setHeader('Content-Type', 'text/event-stream');
  res.setHeader('Cache-Control', 'no-cache, no-transform');
  res.setHeader('Connection', 'keep-alive');
  res.setHeader('X-Accel-Buffering', 'no');
  if (typeof res.flushHeaders === 'function') {
    res.flushHeaders();
  }

  const created = Math.floor(Date.now() / 1000);
  let firstChunkSent = false;
  let currentType = thinkingEnabled ? 'thinking' : 'text';
  let thinkingText = '';
  let outputText = '';
  const decoder = new TextDecoder();
  const reader = completionRes.body.getReader();
  let buffered = '';

  const sendFrame = (obj) => {
    res.write(`data: ${JSON.stringify(obj)}\n\n`);
    if (typeof res.flush === 'function') {
      res.flush();
    }
  };

  const finish = (reason) => {
    sendFrame({
      id: sessionID,
      object: 'chat.completion.chunk',
      created,
      model,
      choices: [{ delta: {}, index: 0, finish_reason: reason }],
      usage: buildUsage(finalPrompt, thinkingText, outputText),
    });
    res.write('data: [DONE]\n\n');
    res.end();
  };

  try {
    // eslint-disable-next-line no-constant-condition
    while (true) {
      const { value, done } = await reader.read();
      if (done) {
        break;
      }
      buffered += decoder.decode(value, { stream: true });
      const lines = buffered.split('\n');
      buffered = lines.pop() || '';

      for (const rawLine of lines) {
        const line = rawLine.trim();
        if (!line.startsWith('data:')) {
          continue;
        }
        const dataStr = line.slice(5).trim();
        if (!dataStr) {
          continue;
        }
        if (dataStr === '[DONE]') {
          finish('stop');
          return;
        }
        let chunk;
        try {
          chunk = JSON.parse(dataStr);
        } catch (_err) {
          continue;
        }
        if (chunk.error || chunk.code === 'content_filter') {
          finish('content_filter');
          return;
        }
        const parsed = parseChunkForContent(chunk, thinkingEnabled, currentType);
        currentType = parsed.newType;
        if (parsed.finished) {
          finish('stop');
          return;
        }

        for (const p of parsed.parts) {
          if (!p.text) {
            continue;
          }
          if (searchEnabled && isCitation(p.text)) {
            continue;
          }
          const delta = {};
          if (!firstChunkSent) {
            delta.role = 'assistant';
            firstChunkSent = true;
          }
          if (p.type === 'thinking') {
            thinkingText += p.text;
            delta.reasoning_content = p.text;
          } else {
            outputText += p.text;
            delta.content = p.text;
          }
          sendFrame({
            id: sessionID,
            object: 'chat.completion.chunk',
            created,
            model,
            choices: [{ delta, index: 0 }],
          });
        }
      }
    }
    finish('stop');
  } catch (_err) {
    finish('stop');
  }
};

function setCorsHeaders(res) {
  res.setHeader('Access-Control-Allow-Origin', '*');
  res.setHeader('Access-Control-Allow-Credentials', 'true');
  res.setHeader('Access-Control-Allow-Methods', 'GET, POST, OPTIONS, PUT, DELETE');
  res.setHeader('Access-Control-Allow-Headers', 'Content-Type, Authorization, X-API-Key, X-Ds2-Target-Account');
}

function header(req, key) {
  if (!req || !req.headers) {
    return '';
  }
  return asString(req.headers[key.toLowerCase()]);
}

async function readRawBody(req) {
  if (Buffer.isBuffer(req.body)) {
    return req.body;
  }
  if (typeof req.body === 'string') {
    return Buffer.from(req.body);
  }
  if (req.body && typeof req.body === 'object') {
    return Buffer.from(JSON.stringify(req.body));
  }
  const chunks = [];
  for await (const chunk of req) {
    chunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk));
  }
  return Buffer.concat(chunks);
}

async function fetchStreamPrepare(req, rawBody) {
  const proto = asString(header(req, 'x-forwarded-proto')) || 'https';
  const host = asString(header(req, 'host'));
  const url = new URL(`${proto}://${host}${req.url || '/v1/chat/completions'}`);
  url.searchParams.set('__go', '1');
  url.searchParams.set('__stream_prepare', '1');

  const upstream = await fetch(url.toString(), {
    method: 'POST',
    headers: {
      authorization: asString(header(req, 'authorization')),
      'x-api-key': asString(header(req, 'x-api-key')),
      'x-ds2-target-account': asString(header(req, 'x-ds2-target-account')),
      'x-ds2-internal-token': internalSecret(),
      'content-type': asString(header(req, 'content-type')) || 'application/json',
    },
    body: rawBody,
  });

  const text = await upstream.text();
  let body = {};
  try {
    body = JSON.parse(text || '{}');
  } catch (_err) {
    body = {};
  }

  return {
    ok: upstream.ok,
    status: upstream.status,
    contentType: upstream.headers.get('content-type') || 'application/json',
    text,
    body,
  };
}

function relayPreparedFailure(res, prep) {
  res.statusCode = prep.status || 500;
  res.setHeader('Content-Type', prep.contentType || 'application/json');
  if (prep.text) {
    res.end(prep.text);
    return;
  }
  writeOpenAIError(res, prep.status || 500, 'vercel prepare failed');
}

async function safeReadText(resp) {
  if (!resp) {
    return '';
  }
  try {
    const text = await resp.text();
    return text.trim();
  } catch (_err) {
    return '';
  }
}

function internalSecret() {
  return asString(process.env.DS2API_VERCEL_INTERNAL_SECRET) || asString(process.env.DS2API_ADMIN_KEY) || 'admin';
}

function parseChunkForContent(chunk, thinkingEnabled, currentType) {
  if (!chunk || typeof chunk !== 'object' || !Object.prototype.hasOwnProperty.call(chunk, 'v')) {
    return { parts: [], finished: false, newType: currentType };
  }
  const pathValue = asString(chunk.p);
  if (shouldSkipPath(pathValue)) {
    return { parts: [], finished: false, newType: currentType };
  }
  if (pathValue === 'response/status' && asString(chunk.v) === 'FINISHED') {
    return { parts: [], finished: true, newType: currentType };
  }

  let newType = currentType;
  const parts = [];

  if (pathValue === 'response/fragments' && asString(chunk.o).toUpperCase() === 'APPEND' && Array.isArray(chunk.v)) {
    for (const frag of chunk.v) {
      if (!frag || typeof frag !== 'object') {
        continue;
      }
      const fragType = asString(frag.type).toUpperCase();
      const content = asString(frag.content);
      if (!content) {
        continue;
      }
      if (fragType === 'THINK' || fragType === 'THINKING') {
        newType = 'thinking';
        parts.push({ text: content, type: 'thinking' });
      } else if (fragType === 'RESPONSE') {
        newType = 'text';
        parts.push({ text: content, type: 'text' });
      } else {
        parts.push({ text: content, type: 'text' });
      }
    }
  }

  if (pathValue === 'response' && Array.isArray(chunk.v)) {
    for (const item of chunk.v) {
      if (!item || typeof item !== 'object') {
        continue;
      }
      if (item.p === 'fragments' && item.o === 'APPEND' && Array.isArray(item.v)) {
        for (const frag of item.v) {
          const fragType = asString(frag && frag.type).toUpperCase();
          if (fragType === 'THINK' || fragType === 'THINKING') {
            newType = 'thinking';
          } else if (fragType === 'RESPONSE') {
            newType = 'text';
          }
        }
      }
    }
  }

  let partType = 'text';
  if (pathValue === 'response/thinking_content') {
    partType = 'thinking';
  } else if (pathValue === 'response/content') {
    partType = 'text';
  } else if (pathValue.includes('response/fragments') && pathValue.includes('/content')) {
    partType = newType;
  } else if (!pathValue && thinkingEnabled) {
    partType = newType;
  }

  const val = chunk.v;
  if (typeof val === 'string') {
    if (val === 'FINISHED' && (!pathValue || pathValue === 'status')) {
      return { parts: [], finished: true, newType };
    }
    if (val) {
      parts.push({ text: val, type: partType });
    }
    return { parts, finished: false, newType };
  }

  if (Array.isArray(val)) {
    for (const entry of val) {
      if (typeof entry === 'string') {
        if (entry) {
          parts.push({ text: entry, type: partType });
        }
        continue;
      }
      if (!entry || typeof entry !== 'object') {
        continue;
      }
      if (asString(entry.p) === 'status' && asString(entry.v) === 'FINISHED') {
        return { parts: [], finished: true, newType };
      }
      const content = asString(entry.content);
      if (!content) {
        continue;
      }
      const t = asString(entry.type).toUpperCase();
      if (t === 'THINK' || t === 'THINKING') {
        parts.push({ text: content, type: 'thinking' });
      } else if (t === 'RESPONSE') {
        parts.push({ text: content, type: 'text' });
      } else {
        parts.push({ text: content, type: partType });
      }
    }
    return { parts, finished: false, newType };
  }

  if (val && typeof val === 'object') {
    const resp = val.response && typeof val.response === 'object' ? val.response : val;
    if (Array.isArray(resp.fragments)) {
      for (const frag of resp.fragments) {
        if (!frag || typeof frag !== 'object') {
          continue;
        }
        const content = asString(frag.content);
        if (!content) {
          continue;
        }
        const t = asString(frag.type).toUpperCase();
        if (t === 'THINK' || t === 'THINKING') {
          newType = 'thinking';
          parts.push({ text: content, type: 'thinking' });
        } else if (t === 'RESPONSE') {
          newType = 'text';
          parts.push({ text: content, type: 'text' });
        } else {
          parts.push({ text: content, type: partType });
        }
      }
    }
  }
  return { parts, finished: false, newType };
}

function shouldSkipPath(pathValue) {
  if (pathValue === 'response/search_status') {
    return true;
  }
  for (const p of SKIP_PATTERNS) {
    if (pathValue.includes(p)) {
      return true;
    }
  }
  return false;
}

function isCitation(text) {
  return asString(text).trim().startsWith('[citation:');
}

function buildUsage(prompt, thinking, output) {
  const promptTokens = estimateTokens(prompt);
  const reasoningTokens = estimateTokens(thinking);
  const completionTokens = estimateTokens(output);
  return {
    prompt_tokens: promptTokens,
    completion_tokens: reasoningTokens + completionTokens,
    total_tokens: promptTokens + reasoningTokens + completionTokens,
    completion_tokens_details: {
      reasoning_tokens: reasoningTokens,
    },
  };
}

function estimateTokens(text) {
  const t = asString(text);
  if (!t) {
    return 0;
  }
  const n = Math.floor(Array.from(t).length / 4);
  return n < 1 ? 1 : n;
}

async function proxyToGo(req, res, rawBody) {
  const proto = asString(header(req, 'x-forwarded-proto')) || 'https';
  const host = asString(header(req, 'host'));
  const url = new URL(`${proto}://${host}${req.url || '/v1/chat/completions'}`);
  url.searchParams.set('__go', '1');

  const upstream = await fetch(url.toString(), {
    method: 'POST',
    headers: {
      authorization: asString(header(req, 'authorization')),
      'x-api-key': asString(header(req, 'x-api-key')),
      'x-ds2-target-account': asString(header(req, 'x-ds2-target-account')),
      'content-type': asString(header(req, 'content-type')) || 'application/json',
    },
    body: rawBody,
  });

  res.statusCode = upstream.status;
  upstream.headers.forEach((value, key) => {
    if (key.toLowerCase() === 'content-length') {
      return;
    }
    res.setHeader(key, value);
  });
  const bytes = Buffer.from(await upstream.arrayBuffer());
  res.end(bytes);
}

function writeOpenAIError(res, status, message) {
  res.statusCode = status;
  res.setHeader('Content-Type', 'application/json');
  res.end(
    JSON.stringify({
      error: {
        message,
        type: openAIErrorType(status),
      },
    }),
  );
}

function openAIErrorType(status) {
  switch (status) {
    case 400:
      return 'invalid_request_error';
    case 401:
      return 'authentication_error';
    case 403:
      return 'permission_error';
    case 429:
      return 'rate_limit_error';
    case 503:
      return 'service_unavailable_error';
    default:
      return status >= 500 ? 'api_error' : 'invalid_request_error';
  }
}

function toBool(v) {
  return v === true;
}

function asString(v) {
  if (typeof v === 'string') {
    return v.trim();
  }
  if (Array.isArray(v)) {
    return asString(v[0]);
  }
  if (v == null) {
    return '';
  }
  return String(v).trim();
}
