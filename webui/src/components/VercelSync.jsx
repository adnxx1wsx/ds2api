import { useState, useEffect } from 'react'
import { Cloud, ArrowRight, ExternalLink, Info, CheckCircle2, XCircle } from 'lucide-react'

export default function VercelSync({ onMessage, authFetch }) {
    const [vercelToken, setVercelToken] = useState('')
    const [projectId, setProjectId] = useState('')
    const [teamId, setTeamId] = useState('')
    const [loading, setLoading] = useState(false)
    const [result, setResult] = useState(null)
    const [preconfig, setPreconfig] = useState(null)

    const apiFetch = authFetch || fetch

    useEffect(() => {
        const loadPreconfig = async () => {
            try {
                const res = await apiFetch('/admin/vercel/config')
                if (res.ok) {
                    const data = await res.json()
                    setPreconfig(data)
                    if (data.project_id) setProjectId(data.project_id)
                    if (data.team_id) setTeamId(data.team_id)
                }
            } catch (e) {
                console.error('Failed to load preconfig:', e)
            }
        }
        loadPreconfig()
    }, [])

    const handleSync = async () => {
        const tokenToUse = preconfig?.has_token && !vercelToken ? '__USE_PRECONFIG__' : vercelToken

        if (!tokenToUse && !preconfig?.has_token) {
            onMessage('error', '需要 Vercel 访问令牌')
            return
        }
        if (!projectId) {
            onMessage('error', '需要项目 ID')
            return
        }

        setLoading(true)
        setResult(null)
        try {
            const res = await apiFetch('/admin/vercel/sync', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    vercel_token: vercelToken,
                    project_id: projectId,
                    team_id: teamId || undefined,
                }),
            })
            const data = await res.json()
            if (res.ok) {
                setResult({ ...data, success: true })
                onMessage('success', data.message)
            } else {
                setResult({ ...data, success: false })
                onMessage('error', data.detail || '同步失败')
            }
        } catch (e) {
            onMessage('error', 'Network error')
        } finally {
            setLoading(false)
        }
    }

    return (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-8 max-w-5xl mx-auto h-[calc(100vh-140px)]">
            {/* Configuration Form */}
            <div className="bg-card border border-border rounded-xl shadow-sm p-6 space-y-6">
                <div className="border-b border-border pb-6">
                    <h2 className="text-xl font-semibold flex items-center gap-2">
                        <Cloud className="w-6 h-6 text-primary" />
                        Vercel 部署
                    </h2>
                    <p className="text-muted-foreground text-sm mt-1">
                        将当前密钥和账号配置直接同步到 Vercel 环境变量中。
                    </p>
                </div>

                <div className="space-y-4">
                    <div className="space-y-2">
                        <label className="text-sm font-medium flex items-center justify-between">
                            Vercel 访问令牌
                            <a href="https://vercel.com/account/tokens" target="_blank" rel="noopener noreferrer" className="text-xs text-primary hover:underline flex items-center gap-1">
                                获取令牌 <ExternalLink className="w-3 h-3" />
                            </a>
                        </label>
                        <div className="relative">
                            <input
                                type="password"
                                className="w-full h-10 px-3 bg-background border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-ring focus:border-ring transition-all pr-10"
                                placeholder={preconfig?.has_token ? "Using pre-configured token" : "Enter Vercel Access Token"}
                                value={vercelToken}
                                onChange={e => setVercelToken(e.target.value)}
                            />
                            {preconfig?.has_token && !vercelToken && (
                                <div className="absolute right-3 top-2.5 text-emerald-500">
                                    <CheckCircle2 className="w-5 h-5" />
                                </div>
                            )}
                        </div>
                    </div>

                    <div className="space-y-2">
                        <label className="text-sm font-medium">项目 ID</label>
                        <input
                            type="text"
                            className="w-full h-10 px-3 bg-background border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-ring focus:border-ring transition-all"
                            placeholder="prj_xxxxxxxxxxxx or Project Name"
                            value={projectId}
                            onChange={e => setProjectId(e.target.value)}
                        />
                        <p className="text-xs text-muted-foreground">可在项目设置 (Project Settings) → 常规 (General) 中找到</p>
                    </div>

                    <div className="space-y-2">
                        <label className="text-sm font-medium flex items-center gap-2">
                            团队 ID <span className="text-xs text-muted-foreground font-normal">(可选)</span>
                        </label>
                        <input
                            type="text"
                            className="w-full h-10 px-3 bg-background border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-ring focus:border-ring transition-all"
                            placeholder="team_xxxxxxxxxxxx"
                            value={teamId}
                            onChange={e => setTeamId(e.target.value)}
                        />
                    </div>
                </div>

                <div className="pt-4">
                    <button
                        onClick={handleSync}
                        disabled={loading}
                        className="w-full flex items-center justify-center gap-2 py-3 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-all font-medium text-sm shadow-sm hover:shadow-md disabled:opacity-50 disabled:shadow-none"
                    >
                        {loading ? (
                            <span className="flex items-center gap-2">
                                <span className="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin" />
                                Syncing...
                            </span>
                        ) : (
                            <span className="flex items-center gap-2">
                                Sync & Redeploy <ArrowRight className="w-4 h-4" />
                            </span>
                        )}
                    </button>
                    <p className="text-xs text-center text-muted-foreground mt-4">
                        这将触发 Vercel 的重新部署，大约需要 30-60 秒。
                    </p>
                </div>
            </div>

            {/* Status & Guide */}
            <div className="space-y-6">
                {result && (
                    <div className={`p-6 rounded-xl border ${result.success ? 'bg-emerald-500/10 border-emerald-500/20' : 'bg-destructive/10 border-destructive/20'} animate-in fade-in slide-in-from-right-4`}>
                        <div className="flex items-start gap-4">
                            {result.success ? (
                                <div className="p-2 bg-emerald-500 text-white rounded-full shadow-lg shadow-emerald-500/30">
                                    <CheckCircle2 className="w-6 h-6" />
                                </div>
                            ) : (
                                <div className="p-2 bg-destructive text-white rounded-full shadow-lg shadow-destructive/30">
                                    <XCircle className="w-6 h-6" />
                                </div>
                            )}
                            <div className="space-y-1">
                                <h3 className={`font-semibold text-lg ${result.success ? 'text-emerald-500' : 'text-destructive'}`}>
                                    {result.success ? '同步成功' : '同步失败'}
                                </h3>
                                <p className="text-sm opacity-90">{result.message}</p>

                                {result.deployment_url && (
                                    <div className="pt-3 mt-3 border-t border-emerald-500/20">
                                        <a href={`https://${result.deployment_url}`} target="_blank" rel="noopener noreferrer" className="inline-flex items-center gap-1 text-sm font-medium hover:underline">
                                            访问部署地址 <ExternalLink className="w-3 h-3" />
                                        </a>
                                    </div>
                                )}
                            </div>
                        </div>
                    </div>
                )}

                <div className="bg-secondary/20 border border-border rounded-xl p-6">
                    <h3 className="font-semibold flex items-center gap-2 mb-4">
                        <Info className="w-5 h-5 text-primary" />
                        工作原理
                    </h3>
                    <ul className="space-y-4">
                        <li className="flex gap-3">
                            <span className="shrink-0 w-6 h-6 rounded-full bg-background border border-border flex items-center justify-center text-xs font-bold text-muted-foreground">1</span>
                            <p className="text-sm text-muted-foreground">当前配置 (密钥和账号) 被导出为 JSON 字符串。</p>
                        </li>
                        <li className="flex gap-3">
                            <span className="shrink-0 w-6 h-6 rounded-full bg-background border border-border flex items-center justify-center text-xs font-bold text-muted-foreground">2</span>
                            <p className="text-sm text-muted-foreground">JSON 被编码为 Base64 以确保格式兼容性。</p>
                        </li>
                        <li className="flex gap-3">
                            <span className="shrink-0 w-6 h-6 rounded-full bg-background border border-border flex items-center justify-center text-xs font-bold text-muted-foreground">3</span>
                            <p className="text-sm text-muted-foreground">更新 Vercel 项目中的 <code className="bg-background px-1 py-0.5 rounded border border-border text-xs">DS2API_CONFIG_JSON</code> 环境变量。</p>
                        </li>
                        <li className="flex gap-3">
                            <span className="shrink-0 w-6 h-6 rounded-full bg-background border border-border flex items-center justify-center text-xs font-bold text-muted-foreground">4</span>
                            <p className="text-sm text-muted-foreground">触发重新部署以应用新的环境变量。</p>
                        </li>
                    </ul>
                </div>
            </div>
        </div>
    )
}
