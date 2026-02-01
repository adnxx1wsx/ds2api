# -*- coding: utf-8 -*-
"""Claude API 路由"""
import json
import random
import time

from curl_cffi import requests as cffi_requests
from fastapi import APIRouter, HTTPException, Request
from fastapi.responses import JSONResponse, StreamingResponse

from core.config import CONFIG, logger
from core.auth import (
    determine_mode_and_token,
    get_auth_headers,
)
from core.deepseek import call_completion_endpoint
from core.session_manager import (
    create_session,
    get_pow,
    cleanup_account,
)
from core.models import get_model_config, get_claude_models_response
from core.sse_parser import (
    parse_deepseek_sse_line,
    parse_sse_chunk_for_content,
    extract_content_from_chunk,
    collect_deepseek_response,
    parse_tool_calls,
)
from core.constants import STREAM_IDLE_TIMEOUT
from core.utils import estimate_tokens
from core.messages import (
    messages_prepare,
    convert_claude_to_deepseek,
    CLAUDE_DEFAULT_MODEL,
)

router = APIRouter()



# ----------------------------------------------------------------------
# 通过 OpenAI 接口调用 Claude
# ----------------------------------------------------------------------
async def call_claude_via_openai(request: Request, claude_payload: dict):
    """通过现有OpenAI接口调用Claude（实际调用DeepSeek）"""
    deepseek_payload = convert_claude_to_deepseek(claude_payload)

    try:
        session_id = create_session(request)
        if not session_id:
            raise HTTPException(status_code=401, detail="invalid token.")

        pow_resp = get_pow(request)
        if not pow_resp:
            raise HTTPException(
                status_code=401,
                detail="Failed to get PoW (invalid token or unknown error).",
            )

        model = deepseek_payload.get("model", "deepseek-chat")
        messages = deepseek_payload.get("messages", [])

        # 使用会话管理器获取模型配置
        thinking_enabled, search_enabled = get_model_config(model)
        if thinking_enabled is None:
            # 默认配置
            thinking_enabled = False
            search_enabled = False

        final_prompt = messages_prepare(messages)

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
        return deepseek_resp

    except Exception as e:
        logger.error(f"[call_claude_via_openai] 调用失败: {e}")
        return None


# ----------------------------------------------------------------------
# Claude 路由：模型列表
# ----------------------------------------------------------------------
@router.get("/anthropic/v1/models")
def list_claude_models():
    data = get_claude_models_response()
    return JSONResponse(content=data, status_code=200)


