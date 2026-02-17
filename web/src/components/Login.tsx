import { useState } from 'react'

interface LoginProps {
  onLogin: (username: string, password: string, domain?: string) => Promise<void>
  onSwitch: () => void
}

export function Login({ onLogin, onSwitch }: LoginProps) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [domain, setDomain] = useState('')
  const [showOtherServer, setShowOtherServer] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      await onLogin(username, password, domain.trim() || undefined)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed')
    } finally {
      setLoading(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="auth-form">
      <div className="auth-field">
        <label className="auth-label">Имя пользователя</label>
        <input
          type="text"
          placeholder="username"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          className="auth-input"
          autoComplete="username"
          required
        />
        <p className="auth-hint">Можно ввести просто имя — сервер сам найдёт ваш домен среди связанных.</p>
      </div>
      <div className="auth-field">
        <label className="auth-label">Пароль</label>
        <input
          type="password"
          placeholder="••••••••"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          className="auth-input"
          autoComplete="current-password"
          required
        />
      </div>
      {!showOtherServer ? (
        <button
          type="button"
          onClick={() => setShowOtherServer(true)}
          className="auth-link"
          style={{ marginBottom: 8 }}
        >
          Войти с другого сервера (указать домен)
        </button>
      ) : (
        <div className="auth-field">
          <label className="auth-label">Домен домашнего сервера</label>
          <input
            type="text"
            placeholder="например astralrelay.online"
            value={domain}
            onChange={(e) => setDomain(e.target.value)}
            className="auth-input"
            autoComplete="off"
          />
        </div>
      )}
      {error && <p className="auth-error">{error}</p>}
      <button type="submit" disabled={loading} className="auth-button btn-primary">
        {loading ? <span className="loading-dots">Вход</span> : 'Войти'}
      </button>
      <button type="button" onClick={onSwitch} className="auth-link">
        Нет аккаунта? Регистрация
      </button>
    </form>
  )
}
