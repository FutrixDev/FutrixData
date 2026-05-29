<script setup lang="ts">
import { computed } from 'vue'
import { redisTypeAccent, redisTypeShort, normalizeRedisType } from '@/modules/redis/type-theme'
import { tApp } from '@/modules/i18n/appI18n'

type BadgeState = 'pending' | 'resolved' | 'error'

const props = withDefaults(
  defineProps<{
    type?: string | null | undefined
    state?: BadgeState
  }>(),
  {
    type: '',
    state: 'resolved',
  },
)

const resolvedState = computed<BadgeState>(() => {
  if (props.state === 'pending' || props.state === 'error') return props.state
  return normalizeRedisType(props.type) === 'UNKNOWN' ? 'error' : 'resolved'
})

const accent = computed(() => redisTypeAccent(props.type))
const short = computed(() => redisTypeShort(props.type))
</script>

<template>
  <span
    v-if="resolvedState === 'pending'"
    class="redis-type-badge--pending inline-flex items-center gap-0.5 px-1 py-0.5 rounded border border-slate-200 dark:border-slate-700/60 bg-slate-100/60 dark:bg-slate-700/30"
    :aria-label="tApp('redis.typeBadge.ariaLoading')"
  >
    <span class="bar" />
    <span class="bar" />
    <span class="bar" />
  </span>
  <span
    v-else-if="resolvedState === 'error'"
    :class="accent.pill"
    :aria-label="tApp('redis.typeBadge.ariaUnknown')"
  >?</span>
  <span v-else :class="accent.pill">{{ short }}</span>
</template>

<style scoped>
.redis-type-badge--pending {
  width: 32px;
  height: 16px;
  vertical-align: middle;
}
.redis-type-badge--pending .bar {
  display: inline-block;
  width: 6px;
  height: 6px;
  border-radius: 2px;
  background: currentColor;
  opacity: 0.35;
  animation: redis-badge-pulse 1.2s ease-in-out infinite;
}
.redis-type-badge--pending .bar:nth-child(2) {
  animation-delay: 0.15s;
}
.redis-type-badge--pending .bar:nth-child(3) {
  animation-delay: 0.3s;
}
@keyframes redis-badge-pulse {
  0%, 100% { opacity: 0.25; }
  50% { opacity: 0.7; }
}
</style>
