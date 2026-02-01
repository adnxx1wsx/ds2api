# -*- coding: utf-8 -*-
"""DeepSeek SSE 流解析模块

这个模块包含解析 DeepSeek SSE 响应的公共逻辑，供 openai.py、claude.py 和 accounts.py 共用。
合并了原 sse_parser.py 和 stream_parser.py 的功能。
"""
import json
import re
from typing import List, Tuple, Optional, Dict, Any, Generator

from .config import logger
from .constants import SKIP_PATTERNS

# 预编译正则表达式
_TOOL_CALL_PATTERN = re.compile(r'\{\s*["\']tool_calls["\']\s*:\s*\[(.*?)\]\s*\}', re.DOTALL)
_CITATION_PATTERN = re.compile(r"^\[citation:")


# ----------------------------------------------------------------------
# 基础解析函数
# ----------------------------------------------------------------------

def parse_deepseek_sse_line(raw_line: bytes) -> Optional[Dict[str, Any]]:
    """解析 DeepSeek SSE 行
    
    Args:
        raw_line: 原始字节行
        
    Returns:
        解析后的 chunk 字典，如果解析失败或应跳过则返回 None
    """
    try:
        line = raw_line.decode("utf-8")
    except Exception as e:
        logger.warning(f"[parse_deepseek_sse_line] 解码失败: {e}")
        return None
    
    if not line or not line.startswith("data:"):
        return None
    
    data_str = line[5:].strip()
    
    if data_str == "[DONE]":
        return {"type": "done"}
    
    try:
        chunk = json.loads(data_str)
        return chunk
    except json.JSONDecodeError as e:
        logger.warning(f"[parse_deepseek_sse_line] JSON解析失败: {e}")
        return None


def should_skip_chunk(chunk_path: str) -> bool:
    """判断是否应该跳过这个 chunk（状态相关，不是内容）"""
    if chunk_path == "response/search_status":
        return True
    return any(kw in chunk_path for kw in SKIP_PATTERNS)


def is_response_finished(chunk_path: str, v_value: Any) -> bool:
    """判断是否是响应结束信号"""
    return chunk_path == "response/status" and isinstance(v_value, str) and v_value == "FINISHED"


def is_finished_signal(chunk_path: str, v_value: str) -> bool:
    """判断字符串 v_value 是否是结束信号"""
    return v_value == "FINISHED" and (not chunk_path or chunk_path == "status")


def is_search_result(item: dict) -> bool:
    """判断是否是搜索结果项（url/title/snippet）"""
    return "url" in item and "title" in item


# ----------------------------------------------------------------------
# 内容提取函数
# ----------------------------------------------------------------------

def extract_content_from_item(item: dict, default_type: str = "text") -> Optional[Tuple[str, str]]:
    """从包含 content 和 type 的项中提取内容
    
    返回 (content, content_type) 或 None
    """
    if "content" in item and "type" in item:
        inner_type = item.get("type", "").upper()
        content = item.get("content", "")
        if content:
            if inner_type == "THINK" or inner_type == "THINKING":
                return (content, "thinking")
            elif inner_type == "RESPONSE":
                return (content, "text")
            else:
                return (content, default_type)
    return None


def extract_content_recursive(items: List[Dict], default_type: str = "text") -> Optional[List[Tuple[str, str]]]:
    """递归提取列表中的内容
    
    返回 [(content, content_type), ...] 列表，
    如果遇到 FINISHED 信号返回 None
    """
    extracted: List[Tuple[str, str]] = []
    for item in items:
        if not isinstance(item, dict):
            continue
        
        item_p = item.get("p", "")
        item_v = item.get("v")
        
        # 跳过搜索结果项
        if is_search_result(item):
            continue
        
        # 只有当 p="status" (精确匹配) 且 v="FINISHED" 才认为是真正结束
        if item_p == "status" and item_v == "FINISHED":
            return None  # 信号结束
        
        # 跳过状态相关
        if should_skip_chunk(item_p):
            continue
        
        # 直接处理包含 content 和 type 的项
        result = extract_content_from_item(item, default_type)
        if result:
            extracted.append(result)
            continue
        
        # 确定类型（基于 p 字段）
        if "thinking" in item_p:
            content_type = "thinking"
        elif "content" in item_p or item_p == "response" or item_p == "fragments":
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


# ----------------------------------------------------------------------
# 高级解析函数
# ----------------------------------------------------------------------

