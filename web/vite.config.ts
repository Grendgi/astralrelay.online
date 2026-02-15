import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

const apiTarget = process.env.VITE_API_PROXY_TARGET || 'http://localhost:8080'

export default defineConfig({
  plugins: [react()],
  build: {
    target: 'es2020', // BigInt literals (0n, 8n) used in crypto/fingerprint.ts
  },
  test: {
    setupFiles: ['src/test-setup.ts'],
    environment: 'node',
  },
  server: {
    port: 3000,
    host: '0.0.0.0',
    proxy: {
      '/api': { target: apiTarget, changeOrigin: true },
      '/health': { target: apiTarget, changeOrigin: true },
    },
  },
})
