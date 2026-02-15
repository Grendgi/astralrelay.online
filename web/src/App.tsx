import { useState } from 'react'
import { Login } from './components/Login'
import { Register } from './components/Register'
import { Chat } from './components/Chat'
import { useAuth } from './hooks/useAuth'
import { useTheme } from './hooks/useTheme'

function App() {
  const { user, token, keys, login, register, logout, addOtpks, rotateSignedPrekey } = useAuth()
  const { theme, toggleTheme } = useTheme()
  const [showRegister, setShowRegister] = useState(false)

  if (user && token) {
    return <Chat user={user} token={token} keys={keys} onLogout={logout} addOtpks={addOtpks} rotateSignedPrekey={rotateSignedPrekey} />
  }

  return (
    <div className="auth-bg auth-container">
      <button onClick={toggleTheme} className="auth-theme-toggle" title={theme === 'dark' ? 'Светлая тема' : 'Тёмная тема'}>
        {theme === 'dark' ? '☀️' : '🌙'}
      </button>
      <div className="auth-card">
        <h1 className="auth-title">Messenger</h1>
        <p className="auth-subtitle">E2EE · Федеративный</p>

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

export default App