def parse_sse_chunk_for_content(
    chunk: Dict[str, Any], 
    thinking_enabled: bool = False, 
    current_fragment_type: str = "thinking"
) -> Tuple[List[Tuple[str, str]], bool, str]:
    """解析单个 SSE chunk 并提取内容
    
    Args:
        chunk: 解析后的 JSON chunk
        thinking_enabled: 是否启用思考模式
        current_fragment_type: 当前活跃的 fragment 类型 ("thinking" 或 "text")
                              用于处理没有明确路径的空 p 字段内容
    
    Returns:
        (contents, is_finished, new_fragment_type)
        - contents: [(content, content_type), ...] 列表
        - is_finished: 是否是结束信号
        - new_fragment_type: 更新后的 fragment 类型，供下一个 chunk 使用
    """
    if "v" not in chunk:
        return ([], False, current_fragment_type)
    
    v_value = chunk["v"]
    chunk_path = chunk.get("p", "")
    contents: List[Tuple[str, str]] = []
    new_fragment_type = current_fragment_type
    
    # 跳过状态相关 chunk
    if should_skip_chunk(chunk_path):
        return ([], False, current_fragment_type)
    
    # 检查是否是真正的响应结束信号
    if is_response_finished(chunk_path, v_value):
        return ([], True, current_fragment_type)
    
    # 检测 fragment 类型变化（来自 APPEND 操作）
    # 格式: {'p': 'response', 'o': 'BATCH', 'v': [{'p': 'fragments', 'o': 'APPEND', 'v': [{'type': 'THINK/RESPONSE', ...}]}]}
    if chunk_path == "response" and isinstance(v_value, list):
        for batch_item in v_value:
            if isinstance(batch_item, dict) and batch_item.get("p") == "fragments" and batch_item.get("o") == "APPEND":
                fragments = batch_item.get("v", [])
                for frag in fragments:
                    if isinstance(frag, dict):
                        frag_type = frag.get("type", "").upper()
                        if frag_type == "THINK" or frag_type == "THINKING":
                            new_fragment_type = "thinking"
                        elif frag_type == "RESPONSE":
                            new_fragment_type = "text"
    
    # 也检测直接的 fragments 路径
    if "response/fragments" in chunk_path and isinstance(v_value, list):
        for frag in v_value:
            if isinstance(frag, dict):
                frag_type = frag.get("type", "").upper()
                if frag_type == "THINK" or frag_type == "THINKING":
                    new_fragment_type = "thinking"
                elif frag_type == "RESPONSE":
                    new_fragment_type = "text"
    
    # 确定当前内容类型
    if chunk_path == "response/thinking_content":
        ptype = "thinking"
    elif chunk_path == "response/content":
        ptype = "text"
    elif "response/fragments" in chunk_path and "/content" in chunk_path:
        # 如 response/fragments/-1/content - 使用当前 fragment 类型
        ptype = new_fragment_type
    elif not chunk_path:
        # 空路径内容：使用当前活跃的 fragment 类型
        if thinking_enabled:
            ptype = new_fragment_type
        else:
            ptype = "text"
    else:
        ptype = "text"
    
    # 处理字符串值
    if isinstance(v_value, str):
        if is_finished_signal(chunk_path, v_value):
            return ([], True, new_fragment_type)
        if v_value:
            contents.append((v_value, ptype))
    
    # 处理列表值
    elif isinstance(v_value, list):
        result = extract_content_recursive(v_value, ptype)
        if result is None:
            return ([], True, new_fragment_type)
        contents.extend(result)
    
    return (contents, False, new_fragment_type)


def extract_content_from_chunk(chunk: Dict[str, Any]) -> Tuple[str, str, bool]:
    """从 DeepSeek chunk 中提取内容（简化版本，兼容旧接口）
    
    Args:
        chunk: 解析后的 chunk 字典
        
    Returns:
        (content, content_type, is_finished) 元组
        content_type 为 "thinking" 或 "text"
        is_finished 为 True 表示响应结束
    """
    if chunk.get("type") == "done":
        return "", "text", True
    
    # 检测内容审核/敏感词阻止
    if "error" in chunk or chunk.get("code") == "content_filter":
        logger.warning(f"[extract_content_from_chunk] 检测到内容过滤: {chunk}")
        return "", "text", True
    
    if "v" not in chunk:
        return "", "text", False
    
    v_value = chunk["v"]
    ptype = "text"
    
    # 检查路径确定类型
    path = chunk.get("p", "")
    if path == "response/search_status":
        return "", "text", False  # 跳过搜索状态
    elif path == "response/thinking_content":
        ptype = "thinking"
    elif path == "response/content":
        ptype = "text"
    
    if isinstance(v_value, str):
        if v_value == "FINISHED":
            return "", ptype, True
        return v_value, ptype, False
    elif isinstance(v_value, list):
        for item in v_value:
            if isinstance(item, dict):
                if item.get("p") == "status" and item.get("v") == "FINISHED":
                    return "", ptype, True
        return "", ptype, False
    
    return "", ptype, False