# ----------------------------------------------------------------------
# Claude 路由：/anthropic/v1/messages
# ----------------------------------------------------------------------
@router.post("/anthropic/v1/messages")
async def claude_messages(request: Request):
    try:
        try:
            determine_mode_and_token(request)
        except HTTPException as exc:
            return JSONResponse(
                status_code=exc.status_code, content={"error": exc.detail}
            )
        except Exception as exc:
            logger.error(f"[claude_messages] determine_mode_and_token 异常: {exc}")
            return JSONResponse(
                status_code=500, content={"error": "Claude authentication failed."}
            )

        req_data = await request.json()
        model = req_data.get("model")
        messages = req_data.get("messages", [])

        if not model or not messages:
            raise HTTPException(
                status_code=400, detail="Request must include 'model' and 'messages'."
            )

        # 标准化消息内容
        normalized_messages = []
        for message in messages:
            normalized_message = message.copy()
            if isinstance(message.get("content"), list):
                content_parts = []
                for content_block in message["content"]:
                    if content_block.get("type") == "text" and "text" in content_block:
                        content_parts.append(content_block["text"])
                    elif content_block.get("type") == "tool_result":
                        if "content" in content_block:
                            content_parts.append(str(content_block["content"]))
                if content_parts:
                    normalized_message["content"] = "\n".join(content_parts)
                elif isinstance(message.get("content"), list) and message["content"]:
                    normalized_message["content"] = message["content"]
                else:
                    normalized_message["content"] = ""
            normalized_messages.append(normalized_message)

        tools_requested = req_data.get("tools") or []
        has_tools = len(tools_requested) > 0

        payload = req_data.copy()
        payload["messages"] = normalized_messages.copy()

        # 如果有工具定义，添加工具使用指导的系统消息
        if has_tools and not any(m.get("role") == "system" for m in payload["messages"]):
            tool_schemas = []
            for tool in tools_requested:
                tool_name = tool.get("name", "unknown")
                tool_desc = tool.get("description", "No description available")
                schema = tool.get("input_schema", {})

                tool_info = f"Tool: {tool_name}\nDescription: {tool_desc}"
                if "properties" in schema:
                    props = []
                    required = schema.get("required", [])
                    for prop_name, prop_info in schema["properties"].items():
                        prop_type = prop_info.get("type", "string")
                        is_req = " (required)" if prop_name in required else ""
                        props.append(f"  - {prop_name}: {prop_type}{is_req}")
                    if props:
                        tool_info += f"\nParameters:\n{chr(10).join(props)}"
                tool_schemas.append(tool_info)

            system_message = {
                "role": "system",
                "content": f"""You are Claude, a helpful AI assistant. You have access to these tools:

{chr(10).join(tool_schemas)}

When you need to use tools, you can call multiple tools in a single response. Use this format:

{{"tool_calls": [
  {{"name": "tool1", "input": {{"param": "value"}}}},
  {{"name": "tool2", "input": {{"param": "value"}}}}
]}}

IMPORTANT: You can call multiple tools in ONE response.

Remember: Output ONLY the JSON, no other text. The response must start with {{ and end with ]}}""",
            }
            payload["messages"].insert(0, system_message)

        deepseek_resp = await call_claude_via_openai(request, payload)
        if not deepseek_resp:
            raise HTTPException(status_code=500, detail="Failed to get Claude response.")

        if deepseek_resp.status_code != 200:
            deepseek_resp.close()
            return JSONResponse(
                status_code=500,
                content={"error": {"type": "api_error", "message": "Failed to get response"}},
            )

        # 流式响应或普通响应
        if bool(req_data.get("stream", False)):

            def claude_sse_stream():
                # 使用导入的常量（不再本地定义）
                try:
                    message_id = f"msg_{int(time.time())}_{random.randint(1000, 9999)}"
                    input_tokens = sum(len(str(m.get("content", ""))) for m in messages) // 4
                    output_tokens = 0
                    full_response_text = ""
                    last_content_time = time.time()
                    has_content = False


                    for line in deepseek_resp.iter_lines():
                        current_time = time.time()
                        
                        # 智能超时检测
                        if has_content and (current_time - last_content_time) > STREAM_IDLE_TIMEOUT:
                            logger.warning(f"[claude_sse_stream] 智能超时: 已有内容但 {STREAM_IDLE_TIMEOUT}s 无新数据，强制结束")
                            break
                        
                        if not line:
                            continue
                        try:
                            line_str = line.decode("utf-8")
                        except Exception:
                            continue

                        if line_str.startswith("data:"):
                            data_str = line_str[5:].strip()
                            if data_str == "[DONE]":
                                break

                            try:
                                chunk = json.loads(data_str)
                                
                                # 检测内容审核/敏感词阻止
                                if "error" in chunk or chunk.get("code") == "content_filter":
                                    logger.warning(f"[claude_sse_stream] 检测到内容过滤: {chunk}")
                                    break
                                
                                if "v" in chunk and isinstance(chunk["v"], str):
                                    content = chunk["v"]
                                    # 检查是否是 FINISHED 状态
                                    if content == "FINISHED":
                                        break
                                    full_response_text += content
                                    if content:
                                        has_content = True
                                        last_content_time = current_time
                                elif "v" in chunk and isinstance(chunk["v"], list):
                                    for item in chunk["v"]:
                                        if item.get("p") == "status" and item.get("v") == "FINISHED":
                                            break
                            except (json.JSONDecodeError, KeyError):
                                continue

                    # 发送Claude格式的事件
                    message_start = {
                        "type": "message_start",
                        "message": {
                            "id": message_id,
                            "type": "message",
                            "role": "assistant",
                            "model": model,
                            "content": [],
                            "stop_reason": None,
                            "stop_sequence": None,
                            "usage": {"input_tokens": input_tokens, "output_tokens": 0},
                        },
                    }
                    yield f"data: {json.dumps(message_start)}\n\n"

                    # 检查工具调用
                    # 使用公共函数检测工具调用
                    detected_tools = parse_tool_calls(full_response_text, tools_requested)

                    content_index = 0
                    if detected_tools:
                        stop_reason = "tool_use"
                        for tool_info in detected_tools:
                            tool_use_id = f"toolu_{int(time.time())}_{random.randint(1000, 9999)}_{content_index}"
                            tool_name = tool_info["name"]
                            tool_input = tool_info["input"]

                            yield f"data: {json.dumps({'type': 'content_block_start', 'index': content_index, 'content_block': {'type': 'tool_use', 'id': tool_use_id, 'name': tool_name, 'input': tool_input}})}\n\n"
                            yield f"data: {json.dumps({'type': 'content_block_stop', 'index': content_index})}\n\n"

                            content_index += 1
                            output_tokens += len(str(tool_input)) // 4
                    else:
                        stop_reason = "end_turn"
                        if full_response_text:
                            yield f"data: {json.dumps({'type': 'content_block_start', 'index': 0, 'content_block': {'type': 'text', 'text': ''}})}\n\n"
                            yield f"data: {json.dumps({'type': 'content_block_delta', 'index': 0, 'delta': {'type': 'text_delta', 'text': full_response_text}})}\n\n"
                            yield f"data: {json.dumps({'type': 'content_block_stop', 'index': 0})}\n\n"
                            output_tokens += len(full_response_text) // 4

                    yield f"data: {json.dumps({'type': 'message_delta', 'delta': {'stop_reason': stop_reason, 'stop_sequence': None}, 'usage': {'output_tokens': output_tokens}})}\n\n"
                    yield f"data: {json.dumps({'type': 'message_stop'})}\n\n"

                except Exception as e:
                    logger.error(f"[claude_sse_stream] 异常: {e}")
                    error_event = {
                        "type": "error",
                        "error": {"type": "api_error", "message": f"Stream processing error: {str(e)}"},
                    }
                    yield f"data: {json.dumps(error_event)}\n\n"
                finally:
                    try:
                        deepseek_resp.close()
                    except Exception:
                        pass
                    cleanup_account(request)

            return StreamingResponse(
                claude_sse_stream(),
                media_type="text/event-stream",
                headers={"Content-Type": "text/event-stream"},
            )
        else:
            # 非流式响应处理
            try:
                final_content = ""
                final_reasoning = ""

                for line in deepseek_resp.iter_lines():
                    if not line:
                        continue
                    try:
                        line_str = line.decode("utf-8")
                    except Exception as e:
                        logger.warning(f"[claude_messages] 行解码失败: {e}")
                        continue

                    if line_str.startswith("data:"):
                        data_str = line_str[5:].strip()
                        if data_str == "[DONE]":
                            break

                        try:
                            chunk = json.loads(data_str)
                            if "v" in chunk:
                                v_value = chunk["v"]
                                if "p" in chunk and chunk.get("p") == "response/search_status":
                                    continue
                                ptype = "text"
                                if "p" in chunk and chunk.get("p") == "response/thinking_content":
                                    ptype = "thinking"
                                elif "p" in chunk and chunk.get("p") == "response/content":
                                    ptype = "text"
                                if isinstance(v_value, str):
                                    if ptype == "thinking":
                                        final_reasoning += v_value
                                    else:
                                        final_content += v_value
                                elif isinstance(v_value, list):
                                    for item in v_value:
                                        if item.get("p") == "status" and item.get("v") == "FINISHED":
                                            break
                        except json.JSONDecodeError as e:
                            logger.warning(f"[claude_messages] JSON解析失败: {e}")
                            continue
                        except Exception as e:
                            logger.warning(f"[claude_messages] chunk处理失败: {e}")
                            continue

                try:
                    deepseek_resp.close()
                except Exception as e:
                    logger.warning(f"[claude_messages] 关闭响应异常: {e}")

                # 检查工具调用
                detected_tools = parse_tool_calls(final_content, tools_requested)

                # 构造响应
                claude_response = {
                    "id": f"msg_{int(time.time())}_{random.randint(1000, 9999)}",
                    "type": "message",
                    "role": "assistant",
                    "model": model,
                    "content": [],
                    "stop_reason": "tool_use" if detected_tools else "end_turn",
                    "stop_sequence": None,
                    "usage": {
                        "input_tokens": len(str(normalized_messages)) // 4,
                        "output_tokens": (len(final_content) + len(final_reasoning)) // 4,
                    },
                }

                if final_reasoning:
                    claude_response["content"].append({"type": "thinking", "thinking": final_reasoning})

                if detected_tools:
                    for i, tool_info in enumerate(detected_tools):
                        tool_use_id = f"toolu_{int(time.time())}_{random.randint(1000, 9999)}_{i}"
                        claude_response["content"].append({
                            "type": "tool_use",
                            "id": tool_use_id,
                            "name": tool_info["name"],
                            "input": tool_info["input"],
                        })
                else:
                    if final_content or not final_reasoning:
                        claude_response["content"].append({
                            "type": "text",
                            "text": final_content or "抱歉，没有生成有效的响应内容。",
                        })

                return JSONResponse(content=claude_response, status_code=200)

            except Exception as e:
                logger.error(f"[claude_messages] 非流式响应处理异常: {e}")
                try:
                    deepseek_resp.close()
                except Exception as close_e:
                    logger.warning(f"[claude_messages] 关闭响应异常2: {close_e}")
                return JSONResponse(
                    status_code=500,
                    content={"error": {"type": "api_error", "message": "Response processing error"}},
                )

    except HTTPException as exc:
        return JSONResponse(
            status_code=exc.status_code,
            content={"error": {"type": "invalid_request_error", "message": exc.detail}},
        )
    except Exception as exc:
        logger.error(f"[claude_messages] 未知异常: {exc}")
        return JSONResponse(
            status_code=500,
            content={"error": {"type": "api_error", "message": "Internal Server Error"}},
        )
    finally:
        cleanup_account(request)


