import path from 'path'
import tailwindcss from '@tailwindcss/vite'
import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vite'
import { resolveWailsAliasDir } from './vite.wails'

const rootDir = path.resolve(__dirname)
const dataRoot = path.resolve(__dirname, '../data')
const isVitest = process.env.VITEST === 'true'
const wailsAliasDir = resolveWailsAliasDir(__dirname, isVitest)

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  optimizeDeps: {
    exclude: ['monaco-editor'],
  },
  build: {
    rollupOptions: {
      external: (id) => id.includes('/data/') && id.endsWith('.json'),
    },
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
      '@wailsjs': wailsAliasDir,
    },
  },
  server: {
    fs: {
      allow: [rootDir, dataRoot],
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    deps: {
      inline: ['vue', '@vue', 'vue-router', 'pinia'],
    },
  },
})
