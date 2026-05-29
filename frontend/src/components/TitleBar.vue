<template>
  <div
    class="title-bar flex items-center bg-muted/30 dark:bg-muted/20 backdrop-blur-md border-b border-border/60 h-14"
    style="--wails-draggable: drag"
    @dblclick="toggleMaximise"
  >
    <div class="flex-1 flex items-center h-full min-w-0 pl-6">
      <div class="flex items-center gap-3 text-sm">
        <img :src="logoSvgUrl" alt="FutrixData" class="h-10 w-auto select-none" draggable="false">
        <span class="text-[12px] uppercase tracking-[0.3em] text-muted-foreground mt-1">FutrixData</span>
        <span class="text-muted-foreground/70">•</span>
        <span class="font-medium text-foreground">{{ pageTitle }}</span>
      </div>
    </div>
    <div class="flex items-center h-full pr-4 space-x-2" style="--wails-draggable: no-drag" data-no-drag>
      <button
        data-testid="ai-toggle"
        class="btn ghost ai-toggle"
        type="button"
        :title="tApp('titleBar.ai')"
        :aria-label="tApp('titleBar.ai')"
        @click="aiStore.toggleOpen()"
      >
        <span class="ai-composer-icon ai-toggle-icon" aria-hidden="true">
          <svg class="ai-composer-glyph" viewBox="0 0 24 24" aria-hidden="true">
            <path
              d="M12 3.5l2.6 5.5 6 .9-4.3 4.2 1 6-5.3-3-5.3 3 1-6L3.4 9.9l6-.9L12 3.5z"
              fill="none"
              stroke="currentColor"
              stroke-width="1.4"
              stroke-linejoin="round"
            />
          </svg>
        </span>
      </button>
      <div v-if="!isMac" class="window-controls flex items-center pl-2 border-l border-border/60">
        <button class="window-control" type="button" :title="tApp('titleBar.minimize')" :aria-label="tApp('titleBar.minimize')" @click="minimise">
          <Minus :size="14" />
        </button>
        <button class="window-control" type="button" :title="tApp('titleBar.maximize')" :aria-label="tApp('titleBar.maximize')" @click="toggleMaximise">
          <Square :size="12" />
        </button>
        <button class="window-control danger" type="button" :title="tApp('titleBar.close')" :aria-label="tApp('titleBar.close')" @click="closeWindow">
          <X :size="14" />
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { Minus, Square, X } from 'lucide-vue-next'
import { Environment, WindowMinimise, WindowToggleMaximise, Quit } from '@wailsjs/runtime/runtime'
import { useAiChatStore } from '@/stores/ai-chat'
import { tApp } from '@/modules/i18n/appI18n'
import logoSvgUrl from '@/assets/svgs/logo.svg'

const isMac = ref(false)
const route = useRoute()
const aiStore = useAiChatStore()
const pageTitle = computed(() => {
  const titleKey = route.meta?.titleKey
  if (typeof titleKey === 'string' && titleKey) {
    return tApp(titleKey)
  }
  const fallbackTitle = route.meta?.title
  if (typeof fallbackTitle === 'string' && fallbackTitle) {
    return fallbackTitle
  }
  return tApp('route.default')
})

const hasWailsRuntime = () =>
  typeof window !== 'undefined' && Boolean((window as { runtime?: unknown }).runtime)

onMounted(async () => {
  if (!hasWailsRuntime()) {
    return
  }
  try {
    const env = await Environment()
    isMac.value = env.platform === 'darwin'
  } catch {
    isMac.value = false
  }
})

const minimise = () => {
  if (!hasWailsRuntime()) return
  WindowMinimise()
}

const toggleMaximise = (event?: MouseEvent) => {
  if (event) {
    const target = event.target as HTMLElement | null
    if (target && target.closest('button, a, input, select, textarea, [data-no-drag]')) {
      return
    }
  }
  if (!hasWailsRuntime()) return
  WindowToggleMaximise()
}

const closeWindow = () => {
  if (!hasWailsRuntime()) return
  Quit()
}
</script>

<style scoped>
.title-bar {
  user-select: none;
}

.ai-toggle-icon {
  width: 18px;
  height: 18px;
}

.window-control {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  border: 1px solid var(--edge);
  background: transparent;
  color: var(--soft-ink);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.window-control:hover {
  background: rgba(255, 255, 255, 0.15);
  color: var(--ink);
}

.window-control.danger:hover {
  color: #ef4444;
  background: rgba(239, 68, 68, 0.2);
}

:global(.dark) .title-bar img[alt="FutrixData"] {
  filter: brightness(1.8);
}
</style>
