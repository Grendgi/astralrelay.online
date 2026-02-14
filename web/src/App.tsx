import { useState } from 'react'
import { Login } from './components/Login'
import { Register } from './components/Register'
import { Chat } from './components/Chat'
import { useAuth } from './hooks/useAuth'
import { useTheme } from './hooks/useTheme'

function App() {
  const { user, token, keys, login, register, logout } = useAuth()
  const { theme, toggleTheme } = useTheme()
  const [showRegister, setShowRegister] = useState(false)

  if (user && token) {
    return <Chat user={user} token={token} keys={keys} onLogout={logout} />
  }

  return (
    <div style={styles.container} className="auth-bg">
      <button onClick={toggleTheme} style={styles.themeToggle} title={theme === 'dark' ? 'Светлая тема' : 'Тёмная тема'}>
        {theme === 'dark' ? '☀️' : '🌙'}
      </button>
      <div style={styles.card}>
        <h1 style={styles.title}>Messenger</h1>
        <p style={styles.subtitle}>E2EE · Федеративный</p>

        {showRegister ? (
          <Register
            onRegister={register}
            onSwitch={() => setShowRegister(false)}
          />
        ) : (
          <Login
            onLogin={login}
            onSwitch={() => setShowRegister(true)}
          />
        )}
      </div>
    </div>
  )
}

const styles: Record<string, React.CSSProperties> = {
  container: {
    minHeight: '100vh',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    padding: 16,
    position: 'relative',
  },
  themeToggle: {
    position: 'absolute',
    top: 16,
    right: 16,
    background: 'var(--surface)',
    border: '1px solid var(--border)',
    color: 'var(--text)',
    padding: '8px 12px',
    borderRadius: 8,
    cursor: 'pointer',
    fontSize: 18,
  },
  card: {
    background: 'var(--surface)',
    borderRadius: 16,
    padding: 40,
    width: '100%',
    maxWidth: 400,
    border: '1px solid var(--border)',
    boxShadow: '0 8px 32px rgba(0,0,0,0.12)',
  },
  title: {
    margin: 0,
    fontSize: 28,
    fontWeight: 700,
    letterSpacing: '-0.02em',
  },
  subtitle: {
    margin: '8px 0 24px',
    color: 'var(--text-muted)',
    fontSize: 14,
  },
}

export default App
