<template>
  <div class="ai-prefs">
    <div class="ai-prefs__header">
      <div class="ai-prefs__icon-wrap">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2a2 2 0 0 1 2 2c0 .74-.4 1.39-1 1.73v.01a2 2 0 0 1-2 0v-.01c-.6-.34-1-.99-1-1.73a2 2 0 0 1 2-2z"/><path d="m15 5 4.07 2.35a2 2 0 0 1 .93 1.72v4.86a2 2 0 0 1-.93 1.72L15 18l-3-1.73L9 18l-4.07-2.35a2 2 0 0 1-.93-1.72V9.07a2 2 0 0 1 .93-1.72L9 5l3 1.73L15 5z"/><circle cx="12" cy="12" r="3"/></svg>
      </div>
      <div>
        <h3 class="ai-prefs__title">{{ tApp('ai.preferences.title') }}</h3>
        <p class="ai-prefs__desc">{{ tApp('ai.preferences.subtitle') }}</p>
      </div>
    </div>

    <!-- Toggle: Default Open -->
    <label class="ai-prefs__toggle">
      <div class="ai-prefs__toggle-info">
        <svg class="ai-prefs__toggle-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="2" ry="2"/><line x1="9" y1="3" x2="9" y2="21"/></svg>
        <span class="ai-prefs__toggle-label">{{ tApp('ai.preferences.defaultOpen') }}</span>
      </div>
      <span class="ai-prefs__switch" :class="{ 'ai-prefs__switch--on': store.prefs.defaultOpen }">
        <input
          id="ai-default-open"
          name="ai-default-open"
          data-testid="ai-default-open"
          type="checkbox"
          :checked="store.prefs.defaultOpen"
          @change="store.setDefaultOpen(($event.target as HTMLInputElement).checked)"
        />
        <span class="ai-prefs__switch-track">
          <span class="ai-prefs__switch-thumb" />
        </span>
      </span>
    </label>

    <!-- Retention -->
    <label class="ai-prefs__field">
      <div class="ai-prefs__field-info">
        <svg class="ai-prefs__field-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>
        <span class="ai-prefs__field-label">{{ tApp('ai.preferences.retention') }}</span>
      </div>
      <input
        id="ai-retention"
        name="ai-retention"
        class="ai-prefs__number"
        data-testid="ai-retention"
        type="number"
        min="1"
        autocapitalize="off"
        autocorrect="off"
        spellcheck="false"
        :value="store.prefs.retention"
        @input="store.setRetentionLimit(Number(($event.target as HTMLInputElement).value))"
      />
    </label>

  </div>
</template>

<script setup lang="ts">
import { useAiChatStore } from '@/stores/ai-chat'
import { tApp } from '@/modules/i18n/appI18n'

const store = useAiChatStore()
</script>