# ----------------------------------------------------------------------
# 响应收集函数
# ----------------------------------------------------------------------

def collect_deepseek_response(response: Any) -> Tuple[str, str]:
    """收集 DeepSeek 流响应的完整内容
    
    Args:
        response: DeepSeek 流响应对象
        
    Returns:
        (reasoning_content, text_content) 元组
    """
    thinking_parts: List[str] = []
    text_parts: List[str] = []
    
    try:
        for raw_line in response.iter_lines():
            chunk = parse_deepseek_sse_line(raw_line)
            if not chunk:
                continue
            
            content, content_type, is_finished = extract_content_from_chunk(chunk)
            
            if is_finished:
                break
            
            if content:
                if content_type == "thinking":
                    thinking_parts.append(content)
                else:
                    text_parts.append(content)
    except Exception as e:
        logger.error(f"[collect_deepseek_response] 收集响应失败: {e}")
    finally:
        try:
            response.close()
        except Exception:
            pass
    
    return "".join(thinking_parts), "".join(text_parts)


# ----------------------------------------------------------------------
# 工具调用解析
# ----------------------------------------------------------------------

def parse_tool_calls(text: str, tools_requested: List[Dict]) -> List[Dict[str, Any]]:
    """从响应文本中解析工具调用
    
    Args:
        text: 响应文本
        tools_requested: 请求中定义的工具列表
        
    Returns:
        检测到的工具调用列表，每项包含 name 和 input
    """
    detected_tools: List[Dict[str, Any]] = []
    cleaned_text = text.strip()
    
    # 尝试直接解析完整 JSON
    if cleaned_text.startswith('{"tool_calls":') and cleaned_text.endswith("]}"):
        try:
            tool_data = json.loads(cleaned_text)
            for tool_call in tool_data.get("tool_calls", []):
                tool_name = tool_call.get("name")
                tool_input = tool_call.get("input", {})
                if any(tool.get("name") == tool_name for tool in tools_requested):
                    detected_tools.append({"name": tool_name, "input": tool_input})
            if detected_tools:
                return detected_tools
        except json.JSONDecodeError:
            pass
    
    # 使用正则匹配
    matches = _TOOL_CALL_PATTERN.findall(cleaned_text)
    for match in matches:
        try:
            tool_calls_json = f'{{"tool_calls": [{match}]}}'
            tool_data = json.loads(tool_calls_json)
            for tool_call in tool_data.get("tool_calls", []):
                tool_name = tool_call.get("name")
                tool_input = tool_call.get("input", {})
                if any(tool.get("name") == tool_name for tool in tools_requested):
                    detected_tools.append({"name": tool_name, "input": tool_input})
        except json.JSONDecodeError:
            continue
    
    return detected_tools


# ----------------------------------------------------------------------
# 引用过滤
# ----------------------------------------------------------------------

def should_filter_citation(text: str, search_enabled: bool) -> bool:
    """检查是否应该过滤引用内容
    
    Args:
        text: 内容文本
        search_enabled: 是否启用搜索
        
    Returns:
        是否应该过滤
    """
    if not search_enabled:
        return False
    return _CITATION_PATTERN.match(text) is not None


# ----------------------------------------------------------------------
# 工具调用格式化
# ----------------------------------------------------------------------

def format_openai_tool_calls(
    detected_tools: List[Dict[str, Any]], 
    base_id: str = ""
) -> List[Dict[str, Any]]:
    """将检测到的工具调用格式化为 OpenAI API 格式
    
    Args:
        detected_tools: parse_tool_calls 返回的工具调用列表
        base_id: 用于生成唯一 ID 的基础字符串（可选）
        
    Returns:
        OpenAI 格式的 tool_calls 数组，例如：
        [{"id": "call_xxx", "type": "function", "function": {"name": "...", "arguments": "..."}}]
    """
    import random
    import time
    
    tool_calls_data = []
    for idx, tool_info in enumerate(detected_tools):
        tool_calls_data.append({
            "id": f"call_{base_id or int(time.time())}_{random.randint(1000,9999)}_{idx}",
            "type": "function",
            "function": {
                "name": tool_info["name"],
                "arguments": json.dumps(tool_info.get("input", {}), ensure_ascii=False)
            }
        })
    return tool_calls_data
