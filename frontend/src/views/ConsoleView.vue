<script setup lang="ts">
import { computed, provide } from 'vue'
import { consoleViewContextKey } from './console/context'
import ConsoleDangerDialogs from './console/components/ConsoleDangerDialogs.vue'
import ConsoleEntitiesPanel from './console/components/ConsoleEntitiesPanel.vue'
import ConsoleResultsPanel from './console/components/ConsoleResultsPanel.vue'
import ConsoleStatementPanel from './console/components/ConsoleStatementPanel.vue'
import ConsoleToolbar from './console/components/ConsoleToolbar.vue'
import RedisKeyInspector from './console/components/RedisKeyInspector.vue'
import RedisConsoleShell from './console/components/RedisConsoleShell.vue'
import { useConsoleView } from './console/composables/useConsoleView'
import { tApp } from '@/modules/i18n/appI18n'

const {
  consoleShell,
  statementResultsShell,
  splitResizing,
  statementSplitResizing,
  consoleSplitStyle,
  statementResultsSplitStyle,
  startSplitResize,
  startStatementSplitResize,
  resetSplitWidth,
  resetStatementSplitHeight,
  nudgeSplitWidth,
  nudgeStatementSplitHeight,
  ctx,
} = useConsoleView()
const isRedis = ctx.isRedis
const isSqlEditorParity = ctx.isSqlEditorParity
const parityWorkspaceKind = ctx.parityWorkspaceKind
const isSqlEditorParityActive = computed(() => Boolean(isSqlEditorParity?.value))
const isElasticWorkspace = computed(() => parityWorkspaceKind?.value === 'elastic')
const isChromaWorkspace = computed(() => parityWorkspaceKind?.value === 'chroma')

provide(consoleViewContextKey, ctx)
</script>

<template>
  <section class="view active" id="view-console">
    <ConsoleToolbar />

    <RedisConsoleShell v-if="isRedis" />
    <div
      v-else
      class="console-shell"
      :class="{
        resizing: splitResizing,
        'sql-editor-parity': isSqlEditorParityActive,
        'elastic-stitch': isSqlEditorParityActive && isElasticWorkspace,
        'chroma-stitch': isSqlEditorParityActive && isChromaWorkspace,
      }"
      :style="consoleSplitStyle"
      ref="consoleShell"
    >
      <ConsoleEntitiesPanel />

      <div
        class="console-splitter"
        @mousedown.prevent="startSplitResize"
        @dblclick.prevent="resetSplitWidth"
        @keydown.left.prevent="nudgeSplitWidth(-20)"
        @keydown.right.prevent="nudgeSplitWidth(20)"
        @keydown.home.prevent="resetSplitWidth"
        role="separator"
        aria-orientation="vertical"
        :aria-label="tApp('console.view.resizeEntitiesEditor')"
        tabindex="0"
      >
        <span class="console-splitter-grip" aria-hidden="true"></span>
      </div>

      <div
        class="panel console-panel console-panel--statement"
        :class="{
          'sql-editor-parity': isSqlEditorParityActive,
          'elastic-stitch': isSqlEditorParityActive && isElasticWorkspace,
          'chroma-stitch': isSqlEditorParityActive && isChromaWorkspace,
        }"
      >
        <RedisKeyInspector />
        <div
          class="console-editor-results-shell"
          :class="{ resizing: statementSplitResizing, 'sql-editor-parity': isSqlEditorParity }"
          :style="statementResultsSplitStyle"
          ref="statementResultsShell"
        >
          <ConsoleStatementPanel />
          <div
            class="console-editor-results-splitter"
            @mousedown.prevent="startStatementSplitResize"
            @dblclick.prevent="resetStatementSplitHeight"
            @keydown.up.prevent="nudgeStatementSplitHeight(-20)"
            @keydown.down.prevent="nudgeStatementSplitHeight(20)"
            @keydown.home.prevent="resetStatementSplitHeight"
            role="separator"
            aria-orientation="horizontal"
            :aria-label="tApp('console.view.resizeEditorResults')"
            tabindex="0"
          >
            <span class="console-editor-results-splitter-grip" aria-hidden="true"></span>
          </div>
          <ConsoleResultsPanel />
        </div>
      </div>
    </div>

    <ConsoleDangerDialogs />
  </section>
</template>
