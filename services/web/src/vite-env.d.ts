/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_GIT_COMMIT_SHA?: string
  readonly VITE_GITHUB_REPOSITORY?: string
  readonly VITE_GITHUB_REPOSITORY_URL?: string
}

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<{}, {}, any>
  export default component
}
