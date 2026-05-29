<template>
  <div v-if="message" class="notice show" :class="typeClass">
    {{ message }}
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{ message: string; type?: string }>()

const typeClass = computed(() => (props.type === 'error' ? 'error' : ''))
</script>
