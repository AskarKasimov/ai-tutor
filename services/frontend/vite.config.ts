import { tanstackRouter } from '@tanstack/router-plugin/vite'
import react from '@vitejs/plugin-react'
import { defineConfig, loadEnv } from 'vite'
import { resolve } from 'node:path'

export default defineConfig(({ mode }) => {
  const envDir = resolve(import.meta.dirname, '../..')
  const env = loadEnv(mode, envDir, 'VITE_')
  return {
    envDir,
    server: {
      watch: {
        usePolling: process.env.DEV_POLLING === 'true',
        interval: 300,
      },
      proxy: {
        '/api/v1': {
          target: env.VITE_API_PROXY_TARGET || 'http://127.0.0.1:8002',
          changeOrigin: true,
          rewrite: (path) => path.replace(/^\/api\/v1/, ''),
          proxyTimeout: 240_000,
        },
      },
    },
    plugins: [
      tanstackRouter({
        target: 'react',
        autoCodeSplitting: true,
        routeFileIgnorePattern: '\\.test\\.tsx$',
      }),
      react(),
    ],
  }
})
