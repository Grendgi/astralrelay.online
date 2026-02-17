import { useState } from 'react'
import { Landing } from './components/Landing'
import { Chat } from './components/Chat'
import { useAuth } from './hooks/useAuth'
import { useTheme } from './hooks/useTheme'

function App() {
  const { user, token, keys, keysLocked, login, register, logout, addOtpks, rotateSignedPrekey, unlockKeys, lockKeysWithPassphrase } = useAuth()
  const { theme, toggleTheme } = useTheme()
  const [showRegister, setShowRegister] = useState(false)
  const [unlockPassphrase, setUnlockPassphrase] = useState('')
  const [unlockError, setUnlockError] = useState('')
  const [unlocking, setUnlocking] = useState(false)

  if (user && token && keysLocked) {
    return (
      <div className="auth-bg auth-container">
        <div className="auth-card">
          <h1 className="auth-title">Разблокировать ключи E2EE</h1>
          <p className="auth-subtitle">Введите пароль для доступа к ключам шифрования</p>
          <form
            className="auth-form"
            onSubmit={async (e) => {
              e.preventDefault()
              setUnlockError('')
              setUnlocking(true)
              try {
                await unlockKeys(unlockPassphrase)
              } catch (err) {
                setUnlockError(err instanceof Error ? err.message : 'Ошибка разблокировки')
              } finally {
                setUnlocking(false)
              }
            }}
          >
            <div className="auth-field">
              <label className="auth-label">Пароль ключей</label>
              <input
                type="password"
                className="auth-input"
                placeholder="••••••••"
                value={unlockPassphrase}
                onChange={(e) => setUnlockPassphrase(e.target.value)}
                autoComplete="current-password"
                required
              />
            </div>
            {unlockError && <p className="auth-error">{unlockError}</p>}
            <button type="submit" className="auth-button btn-primary" disabled={unlocking}>
              {unlocking ? '…' : 'Разблокировать'}
            </button>
          </form>
        </div>
      </div>
    )
  }

  if (user && token) {
    return <Chat user={user} token={token} keys={keys} onLogout={logout} addOtpks={addOtpks} rotateSignedPrekey={rotateSignedPrekey} lockKeysWithPassphrase={lockKeysWithPassphrase} />
  }

  return (
    <div className="auth-bg auth-container">
      <button onClick={toggleTheme} className="auth-theme-toggle" title={theme === 'dark' ? 'Светлая тема' : 'Тёмная тема'}>
        {theme === 'dark' ? '☀️' : '🌙'}
      </button>
      <Landing
        showRegister={showRegister}
        onShowRegisterChange={setShowRegister}
        onLogin={login}
        onRegister={register}
      />
    </div>
  )
}

export default App
