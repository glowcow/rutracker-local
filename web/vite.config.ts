import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    // Content-hashed output would otherwise accumulate generation after
    // generation in dist/, and `go:embed all:dist` bakes every one of them in.
    emptyOutDir: true,
  },
  server: {
    host: '0.0.0.0',
    port: 5173,
    watch: {
      usePolling: true,
    },
    // Dev only: /api goes to the Go backend. VITE_API_HOST points it
    // elsewhere — e.g. host.docker.internal when vite runs in a container.
    proxy: {
      '/api': {
        target: process.env.VITE_API_HOST || 'http://localhost:8080',
        changeOrigin: true,
        // The API refuses a mutation whose Origin is not its own host, and
        // the proxy would forward the browser's localhost one.
        configure(proxy) {
          proxy.on('proxyReq', (proxyReq) => {
            proxyReq.removeHeader('origin')
            proxyReq.removeHeader('sec-fetch-site')
          })
        },
      },
    },
  },
})
