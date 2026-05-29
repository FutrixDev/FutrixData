<script setup lang="ts">
defineProps<{
  eyebrow?: string
  title?: string
  titleMono?: boolean
  chip?: string
  chipTone?: 'indigo' | 'amber' | 'slate' | 'rose'
}>()

const chipToneClass = (tone?: string) => {
  switch (tone) {
    case 'amber':
      return 'bg-amber-50 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300'
    case 'rose':
      return 'bg-rose-50 text-rose-700 dark:bg-rose-500/15 dark:text-rose-300'
    case 'slate':
      return 'bg-slate-100 text-slate-600 dark:bg-slate-700/40 dark:text-slate-300'
    case 'indigo':
    default:
      return 'bg-indigo-50 text-indigo-700 dark:bg-indigo-500/15 dark:text-indigo-300'
  }
}
</script>

<template>
  <header class="console-pane-header flex items-center justify-between gap-3 min-h-[44px] px-4 shrink-0">
    <div class="min-w-0 flex flex-col gap-0.5">
      <div
        v-if="eyebrow"
        class="text-[10px] font-semibold uppercase tracking-[0.08em] text-slate-400 dark:text-text-muted-dark"
      >
        {{ eyebrow }}
      </div>
      <div class="flex items-center gap-2 min-w-0">
        <h2
          v-if="title"
          class="m-0 truncate text-[13px] font-semibold text-slate-800 dark:text-text-main-dark"
          :class="titleMono ? 'font-mono' : ''"
        >
          {{ title }}
        </h2>
        <span
          v-if="chip"
          class="inline-flex items-center px-1.5 py-0.5 text-[10px] font-semibold rounded uppercase tracking-wide"
          :class="chipToneClass(chipTone)"
        >
          {{ chip }}
        </span>
        <slot name="meta" />
      </div>
    </div>
    <div class="flex items-center gap-1 shrink-0">
      <slot name="actions" />
    </div>
  </header>
</template>
