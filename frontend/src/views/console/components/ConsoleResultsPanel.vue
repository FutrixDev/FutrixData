<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { tApp } from '@/modules/i18n/appI18n'
import ConsoleResultsContent from './ConsoleResultsContent.vue'

const expanded = ref(false)

const openExpanded = () => { expanded.value = true }
const closeExpanded = () => { expanded.value = false }

const dialogOffsetX = ref(0)
const dialogOffsetY = ref(0)

const dragState = {
  dragging: false,
  startX: 0,
  startY: 0,
  startOffsetX: 0,
  startOffsetY: 0,
}

const onKeydown = (event: KeyboardEvent) => {
  if (event.key === 'Escape') {
    closeExpanded()
  }
}

const onDialogDragMove = (event: MouseEvent) => {
  if (!dragState.dragging) return
  dialogOffsetX.value = dragState.startOffsetX + (event.clientX - dragState.startX)
  dialogOffsetY.value = dragState.startOffsetY + (event.clientY - dragState.startY)
}

const endDialogDrag = () => {
  if (!dragState.dragging) return
  dragState.dragging = false
  window.removeEventListener('mousemove', onDialogDragMove)
  window.removeEventListener('mouseup', endDialogDrag)
  document.body.style.userSelect = ''
}

const startDialogDrag = (event: MouseEvent) => {
  if (event.button !== 0) return
  const target = event.target as HTMLElement | null
  if (target?.closest('button, a, input, select, textarea, [data-no-dialog-drag]')) return
  dragState.dragging = true
  dragState.startX = event.clientX
  dragState.startY = event.clientY
  dragState.startOffsetX = dialogOffsetX.value
  dragState.startOffsetY = dialogOffsetY.value
  window.addEventListener('mousemove', onDialogDragMove)
  window.addEventListener('mouseup', endDialogDrag)
  document.body.style.userSelect = 'none'
}

watch(expanded, (open) => {
  if (open) {
    window.addEventListener('keydown', onKeydown)
  } else {
    window.removeEventListener('keydown', onKeydown)
    endDialogDrag()
    dialogOffsetX.value = 0
    dialogOffsetY.value = 0
  }
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
  endDialogDrag()
})
</script>

<template>
  <div class="console-results-panel">
    <ConsoleResultsContent v-if="!expanded" @open-expanded="openExpanded" />
  </div>

  <Teleport to="body">
    <div
      v-if="expanded"
      class="dialog-backdrop dialog-backdrop--results"
      role="dialog"
      aria-modal="true"
      data-testid="results-dialog"
      @click.self="closeExpanded"
    >
      <div
        class="dialog-card dialog-card--results"
        :style="{ transform: `translate(${dialogOffsetX}px, ${dialogOffsetY}px)` }"
      >
        <div class="dialog-head" @mousedown="startDialogDrag">
          <div>
            <h4>{{ tApp('console.resultsPanel.title') }}</h4>
            <div class="meta">{{ tApp('console.resultsPanel.expandedView') }}</div>
          </div>
          <button class="btn ghost small" type="button" data-no-dialog-drag @click="closeExpanded">
            {{ tApp('console.resultsPanel.close') }}
          </button>
        </div>
        <div class="dialog-body dialog-body--results">
          <ConsoleResultsContent variant="dialog" />
        </div>
      </div>
    </div>
  </Teleport>
</template>
