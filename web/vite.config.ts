import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  // Wipe dist/ before building. The old `emptyOutDir: false` claimed Vite
  // "overwrites every file", which is wrong for content-hashed output —
  // every build added a new index-<hash>.js/css generation and local dist
  // had accumulated ~10 of them, all embedded into locally built binaries
  // via `go:embed all:dist`. The committed .gitkeep is re-created by the
  // build script right after.
  build: {
    emptyOutDir: true,
  },
  server: {
    host: '0.0.0.0',
    port: 5173,
    watch: {
      usePolling: true,
    },
    // During dev, /api requests go to the Go backend. Set VITE_API_HOST
    // to point at a different host (default: localhost:8080), e.g. the
    // prod instance for a real-data preview.
    proxy: {
      '/api': {
        target: process.env.VITE_API_HOST || 'http://localhost:8080',
        changeOrigin: true,
        // The API's cross-origin guard rejects mutations whose Origin
        // doesn't match its Host. The proxy forwards the browser's
        // localhost Origin verbatim, so favorites POSTs bounced with 403 —
        // strip the browser-context headers; header-less requests pass.
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
