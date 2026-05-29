<template>
  <div
    class="app-nav-panel bg-sidebar/70 border-r border-sidebar-border/60 shadow-sm flex flex-col items-stretch pt-12 pb-6 pl-5 pr-3 gap-4"
  >


    <div class="flex flex-col gap-2">
      <router-link
        v-for="item in sidebarItems"
        :key="item.id"
        :to="item.to"
        class="app-nav-link flex items-center gap-3 rounded-xl px-3 py-2.5 text-[13px] font-semibold text-muted-foreground hover:bg-muted/70 hover:text-foreground transition-colors relative border border-transparent"
        :class="{
          'bg-primary/10 text-foreground border-primary/20 shadow-sm': isActive(item.id)
        }"
        :title="item.title"
        :aria-label="item.title"
      >
        <span class="app-nav-icon w-[18px] h-[18px] flex-shrink-0 inline-flex" aria-hidden="true" v-html="item.iconSvg" />
        <span class="app-nav-label leading-tight whitespace-normal break-words">{{ item.title }}</span>
      </router-link>
    </div>

    <div class="mt-auto">
      <div class="h-px bg-sidebar-border/60 mx-2" />
      <div class="app-nav-footer flex items-center justify-between gap-2 px-1 pt-3">
        <router-link
          to="/my"
          class="flex shrink-0 items-center justify-center w-10 h-10 rounded-full text-muted-foreground hover:text-foreground transition-colors relative shadow-sm overflow-hidden"
          :class="[
            userAvatarUrl
              ? 'border-2 border-muted/40 hover:border-primary/40'
              : 'bg-muted/30 hover:bg-muted/60 border border-muted/40',
            isActive('my') && 'border-primary/30 shadow-md ring-1 ring-primary/15'
          ]"
          :title="tApp('nav.my')"
          :aria-label="tApp('nav.my')"
        >
          <img
            v-if="userAvatarUrl"
            :src="userAvatarUrl"
            alt=""
            class="w-full h-full object-cover"
            referrerpolicy="no-referrer"
            @error="($event.target as HTMLImageElement).style.display = 'none'"
          />
          <span v-else class="w-[18px] h-[18px] inline-flex" aria-hidden="true" v-html="myIconSvg" />
        </router-link>

        <ThemeToggle />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import ThemeToggle from '@/components/ThemeToggle.vue'
import aiSettingsIconSvg from '@/assets/svgs/nav-ai-settings.svg?raw'
import historyIconSvg from '@/assets/svgs/nav-history.svg?raw'
import myIconSvg from '@/assets/svgs/nav-my.svg?raw'
import riskRulesIconSvg from '@/assets/svgs/nav-risk-rules.svg?raw'
import sensitivityIconSvg from '@/assets/svgs/nav-sensitivity.svg?raw'
import sourcesIconSvg from '@/assets/svgs/nav-sources.svg?raw'
import { tApp } from '@/modules/i18n/appI18n'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const authStore = useAuthStore()

const userAvatarUrl = computed(() => authStore.currentUser?.avatarUrl || '')
const sidebarItems = computed(() => [
  { id: 'datasources', title: tApp('nav.sources'), iconSvg: sourcesIconSvg, to: '/' },
  { id: 'history', title: tApp('nav.history'), iconSvg: historyIconSvg, to: '/history' },
  { id: 'sensitivity', title: tApp('nav.dataSensitivity'), iconSvg: sensitivityIconSvg, to: '/sensitivity' },
  { id: 'risk-rules', title: tApp('nav.riskRules'), iconSvg: riskRulesIconSvg, to: '/risk-rules' },
  { id: 'ai-settings', title: tApp('nav.aiSettings'), iconSvg: aiSettingsIconSvg, to: '/ai-settings' },
])

const isActive = (id: string) => {
  if (id === 'datasources') {
    return route.path === '/' || route.path.startsWith('/datasources')
  }
  if (id === 'risk-rules') {
    return route.path.startsWith('/risk-rules')
  }
  if (id === 'ai-settings') {
    return route.path.startsWith('/ai-settings')
  }
  if (id === 'history') {
    return route.path.startsWith('/history')
  }
  if (id === 'sensitivity') {
    return route.path.startsWith('/sensitivity')
  }
  if (id === 'my') {
    return route.path.startsWith('/my')
  }
  return false
}
</script>
