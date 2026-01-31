import { useState } from 'react'
import { Key, ArrowRight, ShieldCheck, Lock } from 'lucide-react'
import clsx from 'clsx'

export default function Login({ onLogin, onMessage }) {
    const [adminKey, setAdminKey] = useState('')
    const [loading, setLoading] = useState(false)
    const [remember, setRemember] = useState(true)

    const handleLogin = async (e) => {
        e.preventDefault()
        if (!adminKey.trim()) return

        setLoading(true)

        try {
            const res = await fetch('/admin/login', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ admin_key: adminKey }),
            })

            const data = await res.json()

            if (res.ok && data.success) {
                const storage = remember ? localStorage : sessionStorage
                storage.setItem('ds2api_token', data.token)
                storage.setItem('ds2api_token_expires', Date.now() + data.expires_in * 1000)

                onLogin(data.token)
                if (data.message) {
                    onMessage('warning', data.message)
                }
            } else {
                onMessage('error', data.detail || '登录失败')
            }
        } catch (e) {
            onMessage('error', '网络错误: ' + e.message)
        } finally {
            setLoading(false)
        }
    }

    return (
        <div className="min-h-screen w-full flex flex-col items-center justify-center p-4 bg-background text-foreground">

            <div className="w-full max-w-[400px] relative z-10 animate-in fade-in zoom-in-95 duration-200">
                <div className="w-full bg-card border border-border rounded-xl p-8 shadow-sm">
                    <div className="text-center space-y-2 mb-8">
                        <div className="inline-flex items-center justify-center w-12 h-12 rounded-xl bg-primary/10 text-primary mb-2">
                            <Lock className="w-6 h-6" />
                        </div>
                        <h1 className="text-xl font-semibold tracking-tight text-foreground">
                            欢迎回来
                        </h1>
                        <p className="text-muted-foreground text-sm">
                            请输入管理员密钥继续
                        </p>
                    </div>

                    <form onSubmit={handleLogin} className="space-y-4">
                        <div className="space-y-4">
                            <div className="space-y-2">
                                <label className="text-xs font-medium text-muted-foreground ml-0.5">
                                    管理员密钥
                                </label>
                                <div className="relative group">
                                    <div className="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none text-muted-foreground/50 transition-colors">
                                        <Key className="w-4 h-4" />
                                    </div>
                                    <input
                                        type="password"
                                        id="admin_key"
                                        className="w-full h-12 px-4 bg-[#09090b] border border-border rounded-lg text-sm focus:ring-2 focus:ring-primary/20 focus:border-primary transition-all text-foreground font-mono"
                                        placeholder="••••••••••••••••"
                                        value={adminKey}
                                        onChange={(e) => setAdminKey(e.target.value)}
                                        onKeyDown={(e) => e.key === 'Enter' && handleLogin()}
                                    />
                                </div>
                            </div>

                            <div className="flex items-center space-x-2.5">
                                <button
                                    type="button"
                                    role="checkbox"
                                    aria-checked={remember}
                                    onClick={() => setRemember(!remember)}
                                    className={clsx(
                                        "w-4 h-4 rounded border flex items-center justify-center transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-ring/40",
                                        remember ? "bg-primary border-primary text-primary-foreground" : "border-muted-foreground/40 bg-transparent hover:border-muted-foreground"
                                    )}
                                >
                                    {remember && <div className="w-2 h-2 rounded-[1px] bg-current" />}
                                </button>
                                <span
                                    onClick={() => setRemember(!remember)}
                                    className="text-xs text-muted-foreground cursor-pointer select-none hover:text-foreground transition-colors"
                                >
                                    记住登录状态
                                </span>
                            </div>
                        </div>

                        <button
                            type="submit"
                            disabled={loading}
                            className="w-full flex items-center justify-center py-2.5 px-4 rounded-lg bg-primary hover:bg-primary/90 text-primary-foreground font-medium text-sm transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-ring disabled:opacity-50 disabled:cursor-not-allowed"
                        >
                            {loading ? (
                                <div className="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin" />
                            ) : (
                                <div className="flex items-center gap-2">
                                    <span>登录</span>
                                    <ArrowRight className="w-4 h-4" />
                                </div>
                            )}
                        </button>
                    </form>

                    <div className="mt-6 pt-6 border-t border-border flex justify-center">
                        <div className="flex items-center gap-1.5 text-[10px] text-muted-foreground/60 font-medium tracking-wide uppercase">
                            <ShieldCheck className="w-3 h-3" />
                            <span>Secured Connection</span>
                        </div>
                    </div>
                </div>

                <div className="mt-8 text-center">
                    <p className="text-[10px] text-muted-foreground/30 font-mono">DS2API Admin Portal</p>
                </div>
            </div>
        </div>
    )
}
