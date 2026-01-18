import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: '../app/frontend_dist',
    emptyOutDir: true,
  },
  server: {
    port: 34115,
  },
})


