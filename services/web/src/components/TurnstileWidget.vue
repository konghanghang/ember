<template>
  <div ref="containerRef" class="turnstile-wrapper"></div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted, onBeforeUnmount } from 'vue'

const props = defineProps<{
  siteKey: string
  action?: string
}>()

const emit = defineEmits<{
  (event: 'resolved', token: string): void
  (event: 'ready'): void
  (event: 'error', message: string): void
}>()

const containerRef = ref<HTMLElement | null>(null)
const widgetId = ref<number | null>(null)
let rendering = false

type TurnstileOptions = {
  sitekey: string
  callback: (token: string) => void
  'expired-callback'?: () => void
  'error-callback'?: () => void
  action?: string
  theme?: 'light' | 'dark'
  tabindex?: number
}

type TurnstileInstance = {
  render: (element: HTMLElement, options: TurnstileOptions) => number
  reset: (widgetId?: number) => void
}

interface TurnstileWindow extends Window {
  turnstile?: TurnstileInstance
}

const turnstileUrl = 'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit'
let scriptLoader: Promise<void> | null = null

const ensureScriptLoaded = () => {
  if (scriptLoader) {
    return scriptLoader
  }

  const globalTurnstile = (window as TurnstileWindow).turnstile
  if (globalTurnstile) {
    scriptLoader = Promise.resolve()
    return scriptLoader
  }

  const existingScript = document.querySelector<HTMLScriptElement>(`script[src="${turnstileUrl}"]`)
  if (existingScript) {
    scriptLoader = new Promise((resolve, reject) => {
      const onLoad = () => {
        resolve()
      }
      const onError = () => {
        scriptLoader = null
        reject(new Error('Turnstile script failed to load'))
      }
      existingScript.addEventListener('load', onLoad, { once: true })
      existingScript.addEventListener('error', onError, { once: true })
    })
    return scriptLoader
  }

  scriptLoader = new Promise((resolve, reject) => {
    const script = document.createElement('script')
    script.src = turnstileUrl
    script.async = true
    script.defer = true
    script.setAttribute('data-ember-turnstile', 'true')
    script.addEventListener('load', () => resolve(), { once: true })
    script.addEventListener('error', () => {
      scriptLoader = null
      reject(new Error('Turnstile script failed to load'))
    }, { once: true })
    document.head.appendChild(script)
  })

  return scriptLoader
}

const renderWidget = async () => {
  if (rendering || !props.siteKey || !containerRef.value) {
    return
  }

  rendering = true
  containerRef.value.innerHTML = ''

  try {
    await ensureScriptLoaded()
    const turnstile = (window as TurnstileWindow).turnstile
    if (!turnstile) {
      throw new Error('Turnstile is unavailable')
    }

    widgetId.value = turnstile.render(containerRef.value, {
      sitekey: props.siteKey,
      callback: (token: string) => emit('resolved', token),
      'expired-callback': () => emit('error', 'Turnstile 校验已过期，请重新完成验证'),
      'error-callback': () => emit('error', 'Turnstile 校验失败，请重试'),
      action: props.action ?? 'login',
      theme: 'light',
      tabindex: 0
    })
    emit('ready')
  } catch (error) {
    emit('error', 'Turnstile 加载失败，请刷新页面')
  } finally {
    rendering = false
  }
}

const resetWidget = () => {
  const turnstile = (window as TurnstileWindow).turnstile
  if (!turnstile || widgetId.value === null || !containerRef.value) {
    return
  }

  turnstile.reset(widgetId.value)
}

defineExpose({ reset: resetWidget })

onMounted(() => {
  void renderWidget()
})

watch(
  () => props.siteKey,
  (current) => {
    if (!current) {
      return
    }
    widgetId.value = null
    void renderWidget()
  }
)

onBeforeUnmount(() => {
  resetWidget()
})
</script>
