<script setup lang="ts">
interface InlineMetaItem {
  label?: string
  value: string
  mono?: boolean
}

defineProps<{
  items: InlineMetaItem[]
}>()
</script>

<template>
  <div class="console-inline-meta flex items-center gap-2 text-[12.5px] text-slate-500 dark:text-text-muted-dark min-w-0">
    <template v-for="(item, idx) in items" :key="idx">
      <span v-if="idx > 0" class="text-slate-300 dark:text-slate-600 select-none" aria-hidden="true">·</span>
      <span class="inline-flex items-center gap-1 truncate">
        <span
          v-if="item.label"
          class="text-[10px] font-semibold uppercase tracking-[0.06em] text-slate-400 dark:text-slate-500"
        >
          {{ item.label }}
        </span>
        <span :class="item.mono ? 'font-mono' : ''" class="truncate">{{ item.value }}</span>
      </span>
    </template>
  </div>
</template>
