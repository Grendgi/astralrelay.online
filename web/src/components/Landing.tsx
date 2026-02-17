import { useEffect, useState } from 'react'
import { api } from '../api/client'
import { Login } from './Login'
import { Register } from './Register'

interface LandingProps {
  showRegister: boolean
  onShowRegisterChange: (show: boolean) => void
  onLogin: (username: string, password: string) => Promise<void>
  onRegister: (username: string, password: string) => Promise<void>
}

export function Landing({
  showRegister,
  onShowRegisterChange,
  onLogin,
  onRegister,
}: LandingProps) {
  const [stats, setStats] = useState<{ users: number; servers: number } | null>(null)

  useEffect(() => {
    api
      .getStats()
      .then(setStats)
      .catch(() => setStats(null))
  }, [])

  return (
    <div className="landing">
      {stats !== null && (
        <div className="landing-stats">
          <div className="landing-stat">
            <span className="landing-stat-value">{stats.users}</span>
            <span className="landing-stat-label">пользователей</span>
          </div>
          <div className="landing-stat">
            <span className="landing-stat-value">{stats.servers}</span>
            <span className="landing-stat-label">серверов в федерации</span>
          </div>
        </div>
      )}
      <div className="auth-card">
        <h1 className="auth-title">Messenger</h1>
        <p className="auth-subtitle">E2EE · Федеративный</p>
        {showRegister ? (
          <Register onRegister={onRegister} onSwitch={() => onShowRegisterChange(false)} />
        ) : (
          <Login onLogin={onLogin} onSwitch={() => onShowRegisterChange(true)} />
        )}
      </div>
    </div>
  )
}
