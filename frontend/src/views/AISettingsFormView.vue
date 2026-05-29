<template>
  <section class="view active ai-settings-form-view">
    <div class="ai-settings-form-shell">
      <EmbeddingConfigForm
        v-if="isEmbeddingRoute"
        :mode="mode"
        :config-id="configId"
        @close="closeForm"
      />
      <AIConfigForm
        v-else
        :visible="true"
        :mode="mode"
        :config-id="configId"
        inline
        @close="closeForm"
      />
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AIConfigForm from '@/components/AIConfigForm.vue'
import EmbeddingConfigForm from '@/components/EmbeddingConfigForm.vue'

const route = useRoute()
const router = useRouter()

const isEmbeddingRoute = computed(() =>
  route.name === 'ai-settings-embedding-create' || route.name === 'ai-settings-embedding-edit'
)
const mode = computed(() =>
  (route.name === 'ai-settings-edit' || route.name === 'ai-settings-embedding-edit') ? 'edit' : 'create'
)
const configId = computed(() => (typeof route.params.id === 'string' ? route.params.id : null))

const closeForm = () => {
  router.push({ name: 'ai-settings' })
}
</script>
