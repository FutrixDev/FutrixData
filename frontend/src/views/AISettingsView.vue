<template>
  <section class="view active ai-settings-view">
    <div class="ai-settings-tabs" role="tablist">
      <button
        type="button"
        role="tab"
        class="ai-settings-tab"
        :class="{ active: activeTab === 'chat' }"
        :aria-selected="activeTab === 'chat'"
        @click="activeTab = 'chat'"
      >
        {{ tApp('ai.panel.tabChat') }}
      </button>
      <button
        type="button"
        role="tab"
        class="ai-settings-tab"
        :class="{ active: activeTab === 'embedding' }"
        :aria-selected="activeTab === 'embedding'"
        @click="activeTab = 'embedding'"
      >
        {{ tApp('ai.panel.tabEmbedding') }}
      </button>
    </div>

    <AIConfigPanel
      v-if="activeTab === 'chat'"
      :visible="true"
      inline
      split
      @create="openCreate"
      @edit="openEdit"
    />

    <EmbeddingConfigPanel
      v-if="activeTab === 'embedding'"
      @create="openEmbeddingCreate"
      @edit="openEmbeddingEdit"
    />
  </section>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import AIConfigPanel from '@/components/AIConfigPanel.vue'
import EmbeddingConfigPanel from '@/components/EmbeddingConfigPanel.vue'
import { tApp } from '@/modules/i18n/appI18n'

const router = useRouter()
const activeTab = ref<'chat' | 'embedding'>('chat')

const openCreate = () => {
  router.push({ name: 'ai-settings-create' })
}

const openEdit = (id: string) => {
  router.push({ name: 'ai-settings-edit', params: { id } })
}

const openEmbeddingCreate = () => {
  router.push({ name: 'ai-settings-embedding-create' })
}

const openEmbeddingEdit = (id: string) => {
  router.push({ name: 'ai-settings-embedding-edit', params: { id } })
}
</script>
