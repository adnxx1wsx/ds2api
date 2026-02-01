import { useState, useEffect } from 'react'
import {
    Plus,
    Trash2,
    RefreshCw,
    CheckCircle2,
    AlertCircle,
    Search,
    Play,
    MoreHorizontal,
    X,
    Server,
    ShieldCheck,
    Copy,
    Check
} from 'lucide-react'
import clsx from 'clsx'

export default function AccountManager({ config, onRefresh, onMessage, authFetch }) {
    const [showAddKey, setShowAddKey] = useState(false)
    const [showAddAccount, setShowAddAccount] = useState(false)
    const [newKey, setNewKey] = useState('')
    const [copiedKey, setCopiedKey] = useState(null)
    const [newAccount, setNewAccount] = useState({ email: '', mobile: '', password: '' })
    const [loading, setLoading] = useState(false)
    const [validating, setValidating] = useState({})
    const [validatingAll, setValidatingAll] = useState(false)
    const [testing, setTesting] = useState({})
    const [testingAll, setTestingAll] = useState(false)
    const [batchProgress, setBatchProgress] = useState({ current: 0, total: 0, results: [] })
    const [queueStatus, setQueueStatus] = useState(null)

    const apiFetch = authFetch || fetch

    const fetchQueueStatus = async () => {
        try {
            const res = await apiFetch('/admin/queue/status')
            if (res.ok) {
                const data = await res.json()
                setQueueStatus(data)
            }
        } catch (e) {
            console.error('Failed to fetch queue status:', e)
        }
    }

    useEffect(() => {
        fetchQueueStatus()
        const interval = setInterval(fetchQueueStatus, 5000)
        return () => clearInterval(interval)
    }, [])

    const addKey = async () => {
        if (!newKey.trim()) return
        setLoading(true)
        try {
            const res = await apiFetch('/admin/keys', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ key: newKey.trim() }),
            })
            if (res.ok) {
                onMessage('success', 'API 密钥添加成功')
                setNewKey('')
                setShowAddKey(false)
                onRefresh()
            } else {
                const data = await res.json()
                onMessage('error', data.detail || 'Failed to add')
            }
        } catch (e) {
            onMessage('error', '网络错误')
        } finally {
            setLoading(false)
        }
    }

    const deleteKey = async (key) => {
        if (!confirm('确定要删除此 API 密钥吗？')) return
        try {
            const res = await apiFetch(`/admin/keys/${encodeURIComponent(key)}`, { method: 'DELETE' })
            if (res.ok) {
                onMessage('success', 'Deleted successfully')
                onRefresh()
            } else {
                onMessage('error', 'Delete failed')
            }
        } catch (e) {
            onMessage('error', 'Network error')
        }
    }

    const addAccount = async () => {
        if (!newAccount.password || (!newAccount.email && !newAccount.mobile)) {
            onMessage('error', 'Password and Email/Mobile are required')
            return
        }
        setLoading(true)
        try {
            const res = await apiFetch('/admin/accounts', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(newAccount),
            })
            if (res.ok) {
                onMessage('success', '账号添加成功')
                setNewAccount({ email: '', mobile: '', password: '' })
                setShowAddAccount(false)
                onRefresh()
            } else {
                const data = await res.json()
                onMessage('error', data.detail || 'Failed to add')
            }
        } catch (e) {
            onMessage('error', '网络错误')
        } finally {
            setLoading(false)
        }
    }

    const deleteAccount = async (id) => {
        if (!confirm('确定要删除此账号吗？')) return
        try {
            const res = await apiFetch(`/admin/accounts/${encodeURIComponent(id)}`, { method: 'DELETE' })
            if (res.ok) {
                onMessage('success', 'Deleted successfully')
                onRefresh()
            } else {
                onMessage('error', 'Delete failed')
            }
        } catch (e) {
            onMessage('error', 'Network error')
        }
    }

    const validateAccount = async (identifier) => {
        setValidating(prev => ({ ...prev, [identifier]: true }))
        try {
            const res = await apiFetch('/admin/accounts/validate', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ identifier }),
            })
            const data = await res.json()
            onMessage(data.valid ? 'success' : 'error', `${identifier}: ${data.message}`)
            onRefresh()
        } catch (e) {
            onMessage('error', 'Validation failed: ' + e.message)
        } finally {
            setValidating(prev => ({ ...prev, [identifier]: false }))
        }
    }

    const validateAllAccounts = async () => {
        if (!confirm('校验所有账号？这可能需要一些时间。')) return
        const accounts = config.accounts || []
        if (accounts.length === 0) return

        setValidatingAll(true)
        setBatchProgress({ current: 0, total: accounts.length, results: [] })

        let validCount = 0
        const results = []

        for (let i = 0; i < accounts.length; i++) {
            const acc = accounts[i]
            const id = acc.email || acc.mobile

            try {
                const res = await apiFetch('/admin/accounts/validate', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ identifier: id }),
                })
                const data = await res.json()
                results.push({ id, success: data.valid, message: data.message })
                if (data.valid) validCount++
            } catch (e) {
                results.push({ id, success: false, message: e.message })
            }

            setBatchProgress({ current: i + 1, total: accounts.length, results: [...results] })
        }

        onMessage('success', `Completed: ${validCount}/${accounts.length} valid`)
        onRefresh()
        setValidatingAll(false)
    }

    const testAccount = async (identifier) => {
        setTesting(prev => ({ ...prev, [identifier]: true }))
        try {
            const res = await apiFetch('/admin/accounts/test', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ identifier }),
            })
            const data = await res.json()
            onMessage(data.success ? 'success' : 'error', `${identifier}: ${data.success ? `Success (${data.response_time}ms)` : data.message}`)
            onRefresh()
        } catch (e) {
            onMessage('error', 'Test failed: ' + e.message)
        } finally {
            setTesting(prev => ({ ...prev, [identifier]: false }))
        }
    }

    const testAllAccounts = async () => {
        if (!confirm('测试所有账号的 API 连通性？')) return
        const accounts = config.accounts || []
        if (accounts.length === 0) return

        setTestingAll(true)
        setBatchProgress({ current: 0, total: accounts.length, results: [] })

        let successCount = 0
        const results = []

        for (let i = 0; i < accounts.length; i++) {
            const acc = accounts[i]
            const id = acc.email || acc.mobile

            try {
                const res = await apiFetch('/admin/accounts/test', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ identifier: id }),
                })
                const data = await res.json()
                results.push({ id, success: data.success, message: data.message, time: data.response_time })
                if (data.success) successCount++
            } catch (e) {
                results.push({ id, success: false, message: e.message })
            }

            setBatchProgress({ current: i + 1, total: accounts.length, results: [...results] })
        }

        onMessage('success', `Completed: ${successCount}/${accounts.length} available`)
        onRefresh()
        setTestingAll(false)
    }

    return (
        <div className="space-y-6">
            {/* Queue Status - Flat & Clean */}
            {
                queueStatus && (
                    <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                        <div className="bg-card border border-border rounded-xl p-4 flex flex-col justify-between shadow-sm relative overflow-hidden group">
                            <div className="absolute right-0 top-0 p-4 opacity-5 group-hover:opacity-10 transition-opacity">
                                <CheckCircle2 className="w-16 h-16" />
                            </div>
                            <p className="text-xs font-medium text-muted-foreground uppercase tracking-widest">可用</p>
                            <div className="mt-2 flex items-baseline gap-2">
                                <span className="text-3xl font-bold text-foreground">{queueStatus.available}</span>
                                <span className="text-xs text-muted-foreground">个账号</span>
                            </div>
                        </div>
                        <div className="bg-card border border-border rounded-xl p-4 flex flex-col justify-between shadow-sm relative overflow-hidden group">
                            <div className="absolute right-0 top-0 p-4 opacity-5 group-hover:opacity-10 transition-opacity">
                                <Server className="w-16 h-16" />
                            </div>
                            <p className="text-xs font-medium text-muted-foreground uppercase tracking-widest">正在使用</p>
                            <div className="mt-2 flex items-baseline gap-2">
                                <span className="text-3xl font-bold text-foreground">{queueStatus.in_use}</span>
                                <span className="text-xs text-muted-foreground">线程</span>
                            </div>
                        </div>
                        <div className="bg-card border border-border rounded-xl p-4 flex flex-col justify-between shadow-sm relative overflow-hidden group">
                            <div className="absolute right-0 top-0 p-4 opacity-5 group-hover:opacity-10 transition-opacity">
                                <ShieldCheck className="w-16 h-16" />
                            </div>
                            <p className="text-xs font-medium text-muted-foreground uppercase tracking-widest">账号池总数</p>
                            <div className="mt-2 flex items-baseline gap-2">
                                <span className="text-3xl font-bold text-foreground">{queueStatus.total}</span>
                                <span className="text-xs text-muted-foreground">个账号</span>
                            </div>
                        </div>
                    </div>
                )
            }

            {/* API Keys Section */}
            <div className="bg-card border border-border rounded-xl overflow-hidden shadow-sm">
                <div className="p-6 border-b border-border flex flex-col md:flex-row md:items-center justify-between gap-4">
                    <div>
                        <h2 className="text-lg font-semibold">API 密钥</h2>
                        <p className="text-sm text-muted-foreground">管理 API 访问密钥池</p>
                    </div>
                    <button
                        onClick={() => setShowAddKey(true)}
                        className="flex items-center gap-2 px-4 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors font-medium text-sm shadow-sm"
                    >
                        <Plus className="w-4 h-4" />
                        添加密钥
                    </button>
                </div>

                <div className="divide-y divide-border">
                    {config.keys?.length > 0 ? (
                        config.keys.map((key, i) => (
                            <div key={i} className="p-4 flex items-center justify-between hover:bg-muted/50 transition-colors group">
                                <div className="flex items-center gap-2">
                                    <div className="font-mono text-sm bg-muted/50 px-3 py-1 rounded inline-block">
                                        {key.slice(0, 16)}****
                                    </div>
                                    {copiedKey === key && (
                                        <span className="text-xs text-green-500 animate-pulse">已复制</span>
                                    )}
                                </div>
                                <div className="flex items-center gap-1">
                                    <button
                                        onClick={() => {
                                            navigator.clipboard.writeText(key)
                                            setCopiedKey(key)
                                            setTimeout(() => setCopiedKey(null), 2000)
                                        }}
                                        className="p-2 text-muted-foreground hover:text-primary hover:bg-primary/10 rounded-md transition-colors opacity-0 group-hover:opacity-100"
                                        title="复制密钥"
                                    >
                                        {copiedKey === key ? <Check className="w-4 h-4 text-green-500" /> : <Copy className="w-4 h-4" />}
                                    </button>
                                    <button
                                        onClick={() => deleteKey(key)}
                                        className="p-2 text-muted-foreground hover:text-destructive hover:bg-destructive/10 rounded-md transition-colors opacity-0 group-hover:opacity-100"
                                        title="删除密钥"
                                    >
                                        <Trash2 className="w-4 h-4" />
                                    </button>
                                </div>
                            </div>
                        ))
                    ) : (
                        <div className="p-8 text-center text-muted-foreground">未找到 API 密钥</div>
                    )}
                </div>
            </div>

            {/* Accounts Section */}
            <div className="bg-card border border-border rounded-xl overflow-hidden shadow-sm">
                <div className="p-6 border-b border-border flex flex-col md:flex-row md:items-center justify-between gap-4">
                    <div>
                        <h2 className="text-lg font-semibold">DeepSeek 账号</h2>
                        <p className="text-sm text-muted-foreground">管理 DeepSeek 账号池</p>
                    </div>
                    <div className="flex flex-wrap gap-2">
                        <button
                            onClick={testAllAccounts}
                            disabled={testingAll || validatingAll || !config.accounts?.length}
                            className="flex items-center px-3 py-2 bg-secondary text-secondary-foreground rounded-lg hover:bg-secondary/80 transition-colors text-xs font-medium border border-border disabled:opacity-50"
                        >
                            {testingAll ? <span className="animate-spin mr-2">⟳</span> : <Play className="w-3 h-3 mr-2" />}
                            测试全部
                        </button>
                        <button
                            onClick={validateAllAccounts}
                            disabled={validatingAll || testingAll || !config.accounts?.length}
                            className="flex items-center px-3 py-2 bg-secondary text-secondary-foreground rounded-lg hover:bg-secondary/80 transition-colors text-xs font-medium border border-border disabled:opacity-50"
                        >
                            {validatingAll ? <span className="animate-spin mr-2">⟳</span> : <CheckCircle2 className="w-3 h-3 mr-2" />}
                            校验全部
                        </button>
                        <button
                            onClick={() => setShowAddAccount(true)}
                            className="flex items-center gap-2 px-4 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors font-medium text-sm shadow-sm"
                        >
                            <Plus className="w-4 h-4" />
                            添加账号
                        </button>
                    </div>
                </div>

                {/* Batch Progress */}
                {(testingAll || validatingAll) && batchProgress.total > 0 && (
                    <div className="p-4 border-b border-border bg-muted/30">
                        <div className="flex items-center justify-between text-sm mb-2">
                            <span className="font-medium">{testingAll ? '正在测试所有账号...' : '正在校验所有账号...'}</span>
                            <span className="text-muted-foreground">{batchProgress.current} / {batchProgress.total}</span>
                        </div>
                        <div className="w-full bg-muted rounded-full h-2 overflow-hidden mb-4">
                            <div
                                className="bg-primary h-full transition-all duration-300"
                                style={{ width: `${(batchProgress.current / batchProgress.total) * 100}%` }}
                            />
                        </div>
                        {batchProgress.results.length > 0 && (
                            <div className="grid grid-cols-2 md:grid-cols-4 gap-2 max-h-32 overflow-y-auto custom-scrollbar">
                                {batchProgress.results.map((r, i) => (
                                    <div key={i} className={clsx(
                                        "text-xs px-2 py-1 rounded border truncate",
                                        r.success ? "bg-emerald-500/10 border-emerald-500/20 text-emerald-500" : "bg-destructive/10 border-destructive/20 text-destructive"
                                    )}>
                                        {r.success ? '✓' : '✗'} {r.id}
                                    </div>
                                ))}
                            </div>
                        )}
                    </div>
                )}

                <div className="divide-y divide-border">
                    {config.accounts?.length > 0 ? (
                        config.accounts.map((acc, i) => {
                            const id = acc.email || acc.mobile
                            return (
                                <div key={i} className="p-4 flex flex-col md:flex-row md:items-center justify-between gap-4 hover:bg-muted/50 transition-colors">
                                    <div className="flex items-center gap-3 min-w-0">
                                        <div className={clsx(
                                            "w-2 h-2 rounded-full shrink-0",
                                            acc.has_token ? "bg-emerald-500 shadow-[0_0_8px_rgba(16,185,129,0.5)]" : "bg-amber-500"
                                        )} />
                                        <div className="min-w-0">
                                            <div className="font-medium truncate">{id}</div>
                                            <div className="flex items-center gap-2 text-xs text-muted-foreground mt-0.5">
                                                <span>{acc.has_token ? '已建立会话' : '需重新登录'}</span>
                                                {acc.token_preview && (
                                                    <span className="font-mono bg-muted px-1.5 py-0.5 rounded text-[10px]">
                                                        {acc.token_preview}
                                                    </span>
                                                )}
                                            </div>
                                        </div>
                                    </div>
                                    <div className="flex items-center gap-2 self-start lg:self-auto ml-5 lg:ml-0">
                                        <button
                                            onClick={() => testAccount(id)}
                                            disabled={testing[id]}
                                            className="px-2 lg:px-3 py-1 lg:py-1.5 text-[10px] lg:text-xs font-medium border border-border rounded-md hover:bg-secondary transition-colors disabled:opacity-50"
                                        >
                                            {testing[id] ? '正在测试...' : '测试'}
                                        </button>
                                        <button
                                            onClick={() => validateAccount(id)}
                                            disabled={validating[id]}
                                            className="px-2 lg:px-3 py-1 lg:py-1.5 text-[10px] lg:text-xs font-medium border border-border rounded-md hover:bg-secondary transition-colors disabled:opacity-50"
                                        >
                                            {validating[id] ? '正在校验...' : '校验'}
                                        </button>
                                        <button
                                            onClick={() => deleteAccount(id)}
                                            className="p-1 lg:p-1.5 text-muted-foreground hover:text-destructive hover:bg-destructive/10 rounded-md transition-colors"
                                        >
                                            <Trash2 className="w-3.5 h-3.5 lg:w-4 h-4" />
                                        </button>
                                    </div>
                                </div>
                            )
                        })
                    ) : (
                        <div className="p-8 text-center text-muted-foreground">未找到任何账号</div>
                    )}
                </div>
            </div>

            {/* Modals */}
            {
                showAddKey && (
                    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm p-4 animate-in fade-in">
                        <div className="bg-card w-full max-w-md rounded-xl border border-border shadow-2xl overflow-hidden animate-in zoom-in-95">
                            <div className="p-4 border-b border-border flex justify-between items-center">
                                <h3 className="font-semibold">添加 API 密钥</h3>
                                <button onClick={() => setShowAddKey(false)} className="text-muted-foreground hover:text-foreground">
                                    <X className="w-5 h-5" />
                                </button>
                            </div>
                            <div className="p-6 space-y-4">
                                <div>
                                    <label className="block text-sm font-medium mb-1.5">新密钥值</label>
                                    <div className="flex gap-2">
                                        <input
                                            type="text"
                                            className="input-field bg-[#09090b] flex-1"
                                            placeholder="输入自定义 API 密钥"
                                            value={newKey}
                                            onChange={e => setNewKey(e.target.value)}
                                            autoFocus
                                        />
                                        <button
                                            type="button"
                                            onClick={() => setNewKey('sk-' + crypto.randomUUID().replace(/-/g, ''))}
                                            className="px-3 py-2 bg-secondary text-secondary-foreground rounded-lg hover:bg-secondary/80 transition-colors text-sm font-medium border border-border whitespace-nowrap"
                                        >
                                            生成
                                        </button>
                                    </div>
                                    <p className="text-xs text-muted-foreground mt-1.5">点击「生成」自动创建随机密钥</p>
                                </div>
                                <div className="flex justify-end gap-2 pt-2">
                                    <button onClick={() => setShowAddKey(false)} className="px-4 py-2 rounded-lg border border-border hover:bg-secondary transition-colors text-sm font-medium">取消</button>
                                    <button onClick={addKey} disabled={loading} className="px-4 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors text-sm font-medium disabled:opacity-50">
                                        {loading ? '添加中...' : '添加密钥'}
                                    </button>
                                </div>
                            </div>
                        </div>
                    </div>
                )
            }

            {
                showAddAccount && (
                    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm p-4 animate-in fade-in">
                        <div className="bg-card w-full max-w-md rounded-xl border border-border shadow-2xl overflow-hidden animate-in zoom-in-95">
                            <div className="p-4 border-b border-border flex justify-between items-center">
                                <h3 className="font-semibold">添加 DeepSeek 账号</h3>
                                <button onClick={() => setShowAddAccount(false)} className="text-muted-foreground hover:text-foreground">
                                    <X className="w-5 h-5" />
                                </button>
                            </div>
                            <div className="p-6 space-y-4">
                                <div>
                                    <label className="block text-sm font-medium mb-1.5">邮箱 (可选)</label>
                                    <input
                                        type="email"
                                        className="input-field"
                                        placeholder="user@example.com"
                                        value={newAccount.email}
                                        onChange={e => setNewAccount({ ...newAccount, email: e.target.value })}
                                    />
                                </div>
                                <div>
                                    <label className="block text-sm font-medium mb-1.5">手机号 (可选)</label>
                                    <input
                                        type="text"
                                        className="input-field"
                                        placeholder="+86..."
                                        value={newAccount.mobile}
                                        onChange={e => setNewAccount({ ...newAccount, mobile: e.target.value })}
                                    />
                                </div>
                                <div>
                                    <label className="block text-sm font-medium mb-1.5">密码 <span className="text-destructive">*</span></label>
                                    <input
                                        type="password"
                                        className="input-field bg-[#09090b]"
                                        placeholder="账号密码"
                                        value={newAccount.password}
                                        onChange={e => setNewAccount({ ...newAccount, password: e.target.value })}
                                    />
                                </div>
                                <div className="flex justify-end gap-2 pt-2">
                                    <button onClick={() => setShowAddAccount(false)} className="px-4 py-2 rounded-lg border border-border hover:bg-secondary transition-colors text-sm font-medium">取消</button>
                                    <button onClick={addAccount} disabled={loading} className="px-4 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors text-sm font-medium disabled:opacity-50">
                                        {loading ? '添加中...' : '添加账号'}
                                    </button>
                                </div>
                            </div>
                        </div>
                    </div>
                )
            }
        </div >
    )
}
