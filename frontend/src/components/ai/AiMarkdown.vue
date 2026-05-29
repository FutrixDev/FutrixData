<template>
  <div class="ai-markdown" v-html="rendered"></div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import DOMPurify from 'dompurify'
import { marked } from 'marked'

const props = defineProps<{ content: string }>()

marked.setOptions({
  gfm: true,
  breaks: true,
})

const rendered = computed(() => {
  const raw = props.content || ''
  const html = marked.parse(raw) as string
  return DOMPurify.sanitize(html, { USE_PROFILES: { html: true } })
})
</script>
