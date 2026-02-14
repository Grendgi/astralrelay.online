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
    <form onSubmit={handleSubmit} style={styles.form}>
      <div style={styles.field}>
        <label style={styles.label}>Имя пользователя</label>
        <input
          type="text"
          placeholder="буквы, цифры, дефис, подчёркивание"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          style={styles.input}
          pattern="[a-zA-Z0-9\-_]+"
          autoComplete="username"
          required
        />
      </div>
      <div style={styles.field}>
        <label style={styles.label}>Пароль</label>
        <input
          type="password"
          placeholder="минимум 6 символов"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          style={styles.input}
          minLength={6}
          autoComplete="new-password"
          required
        />
      </div>
      {error && <p style={styles.error}>{error}</p>}
      <button type="submit" disabled={loading} style={styles.button} className="btn-primary">
        {loading ? <span className="loading-dots">Регистрация</span> : 'Зарегистрироваться'}
      </button>
      <button type="button" onClick={onSwitch} style={styles.link}>
        Уже есть аккаунт? Войти
      </button>
    </form>
  )
}

const styles: Record<string, React.CSSProperties> = {
  form: { display: 'flex', flexDirection: 'column', gap: 16 },
  field: { display: 'flex', flexDirection: 'column', gap: 6 },
  label: {
    fontSize: 13,
    fontWeight: 500,
    color: 'var(--text)',
  },
  input: {
    padding: 12,
    borderRadius: 10,
    border: '1px solid var(--border)',
    background: 'var(--bg)',
    color: 'var(--text)',
    fontSize: 15,
    transition: 'border-color 0.2s',
  },
  error: { color: 'var(--error)', fontSize: 13, margin: 0 },
  button: {
    padding: 12,
    borderRadius: 10,
    background: 'var(--accent)',
    color: 'white',
    border: 'none',
    cursor: 'pointer',
    fontWeight: 600,
    fontSize: 15,
    marginTop: 4,
  },
  link: {
    background: 'none',
    border: 'none',
    color: 'var(--accent)',
    cursor: 'pointer',
    fontSize: 14,
    padding: 8,
  },
}
