import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import path from 'path'
import { execFileSync } from 'child_process'

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

function readGitCommitSha() {
  try {
    return execFileSync('git', ['-C', path.resolve(__dirname, '../..'), 'rev-parse', '--short=12', 'HEAD'], {
      encoding: 'utf8',
    }).trim()
  } catch {
    return ''
  }
}

const gitCommitSha = process.env.VITE_GIT_COMMIT_SHA?.trim() || process.env.GITHUB_SHA?.slice(0, 12) || readGitCommitSha()
const githubRepository = process.env.VITE_GITHUB_REPOSITORY?.trim() || process.env.GITHUB_REPOSITORY?.trim() || 'konghanghang/ember'
const githubRepositoryUrl = process.env.VITE_GITHUB_REPOSITORY_URL?.trim() || `https://github.com/${githubRepository}`

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue()],
  define: {
    'import.meta.env.VITE_GIT_COMMIT_SHA': JSON.stringify(gitCommitSha),
    'import.meta.env.VITE_GITHUB_REPOSITORY': JSON.stringify(githubRepository),
    'import.meta.env.VITE_GITHUB_REPOSITORY_URL': JSON.stringify(githubRepositoryUrl),
  },
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
