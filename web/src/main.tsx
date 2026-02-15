import * as React from 'react'
import { createRoot } from 'react-dom/client'
import App from './App'
import './index.css'

// Theme init before render (avoids inline script for CSP)
;(function () {
  const t = localStorage.getItem('messenger-theme')
  if (t === 'light' || t === 'dark') {
    document.documentElement.setAttribute('data-theme', t)
  } else if (window.matchMedia('(prefers-color-scheme: light)').matches) {
    document.documentElement.setAttribute('data-theme', 'light')
  }
})()

createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
)
