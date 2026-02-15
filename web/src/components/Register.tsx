import { useState } from 'react'

interface RegisterProps {
  onRegister: (username: string, password: string) => Promise<void>
  onSwitch: () => void
}

export function Register({ onRegister, onSwitch }: RegisterProps) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      await onRegister(username, password)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Registration failed')
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
          placeholder="буквы, цифры, дефис, подчёркивание"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          className="auth-input"
          pattern="[a-zA-Z0-9\-_]+"
          autoComplete="username"
          required
        />
      </div>
      <div className="auth-field">
        <label className="auth-label">Пароль</label>
        <input
          type="password"
          placeholder="минимум 6 символов"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          className="auth-input"
          minLength={6}
          autoComplete="new-password"
          required
        />
      </div>
      {error && <p className="auth-error">{error}</p>}
      <button type="submit" disabled={loading} className="auth-button btn-primary">
        {loading ? <span className="loading-dots">Регистрация</span> : 'Зарегистрироваться'}
      </button>
      <button type="button" onClick={onSwitch} className="auth-link">
        Уже есть аккаунт? Войти
      </button>
    </form>
  )
}
