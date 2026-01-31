# -*- coding: utf-8 -*-
"""OpenAI 兼容路由"""
import json
import queue
import re
import threading
import time

from curl_cffi import requests as cffi_requests
from fastapi import APIRouter, HTTPException, Request
from fastapi.responses import JSONResponse, StreamingResponse

from core.config import CONFIG, logger
from core.auth import (
    determine_mode_and_token,
    get_auth_headers,
    release_account,
)
from core.deepseek import call_completion_endpoint
from core.session_manager import (
    create_session,
    get_pow,
    cleanup_account,
)
from core.models import get_model_config, get_openai_models_response
from core.stream_parser import (
    parse_deepseek_sse_line,
    extract_content_from_chunk,
    should_filter_citation,
)
from core.messages import messages_prepare

router = APIRouter()

# 添加保活超时配置（5秒）
KEEP_ALIVE_TIMEOUT = 5

# 预编译正则表达式（性能优化）
_CITATION_PATTERN = re.compile(r"^\[citation:")


# ----------------------------------------------------------------------
# 路由：/v1/models
# ----------------------------------------------------------------------
@router.get("/v1/models")
def list_models():
    data = get_openai_models_response()
    return JSONResponse(content=data, status_code=200)


# ----------------------------------------------------------------------
# 路由：/v1/chat/completions
# ----------------------------------------------------------------------
@router.post("/v1/chat/completions")
async def chat_completions(request: Request):
    try:
        # 处理 token 相关逻辑，若登录失败则直接返回错误响应
        try:
            determine_mode_and_token(request)
        except HTTPException as exc:
            return JSONResponse(
                status_code=exc.status_code, content={"error": exc.detail}
            )
        except Exception as exc:
            logger.error(f"[chat_completions] determine_mode_and_token 异常: {exc}")
            return JSONResponse(
                status_code=500, content={"error": "Account login failed."}
            )

        req_data = await request.json()
        model = req_data.get("model")
        messages = req_data.get("messages", [])
        if not model or not messages:
            raise HTTPException(
                status_code=400, detail="Request must include 'model' and 'messages'."
            )
        
        # 使用会话管理器获取模型配置
        thinking_enabled, search_enabled = get_model_config(model)
        if thinking_enabled is None:
            raise HTTPException(
                status_code=503, detail=f"Model '{model}' is not available."
            )
        
        # 使用 messages_prepare 函数构造最终 prompt
        final_prompt = messages_prepare(messages)
        session_id = create_session(request)
        if not session_id:
            raise HTTPException(status_code=401, detail="invalid token.")
        
        pow_resp = get_pow(request)
        if not pow_resp:
            raise HTTPException(
                status_code=401,
                detail="Failed to get PoW (invalid token or unknown error).",
            )
        
        headers = {**get_auth_headers(request), "x-ds-pow-response": pow_resp}
        payload = {
            "chat_session_id": session_id,
            "parent_message_id": None,
            "prompt": final_prompt,
            "ref_file_ids": [],
            "thinking_enabled": thinking_enabled,
            "search_enabled": search_enabled,
        }

        deepseek_resp = call_completion_endpoint(payload, headers, max_attempts=3)
        if not deepseek_resp:
            raise HTTPException(status_code=500, detail="Failed to get completion.")
        created_time = int(time.time())
        completion_id = f"{session_id}"

        # 流式响应（SSE）或普通响应
        if bool(req_data.get("stream", False)):
            if deepseek_resp.status_code != 200:
                deepseek_resp.close()
                return JSONResponse(
                    content=deepseek_resp.content, status_code=deepseek_resp.status_code
                )

            def sse_stream():
                # 智能超时配置
                STREAM_IDLE_TIMEOUT = 30  # 无新内容超时（秒）
                MAX_KEEPALIVE_COUNT = 10  # 最大连续 keepalive 次数
                
                try:
                    final_text = ""
                    final_thinking = ""
                    first_chunk_sent = False
                    result_queue = queue.Queue()
                    last_send_time = time.time()
                    last_content_time = time.time()  # 最后收到有效内容的时间
                    keepalive_count = 0  # 连续 keepalive 计数
                    has_content = False  # 是否收到过内容

                    def process_data():
                        nonlocal has_content
                        ptype = "text"
                        response_started = False  # 追踪是否已开始正式回复
                        logger.info(f"[sse_stream] 开始处理数据流, session_id={session_id}")
                        try:
                            for raw_line in deepseek_resp.iter_lines():
                                try:
                                    line = raw_line.decode("utf-8")
                                except Exception as e:
                                    logger.warning(f"[sse_stream] 解码失败: {e}")
                                    error_type = "thinking" if ptype == "thinking" else "text"
                                    busy_content_str = f'{{"choices":[{{"index":0,"delta":{{"content":"解码失败，请稍候再试","type":"{error_type}"}}}}],"model":"","chunk_token_usage":1,"created":0,"message_id":-1,"parent_id":-1}}'
                                    try:
                                        busy_content = json.loads(busy_content_str)
                                        result_queue.put(busy_content)
                                    except json.JSONDecodeError:
                                        result_queue.put({"choices": [{"index": 0, "delta": {"content": "解码失败", "type": "text"}}]})
                                    result_queue.put(None)
                                    break
                                if not line:
                                    continue
                                if line.startswith("data:"):
                                    data_str = line[5:].strip()
                                    if data_str == "[DONE]":
                                        result_queue.put(None)
                                        break
                                    try:
                                        chunk = json.loads(data_str)
                                        
                                        # 检测内容审核/敏感词阻止
                                        if "error" in chunk or chunk.get("code") == "content_filter":
                                            logger.warning(f"[sse_stream] 检测到内容过滤: {chunk}")
                                            result_queue.put({"choices": [{"index": 0, "finish_reason": "content_filter"}]})
                                            result_queue.put(None)
                                            return
                                        
                                        # logger.info(f"[sse_stream] RAW 原始chunk: {data_str[:300]}")
                                        
                                        if "v" in chunk:
                                            v_value = chunk["v"]
                                            content = ""
                                            chunk_path = chunk.get("p", "")
                                            
                                            if chunk_path == "response/search_status":
                                                continue
                                            
                                            # 检测是否开始正式回复
                                            # 只有当 fragments 包含 RESPONSE 类型时才认为开始正式回复
                                            if "response/fragments" in chunk_path and isinstance(v_value, list):
                                                for frag in v_value:
                                                    if isinstance(frag, dict) and frag.get("type", "").upper() == "RESPONSE":
                                                        response_started = True
                                                        break
                                            
                                            # 确定当前类型
                                            if chunk_path == "response/thinking_content":
                                                ptype = "thinking"
                                            elif chunk_path == "response/content":
                                                ptype = "text"
                                                response_started = True  # 有 response/content 也意味着开始正式回复
                                            elif "response/fragments" in chunk_path:
                                                # fragments 的类型由内层 type 决定，默认用之前的 ptype
                                                pass
                                            elif not chunk_path:
                                                # 没有 p 字段的内容：
                                                # - reasoner 模式下，未开始正式回复前是 thinking
                                                # - 开始正式回复后是 text
                                                if thinking_enabled and not response_started:
                                                    ptype = "thinking"
                                                else:
                                                    ptype = "text"
                                            
                                            # logger.info(f"[sse_stream] ptype={ptype}, response_started={response_started}, chunk_path='{chunk_path}', v_type={type(v_value).__name__}, v={str(v_value)[:50]}")
                                            if isinstance(v_value, str):
                                                # 检查是否是 FINISHED 状态
                                                if v_value == "FINISHED":
                                                    result_queue.put({"choices": [{"index": 0, "finish_reason": "stop"}]})
                                                    result_queue.put(None)
                                                    return
                                                content = v_value
                                                if content:
                                                    has_content = True
                                            elif isinstance(v_value, list):
                                                # DeepSeek 可能发送嵌套列表格式
                                                # 需要递归提取内容
                                                def extract_content_recursive(items, default_type="text"):
                                                    """递归提取列表中的内容"""
                                                    extracted = []
                                                    for item in items:
                                                        if not isinstance(item, dict):
                                                            continue
                                                        
                                                        # 检查是否是 FINISHED 状态
                                                        if item.get("p") == "status" and item.get("v") == "FINISHED":
                                                            return None  # 信号结束
                                                        
                                                        item_p = item.get("p", "")
                                                        item_v = item.get("v")
                                                        
                                                        # 跳过搜索状态
                                                        if item_p == "response/search_status":
                                                            continue
                                                        
                                                        # 确定类型
                                                        if "thinking" in item_p:
                                                            content_type = "thinking"
                                                        elif "content" in item_p or item_p == "response":
                                                            content_type = "text"
                                                        else:
                                                            content_type = default_type
                                                        
                                                        # 处理不同的 v 类型
                                                        if isinstance(item_v, str):
                                                            if item_v and item_v != "FINISHED":
                                                                extracted.append((item_v, content_type))
                                                        elif isinstance(item_v, list):
                                                            # 内层可能是 [{"content": "text", "type": "THINK/RESPONSE", ...}] 格式
                                                            for inner in item_v:
                                                                if isinstance(inner, dict):
                                                                    # 检查内层的 type 字段
                                                                    inner_type = inner.get("type", "").upper()
                                                                    # logger.info(f"[sse_stream] 内层 type={inner_type}, content={str(inner.get('content', ''))[:50]}")
                                                                    # DeepSeek 使用 THINK 而不是 THINKING
                                                                    if inner_type == "THINK" or inner_type == "THINKING":
                                                                        final_type = "thinking"
                                                                    elif inner_type == "RESPONSE":
                                                                        final_type = "text"
                                                                    else:
                                                                        final_type = content_type  # 继承外层类型
                                                                    
                                                                    content = inner.get("content", "")
                                                                    if content:
                                                                        extracted.append((content, final_type))
                                                                elif isinstance(inner, str) and inner:
                                                                    extracted.append((inner, content_type))
                                                    return extracted
                                                
                                                result = extract_content_recursive(v_value, ptype)
                                                
                                                if result is None:
                                                    # FINISHED 信号
                                                    result_queue.put({"choices": [{"index": 0, "finish_reason": "stop"}]})
                                                    result_queue.put(None)
                                                    return
                                                
                                                for content_text, content_type in result:
                                                    if content_text:
                                                        logger.debug(f"[sse_stream] 提取内容: {content_text[:30] if len(content_text) > 30 else content_text}")
                                                        chunk = {
                                                            "choices": [{
                                                                "index": 0,
                                                                "delta": {"content": content_text, "type": content_type}
                                                            }],
                                                            "model": "",
                                                            "chunk_token_usage": len(content_text) // 4,
                                                            "created": 0,
                                                            "message_id": -1,
                                                            "parent_id": -1
                                                        }
                                                        result_queue.put(chunk)
                                                        has_content = True
                                                continue
                                            
                                            unified_chunk = {
                                                "choices": [{
                                                    "index": 0,
                                                    "delta": {"content": content, "type": ptype}
                                                }],
                                                "model": "",
                                                "chunk_token_usage": len(content) // 4,
                                                "created": 0,
                                                "message_id": -1,
                                                "parent_id": -1
                                            }
                                            result_queue.put(unified_chunk)
                                    except Exception as e:
                                        logger.warning(f"[sse_stream] 无法解析: {data_str}, 错误: {e}")
                                        error_type = "thinking" if ptype == "thinking" else "text"
                                        busy_content_str = f'{{"choices":[{{"index":0,"delta":{{"content":"解析失败，请稍候再试","type":"{error_type}"}}}}],"model":"","chunk_token_usage":1,"created":0,"message_id":-1,"parent_id":-1}}'
                                        try:
                                            busy_content = json.loads(busy_content_str)
                                            result_queue.put(busy_content)
                                        except json.JSONDecodeError:
                                            result_queue.put({"choices": [{"index": 0, "delta": {"content": "解析失败", "type": "text"}}]})
                                        result_queue.put(None)
                                        break
                        except Exception as e:
                            logger.warning(f"[sse_stream] 错误: {e}")
                            try:
                                error_response = {"choices": [{"index": 0, "delta": {"content": "服务器错误，请稍候再试", "type": "text"}}]}
                                result_queue.put(error_response)
                            except Exception:
                                pass
                            result_queue.put(None)
                        finally:
                            deepseek_resp.close()

                    process_thread = threading.Thread(target=process_data)
                    process_thread.start()

                    while True:
                        current_time = time.time()
                        
                        # 智能超时检测：如果已有内容且长时间无新数据，强制结束
                        if has_content and (current_time - last_content_time) > STREAM_IDLE_TIMEOUT:
                            logger.warning(f"[sse_stream] 智能超时: 已有内容但 {STREAM_IDLE_TIMEOUT}s 无新数据，强制结束")
                            break
                        
                        # 连续 keepalive 检测：如果已有内容且连续多次 keepalive，强制结束
                        if has_content and keepalive_count >= MAX_KEEPALIVE_COUNT:
                            logger.warning(f"[sse_stream] 智能超时: 连续 {MAX_KEEPALIVE_COUNT} 次 keepalive，强制结束")
                            break
                        
                        if current_time - last_send_time >= KEEP_ALIVE_TIMEOUT:
                            yield ": keep-alive\n\n"
                            last_send_time = current_time
                            keepalive_count += 1
                            continue
                            
                        try:
                            chunk = result_queue.get(timeout=0.05)
                            keepalive_count = 0  # 重置 keepalive 计数
                            
                            if chunk is None:
                                prompt_tokens = len(final_prompt) // 4
                                thinking_tokens = len(final_thinking) // 4
                                completion_tokens = len(final_text) // 4
                                usage = {
                                    "prompt_tokens": prompt_tokens,
                                    "completion_tokens": thinking_tokens + completion_tokens,
                                    "total_tokens": prompt_tokens + thinking_tokens + completion_tokens,
                                    "completion_tokens_details": {"reasoning_tokens": thinking_tokens},
                                }
                                finish_chunk = {
                                    "id": completion_id,
                                    "object": "chat.completion.chunk",
                                    "created": created_time,
                                    "model": model,
                                    "choices": [{"delta": {}, "index": 0, "finish_reason": "stop"}],
                                    "usage": usage,
                                }
                                yield f"data: {json.dumps(finish_chunk, ensure_ascii=False)}\n\n"
                                yield "data: [DONE]\n\n"
                                last_send_time = current_time
                                break
                                
                            new_choices = []
                            for choice in chunk.get("choices", []):
                                delta = choice.get("delta", {})
                                ctype = delta.get("type")
                                ctext = delta.get("content", "")
                                if choice.get("finish_reason") == "backend_busy":
                                    ctext = "服务器繁忙，请稍候再试"
                                if choice.get("finish_reason") == "content_filter":
                                    # 内容过滤，正常结束
                                    pass
                                if search_enabled and ctext.startswith("[citation:"):
                                    ctext = ""
                                if ctype == "thinking":
                                    if thinking_enabled:
                                        final_thinking += ctext
                                elif ctype == "text":
                                    final_text += ctext
                                delta_obj = {}
                                if not first_chunk_sent:
                                    delta_obj["role"] = "assistant"
                                    first_chunk_sent = True
                                if ctype == "thinking":
                                    if thinking_enabled:
                                        delta_obj["reasoning_content"] = ctext
                                elif ctype == "text":
                                    delta_obj["content"] = ctext
                                if delta_obj:
                                    new_choices.append({"delta": delta_obj, "index": choice.get("index", 0)})
                                    
                            if new_choices:
                                last_content_time = current_time  # 更新最后内容时间
                                out_chunk = {
                                    "id": completion_id,
                                    "object": "chat.completion.chunk",
                                    "created": created_time,
                                    "model": model,
                                    "choices": new_choices,
                                }
                                yield f"data: {json.dumps(out_chunk, ensure_ascii=False)}\n\n"
                                last_send_time = current_time
                        except queue.Empty:
                            continue
                            
                    # 如果是超时退出，也发送结束标记
                    if has_content:
                        prompt_tokens = len(final_prompt) // 4
                        thinking_tokens = len(final_thinking) // 4
                        completion_tokens = len(final_text) // 4
                        usage = {
                            "prompt_tokens": prompt_tokens,
                            "completion_tokens": thinking_tokens + completion_tokens,
                            "total_tokens": prompt_tokens + thinking_tokens + completion_tokens,
                            "completion_tokens_details": {"reasoning_tokens": thinking_tokens},
                        }
                        finish_chunk = {
                            "id": completion_id,
                            "object": "chat.completion.chunk",
                            "created": created_time,
                            "model": model,
                            "choices": [{"delta": {}, "index": 0, "finish_reason": "stop"}],
                            "usage": usage,
                        }
                        yield f"data: {json.dumps(finish_chunk, ensure_ascii=False)}\n\n"
                        yield "data: [DONE]\n\n"
                        
                except Exception as e:
                    logger.error(f"[sse_stream] 异常: {e}")
                finally:
                    cleanup_account(request)

            return StreamingResponse(
                sse_stream(),
                media_type="text/event-stream",
                headers={"Content-Type": "text/event-stream"},
            )
        else:
            # 非流式响应处理
            think_list = []
            text_list = []
            result = None

            data_queue = queue.Queue()

            def collect_data():
                nonlocal result
                ptype = "text"
                try:
                    for raw_line in deepseek_resp.iter_lines():
                        try:
                            line = raw_line.decode("utf-8")
                        except Exception as e:
                            logger.warning(f"[chat_completions] 解码失败: {e}")
                            if ptype == "thinking":
                                think_list.append("解码失败，请稍候再试")
                            else:
                                text_list.append("解码失败，请稍候再试")
                            data_queue.put(None)
                            break
                        if not line:
                            continue
                        if line.startswith("data:"):
                            data_str = line[5:].strip()
                            if data_str == "[DONE]":
                                data_queue.put(None)
                                break
                            try:
                                chunk = json.loads(data_str)
                                if "v" in chunk:
                                    v_value = chunk["v"]
                                    if "p" in chunk and chunk.get("p") == "response/search_status":
                                        continue
                                    if "p" in chunk and chunk.get("p") == "response/thinking_content":
                                        ptype = "thinking"
                                    elif "p" in chunk and chunk.get("p") == "response/content":
                                        ptype = "text"
                                    if isinstance(v_value, str):
                                        if search_enabled and v_value.startswith("[citation:"):
                                            continue
                                        if ptype == "thinking":
                                            think_list.append(v_value)
                                        else:
                                            text_list.append(v_value)
                                    elif isinstance(v_value, list):
                                        for item in v_value:
                                            if item.get("p") == "status" and item.get("v") == "FINISHED":
                                                final_reasoning = "".join(think_list)
                                                final_content = "".join(text_list)
                                                prompt_tokens = len(final_prompt) // 4
                                                reasoning_tokens = len(final_reasoning) // 4
                                                completion_tokens = len(final_content) // 4
                                                result = {
                                                    "id": completion_id,
                                                    "object": "chat.completion",
                                                    "created": created_time,
                                                    "model": model,
                                                    "choices": [{
                                                        "index": 0,
                                                        "message": {
                                                            "role": "assistant",
                                                            "content": final_content,
                                                            "reasoning_content": final_reasoning,
                                                        },
                                                        "finish_reason": "stop",
                                                    }],
                                                    "usage": {
                                                        "prompt_tokens": prompt_tokens,
                                                        "completion_tokens": reasoning_tokens + completion_tokens,
                                                        "total_tokens": prompt_tokens + reasoning_tokens + completion_tokens,
                                                        "completion_tokens_details": {"reasoning_tokens": reasoning_tokens},
                                                    },
                                                }
                                                data_queue.put("DONE")
                                                return
                            except Exception as e:
                                logger.warning(f"[collect_data] 无法解析: {data_str}, 错误: {e}")
                                if ptype == "thinking":
                                    think_list.append("解析失败，请稍候再试")
                                else:
                                    text_list.append("解析失败，请稍候再试")
                                data_queue.put(None)
                                break
                except Exception as e:
                    logger.warning(f"[collect_data] 错误: {e}")
                    if ptype == "thinking":
                        think_list.append("处理失败，请稍候再试")
                    else:
                        text_list.append("处理失败，请稍候再试")
                    data_queue.put(None)
                finally:
                    deepseek_resp.close()
                    if result is None:
                        final_content = "".join(text_list)
                        final_reasoning = "".join(think_list)
                        prompt_tokens = len(final_prompt) // 4
                        reasoning_tokens = len(final_reasoning) // 4
                        completion_tokens = len(final_content) // 4
                        result = {
                            "id": completion_id,
                            "object": "chat.completion",
                            "created": created_time,
                            "model": model,
                            "choices": [{
                                "index": 0,
                                "message": {
                                    "role": "assistant",
                                    "content": final_content,
                                    "reasoning_content": final_reasoning,
                                },
                                "finish_reason": "stop",
                            }],
                            "usage": {
                                "prompt_tokens": prompt_tokens,
                                "completion_tokens": reasoning_tokens + completion_tokens,
                                "total_tokens": prompt_tokens + reasoning_tokens + completion_tokens,
                            },
                        }
                    data_queue.put("DONE")

            collect_thread = threading.Thread(target=collect_data)
            collect_thread.start()

            def generate():
                last_send_time = time.time()
                while True:
                    current_time = time.time()
                    if current_time - last_send_time >= KEEP_ALIVE_TIMEOUT:
                        yield ""
                        last_send_time = current_time
                    if not collect_thread.is_alive() and result is not None:
                        yield json.dumps(result)
                        break
                    time.sleep(0.1)

            return StreamingResponse(generate(), media_type="application/json")
    except HTTPException as exc:
        return JSONResponse(status_code=exc.status_code, content={"error": exc.detail})
    except Exception as exc:
        logger.error(f"[chat_completions] 未知异常: {exc}")
        return JSONResponse(status_code=500, content={"error": "Internal Server Error"})
    finally:
        cleanup_account(request)
