import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import path from 'path'

const elementPlusPackages = new Set([
  '@vueuse/core',
  '@vueuse/shared',
])

function getNodeModulePackageName(id: string) {
  const normalizedId = id.replace(/\\/g, '/')
  const modulePath = normalizedId.split('/node_modules/')[1]

  if (!modulePath) {
    return null
  }

  const segments = modulePath.split('/')

  if (segments[0].startsWith('@')) {
    return `${segments[0]}/${segments[1]}`
  }

  return segments[0]
}

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  build: {
    rollupOptions: {
      output: {
        manualChunks(id) {
          const packageName = getNodeModulePackageName(id)

          if (!packageName) {
            return undefined
          }

          if (
            packageName === 'vue'
            || packageName === 'vue-router'
            || packageName === 'pinia'
            || packageName.startsWith('@vue/')
          ) {
            return 'vue-vendor'
          }

          if (elementPlusPackages.has(packageName)) {
            return 'vue-vendor'
          }

          if (packageName === 'axios') {
            return 'network-vendor'
          }

          return undefined
        },
      },
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    include: ['src/**/*.spec.ts', 'src/**/*.test.ts'],
  },
  server: {
    port: 3000,
    proxy: {
      '/api': {
        target: 'http://localhost:8080', // Go API Backend
        changeOrigin: true,
      }
    }
  }
})