# ----------------------------------------------------------------------
# Claude 路由：/anthropic/v1/messages/count_tokens
# ----------------------------------------------------------------------
@router.post("/anthropic/v1/messages/count_tokens")
async def claude_count_tokens(request: Request):
    try:
        try:
            determine_mode_and_token(request)
        except HTTPException as exc:
            return JSONResponse(status_code=exc.status_code, content={"error": exc.detail})
        except Exception as exc:
            logger.error(f"[claude_count_tokens] determine_mode_and_token 异常: {exc}")
            return JSONResponse(status_code=500, content={"error": "Claude authentication failed."})

        req_data = await request.json()
        model = req_data.get("model")
        messages = req_data.get("messages", [])
        system = req_data.get("system", "")

        if not model or not messages:
            raise HTTPException(
                status_code=400, detail="Request must include 'model' and 'messages'."
            )

        input_tokens = 0

        if system:
            input_tokens += estimate_tokens(system)

        for message in messages:
            content = message.get("content", "")
            input_tokens += 2  # 角色标记

            if isinstance(content, list):
                for content_block in content:
                    if isinstance(content_block, dict):
                        if content_block.get("type") == "text":
                            input_tokens += estimate_tokens(content_block.get("text", ""))
                        elif content_block.get("type") == "tool_result":
                            input_tokens += estimate_tokens(content_block.get("content", ""))
                        else:
                            input_tokens += estimate_tokens(str(content_block))
                    else:
                        input_tokens += estimate_tokens(str(content_block))
            else:
                input_tokens += estimate_tokens(content)

        tools = req_data.get("tools", [])
        if tools:
            for tool in tools:
                input_tokens += estimate_tokens(tool.get("name", ""))
                input_tokens += estimate_tokens(tool.get("description", ""))
                input_schema = tool.get("input_schema", {})
                input_tokens += estimate_tokens(json.dumps(input_schema, ensure_ascii=False))

        response = {"input_tokens": max(1, input_tokens)}
        return JSONResponse(content=response, status_code=200)

    except HTTPException as exc:
        return JSONResponse(
            status_code=exc.status_code,
            content={"error": {"type": "invalid_request_error", "message": exc.detail}},
        )
    except Exception as exc:
        logger.error(f"[claude_count_tokens] 未知异常: {exc}")
        return JSONResponse(
            status_code=500,
            content={"error": {"type": "api_error", "message": "Internal Server Error"}},
        )
