<template>
  <button
    class="theme-toggle flex shrink-0 items-center justify-center w-10 h-10 rounded-full text-muted-foreground bg-muted/30 hover:bg-muted/60 hover:text-foreground transition-colors relative border border-muted/40 shadow-sm"
    type="button"
    :title="themeSwitchLabel"
    :aria-label="themeSwitchLabel"
    @click="toggleTheme"
  >
    <img
      :src="nextTheme === 'dark' ? moonIcon : sunIcon"
      class="w-[18px] h-[18px]"
      alt=""
      aria-hidden="true"
      draggable="false"
    />
  </button>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { WindowSetDarkTheme, WindowSetLightTheme } from '@wailsjs/runtime/runtime'
import sunIcon from '@/assets/svgs/sun.svg'
import moonIcon from '@/assets/svgs/moon.svg'
import { tApp } from '@/modules/i18n/appI18n'

type Theme = 'light' | 'dark'

const currentTheme = ref<Theme>('light')

const nextTheme = computed(() => (currentTheme.value === 'light' ? 'dark' : 'light'))
const nextThemeLabel = computed(() => tApp(nextTheme.value === 'dark' ? 'theme.dark' : 'theme.light'))
const themeSwitchLabel = computed(() => tApp('theme.switchToTheme', { theme: nextThemeLabel.value }))

const hasWailsRuntime = () =>
  typeof window !== 'undefined' && Boolean((window as { runtime?: unknown }).runtime)

const hasLocalStorage = () =>
  typeof localStorage !== 'undefined' &&
  typeof localStorage.getItem === 'function' &&
  typeof localStorage.setItem === 'function'

const applyTheme = (theme: Theme) => {
  const html = document.documentElement
  html.classList.remove('light', 'dark')
  html.classList.add(theme)
  if (!hasWailsRuntime()) {
    return
  }
  if (theme === 'dark') {
    WindowSetDarkTheme()
  } else {
    WindowSetLightTheme()
  }
}

const toggleTheme = () => {
  currentTheme.value = nextTheme.value
  applyTheme(currentTheme.value)
  if (hasLocalStorage()) {
    localStorage.setItem('theme', currentTheme.value)
  }
}

onMounted(() => {
  const saved = hasLocalStorage() ? (localStorage.getItem('theme') as Theme | null) : null
  if (saved === 'light' || saved === 'dark') {
    currentTheme.value = saved
  } else if (window.matchMedia('(prefers-color-scheme: dark)').matches) {
    currentTheme.value = 'dark'
  }
  applyTheme(currentTheme.value)
})
</script>
