import { useState, useRef } from 'react'
import {
    Send,
    Square,
    MessageSquare,
    Cpu,
    Search as SearchIcon,
    Sparkles,
    Bot,
    User,
    Loader2,
    CheckCircle2,
    AlertCircle,
    ChevronDown,
    ShieldCheck,
    Terminal,
    Zap
} from 'lucide-react'
import clsx from 'clsx'

const MODELS = [
    { id: "deepseek-chat", name: "deepseek-chat", icon: MessageSquare, desc: "通用高智商模型 (V3)", color: "text-amber-500" },
    { id: "deepseek-reasoner", name: "deepseek-reasoner", icon: Cpu, desc: "深度推理/思维链 (R1)", color: "text-amber-600" },
];

export default function ApiTester({ config, onMessage, authFetch }) {
    const [model, setModel] = useState('deepseek-chat')
    const [message, setMessage] = useState('Hello, please introduce yourself in one sentence.')
    const [apiKey, setApiKey] = useState('')
    const [selectedAccount, setSelectedAccount] = useState('')
    const [response, setResponse] = useState(null)
    const [loading, setLoading] = useState(false)
    const [streamingContent, setStreamingContent] = useState('')
    const [streamingThinking, setStreamingThinking] = useState('')
    const [isStreaming, setIsStreaming] = useState(false)
    const abortControllerRef = useRef(null)

    const [sidebarOpen, setSidebarOpen] = useState(false)
    const [configExpanded, setConfigExpanded] = useState(false)

    const apiFetch = authFetch || fetch
    const accounts = config.accounts || []

    const stopGeneration = () => {
        if (abortControllerRef.current) {
            abortControllerRef.current.abort()
            abortControllerRef.current = null
        }
        setLoading(false)
        setIsStreaming(false)
    }

    const directTest = async () => {
        if (loading) return

        setLoading(true)
        setIsStreaming(true)
        setResponse(null)
        setStreamingContent('')
        setStreamingThinking('')

        abortControllerRef.current = new AbortController()

        try {
            const key = apiKey || (config.keys?.[0] || '')
            if (!key) {
                onMessage('error', '请提供 API 密钥')
                setLoading(false)
                setIsStreaming(false)
                return
            }

            const res = await fetch('/v1/chat/completions', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${key}`,
                },
                body: JSON.stringify({
                    model,
                    messages: [{ role: 'user', content: message }],
                    stream: true,
                }),
                signal: abortControllerRef.current.signal,
            })

            if (!res.ok) {
                const data = await res.json()
                setResponse({ success: false, error: data.error?.message || '请求失败' })
                onMessage('error', data.error?.message || '请求失败')
                setLoading(false)
                setIsStreaming(false)
                return
            }

            setResponse({ success: true, status_code: res.status })

            const reader = res.body.getReader()
            const decoder = new TextDecoder()
            let buffer = ''

            while (true) {
                const { done, value } = await reader.read()
                if (done) break

                buffer += decoder.decode(value, { stream: true })
                const lines = buffer.split('\n')
                buffer = lines.pop() || ''

                for (const line of lines) {
                    const trimmed = line.trim()
                    if (!trimmed || !trimmed.startsWith('data: ')) continue

                    const dataStr = trimmed.slice(6)
                    if (dataStr === '[DONE]') continue

                    try {
                        const json = JSON.parse(dataStr)
                        const choice = json.choices?.[0]
                        if (choice?.delta) {
                            const delta = choice.delta
                            if (delta.reasoning_content) {
                                setStreamingThinking(prev => prev + delta.reasoning_content)
                            }
                            if (delta.content) {
                                setStreamingContent(prev => prev + delta.content)
                            }
                        }
                    } catch (e) {
                        console.error('Invalid JSON hunk:', dataStr, e)
                    }
                }
            }
        } catch (e) {
            if (e.name === 'AbortError') {
                onMessage('info', '已停止生成')
            } else {
                onMessage('error', 'Network error: ' + e.message)
                setResponse({ error: e.message, success: false })
            }
        } finally {
            setLoading(false)
            setIsStreaming(false)
            abortControllerRef.current = null
        }
    }

    const sendTest = async () => {
        if (selectedAccount) {
            setLoading(true)
            setResponse(null)
            try {
                const res = await apiFetch('/admin/accounts/test', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        identifier: selectedAccount,
                        model,
                        message,
                    }),
                })
                const data = await res.json()
                setResponse({
                    success: data.success,
                    status_code: res.status,
                    response: data,
                    account: selectedAccount,
                })
                if (data.success) {
                    onMessage('success', `${selectedAccount}: Test Success (${data.response_time}ms)`)
                } else {
                    onMessage('error', `${selectedAccount}: ${data.message}`)
                }
            } catch (e) {
                onMessage('error', 'Network error: ' + e.message)
                setResponse({ error: e.message })
            } finally {
                setLoading(false)
            }
            return
        }

        directTest()
    }

    return (
        <div className="flex flex-col lg:grid lg:grid-cols-12 gap-6 h-[calc(100vh-140px)]">
            {/* Configuration Panel */}
            <div className={clsx(
                "lg:col-span-3 flex flex-col transition-all duration-300 ease-in-out z-20",
                configExpanded ? "h-auto" : "h-14 lg:h-full"
            )}>
                <div className="bg-card border border-border rounded-xl flex flex-col h-full shadow-sm">
                    {/* Mobile Toggle Header */}
                    <button
                        onClick={() => setConfigExpanded(!configExpanded)}
                        className="lg:hidden flex items-center justify-between p-4 w-full bg-muted/20 hover:bg-muted/30 transition-colors"
                    >
                        <div className="flex items-center gap-2.5 font-medium text-sm text-foreground">
                            <div className="p-1.5 rounded-md bg-transparent text-foreground">
                                <Terminal className="w-4 h-4" />
                            </div>
                            <span>配置</span>
                        </div>
                        <div className={clsx("transition-transform duration-300 text-muted-foreground", configExpanded ? "rotate-180" : "")}>
                            <ChevronDown className="w-4 h-4" />
                        </div>
                    </button>

                    <div className={clsx(
                        "p-4 space-y-6 overflow-y-auto custom-scrollbar flex-1",
                        !configExpanded && "hidden lg:block"
                    )}>
                        <div className="space-y-3">
                            <label className="text-[11px] font-semibold text-muted-foreground uppercase tracking-wider ml-0.5">模型</label>
                            <div className="grid grid-cols-1 gap-2">
                                {MODELS.map(m => {
                                    const Icon = m.icon
                                    return (
                                        <button
                                            key={m.id}
                                            onClick={() => setModel(m.id)}
                                            className={clsx(
                                                "group relative flex items-start gap-3 p-3 rounded-lg border text-left transition-all duration-200",
                                                model === m.id
                                                    ? "bg-secondary border-primary/50 shadow-sm"
                                                    : "bg-transparent border-transparent hover:bg-muted"
                                            )}
                                        >
                                            <div className={clsx(
                                                "p-1.5 rounded-md shrink-0 transition-colors",
                                                model === m.id ? "text-primary" : "text-muted-foreground group-hover:text-foreground"
                                            )}>
                                                <Icon className="w-4 h-4" />
                                            </div>
                                            <div className="min-w-0 flex-1">
                                                <div className={clsx("font-medium text-sm", model === m.id ? "text-foreground" : "text-foreground/80")}>
                                                    {m.name}
                                                </div>
                                                <div className="text-[11px] text-muted-foreground mt-0.5">{m.desc}</div>
                                            </div>
                                            {model === m.id && (
                                                <div className="absolute top-3 right-3 text-primary">
                                                    <div className="w-1.5 h-1.5 rounded-full bg-current" />
                                                </div>
                                            )}
                                        </button>
                                    )
                                })}
                            </div>
                        </div>

                        <div className="space-y-2">
                            <label className="text-[11px] font-semibold text-muted-foreground uppercase tracking-wider ml-0.5">账号策略</label>
                            <div className="relative">
                                <select
                                    className="w-full h-10 pl-3 pr-8 bg-secondary border border-border rounded-lg text-sm appearance-none focus:outline-none focus:ring-1 focus:ring-ring focus:border-ring transition-all cursor-pointer hover:bg-muted"
                                    value={selectedAccount}
                                    onChange={e => setSelectedAccount(e.target.value)}
                                >
                                    <option value="" className="bg-popover text-popover-foreground">🎲 随机切换 (支持流式预览)</option>
                                    {accounts.map((acc, i) => (
                                        <option key={i} value={acc.email || acc.mobile} className="bg-popover text-popover-foreground">
                                            👤 {acc.email || acc.mobile}
                                        </option>
                                    ))}
                                </select>
                                <ChevronDown className="absolute right-2.5 top-3 w-4 h-4 text-muted-foreground pointer-events-none" />
                            </div>
                        </div>

                        <div className="space-y-2">
                            <label className="text-[11px] font-semibold text-muted-foreground uppercase tracking-wider ml-0.5">API 密钥 (可选)</label>
                            <input
                                type="password"
                                className="w-full h-10 px-3 bg-muted/30 border border-border rounded-lg text-sm font-mono placeholder:text-muted-foreground/40 focus:outline-none focus:ring-1 focus:ring-ring focus:border-ring transition-all"
                                placeholder={config.keys?.[0] ? `默认: ...${config.keys[0].slice(-6)}` : '输入自定义 Key'}
                                value={apiKey}
                                onChange={e => setApiKey(e.target.value)}
                            />
                        </div>
                    </div>
                </div>
            </div>

            {/* Chat Interface */}
            <div className="lg:col-span-9 flex flex-col bg-card border border-border rounded-xl shadow-sm overflow-hidden min-h-0 flex-1 relative">

                {/* Messages Area */}
                <div className="flex-1 overflow-y-auto p-4 lg:p-6 space-y-8 custom-scrollbar scroll-smooth">
                    {/* User Message */}
                    <div className="flex gap-4 max-w-4xl mx-auto flex-row-reverse group">
                        <div className="w-8 h-8 rounded-lg bg-secondary flex items-center justify-center shrink-0 border border-border">
                            <User className="w-4 h-4 text-muted-foreground" />
                        </div>
                        <div className="space-y-1 max-w-[85%] lg:max-w-[75%]">
                            <div className="bg-primary text-primary-foreground rounded-2xl rounded-tr-sm px-5 py-3 text-sm leading-relaxed shadow-sm">
                                {message}
                            </div>
                        </div>
                    </div>

                    {/* AI Response */}
                    {(response || isStreaming) && (
                        <div className="flex gap-4 max-w-4xl mx-auto animate-in fade-in slide-in-from-bottom-2 duration-300">
                            <div className={clsx(
                                "w-8 h-8 rounded-lg flex items-center justify-center shrink-0 border border-border",
                                response?.success !== false ? "bg-muted" : "bg-destructive/10 border-destructive/20"
                            )}>
                                <Bot className={clsx("w-4 h-4", response?.success !== false ? "text-foreground" : "text-destructive")} />
                            </div>
                            <div className="space-y-3 flex-1 min-w-0">
                                <div className="flex items-center gap-2">
                                    <span className="font-semibold text-sm text-foreground">
                                        DeepSeek
                                    </span>
                                    {response && (
                                        <span className={clsx(
                                            "text-[10px] px-1.5 py-0.5 rounded-sm border uppercase font-medium tracking-wider",
                                            response.success ? "border-emerald-500/20 text-emerald-500 bg-emerald-500/10" : "border-destructive/20 text-destructive bg-destructive/10"
                                        )}>
                                            {response.status_code || 'ERR'}
                                        </span>
                                    )}
                                </div>

                                {(streamingThinking || response?.response?.thinking) && (
                                    <div className="text-xs bg-secondary/50 border border-border rounded-lg p-3 space-y-1.5">
                                        <div className="flex items-center gap-1.5 text-muted-foreground">
                                            <Zap className="w-3.5 h-3.5" />
                                            <span className="font-medium">Thinking Process</span>
                                        </div>
                                        <div className="whitespace-pre-wrap leading-relaxed text-muted-foreground font-mono text-[11px] max-h-60 overflow-y-auto custom-scrollbar pl-5 border-l-2 border-border/50">
                                            {streamingThinking || response?.response?.thinking}
                                        </div>
                                    </div>
                                )}

                                <div className="text-sm leading-7 text-foreground whitespace-pre-wrap">
                                    {!selectedAccount ? (
                                        streamingContent || (response?.error && <span className="text-destructive font-medium">{response.error}</span>)
                                    ) : (
                                        response?.response?.message || <span className="text-muted-foreground italic">Generating response...</span>
                                    )}
                                    {isStreaming && <span className="inline-block w-1.5 h-4 bg-primary ml-1 align-middle animate-pulse" />}
                                </div>
                            </div>
                        </div>
                    )}
                </div>

                {/* Input Area */}
                <div className="p-4 lg:p-6 border-t border-border bg-card">
                    <div className="max-w-4xl mx-auto relative group">
                        <textarea
                            className="w-full bg-[#09090b] border border-border rounded-xl pl-4 pr-12 py-3 text-sm focus:ring-2 focus:ring-primary/20 focus:border-primary transition-all resize-none custom-scrollbar placeholder:text-muted-foreground/50 text-foreground shadow-inner"
                            placeholder="Type a message..."
                            rows={1}
                            style={{ minHeight: '52px' }}
                            value={message}
                            onChange={e => setMessage(e.target.value)}
                            onKeyDown={e => {
                                if (e.key === 'Enter' && !e.shiftKey) {
                                    e.preventDefault()
                                    sendTest()
                                }
                            }}
                        />
                        <div className="absolute right-2 bottom-2">
                            {loading && isStreaming ? (
                                <button
                                    onClick={stopGeneration}
                                    className="p-2 text-muted-foreground hover:text-destructive transition-colors"
                                >
                                    <Square className="w-4 h-4 fill-current" />
                                </button>
                            ) : (
                                <button
                                    onClick={sendTest}
                                    disabled={loading || !message.trim()}
                                    className="p-2 text-primary hover:text-primary/80 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                                >
                                    {loading ? <Loader2 className="w-4 h-4 animate-spin" /> : <Send className="w-4 h-4" />}
                                </button>
                            )}
                        </div>
                    </div>
                    <div className="max-w-4xl mx-auto mt-3 flex justify-center">
                        <span className="text-[10px] text-muted-foreground/40 font-medium">DeepSeek Admin Interface</span>
                    </div>
                </div>
            </div>
        </div>
    )
}
